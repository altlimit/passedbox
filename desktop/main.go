package main

import (
	"embed"
	_ "embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// AppVersion can be injected via ldflags at build time
var AppVersion = "dev"

// AppService provides basic application info to the frontend
type AppService struct{}

// GetAppVersion returns the current injected application version
func (a *AppService) GetAppVersion() string {
	return AppVersion
}

func init() {
	// Register custom events if needed
}

// main function serves as the application's entry point.
func main() {

	// Create a new Wails application by providing the necessary options.
	app := application.New(application.Options{
		Name:        "passedbox",
		Description: "PassedBox - Encrypted Vault Storage",
		Services: []application.Service{
			application.NewService(NewVaultManager()),
			application.NewService(&AppService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create a new window with the necessary options.
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "PassedBox",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
		EnableFileDrop:   true,
	})

	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		details := event.Context().DropTargetDetails()

		// Send to frontend
		app.Event.Emit("files-dropped", map[string]any{
			"paths":  files,
			"target": details.ElementID,
		})
	})

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
