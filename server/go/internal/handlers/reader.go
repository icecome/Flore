package handlers

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rss/go-server/internal/services"
)

// 预编译正则，避免每次函数调用重复编译
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
	// frameCheckCache 缓存原文是否可被 iframe 直接嵌入（基于 X-Frame-Options / CSP frame-ancestors 预检），
	// 避免重复文章重复网络预检。有容量上限（frameCheckCacheMax），超出按 FIFO 淘汰，防止长期运行内存膨胀。
	frameCheckCacheMu sync.Mutex
	frameCheckCache   map[string]frameCheckResult
	frameCheckOrder   []string
}

// NewReaderHandler 创建处理器
func NewReaderHandler() *ReaderHandler {
	return &ReaderHandler{
		service:         services.NewReaderService(),
		baseURL:         resolveBackendBaseURL(),
		frameCheckCache: make(map[string]frameCheckResult),
	}
}

// frameCheckCacheMax frameCheckCache 最大条目数，超出后按 FIFO 淘汰最旧条目。
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
		api.GET("/favicon-direct", h.ProxyFaviconDirect)

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
