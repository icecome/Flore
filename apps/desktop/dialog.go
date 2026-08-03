package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) OpenExternal(url string) error {
	ctx := a.context()
	if ctx == nil {
		return fmt.Errorf("app context not ready")
	}
	parsed, err := neturl.Parse(url)
	if err != nil {
		a.logger.Printf("OpenExternal rejected unparsable URL")
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		a.logger.Printf("OpenExternal rejected invalid URL scheme: %q", parsed.Scheme)
		return fmt.Errorf("unsupported URL scheme: %q", parsed.Scheme)
	}
	wailsRuntime.BrowserOpenURL(ctx, url)
	return nil
}

// opmlImportLimit OPML 导入文件大小上限。
const opmlImportLimit = 64 << 20

// PickOPMLFile 打开原生文件选择对话框，选择 OPML 文件后返回文件内容。
// 前端在桌面模式下使用此方法代替 <input type="file">，因为 WebView2 中
// 程序式 click 触发 file input 可能被安全策略阻止。
func (a *App) PickOPMLFile() (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", fmt.Errorf("app context not ready")
	}
	file, err := wailsRuntime.OpenFileDialog(ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择 OPML 文件",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "OPML 文件",
				Pattern:     "*.opml;*.xml",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("file dialog error: %w", err)
	}
	if file == "" {
		return "", nil // 用户取消
	}
	f, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("open file error: %w", err)
	}
	// N2：多读 1 字节用于判定是否超限，避免静默截断导致订阅源丢失。
	data, err := io.ReadAll(io.LimitReader(f, opmlImportLimit+1))
	_ = f.Close()
	if err != nil {
		return "", fmt.Errorf("read file error: %w", err)
	}
	if len(data) > opmlImportLimit {
		return "", fmt.Errorf("OPML 文件超过 %d MB 上限，请拆分后再导入", opmlImportLimit>>20)
	}
	return string(data), nil
}

// httpClient 是带超时的本地 HTTP 客户端，避免默认 Client 无超时阻塞。
var httpClient = &http.Client{Timeout: 30 * time.Second}

// settingsClient 用于设置读取，超时极短，
// 保证即使后端假死也绝不会长时间占用调用方（M2）。
var settingsClient = &http.Client{Timeout: 300 * time.Millisecond}

// healthClient 用于后端健康探测，单次探测必须快速失败以便继续轮询。
var healthClient = &http.Client{Timeout: 2 * time.Second}

// exportClient 用于数据库/OPML 导出：整库导出可能达数百 MB，
// 全局 30s 超时会覆盖整个 body 读取过程导致大库必然失败（M6）。
var exportClient = &http.Client{Timeout: 30 * time.Minute}

// authorize 给请求附加 Bearer Token（M5）。
func (a *App) authorize(req *http.Request) {
	if a.apiToken == "" || req == nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.apiToken)
}

// doRequest 构建带鉴权头的请求并执行。
func (a *App) doRequest(client *http.Client, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	a.authorize(req)
	return client.Do(req)
}

// drainAndClose 在关闭前排空响应体，使连接可被复用而非被强制断开（T4）。
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

// SaveFileDialogOptions 保存文件对话框的参数。
type SaveFileDialogOptions struct {
	Title           string
	DefaultFilename string
	DisplayName     string
	Pattern         string
}

// pickSavePath 打开原生保存文件对话框并返回用户选择的路径，取消时返回 ("", nil)。
func (a *App) pickSavePath(opts SaveFileDialogOptions) (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", fmt.Errorf("app context not ready")
	}
	path, err := wailsRuntime.SaveFileDialog(ctx, wailsRuntime.SaveDialogOptions{
		Title:           opts.Title,
		DefaultFilename: opts.DefaultFilename,
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: opts.DisplayName,
				Pattern:     opts.Pattern,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog error: %w", err)
	}
	return path, nil
}

// saveFileWithDialog 打开原生保存文件对话框，将 data 写入用户选择的路径。
// 用户取消时返回 ("", nil)。
func (a *App) saveFileWithDialog(data []byte, opts SaveFileDialogOptions) (string, error) {
	path, err := a.pickSavePath(opts)
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write file error: %w", err)
	}
	return path, nil
}

