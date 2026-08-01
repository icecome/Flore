package services

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/models"
)

// ReaderService 阅读器业务逻辑
type ReaderService struct {
	db   *gorm.DB
	dbMu sync.RWMutex

	// unreadCountCache 缓存各订阅源未读数，避免每次 GetSources 都 GROUP BY 聚合。
	// 变更事件（标已读/抓取写入/删源/导入恢复）通过 invalidateUnreadCount 整体失效，
	// 下次访问自动重算；进程重启后首次访问亦自愈。
	unreadCountCache   map[int]int64
	unreadCountMu      sync.RWMutex
	unreadCountReady   bool

	// cachedHTTPClient 缓存的 HTTP 客户端，在代理设置变更时重建
	cachedHTTPClient    *http.Client
	cachedHTTPClientMu  sync.RWMutex
	cachedHTTPClientSeq int64
	proxySettingsSeq    int64

	// coordinator 是抓取动作的唯一调度权威，由 main 启动时注入。
	// 所有抓取入口（手动/调度/托盘）统一走 coordinator.Submit，去重与并发控制集中在此。
	coordinator *FetchCoordinator

	// filterRulesCache 缓存过滤规则，避免每篇新文章都查一次 DB
	filterRulesCache    []FilterRuleWithConditions
	filterRulesCacheMu  sync.RWMutex
	filterRulesCacheSeq int64 // 规则变更时递增，使缓存失效
}

// 单例：确保 handler 和 scheduler 共享同一个实例，
// 数据库恢复后只需更新一次 db 引用即可同步所有调用方
var (
	readerServiceInstance *ReaderService
	readerServiceOnce     sync.Once
)

// NewReaderService 创建阅读器服务（单例）
func NewReaderService() *ReaderService {
	readerServiceOnce.Do(func() {
		s := &ReaderService{db: database.DB}
		s.proxySettingsSeq = s.loadProxySettingsSeq()
		readerServiceInstance = s
	})
	return readerServiceInstance
}

func (s *ReaderService) loadProxySettingsSeq() int64 {
	if s.GetSettingBool("proxyEnabled", false) && s.GetSetting("proxyUrl") != "" {
		return 1
	}
	return 0
}

func (s *ReaderService) getDb() *gorm.DB {
	s.dbMu.RLock()
	defer s.dbMu.RUnlock()
	return s.db
}

func (s *ReaderService) setDb(db *gorm.DB) {
	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	s.db = db
}

// execLocked 在持有读锁期间执行 SQL。
// 根因修复 m-03：原 getDb() 在返回前就释放读锁，导致 ExportDatabase/Backup/Vacuum
// 在 Exec 执行时持有的 *gorm.DB 可能被并发的导入（写锁）关闭，产生竞态。
// 此处将读锁持有到 Exec 完成，杜绝该窗口。
func (s *ReaderService) execLocked(query string, args ...interface{}) error {
	s.dbMu.RLock()
	defer s.dbMu.RUnlock()
	return s.db.Exec(query, args...).Error
}

// SetCoordinator 注入抓取协调器，由 main 在启动 coordinator 后调用。
func (s *ReaderService) SetCoordinator(c *FetchCoordinator) {
	s.coordinator = c
}

// Coordinator 返回抓取协调器，供 handler/scheduler 调用 Submit。
func (s *ReaderService) Coordinator() *FetchCoordinator {
	return s.coordinator
}

// GetSources 获取所有订阅源，包含未读数与健康状态
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
	Name          *string `json:"name"`
	URL           *string `json:"url"`
	FolderID      *int    `json:"folderId"`
	Active        *bool   `json:"active"`
	IsPrivate     *bool   `json:"isPrivate"`
	HideInTimeline *bool  `json:"hideInTimeline"`
	Interval      *int    `json:"interval"`
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
func (s *ReaderService) GetFolders() ([]models.Folder, error) {
	var folders []models.Folder
	if err := s.getDb().Find(&folders).Error; err != nil {
		return nil, err
	}
	return folders, nil
}

// CreateFolder 创建文件夹
func (s *ReaderService) CreateFolder(name string) (*models.Folder, error) {
	now := models.MilliTime{T: time.Now()}
	folder := models.Folder{Name: name, CreatedAtTime: now, UpdatedAtTime: now}
	if err := s.getDb().Create(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

// UpdateFolder 更新文件夹
func (s *ReaderService) UpdateFolder(id int, name string) error {
	return s.getDb().Model(&models.Folder{}).Where("id = ?", id).Update("name", name).Error
}

// DeleteFolder 删除文件夹，保留下级文件夹并提升到根层级
func (s *ReaderService) DeleteFolder(id int) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		// 将文件夹内的 Source 移出到根目录
		if err := tx.Model(&models.Source{}).Where("folderId = ?", id).Update("folderId", nil).Error; err != nil {
			return err
		}
		// 将子文件夹提升到根层级
		if err := tx.Model(&models.Folder{}).Where("parentId = ?", id).Update("parentId", nil).Error; err != nil {
			return err
		}
		// 删除文件夹本身
		return tx.Delete(&models.Folder{}, id).Error
	})
}

