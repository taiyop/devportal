package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func init() {
	application.RegisterEvent[AppView]("app-status")
	application.RegisterEvent[LogEvent]("app-log")
	application.RegisterEvent[GatewayStatus]("gateway-status")
}

func main() {
	portal, err := LoadPortal()
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:        "DevPortal",
		Description: "Local launch board for everyday dev tools",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	if err := configureUpdater(app); err != nil {
		log.Fatal(err)
	}
	installApplicationMenu(app)

	service := NewPortalService(app, portal)
	app.RegisterService(application.NewService(service))
	installGuard(portal, portal.inner.runningPath)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "DevPortal",
		Width:            1180,
		Height:           780,
		MinWidth:         860,
		MinHeight:        600,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/",
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropLiquidGlass,
			TitleBar: application.MacTitleBarHiddenInset,
			LiquidGlass: application.MacLiquidGlass{
				Style:    application.LiquidGlassStyleAutomatic,
				Material: application.NSVisualEffectMaterialSidebar,
			},
		},
	})

	app.OnShutdown(func() {
		portal.StopAll()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
