package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
	rsssync "github.com/rss/go-server/internal/sync"
)

// countingSyncProvider 计数版 mock（周期调度测试用）
type countingSyncProvider struct {
	mockSyncProvider
	pulls atomic.Int64
}

func (c *countingSyncProvider) PullSubscriptions(ctx context.Context) ([]rsssync.RemoteFeed, error) {
	c.pulls.Add(1)
	return nil, nil
}

// mockSyncProvider 内存 mock：记录推送调用，返回预设数据
type mockSyncProvider struct {
	feeds        []rsssync.RemoteFeed
	items        []rsssync.RemoteItem
	pushedSubs   []rsssync.SubscriptionOp
	pushedStates []rsssync.StateChange
}

func (m *mockSyncProvider) Name() string { return "mock" }

func (m *mockSyncProvider) PullSubscriptions(ctx context.Context) ([]rsssync.RemoteFeed, error) {
	return m.feeds, nil
}

func (m *mockSyncProvider) PushSubscription(ctx context.Context, op rsssync.SubscriptionOp) error {
	m.pushedSubs = append(m.pushedSubs, op)
	return nil
}

func (m *mockSyncProvider) PullItems(ctx context.Context, since time.Time) ([]rsssync.RemoteItem, error) {
	return m.items, nil
}

func (m *mockSyncProvider) PushStates(ctx context.Context, states []rsssync.StateChange) error {
	m.pushedStates = append(m.pushedStates, states...)
	return nil
}

func (m *mockSyncProvider) HealthCheck(ctx context.Context) error { return nil }

// newSyncEngineTest 内存库 + 引擎，注入 mock provider
func newSyncEngineTest(t *testing.T) (*ReaderService, *SyncEngine, *mockSyncProvider) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.Folder{}, &models.Source{}, &models.Item{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	service := &ReaderService{
		db:            db,
		indexChan:     make(chan int, 64),
		indexOverflow: make(chan int, 64),
	}
	engine := NewSyncEngine(service)
	mock := &mockSyncProvider{}
	engine.newProvider = func() (rsssync.RemoteProvider, error) { return mock, nil }
	return service, engine, mock
}

