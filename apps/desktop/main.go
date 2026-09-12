package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if handled, err := runDesktopSetup(os.Args[1:]); handled {
		if err != nil {
			slog.Error("desktop integration setup failed", "error", err)
			os.Exit(1)
		}
		return
	}
	desktop := NewApp()
	dataDir, err := canonicalProfilePath(desktopProfileDirectory())
	if err != nil {
		slog.Error("profile path unavailable", "error", err)
		os.Exit(1)
	}
	desktop.dataDir = dataDir
	workingDir, _ := os.Getwd()
	paths, entryErr := parseNativeFileArguments(os.Args[1:], workingDir)
	if entryErr == nil {
		entryErr = stageFileActivation(dataDir, paths, workingDir)
	}
	desktop.nativeEntryError(entryErr)
	if entryErr != nil {
		showNativeEntryFailure("无法接收这次文件选择。请检查路径、磁盘空间及草稿容量，然后重新选择。")
		os.Exit(1)
	}
	single, err := profileSingleInstanceOptions(dataDir, func(_ []string, _ string) {
		// The second process already committed its entry before Wails.New.
		desktop.wakeEntries()
	})
	if err != nil {
		slog.Error("single instance setup failed", "error", err)
		os.Exit(1)
	}
	host := application.New(application.Options{
		SingleInstance: single,
		Name:           "LinkSend",
		Description:    "端到端直连文件传输",
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
		EnableFileDrop:     true,
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
	desktop.registerNativeEntries(window)
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		slog.Info("desktop runtime ready")
		desktop.runtimeEntriesReady()
	})

	if err := host.Run(); err != nil {
		slog.Error("desktop failed", "error", err)
		os.Exit(1)
	}
}
