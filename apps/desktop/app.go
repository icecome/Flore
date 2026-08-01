package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context

	backendMutex sync.Mutex
	goCmd        *exec.Cmd
	goStarted    bool
	goPort       int
	logger       *logFile
	forceQuit    atomic.Bool

	// 窗口行为设置缓存，避免每次窗口操作都同步请求后端
	settingsMutex      sync.Mutex
	minimizeBehavior   string
	closeBehavior      string
	notifyEnabled      bool
	settingsCacheReady bool

	// notifyCancel 用于在 shutdown 时停止 startNotifyWatcher goroutine
	notifyCancel context.CancelFunc
}

// logFile 将日志写入用户数据目录，便于在 GUI 模式下调试后端启动问题。
type logFile struct {
	file *os.File
}

func newLogFile(dir string) *logFile {
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "flore-desktop.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return &logFile{file: nil}
	}
	return &logFile{file: f}
}

func (l *logFile) Printf(format string, args ...interface{}) {
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "["+time.Now().Format("15:04:05")+"] "+format+"\n", args...)
	_ = l.file.Sync()
}

// NewApp creates a new App application struct
func NewApp() *App {
	a := &App{}
	a.logger = newLogFile(a.appDataDir())
	return a
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logger.Printf("Wails startup")

	// 设置窗口标题，覆盖 Wails 开发模式下的默认标题
	wailsRuntime.WindowSetTitle(a.ctx, "Flore")

	// 注册 Flore 到 Windows 注册表，使原生 Toast 显示应用名而非 wails.localhost
	a.registerNotificationApp()

	// 启动系统托盘
	a.startTray()

	// 异步启动本地后端服务，避免阻塞 UI（健康检查最长 15s）
	go a.startBackends()

	// 启动后台通知监听：检测抓取完成并发送原生系统通知，覆盖托盘/调度盲区（M-A3）
	// 使用独立 cancel context，shutdown 时显式停止 goroutine 避免泄漏
	notifyCtx, notifyCancel := context.WithCancel(context.Background())
	a.notifyCancel = notifyCancel
	go a.startNotifyWatcher(notifyCtx)
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	a.logger.Printf("Wails shutdown")
	// 停止通知监听 goroutine，避免泄漏
	if a.notifyCancel != nil {
		a.notifyCancel()
	}
	a.stopTray()
	a.stopBackends()
	if a.logger != nil && a.logger.file != nil {
		_ = a.logger.file.Close()
	}
}

// startBackends 启动 Go 后端，并使用动态高位端口避免冲突。
func (a *App) startBackends() {
	dbPath := a.readerDatabasePath()
	a.logger.Printf("database path: %s", dbPath)

	goExe := a.findGoBackend()
	a.logger.Printf("found go backend: %s", goExe)
	if goExe == "" {
		a.logger.Printf("go backend skipped: executable not found")
		return
	}

	port, err := a.findFreePort()
	if err != nil {
		a.logger.Printf("failed to find free port for go backend: %v", err)
		return
	}

	cmd := a.startProcess("go-backend", goExe, []string{}, map[string]string{
		"PORT":         fmt.Sprintf("%d", port),
		"DATABASE_URL": dbPath,
		"FLORE_LOG_FILE": filepath.Join(filepath.Dir(dbPath), "flore-backend.log"),
		// 桌面端前端 origin 可能是 Wails dev server 或 wails.localhost，
		// 必须全部加入 CORS 白名单，否则前端 fetch 会被拒绝。
		// 注意：gin-contrib/cors 要求 origin 必须以 http:// 或 https:// 开头，
		// 不能使用 wails:// scheme。
		"CORS_ORIGINS": fmt.Sprintf(
			"http://127.0.0.1:%d,http://localhost:%d,http://localhost:34115,http://wails.localhost,https://wails.localhost",
			port, port,
		),
	})

	// startProcess 失败（返回 nil）时不记录 port，避免后续 getSetting 向未监听端口发起请求触发 30s 超时
	if cmd == nil {
		return
	}

	a.backendMutex.Lock()
	a.goCmd = cmd
	a.goPort = port
	a.backendMutex.Unlock()

	started := a.waitForBackend("go-backend", port, 15)

	a.backendMutex.Lock()
	a.goStarted = started
	a.backendMutex.Unlock()

	if started {
		a.logger.Printf("Go backend ready at http://127.0.0.1:%d", port)
		// 后端就绪后加载窗口行为设置缓存，避免首次窗口操作阻塞
		a.loadWindowSettings()
	}
}

