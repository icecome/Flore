package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// restartWaitPidEnv 由 RestartApp 注入新实例，新实例启动前先等待旧实例退出，
// 以便单实例互斥体被释放（见 main.go）。
const restartWaitPidEnv = "FLORE_RESTART_WAIT_PID"

// backendPathEnv 允许开发态用绝对路径显式指定后端二进制，
// 替代此前从 CWD 解析相对路径的不安全查找（M4）。
const backendPathEnv = "FLORE_BACKEND_PATH"

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
	logPath := filepath.Join(dir, "flore-desktop.log")

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
	a.apiToken = generateAPIToken()
	return a
}

// generateAPIToken 生成 32 字节随机 token，用于后端敏感接口鉴权（M5）。
// crypto/rand 失败时返回空串，此时后端等价于旧行为（不鉴权），不阻断启动。
func generateAPIToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

// GetAPIToken 供前端读取后端 API Token，用于在桌面模式下访问敏感接口（M5）。
func (a *App) GetAPIToken() string {
	return a.apiToken
}

// context 原子读取 Wails 上下文，未就绪时返回 nil（M7）。
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
		"PORT":           fmt.Sprintf("%d", port),
		"DATABASE_URL":   dbPath,
		"FLORE_LOG_FILE": filepath.Join(filepath.Dir(dbPath), "flore-backend.log"),
		// 本地敏感接口鉴权 token，避免同机任意进程直接删除订阅源/导出数据库（M5）
		"FLORE_API_TOKEN": a.apiToken,
		// 桌面端前端 origin 可能是 Wails dev server 或 wails.localhost，
		// 必须全部加入 CORS 白名单，否则前端 fetch 会被拒绝。
		// 注意：gin-contrib/cors 要求 origin 必须以 http:// 或 https:// 开头，
		// 不能使用 wails:// scheme。
		"CORS_ORIGINS": fmt.Sprintf(
			"http://127.0.0.1:%d,http://localhost:%d,http://localhost:34115,http://wails.localhost,https://wails.localhost",
			port, port,
		),
	})

	// startProcess 失败（返回 nil）时不记录 port，避免后续 getSetting 向未监听端口发起请求触发超时
	if cmd == nil {
		return
	}

	exited := make(chan struct{})

	a.backendMutex.Lock()
	// M1：启动期间可能已进入退出流程，此时立即回收刚拉起的进程，避免孤儿。
	if a.backendStopping {
		a.backendMutex.Unlock()
		a.logger.Printf("[go-backend] shutdown already in progress, killing freshly started process")
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return
	}
	a.goCmd = cmd
	a.goPort = port
	a.goExited = exited
	a.stopOnce = &sync.Once{}
	a.backendMutex.Unlock()

	// M9：唯一的 cmd.Wait() 调用点，统一回收并广播退出事件。
	go a.monitorBackend(cmd, exited)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	started := a.waitForBackend(ctx, "go-backend", port, exited)

	a.backendMutex.Lock()
	// 后端在健康检查期间可能已被 stopBackends 替换/清空，或已进入退出流程
	stale := a.goCmd != cmd || a.backendStopping
	if !stale {
		a.goStarted = started
	}
	a.backendMutex.Unlock()

	if started && !stale {
		a.logger.Printf("Go backend ready at http://127.0.0.1:%d", port)
		// 后端就绪后加载窗口行为设置缓存
		a.loadWindowSettings()
	}
}

// monitorBackend 是后端子进程唯一的 wait 回收点：进程退出后关闭 exited 通道，
// 并在非主动退出场景下记录崩溃、清除 goStarted，避免前端一直连到已死端口（M9）。
func (a *App) monitorBackend(cmd *exec.Cmd, exited chan struct{}) {
	err := cmd.Wait()
	close(exited)

	a.backendMutex.Lock()
	current := a.goCmd == cmd
	stopping := a.backendStopping
	if current {
		a.goStarted = false
	}
	a.backendMutex.Unlock()

	if !current {
		return
	}
	if stopping {
		a.logger.Printf("[go-backend] process exited during shutdown (err=%v)", err)
		return
	}
	a.logger.Printf("[go-backend] process exited unexpectedly (err=%v), backend marked as down", err)
}

