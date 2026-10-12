package main

import (
	"desktop/internal/updatelet"
	"desktop/internal/updater"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// updateStateEvent 是后端推送给前端的事件名：内容为 updatelet.State 对象。
const updateStateEvent = "flore:update:state"

// updateStates 是更新流程状态机宿主：检查/下载由后台 goroutine 驱动，
// 状态集中于单一快照，与设置弹窗生命周期解耦。关闭设置弹窗不中止下载，
// 前端挂载时通过 GetUpdateSnapshot 一次性复原展示状态。
var updateStates *updatelet.Updater

// initUpdateHost 在 App 构造时初始化更新状态机。状态变更时写日志并推送给前端
// （Wails 事件）；context 未就绪时静默跳过推送，前端会在挂载时拉取快照补齐。
func initUpdateHost(app *App) {
	updateStates = updatelet.New(version, func(st updatelet.State) {
		app.logger.Printf("[update] state=%s progress=%.2f error=%q", st.Status, st.Progress, st.Error)
		ctx := app.context()
		if ctx == nil {
			return
		}
		wailsRuntime.EventsEmit(ctx, updateStateEvent, st)
	})
}

// GetUpdateSnapshot 返回更新流程当前快照，供前端挂载时复原展示状态。
func (a *App) GetUpdateSnapshot() updatelet.State {
	if updateStates == nil {
		return updatelet.State{Status: updatelet.StatusIdle}
	}
	return updateStates.Snapshot()
}

// CheckForUpdate 后台检查远端是否有可用更新（非阻塞），结果实时写入状态机快照。
// 无更新时状态回落 idle；网络/解析失败时状态转为 error。
func (a *App) CheckForUpdate() error {
	if updateStates == nil {
		return fmt.Errorf("更新功能未初始化")
	}
	return updateStates.Check()
}

// StartUpdate 后台下载已确认的更新（非阻塞），下载完成前即返回。
// 下载进度经状态机推送给前端；进程重启改由 ConfirmRestart 在下载完成后触发，
// 不再在下载中退出进程。
func (a *App) StartUpdate() error {
	if updateStates == nil {
		return fmt.Errorf("更新功能未初始化")
	}
	return updateStates.Download()
}

// ConfirmRestart 在下载完成（ready）后由前端点击确认重启时调用：状态置为
// pendingRestart 并立即退出进程，交由外部脚本完成文件替换与重启。
func (a *App) ConfirmRestart() error {
	if updateStates == nil {
		return fmt.Errorf("更新功能未初始化")
	}
	snapshot := updateStates.Snapshot()
	if snapshot.Status != updatelet.StatusReady {
		return fmt.Errorf("当前没有已下载完成的更新")
	}
	// 写入"已更新"标记，供重启后的实例提示用户
	if snapshot.Info != nil {
		a.writeUpdateMarker(snapshot.Info.LatestVersion)
	}
	updateStates.ApplyReady()
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

// GetUpdateProgress 返回当前下载进度（0~1）；非下载态为 0。
func (a *App) GetUpdateProgress() float64 {
	if updateStates == nil {
		return 0
	}
	return updateStates.Snapshot().Progress
}

// GetCachedUpdate 保留兼容：返回最近一次有效更新信息，无则 nil。
func (a *App) GetCachedUpdate() *updater.UpdateInfo {
	if updateStates == nil {
		return nil
	}
	return updateStates.Snapshot().Info
}

const updateMarkerName = "updated.flag"

// writeUpdateMarker 写入"已更新"标记，供重启后的实例提示用户。
func (a *App) writeUpdateMarker(newVersion string) {
	dir := a.appDataDir()
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, updateMarkerName), []byte(newVersion), 0600)
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