// MoveSourceFolder 移动订阅源到指定文件夹，并自动清理空文件夹
func (s *ReaderService) MoveSourceFolder(sourceID int, folderID *int) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		// 获取源当前的文件夹
		var source models.Source
		if err := tx.First(&source, sourceID).Error; err != nil {
			return err
		}
		oldFolderID := source.FolderID

		// 移动源
		if err := tx.Model(&models.Source{}).Where("id = ?", sourceID).Update("folderId", folderID).Error; err != nil {
			return err
		}

		// 如果源原来属于某个文件夹，检查并清理空文件夹
		if oldFolderID != nil {
			return s.cleanupEmptyFolder(tx, *oldFolderID)
		}
		return nil
	})
}

// buildItemQuery 构建文章查询的公共条件（供 GetItems / CountItems 复用）
func (s *ReaderService) buildItemQuery(sourceID *int, folderID *int, onlyUnread bool, onlyStarred bool, onlyReadLater bool, hidePrivate bool) *gorm.DB {
	query := s.getDb().Table("Item").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id")
	if sourceID != nil {
		query = query.Where("Item.sourceId = ?", *sourceID)
	}
	if folderID != nil {
		query = query.Where("Source.folderId = ?", *folderID)
	}
	if sourceID == nil && folderID == nil {
		query = applyTimelineFilters(query, hidePrivate)
	}
	query = applyItemStatusFilters(query, onlyUnread, onlyStarred, onlyReadLater)
	return query
}

func applyTimelineFilters(query *gorm.DB, hidePrivate bool) *gorm.DB {
	query = query.Where("Source.hideInTimeline = ?", false)
	if hidePrivate {
		query = query.Where("Source.isPrivate = ?", false)
	}
	return query
}

func applyItemStatusFilters(query *gorm.DB, onlyUnread, onlyStarred, onlyReadLater bool) *gorm.DB {
	if onlyUnread {
		query = query.Where("Item.isRead = ?", false)
	}
	if onlyStarred {
		query = query.Where("Item.isStarred = ?", true)
	}
	if onlyReadLater {
		query = query.Where("Item.isReadLater = ?", true)
	}
	return query
}

// GetItems 获取文章列表，支持按 sourceId 或 folderId 筛选
// orderBy: "newest"（默认，pubDate desc）或 "oldest"（pubDate asc）
func (s *ReaderService) GetItems(sourceID *int, folderID *int, onlyUnread bool, onlyStarred bool, onlyReadLater bool, hidePrivate bool, limit int, offset int, orderBy string) ([]models.ItemWithSource, error) {
	items := []models.ItemWithSource{}

	query := s.buildItemQuery(sourceID, folderID, onlyUnread, onlyStarred, onlyReadLater, hidePrivate).
		Select("Item.*, Source.name as source_name, Source.url as source_url")

	if limit <= 0 {
		limit = 50
	}

	// 加 Item.id 作为最终 tie-breaker，保证排序严格全序，使 offset 分页稳定
	orderClause := "Item.pubDate desc, Item.createdAt desc, Item.id desc"
	if orderBy == "oldest" {
		orderClause = "Item.pubDate asc, Item.createdAt asc, Item.id asc"
	}

	if err := query.Order(orderClause).
		Limit(limit).Offset(offset).
		Scan(&items).Error; err != nil {
		return nil, err
	}

	return items, nil
}

// GetItem 获取单篇文章
func (s *ReaderService) GetItem(id int) (*models.Item, error) {
	var item models.Item
	if err := s.getDb().First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// MarkRead 标记已读/未读（单篇）
func (s *ReaderService) MarkRead(id int, read bool) error {
	if err := s.getDb().Model(&models.Item{}).Where("id = ?", id).Update("isRead", read).Error; err != nil {
		return err
	}
	s.invalidateUnreadCount()
	return nil
}

// MarkReadBatch 批量标记已读/未读
func (s *ReaderService) MarkReadBatch(ids []int, read bool) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.getDb().Model(&models.Item{}).Where("id IN ?", ids).Update("isRead", read).Error; err != nil {
		return err
	}
	s.invalidateUnreadCount()
	return nil
}

// ClearFolder 清空文件夹内订阅源，并自动删除空文件夹
func (s *ReaderService) ClearFolder(folderID int) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		if err := s.clearFolderSources(tx, folderID); err != nil {
			return err
		}
		if err := s.deleteChildFolders(tx, folderID); err != nil {
			return err
		}
		return tx.Delete(&models.Folder{}, folderID).Error
	})
}

func (s *ReaderService) clearFolderSources(tx *gorm.DB, folderID int) error {
	return tx.Model(&models.Source{}).Where("folderId = ?", folderID).Update("folderId", nil).Error
}