// stopBackends 停止已启动的后端进程。
// 优先 POST /api/shutdown 触发后端优雅关闭，超时后再 Kill。
func (a *App) stopBackends() {
	a.backendMutex.Lock()
	// M1：先置位停止标志，让并发中的 startBackends 自行回收。
	a.backendStopping = true
	cmd := a.goCmd
	port := a.goPort
	started := a.goStarted
	exited := a.goExited
	once := a.stopOnce
	a.backendMutex.Unlock()

	if cmd == nil || cmd.Process == nil || exited == nil {
		a.resetBackendState()
		return
	}
	if once == nil {
		once = &sync.Once{}
	}

	// N6：整个停止流程只执行一次
	once.Do(func() {
		// 先尝试 POST /api/shutdown 触发后端自身优雅关闭（含 WAL checkpoint 与连接释放）
		if started {
			a.requestGracefulShutdown(port)
		}

		// 等待进程退出，最多 2.5 秒
		select {
		case <-exited:
			a.logger.Printf("[go-backend] process exited gracefully")
		case <-time.After(2500 * time.Millisecond):
			if err := cmd.Process.Kill(); err != nil {
				a.logger.Printf("[go-backend] failed to kill process: %v", err)
			} else {
				a.logger.Printf("[go-backend] process killed after timeout")
			}
			<-exited
		}
	})

	a.resetBackendState()
}

// resetBackendState 清空后端相关字段，避免退出后残留陈旧句柄/端口（N6）。
func (a *App) resetBackendState() {
	a.backendMutex.Lock()
	a.goCmd = nil
	a.goPort = 0
	a.goStarted = false
	a.goExited = nil
	a.stopOnce = nil
	a.backendMutex.Unlock()
}

// requestGracefulShutdown 向后端发送优雅关闭请求（fire-and-forget）。
func (a *App) requestGracefulShutdown(port int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/shutdown", port)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		a.logger.Printf("[go-backend] build shutdown request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	a.authorize(req)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		a.logger.Printf("[go-backend] graceful shutdown request failed: %v", err)
		return
	}
	drainAndClose(resp)
}

// startProcess 启动一个子进程并返回 exec.Cmd。
func (a *App) startProcess(name string, exe string, args []string, env map[string]string) *exec.Cmd {
	// M4：只允许执行绝对路径的二进制，杜绝从 CWD 解析相对路径带来的二进制植入。
	if !filepath.IsAbs(exe) {
		a.logger.Printf("[%s] refused to start non-absolute executable path: %q", name, exe)
		return nil
	}

	cmd := exec.Command(exe, args...)
	// M3：日志不可用时保持 nil（exec 会自动挂到 os.DevNull），
	// 绝不能赋值一个 typed-nil *os.File。
	if w := a.logger.writer(); w != nil {
		cmd.Stdout = w
		cmd.Stderr = w
	}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.SysProcAttr = hiddenSysProcAttr()

	if err := cmd.Start(); err != nil {
		a.logger.Printf("[%s] failed to start %s: %v", name, exe, err)
		return nil
	}

	// C2：绑定 Job Object，主进程崩溃/强杀时由内核连带终止子进程，杜绝孤儿后端。
	if err := assignToJob(cmd.Process); err != nil {
		a.logger.Printf("[%s] warning: failed to assign process to job object: %v", name, err)
	}

	// N1：日志中掩码敏感 env（尤其是 FLORE_API_TOKEN），避免 token 落盘。
	a.logger.Printf("[%s] started %s (pid=%d) env=%s", name, exe, cmd.Process.Pid, maskEnv(env))
	return cmd
}

// maskEnv 将敏感环境变量值替换为掩码后再输出到日志（N1）。
func maskEnv(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	// 排序保证日志输出稳定可读
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("map[")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(k)
		b.WriteString(":")
		if isSensitiveEnvKey(k) {
			b.WriteString("***")
		} else {
			b.WriteString(env[k])
		}
	}
	b.WriteString("]")
	return b.String()
}

func isSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "APIKEY", "API_KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// waitForBackend 轮询后端健康检查接口，直到服务就绪、进程退出或 ctx 取消。
func (a *App) waitForBackend(ctx context.Context, name string, port int, exited <-chan struct{}) bool {
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		// 请求绑定 ctx，保证单次探测不会超出整体超时预算
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return false
		}
		resp, err := healthClient.Do(req)
		if err == nil {
			ok := resp.StatusCode == http.StatusOK
			drainAndClose(resp)
			if ok {
				a.logger.Printf("[%s] health check passed on port %d", name, port)
				return true
			}
		}
		select {
		case <-ctx.Done():
			a.logger.Printf("[%s] health check timed out on port %d", name, port)
			return false
		case <-exited:
			a.logger.Printf("[%s] process exited before becoming healthy", name)
			return false
		case <-ticker.C:
		}
	}
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
// 只信任两个来源（M4）：
//  1. 环境变量 FLORE_BACKEND_PATH 指定的绝对路径（开发态显式指定）
//  2. 可执行文件所在目录及其 build/bin 子目录（生产/便携部署）
//
// 已彻底移除基于当前工作目录的相对路径候选：CWD 由启动方控制，
// 攻击者可在任意可写目录放置同名二进制诱导执行（条件性 RCE）。
func (a *App) findGoBackend() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	targetName := "flore-backend" + ext

	if env := os.Getenv(backendPathEnv); env != "" {
		if !filepath.IsAbs(env) {
			a.logger.Printf("%s must be an absolute path, ignored: %q", backendPathEnv, env)
		} else if _, err := os.Stat(env); err != nil {
			a.logger.Printf("%s points to a missing file, ignored: %q", backendPathEnv, env)
		} else {
			return env
		}
	}

	return a.findBackendByExecutable(targetName)
}

// findBackendByExecutable 从可执行文件所在目录向上遍历（最多 3 层），查找后端可执行文件。
// 限制查找深度避免在高层目录意外命中同名不可信文件。
func (a *App) findBackendByExecutable(targetName string) string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	exePath, err = filepath.Abs(exePath)
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
// 拒绝时返回 error，便于前端提示用户（T2）。
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
func (a *App) getPort() int {
	a.backendMutex.Lock()
	defer a.backendMutex.Unlock()
	return a.goPort
}

// getNotifyEnabled 读取通知开关（纯缓存，永不同步 HTTP）。
func (a *App) getNotifyEnabled() bool {
	a.settingsMutex.Lock()
	defer a.settingsMutex.Unlock()
	return a.notifyEnabled
}

