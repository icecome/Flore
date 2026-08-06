package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"desktop/internal/updater"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// restartWaitPidEnv 由 RestartApp 注入新实例，新实例启动前先等待旧实例退出，
// 以便单实例互斥体被释放（见 main.go）。
const restartWaitPidEnv = "FLORE_RESTART_WAIT_PID"

// backendPathEnv 允许开发态用绝对路径显式指定后端二进制，
// 替代此前从 CWD 解析相对路径的不安全查找（M4）。
const backendPathEnv = "FLORE_BACKEND_PATH"

// version 由 version.go 在构建时生成（来源 package.json 的 version 字段），
// 桌面壳更新器据此与远端 manifest 比对判断是否需要更新。开发模式下回退 "dev"。

// App struct
type App struct {
	// ctx 由 Wails 在 OnStartup 注入，被多个 goroutine（托盘线程、通知 watcher、
	// 后端监控）并发读取，必须原子访问（M7）。
	ctx atomic.Pointer[context.Context]

	backendMutex sync.Mutex
	goCmd        *exec.Cmd
	goStarted    bool
	goPort       int
	// backendStopping 表示已进入退出流程；startBackends 在启动后发现该标志
	// 会立即回收刚拉起的进程，消除启停竞态（M1）。
	backendStopping bool
	// goExited 在后端进程被 wait 回收后关闭，stopBackends 与崩溃监控共用（M9）。
	goExited chan struct{}
	stopOnce *sync.Once

	logger    *logFile
	forceQuit atomic.Bool

	// apiToken 注入后端 FLORE_API_TOKEN，桌面壳自身请求敏感接口时携带（M5）。
	apiToken string

	// updateMu 保护 cachedUpdate，避免并发检查/应用更新竞态。
	updateMu     sync.Mutex
	cachedUpdate *updater.UpdateInfo
	// updateProgress 当前更新下载进度 0~1（以 uint64 存 float64 位，兼容 Go 1.26），
	// 原子访问，供前端轮询（GetUpdateProgress）。
	updateProgress atomic.Uint64

	// 窗口行为设置缓存。startup 时预置默认值，之后只由后台 goroutine 刷新；
	// 窗口控制路径（含运行在主 UI 线程的 OnBeforeClose）只读缓存，
	// 不再存在「缓存未就绪 → 同步 HTTP」的 fallback 分支（M2）。
	settingsMutex    sync.Mutex
	minimizeBehavior string
	closeBehavior    string
	notifyEnabled    bool

	// notifyCancel 用于在 shutdown 时停止 startNotifyWatcher goroutine，
	// 由 notifyMutex 保护（M7）。
	notifyMutex  sync.Mutex
	notifyCancel context.CancelFunc
}

// logFile 将日志写入用户数据目录，便于在 GUI 模式下调试后端启动问题。
type logFile struct {
	mu   sync.Mutex
	file *os.File
	path string
	// maxSize / maxBackups 供运行期轮转使用（N4）
	maxSize    int64
	maxBackups int
}

const (
	logMaxSize    = 10 << 20
	logMaxBackups = 5
)

func newLogFile(dir string) *logFile {
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "floredesktop.log")

	f, err := newRotatingLogFile(logPath, logMaxSize, logMaxBackups)
	if err != nil {
		// 降级到 os.DevNull：绝不能返回 nil *os.File，
		// 否则子进程的 Stdout/Stderr 会拿到 typed-nil 接口值导致写入 panic（M3）。
		devnull, devErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if devErr != nil {
			return &logFile{file: nil, path: logPath}
		}
		return &logFile{file: devnull, path: logPath}
	}
	return &logFile{
		file:       f,
		path:       logPath,
		maxSize:    logMaxSize,
		maxBackups: logMaxBackups,
	}
}

// newRotatingLogFile 打开日志文件，支持轮转
func newRotatingLogFile(path string, maxSize int64, maxBackups int) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	if info.Size() > maxSize {
		f.Close()
		if err := rotateLogFile(path, maxBackups); err != nil {
			return nil, fmt.Errorf("rotate log: %w", err)
		}
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return nil, fmt.Errorf("reopen log file: %w", err)
		}
	}

	return f, nil
}

func rotateLogFile(path string, maxBackups int) error {
	if maxBackups > 0 {
		oldPath := path + "." + fmt.Sprint(maxBackups)
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old backup: %w", err)
		}
	}
	for i := maxBackups - 1; i >= 1; i-- {
		src := path + "." + fmt.Sprint(i)
		dst := path + "." + fmt.Sprint(i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("rename backup %d: %w", i, err)
			}
		}
	}
	return os.Rename(path, path+".1")
}

