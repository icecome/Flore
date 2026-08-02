package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
)

// ExportScope 文章导出范围
type ExportScope struct {
	IDs         []int `json:"ids"`
	SourceID    *int  `json:"sourceId"`
	FolderID    *int  `json:"folderId"`
	Starred     bool  `json:"starred"`
	Unread      bool  `json:"unread"`
	ReadLater   bool  `json:"readLater"`
	HidePrivate bool  `json:"hidePrivate"`
}

// GetItemsForExport 根据导出范围查询文章，限制最大 5000 条防止 OOM
// 超过上限时返回错误，避免静默截断导致导出数据不完整
func (s *ReaderService) GetItemsForExport(scope ExportScope) ([]models.ItemWithSource, error) {
	const maxExportIDs = 5000
	if len(scope.IDs) > 0 {
		if len(scope.IDs) > maxExportIDs {
			return nil, fmt.Errorf("too many ids, max %d per export", maxExportIDs)
		}
		return s.getItemsByIDs(scope.IDs)
	}

	query := s.buildExportQuery(scope)
	var items []models.ItemWithSource
	if err := query.Order("Item.pubDate desc, Item.createdAt desc").Limit(maxExportIDs).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *ReaderService) buildExportQuery(scope ExportScope) *gorm.DB {
	query := s.getDb().Table("Item").
		Select("Item.*, Source.name as source_name, Source.url as source_url").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id")

	query = applyScopeFilter(query, scope)
	return query
}

func applyScopeFilter(query *gorm.DB, scope ExportScope) *gorm.DB {
	if scope.SourceID != nil {
		query = query.Where("Item.sourceId = ?", *scope.SourceID)
	} else if scope.FolderID != nil {
		query = query.Where("Source.folderId = ?", *scope.FolderID)
	}
	if scope.Starred {
		query = query.Where("Item.isStarred = ?", true)
	}
	if scope.Unread {
		query = query.Where("Item.isRead = ?", false)
	}
	if scope.ReadLater {
		query = query.Where("Item.isReadLater = ?", true)
	}
	if scope.HidePrivate && scope.SourceID == nil && scope.FolderID == nil {
		query = query.Where("Source.isPrivate = ?", false)
	}
	return query
}

// getItemsByIDs 根据 ID 列表查询文章，保持传入顺序
func (s *ReaderService) getItemsByIDs(ids []int) ([]models.ItemWithSource, error) {
	items := []models.ItemWithSource{}
	if err := s.getDb().Table("Item").
		Select("Item.*, Source.name as source_name, Source.url as source_url").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id").
		Where("Item.id IN ?", ids).
		Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// GetItemWithSource 获取单篇文章（含来源信息）
func (s *ReaderService) GetItemWithSource(id int) (*models.ItemWithSource, error) {
	var item models.ItemWithSource
	if err := s.getDb().Table("Item").
		Select("Item.*, Source.name as source_name, Source.url as source_url").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id").
		Where("Item.id = ?", id).
		Scan(&item).Error; err != nil {
		return nil, err
	}
	// Scan 不会返回 ErrRecordNotFound，需通过 ID 是否为 0 判断是否命中
	if item.ID == 0 {
		return nil, fmt.Errorf("item %d not found", id)
	}
	return &item, nil
}

// ExportItemMarkdown 将单篇文章转为 Markdown 字符串
func (s *ReaderService) ItemToMarkdown(item models.ItemWithSource) string {
	var b bytes.Buffer
	pubDate := ""
	if t := item.PubDate.Time(); t != nil {
		pubDate = t.Format(time.RFC3339)
	}
	author := ""
	if item.Author != nil {
		author = *item.Author
	}
	desc := ""
	if item.Desc != nil {
		desc = *item.Desc
	}

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: \"%s\"\n", escapeYamlString(item.Title)))
	b.WriteString(fmt.Sprintf("author: \"%s\"\n", escapeYamlString(author)))
	b.WriteString(fmt.Sprintf("link: \"%s\"\n", escapeYamlString(item.Link)))
	b.WriteString(fmt.Sprintf("source: \"%s\"\n", escapeYamlString(item.SourceName)))
	b.WriteString(fmt.Sprintf("sourceUrl: \"%s\"\n", escapeYamlString(item.SourceURL)))
	b.WriteString(fmt.Sprintf("pubDate: \"%s\"\n", pubDate))
	b.WriteString(fmt.Sprintf("isRead: %t\n", item.IsRead))
	b.WriteString(fmt.Sprintf("isStarred: %t\n", item.IsStarred))
	b.WriteString(fmt.Sprintf("isReadLater: %t\n", item.IsReadLater))
	b.WriteString("---\n\n")
	b.WriteString(desc)
	b.WriteString("\n")
	return b.String()
}

