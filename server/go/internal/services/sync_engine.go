package services

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
	rsssync "github.com/rss/go-server/internal/sync"
)

// SyncEngine 远端同步引擎：订阅对齐、增量拉取、状态回推、周期 Pull。
type SyncEngine struct {
	service *ReaderService

	// syncing 手动同步进行中标记（防重入）
	syncMu  sync.Mutex
	syncing bool

	// pending 状态回推队列：kind -> 远端条目 ID 集合
	pendingMu sync.Mutex
	pending   map[rsssync.StateChangeKind]map[int64]struct{}
	// flushTimer 防抖定时器（2s 聚合批量推送）
	flushTimer *time.Timer

	// newProvider provider 构建入口（默认按配置构建；测试注入 mock）
	newProvider func() (rsssync.RemoteProvider, error)

	// 周期 Pull 调度（P1）
	schedMu       sync.Mutex
	schedRunning  bool
	schedStop     chan struct{}
	schedWg       sync.WaitGroup
	schedInterval time.Duration // 检查节拍（默认 1 分钟，测试可注入）
	lastSchedPull time.Time
	schedFailures int // 连续失败次数（退避倍率）

	// fullPullCount GReader 全量校准计数（M-R3）：ot 增量游标不覆盖状态变更，每 N 次做一次全量拉取
	fullPullCount int

	// fileHTTPClient 文件后端探活用 HTTP 客户端（默认 BuildFetchHTTPClient，测试可注入绕过 SSRF）
	fileHTTPClient *http.Client
}

// SyncResult 一次手动同步的结果
type SyncResult struct {
	FeedsImported int      `json:"feedsImported"`
	FeedsMerged   int      `json:"feedsMerged"`
	ItemsUpserted int      `json:"itemsUpserted"`
	LocalOnlyURLs []string `json:"localOnlyUrls"`
}

// SyncStatus 同步健康状态（供前端指示器）
type SyncStatus struct {
	Enabled    bool   `json:"enabled"`
	Provider   string `json:"provider"`
	Syncing    bool   `json:"syncing"`
	LastPullAt string `json:"lastPullAt"`
	LastResult string `json:"lastResult"`
	LastError  string `json:"lastError"`
}

// NewSyncEngine 创建同步引擎
func NewSyncEngine(service *ReaderService) *SyncEngine {
	return &SyncEngine{
		service: service,
		pending: make(map[rsssync.StateChangeKind]map[int64]struct{}),
		newProvider: func() (rsssync.RemoteProvider, error) {
			return service.buildSyncProvider()
		},
	}
}

// buildSyncProvider 按当前配置构建 provider（每次同步重建，配置变更即时生效）
func (s *ReaderService) buildSyncProvider() (rsssync.RemoteProvider, error) {
	if !s.GetSettingBool("sync_api_enabled", false) {
		return nil, fmt.Errorf("sync api not enabled")
	}
	provider := s.GetSetting("sync_api_provider")
	endpoint := s.GetSetting("sync_api_endpoint")
	if endpoint == "" {
		return nil, fmt.Errorf("sync api endpoint is empty")
	}
	switch provider {
	case rsssync.ProviderMiniflux:
		token := s.GetSetting("sync_api_token")
		if token == "" {
			return nil, fmt.Errorf("sync api token is empty")
		}
		return rsssync.NewMinifluxProvider(endpoint, token, s.BuildFetchHTTPClient()), nil
	case rsssync.ProviderGReader:
		username := s.GetSetting("sync_api_username")
		password := s.GetSetting("sync_api_password")
		if username == "" || password == "" {
			return nil, fmt.Errorf("sync api username or password is empty")
		}
		return rsssync.NewGReaderProvider(endpoint, username, password, s.BuildFetchHTTPClient()), nil
	default:
		return nil, fmt.Errorf("sync api provider %q is not available yet", provider)
	}
}

// StartPeriodicPull 启动周期 Pull 后台循环（幂等；随 ReaderService 关闭）
func (e *SyncEngine) StartPeriodicPull() {
	e.schedMu.Lock()
	if e.schedRunning {
		e.schedMu.Unlock()
		return
	}
	e.schedRunning = true
	e.schedStop = make(chan struct{})
	stop := e.schedStop
	e.schedMu.Unlock()

	e.schedWg.Add(1)
	go e.scheduleLoop(stop)
}

