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

// DefaultPort 后端默认监听端口（PORT 环境变量缺失时使用），backend 组装层与本包共用
const DefaultPort = "3002"

// ReaderHandler 阅读器 HTTP 处理器
type ReaderHandler struct {
	service     *services.ReaderService
	rateLimiter *rateLimiter
	// baseURL 是后端服务的可信基础地址（scheme://host:port），
	// 用于代理端点重写 URL，避免使用客户端可控的 Host 头
	baseURL string
	// frameChecker 缓存原文是否可被 iframe 直接嵌入（基于 X-Frame-Options / CSP frame-ancestors 预检），
	// 避免重复文章重复网络预检。有容量上限，超出按 FIFO 淘汰，防止长期运行内存膨胀。
	frameChecker *services.FrameChecker
}

// NewReaderHandler 创建处理器
func NewReaderHandler() *ReaderHandler {
	return &ReaderHandler{
		service:      services.NewReaderService(),
		baseURL:      resolveBackendBaseURL(),
		frameChecker: services.NewFrameChecker(frameCheckCacheMax),
	}
}

// resolveBackendBaseURL 从环境变量构建可信的后端基础地址；
// PORT=0（动态端口）时回读 FLORE_PORT_FILE 得到实际端口。
func resolveBackendBaseURL() string {
	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort
	}
	// PORT=0（动态端口）时，实际端口由系统分配并写入 FLORE_PORT_FILE（见 backend.Start 的 listener 绑定段）。
	// 此时回读文件以得到正确端口，避免 baseURL 固化为 "http://127.0.0.1:0" 导致代理 URL 失效（M-P1）。
	if port == "0" {
		if portFile := os.Getenv("FLORE_PORT_FILE"); portFile != "" {
			if raw, err := os.ReadFile(portFile); err == nil {
				if p := strings.TrimSpace(string(raw)); p != "" {
					port = p
				}
			}
		}
	}
	return "http://" + bindAddr + ":" + port
}

