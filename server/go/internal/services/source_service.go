package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rss/go-server/internal/models"
	"gorm.io/gorm"
)

func (s *ReaderService) GetSources() ([]models.Source, error) {
	var sources []models.Source
	if err := s.getDb().Find(&sources).Error; err != nil {
		return nil, err
	}

	healthMap, err := s.loadHealthMap(sourceIDs(sources))
	if err != nil {
		return nil, err
	}

	countMap, err := s.loadUnreadCountMap()
	if err != nil {
		return nil, err
	}

	mergeSourceData(sources, healthMap, countMap)
	return sources, nil
}

func sourceIDs(sources []models.Source) []int {
	ids := make([]int, len(sources))
	for i, src := range sources {
		ids[i] = src.ID
	}
	return ids
}

func (s *ReaderService) loadHealthMap(sourceIDs []int) (map[int]models.SourceHealth, error) {
	if len(sourceIDs) == 0 {
		return nil, nil
	}
	var healths []models.SourceHealth
	if err := s.getDb().Where("sourceId IN ?", sourceIDs).Find(&healths).Error; err != nil {
		return nil, fmt.Errorf("failed to load source health: %w", err)
	}
	healthMap := make(map[int]models.SourceHealth, len(healths))
	for _, h := range healths {
		healthMap[h.SourceID] = h
	}
	return healthMap, nil
}

// loadUnreadCountMap 返回各订阅源未读数。命中缓存直接返回副本，未就绪时重算并填充缓存。
func (s *ReaderService) loadUnreadCountMap() (map[int]int64, error) {
	s.unreadCountMu.RLock()
	if s.unreadCountReady {
		cp := make(map[int]int64, len(s.unreadCountCache))
		for k, v := range s.unreadCountCache {
			cp[k] = v
		}
		s.unreadCountMu.RUnlock()
		return cp, nil
	}
	s.unreadCountMu.RUnlock()

	m, err := s.computeUnreadCountMap()
	if err != nil {
		return nil, err
	}
	s.unreadCountMu.Lock()
	s.unreadCountCache = m
	s.unreadCountReady = true
	s.unreadCountMu.Unlock()
	return m, nil
}

// computeUnreadCountMap 执行实际 SQL 聚合，得到各源未读数。
func (s *ReaderService) computeUnreadCountMap() (map[int]int64, error) {
	type unreadCount struct {
		SourceID int
		Count    int64
	}
	var counts []unreadCount
	if err := s.getDb().Model(&models.Item{}).
		Select("sourceId AS source_id, COUNT(*) AS count").
		Where("isRead = ?", false).
		Group("sourceId").
		Find(&counts).Error; err != nil {
		return nil, fmt.Errorf("failed to count unread items: %w", err)
	}
	countMap := make(map[int]int64, len(counts))
	for _, c := range counts {
		countMap[c.SourceID] = c.Count
	}
	return countMap, nil
}

// invalidateUnreadCount 使未读计数缓存整体失效。
// 采用整体失效而非按源部分失效，避免部分失效导致该源未读被错误归零；
// 下次 GetSources 触发一次整体重算，当前规模下成本可忽略（且 P0-1 已停轮询，几乎零抖动）。
func (s *ReaderService) invalidateUnreadCount(_ ...int) {
	s.unreadCountMu.Lock()
	defer s.unreadCountMu.Unlock()
	s.unreadCountReady = false
	s.unreadCountCache = nil
}

func mergeSourceData(sources []models.Source, healthMap map[int]models.SourceHealth, countMap map[int]int64) {
	for i := range sources {
		sources[i].UnreadCount = countMap[sources[i].ID]
		if h, ok := healthMap[sources[i].ID]; ok {
			sources[i].LastFetchAt = h.LastFetchAt
			sources[i].LastSuccessAt = h.LastSuccessAt
			sources[i].FetchFailCount = h.FetchFailCount
			sources[i].LastError = h.LastError
		}
	}
}