func createTestSource(t *testing.T, db *gorm.DB, name, url, remoteOrigin, remoteID string, folderID *int) models.Source {
	t.Helper()
	source := models.Source{
		Name:         name,
		URL:          url,
		ListRule:     "rss",
		Interval:     120,
		Active:       true,
		FolderID:     folderID,
		RemoteOrigin: remoteOrigin,
		RemoteID:     remoteID,
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	return source
}

// TestSyncEngineFullSyncThreeBranches 验证订阅对齐三分支：
// 远端独有（导入）、双方都有（合并标记）、本地独有（仅上报不推送）
func TestSyncEngineFullSyncThreeBranches(t *testing.T) {
	_, engine, mock := newSyncEngineTest(t)
	db := engine.service.getDb()

	// 本地预置：与远端 URL 相同的源 + 纯本地源
	createTestSource(t, db, "A local", "https://a.com/feed.xml", "", "", nil)
	createTestSource(t, db, "Local only", "https://local.com/feed.xml", "", "", nil)

	mock.feeds = []rsssync.RemoteFeed{
		{RemoteID: 42, URL: "https://a.com/feed.xml", Title: "A remote", FolderName: "Tech"},
		{RemoteID: 43, URL: "https://new.com/feed.xml", Title: "New feed"},
	}
	mock.items = []rsssync.RemoteItem{
		{
			RemoteID: 100, SourceRemoteID: 43, SourceURL: "https://new.com/feed.xml",
			Link: "https://new.com/article/1", Title: "New article",
			Content: "<p>content</p>", Author: "author", IsRead: true,
		},
	}

	result, err := engine.TriggerFullSync(context.Background())
	if err != nil {
		t.Fatalf("full sync: %v", err)
	}

	// 对齐结果：1 合并 + 1 导入；LocalOnly 仅含纯本地源（「双方都有」不误报）
	if result.FeedsMerged != 1 || result.FeedsImported != 1 {
		t.Fatalf("align counts mismatch: merged=%d imported=%d", result.FeedsMerged, result.FeedsImported)
	}
	if len(result.LocalOnlyURLs) != 1 || result.LocalOnlyURLs[0] != "https://local.com/feed.xml" {
		t.Fatalf("LocalOnlyURLs mismatch: %v", result.LocalOnlyURLs)
	}
	if result.ItemsUpserted != 1 {
		t.Fatalf("expected 1 item upserted, got %d", result.ItemsUpserted)
	}

	// 双方都有：合并标记为同步源，文件夹以远端归类为准
	var merged models.Source
	if err := db.Where("url = ?", "https://a.com/feed.xml").First(&merged).Error; err != nil {
		t.Fatalf("query merged source: %v", err)
	}
	if merged.RemoteOrigin != "mock" || merged.RemoteID != "42" {
		t.Fatalf("merged source remote fields mismatch: %+v", merged)
	}
	if merged.FolderID == nil {
		t.Fatal("expected merged source assigned to folder Tech")
	}
	var folder models.Folder
	if err := db.Where("id = ?", *merged.FolderID).First(&folder).Error; err != nil || folder.Name != "Tech" {
		t.Fatalf("expected folder Tech, got err=%v folder=%+v", err, folder)
	}

	// 远端独有：导入为同步源
	var imported models.Source
	if err := db.Where("url = ?", "https://new.com/feed.xml").First(&imported).Error; err != nil {
		t.Fatalf("query imported source: %v", err)
	}
	if imported.RemoteOrigin != "mock" || imported.RemoteID != "43" {
		t.Fatalf("imported source remote fields mismatch: %+v", imported)
	}

	// 条目落库：状态与远端一致
	var item models.Item
	if err := db.Where("sourceId = ?", imported.ID).First(&item).Error; err != nil {
		t.Fatalf("query synced item: %v", err)
	}
	if item.Link != "https://new.com/article/1" || item.RemoteID != "100" || !item.IsRead {
		t.Fatalf("synced item mismatch: %+v", item)
	}
}

// TestSyncEngineUpsertLWW 验证条目按 (sourceId, link) 对齐，
// 已存在条目被远端状态覆盖（LWW），新条目创建
func TestSyncEngineUpsertLWW(t *testing.T) {
	_, engine, _ := newSyncEngineTest(t)
	db := engine.service.getDb()

	source := createTestSource(t, db, "Synced", "https://a.com/feed.xml", "mock", "42", nil)

	existingDesc := "old"
	existing := models.Item{
		SourceID: source.ID, Title: "Old title", Link: "https://a.com/existing",
		Desc: &existingDesc, IsRead: false,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing item: %v", err)
	}

	err := engine.upsertRemoteItems([]rsssync.RemoteItem{
		{
			RemoteID: 200, SourceRemoteID: 42, SourceURL: "https://a.com/feed.xml",
			Link: "https://a.com/existing", Title: "New title", IsRead: true,
		},
		{
			RemoteID: 201, SourceRemoteID: 42, SourceURL: "https://a.com/feed.xml",
			Link: "https://a.com/fresh", Title: "Fresh", IsRead: false, IsStarred: true,
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// 已存在：标题/状态/远端 ID 覆盖
	var updated models.Item
	if err := db.Where("link = ?", "https://a.com/existing").First(&updated).Error; err != nil {
		t.Fatalf("query updated item: %v", err)
	}
	if updated.Title != "New title" || !updated.IsRead || updated.RemoteID != "200" {
		t.Fatalf("LWW overwrite mismatch: %+v", updated)
	}

	// 新建：字段完整
	var fresh models.Item
	if err := db.Where("link = ?", "https://a.com/fresh").First(&fresh).Error; err != nil {
		t.Fatalf("query fresh item: %v", err)
	}
	if fresh.RemoteID != "201" || !fresh.IsStarred || fresh.IsRead {
		t.Fatalf("fresh item mismatch: %+v", fresh)
	}

	// 条目总数：无重复
	var count int64
	if err := db.Model(&models.Item{}).Where("sourceId = ?", source.ID).Count(&count).Error; err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 items, got %d", count)
	}
}

// TestSyncEngineStateChangeDebounce 验证状态回推队列：
// 同步源条目入队、同 ID 去重聚合、本地源条目跳过、批量推送格式
func TestSyncEngineStateChangeDebounce(t *testing.T) {
	_, engine, mock := newSyncEngineTest(t)
	db := engine.service.getDb()

	syncedSource := createTestSource(t, db, "Synced", "https://a.com/feed.xml", "mock", "42", nil)
	localSource := createTestSource(t, db, "Local", "https://local.com/feed.xml", "", "", nil)

	newItem := func(sourceID int, link, remoteID string) models.Item {
		desc := "d"
		item := models.Item{SourceID: sourceID, Title: link, Link: link, Desc: &desc, RemoteID: remoteID}
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("create item: %v", err)
		}
		return item
	}
	item77 := newItem(syncedSource.ID, "https://a.com/77", "77")
	item88 := newItem(syncedSource.ID, "https://a.com/88", "88")
	itemLocal := newItem(localSource.ID, "https://local.com/1", "")

	// 同 remoteID 记录两次（去重）+ 不同条目一次；本地源条目不入队
	engine.RecordStateChange(item77.ID, rsssync.StateChangeRead)
	engine.RecordStateChange(item77.ID, rsssync.StateChangeRead)
	engine.RecordStateChange(item88.ID, rsssync.StateChangeRead)
	engine.RecordStateChange(itemLocal.ID, rsssync.StateChangeRead)

	// 停掉 2s 防抖定时器，同步执行 flush
	engine.pendingMu.Lock()
	if engine.flushTimer != nil {
		engine.flushTimer.Stop()
		engine.flushTimer = nil
	}
	engine.pendingMu.Unlock()
	engine.flushPendingStates()

	if len(mock.pushedStates) != 1 {
		t.Fatalf("expected 1 batch push, got %d", len(mock.pushedStates))
	}
	batch := mock.pushedStates[0]
	if batch.Kind != rsssync.StateChangeRead {
		t.Fatalf("expected read kind, got %s", batch.Kind)
	}
	ids := map[int64]struct{}{}
	for _, id := range batch.RemoteIDs {
		ids[id] = struct{}{}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique remote IDs (77, 88), got %v", batch.RemoteIDs)
	}
	if _, ok := ids[77]; !ok {
		t.Fatalf("expected remote ID 77 in batch, got %v", batch.RemoteIDs)
	}
	if _, ok := ids[88]; !ok {
		t.Fatalf("expected remote ID 88 in batch, got %v", batch.RemoteIDs)
	}

	// flush 后队列清空：再次 flush 无新增推送
	engine.flushPendingStates()
	if len(mock.pushedStates) != 1 {
		t.Fatalf("expected no extra push after empty flush, got %d", len(mock.pushedStates))
	}
}

// TestSyncEnginePushLocalOnlySources 对齐向导：本地独有源推送远端（带文件夹归类）
func TestSyncEnginePushLocalOnlySources(t *testing.T) {
	_, engine, mock := newSyncEngineTest(t)
	db := engine.service.getDb()

	folder := models.Folder{Name: "News"}
	if err := db.Create(&folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}
	createTestSource(t, db, "Pushed", "https://push.com/feed.xml", "", "", &folder.ID)
	createTestSource(t, db, "Remote", "https://remote.com/feed.xml", "mock", "1", nil)

	result, err := engine.PushLocalOnlySources(context.Background(), []string{
		"https://push.com/feed.xml",
		"https://remote.com/feed.xml",  // 同步源，不应被推送
		"https://missing.com/feed.xml", // 不存在
	})
	if err != nil {
		t.Fatalf("push local only: %v", err)
	}
	if result.Pushed != 1 {
		t.Fatalf("expected 1 pushed, got %d", result.Pushed)
	}
	if len(mock.pushedSubs) != 1 {
		t.Fatalf("expected 1 subscribe op, got %d", len(mock.pushedSubs))
	}
	op := mock.pushedSubs[0]
	if op.Op != rsssync.OpSubscribe || op.URL != "https://push.com/feed.xml" {
		t.Fatalf("subscribe op mismatch: %+v", op)
	}
	if op.FolderName != "News" {
		t.Fatalf("expected folder name News, got %q", op.FolderName)
	}
}

// waitFor 等待条件成立或超时
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// TestSyncEnginePeriodicPullThrottle 周期 Pull：启动即拉一次，
// 间隔（sync_pull_interval=1s）内不重复触发，到期后再拉
func TestSyncEnginePeriodicPullThrottle(t *testing.T) {
	service, engine, _ := newSyncEngineTest(t)
	counting := &countingSyncProvider{}
	engine.newProvider = func() (rsssync.RemoteProvider, error) { return counting, nil }

	if err := service.UpdateSettings(map[string]string{
		"sync_api_enabled":   "true",
		"sync_pull_interval": "1",
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	engine.schedInterval = 20 * time.Millisecond
	engine.StartPeriodicPull()
	defer engine.StopPeriodicPull()

	// 初始 Pull 立即发生
	if !waitFor(t, 2*time.Second, func() bool { return counting.pulls.Load() >= 1 }) {
		t.Fatalf("expected initial pull, got %d", counting.pulls.Load())
	}
	// 间隔内节流：不出现第二次
	time.Sleep(500 * time.Millisecond)
	if got := counting.pulls.Load(); got != 1 {
		t.Fatalf("expected throttled to 1 pull within interval, got %d", got)
	}
	// 到期后再次触发
	if !waitFor(t, 3*time.Second, func() bool { return counting.pulls.Load() >= 2 }) {
		t.Fatalf("expected second pull after interval, got %d", counting.pulls.Load())
	}
}

// TestSyncEnginePeriodicPullDisabled 同步未启用时不拉取
func TestSyncEnginePeriodicPullDisabled(t *testing.T) {
	_, engine, _ := newSyncEngineTest(t)
	counting := &countingSyncProvider{}
	engine.newProvider = func() (rsssync.RemoteProvider, error) { return counting, nil }

	engine.schedInterval = 20 * time.Millisecond
	engine.StartPeriodicPull()
	defer engine.StopPeriodicPull()

	time.Sleep(300 * time.Millisecond)
	if got := counting.pulls.Load(); got != 0 {
		t.Fatalf("expected no pull when sync disabled, got %d", got)
	}
}

// TestSyncEnginePeriodicPullManualOnly 仅手动模式（interval=0）：即使启用同步也不调度
func TestSyncEnginePeriodicPullManualOnly(t *testing.T) {
	service, engine, _ := newSyncEngineTest(t)
	counting := &countingSyncProvider{}
	engine.newProvider = func() (rsssync.RemoteProvider, error) { return counting, nil }

	if err := service.UpdateSettings(map[string]string{
		"sync_api_enabled":   "true",
		"sync_pull_interval": "0",
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	engine.schedInterval = 20 * time.Millisecond
	engine.StartPeriodicPull()
	defer engine.StopPeriodicPull()

	time.Sleep(300 * time.Millisecond)
	if got := counting.pulls.Load(); got != 0 {
		t.Fatalf("expected no pull in manual-only mode, got %d", got)
	}
}

// greaderNamedProvider mock 的 GReader 命名包装（校准测试用）
type greaderNamedProvider struct{ mockSyncProvider }

func (g *greaderNamedProvider) Name() string { return rsssync.ProviderGReader }

// TestSyncEnginePullSinceCalibration 校准游标：非 GReader 恒为增量游标（回退 1 分钟）；
// GReader 每 N 次增量后返回零值触发全量校准
func TestSyncEnginePullSinceCalibration(t *testing.T) {
	service, engine, mock := newSyncEngineTest(t)
	if err := service.UpdateSettings(map[string]string{
		"sync_last_pull_at": strconv.FormatInt(time.Now().Add(-time.Hour).UnixMilli(), 10),
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	// 非 GReader：始终增量（非零）
	for i := 0; i < greaderFullPullEvery+3; i++ {
		if since := engine.computePullSince(mock); since.IsZero() {
			t.Fatalf("non-greader pull %d: expected incremental since, got zero", i)
		}
	}

	// GReader：前 N-1 次增量，第 N 次全量（零值），随后循环
	greader := &greaderNamedProvider{}
	engine.fullPullCount = 0
	for i := 0; i < greaderFullPullEvery; i++ {
		since := engine.computePullSince(greader)
		wantFull := i == greaderFullPullEvery-1
		if since.IsZero() != wantFull {
			t.Fatalf("greader pull %d: full=%v, wantFull=%v (since=%v)", i+1, since.IsZero(), wantFull, since)
		}
	}
	// 全量后回到增量
	if since := engine.computePullSince(greader); since.IsZero() {
		t.Fatal("greader after calibration: expected incremental since, got zero")
	}
}

// TestSyncEnginePeriodicPullBackoff 失败退避：间隔按连续失败次数翻倍
func TestSyncEnginePeriodicPullBackoff(t *testing.T) {
	_, engine, _ := newSyncEngineTest(t)

	if err := engine.service.UpdateSettings(map[string]string{
		"sync_api_enabled":   "true",
		"sync_pull_interval": "60",
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	base := time.Duration(60) * time.Second
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, base},
		{1, 2 * base},
		{3, 8 * base},
		{9, 16 * base}, // 倍率封顶 2^4；30min 上限在默认 300s 间隔下才触顶
	}
	for _, c := range cases {
		engine.schedMu.Lock()
		engine.schedFailures = c.failures
		got := engine.effectivePullIntervalLocked()
		engine.schedMu.Unlock()
		if got != c.want {
			t.Errorf("failures=%d: got %v, want %v", c.failures, got, c.want)
		}
	}
}

// TestWebDAVConnectionProbe PROPFIND 探活：2xx 通过、401 返回 ErrNotAuthorized
func TestWebDAVConnectionProbe(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "ok", statusCode: http.StatusOK},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PROPFIND" {
					t.Errorf("expected PROPFIND, got %s", r.Method)
				}
				if got := r.Header.Get("Depth"); got != "0" {
					t.Errorf("expected Depth: 0, got %q", got)
				}
				user, pass, ok := r.BasicAuth()
				if !ok || user != "alice" || pass != "secret" {
					t.Errorf("expected basic auth alice/secret, got ok=%v user=%q", ok, user)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer ts.Close()

			service, engine, _ := newSyncEngineTest(t)
			engine.fileHTTPClient = ts.Client() // 注入直连客户端，绕过 SSRF 对私有地址的拦截
			if err := service.UpdateSettings(map[string]string{
				"sync_file_provider": "webdav",
				"sync_file_url":      ts.URL,
				"sync_file_username": "alice",
				"sync_file_password": "secret",
			}); err != nil {
				t.Fatalf("update settings: %v", err)
			}

			err := engine.TestFileConnection()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, rsssync.ErrNotAuthorized) {
					t.Fatalf("expected ErrNotAuthorized, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestWebDAVConnectionMissingURL WebDAV 未填地址应报可读错误
func TestWebDAVConnectionMissingURL(t *testing.T) {
	service, engine, _ := newSyncEngineTest(t)
	if err := service.UpdateSettings(map[string]string{"sync_file_provider": "webdav"}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	err := engine.TestFileConnection()
	if err == nil || !strings.Contains(err.Error(), "webdav url is empty") {
		t.Fatalf("expected empty-url error, got %v", err)
	}
}

// TestFileProviderNotConfigured 未配置文件后端应报可读错误
func TestFileProviderNotConfigured(t *testing.T) {
	_, engine, _ := newSyncEngineTest(t)
	err := engine.TestFileConnection()
	if err == nil || !strings.Contains(err.Error(), "file provider not configured") {
		t.Fatalf("expected not-configured error, got %v", err)
	}
}

// TestFileProviderS3Rejected 未实现的文件后端必须在配置写入阶段就被拒绝，
// 不能先收下凭证再在运行时才报错。
func TestFileProviderS3Rejected(t *testing.T) {
	service, _, _ := newSyncEngineTest(t)
	err := service.UpdateSyncConfig(map[string]string{"sync_file_provider": "s3"})
	if err == nil || !strings.Contains(err.Error(), "unsupported file provider") {
		t.Fatalf("expected unsupported-provider error at config time, got %v", err)
	}
}