// stopBackends 停止已启动的后端进程。
// 优先 POST /api/shutdown 触发后端优雅关闭，超时后再 Kill。
func (a *App) stopBackends() {
	a.backendMutex.Lock()
	cmd := a.goCmd
	port := a.goPort
	started := a.goStarted
	a.backendMutex.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	// 先尝试 POST /api/shutdown 触发后端自身优雅关闭（含 WAL checkpoint 与连接释放）
	if started {
		a.requestGracefulShutdown(port)
	}

	// 等待进程退出，最多 2.5 秒
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		a.logger.Printf("[go-backend] process exited gracefully")
		return
	case <-time.After(2500 * time.Millisecond):
		if err := cmd.Process.Kill(); err != nil {
			a.logger.Printf("[go-backend] failed to kill process: %v", err)
		} else {
			a.logger.Printf("[go-backend] process killed after timeout")
		}
		<-done
	}
}

// requestGracefulShutdown 向后端发送优雅关闭请求（fire-and-forget）。
func (a *App) requestGracefulShutdown(port int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/shutdown", port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		a.logger.Printf("[go-backend] graceful shutdown request failed: %v", err)
		return
	}
	resp.Body.Close()
}

// startProcess 启动一个子进程并返回 exec.Cmd。
func (a *App) startProcess(name string, exe string, args []string, env map[string]string) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = a.logger.file
	cmd.Stderr = a.logger.file
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Windows 下隐藏后端进程控制台窗口，避免启动时弹出黑色终端。
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	if err := cmd.Start(); err != nil {
		a.logger.Printf("[%s] failed to start %s: %v", name, exe, err)
		return nil
	}
	a.logger.Printf("[%s] started %s (pid=%d) env=%v", name, exe, cmd.Process.Pid, env)
	return cmd
}

// waitForBackend 轮询后端健康检查接口，直到服务就绪或超时。
func (a *App) waitForBackend(name string, port int, timeoutSec int) bool {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				a.logger.Printf("[%s] health check passed on port %d", name, port)
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	a.logger.Printf("[%s] health check timed out on port %d", name, port)
	return false
}

// findFreePort 使用 127.0.0.1:0 让系统分配一个空闲高位端口。
func (a *App) findFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// findGoBackend 查找 Go 后端可执行文件。
func (a *App) findGoBackend() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	targetName := "flore-backend" + ext

	if found := a.findBackendByExecutable(targetName); found != "" {
		return found
	}

	if found := a.findBackendByCWD(targetName, ext); found != "" {
		return found
	}

	return ""
}

// findBackendByExecutable 从可执行文件所在目录向上遍历（最多 3 层），查找后端可执行文件。
// 限制查找深度避免在高层目录意外命中同名不可信文件。
func (a *App) findBackendByExecutable(targetName string) string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}

	appDir := filepath.Dir(exePath)
	const maxDepth = 3
	depth := 0
	for dir := appDir; dir != "" && dir != filepath.Dir(dir) && depth < maxDepth; dir = filepath.Dir(dir) {
		for _, c := range []string{
			filepath.Join(dir, targetName),
			filepath.Join(dir, "build", "bin", targetName),
		} {
			if _, statErr := os.Stat(c); statErr == nil {
				return c
			}
		}
		depth++
	}
	return ""
}