// StopPeriodicPull 停止周期 Pull 循环并等待退出
func (e *SyncEngine) StopPeriodicPull() {
	e.schedMu.Lock()
	if !e.schedRunning {
		e.schedMu.Unlock()
		return
	}
	e.schedRunning = false
	close(e.schedStop)
	e.schedMu.Unlock()
	e.schedWg.Wait()
}

// scheduleLoop 每个节拍检查一次是否到期；启动即做一次初始 Pull（不阻塞）
func (e *SyncEngine) scheduleLoop(stop chan struct{}) {
	defer e.schedWg.Done()
	interval := e.schedInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	e.scheduledPull()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			e.scheduledPull()
		}
	}
}

// scheduledPull 到期执行一轮增量 Pull；间隔为 sync_pull_interval * 失败退避倍数。
// 同步未启用、仅手动（interval<=0）或手动同步进行中时跳过（后者不更新游标，下轮再试）。
func (e *SyncEngine) scheduledPull() {
	if !e.service.GetSettingBool("sync_api_enabled", false) {
		return
	}
	if e.service.GetSettingInt("sync_pull_interval", 300) <= 0 {
		return
	}
	e.schedMu.Lock()
	due := time.Since(e.lastSchedPull) >= e.effectivePullIntervalLocked()
	e.schedMu.Unlock()
	if !due {
		return
	}

	e.syncMu.Lock()
	busy := e.syncing
	e.syncMu.Unlock()
	if busy {
		return
	}

	e.schedMu.Lock()
	e.lastSchedPull = time.Now()
	e.schedMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := e.TriggerFullSync(ctx); err != nil {
		e.schedMu.Lock()
		if e.schedFailures < 5 {
			e.schedFailures++
		}
		failures := e.schedFailures
		e.schedMu.Unlock()
		slog.Warn("sync engine: scheduled pull failed",
			"error", err, "failures", failures)
		return
	}
	e.schedMu.Lock()
	e.schedFailures = 0
	e.schedMu.Unlock()
}

// effectivePullIntervalLocked 计算生效间隔：基础间隔（sync_pull_interval 秒）按
// 连续失败次数翻倍退避，上限 30 分钟。调用方需持有 schedMu。
func (e *SyncEngine) effectivePullIntervalLocked() time.Duration {
	base := time.Duration(e.service.GetSettingInt("sync_pull_interval", 300)) * time.Second
	multiplier := int64(1) << min(e.schedFailures, 4)
	interval := base * time.Duration(multiplier)
	if interval > 30*time.Minute {
		interval = 30 * time.Minute
	}
	return interval
}

// greaderFullPullEvery GReader 全量校准间隔：每 N 次增量拉取后做一次不带 ot 的全量拉取。
// GReader 的 ot 增量游标按 published 过滤，状态变更不更新 published，
// 全量校准可补回远端设备的状态变更。
const greaderFullPullEvery = 10

// computePullSince 计算本轮拉取起点。
// 非 GReader：上次成功游标回退 1 分钟（容忍时钟偏差，靠 link 对齐幂等）；
// GReader：每 greaderFullPullEvery 次增量后返回零值触发全量校准，否则沿用增量游标。
func (e *SyncEngine) computePullSince(provider rsssync.RemoteProvider) time.Time {
	lastPullMs := int64(e.service.GetSettingInt("sync_last_pull_at", 0))
	since := time.UnixMilli(lastPullMs).Add(-time.Minute)
	if provider.Name() != rsssync.ProviderGReader {
		return since
	}
	e.fullPullCount++
	if e.fullPullCount < greaderFullPullEvery {
		return since
	}
	e.fullPullCount = 0
	return time.Time{}
}

// TestConnection 检测当前配置的可达性与认证
func (e *SyncEngine) TestConnection() error {
	provider, err := e.newProvider()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return provider.HealthCheck(ctx)
}

// TestFileConnection 检测文件后端（WebDAV）连通性。
// 仅做可达性与认证探活，不校验存储内容。
func (e *SyncEngine) TestFileConnection() error {
	provider := e.service.GetSetting("sync_file_provider")
	switch provider {
	case "webdav":
		return e.testWebDAVConnection()
	case "":
		return fmt.Errorf("file provider not configured")
	default:
		return fmt.Errorf("file provider %q not supported", provider)
	}
}

