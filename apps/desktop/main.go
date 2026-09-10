package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	desktop := NewApp()
	host := application.New(application.Options{
		Name:        "LinkSend",
		Description: "端到端直连文件传输",
		Services: []application.Service{
			application.NewService(desktop),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		ShouldQuit: desktop.shouldQuit,
		ErrorHandler: func(err error) {
			slog.Error("wails application error", "error", err)
		},
	})

	window := host.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:               "main",
		Title:              "LinkSend",
		Width:              1120,
		Height:             760,
		MinWidth:           720,
		MinHeight:          560,
		URL:                "/",
		BackgroundColour:   application.NewRGB(247, 249, 251),
		UseApplicationMenu: true,
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 42,
		},
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault,
		},
	})
	desktop.attachRuntime(host, window)

	if err := host.Run(); err != nil {
		slog.Error("desktop failed", "error", err)
		os.Exit(1)
	}
}