// findBackendByCWD 从当前工作目录出发，查找后端可执行文件。
func (a *App) findBackendByCWD(targetName string, ext string) string {
	cwd, _ := os.Getwd()
	candidates := []string{
		targetName,
		filepath.Join("build", "bin", targetName),
		filepath.Join("..", "build", "bin", targetName),
		filepath.Join("..", "..", "build", "bin", targetName),
		filepath.Join(cwd, targetName),
		filepath.Join(cwd, "..", "server", "go", "flore-backend"+ext),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}



// appDataDir 返回应用数据目录，是所有本地产物（数据库、日志、WebView2 缓存等）的统一落盘根。
// 解析顺序：
//  1. 环境变量 FLORE_DATA_DIR（显式指定，优先级最高）
//  2. 便携模式：可执行文件同级目录存在 data/ 时，使用 data/（便携版跟随目录走）
//  3. 回退：用户数据目录（Windows 优先 %LOCALAPPDATA%/Flore，否则 HOME/.flore）
//
// 注意：便携判定仅“探测” data/ 是否存在、不会主动创建；若用户删除了该空目录，
// 应用会静默回退到第 3 项，这是已知的便携性边界，不在本次修复范围内。
func (a *App) appDataDir() string {
	if env := os.Getenv("FLORE_DATA_DIR"); env != "" {
		return env
	}
	if dir := a.findPortableDataDir(); dir != "" {
		return dir
	}
	return a.userDataDirectory()
}

// findPortableDataDir 便携模式检测：可执行文件同级目录存在 data/ 时返回其路径。
func (a *App) findPortableDataDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	dataDir := filepath.Join(filepath.Dir(exePath), "data")
	info, statErr := os.Stat(dataDir)
	if statErr != nil || !info.IsDir() {
		return ""
	}
	return dataDir
}

// webviewDataPath 返回 WebView2 用户数据目录，统一收归到应用数据目录下，
// 避免 Wails 默认把缓存散落到 %APPDATA%\[BinaryName.exe]。
// WebView2 对无效路径会直接弹窗并报错退出，这里提前确保目录存在。
func (a *App) webviewDataPath() string {
	dir := filepath.Join(a.appDataDir(), "webview2")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

// readerDatabasePath 返回 Reader 数据库文件路径。
// 优先使用环境变量 DATABASE_URL，否则落到应用数据目录下的 reader.db。
func (a *App) readerDatabasePath() string {
	if env := os.Getenv("DATABASE_URL"); env != "" {
		return env
	}
	baseDir := a.appDataDir()
	_ = os.MkdirAll(baseDir, 0700)
	return filepath.Join(baseDir, "reader.db")
}

// userDataDirectory 返回用户数据目录，Windows 下优先使用 LOCALAPPDATA/APPDATA，否则使用 HOME/.flore。
func (a *App) userDataDirectory() string {
	if runtime.GOOS == "windows" {
		for _, envName := range []string{"LOCALAPPDATA", "APPDATA"} {
			if dir := os.Getenv(envName); dir != "" {
				return filepath.Join(dir, "Flore")
			}
		}
		return filepath.Join(os.TempDir(), "Flore")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".flore")
}

// GetBackendStatus 返回后端服务状态与动态地址，供前端在桌面模式下使用。
type BackendStatus struct {
	GoStarted bool   `json:"goStarted"`
	GoBaseURL string `json:"goBaseURL"`
}

func (a *App) GetBackendStatus() BackendStatus {
	a.backendMutex.Lock()
	defer a.backendMutex.Unlock()
	return BackendStatus{
		GoStarted: a.goStarted,
		GoBaseURL: fmt.Sprintf("http://127.0.0.1:%d", a.goPort),
	}
}

// OpenExternal 在系统默认浏览器中打开指定 URL。
// 仅允许 http/https scheme，避免任意协议唤起（如 file://、javascript:）。
func (a *App) OpenExternal(url string) {
	if a.ctx == nil {
		return
	}
	parsed, err := neturl.Parse(url)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		a.logger.Printf("OpenExternal rejected invalid URL scheme: %q", url)
		return
	}
	wailsRuntime.BrowserOpenURL(a.ctx, url)
}

