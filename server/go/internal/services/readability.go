package services

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// lazySrcAttrs 常见的懒加载图片自定义属性。Readability 提取后的内容里，
// 部分站点把真实地址放在 data-src / data-original 等属性上，而标准 src 缺失或为空。
// 由于前端 DOMPurify 配置 ALLOW_DATA_ATTR:false 会剥离 data-*，这些图会彻底消失。
// 因此在返回前把懒加载地址归一到标准 src，从根本上消除"无规律丢图"。
var lazySrcAttrs = []string{
	"data-src", "data-original", "data-lazy-src", "data-original-src",
	"data-lazy", "data-load-src", "data-loading-src", "data-true-src", "data-default-src",
}

// normalizeReadabilityContent 对 Readability 提取出的正文 HTML 做归一化：
//  1. 将相对路径的图片/视频/音频地址解析为绝对 URL（基于文章原文 URL），
//     避免前端图片代理因收到相对路径而被 ValidateURLOnly 拒绝。
//  2. 将懒加载属性（data-src 等）归一到标准 src / srcset，避免被 DOMPurify 剥离后丢图。
//  3. 将 <source> / <img> 的 srcset 相对路径解析为绝对 URL。
//
// 这样前端图片代理重写只需处理绝对 URL，且不会因相对路径或 data-* 剥离而丢失图片。
func normalizeReadabilityContent(content string, base *url.URL) string {
	if content == "" || base == nil {
		return content
	}
	// 使用 ParseFragment 避免被 html.Parse 额外包一层 <html><head><body>
	ctx := &html.Node{Data: "body", Type: html.ElementNode}
	nodes, err := html.ParseFragment(strings.NewReader(content), ctx)
	if err != nil || len(nodes) == 0 {
		return content
	}

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				// 懒加载：仅当 src 为空时，把 data-src 等归一到 src
				if attrVal(n, "src") == "" {
					for _, a := range lazySrcAttrs {
						if v := attrVal(n, a); v != "" {
							setAttr(n, "src", v)
							break
						}
					}
				}
				// data-srcset 归一到 srcset
				if attrVal(n, "srcset") == "" {
					if v := attrVal(n, "data-srcset"); v != "" {
						setAttr(n, "srcset", v)
					}
				}
				if v := attrVal(n, "src"); v != "" {
					setAttr(n, "src", toAbsoluteURL(v, base))
				}
				if v := attrVal(n, "srcset"); v != "" {
					setAttr(n, "srcset", rewriteSrcsetAbsolute(v, base))
				}
			case "source":
				if v := attrVal(n, "srcset"); v != "" {
					setAttr(n, "srcset", rewriteSrcsetAbsolute(v, base))
				}
				if v := attrVal(n, "src"); v != "" {
					setAttr(n, "src", toAbsoluteURL(v, base))
				}
			case "video", "audio":
				if v := attrVal(n, "poster"); v != "" {
					setAttr(n, "poster", toAbsoluteURL(v, base))
				}
				if v := attrVal(n, "src"); v != "" {
					setAttr(n, "src", toAbsoluteURL(v, base))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}

	var buf bytes.Buffer
	for _, n := range nodes {
		if err := html.Render(&buf, n); err != nil {
			return content
		}
	}
	return buf.String()
}

func attrVal(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, name, val string) {
	for i, a := range n.Attr {
		if a.Key == name {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: name, Val: val})
}

// toAbsoluteURL 将可能为相对路径的地址解析为基于 base 的绝对 URL。
// data:/blob: 直接返回；//host/path 补上 base 的 scheme；已绝对则原样返回。
func toAbsoluteURL(src string, base *url.URL) string {
	if src == "" || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "blob:") {
		return src
	}
	if strings.HasPrefix(src, "//") {
		return base.Scheme + ":" + src
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}
	if parsed, err := url.Parse(src); err == nil {
		return base.ResolveReference(parsed).String()
	}
	return src
}

// rewriteSrcsetAbsolute 将 srcset 中每个候选 URL 解析为绝对 URL，保留宽度/像素密度描述符。
func rewriteSrcsetAbsolute(srcset string, base *url.URL) string {
	parts := strings.Split(srcset, ",")
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		rest := ""
		if len(fields) > 1 {
			rest = " " + strings.Join(fields[1:], " ")
		}
		parts[i] = toAbsoluteURL(fields[0], base) + rest
	}
	return strings.Join(parts, ", ")
}

// ReadabilityResult 阅读模式提取结果
type ReadabilityResult struct {
	Title       string `json:"title"`
	Byline      string `json:"byline"`
	Content     string `json:"content"`
	TextContent string `json:"textContent"`
	Excerpt     string `json:"excerpt"`
	SiteName    string `json:"siteName"`
	URL         string `json:"url"`
}

// readabilityHTTPClient 复用同一个 HTTP client，避免每次请求都创建新连接池。
// 使用 CookieJar 模拟浏览器行为，部分网站依赖 Cookie 验证身份。
var readabilityHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
	Transport: TransportWithSSRFProtection(),
}