// GetSource 获取单个订阅源
func (s *ReaderService) GetSource(id int) (*models.Source, error) {
	var source models.Source
	if err := s.getDb().First(&source, id).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

// UpdateSourceRequest 更新订阅源的请求参数
type UpdateSourceRequest struct {
	Name           *string `json:"name"`
	URL            *string `json:"url"`
	FolderID       *int    `json:"folderId"`
	Active         *bool   `json:"active"`
	IsPrivate      *bool   `json:"isPrivate"`
	HideInTimeline *bool   `json:"hideInTimeline"`
	Interval       *int    `json:"interval"`
}

// UpdateSource 更新订阅源
func (s *ReaderService) UpdateSource(id int, req UpdateSourceRequest) error {
	updates := buildUpdateMap(req)
	if len(updates) == 0 {
		return fmt.Errorf("no allowed fields to update")
	}
	return s.getDb().Model(&models.Source{}).Where("id = ?", id).Updates(updates).Error
}

func buildUpdateMap(req UpdateSourceRequest) map[string]interface{} {
	updates := make(map[string]interface{})
	setSourceBasicFields(updates, req)
	setSourceFlagFields(updates, req)
	return updates
}

func setSourceBasicFields(updates map[string]interface{}, req UpdateSourceRequest) {
	setIfSet(updates, "name", req.Name)
	setIfSet(updates, "url", req.URL)
	setIfSet(updates, "folderId", req.FolderID)
	setIfSet(updates, "interval", req.Interval)
}

func setSourceFlagFields(updates map[string]interface{}, req UpdateSourceRequest) {
	setIfSet(updates, "active", req.Active)
	setIfSet(updates, "isPrivate", req.IsPrivate)
	setIfSet(updates, "hideInTimeline", req.HideInTimeline)
}

func setIfSet[T any](m map[string]interface{}, key string, val *T) {
	if val != nil {
		m[key] = *val
	}
}

// GetFolders 获取所有文件夹
type CreateSourceRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	FolderID *int   `json:"folderId"`
	Interval int    `json:"interval"`
}

// CreateSource 在本地创建 RSS/Atom 订阅源
func (s *ReaderService) CreateSource(req CreateSourceRequest) (*models.Source, error) {
	if err := validateCreateSource(req); err != nil {
		return nil, err
	}
	if err := ValidateURL(req.URL); err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if err := s.checkDuplicateURL(req.URL); err != nil {
		return nil, err
	}
	req.Interval = normalizeInterval(req.Interval)

	// 标题为空时，尝试从 feed 中获取标题
	if req.Name == "" {
		client := s.BuildFetchHTTPClient()
		feedTitle, err := FetchFeedTitle(context.Background(), req.URL, client)
		if err == nil && feedTitle != "" {
			req.Name = feedTitle
		} else if err != nil {
			slog.Warn("failed to fetch feed title", "url", req.URL, "error", err)
		}
	}

	now := models.MilliTime{T: time.Now()}
	source := models.Source{
		Name:          req.Name,
		URL:           req.URL,
		FolderID:      req.FolderID,
		ListRule:      "rss",
		Interval:      req.Interval,
		Active:        true,
		CreatedAtTime: now,
		UpdatedAtTime: now,
	}
	if err := s.getDb().Create(&source).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func validateCreateSource(req CreateSourceRequest) error {
	if req.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (s *ReaderService) checkDuplicateURL(url string) error {
	var count int64
	if err := s.getDb().Model(&models.Source{}).Where("url = ?", url).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check duplicate url: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("source with url %q already exists", url)
	}
	return nil
}

func normalizeInterval(interval int) int {
	if interval <= 0 {
		interval = 120
	}
	if interval < 5 {
		interval = 5
	}
	return interval
}

// DeleteSource 删除订阅源，并自动清理空文件夹
func (s *ReaderService) DeleteSource(id int) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		// 先获取源所在的文件夹
		var source models.Source
		if err := tx.First(&source, id).Error; err != nil {
			return err
		}
		folderID := source.FolderID

		// 删除源
		if err := tx.Delete(&models.Source{}, id).Error; err != nil {
			return err
		}

		// 如果源属于某个文件夹，检查并清理空文件夹
		if folderID != nil {
			return s.cleanupEmptyFolder(tx, *folderID)
		}
		return nil
	})
}

// DeleteSourcesBatch 批量删除订阅源，并自动清理空文件夹
func (s *ReaderService) DeleteSourcesBatch(ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	err := s.getDb().Transaction(func(tx *gorm.DB) error {
		folderIDs, err := s.batchDeleteGetFolderIDs(tx, ids)
		if err != nil {
			return err
		}
		if err := s.batchDeleteSources(tx, ids); err != nil {
			return err
		}
		return s.batchDeleteCleanupFolders(tx, folderIDs)
	})
	if err != nil {
		return err
	}
	s.invalidateUnreadCount()
	return nil
}