// PickOPMLFile 打开原生文件选择对话框，选择 OPML 文件后返回文件内容。
// 前端在桌面模式下使用此方法代替 <input type="file">，因为 WebView2 中
// 程序式 click 触发 file input 可能被安全策略阻止。
func (a *App) PickOPMLFile() (string, error) {
	file, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
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
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	_ = f.Close()
	if err != nil {
		return "", fmt.Errorf("read file error: %w", err)
	}
	return string(data), nil
}

// httpClient 是带超时的本地 HTTP 客户端，避免默认 Client 无超时阻塞。
var httpClient = &http.Client{Timeout: 30 * time.Second}

// SaveFileDialogOptions 保存文件对话框的参数。
type SaveFileDialogOptions struct {
	Title           string
	DefaultFilename string
	DisplayName    string
	Pattern        string
}

// saveFileWithDialog 打开原生保存文件对话框，将 data 写入用户选择的路径。
// 用户取消时返回 ("", nil)。
func (a *App) saveFileWithDialog(data []byte, opts SaveFileDialogOptions) (string, error) {
	path, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
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
	if path == "" {
		return "", nil // 用户取消
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write file error: %w", err)
	}
	return path, nil
}

// fetchBackendData 从后端 API 获取导出数据。
func (a *App) fetchBackendData(path string) ([]byte, error) {
	a.backendMutex.Lock()
	port := a.goPort
	a.backendMutex.Unlock()
	if port == 0 {
		return nil, fmt.Errorf("backend not ready")
	}
	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return nil, fmt.Errorf("export request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("export request returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// SaveOPMLFile 打开原生保存文件对话框，导出 OPML 文件。
func (a *App) SaveOPMLFile() (string, error) {
	data, err := a.fetchBackendData("/api/opml/export")
	if err != nil {
		return "", err
	}
	return a.saveFileWithDialog(data, SaveFileDialogOptions{
		Title:           "导出 OPML 文件",
		DefaultFilename: "subscriptions.opml",
		DisplayName:    "OPML 文件",
		Pattern:        "*.opml",
	})
}

// SaveDatabaseFile 打开原生保存文件对话框，导出数据库备份文件。
func (a *App) SaveDatabaseFile() (string, error) {
	data, err := a.fetchBackendData("/api/database/export")
	if err != nil {
		return "", err
	}
	return a.saveFileWithDialog(data, SaveFileDialogOptions{
		Title:           "导出数据库备份",
		DefaultFilename: fmt.Sprintf("rss-backup-%s.db", time.Now().Format("2006-01-02-150405")),
		DisplayName:    "SQLite 数据库",
		Pattern:        "*.db",
	})
}

// SaveConfigFile 打开原生保存文件对话框，保存 JSON 配置文件。
// configJSON 为 JSON 格式的配置字符串。
func (a *App) SaveConfigFile(configJSON string) (string, error) {
	return a.saveFileWithDialog([]byte(configJSON), SaveFileDialogOptions{
		Title:           "导出配置",
		DefaultFilename: fmt.Sprintf("flore-config-%s.json", time.Now().Format("2006-01-02")),
		DisplayName:    "JSON 配置文件",
		Pattern:        "*.json",
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
		DisplayName:    "PNG 图片",
		Pattern:        "*.png",
	})
}

// getPort 返回当前后端端口（线程安全）。
func (a *App) getPort() int {
	a.backendMutex.Lock()
	defer a.backendMutex.Unlock()
	return a.goPort
}

// getNotifyEnabled 读取通知开关，缓存就绪时直接返回，否则回退同步查询后端。
func (a *App) getNotifyEnabled() bool {
	a.settingsMutex.Lock()
	ready := a.settingsCacheReady
	val := a.notifyEnabled
	a.settingsMutex.Unlock()
	if ready {
		return val
	}
	return a.getSetting("notifyEnabled", "false") == "true"
}

// getSetting 从后端 API 读取单个设置项，失败时返回 defaultValue。
// 用于桌面端在窗口行为控制中读取用户配置。
func (a *App) getSetting(key, defaultValue string) string {
	a.backendMutex.Lock()
	port := a.goPort
	a.backendMutex.Unlock()
	if port == 0 {
		return defaultValue
	}

	value, ok := a.fetchSettingValue(key, port)
	if !ok {
		return defaultValue
	}
	return value
}

// fetchSettingValue 从后端获取单个设置项的值，成功时返回 (value, true)。
func (a *App) fetchSettingValue(key string, port int) (string, bool) {
	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/api/settings/%s", port, key))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false
	}

	var result struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false
	}
	if result.Value == "" {
		return "", false
	}
	return result.Value, true
}