// testWebDAVConnection 对 WebDAV 地址发 PROPFIND 探活（Depth: 0，携带 Basic Auth）。
// 2xx 通过；401/403 认证失败（ErrNotAuthorized 可识别）；其余状态返回状态码。
func (e *SyncEngine) testWebDAVConnection() error {
	webdavURL := e.service.GetSetting("sync_file_url")
	if webdavURL == "" {
		return fmt.Errorf("webdav url is empty")
	}
	username := e.service.GetSetting("sync_file_username")
	password := e.service.GetSetting("sync_file_password")

	req, err := http.NewRequest("PROPFIND", webdavURL, nil)
	if err != nil {
		return fmt.Errorf("webdav invalid url: %w", err)
	}
	req.Header.Set("Depth", "0")
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := e.fileProbeHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("webdav unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: webdav authentication failed (http %d)", rsssync.ErrNotAuthorized, resp.StatusCode)
	}
	return fmt.Errorf("webdav probe failed (http %d)", resp.StatusCode)
}

// fileProbeHTTPClient 返回文件后端探活客户端：优先测试注入，默认复用抓取客户端（含 SSRF 防护）
func (e *SyncEngine) fileProbeHTTPClient() *http.Client {
	if e.fileHTTPClient != nil {
		return e.fileHTTPClient
	}
	return e.service.BuildFetchHTTPClient()
}

// TriggerFullSync 手动触发全量同步：订阅对齐 -> 增量拉取 -> upsert。
// 重入保护：已在同步中时直接返回错误。
func (e *SyncEngine) TriggerFullSync(ctx context.Context) (*SyncResult, error) {
	e.syncMu.Lock()
	if e.syncing {
		e.syncMu.Unlock()
		return nil, fmt.Errorf("sync already in progress")
	}
	e.syncing = true
	e.syncMu.Unlock()
	defer func() {
		e.syncMu.Lock()
		e.syncing = false
		e.syncMu.Unlock()
	}()

	provider, err := e.newProvider()
	if err != nil {
		e.recordSyncResult("failed", err.Error())
		return nil, err
	}

	start := time.Now()
	result := &SyncResult{LocalOnlyURLs: []string{}}

	// 1. 订阅对齐
	feeds, err := provider.PullSubscriptions(ctx)
	if err != nil {
		e.recordSyncResult("failed", err.Error())
		return nil, err
	}
	if err := e.alignSubscriptions(feeds, provider.Name(), result); err != nil {
		e.recordSyncResult("failed", err.Error())
		return nil, err
	}

	// 2. 增量拉取（游标回退 1 分钟容忍时钟偏差，靠 link 对齐幂等）
	since := e.computePullSince(provider)
	items, err := provider.PullItems(ctx, since)
	if err != nil {
		e.recordSyncResult("failed", err.Error())
		return nil, err
	}

	// 3. 写入本地
	if err := e.upsertRemoteItems(items); err != nil {
		e.recordSyncResult("failed", err.Error())
		return nil, err
	}
	result.ItemsUpserted = len(items)

	if err := e.service.UpdateSettings(map[string]string{
		"sync_last_pull_at": strconv.FormatInt(time.Now().UnixMilli(), 10),
	}); err != nil {
		slog.Warn("sync engine: update pull cursor failed", "error", err)
	}
	e.recordSyncResult("ok", fmt.Sprintf("%d feeds, %d items in %s",
		len(feeds), len(items), time.Since(start).Round(time.Millisecond)))
	slog.Info("sync engine: full sync done",
		"provider", provider.Name(), "feeds", len(feeds), "items", len(items))
	return result, nil
}

// GetSyncStatus 当前同步状态（供指示器轮询）
func (e *SyncEngine) GetSyncStatus() *SyncStatus {
	e.syncMu.Lock()
	syncing := e.syncing
	e.syncMu.Unlock()
	status := &SyncStatus{
		Enabled:    e.service.GetSettingBool("sync_api_enabled", false),
		Provider:   e.service.GetSetting("sync_api_provider"),
		Syncing:    syncing,
		LastPullAt: e.service.GetSetting("sync_last_pull_at"),
		LastResult: e.service.GetSetting("sync_last_result"),
		LastError:  e.service.GetSetting("sync_last_error"),
	}
	return status
}

