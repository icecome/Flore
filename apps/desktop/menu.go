package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// setupAppMenu 设置应用主菜单（中文标签）。
// Wails v2 的 EditMenu/WindowMenu 角色在 macOS 端硬编码英文标签（见
// wails/internal/frontend/desktop/darwin/WailsMenu.m），且 MenuItem.Label
// 在角色菜单上被忽略。这里用自定义菜单 + Go 回调 + WindowExecJS
// 触发 webview 的 document.execCommand（编辑动作）、WailsRuntime
// 触发窗口动作，实现中文标题且保留标准行为。
func (a *App) setupAppMenu(ctx context.Context) {
	// 编辑菜单
	editMenu := menu.NewMenu()
	editMenu.AddText("撤销", keys.CmdOrCtrl("z"), func(_ *menu.CallbackData) {
		wailsRuntime.WindowExecJS(ctx, "document.execCommand('undo')")
	})
	editMenu.AddText("重做", keys.Combo("z", keys.ShiftKey, keys.CmdOrCtrlKey), func(_ *menu.CallbackData) {
		wailsRuntime.WindowExecJS(ctx, "document.execCommand('redo')")
	})
	editMenu.AddSeparator()
	editMenu.AddText("剪切", keys.CmdOrCtrl("x"), func(_ *menu.CallbackData) {
		wailsRuntime.WindowExecJS(ctx, "document.execCommand('cut')")
	})
	editMenu.AddText("复制", keys.CmdOrCtrl("c"), func(_ *menu.CallbackData) {
		wailsRuntime.WindowExecJS(ctx, "document.execCommand('copy')")
	})
	editMenu.AddText("粘贴", keys.CmdOrCtrl("v"), func(_ *menu.CallbackData) {
		wailsRuntime.WindowExecJS(ctx, "document.execCommand('paste')")
	})
	editMenu.AddText("全选", keys.CmdOrCtrl("a"), func(_ *menu.CallbackData) {
		wailsRuntime.WindowExecJS(ctx, "document.execCommand('selectAll')")
	})

	// 窗口菜单
	windowMenu := menu.NewMenu()
	windowMenu.AddText("最小化", keys.CmdOrCtrl("m"), func(_ *menu.CallbackData) {
		wailsRuntime.WindowMinimise(ctx)
	})
	windowMenu.AddText("缩放", nil, func(_ *menu.CallbackData) {
		wailsRuntime.WindowToggleMaximise(ctx)
	})
	windowMenu.AddSeparator()
	windowMenu.AddText("进入全屏", keys.Combo("f", keys.ControlKey, keys.CmdOrCtrlKey), func(_ *menu.CallbackData) {
		wailsRuntime.WindowFullscreen(ctx)
	})

	// 顶层组装：第一个子用 AppMenu 角色（macOS 自动生成 About/Hide/Quit 并显示本地化应用名），
	// 其余为中文自定义菜单
	applicationMenu := menu.NewMenu()
	applicationMenu.Append(menu.AppMenu())
	applicationMenu.Append(menu.SubMenu("编辑", editMenu))
	applicationMenu.Append(menu.SubMenu("窗口", windowMenu))

	wailsRuntime.MenuSetApplicationMenu(ctx, applicationMenu)
}