// fetchSettingValue 从后端获取单个设置项的值，成功时返回 (value, true)。
func (a *App) fetchSettingValue(key string, port int) (string, bool) {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/settings/%s", port, key)
	resp, err := a.doRequest(settingsClient, http.MethodGet, url, nil)
	if err != nil {
		return "", false
	}
	defer drainAndClose(resp)
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

// loadWindowSettings 从后端加载窗口行为设置到缓存。
// 始终在后台 goroutine 中调用；UI 线程只读缓存（M2）。
// settingsClient 超时极短（300ms），因此这里做有限重试，
// 且只在取到值时才覆盖缓存——一次网络抖动不应把用户的 tray 配置悄悄退回默认值。
func (a *App) loadWindowSettings() {
	port := a.getPort()
	if port == 0 {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		minimize, okMinimize := a.fetchSettingValue("minimizeBehavior", port)
		closeBeh, okClose := a.fetchSettingValue("closeBehavior", port)
		notify, okNotify := a.fetchSettingValue("notifyEnabled", port)
		if !okMinimize && !okClose && !okNotify {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		a.settingsMutex.Lock()
		if okMinimize {
			a.minimizeBehavior = minimize
		}
		if okClose {
			a.closeBehavior = closeBeh
		}
		if okNotify {
			a.notifyEnabled = notify == "true"
		}
		a.settingsMutex.Unlock()
		return
	}
	a.logger.Printf("loadWindowSettings: no settings fetched, keeping current cache")
}

// getMinimizeBehavior 读取缓存的最小化行为（纯缓存，永不同步 HTTP）。
func (a *App) getMinimizeBehavior() string {
	a.settingsMutex.Lock()
	defer a.settingsMutex.Unlock()
	return a.minimizeBehavior
}

// getCloseBehavior 读取缓存的关闭行为（纯缓存，永不同步 HTTP）。
func (a *App) getCloseBehavior() string {
	a.settingsMutex.Lock()
	defer a.settingsMutex.Unlock()
	return a.closeBehavior
}

// RefreshWindowSettings 供前端在修改窗口行为设置后调用，刷新桌面壳缓存。
func (a *App) RefreshWindowSettings() {
	go a.loadWindowSettings()
}

// RestartApp 重启桌面应用。
// 由于已启用单实例锁（C3），不能再「先起新实例再退出」——新实例会被互斥体挡掉并自杀。
// 这里改为：新实例通过 FLORE_RESTART_WAIT_PID 得知旧实例 PID，
// 在 wails.Run 之前阻塞等待旧实例完全退出（互斥体随之释放）后再继续启动。
// 同时用 CREATE_BREAKAWAY_FROM_JOB 让新实例脱离当前 Job Object，
// 避免旧实例退出关闭 Job 时把新实例一并杀掉（C2）。
func (a *App) RestartApp() error {
	ctx := a.context()
	if ctx == nil {
		return fmt.Errorf("app context not ready")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve executable: %w", err)
	}

	// N5：透传原始命令行参数，避免重启后丢失用户传入的开关
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", restartWaitPidEnv, os.Getpid()))
	cmd.SysProcAttr = detachedSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new instance: %w", err)
	}
	// 立即 Release，避免留下僵尸子进程句柄
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	go func() {
		a.logger.Printf("restarting app, exiting current instance")
		a.forceQuit.Store(true)
		wailsRuntime.Quit(ctx)
	}()
	return nil
}

// WindowMinimise 最小化窗口。
// 根据用户配置的 minimizeBehavior 决定行为：
// - "tray"：隐藏窗口（后台运行，需系统托盘支持）
// - "taskbar"（默认）：正常最小化到任务栏
func (a *App) WindowMinimise() {
	ctx := a.context()
	if ctx == nil {
		return
	}
	if a.trayAvailable() && a.getMinimizeBehavior() == "tray" {
		wailsRuntime.WindowHide(ctx)
		return
	}
	wailsRuntime.WindowMinimise(ctx)
}

// WindowMaximise 最大化窗口。
func (a *App) WindowMaximise() {
	if ctx := a.context(); ctx != nil {
		wailsRuntime.WindowMaximise(ctx)
	}
}

// WindowUnmaximise 还原窗口。
func (a *App) WindowUnmaximise() {
	if ctx := a.context(); ctx != nil {
		wailsRuntime.WindowUnmaximise(ctx)
	}
}

// WindowToggleMaximise 切换窗口最大化/还原状态。
func (a *App) WindowToggleMaximise() {
	if ctx := a.context(); ctx != nil {
		wailsRuntime.WindowToggleMaximise(ctx)
	}
}

// WindowIsMaximised 返回窗口是否已最大化。
func (a *App) WindowIsMaximised() bool {
	ctx := a.context()
	if ctx == nil {
		return false
	}
	return wailsRuntime.WindowIsMaximised(ctx)
}

// GetWindowState 返回持久化的窗口状态（供前端启动时同步图标使用）。
func (a *App) GetWindowState() WindowState {
	return a.LoadWindowState()
}

// WindowClose 关闭应用程序窗口。
// 根据用户配置的 closeBehavior 决定行为：
// - "tray"：隐藏窗口（后台运行，需系统托盘支持）
// - "quit"（默认）：退出应用
func (a *App) WindowClose() {
	ctx := a.context()
	if ctx == nil {
		return
	}
	// C1 兜底：只有托盘确实可用时才允许隐藏，否则直接退出，
	// 避免用户在托盘不可用时被锁死在「窗口已隐藏且无任何入口」的状态。
	if a.getCloseBehavior() == "tray" {
		if a.trayAvailable() {
			wailsRuntime.WindowHide(ctx)
			return
		}
		a.logger.Printf("closeBehavior=tray but tray is unavailable, quitting instead")
	}
	wailsRuntime.Quit(ctx)
}

// ShouldPreventClose 供 main.go 的 OnBeforeClose 回调使用。
// 当 closeBehavior 为 "tray" 且系统托盘支持时阻止窗口关闭并隐藏，实现后台驻留。
// 若 forceQuit 为 true（由托盘菜单"退出"触发），则跳过阻止，直接退出。
// 注意：本方法运行在 Windows 主 UI 线程，只允许读缓存，禁止任何同步 HTTP（M2）。
func (a *App) ShouldPreventClose() bool {
	if a.forceQuit.Load() {
		return false
	}
	if a.getCloseBehavior() != "tray" {
		return false
	}
	// C1 兜底：托盘不可用时不得阻止关闭，否则应用无法退出。
	if !a.trayAvailable() {
		a.logger.Printf("closeBehavior=tray but tray is unavailable, allowing close")
		return false
	}
	if ctx := a.context(); ctx != nil {
		wailsRuntime.WindowHide(ctx)
	}
	return true
}

// ShowWindow 显示主窗口（用于前端在需要时恢复隐藏的窗口）。
func (a *App) ShowWindow() {
	if ctx := a.context(); ctx != nil {
		wailsRuntime.WindowShow(ctx)
	}
}

// WindowState 保存的窗口状态
type WindowState struct {
	Maximised bool `json:"maximised"`
}

// windowStateFilePath 返回窗口状态文件路径。
func (a *App) windowStateFilePath() string {
	return filepath.Join(a.appDataDir(), "window-state.json")
}

// SaveWindowState 保存窗口状态到本地文件（供前端调用）。
// 采用「临时文件 + rename」原子写，避免退出时断电/强杀留下截断的 JSON（N3）。
func (a *App) SaveWindowState(maximised bool) {
	state := WindowState{Maximised: maximised}
	data, err := json.Marshal(state)
	if err != nil {
		a.logger.Printf("SaveWindowState: failed to marshal: %v", err)
		return
	}

	target := a.windowStateFilePath()
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0700); err != nil {
		a.logger.Printf("SaveWindowState: failed to create dir: %v", err)
		return
	}

	tmp, err := os.CreateTemp(dir, "window-state-*.tmp")
	if err != nil {
		a.logger.Printf("SaveWindowState: failed to create temp file: %v", err)
		return
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		a.logger.Printf("SaveWindowState: failed to write temp file: %v", err)
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		a.logger.Printf("SaveWindowState: failed to sync temp file: %v", err)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		a.logger.Printf("SaveWindowState: failed to close temp file: %v", err)
		return
	}
	// os.Rename 在 Windows 上使用 MoveFileEx(MOVEFILE_REPLACE_EXISTING)，
	// 可原子覆盖已存在文件，无需先删除目标。
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		a.logger.Printf("SaveWindowState: failed to rename temp file: %v", err)
	}
}

// LoadWindowState 从本地文件加载窗口状态
func (a *App) LoadWindowState() WindowState {
	var state WindowState
	state.Maximised = true // 默认最大化
	data, err := os.ReadFile(a.windowStateFilePath())
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state
	}
	return state
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
	if a.context() == nil {
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
		url := fmt.Sprintf("http://127.0.0.1:%d/api/sources/fetch-status", port)
		resp, err := a.doRequest(httpClient, http.MethodGet, url, nil)
		if err != nil {
			lastFetching = false
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		drainAndClose(resp)
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
