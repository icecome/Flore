package handlers

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rss/go-server/internal/services"
)

var (
	linkRePre    = regexp.MustCompile(`(?is)<link\s[^>]*rel\s*=\s*["']?stylesheet["']?[^>]*>`)
	hrefRePre    = regexp.MustCompile(`href\s*=\s*["']([^"']+)["']`)
	cspMetaRePre = regexp.MustCompile(`(?is)<meta\s[^>]*http-equiv\s*=\s*["']?Content-Security-Policy["']?[^>]*>`)
	urlRePre     = regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)
)

// rateLimiter 简单的基于 IP 的速率限制中间件
const frameCheckCacheMax = 1000

// frameCacheLoad 读取 frame 预检缓存。
func (h *ReaderHandler) frameCacheLoad(url string) (frameCheckResult, bool) {
	h.frameCheckCacheMu.Lock()
	defer h.frameCheckCacheMu.Unlock()
	r, ok := h.frameCheckCache[url]
	return r, ok
}

// frameCacheStore 写入 frame 预检缓存；超过上限时淘汰最旧条目。
func (h *ReaderHandler) frameCacheStore(url string, r frameCheckResult) {
	h.frameCheckCacheMu.Lock()
	defer h.frameCheckCacheMu.Unlock()
	if _, exists := h.frameCheckCache[url]; exists {
		h.frameCheckCache[url] = r
		return
	}
	if len(h.frameCheckCache) >= frameCheckCacheMax {
		oldest := h.frameCheckOrder[0]
		h.frameCheckOrder = h.frameCheckOrder[1:]
		delete(h.frameCheckCache, oldest)
	}
	h.frameCheckCache[url] = r
	h.frameCheckOrder = append(h.frameCheckOrder, url)
}

// resolveBackendBaseURL 从环境变量构建可信的后端基础地址
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
	if r, ok := h.frameCacheLoad(originalURL); ok {
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
	h.frameCacheStore(originalURL, frameCheckResult{frameable: frameable, finalURL: finalURL})
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

	cssContent := string(body)

	// 重写 CSS 中的 url() 引用为图片代理地址
	// 使用后端配置的 baseURL 构建 image-proxy 基址，避免使用客户端可控的 c.Request.Host
	imageProxyBase := h.baseURL + "/api/image-proxy"
	cssContent = rewriteCSSUrls(cssContent, cssURL, referer, imageProxyBase)

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
