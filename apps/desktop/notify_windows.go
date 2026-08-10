//go:build windows

package main

import (
	"fmt"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
)

// registerNotificationApp 将 Flore 注册到 Windows 注册表，使原生 Toast 显示应用名
// （Flore）而非 WebView2 的 origin（wails.localhost）。
func (a *App) registerNotificationApp() {
	if err := toast.SetAppData(toast.AppData{AppID: "Flore"}); err != nil {
		a.logger.Printf("register notification app failed: %v", err)
	}
}

// showNativeToast 发送原生 Windows Toast 通知（应用名显示为 Flore）。
func (a *App) showNativeToast(title, body string) error {
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