func (l *logFile) Printf(format string, args ...interface{}) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "["+time.Now().Format("15:04:05")+"] "+format+"\n", args...)
	_ = l.file.Sync()
	l.rotateIfNeededLocked()
}

// rotateIfNeededLocked 在运行期检查体积并轮转，避免长时间运行的实例日志无限增长（N4）。
// 注意：子进程的 Stdout/Stderr 已绑定到旧句柄，轮转后其输出会继续写入被重命名的文件，
// 这是可接受的取舍（后端另有独立 FLORE_LOG_FILE）。
func (l *logFile) rotateIfNeededLocked() {
	if l.file == nil || l.path == "" || l.maxSize <= 0 {
		return
	}
	info, err := l.file.Stat()
	if err != nil || info.Size() <= l.maxSize {
		return
	}
	_ = l.file.Close()
	l.file = nil
	if err := rotateLogFile(l.path, l.maxBackups); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	l.file = f
}

// writer 返回可安全传给子进程的 io.Writer；日志不可用时返回 nil（调用方需判空）。
func (l *logFile) writer() *os.File {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file
}

func (l *logFile) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// NewApp creates a new App application struct
func NewApp() *App {
	a := &App{}
	a.logger = newLogFile(a.appDataDir())
	token, err := generateAPIToken()
	if err != nil {
		// fail-closed：随机源不可用时必须终止启动。
		// 空 token 会让后端敏感接口退化为无鉴权，绝不能静默降级（曾被报告为 CRITICAL）。
		a.logger.Printf("[fatal] failed to generate API token: %v", err)
		os.Exit(1)
	}
	a.apiToken = token
	return a
}

// generateAPIToken 生成 32 字节随机 token，用于后端敏感接口鉴权（M5）。
// crypto/rand 失败时返回错误；调用方必须 fail-closed，不得降级为空 token。
func generateAPIToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// GetAPIToken 供前端读取后端 API Token，用于在桌面模式下访问敏感接口（M5）。
func (a *App) GetAPIToken() string {
	return a.apiToken
}

// ApiResponse 前端经桌面壳代理后端 API 的响应。
// Headers 透传对前端有意义的响应头（下载文件名 Content-Disposition、分页 Link、
// 缓存校验 ETag/Last-Modified、Cache-Control 等）；逐跳头与已由 body 重新编码的
// 长度/编码头（Content-Length/Content-Encoding/...）已剔除，避免前端 Response 头不一致。
type ApiResponse struct {
	Status  int               `json:"status"`
	CType   string            `json:"ctype"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"` // base64 编码的响应体（兼容二进制导出）
}

// defaultProxyTimeoutMs 代理请求默认超时（与前端 DEFAULT_TIMEOUT_MS 对齐）；
// 前端会按各接口实际 timeoutMs 透传覆盖，仅在其未传或≤0 时生效。
const defaultProxyTimeoutMs = 30000

// apiClient 供 ApiRequest 转发使用。单次请求的超时由 context.WithTimeout 按前端
// 透传的 timeoutMs 控制（兜底 30s），此处不设长超时，避免后端挂起时桌面端卡死。
var apiClient = &http.Client{}

// ApiRequest 代理前端 API 请求到本地后端。
// 背景：Wails v2 macOS 生产 webview 使用 wails:// 自定义 scheme，fetch
// http://127.0.0.1:port 被 WebKit 拦截（报 "Load failed"，预检/请求都到不了后端）。
// 前端所有数据请求经由此绑定由壳进程原生 HTTP 转发，规避该限制（壳直连后端一直正常）。
// body 为 base64 编码的请求体（兼容 XML/JSON/FormData/二进制），contentType 透传前端设置。
// timeoutMs 由前端透传（默认 30s），经 context 控制单次请求超时，避免后端挂起时
// 桌面端 loading 无限卡死（此前固定 5 分钟超时，已移除）。
func (a *App) ApiRequest(method, path, body, contentType string, timeoutMs int) (ApiResponse, error) {
	a.backendMutex.Lock()
	port := a.goPort
	a.backendMutex.Unlock()
	if port == 0 {
		return ApiResponse{}, fmt.Errorf("backend not ready")
	}
	bodyBytes, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return ApiResponse{}, fmt.Errorf("invalid base64 body: %w", err)
	}
	u := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	// 超时优先取前端透传值，≤0 时回退默认；context 超时优先于 apiClient 的任何兜底。
	to := time.Duration(timeoutMs) * time.Millisecond
	if to <= 0 {
		to = defaultProxyTimeoutMs * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return ApiResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiToken)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return ApiResponse{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ApiResponse{}, err
	}
	// 透传对前端有意义的响应头；剔除逐跳头与已由 body 重新编码的长度/编码头。
	skipHeaders := map[string]bool{
		"Content-Length":    true,
		"Content-Encoding":  true,
		"Transfer-Encoding": true,
		"Connection":        true,
		"Trailer":           true,
		"Upgrade":           true,
	}
	headers := make(map[string]string, len(resp.Header))
	for k, vv := range resp.Header {
		if skipHeaders[k] || len(vv) == 0 {
			continue
		}
		headers[k] = vv[0]
	}
	return ApiResponse{
		Status:  resp.StatusCode,
		CType:   resp.Header.Get("Content-Type"),
		Headers: headers,
		Body:    base64.StdEncoding.EncodeToString(data),
	}, nil
}