func init() {
	jar, err := cookiejar.New(nil)
	if err == nil {
		readabilityHTTPClient.Jar = jar
	}
}

// FetchReadability 抓取 URL 并使用阅读模式提取正文
func FetchReadability(rawURL string) (*ReadabilityResult, error) {
	return FetchReadabilityWithClient(rawURL, readabilityHTTPClient)
}

// FetchReadabilityWithClient 使用指定 HTTP 客户端抓取并提取正文
func FetchReadabilityWithClient(rawURL string, client *http.Client) (*ReadabilityResult, error) {
	if err := ValidateURLOnly(rawURL); err != nil {
		return nil, fmt.Errorf("url validation failed: %w", err)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	resp, err := fetchHTMLWithClient(rawURL, client)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && contentType != "" {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(resp.Body, 16<<20)); err != nil {
		return nil, err
	}

	article, err := readability.FromReader(bytes.NewReader(buf.Bytes()), parsedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse readability: %w", err)
	}

	return &ReadabilityResult{
		Title:       article.Title,
		Byline:      article.Byline,
		Content:     normalizeReadabilityContent(article.Content, parsedURL),
		TextContent: article.TextContent,
		Excerpt:     article.Excerpt,
		SiteName:    article.SiteName,
		URL:         rawURL,
	}, nil
}

// ProxyOriginal 抓取原文 HTML 并返回，用于在当前框架内显示
func ProxyOriginal(rawURL string) (*http.Response, error) {
	if err := ValidateURLOnly(rawURL); err != nil {
		return nil, fmt.Errorf("url validation failed: %w", err)
	}
	return fetchHTMLWithClient(rawURL, readabilityHTTPClient)
}

// ProxyOriginalWithClient 使用指定 HTTP 客户端抓取原文 HTML
func ProxyOriginalWithClient(rawURL string, client *http.Client) (*http.Response, error) {
	if err := ValidateURLOnly(rawURL); err != nil {
		return nil, fmt.Errorf("url validation failed: %w", err)
	}
	return fetchHTMLWithClient(rawURL, client)
}

// setBrowserHeaders 设置模拟真实 Chrome 131 浏览器的通用 HTTP 请求头。
// 注意：此函数不设置 Accept-Encoding，让 Go 的 http.Transport 自动处理 gzip 解压；
// 也不设置 Referer, Sec-Fetch-* 等特定于请求类型的头，由调用方按需设置。
func setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-CH-UA", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	req.Header.Set("Priority", "u=0, i")
}

// setDocumentNavHeaders 设置文档导航类请求的 Sec-Fetch-* 头（适用于页面加载）
func setDocumentNavHeaders(req *http.Request) {
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
}

// fetchHTMLWithClient 使用指定客户端创建 HTML 请求，模拟直接导航到目标 URL
func fetchHTMLWithClient(rawURL string, client *http.Client) (*http.Response, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	setDocumentNavHeaders(req)
	// 直接导航不设 Referer，模拟用户在地址栏输入 URL
	return client.Do(req)
}

// FetchImage 代理图片资源，返回原始响应。
// referer 为嵌入该图片的页面 URL（即文章原文 URL），用于绕过 CDN 防盗链检测。
// 内置简单重试机制（最多 2 次），应对瞬态网络错误。
func FetchImage(rawURL string, referer string, client *http.Client) (*http.Response, error) {
	if err := ValidateURLOnly(rawURL); err != nil {
		return nil, fmt.Errorf("url validation failed: %w", err)
	}

	buildReq := func() (*http.Request, error) {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return nil, err
		}
		setBrowserHeaders(req)
		// 图片请求的 Referer 必须设为文章页面的 URL，CDN 防盗链依赖此值
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
		req.Header.Set("Sec-Fetch-Dest", "image")
		req.Header.Set("Sec-Fetch-Mode", "no-cors")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		return req, nil
	}

	const maxAttempts = 2
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := buildReq()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		if attempt < maxAttempts {
			slog.Warn("image fetch failed, retrying", "url", rawURL, "attempt", attempt, "error", err)
			time.Sleep(time.Second)
		}
	}

	return nil, fmt.Errorf("image fetch failed after %d attempts: %w", maxAttempts, lastErr)
}

// FetchCSS 下载外部 CSS 文件，使用浏览器请求头模拟真实浏览器访问。
// 用于原文代理时内联外部样式表，解决 iframe srcdoc 中样式无法加载的问题。
// referer 为嵌入该 CSS 的页面 URL（即文章原文 URL），用于绕过 CDN 防盗链检测。
func FetchCSS(rawURL string, referer string, client *http.Client) (*http.Response, error) {
	if err := ValidateURLOnly(rawURL); err != nil {
		return nil, fmt.Errorf("url validation failed: %w", err)
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	setBrowserHeaders(req)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Accept", "text/css,*/*;q=0.1")
	req.Header.Set("Sec-Fetch-Dest", "style")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	return client.Do(req)
}