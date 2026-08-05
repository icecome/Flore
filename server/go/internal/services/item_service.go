package services

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rss/go-server/internal/models"
	"gorm.io/gorm"
)

func (s *ReaderService) buildItemQuery(sourceID *int, folderID *int, onlyUnread bool, onlyStarred bool, onlyReadLater bool, hidePrivate bool) *gorm.DB {
	query := s.getDb().Table("Item").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id")
	if sourceID != nil {
		query = query.Where("Item.sourceId = ?", *sourceID)
	}
	if folderID != nil {
		// 递归获取文件夹及其所有子文件夹的ID
		folderIDs, err := s.getAllFolderAllDescendantIDs(*folderID)
		if err != nil {
			slog.Error("failed to get folder descendant IDs", "error", err)
			// 查询失败返回空结果，避免影响其他功能
			query = query.Where("1 = 0")
		} else {
			query = query.Where("Source.folderId IN (?)", folderIDs)
		}
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
		// 递归获取文件夹及其所有子文件夹的ID
		folderIDs, err := s.getAllFolderAllDescendantIDs(*folderID)
		if err != nil {
			return fmt.Errorf("failed to get folder descendant IDs: %w", err)
		}
		query = query.Where("sourceId IN (?)", s.getDb().Model(&models.Source{}).Select("id").Where("folderId IN (?)", folderIDs))
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
		// 递归获取文件夹及其所有子文件夹的ID
		folderIDs, err := s.getAllFolderAllDescendantIDs(*folderID)
		if err != nil {
			slog.Error("failed to get folder descendant IDs", "error", err)
			// 查询失败返回空结果，避免影响其他功能
			query = query.Where("1 = 0")
		} else {
			query = query.Where("Source.folderId IN (?)", folderIDs)
		}
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
		// 递归获取文件夹及其所有子文件夹的ID
		folderIDs, err := s.getAllFolderAllDescendantIDs(*folderID)
		if err != nil {
			return fmt.Errorf("failed to get folder descendant IDs: %w", err)
		}
		query = query.Where("sourceId IN (?)", s.getDb().Model(&models.Source{}).Select("id").Where("folderId IN (?)", folderIDs))
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
	matched, err := regexp.MatchString(`^\d+$`, byline)
	if err != nil {
		return false
	}
	if matched {
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
// newCount 为本轮新增文章数，用于自适应调度计算 NextCheckAtUnix
// interval 为该源的抓取间隔（分钟），用于自适应调度计算下次检查时间
