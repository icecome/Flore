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
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	// updateInFlight 防止 StartUpdate 并发调用：
	// 关闭再开设置弹窗后，前端 mount 时会通过 GetUpdateProgress 同步"正在下载"状态，
	// 正常路径不应发生重入；此处再次点击属于异常路径，直接拒绝以避免两份下载
	// 抢同一个 temp 文件 / 进程被 Quit 中断，进而触发"下载失败"。
	updateInFlight atomic.Bool

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
	// 安全校验：path 必须为站内绝对路径，且解析后 host 必须始终指向本机后端，防 @host 混淆外发与 token 泄露。
	if !strings.HasPrefix(path, "/") {
		return ApiResponse{}, fmt.Errorf("invalid api path: must start with /")
	}
	u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return ApiResponse{}, fmt.Errorf("invalid api path: %w", err)
	}
	if u.Hostname() != "127.0.0.1" || u.Scheme != "http" {
		return ApiResponse{}, fmt.Errorf("invalid api path: host must be 127.0.0.1")
	}
	// 超时优先取前端透传值，≤0 时回退默认；context 超时优先于 apiClient 的任何兜底。
	to := time.Duration(timeoutMs) * time.Millisecond
	if to <= 0 {
		to = defaultProxyTimeoutMs * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(bodyBytes))
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
