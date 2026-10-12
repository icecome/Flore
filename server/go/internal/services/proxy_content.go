package services

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// proxy 正则集：CSP meta 剥离与 CSS url() 重写
var (
	cspMetaRe = regexp.MustCompile(`(?is)<meta\s[^>]*http-equiv\s*=\s*["']?Content-Security-Policy["']?[^>]*>`)
	urlRe     = regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)
)

// FrameCheckResult 缓存原文嵌入预检结果
type FrameCheckResult struct {
	Frameable bool
	FinalURL  string
}

// FrameChecker 原文 iframe 嵌入预检器：按 URL 缓存 X-Frame-Options /
// CSP frame-ancestors 判定结果，容量上限后按 FIFO 淘汰最旧条目。
type FrameChecker struct {
	mu    sync.Mutex
	cache map[string]FrameCheckResult
	order []string
	max   int
}

// NewFrameChecker 创建预检器，maxEntries 为缓存容量上限
func NewFrameChecker(maxEntries int) *FrameChecker {
	return &FrameChecker{
		cache: make(map[string]FrameCheckResult),
		max:   maxEntries,
	}
}

// load 读取预检缓存
func (fc *FrameChecker) load(targetURL string) (FrameCheckResult, bool) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	r, ok := fc.cache[targetURL]
	return r, ok
}

// store 写入预检缓存；超过上限时淘汰最旧条目
func (fc *FrameChecker) store(targetURL string, r FrameCheckResult) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if _, exists := fc.cache[targetURL]; exists {
		fc.cache[targetURL] = r
		return
	}
	if len(fc.cache) >= fc.max {
		oldest := fc.order[0]
		fc.order = fc.order[1:]
		delete(fc.cache, oldest)
	}
	fc.cache[targetURL] = r
	fc.order = append(fc.order, targetURL)
}

// Check 发起一次轻量请求，检查原文是否被禁止 iframe 嵌入。
// 仅读取响应头即可判定，不下载正文；预检失败（网络不可达等）保守返回不可嵌入，
// 由前端改走 /proxy/:id 最小代理。返回 finalURL 为跟随重定向后的最终地址。
func (fc *FrameChecker) Check(client *http.Client, originalURL string) (bool, string) {
	if r, ok := fc.load(originalURL); ok {
		return r.Frameable, r.FinalURL
	}

	req, err := http.NewRequest(http.MethodGet, originalURL, nil)
	if err != nil {
		return false, originalURL
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FloreReader/1.0)")

	resp, err := client.Do(req)
	if err != nil {
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
	fc.store(originalURL, FrameCheckResult{Frameable: frameable, FinalURL: finalURL})
	return frameable, finalURL
}

// StripCSPMetaTags 移除 HTML 中的 Content-Security-Policy meta 标签。
// 原文页面可能包含 frame-ancestors 限制，阻止在 iframe 中渲染；
// 作为代理，安全由我们自己的 CSP 控制。
func StripCSPMetaTags(htmlContent string) string {
	return cspMetaRe.ReplaceAllString(htmlContent, "")
}

// InjectProxyHead 对代理返回的 HTML 做最小内容处理：
// 剥离 CSP meta 标签 + 注入 base 标签、viewport 与 iframe 适配样式。
// 不内联 CSS、不重写任何 URL（含 <style> 与 <img>），CSS/JS/图片由原站直连加载，
// 保证样式与交互完整（与 Folo / Fluent Reader 真实浏览器内核直连原站的做法一致）。
func InjectProxyHead(htmlContent string, originalURL string) string {
	htmlContent = StripCSPMetaTags(htmlContent)

	baseTag := fmt.Sprintf(`<base href="%s" target="_blank">`, html.EscapeString(originalURL))
	viewportMeta := `<meta name="viewport" content="width=device-width, initial-scale=1.0">`
	iframeCSS := `<style>html,body{margin:0;padding:0;max-width:100%;box-sizing:border-box;}body{padding:16px;}</style>`

	injectHead := baseTag + viewportMeta + iframeCSS
	if headIdx := strings.Index(strings.ToLower(htmlContent), "<head>"); headIdx >= 0 {
		return htmlContent[:headIdx+6] + injectHead + htmlContent[headIdx+6:]
	}
	if htmlIdx := strings.Index(strings.ToLower(htmlContent), "<html"); htmlIdx >= 0 {
		closeIdx := strings.Index(htmlContent[htmlIdx:], ">") + htmlIdx + 1
		return htmlContent[:closeIdx] + "<head>" + injectHead + "</head>" + htmlContent[closeIdx:]
	}
	return htmlContent
}

// RewriteCSSUrls 重写 CSS 内容中的 url() 引用为图片代理地址。
// 使用 CSS 文件本身的 URL 作为 base 解析相对路径，确保 CSS 中的背景图、字体等
// 资源通过代理加载；referer 用于设置图片代理的 Referer 头绕过 CDN 防盗链。
func RewriteCSSUrls(cssContent string, cssURL string, referer string, imageProxyBase string) string {
	if imageProxyBase == "" {
		return cssContent
	}

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
			absSrc = ResolveURLStr(src, cssURL)
		}
		if absSrc == "" {
			return match
		}
		proxyURL := fmt.Sprintf("%s?url=%s&ref=%s", imageProxyBase, url.QueryEscape(absSrc), url.QueryEscape(referer))
		return fmt.Sprintf(`url("%s")`, proxyURL)
	})
}

// ResolveURLStr 将可能为相对路径的 URL 解析为绝对 URL（基于 string base URL）。
// 如果 href 已经是绝对 URL 则直接返回；否则基于 baseURLStr 解析。
func ResolveURLStr(href string, baseURLStr string) string {
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

// ProxyErrorPage 返回 iframe 内可展示的错误页
func ProxyErrorPage(message, originalURL string) string {
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
