package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
