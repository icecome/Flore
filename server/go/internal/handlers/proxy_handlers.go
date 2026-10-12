package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rss/go-server/internal/services"
)

// frameCheckCacheMax frame 预检缓存最大条目数，超出后按 FIFO 淘汰最旧条目
const frameCheckCacheMax = 1000

// ProxyOriginal 原文最小代理：抓取原文并以可嵌入 iframe 的形式返回。
// HTML 内容处理（剥离 CSP meta、注入 base 标签）由 services.InjectProxyHead 完成。
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
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(services.ProxyErrorPage(fmt.Sprintf("加载失败：%s", err.Error()), originalURL)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(services.ProxyErrorPage(fmt.Sprintf("原文返回 HTTP %d", resp.StatusCode), originalURL)))
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

	// 读取响应体，限制大小防止 OOM
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(services.ProxyErrorPage(fmt.Sprintf("读取原文失败：%s", err.Error()), originalURL)))
		return
	}

	// 如果 HTML，剥离 CSP meta 并注入 base 标签、viewport 与 iframe 适配样式
	if strings.Contains(contentType, "text/html") {
		c.Data(resp.StatusCode, contentType, []byte(services.InjectProxyHead(string(body), originalURL)))
		return
	}

	c.Data(resp.StatusCode, contentType, body)
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
	frameable, finalURL := h.frameChecker.Check(h.service.BuildFetchHTTPClient(), originalURL)
	c.JSON(http.StatusOK, gin.H{"frameable": frameable, "url": finalURL})
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

	resp, err := services.FetchImage(imageURL, client)
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
	setProxyCORS(c)

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

	// 重写 CSS 中的 url() 引用为图片代理地址
	// 使用后端配置的 baseURL 构建 image-proxy 基址，避免使用客户端可控的 c.Request.Host
	imageProxyBase := h.baseURL + "/api/image-proxy"
	cssContent := services.RewriteCSSUrls(string(body), cssURL, referer, imageProxyBase)

	c.Header("Content-Type", "text/css; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=3600")
	setProxyCORS(c)
	c.String(http.StatusOK, cssContent)
}

// ProxyFavicon 代理第三方站点图标服务，返回指定域名对应的 favicon。
// 设计要点：
//   - 仅访问后台配置的可信图标服务（FAVICON_SERVICE_BASE，默认 favicon.yandex.net），
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

	upstream := resolveFaviconBase() + "/" + domain
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
	setProxyCORS(c)
	c.Data(http.StatusOK, contentType, body)
}

// faviconDomainRe 限制 domain 仅含 hostname 合法字符，防止路径/协议注入
var faviconDomainRe = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// resolveFaviconBase 返回图标服务基址，优先环境变量 FAVICON_SERVICE_BASE，默认国内可达的 favicon.yandex.net
func resolveFaviconBase() string {
	if v := os.Getenv("FAVICON_SERVICE_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://favicon.yandex.net/favicon"
}

// ProxyFaviconDirect 从订阅源站直接抓取 favicon.ico，经后端代理返回。
// 作为 Yandex 服务的备选，适用于第三方服务不可达或无法覆盖的站点。
// 已在全局速率限制 + 类型白名单保护下运行。
func (h *ReaderHandler) ProxyFaviconDirect(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing domain parameter"})
		return
	}
	if !faviconDomainRe.MatchString(domain) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
		return
	}

	// 直接访问源站 favicon.ico，Golang 客户端自动跟随重定向（最多 10 次）
	upstream := "https://" + domain + "/favicon.ico"
	client := h.service.BuildFetchHTTPClient()
	resp, err := client.Get(upstream)
	if err != nil {
		slog.Warn("favicon direct upstream error", "domain", domain, "error", err)
		c.String(http.StatusBadGateway, "favicon direct error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.String(http.StatusBadGateway, "favicon direct: upstream returned HTTP %d", resp.StatusCode)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	// 允许 favicon 常见的 image/png 和 image/x-icon
	if contentType != "image/png" && contentType != "image/x-icon" && contentType != "" {
		slog.Warn("favicon direct rejected content-type", "domain", domain, "contentType", contentType)
		c.String(http.StatusUnsupportedMediaType, "favicon direct: unsupported content type")
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadGateway, "favicon direct: read error: "+err.Error())
		return
	}
	if len(body) == 0 {
		c.String(http.StatusBadGateway, "favicon direct: empty response")
		return
	}

	if contentType == "" {
		// 部分源站不返回 Content-Type，通过魔数检测
		if bytes.HasPrefix(body, []byte("\x89PNG")) || bytes.HasPrefix(body, []byte("GIF")) {
			contentType = "image/png"
		} else if len(body) >= 2 && body[0] == 0 && body[1] == 0 {
			contentType = "image/x-icon"
		} else {
			contentType = "image/png"
		}
	}

	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	setProxyCORS(c)
	c.Data(http.StatusOK, contentType, body)
}
