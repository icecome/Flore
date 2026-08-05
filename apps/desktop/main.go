package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/rss/go-server/backend"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// singleInstanceUniqueId 单实例互斥体标识。
// 必须是固定串：同一份安装的多次启动才能互相识别（C3）。
const singleInstanceUniqueId = "flore-rss-reader-desktop"

// restartWaitTimeout 新实例等待旧实例退出的最长时间。
const restartWaitTimeout = 15 * time.Second

func main() {
	// 桌面后端自衍生模式：以 --backend 启动自身子进程跑后端，
	// 这样分发包里不再有“第二个独立可执行文件”——被 Gatekeeper 静默拦截的正是
	// 这种从网上下载、带 quarantine 的嵌套二进制。子进程即用户已「仍要打开」放行过的
	// 同一份 Flore 二进制，Gatekeeper 不再拦截（详见 backend 包注释）。
	if isBackendMode() {
		runBackendMode()
		return
	}

	// 仅在显式设置 FLORE_DEVTOOLS=1 时启用 DevTools
	if os.Getenv("FLORE_DEVTOOLS") == "1" && os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--auto-open-devtools-for-tabs")
	}

	// RestartApp 拉起的新实例：先等旧实例完全退出，单实例互斥体随之释放，
	// 否则新实例会被下面的 SingleInstanceLock 判定为「第二实例」并自杀（C3）。
	waitForPreviousInstance()

	// Create an instance of the app structure
	app := NewApp()
	// 尽早打印数据目录实际落点，便于在无 GUI/排障时快速定位日志与数据库位置
	app.logger.Printf("resolved app data dir: %s", app.appDataDir())

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
		Frameless:        runtime.GOOS != "darwin",
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
				// 诊断：若反复出现“只有 dock 图标、无窗口”，多为上一次进程未退出而
				// 持有单实例锁，新启动被判定为第二实例而不建窗口。此日志可佐证该情形。
				app.logger.Printf("second instance launch detected (single-instance lock held by another process)")
				ctx := app.context()
				if ctx == nil {
					return
				}
				wailsRuntime.WindowUnminimise(ctx)
				wailsRuntime.WindowShow(ctx)
			},
		},
		// 将 WebView2 用户数据（localStorage/IndexedDB/Cookies/Cache 等）收归到
		// 辅助目录下的 webview2/（便携版位于 exe 同级，安装版位于用户数据目录），
		// 避免默认散落到 %APPDATA%\[BinaryName.exe]，也不污染 data/。
		Windows: &windows.Options{
			WebviewUserDataPath: app.webviewDataPath(),
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHidden(),
		},
		Linux: &linux.Options{
			Icon: appIcon,
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
	// 正常退出（窗口关闭/Quit）。若该日志出现得异常早（启动后几秒内），
	// 通常意味着被单实例锁判定为“第二实例”或窗口未能创建，可据此进一步排查。
	app.logger.Printf("wails.Run returned (app exited normally)")
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

// isBackendMode 判断是否以 --backend 启动（自衍生后端子进程模式）。
func isBackendMode() bool {
	for _, a := range os.Args[1:] {
		if a == "--backend" {
			return true
		}
	}
	return false
}

// runBackendMode 以自身二进制运行后端服务（被 GUI 进程作为子进程拉起）。
func runBackendMode() {
	srv, err := backend.Start()
	if err != nil {
		os.Exit(1)
	}
	srv.RunBlocking()
}
