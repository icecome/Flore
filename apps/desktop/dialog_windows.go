//go:build windows

package main

import "golang.org/x/sys/windows"

// showFatalError 在 GUI 模式下用系统消息框展示致命错误。
// GUI 子系统进程没有控制台，log.Fatalf 的输出用户完全看不到（N7）。
func showFatalError(title, message string) {
	_, _ = windows.MessageBox(
		0,
		windows.StringToUTF16Ptr(message),
		windows.StringToUTF16Ptr(title),
		windows.MB_OK|windows.MB_ICONERROR|windows.MB_SYSTEMMODAL,
	)
}