func (s *ReaderService) deleteChildFolders(tx *gorm.DB, folderID int) error {
	var childFolders []models.Folder
	if err := tx.Where("parentId = ?", folderID).Find(&childFolders).Error; err != nil {
		return err
	}
	for _, cf := range childFolders {
		if err := tx.Model(&models.Source{}).Where("folderId = ?", cf.ID).Update("folderId", nil).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Folder{}, cf.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// MarkStarred 标记收藏
func (s *ReaderService) MarkStarred(id int, starred bool) error {
	return s.getDb().Model(&models.Item{}).Where("id = ?", id).Update("isStarred", starred).Error
}

// MarkReadLater 标记稍后阅读
func (s *ReaderService) MarkReadLater(id int, readLater bool) error {
	return s.getDb().Model(&models.Item{}).Where("id = ?", id).Update("isReadLater", readLater).Error
}

// ClearReadLater 清空稍后阅读标记（支持按订阅源或文件夹限定范围）
func (s *ReaderService) ClearReadLater(sourceID *int, folderID *int) error {
	query := s.getDb().Model(&models.Item{}).Where("isReadLater = ?", true)
	if sourceID != nil {
		query = query.Where("sourceId = ?", *sourceID)
	} else if folderID != nil {
		query = query.Where("sourceId IN (?)", s.getDb().Model(&models.Source{}).Select("id").Where("folderId = ?", *folderID))
	}
	return query.Update("isReadLater", false).Error
}

// GetUnreadCount 获取总未读数（排除在时间线上隐藏和私密的订阅源）
func (s *ReaderService) GetUnreadCount(hidePrivate bool) (int64, error) {
	var count int64
	query := s.getDb().Model(&models.Item{}).
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id").
		Where("Item.isRead = ?", false).
		Where("Source.hideInTimeline = ?", false)
	if hidePrivate {
		query = query.Where("Source.isPrivate = ?", false)
	}
	err := query.Count(&count).Error
	return count, err
}

// CreateSourceRequest 创建订阅源请求（Reader 仅保留 RSS/Atom URL 订阅）
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
		Name:      req.Name,
		URL:       req.URL,
		FolderID:  req.FolderID,
		ListRule:  "rss",
		Interval:  req.Interval,
		Active:    true,
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
const (
	// opmlMaxDepth 限制 outline 递归深度。正常 OPML 层级很少超过 5 层，32 层足够冗余
	opmlMaxDepth = 32
	// opmlMaxNodes 限制单次导入的 outline 节点总数。10000 个订阅源已远超普通用户需求
	opmlMaxNodes = 10000
)

// OPML 导入导出结构
type opmlDocument struct {
	XMLName xml.Name    `xml:"opml"`
	Version string      `xml:"version,attr"`
	Head    opmlHead    `xml:"head"`
	Body    opmlBody    `xml:"body"`
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	Outlines []opmlOutline `xml:"outline"`
}

// ImportOPML 使用 Go 原生 XML 解析导入 OPML。
//
// XML DoS 防御说明：
//   - Go 标准库 encoding/xml 默认不展开 DTD 实体，billion laughs 等实体扩展攻击天然无效
//   - handler 层已通过 http.MaxBytesReader 限制 body 大小为 10MB
//   - 此处额外限制 outline 节点总数与递归深度，防止深层嵌套导致栈溢出或节点爆炸消耗内存
func (s *ReaderService) ImportOPML(xmlData string) error {
	doc, err := s.parseOPMLDocument(xmlData)
	if err != nil {
		return err
	}
	return s.importOPMLBody(doc.Body.Outlines)
}

func (s *ReaderService) parseOPMLDocument(xmlData string) (*opmlDocument, error) {
	var doc opmlDocument
	if err := xml.Unmarshal([]byte(xmlData), &doc); err != nil {
		return nil, fmt.Errorf("parse opml failed: %w", err)
	}
	return &doc, nil
}

func (s *ReaderService) importOPMLBody(outlines []opmlOutline) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		folderMap := make(map[string]int)
		visited := 0
		return s.importOPMLOutlines(tx, outlines, folderMap, &visited)
	})
}

func (s *ReaderService) importOPMLOutlines(tx *gorm.DB, outlines []opmlOutline, folderMap map[string]int, visited *int) error {
	for _, outline := range outlines {
		if err := s.importOutlineRecursive(tx, outline, nil, "", folderMap, 0, visited); err != nil {
			return err
		}
	}
	return nil
}

// importOutlineRecursive 递归处理 OPML outline
// 叶子节点（有xmlUrl）创建源，非叶子节点视为文件夹
// 使用 ParentID 创建多层级文件夹结构
//
// depth 为当前递归深度，visited 为整棵树已访问的节点计数器（指针）。
// 超过 opmlMaxDepth 或 opmlMaxNodes 时返回错误，防御深层嵌套与节点爆炸。
func (s *ReaderService) importOutlineRecursive(tx *gorm.DB, outline opmlOutline, parentID *int, parentPath string, folderMap map[string]int, depth int, visited *int) error {
	if err := s.checkOPMLDepthLimit(depth, visited); err != nil {
		return err
	}

	folderName := firstNonEmpty(outline.Text, outline.Title)
	currentPath := buildOPMLPath(parentPath, folderName)

	if err := s.importOPMLChildren(tx, outline, parentID, currentPath, folderMap, depth, visited); err != nil {
		return err
	}

	if outline.XMLURL != "" {
		return s.createSourceFromOPML(tx, parentID, outline)
	}
	return nil
}

