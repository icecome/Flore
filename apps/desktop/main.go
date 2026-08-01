package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 仅在显式设置 FLORE_DEVTOOLS=1 时启用 DevTools
	if os.Getenv("FLORE_DEVTOOLS") == "1" && os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--auto-open-devtools-for-tabs")
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:            "Flore",
		Width:            1280,
		Height:           860,
		MinWidth:         900,
		MinHeight:        600,
		Frameless:        true,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		// 将 WebView2 用户数据（localStorage/IndexedDB/Cookies/Cache 等）收归到
		// 应用数据目录下的 webview2/，避免默认散落到 %APPDATA%\[BinaryName.exe]。
		// 便携版下该目录位于 data/webview2，随便携包一起迁移，前端设置不再丢失。
		Windows: &windows.Options{
			WebviewUserDataPath: app.webviewDataPath(),
		},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			return app.ShouldPreventClose()
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}