// exportSizeLimit 导出文件大小上限（256MB，与后端备份上限一致）。
const exportSizeLimit int64 = 256 << 20

// streamBackendDataToDialog 从后端流式下载数据并直接写入用户选择的文件（M6）。
// 全程不把整库读进内存；超过上限时删除半成品并返回错误，绝不生成损坏的备份文件。
func (a *App) streamBackendDataToDialog(apiPath string, opts SaveFileDialogOptions) (string, error) {
	port := a.getPort()
	if port == 0 {
		return "", fmt.Errorf("backend not ready")
	}

	path, err := a.pickSavePath(opts)
	if err != nil || path == "" {
		return "", err
	}

	resp, err := a.doRequest(exportClient, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, apiPath), nil)
	if err != nil {
		return "", fmt.Errorf("export request failed: %w", err)
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("export request returned HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create export file: %w", err)
	}

	// 多读 1 字节用于判定是否超限：超限即报错，而不是静默截断出损坏文件。
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, exportSizeLimit+1))
	closeErr := f.Close()

	if copyErr != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write export file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close export file: %w", closeErr)
	}
	if written > exportSizeLimit {
		_ = os.Remove(path)
		return "", fmt.Errorf("导出数据超过 %d MB 上限，已中止以避免生成损坏文件", exportSizeLimit>>20)
	}
	return path, nil
}

// SaveOPMLFile 打开原生保存文件对话框，导出 OPML 文件。
func (a *App) SaveOPMLFile() (string, error) {
	return a.streamBackendDataToDialog("/api/opml/export", SaveFileDialogOptions{
		Title:           "导出 OPML 文件",
		DefaultFilename: "subscriptions.opml",
		DisplayName:     "OPML 文件",
		Pattern:         "*.opml",
	})
}

// SaveDatabaseFile 打开原生保存文件对话框，导出数据库备份文件。
func (a *App) SaveDatabaseFile() (string, error) {
	return a.streamBackendDataToDialog("/api/database/export", SaveFileDialogOptions{
		Title:           "导出数据库备份",
		DefaultFilename: fmt.Sprintf("rss-backup-%s.db", time.Now().Format("2006-01-02-150405")),
		DisplayName:     "SQLite 数据库",
		Pattern:         "*.db",
	})
}

// SaveBackupFile 打开原生保存文件对话框，下载备份 ZIP 文件到用户指定位置。
func (a *App) SaveBackupFile(name string) (string, error) {
	return a.streamBackendDataToDialog("/api/backups/"+neturl.PathEscape(name)+"/download", SaveFileDialogOptions{
		Title:           "导出备份",
		DefaultFilename: name,
		DisplayName:     "ZIP 备份文件",
		Pattern:         "*.zip",
	})
}

// SaveConfigFile 打开原生保存文件对话框，保存 JSON 配置文件。
// configJSON 为 JSON 格式的配置字符串。
func (a *App) SaveConfigFile(configJSON string) (string, error) {
	return a.saveFileWithDialog([]byte(configJSON), SaveFileDialogOptions{
		Title:           "导出配置",
		DefaultFilename: fmt.Sprintf("flore-config-%s.json", time.Now().Format("2006-01-02")),
		DisplayName:     "JSON 配置文件",
		Pattern:         "*.json",
	})
}

// SavePNGFile 打开原生保存文件对话框，保存 PNG 图片。
// data 为 base64 编码的 PNG 图片数据。
func (a *App) SavePNGFile(data string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("base64 decode error: %w", err)
	}
	return a.saveFileWithDialog(raw, SaveFileDialogOptions{
		Title:           "导出 PNG 图片",
		DefaultFilename: fmt.Sprintf("export-%s.png", time.Now().Format("2006-01-02-150405")),
		DisplayName:     "PNG 图片",
		Pattern:         "*.png",
	})
}

// getPort 返回当前后端端口（线程安全）。
