package app

import (
	"embed"

	"github.com/chenyang-zz/cove/internal/services"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	Name        = "Cove"
	Version     = "0.1.0"
	Description = "Cove desktop application"
)

// devServerURL is populated only by the iOS development build. Production
// builds leave it empty and continue to load the embedded frontend assets.
var devServerURL string

func New(assets embed.FS) *application.App {
	windowURL := "/"
	if devServerURL != "" {
		windowURL = devServerURL
	}

	app := application.New(application.Options{
		Name:        Name,
		Description: Description,
		Services: []application.Service{
			application.NewService(services.NewAppInfoService(Name, Version)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  Name,
		Width:  1100,
		Height: 760,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(241, 248, 248),
		URL:              windowURL,
	})

	return app
}