// AlignResult 对齐向导执行结果
type AlignResult struct {
	Pushed int `json:"pushed"`
}

// PushLocalOnlySources 对齐向导：将本地独有源推送到远端
func (e *SyncEngine) PushLocalOnlySources(ctx context.Context, urls []string) (*AlignResult, error) {
	provider, err := e.newProvider()
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return &AlignResult{}, nil
	}
	var sources []models.Source
	if err := e.service.getDb().Where("url IN ? AND remoteOrigin = ''", urls).Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("sync align: query local sources failed: %w", err)
	}
	pushed := 0
	for _, source := range sources {
		folderName := ""
		if source.FolderID != nil {
			var folder models.Folder
			if err := e.service.getDb().Where("id = ?", *source.FolderID).First(&folder).Error; err == nil {
				folderName = folder.Name
			}
		}
		op := rsssync.SubscriptionOp{
			Op:         rsssync.OpSubscribe,
			URL:        source.URL,
			Title:      source.Name,
			FolderName: folderName,
		}
		if err := provider.PushSubscription(ctx, op); err != nil {
			slog.Warn("sync engine: push local source failed", "url", source.URL, "error", err)
			continue
		}
		pushed++
	}
	return &AlignResult{Pushed: pushed}, nil
}

// RecordStateChange 本地状态变更入口（MarkRead/MarkStar 调用）。
// 目标 item 不属于同步源或无远端 ID 时静默跳过。
func (e *SyncEngine) RecordStateChange(itemID int, kind rsssync.StateChangeKind) {
	var row struct {
		RemoteID     string `gorm:"column:remoteId"`
		RemoteOrigin string `gorm:"column:remoteOrigin"`
	}
	// 表名为 Prisma 风格（Item/Source），按 GORM 模型别名拼接避免手写表名漂移
	if err := e.service.getDb().Model(&models.Item{}).
		Select("`Item`.remoteId as remoteId, `Source`.remoteOrigin as remoteOrigin").
		Joins("JOIN `Source` ON `Source`.id = `Item`.sourceId").
		Where("`Item`.id = ?", itemID).Scan(&row).Error; err != nil {
		return
	}
	if row.RemoteOrigin == "" || row.RemoteID == "" {
		return
	}
	remoteID, err := strconv.ParseInt(row.RemoteID, 10, 64)
	if err != nil {
		return
	}

	e.pendingMu.Lock()
	if e.pending[kind] == nil {
		e.pending[kind] = make(map[int64]struct{})
	}
	e.pending[kind][remoteID] = struct{}{}
	if e.flushTimer == nil {
		e.flushTimer = time.AfterFunc(2*time.Second, e.flushPendingStates)
	}
	e.pendingMu.Unlock()
}