// AuthMiddleware 返回一个 Bearer Token 认证中间件；token 为空时返回 nil
func AuthMiddleware(token string) gin.HandlerFunc {
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

// RegisterRoutes 注册路由，apiToken 非空时对敏感路由启用认证。
//
// 鉴权原则（按原则添加新路由，而非按先例）：
//   - 凡返回用户数据（订阅源、文章、计数、搜索、缓存统计、DB 信息、备份列表、
//     设置、同步配置、诊断项清单）或变更用户数据的接口，一律归入 sensitive 组，
//     apiToken 非空时必须携带 Bearer Token。
//   - 仅「无法携带 Authorization 头、需经 <img>/<link> 消费」的只读代理族
//     （image/css/favicon 代理）与仅做存活探活、不暴露数据的 /health 保持开放。
//   - 数据写入（create/delete/export/cleanup/vacuum/restore）本就已在 sensitive 组；
//     此处把读接口一并收紧，闭合「同机进程免 token 读取全量历史」的缺口（P1-013/P1-014）。
//     桌面壳自身请求经 ApiRequest / doRequest 已统一携带 token，不受影响。
func (h *ReaderHandler) RegisterRoutes(r *gin.Engine, apiToken string) {
	// 速率限制：每 IP 每 60 秒最多 120 次请求
	h.rateLimiter = newRateLimiter(120, 60*time.Second)
	api := r.Group("/api", rateLimitMiddleware(h.rateLimiter))

	// 敏感路由组（返回或变更用户数据的接口），需要 Bearer Token 认证。
	// 仅当 apiToken 非空时启用 AuthMiddleware；空 token 时退回开放（Web 本地无 token 部署）。
	auth := AuthMiddleware(apiToken)
	sensitive := api
	if auth != nil {
		sensitive = api.Group("", auth)
	}
	{
		// 订阅源（读返回订阅列表，写/抓取为变更操作，全部入敏感组）
		sensitive.GET("/sources", h.GetSources)
		sensitive.GET("/sources/:id", h.GetSource)
		sensitive.PUT("/sources/:id", h.UpdateSource)
		sensitive.DELETE("/sources/:id", h.DeleteSource)
		sensitive.DELETE("/sources/delete-batch", h.DeleteSourcesBatch)
		sensitive.POST("/sources/:id/fetch", h.FetchSource)
		sensitive.POST("/sources/fetch-all", h.FetchAll)
		sensitive.GET("/sources/fetch-status", h.FetchStatus)
		sensitive.POST("/sources/:id/read-all", h.MarkSourceAllRead)
		sensitive.POST("/sources/create", h.CreateSource)

		// 文件夹/分类（返回/变更订阅结构，全部入敏感组）
		sensitive.GET("/folders", h.GetFolders)
		sensitive.POST("/folders", h.CreateFolder)
		sensitive.PUT("/folders/:id", h.UpdateFolder)
		sensitive.DELETE("/folders/:id", h.DeleteFolder)
		sensitive.POST("/folders/:id/clear", h.ClearFolderSources)
		sensitive.POST("/folders/:id/fetch", h.FetchFolder)

		// OPML（导入为批量写入、导出为全量订阅，均入敏感组）
		sensitive.POST("/opml/import", h.ImportOPML)
		sensitive.GET("/opml/export", h.ExportOPML)

		// 文章（读返回阅读历史、写为状态变更，全部入敏感组）
		sensitive.GET("/items", h.GetItems)
		sensitive.GET("/items/count", h.GetItemsCount)
		sensitive.GET("/items/:id", h.GetItem)
		sensitive.POST("/items/:id/read", h.MarkRead)
		sensitive.POST("/items/:id/unread", h.MarkUnread)
		sensitive.POST("/items/:id/star", h.MarkStar)
		sensitive.POST("/items/:id/unstar", h.MarkUnstar)
		sensitive.POST("/items/:id/read-later", h.MarkReadLater)
		sensitive.POST("/items/:id/unread-later", h.MarkUnreadLater)
		sensitive.POST("/items/read-later/clear", h.ClearReadLater)
		sensitive.POST("/items/read-all", h.MarkItemsRead)
		sensitive.POST("/items/mark-read", h.MarkReadBatchItems)
		sensitive.GET("/items/search", h.SearchItems)
		sensitive.POST("/items/:id/index-search", h.IndexItemForSearch)
		sensitive.POST("/items/index-search/rebuild", h.RebuildSearchIndex)
		sensitive.GET("/items/:id/readability", h.GetReadability)
		sensitive.POST("/items/export", h.ExportItems)
		sensitive.GET("/items/:id/export.md", h.ExportItemMarkdown)
		sensitive.POST("/items/:id/apply-filters", h.ApplyFilterRules)

		// 过滤规则（返回/变更规则，全部入敏感组）
		sensitive.GET("/filter-rules", h.ListFilterRules)
		sensitive.POST("/filter-rules", h.CreateFilterRule)
		sensitive.PUT("/filter-rules/:id", h.UpdateFilterRule)
		sensitive.DELETE("/filter-rules/:id", h.DeleteFilterRule)
		sensitive.POST("/filter-rules/:id/test", h.TestFilterRule)

		// 服务端同步（config 含脱敏密钥，读写均入敏感组从严）
		sensitive.GET("/sync/config", h.GetSyncConfig)
		sensitive.PUT("/sync/config", h.UpdateSyncConfig)
		sensitive.POST("/sync/test", h.TestSyncConnection)
		sensitive.POST("/sync/test-file", h.TestFileSyncConnection)
		sensitive.POST("/sync/trigger", h.TriggerSync)
		sensitive.GET("/sync/status", h.GetSyncStatus)
		sensitive.POST("/sync/align", h.AlignSubscriptions)
		// WebDAV 备份（文件后端）：列出/上传/下载整包备份到用户自有 WebDAV 存储
		sensitive.GET("/sync/file-backups", h.ListFileBackups)
		sensitive.POST("/sync/file-backups/upload", h.UploadBackupToWebDAV)
		sensitive.POST("/sync/file-backups/create", h.CreateBackupToWebDAV)
		sensitive.GET("/sync/file-backups/download", h.DownloadBackupFromWebDAV)

		// 原文代理
		// 预检原文是否可被 iframe 直接嵌入（检查 X-Frame-Options / CSP frame-ancestors），供前端决定直连还是走代理
		sensitive.GET("/proxy/check/:id", h.CheckFrameable)
		sensitive.GET("/proxy/:id", h.ProxyOriginal)
		// 图片/CSS/图标代理为只读透传，已被全局速率限制 + 类型白名单 + 尺寸上限保护，
		// 且需经 <img>/<link> 消费（无法携带 Authorization 头），故置于非敏感组。
		api.GET("/image-proxy", h.ProxyImage)
		api.GET("/css-proxy", h.ProxyCSS)
		api.GET("/favicon-proxy", h.ProxyFavicon)
		api.GET("/favicon-direct", h.ProxyFaviconDirect)

		// 统计（返回未读数，属用户数据，入敏感组）
		sensitive.GET("/stats/unread", h.GetUnreadCount)

		// 数据库备份/恢复（敏感：可替换整个数据库）
		sensitive.GET("/database/export", h.ExportDatabase)
		sensitive.POST("/database/restore", h.ImportDatabase)

		// 数据管理（info 暴露 DB 绝对路径、cache/stats 暴露内部结构，均入敏感组）
		sensitive.GET("/database/info", h.GetDatabaseInfo)
		sensitive.GET("/cache/stats", h.GetCacheStats)
		sensitive.POST("/cache/clear-readability", h.ClearReadabilityCache)
		sensitive.POST("/cache/rebuild-search", h.RebuildSearchIndex)
		sensitive.POST("/database/vacuum", h.VacuumDatabase)

		// 备份管理（全部纳入敏感组：备份含全量数据库，下载/恢复/导入/清理均可被
		// 同机进程滥用，与上方 database/export、database/restore 的防护口径一致）
		sensitive.GET("/backups", h.ListBackups)
		sensitive.POST("/backups/create", h.CreateBackup)
		sensitive.DELETE("/backups/:name", h.DeleteBackup)
		sensitive.POST("/backups/cleanup", h.CleanupBackups)
		sensitive.POST("/backups/:name/restore", h.RestoreBackup)
		sensitive.GET("/backups/:name/download", h.DownloadBackup)
		sensitive.GET("/backups/:name/contents", h.GetBackupContents)
		sensitive.POST("/backups/:name/restore-config", h.RestoreConfigFromBackup)
		sensitive.POST("/backups/:name/restore-opml", h.RestoreOPMLFromBackup)
		sensitive.POST("/backups/restore/import", h.ImportBackupAndValidate)

		// 设置（Settings 表，键值对存储；返回全量键值含 sync_api_password、proxyUrl
		// 等敏感明文，单 key 读取同样可探查，全部纳入 sensitive 组）
		sensitive.GET("/settings", h.GetSettings)
		sensitive.PUT("/settings", h.UpdateSettings)
		sensitive.GET("/settings/:key", h.GetSetting)

		// 文章留存清理（批量删除已读文章，入敏感组）
		sensitive.POST("/articles/cleanup", h.CleanupArticles)

		// 诊断包（items 清单返回内部数据；generate/download 为敏感写/读，全部入敏感组）
		sensitive.GET("/diagnostic/items", h.GetDiagnoseItems)
		sensitive.POST("/diagnostic/generate", h.GenerateDiagnosticPackage)
		sensitive.GET("/diagnostic/:path/download", h.ExtractDiagnosticPackage)

		// 版本信息（不含用户数据，保持开放）
		api.GET("/version", h.GetVersion)
	}

	// 健康检查（仅存活探活，不暴露用户数据，保持开放）
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
// 对于已知业务错误（如 "invalid"、"not found"、"missing"、"not enabled"、"empty"、
// 认证失败、探活失败等后端主动生成的配置/认证文案）直接返回；
// 对于未知错误返回通用提示，避免泄露内部路径和实现细节。
func safeError(detailed string) string {
	slog.Error(detailed)
	lower := strings.ToLower(detailed)
	for _, safe := range []string{
		"invalid", "not found", "missing", "already exists", "no allowed fields",
		"not enabled", "not configured", "is empty", "authentication",
		"probe failed", "unreachable", "not supported", "blocked connection",
	} {
		if strings.Contains(lower, safe) {
			return detailed
		}
	}
	return "internal_server_error"
}

// GetSources 获取订阅源列表