// loadWindowSettings 从后端加载窗口行为设置到缓存，避免每次窗口操作都同步 HTTP 请求。
// 在后端就绪后调用一次，用户通过前端修改设置时再次调用刷新。
func (a *App) loadWindowSettings() {
	minimize := a.getSetting("minimizeBehavior", "taskbar")
	closeBeh := a.getSetting("closeBehavior", "quit")
	notify := a.getSetting("notifyEnabled", "false") == "true"
	a.settingsMutex.Lock()
	a.minimizeBehavior = minimize
	a.closeBehavior = closeBeh
	a.notifyEnabled = notify
	a.settingsCacheReady = true
	a.settingsMutex.Unlock()
}

// getMinimizeBehavior 读取缓存的最小化行为，缓存未就绪时 fallback 到同步获取
func (a *App) getMinimizeBehavior() string {
	a.settingsMutex.Lock()
	ready := a.settingsCacheReady
	val := a.minimizeBehavior
	a.settingsMutex.Unlock()
	if ready {
		return val
	}
	return a.getSetting("minimizeBehavior", "taskbar")
}

// getCloseBehavior 读取缓存的关闭行为，缓存未就绪时 fallback 到同步获取
func (a *App) getCloseBehavior() string {
	a.settingsMutex.Lock()
	ready := a.settingsCacheReady
	val := a.closeBehavior
	a.settingsMutex.Unlock()
	if ready {
		return val
	}
	return a.getSetting("closeBehavior", "quit")
}

// RefreshWindowSettings 供前端在修改窗口行为设置后调用，刷新桌面壳缓存。
func (a *App) RefreshWindowSettings() {
	go a.loadWindowSettings()
}

// RestartApp 重启桌面应用：启动一个新实例并退出当前实例。
// 用于数据库导入等需要重新加载后端服务的场景。
// 新实例以独立进程组启动，避免被当前实例退出影响。
func (a *App) RestartApp() error {
	if a.ctx == nil {
		return fmt.Errorf("app context not ready")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve executable: %w", err)
	}

	cmd := exec.Command(exe)
	// Windows 下用 CREATE_NEW_PROCESS_GROUP 启动独立进程组，
	// 避免被当前实例退出影响；其他平台默认行为已可满足需求
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    false,
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new instance: %w", err)
	}

	// 短暂等待新进程初始化后退出当前实例
	go func() {
		time.Sleep(500 * time.Millisecond)
		a.logger.Printf("restarting app, exiting current instance")
		wailsRuntime.Quit(a.ctx)
	}()
	return nil
}

// WindowMinimise 最小化窗口。
// 根据用户配置的 minimizeBehavior 决定行为：
// - "tray"：隐藏窗口（后台运行，需系统托盘支持）
// - "taskbar"（默认）：正常最小化到任务栏
func (a *App) WindowMinimise() {
	if trayAvailable.Load() && a.getMinimizeBehavior() == "tray" {
		wailsRuntime.WindowHide(a.ctx)
		return
	}
	wailsRuntime.WindowMinimise(a.ctx)
}

// WindowMaximise 最大化窗口。
func (a *App) WindowMaximise() {
	wailsRuntime.WindowMaximise(a.ctx)
}

// WindowUnmaximise 还原窗口。
func (a *App) WindowUnmaximise() {
	wailsRuntime.WindowUnmaximise(a.ctx)
}

// WindowToggleMaximise 切换窗口最大化/还原状态。
func (a *App) WindowToggleMaximise() {
	wailsRuntime.WindowToggleMaximise(a.ctx)
}

