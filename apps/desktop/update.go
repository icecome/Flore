package main

import (
	"desktop/internal/updater"
	"fmt"
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
// 由外部脚本在进程释放后完成文件覆盖并重启。
func (a *App) StartUpdate() error {
	a.updateMu.Lock()
	info := a.cachedUpdate
	a.updateMu.Unlock()
	if info == nil {
		return fmt.Errorf("当前没有可应用的更新")
	}
	a.writeUpdateMarker(info.LatestVersion)
	if err := updater.ApplyUpdate(info); err != nil {
		// 应用失败清理标记，避免下次启动误报已更新
		a.removeUpdateMarker()
		return err
	}
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
