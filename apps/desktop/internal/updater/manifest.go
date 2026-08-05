package updater

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"
)

// defaultManifestURL 是 manifest 主拉取地址（Cloudflare R2 自定义域，国内可达）。
const defaultManifestURL = "https://cdn-dl.icecome.com/update.json"

// fallbackManifestURL 为兜底地址：当主通道（R2 自定义域）不可达时，
// 从 GitHub Release 拉取 update.json（由 release.yml 作为 Release 资产发布）。
const fallbackManifestURL = "https://github.com/icecome/Flore/releases/latest/download/update.json"

// Asset 描述一个平台的可下载更新资产。
type Asset struct {
	Platform  string   `json:"platform"`
	Variant   string   `json:"variant"`
	FileName  string   `json:"fileName"`
	Size      int64    `json:"size"`
	SHA256    string   `json:"sha256"`
	Signature string   `json:"signature"`
	URLs      []string `json:"urls"`
}

// Manifest 是 update.json 的结构。
type Manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	App           string  `json:"app"`
	Latest        string  `json:"latest"`
	MinSupported  string  `json:"minSupported"`
	PublishedAt   string  `json:"publishedAt"`
	Notes         string  `json:"notes"`
	Assets        []Asset `json:"assets"`
}

// currentPlatform 返回 "os/arch" 形式，如 "windows/amd64"。
func currentPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// manifestURLs 返回按优先级排序的 manifest 拉取地址。
// 安全说明：不允许通过环境变量覆盖 manifest 地址（曾被用于劫持更新通道，
// 配合无签名校验可导致任意代码执行）；manifest 仅从固定 HTTPS 通道拉取，
// 且资产另有 Ed25519 签名校验兜底。
func manifestURLs() []string {
	urls := []string{defaultManifestURL}
	if fallbackManifestURL != "" {
		urls = append(urls, fallbackManifestURL)
	}
	return urls
}

// FetchManifest 拉取更新清单，主地址失败则回退兜底地址。
func FetchManifest() (*Manifest, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
		},
	}
	var lastErr error
	for _, u := range manifestURLs() {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("manifest %s 返回 HTTP %d", u, resp.StatusCode)
			continue
		}
		var m Manifest
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			_ = resp.Body.Close()
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		return &m, nil
	}
	return nil, fmt.Errorf("拉取更新清单失败: %w", lastErr)
}

// matchAsset 从 manifest 中找出当前平台匹配的资产，优先度：setup > portable > 空 variant。
func matchAsset(m *Manifest) *Asset {
	plat := currentPlatform()
	// 优先找 setup（安装版），其次 portable（便携包），最后任意 variant
	for _, desiredVariant := range []string{"setup", "portable", ""} {
		for i := range m.Assets {
			a := &m.Assets[i]
			if a.Platform != plat {
				continue
			}
			if a.Variant == desiredVariant {
				return a
			}
		}
	}
	return nil
}