// WindowIsMaximised 返回窗口是否已最大化。
func (a *App) WindowIsMaximised() bool {
	return wailsRuntime.WindowIsMaximised(a.ctx)
}

// WindowClose 关闭应用程序窗口。
// 根据用户配置的 closeBehavior 决定行为：
// - "tray"：隐藏窗口（后台运行，需系统托盘支持）
// - "quit"（默认）：退出应用
func (a *App) WindowClose() {
	if trayAvailable.Load() && a.getCloseBehavior() == "tray" {
		wailsRuntime.WindowHide(a.ctx)
		return
	}
	wailsRuntime.Quit(a.ctx)
}

// ShouldPreventClose 供 main.go 的 OnBeforeClose 回调使用。
// 当 closeBehavior 为 "tray" 且系统托盘支持时阻止窗口关闭并隐藏，实现后台驻留。
// 若 forceQuit 为 true（由托盘菜单"退出"触发），则跳过阻止，直接退出。
func (a *App) ShouldPreventClose() bool {
	if a.forceQuit.Load() {
		return false
	}
	if trayAvailable.Load() && a.getCloseBehavior() == "tray" {
		if a.ctx != nil {
			wailsRuntime.WindowHide(a.ctx)
		}
		return true
	}
	return false
}

// ShowWindow 显示主窗口（用于前端在需要时恢复隐藏的窗口）。
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsRuntime.WindowShow(a.ctx)
	}
}

// registerNotificationApp 将 Flore 注册到 Windows 注册表，使原生 Toast 显示应用名（Flore）
// 而非 WebView2 的 origin（wails.localhost）。非 Windows 平台为 noop。
func (a *App) registerNotificationApp() {
	if err := toast.SetAppData(toast.AppData{AppID: "Flore"}); err != nil {
		a.logger.Printf("register notification app failed: %v", err)
	}
}

// ShowNotification 发送原生 Windows Toast 通知（应用名显示为 Flore）。
// 供后台通知监听统一使用；非 Windows 平台为 noop。
func (a *App) ShowNotification(title, body string) error {
	if a.ctx == nil {
		return fmt.Errorf("app context not ready")
	}
	if title == "" {
		title = "Flore"
	}
	ntf := toast.Notification{
		AppID: "Flore",
		Title: title,
		Body:  body,
	}
	err := ntf.Push()
	if err != nil {
		a.logger.Printf("show notification failed: %v", err)
	}
	return err
}

// startNotifyWatcher 后台轮询后端抓取状态，当检测到一轮抓取完成（fetching 下降沿）
// 且本轮有新增文章时发送原生系统通知。覆盖手动刷新、托盘抓取与后台调度三路场景，
// 消除此前仅前端点击才发通知的盲区（M-A3）。
// ctx 被 cancel 时优雅退出（由 shutdown 触发）。
func (a *App) startNotifyWatcher(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var lastFetching bool
	for {
		select {
		case <-ctx.Done():
			a.logger.Printf("notify watcher: stopped")
			return
		case <-ticker.C:
		}
		port := a.getPort()
		if port == 0 {
			lastFetching = false
			continue
		}
		resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/api/sources/fetch-status", port))
		if err != nil {
			lastFetching = false
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		_ = resp.Body.Close()
		if err != nil {
			lastFetching = false
			continue
		}
		var st struct {
			Fetching bool `json:"fetching"`
			NewItems int  `json:"newItems"`
		}
		if err := json.Unmarshal(body, &st); err != nil {
			lastFetching = false
			continue
		}
		// 下降沿：本轮抓取刚完成，且确有新增文章，且用户开启了通知
		if lastFetching && !st.Fetching && st.NewItems > 0 && a.getNotifyEnabled() {
			if err := a.ShowNotification("Flore 新文章", fmt.Sprintf("抓取到 %d 篇新文章", st.NewItems)); err != nil {
				a.logger.Printf("notify watcher: failed to show notification: %v", err)
			}
		}
		lastFetching = st.Fetching
	}
}
