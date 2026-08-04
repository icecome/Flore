package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rss/go-server/internal/services"
	"gorm.io/gorm"
)

func (h *ReaderHandler) MarkItemsRead(c *gin.Context) {
	var req struct {
		SourceID *int `json:"sourceId"`
		FolderID *int `json:"folderId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	if err := h.service.MarkAllRead(req.SourceID, req.FolderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkReadBatchItems 批量标记已读/未读（按文章 ID 列表）
func (h *ReaderHandler) MarkReadBatchItems(c *gin.Context) {
	var req struct {
		IDs  []int `json:"ids" binding:"required"`
		Read bool  `json:"read"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	if err := h.service.MarkReadBatch(req.IDs, req.Read); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ClearFolderSources 将文件夹内所有订阅源移出分组
func (h *ReaderHandler) ClearFolderSources(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.ClearFolder(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetItemsCount 获取文章总数
func (h *ReaderHandler) GetItemsCount(c *gin.Context) {
	var sourceID *int
	if sid := c.Query("sourceId"); sid != "" {
		id, err := strconv.Atoi(sid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sourceId"})
			return
		}
		sourceID = &id
	}

	var folderID *int
	if fid := c.Query("folderId"); fid != "" {
		id, err := strconv.Atoi(fid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folderId"})
			return
		}
		folderID = &id
	}

	onlyUnread := c.Query("unread") == "true"
	onlyStarred := c.Query("starred") == "true"
	onlyReadLater := c.Query("readLater") == "true"
	hidePrivate := c.Query("hidePrivate") == "true"

	count, err := h.service.CountItems(sourceID, folderID, onlyUnread, onlyStarred, onlyReadLater, hidePrivate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// GetItems 获取文章列表
func (h *ReaderHandler) GetItems(c *gin.Context) {
	var sourceID *int
	if sid := c.Query("sourceId"); sid != "" {
		id, err := strconv.Atoi(sid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sourceId"})
			return
		}
		sourceID = &id
	}

	var folderID *int
	if fid := c.Query("folderId"); fid != "" {
		id, err := strconv.Atoi(fid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folderId"})
			return
		}
		folderID = &id
	}

	onlyUnread := c.Query("unread") == "true"
	onlyStarred := c.Query("starred") == "true"
	onlyReadLater := c.Query("readLater") == "true"
	hidePrivate := c.Query("hidePrivate") == "true"
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		limit = 50
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		offset = 0
	}
	orderBy := c.DefaultQuery("orderBy", "newest")
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	items, err := h.service.GetItems(sourceID, folderID, onlyUnread, onlyStarred, onlyReadLater, hidePrivate, limit, offset, orderBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, items)
}

// GetItem 获取单篇文章
func (h *ReaderHandler) GetItem(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	item, err := h.service.GetItem(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		}
		return
	}
	c.JSON(http.StatusOK, item)
}

// MarkRead 标记已读
func (h *ReaderHandler) MarkRead(c *gin.Context) {
	h.toggleRead(c, true)
}

// MarkUnread 标记未读
func (h *ReaderHandler) MarkUnread(c *gin.Context) {
	h.toggleRead(c, false)
}

func (h *ReaderHandler) toggleRead(c *gin.Context, read bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.MarkRead(id, read); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkStar 收藏
func (h *ReaderHandler) MarkStar(c *gin.Context) {
	h.toggleStar(c, true)
}

// MarkUnstar 取消收藏
func (h *ReaderHandler) MarkUnstar(c *gin.Context) {
	h.toggleStar(c, false)
}

// MarkReadLater 标记稍后阅读
func (h *ReaderHandler) MarkReadLater(c *gin.Context) {
	h.toggleReadLater(c, true)
}

// MarkUnreadLater 取消稍后阅读
func (h *ReaderHandler) MarkUnreadLater(c *gin.Context) {
	h.toggleReadLater(c, false)
}

// ClearReadLater 清空稍后阅读标记
func (h *ReaderHandler) ClearReadLater(c *gin.Context) {
	var sourceID *int
	if sid := c.Query("sourceId"); sid != "" {
		id, err := strconv.Atoi(sid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sourceId"})
			return
		}
		sourceID = &id
	}

	var folderID *int
	if fid := c.Query("folderId"); fid != "" {
		id, err := strconv.Atoi(fid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folderId"})
			return
		}
		folderID = &id
	}

	if err := h.service.ClearReadLater(sourceID, folderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ReaderHandler) toggleReadLater(c *gin.Context, readLater bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.MarkReadLater(id, readLater); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ReaderHandler) toggleStar(c *gin.Context, starred bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.MarkStarred(id, starred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetUnreadCount 获取未读数
func (h *ReaderHandler) GetUnreadCount(c *gin.Context) {
	hidePrivate := c.Query("hidePrivate") == "true"
	count, err := h.service.GetUnreadCount(hidePrivate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// ExportDatabase 导出 SQLite 数据库快照
func (h *ReaderHandler) SearchItems(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing q parameter"})
		return
	}
	// 限制 keyword 长度，防止超长输入拖慢 FTS5 全文匹配
	const maxKeywordLen = 200
	if len(keyword) > maxKeywordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("keyword too long, max %d chars", maxKeywordLen)})
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		limit = 50
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	items, err := h.service.SearchItems(keyword, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, items)
}

// IndexItemForSearch 为单篇文章建立/更新 FTS5 索引
func (h *ReaderHandler) IndexItemForSearch(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.IndexItemForSearch(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RebuildSearchIndex 重建所有文章的 FTS5 索引
func (h *ReaderHandler) RebuildSearchIndex(c *gin.Context) {
	if err := h.service.RebuildSearchIndex(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetReadability 获取文章阅读模式内容
func (h *ReaderHandler) GetReadability(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	refresh := c.Query("refresh") == "true"
	result, err := h.service.GetReadability(id, refresh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ExportItems 批量导出文章
func (h *ReaderHandler) ExportItems(c *gin.Context) {
	var req services.ExportScope
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	items, err := h.service.GetItemsForExport(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no items to export"})
		return
	}

	format := c.DefaultQuery("format", "markdown")
	date := time.Now().Format("2006-01-02")

	switch format {
	case "json":
		buf := new(bytes.Buffer)
		if err := h.service.ExportItemsJSON(items, buf); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
			return
		}
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"articles-%s.json\"", date))
		if _, err := buf.WriteTo(c.Writer); err != nil {
			slog.Error("failed to write exported json", "error", err)
		}
	default:
		buf := new(bytes.Buffer)
		if err := h.service.ExportItemsMarkdown(items, buf); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"articles-%s.zip\"", date))
		if _, err := buf.WriteTo(c.Writer); err != nil {
			slog.Error("failed to write exported markdown", "error", err)
		}
	}
}

// ExportItemMarkdown 导出单篇文章为 Markdown
func (h *ReaderHandler) ExportItemMarkdown(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	item, err := h.service.GetItemWithSource(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}

	content := h.service.ItemToMarkdown(*item)
	filename := h.service.GenerateSafeFilename(*item, make(map[string]bool))
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.String(http.StatusOK, content)
}

// ApplyFilterRules 对单篇文章应用过滤规则
func (h *ReaderHandler) ApplyFilterRules(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.service.ApplyFilterRules(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListFilterRules 列出所有过滤规则
func (h *ReaderHandler) ListFilterRules(c *gin.Context) {
	rules, err := h.service.GetFilterRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, rules)
}

// CreateFilterRule 创建过滤规则
func (h *ReaderHandler) CreateFilterRule(c *gin.Context) {
	var input services.CreateFilterRuleRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	rule, err := h.service.CreateFilterRule(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// UpdateFilterRule 更新过滤规则
func (h *ReaderHandler) UpdateFilterRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var input services.CreateFilterRuleRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	rule, err := h.service.UpdateFilterRule(id, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// DeleteFilterRule 删除过滤规则
func (h *ReaderHandler) DeleteFilterRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.service.DeleteFilterRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TestFilterRule 测试规则匹配，返回最近匹配的文章列表
func (h *ReaderHandler) TestFilterRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	items, err := h.service.TestFilterRule(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, items)
}

// ProxyOriginal 代理原文页面，用于在当前框架内显示
