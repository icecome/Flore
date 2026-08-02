package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	systray "github.com/energye/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/favicon.ico
var trayIconData []byte

// 托盘状态由 trayMutex 保护：startTray 在主 UI 线程调用，
// stopTray / trayAvailable 可能来自其它 goroutine（M7）。
var (
	trayMutex sync.Mutex
	trayReady bool
	trayEnded bool
)

// startTray 启动系统托盘。
//
// M8：Win32 的 GetMessage 只能取到「由调用线程创建的窗口」的消息，
// 因此托盘窗口的创建与消息泵必须固定在同一个 OS 线程上。
// energye/systray 的 RunWithExternalLoop 会把消息泵放进一个未锁定的 goroutine，
// 与创建窗口的线程无亲和关系，导致点击/菜单命令偶发失效。
// 这里改为开专属 goroutine + runtime.LockOSThread() + 阻塞式 systray.Run，
// 保证 registerSystray（建窗）与 nativeLoop（消息泵）在同一线程。
func (a *App) startTray() {
	a.logger.Printf("[systray] startTray called, icon data size: %d bytes", len(trayIconData))

	readyCh := make(chan struct{})
	var readyOnce sync.Once

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		systray.Run(func() {
			a.logger.Printf("[systray] onReady called")
			systray.SetIcon(trayIconData)

			a.buildTrayMenu()

			trayMutex.Lock()
			trayReady = true
			trayMutex.Unlock()

			readyOnce.Do(func() { close(readyCh) })
			a.logger.Printf("[systray] menu items created successfully")
		}, func() {
			a.logger.Printf("[systray] onExit called")
			trayMutex.Lock()
			trayReady = false
			trayMutex.Unlock()
			readyOnce.Do(func() { close(readyCh) })
		})

		// systray.Run 返回意味着消息泵已结束（托盘不再可用）
		trayMutex.Lock()
		trayReady = false
		trayMutex.Unlock()
		readyOnce.Do(func() { close(readyCh) })
		a.logger.Printf("[systray] message loop exited")
	}()

	// 等待托盘就绪（上限 2 秒；正常情况只需几毫秒）。
	// 超时说明托盘注册失败：trayAvailable() 保持 false，
	// 窗口关闭路径退化为直接退出，不会把用户锁死在无退出入口的状态（C1 兜底）。
	select {
	case <-readyCh:
	case <-time.After(2 * time.Second):
		a.logger.Printf("[systray] tray not ready within 2s, continuing without tray")
	}
	a.logger.Printf("[systray] startTray finished, available=%v", a.trayAvailable())
}

// buildTrayMenu 创建托盘菜单项与点击回调。
func (a *App) buildTrayMenu() {
	// 左键单击 → 显示主窗口
	systray.SetOnClick(func(menu systray.IMenu) {
		a.logger.Printf("[systray] OnClick triggered")
		a.ShowWindow()
	})

	// 右键单击 → 显示托盘菜单
	// C1：energye/systray 约定「一旦设置 OnRClick 回调，库不再自动弹菜单，
	// 必须由回调自己调用 menu.ShowMenu()」。缺失该调用会导致右键菜单永不弹出，
	// closeBehavior=tray 时用户无任何退出入口。
	systray.SetOnRClick(func(menu systray.IMenu) {
		a.logger.Printf("[systray] OnRightClick triggered")
		if menu == nil {
			a.logger.Printf("[systray] OnRightClick: menu is nil, cannot show menu")
			return
		}
		if err := menu.ShowMenu(); err != nil {
			a.logger.Printf("[systray] ShowMenu failed: %v", err)
		}
	})

	mShow := systray.AddMenuItem("显示主窗口", "")
	mShow.Click(func() {
		a.logger.Printf("[systray] menu: 显示主窗口 clicked")
		a.ShowWindow()
	})

	mHide := systray.AddMenuItem("最小化主窗口", "")
	mHide.Click(func() {
		a.logger.Printf("[systray] menu: 最小化主窗口 clicked")
		ctx := a.context()
		if ctx == nil {
			a.logger.Printf("[systray] menu: ctx is nil")
			return
		}
		wailsRuntime.WindowHide(ctx)
	})

	systray.AddSeparator()

	mFetch := systray.AddMenuItem("收取所有订阅源的文章", "")
	mFetch.Click(func() {
		a.logger.Printf("[systray] menu: fetch-all clicked")
		go a.fetchAllFromTray()
	})

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("退出", "")
	mQuit.Click(func() {
		a.logger.Printf("[systray] menu: quit clicked")
		a.forceQuit.Store(true)
		if ctx := a.context(); ctx != nil {
			wailsRuntime.Quit(ctx)
		}
	})
}

// stopTray 停止托盘并重置状态（T3）。
func (a *App) stopTray() {
	a.logger.Printf("[systray] stopTray called")
	trayMutex.Lock()
	already := trayEnded
	trayEnded = true
	trayReady = false
	trayMutex.Unlock()

	if already {
		return
	}
	// systray.Quit 内部有 quitOnce 保护，重复调用安全
	systray.Quit()
	a.logger.Printf("[systray] tray stopped")
}

// trayAvailable 返回托盘是否已就绪且未被停止。
func (a *App) trayAvailable() bool {
	trayMutex.Lock()
	defer trayMutex.Unlock()
	return trayReady && !trayEnded
}

func (a *App) fetchAllFromTray() {
	a.logger.Printf("[systray] fetchAllFromTray called")
	port := a.getPort()
	if port == 0 {
		a.logger.Printf("[systray] backend port is 0, skipping")
		return
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/sources/fetch-all", port)
	a.logger.Printf("[systray] fetching from %s", url)
	resp, err := a.doRequest(httpClient, http.MethodPost, url, nil)
	if err != nil {
		a.logger.Printf("[systray] fetch-all error: %v", err)
		return
	}
	status := resp.StatusCode
	// 排空并关闭，保持连接可复用（T4）
	drainAndClose(resp)
	if status != http.StatusOK {
		a.logger.Printf("[systray] fetch-all returned HTTP %d", status)
		return
	}
	a.logger.Printf("[systray] fetch-all triggered successfully")
}