// escapeYamlString 转义 YAML 双引号字符串中的特殊字符
func escapeYamlString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// GenerateSafeFilename 生成安全的 Markdown 文件名
func (s *ReaderService) GenerateSafeFilename(item models.ItemWithSource, used map[string]bool) string {
	base := time.Now().Format("2006-01-02")
	if t := item.PubDate.Time(); t != nil {
		base = t.Format("2006-01-02")
	}
	slug := slugify(item.Title)
	if slug == "" {
		slug = fmt.Sprintf("article-%d", item.ID)
	}
	name := fmt.Sprintf("%s-%s.md", base, slug)
	if !used[name] {
		used[name] = true
		return name
	}
	// 处理重名
	for i := 1; ; i++ {
		name = fmt.Sprintf("%s-%s-%d.md", base, slug, i)
		if !used[name] {
			used[name] = true
			return name
		}
	}
}

// slugifyRe 是 slugify 使用的预编译正则，避免每次调用都重新编译
var slugifyRe = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// slugify 将标题转为 URL 友好的短横线形式
func slugify(title string) string {
	if title == "" {
		return ""
	}
	// 保留中英文、数字
	slug := slugifyRe.ReplaceAllString(title, "-")
	slug = strings.Trim(slug, "-")
	slug = strings.ToLower(slug)
	if len(slug) > 80 {
		slug = slug[:80]
	}
	return slug
}

// ExportItemsMarkdown 将文章导出为 ZIP 格式的 Markdown 压缩包
func (s *ReaderService) ExportItemsMarkdown(items []models.ItemWithSource, w io.Writer) error {
	zw := zip.NewWriter(w)

	used := make(map[string]bool)
	for _, item := range items {
		filename := s.GenerateSafeFilename(item, used)
		content := s.ItemToMarkdown(item)
		f, err := zw.Create(filename)
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := f.Write([]byte(content)); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

// exportItemJSON 是 JSON 导出的内部结构
type exportItemJSON struct {
	ID          int     `json:"id"`
	SourceID    int     `json:"sourceId"`
	Title       string  `json:"title"`
	Link        string  `json:"link"`
	Desc        *string `json:"desc"`
	Author      *string `json:"author"`
	PubDate     *string `json:"pubDate"`
	IsRead      bool    `json:"isRead"`
	IsStarred   bool    `json:"isStarred"`
	IsReadLater bool    `json:"isReadLater"`
	CreatedAt   string  `json:"createdAt"`
	SourceName  string  `json:"sourceName"`
	SourceURL   string  `json:"sourceUrl"`
}

// ExportItemsJSON 将文章导出为 JSON
func (s *ReaderService) ExportItemsJSON(items []models.ItemWithSource, w io.Writer) error {
	exported := make([]exportItemJSON, 0, len(items))
	for _, item := range items {
		pubDate := ""
		if t := item.PubDate.Time(); t != nil {
			pubDate = t.Format(time.RFC3339)
		}
		exported = append(exported, exportItemJSON{
			ID:          item.ID,
			SourceID:    item.SourceID,
			Title:       item.Title,
			Link:        item.Link,
			Desc:        item.Desc,
			Author:      item.Author,
			PubDate:     &pubDate,
			IsRead:      item.IsRead,
			IsStarred:   item.IsStarred,
			IsReadLater: item.IsReadLater,
			CreatedAt:   item.CreatedAtTime.Time().Format(time.RFC3339),
			SourceName:  item.SourceName,
			SourceURL:   item.SourceURL,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]interface{}{
		"exportedAt": time.Now().Format(time.RFC3339),
		"count":      len(exported),
		"items":      exported,
	}); err != nil {
		return fmt.Errorf("failed to encode json: %w", err)
	}
	return nil
}