func (s *ReaderService) checkOPMLDepthLimit(depth int, visited *int) error {
	if depth > opmlMaxDepth {
		return fmt.Errorf("opml: outline nesting depth exceeds limit %d", opmlMaxDepth)
	}
	*visited++
	if *visited > opmlMaxNodes {
		return fmt.Errorf("opml: total outline nodes exceeds limit %d", opmlMaxNodes)
	}
	return nil
}

func (s *ReaderService) importOPMLChildren(tx *gorm.DB, outline opmlOutline, parentID *int, currentPath string, folderMap map[string]int, depth int, visited *int) error {
	for _, child := range outline.Outlines {
		currentFolderID, err := resolveOPMLFolderID(tx, firstNonEmpty(outline.Text, outline.Title), currentPath, parentID, folderMap)
		if err != nil {
			return err
		}
		if err := s.importOutlineRecursive(tx, child, currentFolderID, currentPath, folderMap, depth+1, visited); err != nil {
			return err
		}
	}
	return nil
}

// buildOPMLPath 构建 OPML 当前全路径
func buildOPMLPath(parentPath, folderName string) string {
	if folderName == "" {
		return parentPath
	}
	if parentPath != "" {
		return parentPath + "/" + folderName
	}
	return folderName
}

// resolveOPMLFolderID 创建或获取当前层级文件夹 ID
func resolveOPMLFolderID(tx *gorm.DB, folderName, currentPath string, parentID *int, folderMap map[string]int) (*int, error) {
	if folderName == "" || currentPath == "" {
		return parentID, nil
	}
	if id, exists := folderMap[currentPath]; exists {
		return &id, nil
	}
	now := models.MilliTime{T: time.Now()}
	newFolder := models.Folder{
		Name:          folderName,
		ParentID:      parentID,
		CreatedAtTime: now,
		UpdatedAtTime: now,
	}
	if err := tx.Create(&newFolder).Error; err != nil {
		return nil, fmt.Errorf("failed to create OPML folder %q: %w", folderName, err)
	}
	folderMap[currentPath] = newFolder.ID
	return &newFolder.ID, nil
}

