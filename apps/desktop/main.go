package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

// singleInstanceUniqueId 单实例互斥体标识。
// 必须是固定串：同一份安装的多次启动才能互相识别（C3）。
const singleInstanceUniqueId = "flore-rss-reader-desktop"

// restartWaitTimeout 新实例等待旧实例退出的最长时间。
const restartWaitTimeout = 15 * time.Second

func main() {
	// 仅在显式设置 FLORE_DEVTOOLS=1 时启用 DevTools
	if os.Getenv("FLORE_DEVTOOLS") == "1" && os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--auto-open-devtools-for-tabs")
	}

	// RestartApp 拉起的新实例：先等旧实例完全退出，单实例互斥体随之释放，
	// 否则新实例会被下面的 SingleInstanceLock 判定为「第二实例」并自杀（C3）。
	waitForPreviousInstance()

	// Create an instance of the app structure
	app := NewApp()

	// 加载上次保存的窗口状态
	windowState := app.LoadWindowState()
	startState := options.Normal
	if windowState.Maximised {
		startState = options.Maximised
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:            "Flore",
		Width:            1280,
		Height:           860,
		MinWidth:         900,
		MinHeight:        600,
		Frameless:        true,
		WindowStartState: startState,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		// C3：单实例锁。多实例会并发读写同一个 reader.db 与同一个
		// WebView2 用户数据目录，造成 SQLITE_BUSY / 计数错乱 / 配置互相覆盖。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: singleInstanceUniqueId,
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				ctx := app.context()
				if ctx == nil {
					return
				}
				wailsRuntime.WindowUnminimise(ctx)
				wailsRuntime.WindowShow(ctx)
			},
		},
		// 将 WebView2 用户数据（localStorage/IndexedDB/Cookies/Cache 等）收归到
		// 应用数据目录下的 webview2/，避免默认散落到 %APPDATA%\[BinaryName.exe]。
		// 便携版下该目录位于 data/webview2，随便携包一起迁移，前端设置不再丢失。
		Windows: &windows.Options{
			WebviewUserDataPath: app.webviewDataPath(),
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			return app.ShouldPreventClose()
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		// N7：GUI 子系统进程没有控制台，log.Fatalf 用户完全看不到。
		// 先写日志文件，再弹系统消息框。
		msg := fmt.Sprintf("Flore 启动失败：%v", err)
		app.logger.Printf("fatal: %v", err)
		app.logger.Close()
		showFatalError("Flore", msg)
		os.Exit(1)
	}
}

// waitForPreviousInstance 处理 RestartApp 场景：等待 FLORE_RESTART_WAIT_PID
// 指定的旧实例退出后再继续启动，并清除该环境变量避免向后端子进程传播。
func waitForPreviousInstance() {
	raw := os.Getenv(restartWaitPidEnv)
	if raw == "" {
		return
	}
	_ = os.Unsetenv(restartWaitPidEnv)

	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return
	}
	waitForProcessExit(pid, restartWaitTimeout)
}
