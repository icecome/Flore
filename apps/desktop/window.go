package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

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
	return filepath.Join(a.appDataDir(), "windowstate.json")
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

	tmp, err := os.CreateTemp(dir, "windowstate*.tmp")
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
