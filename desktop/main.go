package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
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

func init() {
	application.RegisterEvent[DesktopStreamMessage](desktopStreamEvent)
}

func main() {
	waitForUpdateRelaunch()

	assets, err := fs.Sub(desktopAssets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	desktop := NewDesktop()
	app := application.New(application.Options{
		Name:        "PACT",
		Description: "Live coordination and shared context for people and AI agents.",
		Services: []application.Service{
			application.NewService(desktop),
		},
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(assets),
			DisableLogging: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	desktop.attachApplication(app)
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "main",
		Title:                      "PACT",
		Width:                      1440,
		Height:                     900,
		MinWidth:                   980,
		MinHeight:                  680,
		StartState:                 application.WindowStateMaximised,
		BackgroundColour:           application.NewRGB(246, 247, 244),
		DefaultContextMenuDisabled: true,
		URL:                        "/",
	})
	err = app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
