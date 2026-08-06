package main

import (
	"desktop/internal/updater"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) CheckForUpdate() (*updater.UpdateInfo, error) {
	info, err := updater.CheckForUpdate(version)
	if err != nil {
		return nil, err
	}
	a.updateMu.Lock()
	a.cachedUpdate = info
	a.updateMu.Unlock()
	return info, nil
}

// StartUpdate 应用已缓存的更新：准备替换文件并退出当前进程，
// 由外部脚本在进程释放后完成文件覆盖并重启。下载进度经回调写入 updateProgress，
// 供前端 GetUpdateProgress 轮询展示；全部下载源失败时返回错误（由前端提示手动下载）。
func (a *App) StartUpdate() error {
	a.updateMu.Lock()
	info := a.cachedUpdate
	a.updateMu.Unlock()
	if info == nil {
		return fmt.Errorf("当前没有可应用的更新")
	}
	a.updateProgress.Store(math.Float64bits(0))
	a.writeUpdateMarker(info.LatestVersion)
	if err := updater.ApplyUpdate(info, func(p float64) {
		a.updateProgress.Store(math.Float64bits(p))
	}); err != nil {
		// 应用失败清理进度与标记，避免下次启动误报已更新
		a.updateProgress.Store(math.Float64bits(0))
		a.removeUpdateMarker()
		return err
	}
	// 下载完成，进度置满；后续文件替换由外部脚本在进程退出后完成
	a.updateProgress.Store(math.Float64bits(1))
	// 强制退出（忽略 tray/quit 行为），让外部脚本接管文件替换
	go func() {
		a.forceQuit.Store(true)
		if ctx := a.context(); ctx != nil {
			wailsRuntime.Quit(ctx)
		} else {
			os.Exit(0)
		}
	}()
	return nil
}

// GetCachedUpdate 返回最近一次检查（后台或手动）的更新结果；无则 nil。
// 设置面板挂载时调用，使后台检查结果无需手动点击即可展示。
func (a *App) GetCachedUpdate() *updater.UpdateInfo {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.cachedUpdate
}

// GetUpdateProgress 返回当前更新下载进度（0~1）；无进行中为 0。
func (a *App) GetUpdateProgress() float64 {
	return math.Float64frombits(a.updateProgress.Load())
}

// backgroundCheckUpdate 在应用启动后于后台静默检查更新，结果写入缓存，
// 使设置面板无需手动点击即可展示可用更新（解决“关闭设置后更新状态丢失”）。
func (a *App) backgroundCheckUpdate() {
	info, err := updater.CheckForUpdate(version)
	if err != nil {
		a.logger.Printf("[update] 后台检查更新失败: %v", err)
		return
	}
	a.updateMu.Lock()
	a.cachedUpdate = info
	a.updateMu.Unlock()
	a.logger.Printf("[update] 后台检查更新完成: hasUpdate=%v", info != nil)
}

const updateMarkerName = "updated.flag"

// writeUpdateMarker 写入"已更新"标记，供重启后的实例提示用户。
func (a *App) writeUpdateMarker(newVersion string) {
	dir := a.appDataDir()
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, updateMarkerName), []byte(newVersion), 0600)
}

// removeUpdateMarker 删除"已更新"标记。
func (a *App) removeUpdateMarker() {
	_ = os.Remove(filepath.Join(a.appDataDir(), updateMarkerName))
}

// consumeUpdateMarker 读取并删除"已更新"标记，返回新版本号（无则空串）。
func (a *App) consumeUpdateMarker() string {
	path := filepath.Join(a.appDataDir(), updateMarkerName)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	_ = os.Remove(path)
	return strings.TrimSpace(string(data))
}

// context 原子读取 Wails 上下文，未就绪时返回 nil（M7）。
