package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rss/go-server/internal/services"
	"gorm.io/gorm"
)

func (h *ReaderHandler) GetSources(c *gin.Context) {
	sources, err := h.service.GetSources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, sources)
}

// GetSource 获取单个订阅源
func (h *ReaderHandler) GetSource(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	source, err := h.service.GetSource(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		}
		return
	}
	c.JSON(http.StatusOK, source)
}

// UpdateSource 更新订阅源（支持 folderId）
// 使用具名结构体替代 map[string]interface{}，提供编译期类型安全
func (h *ReaderHandler) UpdateSource(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Name           *string `json:"name"`
		URL            *string `json:"url"`
		FolderID       *int    `json:"folderId"`
		Active         *bool   `json:"active"`
		IsPrivate      *bool   `json:"isPrivate"`
		HideInTimeline *bool   `json:"hideInTimeline"`
		Interval       *int    `json:"interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	if err := h.service.UpdateSource(id, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteSource 删除订阅源
func (h *ReaderHandler) DeleteSource(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteSource(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteSourcesBatch 批量删除订阅源
func (h *ReaderHandler) DeleteSourcesBatch(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	// 限制单次批量删除上限，防止超大事务长时间持锁
	const maxBatchSize = 500
	if len(req.IDs) > maxBatchSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("too many ids, max %d per batch", maxBatchSize)})
		return
	}

	if err := h.service.DeleteSourcesBatch(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetFolders 获取所有文件夹
func (h *ReaderHandler) GetFolders(c *gin.Context) {
	folders, err := h.service.GetFolders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, folders)
}

// CreateFolder 创建文件夹
func (h *ReaderHandler) CreateFolder(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	folder, err := h.service.CreateFolder(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusCreated, folder)
}

// UpdateFolder 更新文件夹
func (h *ReaderHandler) UpdateFolder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	if err := h.service.UpdateFolder(id, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteFolder 删除文件夹
func (h *ReaderHandler) DeleteFolder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteFolder(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ImportOPML 导入 OPML（限制 body 大小为 10MB）
func (h *ReaderHandler) ImportOPML(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20) // 10MB
	xml, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	if err := h.service.ImportOPML(string(xml)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ExportOPML 导出 OPML
func (h *ReaderHandler) ExportOPML(c *gin.Context) {
	xml, err := h.service.ExportOPML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.Header("Content-Type", "application/xml")
	c.Header("Content-Disposition", "attachment; filename=\"subscriptions.opml\"")
	c.String(http.StatusOK, xml)
}

// FetchSource 触发单个订阅源抓取（异步，立即返回）
func (h *ReaderHandler) FetchSource(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.service.Coordinator() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "coordinator not ready"})
		return
	}
	h.service.Coordinator().Submit(id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// FetchAllSources 触发所有订阅源抓取（异步，立即返回）
func (h *ReaderHandler) FetchAll(c *gin.Context) {
	if h.service.Coordinator() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "coordinator not ready"})
		return
	}
	// 默认跳过退避期僵尸源以加速刷新；?force=true 时强制抓取全部（含重试僵尸源）
	force := c.Query("force") == "true"
	h.service.Coordinator().SubmitAllSources(!force)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// FetchStatusResponse 抓取状态响应。
// 是「抓取任务结果」领域对象的轻量形态（P2-12）：携带完成信号与本轮新增数，
// 前端据此停止旋转并发送系统通知，根治此前用全库未读差值近似导致的计数失真（C-02）。
type FetchStatusResponse struct {
	Fetching bool `json:"fetching"`
	NewItems int  `json:"newItems"`
}

// FetchStatus 返回当前抓取状态与本轮新增数，供前端轮询判断何时停止刷新按钮旋转
func (h *ReaderHandler) FetchStatus(c *gin.Context) {
	resp := FetchStatusResponse{}
	if coord := h.service.Coordinator(); coord != nil {
		resp.Fetching = coord.IsFetching()
		resp.NewItems = coord.LastRoundNewItems()
	}
	c.JSON(http.StatusOK, resp)
}

// FetchFolder 触发文件夹下所有订阅源抓取（异步，立即返回）
func (h *ReaderHandler) FetchFolder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.service.Coordinator() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "coordinator not ready"})
		return
	}
	h.service.Coordinator().SubmitFolder(id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkAllRead 全部已读（单个订阅源）
func (h *ReaderHandler) MarkSourceAllRead(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.MarkAllRead(&id, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkAllReadItems 批量标记已读（支持 sourceId/folderId/全部）
func (h *ReaderHandler) CreateSource(c *gin.Context) {
	var req services.CreateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}

	if req.Interval <= 0 {
		req.Interval = 120
	}

	source, err := h.service.CreateSource(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusCreated, source)
}

// SearchItems 搜索文章