// createSourceFromOPML 从 OPML outline 创建订阅源（轻量 URL 格式校验）
func (s *ReaderService) createSourceFromOPML(tx *gorm.DB, folderID *int, outline opmlOutline) error {
	url := outline.XMLURL
	if url == "" {
		return nil
	}
	// 轻量级校验：只检查格式，不发起 DNS 查询。SSRF 防护在 fetcher.go 拉取时做
	if err := ValidateURLOnly(url); err != nil {
		slog.Warn("opml: skipped invalid URL", "url", url, "error", err)
		return nil // 跳过无效 URL，继续处理下一个
	}
	name := firstNonEmpty(outline.Text, outline.Title, url)
	now := models.MilliTime{T: time.Now()}
	source := models.Source{
		Name:      name,
		URL:       url,
		FolderID:  folderID,
		ListRule:  "rss",
		Interval:  120,
		Active:    true,
		CreatedAtTime: now,
		UpdatedAtTime: now,
	}
	return tx.Create(&source).Error
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ExportOPML 使用 Go 原生 XML 生成 OPML，支持嵌套文件夹树
func (s *ReaderService) ExportOPML() (string, error) {
	sources, err := s.GetSources()
	if err != nil {
		return "", err
	}
	folders, err := s.GetFolders()
	if err != nil {
		return "", err
	}

	folderSources, noFolderSources := groupSourcesByFolder(sources)
	folderChildren, rootFolders := buildFolderTree(folders)

	outlines := buildOPMLOutlines(noFolderSources, rootFolders, folderSources, folderChildren)

	doc := opmlDocument{
		Version: "2.0",
		Head:    opmlHead{Title: "Flore Subscriptions"},
		Body:    opmlBody{Outlines: outlines},
	}

	output, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return xml.Header + string(output), nil
}

// groupSourcesByFolder 按 folderId 分组源
func groupSourcesByFolder(sources []models.Source) (map[int][]models.Source, []models.Source) {
	folderSources := make(map[int][]models.Source)
	var noFolderSources []models.Source
	for _, src := range sources {
		if src.FolderID != nil {
			folderSources[*src.FolderID] = append(folderSources[*src.FolderID], src)
		} else {
			noFolderSources = append(noFolderSources, src)
		}
	}
	return folderSources, noFolderSources
}

// buildFolderTree 构建文件夹树：parentId -> children
func buildFolderTree(folders []models.Folder) (map[int][]models.Folder, []models.Folder) {
	folderChildren := make(map[int][]models.Folder)
	var rootFolders []models.Folder
	for _, f := range folders {
		if f.ParentID != nil {
			folderChildren[*f.ParentID] = append(folderChildren[*f.ParentID], f)
		} else {
			rootFolders = append(rootFolders, f)
		}
	}
	return folderChildren, rootFolders
}

// buildOPMLOutlines 构建 OPML outline 列表
func buildOPMLOutlines(noFolderSources []models.Source, rootFolders []models.Folder, folderSources map[int][]models.Source, folderChildren map[int][]models.Folder) []opmlOutline {
	var buildFolderOutlines func(folderID int) []opmlOutline
	buildFolderOutlines = func(folderID int) []opmlOutline {
		var children []opmlOutline
		for _, cf := range folderChildren[folderID] {
			children = append(children, opmlOutline{
				Text:     cf.Name,
				Title:    cf.Name,
				Outlines: buildFolderOutlines(cf.ID),
			})
		}
		for _, src := range folderSources[folderID] {
			children = append(children, opmlOutline{
				Text:   src.Name,
				Title:  src.Name,
				Type:   "rss",
				XMLURL: src.URL,
			})
		}
		return children
	}

	var outlines []opmlOutline
	for _, src := range noFolderSources {
		outlines = append(outlines, opmlOutline{
			Text:   src.Name,
			Title:  src.Name,
			Type:   "rss",
			XMLURL: src.URL,
		})
	}
	for _, folder := range rootFolders {
		outlines = append(outlines, opmlOutline{
			Text:     folder.Name,
			Title:    folder.Name,
			Outlines: buildFolderOutlines(folder.ID),
		})
	}
	return outlines
}

// CountItems 获取文章总数（支持 sourceId/folderId/unread/starred/readLater 筛选）
func (s *ReaderService) CountItems(sourceID *int, folderID *int, onlyUnread bool, onlyStarred bool, onlyReadLater bool, hidePrivate bool) (int64, error) {
	var count int64
	query := s.getDb().Model(&models.Item{}).
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id")
	query = s.applyCountScopeFilters(query, sourceID, folderID, hidePrivate)
	query = applyItemStatusFilters(query, onlyUnread, onlyStarred, onlyReadLater)
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *ReaderService) applyCountScopeFilters(query *gorm.DB, sourceID, folderID *int, hidePrivate bool) *gorm.DB {
	if sourceID != nil {
		query = query.Where("Item.sourceId = ?", *sourceID)
	}
	if folderID != nil {
		query = query.Where("Source.folderId = ?", *folderID)
	}
	if sourceID == nil && folderID == nil {
		query = applyTimelineFilters(query, hidePrivate)
	}
	return query
}

// MarkAllRead 标记某订阅源、文件夹或全部文章为已读。
// 仅更新未读项，避免全表重复写入。
func (s *ReaderService) MarkAllRead(sourceID *int, folderID *int) error {
	query := s.getDb().Model(&models.Item{}).Where("isRead = ?", false)

	if sourceID != nil {
		query = query.Where("sourceId = ?", *sourceID)
	} else if folderID != nil {
		query = query.Where("sourceId IN (?)", s.getDb().Model(&models.Source{}).Select("id").Where("folderId = ?", *folderID))
	}

	if err := query.Update("isRead", true).Error; err != nil {
		return err
	}
	s.invalidateUnreadCount()
	return nil
}

// sanitizeFTS5Query 将用户输入转换为安全的 FTS5 MATCH 查询。
// 只保留字母、数字、Unicode 文字，移除所有 FTS5 特殊字符，
// 然后为每个词添加前缀匹配支持，例如 "hello world" -> "hello* world*"。
// 这种"保守"策略牺牲了短语搜索等高级功能，但彻底杜绝了注入风险。
func sanitizeFTS5Query(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return ""
	}
	clean := cleanFTS5Keyword(keyword)
	return appendPrefixStarsToWords(clean)
}

