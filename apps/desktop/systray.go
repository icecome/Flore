package main

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	systray "github.com/energye/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/favicon.ico
var trayIconData []byte

var trayAvailable atomic.Bool

func (a *App) startTray() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Printf("systray panic recovered: %v", r)
			}
			trayAvailable.Store(false)
		}()
		trayAvailable.Store(false)
		systray.Run(a.onTrayReady, a.onTrayExit)
	}()
}

func (a *App) stopTray() {
	if trayAvailable.Load() {
		systray.Quit()
	}
}

func (a *App) onTrayReady() {
	trayAvailable.Store(true)

	systray.SetIcon(trayIconData)
	systray.SetTooltip("Flore RSS Reader")

	// 左键单击 → 显示主窗口
	systray.SetOnClick(func(menu systray.IMenu) {
		a.ShowWindow()
	})

	// 右键单击 → 显示托盘菜单
	systray.SetOnRClick(func(menu systray.IMenu) {
		menu.ShowMenu()
	})

	mShow := systray.AddMenuItem("显示主窗口", "恢复应用窗口")
	mHide := systray.AddMenuItem("最小化主窗口", "最小化到系统托盘")
	systray.AddSeparator()
	mFetch := systray.AddMenuItem("收取所有订阅源的文章", "立即抓取所有订阅源的最新文章")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "完全退出应用")

	a.wireTrayMenuShow(mShow)
	a.wireTrayMenuHide(mHide)
	a.wireTrayMenuFetch(mFetch)
	a.wireTrayMenuQuit(mQuit)
}

func (a *App) wireTrayMenuShow(m *systray.MenuItem) {
	m.Click(func() {
		a.ShowWindow()
	})
}

func (a *App) wireTrayMenuHide(m *systray.MenuItem) {
	m.Click(func() {
		if a.ctx == nil {
			return
		}
		wailsRuntime.WindowHide(a.ctx)
	})
}

func (a *App) wireTrayMenuFetch(m *systray.MenuItem) {
	m.Click(func() {
		a.fetchAllFromTray()
	})
}

func (a *App) wireTrayMenuQuit(m *systray.MenuItem) {
	m.Click(func() {
		a.forceQuit.Store(true)
		if a.ctx != nil {
			wailsRuntime.Quit(a.ctx)
		}
	})
}

func (a *App) onTrayExit() {
	trayAvailable.Store(false)
}

func (a *App) fetchAllFromTray() {
	a.backendMutex.Lock()
	port := a.goPort
	a.backendMutex.Unlock()
	if port == 0 {
		return
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/sources/fetch-all", port)
	resp, err := httpClient.Post(url, "application/json", nil)
	if err != nil {
		a.logger.Printf("tray fetch-all error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		a.logger.Printf("tray fetch-all returned HTTP %d", resp.StatusCode)
		return
	}
	// 限制响应体大小为 1MB，避免异常响应导致内存占用过高
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20)); err != nil {
		a.logger.Printf("tray fetch-all drain error: %v", err)
		return
	}
	a.logger.Printf("tray fetch-all triggered")
}