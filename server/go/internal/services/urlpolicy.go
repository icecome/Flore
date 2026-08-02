package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/idna"
)

// defaultUserAgent 模拟 Chrome 131 浏览器的 User-Agent
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Allowed Schemes for RSS feeds and readability proxy
var allowedSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

// dnsCache 缓存域名到 IP 的解析结果，TTL 60 秒。
// 避免每次抓取都同步阻塞 DNS 解析，同时保留安全检查（每次仍会验证 IP 是否私有）。
var (
	dnsCache    = make(map[string]cachedDNS)
	dnsCacheMu  sync.RWMutex
	dnsCacheTTL = 60 * time.Second
)

type cachedDNS struct {
	ips      []string
	cachedAt time.Time
}

// lookupHostWithCache 带缓存的 DNS 解析，TTL 内命中缓存，之后刷新。
// 仍会校验每个 IP 是否私有（防 DNS rebinding）。
func lookupHostWithCache(ctx context.Context, host string) ([]string, error) {
	dnsCacheMu.RLock()
	entry, ok := dnsCache[host]
	dnsCacheMu.RUnlock()

	if ok && time.Since(entry.cachedAt) < dnsCacheTTL {
		// 缓存命中，仍需验证 IP（防 DNS rebinding）
		for _, a := range entry.ips {
			if isPrivateIP(a) {
				return nil, fmt.Errorf("blocked connection: %s resolves to private IP %s", host, a)
			}
		}
		return entry.ips, nil
	}

	// 缓存未命中或过期，重新解析
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup failed for %s: %w", host, err)
	}
	for _, a := range addrs {
		if isPrivateIP(a) {
			return nil, fmt.Errorf("blocked connection: %s resolves to private IP %s", host, a)
		}
	}

	dnsCacheMu.Lock()
	dnsCache[host] = cachedDNS{ips: addrs, cachedAt: time.Now()}
	dnsCacheMu.Unlock()
	return addrs, nil
}

var blockedDomains = []string{
	"metadata.google.internal",
	"metadata.azure.com",
	"metadata.tencentyun.com",
	"169.254.169.254",
	"in-addr.arpa",
}

// isPrivateIP 使用 net.IP 标准方法判断 IP 是否属于私有/保留地址段。
// 覆盖 RFC 1918、loopback、link-local、ULA、未指定地址、0.0.0.0/8 段等，
// 且能正确处理 IPv4-mapped IPv6（如 ::ffff:127.0.0.1）等绕过场景。
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// 0.0.0.0/8 段在 RFC 6890 中定义为"本网络上的主机"，IsUnspecified() 仅匹配 0.0.0.0
	// 需要额外检查整个 0.0.0.0/8 段，避免攻击者使用 0.0.0.5 等地址绕过
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// ValidateURL checks whether the given URL is safe to fetch:
//   - scheme must be http or https
//   - target host must not resolve to a private/reserved IP
//   - disallowed DNS names are checked against a simple deny list
func ValidateURL(rawURL string) error {
	host, err := validateURLBasic(rawURL)
	if err != nil {
		return err
	}

	// Resolve DNS with timeout and verify the resolved IP is not private
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}

	for _, addr := range addrs {
		if isPrivateIP(addr) {
			return fmt.Errorf("host %s resolves to private IP %s", host, addr)
		}
	}

	return nil
}

// validateURLBasic parses the URL, checks scheme, hostname, and blocked domains.
// Returns the normalized hostname on success.
func validateURLBasic(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty url")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url format")
	}

	if !allowedSchemes[parsed.Scheme] {
		return "", fmt.Errorf("scheme %q not allowed", parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("empty hostname")
	}

	if normalized, err := idna.ToASCII(host); err == nil {
		host = normalized
	}

	lowerHost := strings.ToLower(host)
	for _, bd := range blockedDomains {
		if lowerHost == bd || strings.HasSuffix(lowerHost, "."+bd) {
			return "", fmt.Errorf("blocked domain %s", host)
		}
	}

	return host, nil
}

// ValidateURLOnly does a lightweight URL format check without DNS resolution.
// Use this for bulk imports (e.g. OPML) where strict SSRF protection is handled
// at fetch time by ValidateURL + TransportWithSSRFProtection.
// Also used by FetchRSSFeedWithClient to avoid double DNS resolution,
// since TransportWithSSRFProtection.DialContext already blocks private IPs.
func ValidateURLOnly(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("empty url")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url format")
	}

	if !allowedSchemes[parsed.Scheme] {
		return fmt.Errorf("scheme %q not allowed", parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("empty hostname")
	}

	// Basic format check: reject obvious internals
	lowerHost := strings.ToLower(host)
	for _, bd := range blockedDomains {
		if lowerHost == bd || strings.HasSuffix(lowerHost, "."+bd) {
			return fmt.Errorf("blocked domain %s", host)
		}
	}

	// Reject IP literals that look private
	if ip := net.ParseIP(host); ip != nil && isPrivateIP(host) {
		return fmt.Errorf("host resolves to private IP")
	}

	return nil
}

// TransportWithSSRFProtection returns an http.RoundTripper that blocks connections
// to private/reserved IP addresses during dial phase (defense-in-depth against DNS rebinding).
//
// 注意：DialContext 阶段必须再次解析域名并校验 IP，不能仅依赖 ValidateURL 的预解析。
// 原因：ValidateURL 与 Dial 之间存在 TOCTOU 窗口，攻击者可利用 DNS rebinding
// 在 ValidateURL 通过后让 Dial 实际连接到私有 IP。
func TransportWithSSRFProtection() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}

			// IP 字面量直接检查
			if isPrivateIP(host) {
				return nil, fmt.Errorf("blocked connection to private IP %s", host)
			}

			// 使用缓存 DNS 解析，TTL 内命中缓存，之后刷新（保留安全检查）
			if _, err := lookupHostWithCache(ctx, host); err != nil {
				return nil, err
			}

			return (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, network, addr)
		},
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     120 * time.Second,
	}
}
