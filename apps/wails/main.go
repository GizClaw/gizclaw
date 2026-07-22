package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	desktopApp, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title:            "GizClaw",
		Width:            1240,
		Height:           820,
		MinWidth:         840,
		MinHeight:        620,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 5, G: 9, B: 20, A: 0},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        desktopApp.startup,
		OnBeforeClose:    desktopApp.beforeClose,
		OnShutdown:       desktopApp.shutdown,
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
		Linux: &linux.Options{WindowIsTranslucent: true},
		Bind:  []any{desktopApp},
	})
	if err != nil {
		log.Fatal(err)
	}
}