func cleanFTS5Keyword(keyword string) string {
	var clean strings.Builder
	clean.Grow(len(keyword))
	for _, r := range keyword {
		if isFTS5SafeChar(r) {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

func isFTS5SafeChar(r rune) bool {
	return r == ' ' || isLetterOrDigitOrUnicode(r)
}

func isLetterOrDigitOrUnicode(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r > 127
}

func appendPrefixStarsToWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = w + "*"
	}
	return strings.Join(words, " ")
}

// escapeSQLWildcards 转义 SQL LIKE 模式中的通配符 % 和 _，避免用户输入影响匹配范围
func escapeSQLWildcards(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// SearchItems 搜索文章。
// 先使用 FTS5 进行全文搜索；对于中文等 FTS5 分词不理想的场景，回退到 LIKE 匹配标题/摘要。
func (s *ReaderService) SearchItems(keyword string, limit int) ([]models.ItemWithSource, error) {
	if limit <= 0 {
		limit = 50
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []models.ItemWithSource{}, nil
	}
	seen := s.searchFTS5(keyword, limit)
	s.searchLikeFallbackIfNeeded(seen, keyword, limit)
	if len(seen) == 0 {
		return []models.ItemWithSource{}, nil
	}
	return s.searchLoadItems(seen)
}

func (s *ReaderService) searchLikeFallbackIfNeeded(seen map[int]struct{}, keyword string, limit int) {
	if len(seen) < limit {
		searchLikeFallback(s.getDb(), keyword, limit, seen)
	}
}

func (s *ReaderService) searchLoadItems(seen map[int]struct{}) ([]models.ItemWithSource, error) {
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	var items []models.ItemWithSource
	err := s.getDb().Table("Item").
		Select("Item.*, Source.name as source_name, Source.url as source_url").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id").
		Where("Item.id IN ?", ids).
		Order("Item.pubDate DESC, Item.createdAt DESC").
		Scan(&items).Error
	return items, err
}

// searchFTS5 通过 FTS5 全文搜索获取文章 ID 集合
func (s *ReaderService) searchFTS5(keyword string, limit int) map[int]struct{} {
	seen := make(map[int]struct{})
	ftsQuery := sanitizeFTS5Query(keyword)
	if ftsQuery == "" {
		return seen
	}
	var ids []int
	if err := s.getDb().Raw(
		"SELECT itemId FROM ItemSearch WHERE ItemSearch MATCH ? ORDER BY rank LIMIT ?",
		ftsQuery, limit,
	).Scan(&ids).Error; err == nil {
		for _, id := range ids {
			seen[id] = struct{}{}
		}
	}
	return seen
}

// searchLikeFallback 使用 LIKE 查询标题/摘要作为补充（对中文更友好）
func searchLikeFallback(db *gorm.DB, keyword string, limit int, seen map[int]struct{}) {
	pattern := "%" + escapeSQLWildcards(keyword) + "%"
	var likeIDs []int
	if err := db.Table("Item").
		Select("Item.id").
		Where("LOWER(Item.title) LIKE LOWER(?) ESCAPE '\\' OR LOWER(Item.desc) LIKE LOWER(?) ESCAPE '\\'", pattern, pattern).
		Limit(limit).
		Pluck("id", &likeIDs).Error; err == nil {
		for _, id := range likeIDs {
			seen[id] = struct{}{}
		}
	}
}

// IndexItemForSearch 为单篇文章建立/更新 FTS5 索引
func (s *ReaderService) IndexItemForSearch(itemID int) error {
	var item models.Item
	if err := s.getDb().First(&item, itemID).Error; err != nil {
		return err
	}
	desc := ""
	if item.Desc != nil {
		desc = *item.Desc
	}
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM ItemSearch WHERE itemId = ?", itemID).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO ItemSearch(title, content, itemId) VALUES (?, ?, ?)",
			item.Title, desc, itemID,
		).Error
	})
}

// RebuildSearchIndex 重建所有文章的 FTS5 索引（包裹事务保证原子性）
func (s *ReaderService) RebuildSearchIndex() error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM ItemSearch").Error; err != nil {
			return err
		}
		return tx.Exec(`
			INSERT INTO ItemSearch(title, content, itemId)
			SELECT title, COALESCE(desc, ''), id FROM Item
		`).Error
	})
}

// GetReadability 根据文章 ID 获取阅读模式内容（优先读取缓存）
// 缓存 TTL 为 7 天，过期后自动重新抓取
func (s *ReaderService) GetReadability(id int, refresh bool) (*ReadabilityResult, error) {
	item, err := s.GetItem(id)
	if err != nil {
		return nil, err
	}
	if !refresh {
		if cached := s.getReadabilityCache(id); cached != nil {
			return cached, nil
		}
	}
	result, err := FetchReadabilityWithClient(item.Link, s.BuildFetchHTTPClient())
	if err != nil {
		return nil, err
	}
	s.sanitizeByline(result, item)
	if err := s.saveReadabilityCache(id, result); err != nil {
		slog.Warn("failed to save readability cache", "item_id", id, "error", err)
	}
	return result, nil
}

func (s *ReaderService) getReadabilityCache(id int) *ReadabilityResult {
	var cache models.ReadabilityCache
	if err := s.getDb().Where("itemId = ?", id).First(&cache).Error; err != nil {
		return nil
	}
	if isReadabilityCacheExpired(cache.CachedAt.T) {
		return nil
	}
	return &ReadabilityResult{
		Title:       cache.Title,
		Byline:      cache.Byline,
		Content:     cache.Content,
		TextContent: cache.TextContent,
		Excerpt:     cache.Excerpt,
		SiteName:    cache.SiteName,
		URL:         cache.URL,
	}
}

func isReadabilityCacheExpired(cachedAt time.Time) bool {
	return cachedAt.IsZero() || time.Since(cachedAt) >= 7*24*time.Hour
}

func (s *ReaderService) sanitizeByline(result *ReadabilityResult, item *models.Item) {
	if isValidByline(result.Byline) {
		return
	}
	if item.Author != nil && *item.Author != "" {
		result.Byline = *item.Author
	}
}