// GetVersion 返回桌面壳版本号（构建时由 -ldflags 注入），供前端与更新器使用。
func (a *App) GetVersion() string {
	return version
}

// GetPlatform 返回当前操作系统标识（"windows"、"darwin"、"linux"），
// 供前端作平台适配渲染（如标题栏布局）。
func (a *App) GetPlatform() string {
	return runtime.GOOS
}

// CheckForUpdate 检查远端是否有可用更新，结果缓存到 cachedUpdate 供 StartUpdate 使用。
// 无更新返回 (nil, nil)；网络/解析失败返回 error。
func (a *App) context() context.Context {
	if p := a.ctx.Load(); p != nil {
		return *p
	}
	return nil
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx.Store(&ctx)
	a.logger.Printf("Wails startup")

	// 预置窗口行为默认值并标记缓存就绪：窗口控制路径（最小化/关闭/OnBeforeClose）
	// 从此永不发起同步 HTTP，杜绝在 Windows 主 UI 线程阻塞（M2）。
	a.settingsMutex.Lock()
	a.minimizeBehavior = "taskbar"
	a.closeBehavior = "quit"
	a.notifyEnabled = false
	a.settingsMutex.Unlock()

	// 设置窗口标题，覆盖 Wails 开发模式下的默认标题
	wailsRuntime.WindowSetTitle(ctx, "Flore")

	// 设置应用主菜单（中文标签），绕过 Wails v2 macOS 端角色菜单的英文硬编码
	a.setupAppMenu(ctx)

	// A1：诊断性强制居中/显示/取消最小化。若窗口因 macOS 窗口管理器/布局问题被创建到
	// 屏幕外或最小化，此操作可将其带回用户视野。30min 驻留但无窗口时，可据此确认
	// 窗口是否实际存在（若用户看到蓝色矩形，说明窗口存在，问题在前端渲染）。
	wailsRuntime.WindowCenter(ctx)
	wailsRuntime.WindowUnminimise(ctx)
	wailsRuntime.WindowShow(ctx)
	// 记录窗口状态，便于排查"只有 dock 图标"问题
	isMaximised := wailsRuntime.WindowIsMaximised(ctx)
	a.logger.Printf("window state: centred/unminimised/shown, isMaximised=%v", isMaximised)

	// 注册 Flore 到 Windows 注册表，使原生 Toast 显示应用名而非 wails.localhost
	a.registerNotificationApp()

	// 启动系统托盘
	a.startTray()

	// 异步启动本地后端服务，避免阻塞 UI（健康检查最长 15s）
	go a.startBackends()

	// 后台静默检查更新并缓存结果，使设置面板无需手动点击即可展示可用更新
	// （解决“关闭设置后更新状态丢失”的问题）。
	go a.backgroundCheckUpdate()

	// 若刚完成自动更新，提示用户
	if v := a.consumeUpdateMarker(); v != "" {
		_ = a.ShowNotification("Flore 已更新", "已更新到 v"+v)
	}

	// 启动后台通知监听：检测抓取完成并发送原生系统通知，覆盖托盘/调度盲区（M-A3）
	// 使用独立 cancel context，shutdown 时显式停止 goroutine 避免泄漏
	notifyCtx, notifyCancel := context.WithCancel(context.Background())
	a.notifyMutex.Lock()
	a.notifyCancel = notifyCancel
	a.notifyMutex.Unlock()
	go a.startNotifyWatcher(notifyCtx)
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	a.logger.Printf("Wails shutdown")
	// 停止通知监听 goroutine，避免泄漏
	a.notifyMutex.Lock()
	cancel := a.notifyCancel
	a.notifyCancel = nil
	a.notifyMutex.Unlock()
	if cancel != nil {
		cancel()
	}
	a.stopTray()
	a.stopBackends()
	a.logger.Close()
}

// startBackends 启动 Go 后端，并使用动态高位端口避免冲突。