// flushPendingStates 防抖到期后批量推送；失败时回滚队列等待下次重试
func (e *SyncEngine) flushPendingStates() {
	e.pendingMu.Lock()
	snapshot := e.pending
	e.pending = make(map[rsssync.StateChangeKind]map[int64]struct{})
	e.flushTimer = nil
	e.pendingMu.Unlock()
	if len(snapshot) == 0 {
		return
	}

	provider, err := e.newProvider()
	if err != nil {
		e.restorePending(snapshot)
		slog.Warn("sync engine: flush states skipped", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	states := make([]rsssync.StateChange, 0, len(snapshot))
	for kind, ids := range snapshot {
		list := make([]int64, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		states = append(states, rsssync.StateChange{RemoteIDs: list, Kind: kind})
	}
	if err := provider.PushStates(ctx, states); err != nil {
		e.restorePending(snapshot)
		e.recordSyncResult("failed", err.Error())
		slog.Warn("sync engine: push states failed, will retry", "error", err)
		return
	}
	slog.Info("sync engine: pushed states", "batches", len(states))
}

// restorePending 推送失败时恢复队列（合并，后续变更/下次触发重试）
func (e *SyncEngine) restorePending(snapshot map[rsssync.StateChangeKind]map[int64]struct{}) {
	e.pendingMu.Lock()
	defer e.pendingMu.Unlock()
	for kind, ids := range snapshot {
		if e.pending[kind] == nil {
			e.pending[kind] = make(map[int64]struct{}, len(ids))
		}
		for id := range ids {
			e.pending[kind][id] = struct{}{}
		}
	}
	// 无定时器时补一个重试定时器（30s）
	if e.flushTimer == nil {
		e.flushTimer = time.AfterFunc(30*time.Second, e.flushPendingStates)
	}
}

// recordSyncResult 记录同步结果到设置表（status 接口透出）
func (e *SyncEngine) recordSyncResult(result, errMessage string) {
	values := map[string]string{
		"sync_last_result": result,
	}
	if errMessage != "" {
		values["sync_last_error"] = errMessage
	} else {
		values["sync_last_error"] = ""
	}
	if err := e.service.UpdateSettings(values); err != nil {
		slog.Warn("sync engine: record result failed", "error", err)
	}
}

// alignSubscriptions 订阅对齐：URL 对齐导入/合并；本地独有源仅上报（向导决定是否推送）
func (e *SyncEngine) alignSubscriptions(feeds []rsssync.RemoteFeed, origin string, result *SyncResult) error {
	db := e.service.getDb()
	var sources []models.Source
	if err := db.Find(&sources).Error; err != nil {
		return fmt.Errorf("sync align: query sources failed: %w", err)
	}
	byURL := make(map[string]*models.Source, len(sources))
	for i := range sources {
		byURL[sources[i].URL] = &sources[i]
	}

	for _, feed := range feeds {
		if feed.URL == "" {
			continue
		}
		folderID, err := e.resolveFolderIDByName(feed.FolderName)
		if err != nil {
			return err
		}
		if existing, ok := byURL[feed.URL]; ok {
			// 双方都有：标记为同步源，远端 ID 与文件夹归属以远端为准
			updates := map[string]interface{}{
				"remoteOrigin": origin,
				"remoteId":     strconv.FormatInt(feed.RemoteID, 10),
				"folderId":     folderID,
			}
			if err := db.Model(existing).Updates(updates).Error; err != nil {
				return fmt.Errorf("sync align: merge source failed: %w", err)
			}
			result.FeedsMerged++
		} else {
			now := time.Now()
			source := models.Source{
				Name:          feed.Title,
				URL:           feed.URL,
				FolderID:      folderID,
				ListRule:      "rss",
				Interval:      120,
				Active:        true,
				RemoteOrigin:  origin,
				RemoteID:      strconv.FormatInt(feed.RemoteID, 10),
				CreatedAtTime: models.MilliTime{T: now},
				UpdatedAtTime: models.MilliTime{T: now},
			}
			if err := db.Create(&source).Error; err != nil {
				return fmt.Errorf("sync align: import source failed: %w", err)
			}
			result.FeedsImported++
		}
	}

	// 本地独有源：仅上报远端不存在的本地源（向导决策是否推送）。
	// 注意 sources 是合并前的快照，「双方都有」的源内存值 remoteOrigin 仍为空，需按远端 URL 集合排除。
	remoteURLs := make(map[string]struct{}, len(feeds))
	for _, feed := range feeds {
		if feed.URL != "" {
			remoteURLs[feed.URL] = struct{}{}
		}
	}
	for _, source := range sources {
		if source.RemoteOrigin == "" {
			if _, ok := remoteURLs[source.URL]; ok {
				continue
			}
			result.LocalOnlyURLs = append(result.LocalOnlyURLs, source.URL)
		}
	}
	return nil
}

// resolveFolderIDByName 按名称查找顶层文件夹，不存在则创建；空名称返回 nil（未分组）
func (e *SyncEngine) resolveFolderIDByName(name string) (*int, error) {
	if name == "" {
		return nil, nil
	}
	var folder models.Folder
	if err := e.service.getDb().Where("name = ?", name).First(&folder).Error; err == nil {
		return &folder.ID, nil
	}
	created, err := e.service.CreateFolder(name)
	if err != nil {
		return nil, fmt.Errorf("sync align: create folder failed: %w", err)
	}
	return &created.ID, nil
}

// pendingRemoteIDs 返回待推送状态变更涉及的远端条目 ID 快照。
// upsert 时用于排除：本地变更尚未推送，远端旧值不得覆盖。
func (e *SyncEngine) pendingRemoteIDs() map[int64]struct{} {
	e.pendingMu.Lock()
	defer e.pendingMu.Unlock()
	ids := make(map[int64]struct{})
	for _, set := range e.pending {
		for id := range set {
			ids[id] = struct{}{}
		}
	}
	return ids
}

// upsertRemoteItems 远端条目写入本地：按 (sourceId, link) 对齐，LWW 覆盖状态
func (e *SyncEngine) upsertRemoteItems(items []rsssync.RemoteItem) error {
	if len(items) == 0 {
		return nil
	}
	db := e.service.getDb()
	// 待推送状态快照：这些条目的本地已读/星标较远端新，跳过状态覆盖
	pendingIDs := e.pendingRemoteIDs()

	// sourceURL -> sourceID 映射（仅同步源）
	urls := make([]string, 0, len(items))
	seenURL := make(map[string]struct{})
	for _, item := range items {
		if _, ok := seenURL[item.SourceURL]; !ok {
			seenURL[item.SourceURL] = struct{}{}
			urls = append(urls, item.SourceURL)
		}
	}
	var sources []models.Source
	if err := db.Where("url IN ? AND remoteOrigin != ''", urls).Find(&sources).Error; err != nil {
		return fmt.Errorf("sync upsert: query sources failed: %w", err)
	}
	sourceIDByURL := make(map[string]int, len(sources))
	for _, source := range sources {
		sourceIDByURL[source.URL] = source.ID
	}

	var newIDs []int
	// changedIDs 标题/正文变更的已存在条目：重建 FTS 索引以检索到更新内容
	var changedIDs []int
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			sourceID, ok := sourceIDByURL[item.SourceURL]
			if !ok || item.Link == "" {
				continue
			}
			remoteID := strconv.FormatInt(item.RemoteID, 10)
			var existing models.Item
			if err := tx.Where("sourceId = ? AND link = ?", sourceID, item.Link).First(&existing).Error; err == nil {
				titleChanged := existing.Title != item.Title
				contentChanged := item.Content != "" && (existing.Desc == nil || *existing.Desc != item.Content)
				updates := map[string]interface{}{
					"title":    item.Title,
					"remoteId": remoteID,
				}
				// 状态字段仅在无待推送变更时覆盖，否则保留本地较新值
				if _, pending := pendingIDs[item.RemoteID]; !pending {
					updates["isRead"] = item.IsRead
					updates["isStarred"] = item.IsStarred
				}
				if item.Content != "" {
					updates["desc"] = &item.Content
				}
				if item.Author != "" {
					updates["author"] = &item.Author
				}
				if !item.PubDate.IsZero() {
					updates["pubDate"] = models.NullableMilliTime{T: &item.PubDate}
				}
				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return err
				}
				if titleChanged || contentChanged {
					changedIDs = append(changedIDs, existing.ID)
				}
			} else {
				desc := item.Content
				author := item.Author
				pubDate := models.NullableMilliTime{}
				if !item.PubDate.IsZero() {
					pubDate = models.NullableMilliTime{T: &item.PubDate}
				}
				newItem := models.Item{
					SourceID:  sourceID,
					Title:     item.Title,
					Link:      item.Link,
					Desc:      &desc,
					Author:    &author,
					PubDate:   pubDate,
					IsRead:    item.IsRead,
					IsStarred: item.IsStarred,
					RemoteID:  remoteID,
				}
				if err := tx.Create(&newItem).Error; err != nil {
					return err
				}
				newIDs = append(newIDs, newItem.ID)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("sync upsert: write items failed: %w", err)
	}

	// 新条目 + 标题/正文变更条目推入后台索引（FTS5 + 过滤规则），非阻塞
	indexIDs := append(newIDs, changedIDs...)
	if len(indexIDs) > 0 {
		if len(newIDs) > 0 {
			e.service.invalidateUnreadCount()
		}
		for _, id := range indexIDs {
			select {
			case e.service.indexChan <- id:
			default:
				select {
				case e.service.indexOverflow <- id:
				default:
					slog.Warn("index overflow full, item search index dropped", "item_id", id)
				}
			}
		}
	}
	return nil
}
