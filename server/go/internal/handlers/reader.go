package handlers

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/rss/go-server/internal/services"
)

// 预编译正则，避免每次函数调用重复编译
var (
	linkRePre    = regexp.MustCompile(`(?is)<link\s[^>]*rel\s*=\s*["']?stylesheet["']?[^>]*>`)
	hrefRePre    = regexp.MustCompile(`href\s*=\s*["']([^"']+)["']`)
	cspMetaRePre = regexp.MustCompile(`(?is)<meta\s[^>]*http-equiv\s*=\s*["']?Content-Security-Policy["']?[^>]*>`)
	urlRePre     = regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)
)

// rateLimiter 简单的基于 IP 的速率限制中间件
type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rateEntry
	limit    int
	window   time.Duration
	stopCh   chan struct{}
}

type rateEntry struct {
	count    int
	expireAt time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		visitors: make(map[string]*rateEntry),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, entry := range rl.visitors {
				if now.After(entry.expireAt) {
					delete(rl.visitors, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *rateLimiter) stop() {
	close(rl.stopCh)
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.visitors[ip]
	now := time.Now()
	if !exists || now.After(entry.expireAt) {
		rl.visitors[ip] = &rateEntry{
			count:    1,
			expireAt: now.Add(rl.window),
		}
		return true
	}

	if entry.count >= rl.limit {
		return false
	}

	entry.count++
	return true
}

func rateLimitMiddleware(rl *rateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		// 本地回环地址豁免：Flore 是本地优先应用，前端与后端在同一机器，
		// 不应被自己触发的请求风暴限制（如 React StrictMode double invoke 或轮询叠加）。
		if ip == "127.0.0.1" || ip == "::1" {
			c.Next()
			return
		}
		if !rl.allow(ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ReaderHandler 阅读器 HTTP 处理器
type ReaderHandler struct {
	service     *services.ReaderService
	rateLimiter *rateLimiter
	// baseURL 是后端服务的可信基础地址（scheme://host:port），
	// 用于代理端点重写 URL，避免使用客户端可控的 Host 头
	baseURL string
	// frameCheckCache 缓存原文是否可被 iframe 直接嵌入（基于 X-Frame-Options / CSP frame-ancestors 预检），避免重复文章重复网络预检
	frameCheckCache sync.Map
}

// NewReaderHandler 创建处理器
func NewReaderHandler() *ReaderHandler {
	return &ReaderHandler{
		service: services.NewReaderService(),
		baseURL: resolveBackendBaseURL(),
	}
}

// resolveBackendBaseURL 从环境变量构建可信的后端基础地址
func resolveBackendBaseURL() string {
	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}
	return "http://" + bindAddr + ":" + port
}

// authMiddleware 返回一个 Bearer Token 认证中间件；token 为空时返回 nil
func authMiddleware(token string) gin.HandlerFunc {
	if token == "" {
		return nil
	}
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if auth == "" || !strings.HasPrefix(auth, prefix) || len(auth) != len(prefix)+len(token) || subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(token)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RegisterRoutes 注册路由，apiToken 非空时对敏感路由启用认证
func (h *ReaderHandler) RegisterRoutes(r *gin.Engine, apiToken string) {
	// 速率限制：每 IP 每 60 秒最多 120 次请求
	h.rateLimiter = newRateLimiter(120, 60*time.Second)
	api := r.Group("/api", rateLimitMiddleware(h.rateLimiter))

	// 敏感路由组（数据库导入/导出、OPML 导入、破坏性写操作），需要 Bearer Token 认证
	auth := authMiddleware(apiToken)
	sensitive := api
	if auth != nil {
		sensitive = api.Group("", auth)
	}
	{
		// 订阅源
		api.GET("/sources", h.GetSources)
		api.GET("/sources/:id", h.GetSource)
		sensitive.PUT("/sources/:id", h.UpdateSource)
		sensitive.DELETE("/sources/:id", h.DeleteSource)
		sensitive.DELETE("/sources/delete-batch", h.DeleteSourcesBatch)
		api.POST("/sources/:id/fetch", h.FetchSource)
		api.POST("/sources/fetch-all", h.FetchAll)
		api.GET("/sources/fetch-status", h.FetchStatus)
		api.POST("/sources/:id/read-all", h.MarkSourceAllRead)
		api.POST("/sources/create", h.CreateSource)

		// 文件夹/分类
		api.GET("/folders", h.GetFolders)
		api.POST("/folders", h.CreateFolder)
		sensitive.PUT("/folders/:id", h.UpdateFolder)
		sensitive.DELETE("/folders/:id", h.DeleteFolder)
		api.POST("/folders/:id/clear", h.ClearFolderSources)
		api.POST("/folders/:id/fetch", h.FetchFolder)

		// OPML 导入（敏感：可批量写入数据）
		sensitive.POST("/opml/import", h.ImportOPML)
		api.GET("/opml/export", h.ExportOPML)

		// 文章
		api.GET("/items", h.GetItems)
		api.GET("/items/count", h.GetItemsCount)
		api.GET("/items/:id", h.GetItem)
		api.POST("/items/:id/read", h.MarkRead)
		api.POST("/items/:id/unread", h.MarkUnread)
		api.POST("/items/:id/star", h.MarkStar)
		api.POST("/items/:id/unstar", h.MarkUnstar)
		api.POST("/items/:id/read-later", h.MarkReadLater)
		api.POST("/items/:id/unread-later", h.MarkUnreadLater)
		api.POST("/items/read-later/clear", h.ClearReadLater)
		api.POST("/items/read-all", h.MarkItemsRead)
		api.POST("/items/mark-read", h.MarkReadBatchItems)
		api.GET("/items/search", h.SearchItems)
		api.POST("/items/:id/index-search", h.IndexItemForSearch)
		api.POST("/items/index-search/rebuild", h.RebuildSearchIndex)
		api.GET("/items/:id/readability", h.GetReadability)
		api.POST("/items/export", h.ExportItems)
		api.GET("/items/:id/export.md", h.ExportItemMarkdown)
		api.POST("/items/:id/apply-filters", h.ApplyFilterRules)

		// 过滤规则
		api.GET("/filter-rules", h.ListFilterRules)
		api.POST("/filter-rules", h.CreateFilterRule)
		sensitive.PUT("/filter-rules/:id", h.UpdateFilterRule)
		sensitive.DELETE("/filter-rules/:id", h.DeleteFilterRule)
		api.POST("/filter-rules/:id/test", h.TestFilterRule)

		// 原文代理
		// 预检原文是否可被 iframe 直接嵌入（检查 X-Frame-Options / CSP frame-ancestors），供前端决定直连还是走代理
		api.GET("/proxy/check/:id", h.CheckFrameable)
		api.GET("/proxy/:id", h.ProxyOriginal)
		// 图片/CSS/图标代理为只读透传，已被全局速率限制 + 类型白名单 + 尺寸上限保护，
		// 且需经 <img>/<link> 消费（无法携带 Authorization 头），故置于非敏感组。
		api.GET("/image-proxy", h.ProxyImage)
		api.GET("/css-proxy", h.ProxyCSS)
		api.GET("/favicon-proxy", h.ProxyFavicon)

		// 统计
		api.GET("/stats/unread", h.GetUnreadCount)

		// 数据库备份/恢复（敏感：可替换整个数据库）
		sensitive.GET("/database/export", h.ExportDatabase)
		sensitive.POST("/database/restore", h.ImportDatabase)

		// 数据管理
		api.GET("/database/info", h.GetDatabaseInfo)
		api.GET("/cache/stats", h.GetCacheStats)
		api.POST("/cache/clear-readability", h.ClearReadabilityCache)
		api.POST("/cache/rebuild-search", h.RebuildSearchIndex)
		api.POST("/database/vacuum", h.VacuumDatabase)

		// 备份管理
		api.GET("/backups", h.ListBackups)
		api.POST("/backups/create", h.CreateBackup)
		sensitive.DELETE("/backups/:name", h.DeleteBackup)
		api.POST("/backups/cleanup", h.CleanupBackups)
		api.POST("/backups/:name/restore", h.RestoreBackup)
		api.GET("/backups/:name/download", h.DownloadBackup)
		api.GET("/backups/:name/contents", h.GetBackupContents)
		sensitive.POST("/backups/:name/restore-config", h.RestoreConfigFromBackup)
		sensitive.POST("/backups/:name/restore-opml", h.RestoreOPMLFromBackup)
		// 新增：导入外部备份并返回内容清单
		api.POST("/backups/restore/import", h.ImportBackupAndValidate)

		// 设置（Settings 表，键值对存储）
		api.GET("/settings", h.GetSettings)
		sensitive.PUT("/settings", h.UpdateSettings)
		// GetSetting 单 key 读取纳入 sensitive 组：
		// settings 可能含 proxyUrl 等敏感字段，未认证访问可被探测
		sensitive.GET("/settings/:key", h.GetSetting)

		// 文章留存清理
		api.POST("/articles/cleanup", h.CleanupArticles)

		// 诊断包
		api.GET("/diagnostic/items", h.GetDiagnoseItems)
		sensitive.POST("/diagnostic/generate", h.GenerateDiagnosticPackage)
		sensitive.GET("/diagnostic/:path/download", h.ExtractDiagnosticPackage)

		// 版本信息
		api.GET("/version", h.GetVersion)
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

// isAllowedImageContentType 检查 Content-Type 是否为安全的图片类型
// 拒绝 image/svg+xml 防止 XSS，拒绝非图片类型防止内容嗅探
func isAllowedImageContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	// 取分号前的主类型
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	for _, allowed := range []string{
		"image/jpeg", "image/png", "image/gif", "image/webp", "image/avif", "image/bmp", "image/x-icon",
	} {
		if ct == allowed {
			return true
		}
	}
	return false
}

// safeError logs the detailed error server-side and returns a safe message.
// 对于已知业务错误（如 "invalid"、"not found"、"missing"）直接返回；
// 对于未知错误返回通用提示，避免泄露内部路径和实现细节。
func safeError(detailed string) string {
	slog.Error(detailed)
	lower := strings.ToLower(detailed)
	for _, safe := range []string{"invalid", "not found", "missing", "already exists", "no allowed fields"} {
		if strings.Contains(lower, safe) {
			return detailed
		}
	}
	return "internal_server_error"
}

// GetSources 获取订阅源列表
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
func (h *ReaderHandler) ExportDatabase(c *gin.Context) {
	snapshotPath, err := h.service.ExportDatabase()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	defer os.Remove(snapshotPath)

	filename := fmt.Sprintf("rss-backup-%s.db", time.Now().Format("2006-01-02-150405"))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.File(snapshotPath)
}

// ImportDatabase 导入 SQLite 数据库文件并替换当前数据库
func (h *ReaderHandler) ImportDatabase(c *gin.Context) {
	// 限制上传文件大小为 256MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<20)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file or file too large"})
		return
	}
	defer file.Close()

	if err := h.service.ImportDatabase(file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CreateSource 创建订阅源
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
func (h *ReaderHandler) ProxyOriginal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	originalURL, err := h.service.GetOriginalURL(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}

	httpClient := h.service.BuildFetchHTTPClient()
	resp, err := services.ProxyOriginalWithClient(originalURL, httpClient)
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(proxyErrorPage(fmt.Sprintf("加载失败：%s", err.Error()), originalURL)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(proxyErrorPage(fmt.Sprintf("原文返回 HTTP %d", resp.StatusCode), originalURL)))
		return
	}

	// 复制响应头
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}
	c.Header("Content-Type", contentType)

	// 最小代理：我们的响应不转发上游的 X-Frame-Options / CSP frame-ancestors，
	// 因此 iframe 可正常嵌入。安全隔离由前端 iframe 的 sandbox 控制。

	// 读取并注入 base target，限制响应体大小防止 OOM
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(proxyErrorPage(fmt.Sprintf("读取原文失败：%s", err.Error()), originalURL)))
		return
	}

	// 如果 HTML，注入 base 标签、viewport 与 iframe 适配样式
	if strings.Contains(contentType, "text/html") {
		htmlContent := string(body)
		// 剥离 CSP meta 标签：原文页面可能包含 frame-ancestors 限制，
		// 导致 iframe 渲染时被浏览器阻止。作为代理，安全由我们自己的 CSP 控制。
		htmlContent = stripCSPMetaTags(htmlContent)

		// 网页模式（最小代理）内容处理策略：仅剥离 CSP meta 标签 + 注入 base，
		// 不内联 CSS、不重写任何 URL（含 <style> 与 <img>）。
		// CSS/JS/图片全部由原站直连加载，保证样式与交互完整，且不会破坏页面结构。
		// 这与 Folo / Fluent Reader 用真实浏览器内核直连原站的做法一致。

		baseTag := fmt.Sprintf(`<base href="%s" target="_blank">`, html.EscapeString(originalURL))
		viewportMeta := `<meta name="viewport" content="width=device-width, initial-scale=1.0">`
		iframeCSS := `<style>html,body{margin:0;padding:0;max-width:100%;box-sizing:border-box;}body{padding:16px;}</style>`

		injectHead := baseTag + viewportMeta + iframeCSS
		if headIdx := strings.Index(strings.ToLower(htmlContent), "<head>"); headIdx >= 0 {
			htmlContent = htmlContent[:headIdx+6] + injectHead + htmlContent[headIdx+6:]
		} else if htmlIdx := strings.Index(strings.ToLower(htmlContent), "<html"); htmlIdx >= 0 {
			closeIdx := strings.Index(htmlContent[htmlIdx:], ">") + htmlIdx + 1
			htmlContent = htmlContent[:closeIdx] + "<head>" + injectHead + "</head>" + htmlContent[closeIdx:]
		}
		c.Data(resp.StatusCode, contentType, []byte(htmlContent))
		return
	}

	c.Data(resp.StatusCode, contentType, body)
}

// frameCheckResult 缓存原文嵌入预检结果
type frameCheckResult struct {
	frameable bool
	finalURL  string
}

// CheckFrameable 预检原文是否可被 iframe 直接嵌入。
// 通过检查响应头的 X-Frame-Options 与 Content-Security-Policy 的 frame-ancestors 指令决定：
//   - 不可嵌入（设了限制）→ 前端改走 /proxy/:id 最小代理（清头 + 注入 base）
//   - 可嵌入 → 前端直连 item.link（CSS/JS/图片全由原站加载，最稳）
//
// 返回 finalURL 为跟随重定向后的最终地址，供前端直连使用。
func (h *ReaderHandler) CheckFrameable(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	originalURL, err := h.service.GetOriginalURL(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}
	frameable, finalURL := h.checkFrameable(originalURL)
	c.JSON(http.StatusOK, gin.H{"frameable": frameable, "url": finalURL})
}

// checkFrameable 发起一次轻量请求，检查原文是否被禁止 iframe 嵌入。
// 结果按原文 URL 缓存，避免重复文章重复预检。
func (h *ReaderHandler) checkFrameable(originalURL string) (bool, string) {
	if v, ok := h.frameCheckCache.Load(originalURL); ok {
		r := v.(frameCheckResult)
		return r.frameable, r.finalURL
	}

	httpClient := h.service.BuildFetchHTTPClient()
	// 仅读取响应头即可判定，不下载正文
	req, err := http.NewRequest(http.MethodGet, originalURL, nil)
	if err != nil {
		return false, originalURL
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FloreReader/1.0)")

	resp, err := httpClient.Do(req)
	if err != nil {
		// 预检失败（网络不可达等）→ 保守走代理，代理可绕过 XFO
		return false, originalURL
	}
	defer resp.Body.Close()
	// 丢弃响应体，仅取头判定
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	finalURL := resp.Request.URL.String()
	frameable := true
	if xfo := resp.Header.Get("X-Frame-Options"); xfo != "" {
		frameable = false
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp != "" {
		if strings.Contains(strings.ToLower(csp), "frame-ancestors") {
			frameable = false
		}
	}
	h.frameCheckCache.Store(originalURL, frameCheckResult{frameable: frameable, finalURL: finalURL})
	return frameable, finalURL
}

// ProxyImage 代理图片资源，通过后端设置正确的 Referer 等请求头绕过 CDN 防盗链。
// 前端阅读模式中的图片通过此端点加载。
// 参数：
//   - url: 图片原始 URL（必需）
//   - ref: 嵌入该图片的页面 URL（即文章原文 URL），用于设置 Referer 头绕过防盗链（可选）
func (h *ReaderHandler) ProxyImage(c *gin.Context) {
	imageURL := c.Query("url")
	if imageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url parameter"})
		return
	}

	referer := c.Query("ref")
	client := h.service.BuildFetchHTTPClient()

	resp, err := services.FetchImage(imageURL, referer, client)
	if err != nil {
		slog.Error("image proxy fetch failed", "url", imageURL, "ref", referer, "error", err)
		c.String(http.StatusBadGateway, "image proxy error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 读取部分响应体以便记录
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		slog.Error("image proxy upstream non-200", "url", imageURL, "ref", referer, "status", resp.StatusCode, "body", string(errBody))
		c.String(http.StatusBadGateway, "image proxy: upstream returned HTTP %d", resp.StatusCode)
		return
	}

	// 复制响应头，限制 Content-Type 为图片白名单，拒绝 SVG 防 XSS
	contentType := resp.Header.Get("Content-Type")
	if !isAllowedImageContentType(contentType) {
		slog.Warn("image proxy rejected content-type", "url", imageURL, "contentType", contentType)
		c.String(http.StatusUnsupportedMediaType, "image proxy: unsupported content type")
		return
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Access-Control-Allow-Origin", "*")

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB 限制
	if err != nil {
		c.String(http.StatusBadGateway, "image proxy: read error: "+err.Error())
		return
	}
	c.Data(resp.StatusCode, contentType, body)
}

// ProxyCSS 代理 CSS 文件，通过后端下载并重写其中的 url() 引用为图片代理地址。
// 解决跨域 iframe 中外部样式无法加载的问题（CDN 防盗链，CSP 限制）。
// 参数：
//   - url: CSS 文件原始 URL（必需）
//   - ref: 嵌入该 CSS 的页面 URL（即文章原文 URL），用于设置 Referer 头绕过防盗链（可选）
func (h *ReaderHandler) ProxyCSS(c *gin.Context) {
	cssURL := c.Query("url")
	if cssURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url parameter"})
		return
	}

	referer := c.Query("ref")
	client := h.service.BuildFetchHTTPClient()

	resp, err := services.FetchCSS(cssURL, referer, client)
	if err != nil {
		slog.Error("css proxy fetch failed", "url", cssURL, "ref", referer, "error", err)
		c.String(http.StatusBadGateway, "css proxy error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		slog.Error("css proxy upstream non-200", "url", cssURL, "ref", referer, "status", resp.StatusCode, "body", string(errBody))
		c.String(http.StatusBadGateway, "css proxy: upstream returned HTTP %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 限制
	if err != nil {
		c.String(http.StatusBadGateway, "css proxy: read error: "+err.Error())
		return
	}

	cssContent := string(body)

	// 重写 CSS 中的 url() 引用为图片代理地址
	// 使用后端配置的 baseURL 构建 image-proxy 基址，避免使用客户端可控的 c.Request.Host
	imageProxyBase := h.baseURL + "/api/image-proxy"
	cssContent = rewriteCSSUrls(cssContent, cssURL, referer, imageProxyBase)

	c.Header("Content-Type", "text/css; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Access-Control-Allow-Origin", "*")
	c.String(http.StatusOK, cssContent)
}

// ProxyFavicon 代理第三方站点图标服务，返回指定域名对应的 favicon。
// 设计要点：
//   - 仅访问后台配置的可信图标服务（FAVICON_SERVICE_BASE，默认 api.iowen.cn），
//     不接收任意 URL，用户只能提供 domain 作为路径片段，杜绝 SSRF。
//   - 经后端代理可避免把订阅域名直接泄露给第三方，且国内网络可达。
//   - 只读透传，已在全局速率限制 + 类型白名单 + 尺寸上限保护下运行，
//     故注册于非敏感路由组（<img> 无法携带 Authorization 头）。
//
// 参数：
//   - domain: 站点域名（如 example.com），将拼接到图标服务基址之后
func (h *ReaderHandler) ProxyFavicon(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing domain parameter"})
		return
	}
	// 仅允许 hostname 合法字符，拒绝任何可构造出 scheme/路径/查询的注入
	if !faviconDomainRe.MatchString(domain) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
		return
	}

	upstream := resolveFaviconBase() + "/" + domain + ".png"
	client := h.service.BuildFetchHTTPClient()
	resp, err := client.Get(upstream)
	if err != nil {
		slog.Warn("favicon proxy upstream error", "domain", domain, "error", err)
		c.String(http.StatusBadGateway, "favicon proxy error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.String(http.StatusBadGateway, "favicon proxy: upstream returned HTTP %d", resp.StatusCode)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if !isAllowedImageContentType(contentType) {
		slog.Warn("favicon proxy rejected content-type", "domain", domain, "contentType", contentType)
		c.String(http.StatusUnsupportedMediaType, "favicon proxy: unsupported content type")
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 限制
	if err != nil {
		c.String(http.StatusBadGateway, "favicon proxy: read error: "+err.Error())
		return
	}
	if len(body) == 0 {
		c.String(http.StatusBadGateway, "favicon proxy: empty response")
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Data(http.StatusOK, contentType, body)
}

// faviconDomainRe 限制 domain 仅含 hostname 合法字符，防止路径/协议注入
var faviconDomainRe = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// resolveFaviconBase 返回图标服务基址，优先环境变量 FAVICON_SERVICE_BASE，默认国内可达的 api.iowen.cn
func resolveFaviconBase() string {
	if v := os.Getenv("FAVICON_SERVICE_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.iowen.cn/favicon"
}

// rewriteCSSUrls 重写 CSS 内容中的 url() 引用为图片代理地址。
// 使用 CSS 文件本身的 URL 作为 base 解析相对路径，确保 CSS 中的背景图、字体等资源通过代理加载。
// referer 用于设置图片代理的 Referer 头，绕过 CDN 防盗链。
func rewriteCSSUrls(cssContent string, cssURL string, referer string, imageProxyBase string) string {
	if imageProxyBase == "" {
		return cssContent
	}

	urlRe := urlRePre
	return urlRe.ReplaceAllStringFunc(cssContent, func(match string) string {
		submatch := urlRe.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		src := submatch[1]
		// 跳过 data:、blob: URI 和已经在代理中的 URL
		if src == "" || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "blob:") || strings.Contains(src, "/image-proxy") || strings.Contains(src, "/css-proxy") {
			return match
		}
		// 解析为绝对 URL：使用 CSS 文件本身的 URL 作为 base
		var absSrc string
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			absSrc = src
		} else if strings.HasPrefix(src, "//") {
			absSrc = "https:" + src
		} else {
			absSrc = resolveURLStr(src, cssURL)
		}
		if absSrc == "" {
			return match
		}
		proxyURL := fmt.Sprintf("%s?url=%s&ref=%s", imageProxyBase, url.QueryEscape(absSrc), url.QueryEscape(referer))
		return fmt.Sprintf(`url("%s")`, proxyURL)
	})
}

// resolveURLStr 将可能为相对路径的 URL 解析为绝对 URL（基于 string base URL）。
// 如果 href 已经是绝对 URL 则直接返回；否则基于 baseURLStr 解析。
func resolveURLStr(href string, baseURLStr string) string {
	hrefURL, err := url.Parse(href)
	if err != nil {
		return href
	}
	if hrefURL.IsAbs() {
		return href
	}
	baseParsed, err := url.Parse(baseURLStr)
	if err != nil {
		return href
	}
	resolved := baseParsed.ResolveReference(hrefURL)
	return resolved.String()
}

// stripCSPMetaTags 移除 HTML 中的 Content-Security-Policy meta 标签。
// 原文页面可能包含 frame-ancestors 限制，阻止在 iframe 中渲染。
// 作为代理，安全由我们自己的 CSP 控制，不需要原文的 CSP 限制。
// 使用一个综合正则匹配所有常见的 CSP meta 标签变体：
//   - 双引号/单引号/无引号的 http-equiv 属性值
//   - http-equiv 在 content 之前或之后
//   - 跨行属性
//   - 自闭合标签
func stripCSPMetaTags(htmlContent string) string {
	re := cspMetaRePre
	return re.ReplaceAllString(htmlContent, "")
}

// rewriteCSSUrlsInContent 重写内联 CSS 内容中的 url() 引用为图片代理地址。
// 使用 CSS 文件本身的 URL 作为 base 解析相对路径，确保 CSS 中的背景图、字体等资源通过代理加载。
// referer 用于设置图片代理的 Referer 头，绕过 CDN 防盗链。
// 不同于 rewriteCSSUrls（用于 CSS 代理端点返回的 CSS 内容），此函数用于内联 <style> 中的 CSS。
func rewriteCSSUrlsInContent(cssContent string, cssURL string, referer string, imageProxyBase string) string {
	if imageProxyBase == "" {
		return cssContent
	}

	urlRe := urlRePre
	return urlRe.ReplaceAllStringFunc(cssContent, func(match string) string {
		submatch := urlRe.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		src := submatch[1]
		if src == "" || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "blob:") || strings.Contains(src, "/image-proxy") || strings.Contains(src, "/css-proxy") {
			return match
		}
		// 解析为绝对 URL：使用 CSS 文件本身的 URL 作为 base
		var absSrc string
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			absSrc = src
		} else if strings.HasPrefix(src, "//") {
			absSrc = "https:" + src
		} else {
			absSrc = resolveURLStr(src, cssURL)
		}
		if absSrc == "" {
			return match
		}
		proxyURL := fmt.Sprintf("%s?url=%s&ref=%s", imageProxyBase, url.QueryEscape(absSrc), url.QueryEscape(referer))
		return fmt.Sprintf(`url("%s")`, proxyURL)
	})
}

// proxyErrorPage 返回 iframe 内可展示的错误页
func proxyErrorPage(message, originalURL string) string {
	safeMessage := html.EscapeString(message)
	safeURL := html.EscapeString(originalURL)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>原文加载失败</title>
<style>
:root{--bg:#faf8f5;--text:#5a4a3a;--muted:#8a7a6a;--primary:#b86a38;}
@media (prefers-color-scheme: dark){:root{--bg:#2a2622;--text:#e8e0d8;--muted:#a09890;--primary:#d88a58;}}
body{margin:0;padding:40px 24px;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;background:var(--bg);color:var(--text);text-align:center;}
.icon{width:48px;height:48px;margin:0 auto 16px;color:var(--primary);}
h1{font-size:18px;font-weight:600;margin:0 0 8px;}
p{font-size:14px;margin:0 0 24px;color:var(--muted);}
a{color:var(--primary);text-decoration:none;font-weight:500;}
a:hover{text-decoration:underline;}
.url{display:block;max-width:100%%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);font-size:12px;margin-top:8px;}
</style>
</head>
<body>
<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>
<h1>无法在当前窗口加载原文</h1>
<p>%s</p>
<p><a href="%s" target="_blank" rel="noopener noreferrer">在新标签页打开原文</a></p>
<p class="url">%s</p>
</body>
</html>`, safeMessage, safeURL, safeURL)
}

// GetDatabaseInfo 获取数据库文件信息
func (h *ReaderHandler) GetDatabaseInfo(c *gin.Context) {
	info, err := h.service.GetDatabaseInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, info)
}

// GetCacheStats 获取缓存统计
func (h *ReaderHandler) GetCacheStats(c *gin.Context) {
	stats, err := h.service.GetCacheStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// ClearReadabilityCache 清空阅读模式缓存
func (h *ReaderHandler) ClearReadabilityCache(c *gin.Context) {
	count, err := h.service.ClearReadabilityCache()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": count})
}

// VacuumDatabase 压缩数据库（VACUUM）
func (h *ReaderHandler) VacuumDatabase(c *gin.Context) {
	if err := h.service.Vacuum(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListBackups 列出备份文件
func (h *ReaderHandler) ListBackups(c *gin.Context) {
	backups, err := h.service.ListBackups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	// 确保即使为 nil 也返回空数组
	if backups == nil {
		backups = []services.BackupEntry{}
	}
	c.JSON(http.StatusOK, backups)
}

// CreateBackup 创建压缩备份
func (h *ReaderHandler) CreateBackup(c *gin.Context) {
	name, err := h.service.CreateCompressedBackup()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name})
}

// DeleteBackup 删除指定备份
func (h *ReaderHandler) DeleteBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.DeleteBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CleanupBackups 按策略清理过期备份
func (h *ReaderHandler) CleanupBackups(c *gin.Context) {
	var req struct {
		MaxKeep int `json:"maxKeep"`
		MaxDays int `json:"maxDays"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	if req.MaxKeep <= 0 {
		req.MaxKeep = 10
	}
	if req.MaxDays <= 0 {
		req.MaxDays = 30
	}
	deleted, err := h.service.CleanupBackups(req.MaxKeep, req.MaxDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// RestoreBackup 从指定压缩备份恢复数据库（M-R2：备份不再是只写，可一键回放）
func (h *ReaderHandler) RestoreBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.RestoreBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DownloadBackup 下载指定备份 Zip 文件
func (h *ReaderHandler) DownloadBackup(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return
	}
	fullPath := filepath.Join(h.service.BackupDirPath(), name)
	if _, err := os.Stat(fullPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
	c.File(fullPath)
}

// GetBackupContents 获取备份文件内容清单
func (h *ReaderHandler) GetBackupContents(c *gin.Context) {
	name := c.Param("name")
	contents, err := h.service.GetBackupContents(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, contents)
}

// ImportBackupAndValidate 导入外部备份文件并返回内容清单（供前端选择恢复粒度）
// 接收 multipart/form-data，包含一个 "backup" 文件字段
func (h *ReaderHandler) ImportBackupAndValidate(c *gin.Context) {
	// 从 multipart form 获取上传的 backup.zip 文件
	file, err := c.FormFile("backup")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get backup file"})
		return
	}

	// 保存到临时文件
	tmpPath, err := h.saveTempUpload(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save backup file"})
		return
	}
	defer os.Remove(tmpPath)

	// 读取备份内容清单
	contents, err := h.service.GetBackupContentsFromPath(tmpPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup file: " + err.Error()})
		return
	}

	// 返回清单信息
	c.JSON(http.StatusOK, gin.H{
		"name":    file.Filename,
		"hasDb":   contents.HasDB,
		"hasCfg":  contents.HasCfg,
		"hasOpml": contents.HasOpm, // 修复：正确使用 HasOpm 字段名
	})
}

// saveTempUpload 保存上传文件到临时路径（限制 256MB）
func (h *ReaderHandler) saveTempUpload(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	tmpFile, err := os.CreateTemp("", "flore-restore-import-*.zip")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.Remove(tmpFile.Name())
		}
	}()

	_, err = io.Copy(tmpFile, io.LimitReader(src, 256<<20))
	if err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

// RestoreConfigFromBackup 从备份中仅恢复配置项
func (h *ReaderHandler) RestoreConfigFromBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.RestoreConfigFromBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RestoreOPMLFromBackup 从备份中仅恢复订阅源
func (h *ReaderHandler) RestoreOPMLFromBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.RestoreOPMLFromBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetSettings 获取所有设置项
func (h *ReaderHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// GetSetting 获取单个设置项
func (h *ReaderHandler) GetSetting(c *gin.Context) {
	key := c.Param("key")
	value := h.service.GetSetting(key)
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}

// UpdateSettings 批量更新设置项
// 接收 JSON 对象：{ "key1": "value1", "key2": "value2", ... }
// 值统一以字符串存储，前端负责类型转换
func (h *ReaderHandler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	// 限制 key 数量和 value 长度，防止滥用存储或 DoS
	const maxSettingsKeys = 100
	const maxValueLen = 10 * 1024 // 10KB
	if len(req) > maxSettingsKeys {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("too many keys, max %d", maxSettingsKeys)})
		return
	}
	for k, v := range req {
		if len(k) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key too long"})
			return
		}
		if len(v) > maxValueLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("value too long for key %s, max %d bytes", k, maxValueLen)})
			return
		}
	}
	if err := h.service.UpdateSettings(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CleanupArticles 按留存策略清理已读文章
func (h *ReaderHandler) CleanupArticles(c *gin.Context) {
	var req struct {
		RetentionDays    int  `json:"retentionDays"`
		RetentionMax     int  `json:"retentionMax"`
		ExcludeStarred   bool `json:"excludeStarred"`
		ExcludeReadLater bool `json:"excludeReadLater"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	deleted, err := h.service.CleanupArticles(req.RetentionDays, req.RetentionMax, req.ExcludeStarred, req.ExcludeReadLater)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// GetVersion 返回应用版本信息
// 版本来源优先级：编译时 -ldflags 注入 > 环境变量 FLORE_VERSION > 默认值 "dev"
// -ldflags 示例：-X github.com/rss/go-server/internal/handlers.appVersion=x.y.z
var appVersion = resolveAppVersion()

// resolveAppVersion 确定版本号：优先环境变量，其次返回 dev 占位符（生产构建由 ldflags 注入真实版本）
func resolveAppVersion() string {
	if v := os.Getenv("FLORE_VERSION"); v != "" {
		return v
	}
	return "dev"
}

func (h *ReaderHandler) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": appVersion,
	})
}
