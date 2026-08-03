package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (a *App) startBackends() {
	dbPath := a.readerDatabasePath()
	a.logger.Printf("database path: %s", dbPath)

	goExe := a.findGoBackend()
	a.logger.Printf("found go backend: %s", goExe)
	if goExe == "" {
		a.logger.Printf("go backend skipped: executable not found")
		return
	}

	// 不再探测空闲端口（findFreePort 存在"探测后交给后端绑定"的 TOCTOU 窗口）。
	// 改为后端自行绑定 PORT=0（系统分配），并通过 FLORE_PORT_FILE 回报实际端口，
	// 消除端口被抢占导致后端绑定失败的竞态。
	portFile := filepath.Join(os.TempDir(), fmt.Sprintf("flore-port-%d.txt", os.Getpid()))
	_ = os.Remove(portFile)

	cmd := a.startProcess("go-backend", goExe, []string{}, map[string]string{
		"PORT":            "0",
		"DATABASE_URL":    dbPath,
		"FLORE_LOG_FILE":  filepath.Join(filepath.Dir(dbPath), "florebackend.log"),
		"FLORE_PORT_FILE": portFile,
		// 本地敏感接口鉴权 token，避免同机任意进程直接删除订阅源/导出数据库（M5）
		"FLORE_API_TOKEN": a.apiToken,
		// 不设置 CORS_ORIGINS：后端默认走 AllowOriginFunc 动态反射本地源，
		// 放行 127.0.0.1/localhost 动态端口与 Wails WebView 源，
		// 拒绝任意外网源与 opaque origin。Web 部署时手动设置 CORS_ORIGINS=* 或具体域名放开。
	})

	// startProcess 失败（返回 nil）时不记录 port，避免后续 getSetting 向未监听端口发起请求触发超时
	if cmd == nil {
		return
	}

	exited := make(chan struct{})

	// 等待后端写入端口文件（最多 15 秒），拿到实际监听端口
	port := a.waitForPortFile(portFile, exited)
	if port == 0 {
		a.logger.Printf("[go-backend] failed to obtain backend port from %s", portFile)
		return
	}

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
			// 等待退出，最多再 3 秒；若 Kill 失败且 exited 未关闭，
			// 也必须继续退出流程，否则桌面端关闭会在此永久阻塞（死锁）。
			select {
			case <-exited:
			case <-time.After(3000 * time.Millisecond):
				a.logger.Printf("[go-backend] process did not exit after kill, continuing shutdown")
			}
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

// waitForPortFile 轮询等待后端写入的端口文件，返回实际监听端口。
// 后端绑定 PORT=0（系统分配端口）后写入 FLORE_PORT_FILE，消除了
// "桌面壳探测空闲端口再交给后端绑定"的 TOCTOU 窗口。
// 后端进程提前退出（exited 关闭）或超时（10 秒）时返回 0。
func (a *App) waitForPortFile(portFile string, exited <-chan struct{}) int {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if data, err := os.ReadFile(portFile); err == nil {
			if p, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && p > 0 {
				return p
			}
		}
		select {
		case <-exited:
			a.logger.Printf("[go-backend] process exited before writing port file")
			return 0
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			a.logger.Printf("[go-backend] timed out waiting for port file %s", portFile)
			return 0
		}
	}
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
	targetName := "florebackend" + ext

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

// appDataDir 返回应用数据目录（data/），存放数据库、日志与窗口状态等本地产物；
// WebView2 缓存与备份目录由 auxRoot 单独管理，与 data/ 同级，不污染 data/。
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
// 若 data/ 不存在则尝试创建（首次启动时自动建立），创建失败则回退到用户数据目录。
func (a *App) findPortableDataDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	dataDir := filepath.Join(filepath.Dir(exePath), "data")
	info, statErr := os.Stat(dataDir)
	if statErr == nil && info.IsDir() {
		return dataDir
	}
	// 目录不存在，尝试创建（安装版/便携版首次启动时自动建立）
	if os.IsNotExist(statErr) {
		if mkErr := os.MkdirAll(dataDir, 0o755); mkErr == nil {
			return dataDir
		}
		// 创建失败（如 Program Files 只读目录），静默回退到用户数据目录
	}
	return ""
}

// auxRoot 返回辅助目录（webview2/、backups/）的根。
// 便携模式：data/ 的父目录即 exe 目录，辅助目录与 data/ 同级，随包迁移；
// 安装模式：沿用用户数据目录（可写，避免写入 Program Files）。
func (a *App) auxRoot() string {
	dir := a.appDataDir()
	if filepath.Base(dir) == "data" {
		return filepath.Dir(dir)
	}
	return dir
}

// webviewDataPath 返回 WebView2 用户数据目录，统一收归到辅助目录下，
// 便携模式位于 exe 同级的 webview2/，安装模式位于用户数据目录，
// 避免 Wails 默认把缓存散落到 %APPDATA%\[BinaryName.exe]，也不污染 data/。
// WebView2 对无效路径会直接弹窗并报错退出，这里提前确保目录存在。
func (a *App) webviewDataPath() string {
	dir := filepath.Join(a.auxRoot(), "webview2")
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