func (s *ReaderService) batchDeleteGetFolderIDs(tx *gorm.DB, ids []int) (map[int]bool, error) {
	var sources []models.Source
	if err := tx.Where("id IN ?", ids).Find(&sources).Error; err != nil {
		return nil, err
	}
	folderIDs := make(map[int]bool)
	for _, src := range sources {
		if src.FolderID != nil {
			folderIDs[*src.FolderID] = true
		}
	}
	return folderIDs, nil
}

func (s *ReaderService) batchDeleteSources(tx *gorm.DB, ids []int) error {
	return tx.Where("id IN ?", ids).Delete(&models.Source{}).Error
}

func (s *ReaderService) batchDeleteCleanupFolders(tx *gorm.DB, folderIDs map[int]bool) error {
	for fid := range folderIDs {
		if err := s.cleanupEmptyFolder(tx, fid); err != nil {
			return err
		}
	}
	return nil
}

// cleanupEmptyFolder 检查文件夹是否为空，如果为空则删除。
// 删除前将子文件夹提升到根层级，避免子文件夹的 parentId 指向已删除的文件夹。
func (s *ReaderService) cleanupEmptyFolder(tx *gorm.DB, folderID int) error {
	var count int64
	if err := tx.Model(&models.Source{}).Where("folderId = ?", folderID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		// 将子文件夹提升到根层级，避免产生孤儿文件夹
		if err := tx.Model(&models.Folder{}).Where("parentId = ?", folderID).Update("parentId", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Folder{}, folderID).Error
	}
	return nil
}

// OPML 导入限制常量：防止恶意构造的 OPML 通过深度嵌套或节点爆炸导致栈溢出/内存耗尽
func (s *ReaderService) updateSourceHealth(sourceID int, lastFetchAt models.NullableMilliTime, lastSuccessAt models.NullableMilliTime, lastError string, newCount int, interval int) {
	var health models.SourceHealth
	if err := s.getDb().Where("sourceId = ?", sourceID).First(&health).Error; err != nil {
		// 记录不存在则创建
		health = models.SourceHealth{SourceID: sourceID}
	}

	health.LastFetchAt = lastFetchAt
	if lastSuccessAt.T != nil {
		health.LastSuccessAt = lastSuccessAt
		health.FetchFailCount = 0
		health.LastError = nil
		health.NextRetryAtUnix = 0
		// 自适应调度：根据新增文章数动态计算下次检查时间
		health.NextCheckAtUnix = s.adaptiveNextCheckAt(interval, newCount)
	} else {
		health.FetchFailCount++
		if lastError != "" {
			health.LastError = &lastError
		}
		// 指数退避：连续失败越多，下次可抓时间越晚，避免僵尸源每次全量刷新都拖慢整体
		health.NextRetryAtUnix = nextRetryUnix(health.FetchFailCount)
	}

	if err := s.getDb().Save(&health).Error; err != nil {
		slog.Warn("failed to update source health", "source_id", health.SourceID, "error", err)
	}
}

// adaptiveNextCheckAt 根据新增文章数计算下次检查时间戳（Unix 秒）。
// 调度周期统一取全局设置 defaultInterval（UI "默认抓取间隔"），所有订阅源按此周期轮询：
//   - 新增 >5 条：缩短到最小间隔（fetchMinInterval），尽快拉全
//   - 否则：按全局间隔轮询（不再无新增即推到 3 天）
//
// interval 参数保留以兼容调用链，当前调度不再依赖单源 interval。
func (s *ReaderService) adaptiveNextCheckAt(interval int, newCount int) int64 {
	globalInterval := s.GetSettingInt("defaultInterval", 120)
	if globalInterval < 5 {
		globalInterval = 5
	}
	minInterval := s.GetSettingInt("fetchMinInterval", 20)
	if minInterval <= 0 {
		minInterval = 20
	}

	now := time.Now()
	if newCount > 5 {
		nextCheck := now.Add(time.Duration(minInterval) * time.Minute)
		return nextCheck.Unix()
	}
	nextCheck := now.Add(time.Duration(globalInterval) * time.Minute)
	return nextCheck.Unix()
}

// CacheStats 缓存统计
