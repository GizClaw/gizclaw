package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	desktopApp, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title:  "GizClaw",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []interface{}{desktopApp},
	})
	if err != nil {
		log.Fatal(err)
	}
}