// saveReadabilityCache 将 readability 结果写入缓存
func (s *ReaderService) saveReadabilityCache(itemID int, result *ReadabilityResult) error {
	cache := models.ReadabilityCache{
		ItemID:      itemID,
		Title:       result.Title,
		Byline:      result.Byline,
		Content:     result.Content,
		TextContent: result.TextContent,
		Excerpt:     result.Excerpt,
		SiteName:    result.SiteName,
		URL:         result.URL,
	}
	return s.getDb().Save(&cache).Error
}

// isValidByline 判断 readability 提取的作者名是否合理
func isValidByline(byline string) bool {
	byline = strings.TrimSpace(byline)
	if len(byline) < 2 {
		return false
	}
	// 纯数字或编号类文本不是有效作者
	if matched, _ := regexp.MatchString(`^\d+$`, byline); matched {
		return false
	}
	return true
}

// GetOriginalURL 根据文章 ID 获取原文链接（用于代理）
func (s *ReaderService) GetOriginalURL(id int) (string, error) {
	item, err := s.GetItem(id)
	if err != nil {
		return "", err
	}
	return item.Link, nil
}

// updateSourceHealth 更新订阅源健康状态
func (s *ReaderService) updateSourceHealth(sourceID int, lastFetchAt models.NullableMilliTime, lastSuccessAt models.NullableMilliTime, lastError string) {
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

// CacheStats 缓存统计
type CacheStats struct {
	ReadabilityCount int64 `json:"readabilityCount"`
	ReadabilitySize  int64 `json:"readabilitySize"`
	FTSItemCount     int64 `json:"ftsItemCount"`
	TotalItems       int64 `json:"totalItems"`
	TotalSources     int64 `json:"totalSources"`
}

// GetCacheStats 获取缓存统计信息
func (s *ReaderService) GetCacheStats() (*CacheStats, error) {
	stats := &CacheStats{}
	if err := s.getDb().Model(&models.ReadabilityCache{}).Count(&stats.ReadabilityCount).Error; err != nil {
		return nil, err
	}
	// 估算 ReadabilityCache 大小（Content 字段为主要体积来源）
	var totalContentLen int64
	row := s.getDb().Model(&models.ReadabilityCache{}).
		Select("COALESCE(SUM(LENGTH(content) + LENGTH(title) + LENGTH(byline) + LENGTH(textContent) + LENGTH(excerpt) + LENGTH(siteName) + LENGTH(url)), 0)").
		Row()
	if err := row.Scan(&totalContentLen); err != nil {
		return nil, fmt.Errorf("failed to query readability size: %w", err)
	}
	stats.ReadabilitySize = totalContentLen

	if err := s.getDb().Raw("SELECT count(*) FROM ItemSearch").Scan(&stats.FTSItemCount).Error; err != nil {
		return nil, fmt.Errorf("failed to query fts count: %w", err)
	}
	if err := s.getDb().Model(&models.Item{}).Count(&stats.TotalItems).Error; err != nil {
		return nil, fmt.Errorf("failed to query total items: %w", err)
	}
	if err := s.getDb().Model(&models.Source{}).Count(&stats.TotalSources).Error; err != nil {
		return nil, fmt.Errorf("failed to query total sources: %w", err)
	}
	return stats, nil
}

// ClearReadabilityCache 清空阅读模式缓存
func (s *ReaderService) ClearReadabilityCache() (int64, error) {
	result := s.getDb().Where("1 = 1").Delete(&models.ReadabilityCache{})
	return result.RowsAffected, result.Error
}

// DatabaseInfo 数据库信息
type DatabaseInfo struct {
	Path            string `json:"path"`
	Size            int64  `json:"size"`
	BackupDir       string `json:"backupDir"`
	BackupDirExists bool   `json:"backupDirExists"`
	// JournalMode 透传当前日志模式（rollback/WAL），使“WAL 是否真的启用”可被观测，
	// 避免配置静默失效（审计 M-E1 根因：配置不可见）。
	JournalMode string `json:"journalMode"`
}

// GetDatabaseInfo 获取数据库文件信息
func (s *ReaderService) GetDatabaseInfo() (*DatabaseInfo, error) {
	dbPath := database.DBPath()
	info := &DatabaseInfo{Path: dbPath}

	if fi, err := os.Stat(dbPath); err == nil {
		info.Size = fi.Size()
		// 检查 WAL 文件
		if walFi, err := os.Stat(dbPath + "-wal"); err == nil {
			info.Size += walFi.Size()
		}
		if shmFi, err := os.Stat(dbPath + "-shm"); err == nil {
			info.Size += shmFi.Size()
		}
	}

	// 透传当前日志模式，使 WAL 是否启用可被前端/诊断观测（M-E1 根因：配置不可见）
	var jm string
	if err := s.db.Raw("PRAGMA journal_mode").Scan(&jm).Error; err == nil {
		info.JournalMode = jm
	}

	backupDir := getBackupDir(dbPath)
	info.BackupDir = backupDir
	if _, err := os.Stat(backupDir); err == nil {
		info.BackupDirExists = true
	}

	return info, nil
}

// getBackupDir 返回备份目录路径（数据库同级 backups/ 子目录）
func getBackupDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

// BackupDirPath 返回备份目录绝对路径（供 handler 安全构造下载路径）
func (s *ReaderService) BackupDirPath() string {
	return getBackupDir(database.DBPath())
}

// Vacuum 压缩数据库（VACUUM）
func (s *ReaderService) Vacuum() error {
	return s.execLocked("VACUUM")
}

// GetAllSettings 获取所有设置项，返回 key->value 映射
func (s *ReaderService) GetAllSettings() (map[string]string, error) {
	var settings []models.Setting
	if err := s.getDb().Find(&settings).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// GetSetting 获取单个设置项，不存在返回空字符串
func (s *ReaderService) GetSetting(key string) string {
	var setting models.Setting
	if err := s.getDb().Where("`key` = ?", key).First(&setting).Error; err != nil {
		return ""
	}
	return setting.Value
}

// GetSettingInt 获取整数型设置项，不存在或解析失败返回默认值
func (s *ReaderService) GetSettingInt(key string, defaultVal int) int {
	v := s.GetSetting(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// GetSettingBool 获取布尔型设置项，不存在或解析失败返回默认值
func (s *ReaderService) GetSettingBool(key string, defaultVal bool) bool {
	v := s.GetSetting(key)
	if v == "" {
		return defaultVal
	}
	return v == "true"
}

// UpdateSettings 批量更新设置项（upsert 语义）
func (s *ReaderService) UpdateSettings(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	err := s.getDb().Transaction(func(tx *gorm.DB) error {
		for k, v := range values {
			setting := models.Setting{Key: k, Value: v}
			if err := tx.Save(&setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 代理设置变更时，递增序列号使缓存失效
	if _, ok := values["proxyEnabled"]; ok {
		atomic.AddInt64(&s.proxySettingsSeq, 1)
	} else if _, ok := values["proxyUrl"]; ok {
		atomic.AddInt64(&s.proxySettingsSeq, 1)
	}
	return nil
}

// CleanupArticles 按留存策略清理已读文章
// retentionDays: 保留最近 N 天的文章（0=不限制）
// retentionMax: 保留最新 N 篇文章（0=不限制）
// excludeStarred: 收藏文章不清理
// excludeReadLater: 稍后阅读文章不清理
func (s *ReaderService) CleanupArticles(retentionDays, retentionMax int, excludeStarred, excludeReadLater bool) (int64, error) {
	if retentionDays <= 0 && retentionMax <= 0 {
		return 0, nil
	}
	tx := s.getDb().Begin()
	defer s.recoverAndRollback(tx)

	query := s.buildCleanupQuery(tx, excludeStarred, excludeReadLater, retentionDays)
	if err := s.applyCleanupRetentionMax(tx, &query, retentionMax, excludeStarred, excludeReadLater); err != nil {
		tx.Rollback()
		return 0, err
	}

	result := query.Delete(&models.Item{})
	if result.Error != nil {
		tx.Rollback()
		return 0, result.Error
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}

func (s *ReaderService) recoverAndRollback(tx *gorm.DB) {
	if r := recover(); r != nil {
		tx.Rollback()
	}
}

func (s *ReaderService) buildCleanupQuery(tx *gorm.DB, excludeStarred, excludeReadLater bool, retentionDays int) *gorm.DB {
	query := tx.Model(&models.Item{}).Where("isRead = ?", true)
	if excludeStarred {
		query = query.Where("isStarred = ?", false)
	}
	if excludeReadLater {
		query = query.Where("isReadLater = ?", false)
	}
	if retentionDays > 0 {
		query = query.Where("createdAt < ?", time.Now().AddDate(0, 0, -retentionDays))
	}
	return query
}

func (s *ReaderService) applyCleanupRetentionMax(tx *gorm.DB, query **gorm.DB, retentionMax int, excludeStarred, excludeReadLater bool) error {
	if retentionMax <= 0 {
		return nil
	}
	keepIDs, err := s.findCleanupKeepItems(tx, retentionMax, excludeStarred, excludeReadLater)
	if err != nil {
		return err
	}
	if len(keepIDs) > 0 {
		*query = (*query).Where("id NOT IN ?", keepIDs)
	}
	return nil
}

func (s *ReaderService) findCleanupKeepItems(tx *gorm.DB, retentionMax int, excludeStarred, excludeReadLater bool) ([]int, error) {
	var keepIDs []int
	subQuery := tx.Model(&models.Item{}).Where("isRead = ?", true)
	if excludeStarred {
		subQuery = subQuery.Where("isStarred = ?", false)
	}
	if excludeReadLater {
		subQuery = subQuery.Where("isReadLater = ?", false)
	}
	err := subQuery.Order("createdAt DESC").Limit(retentionMax).Pluck("id", &keepIDs).Error
	return keepIDs, err
}

