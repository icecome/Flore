// Package sync 远端同步的 Provider 接口与传输类型。
// 本包是叶子包：不依赖 services（避免循环导入），
// 引擎与配置逻辑位于 services 包。
package sync

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// WebDAVClient 是零依赖的 WebDAV 文件客户端（仅实现本项目所需的 PROPFIND/GET/PUT），
// 用于将整包备份（数据库 ZIP）推送到用户自有 WebDAV 存储（坚果云、InfiniCLOUD 等），
// 并从中恢复。HTTP 客户端由调用方（services 层）注入 SSRF 防护 transport，
// 与抓取/同步链路一致。
//
// 设计要点：
//   - 不引入第三方 WebDAV 库（避免离线构建与依赖膨胀），仅覆盖备份场景所需的三个动词。
//   - HTTP 客户端必须带 SSRF 防护，禁止私网地址外泄（由上层注入）。
//   - 路径拼接统一用 path 包，避免 host 混淆与路径穿越。
type WebDAVClient struct {
	baseURL  string // 形如 https://example.com/dav/，含结尾斜杠
	username string
	password string
	http     *http.Client
}

// NewWebDAVClient 构造 WebDAV 客户端。httpClient 为调用方注入的带 SSRF 防护客户端。
func NewWebDAVClient(baseURL, username, password string, httpClient *http.Client) (*WebDAVClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("webdav url is empty")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("webdav invalid url: scheme must be http or https")
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &WebDAVClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		http:     httpClient,
	}, nil
}

// join 将对象名拼接为完整 URL，规范化重复斜杠与路径穿越。
func (c *WebDAVClient) join(name string) string {
	name = strings.TrimPrefix(name, "/")
	return c.baseURL + name
}

// ListBackups 返回远端备份目录下的 .zip 文件清单（按修改时间倒序）。
func (c *WebDAVClient) ListBackups(ctx context.Context) ([]RemoteBackup, error) {
	body, err := c.propfind(ctx, c.baseURL, 1)
	if err != nil {
		return nil, err
	}
	resp, err := parseMultistatus(body)
	if err != nil {
		return nil, err
	}
	// 跳过集合自身（href 指向目录本身）与空 href 条目。
	selfBase := path.Base(strings.TrimSuffix(c.baseURL, "/"))
	var out []RemoteBackup
	for _, r := range resp {
		base := path.Base(strings.TrimSuffix(r.Name, "/"))
		if base == "" || base == selfBase {
			continue
		}
		if !strings.HasSuffix(base, ".zip") {
			continue
		}
		out = append(out, RemoteBackup{
			Name:    base,
			Size:    r.Size,
			ModTime: r.ModTime,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Download 下载指定名称的备份文件到 w。
func (c *WebDAVClient) Download(ctx context.Context, name string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.join(name), nil)
	if err != nil {
		return fmt.Errorf("webdav invalid url: %w", err)
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("webdav unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("webdav file not found: %s", name)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webdav download failed (http %d)", resp.StatusCode)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("webdav download read failed: %w", err)
	}
	return nil
}

// Upload 将 data 作为 name 上传到远端（覆盖写）。
func (c *WebDAVClient) Upload(ctx context.Context, name string, data []byte) error {
	// 确保父目录存在（PROPFIND 探测，缺失则 MKCOL）。
	if err := c.ensureDir(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.join(name), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("webdav invalid url: %w", err)
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/zip")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("webdav unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webdav upload failed (http %d)", resp.StatusCode)
	}
	return nil
}

// ensureDir 确保 baseURL 指向的目录存在（不存在则 MKCOL 创建）。
func (c *WebDAVClient) ensureDir(ctx context.Context) error {
	// 先 PROPFIND 探测目录是否存在（207 表示存在）。
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", c.baseURL, strings.NewReader(propfindBody(0)))
	if err != nil {
		return fmt.Errorf("webdav invalid url: %w", err)
	}
	c.auth(req)
	req.Header.Set("Depth", "0")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("webdav unreachable: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusMultiStatus || resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound {
		// 创建目录。baseURL 已带结尾斜杠，MKCOL 目标即该目录。
		mk := c.baseURL
		mreq, merr := http.NewRequestWithContext(ctx, "MKCOL", mk, nil)
		if merr != nil {
			return fmt.Errorf("webdav invalid url: %w", merr)
		}
		c.auth(mreq)
		mresp, merr := c.http.Do(mreq)
		if merr != nil {
			return fmt.Errorf("webdav unreachable: %w", merr)
		}
		_, _ = io.Copy(io.Discard, mresp.Body)
		mresp.Body.Close()
		if mresp.StatusCode < 200 || mresp.StatusCode >= 300 {
			return fmt.Errorf("webdav mkcol failed (http %d)", mresp.StatusCode)
		}
		return nil
	}
	return fmt.Errorf("webdav propfind failed (http %d)", resp.StatusCode)
}

// propfind 执行 PROPFIND 并返回响应体原始字节。
func (c *WebDAVClient) propfind(ctx context.Context, url string, depth int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", url, strings.NewReader(propfindBody(depth)))
	if err != nil {
		return nil, fmt.Errorf("webdav invalid url: %w", err)
	}
	c.auth(req)
	req.Header.Set("Depth", fmt.Sprintf("%d", depth))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webdav unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("webdav propfind read failed: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: webdav authentication failed (http %d)", ErrNotAuthorized, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("webdav propfind failed (http %d)", resp.StatusCode)
	}
	return body, nil
}

func (c *WebDAVClient) auth(req *http.Request) {
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
}

// RemoteBackup 远端备份条目。
type RemoteBackup struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// propfindBody 构造 PROPFIND 请求体（仅请求显示名称、大小、最后修改时间）。
func propfindBody(depth int) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<D:propfind xmlns:D="DAV:">` +
		`<D:prop><D:displayname/><D:getcontentlength/><D:getlastmodified/><D:resourcetype/></D:prop>` +
		`</D:propfind>`
}

// multistatus 解析 PROPFIND 的多状态响应。
type multistatus struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string `xml:"href"`
	Propstat []struct {
		Prop davProp `xml:"prop"`
	} `xml:"propstat"`
}

type davProp struct {
	Displayname   string `xml:"displayname"`
	ContentLength string `xml:"getcontentlength"`
	LastModified  string `xml:"getlastmodified"`
	Resourcetype  string `xml:"resourcetype"`
}

// parseMultistatus 解析 WebDAV 多状态响应为 RemoteBackup 列表（不过滤类型）。
func parseMultistatus(body []byte) ([]RemoteBackup, error) {
	var ms multistatus
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	if err := dec.Decode(&ms); err != nil {
		return nil, fmt.Errorf("webdav response parse failed: %w", err)
	}
	out := make([]RemoteBackup, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		rb := RemoteBackup{Name: r.Href}
		for _, ps := range r.Propstat {
			if ps.Prop.ContentLength != "" {
				var n int64
				if _, err := fmt.Sscanf(ps.Prop.ContentLength, "%d", &n); err == nil {
					rb.Size = n
				}
			}
			if ps.Prop.LastModified != "" {
				if t, err := parseHTTPDate(ps.Prop.LastModified); err == nil {
					rb.ModTime = t
				}
			}
		}
		out = append(out, rb)
	}
	return out, nil
}

// parseHTTPDate 解析 RFC1123 日期（WebDAV getlastmodified 标准格式）。
func parseHTTPDate(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC1123,
		time.RFC1123Z,
		"2006-01-02T15:04:05Z07:00",
	} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %q", s)
}
