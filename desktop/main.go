package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// desktopAssets are produced from the same React application used by Pact
// Server. The desktop build uses a hash router and native transports.
//
//go:embed all:frontend/dist
var desktopAssets embed.FS

// localHelperAssets contains the platform-native PACT runtime built immediately
// before Wails compiles the desktop application. Codex and Claude launch this
// helper on demand, so MCP remains available when the GUI is closed.
//
//go:embed all:localhelper
var localHelperAssets embed.FS

func main() {
	assets, err := fs.Sub(desktopAssets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	desktop := NewDesktop()
	err = wails.Run(&options.App{
		Title:                    "PACT",
		Width:                    1440,
		Height:                   900,
		MinWidth:                 980,
		MinHeight:                680,
		WindowStartState:         options.Maximised,
		BackgroundColour:         &options.RGBA{R: 246, G: 247, B: 244, A: 255},
		EnableDefaultContextMenu: false,
		AssetServer:              &assetserver.Options{Assets: assets},
		OnStartup:                desktop.Startup,
		OnShutdown:               desktop.Shutdown,
		Bind:                     []interface{}{desktop},
	})
	if err != nil {
		log.Fatal(err)
	}
}
