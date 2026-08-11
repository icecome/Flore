//go:build !windows

package main

// registerNotificationApp 非 Windows 平台为 noop。
func (a *App) registerNotificationApp() {}

// showNativeToast 非 Windows 平台为 noop（不引入 go-toast 依赖）。
func (a *App) showNativeToast(title, body string) error { return nil }
