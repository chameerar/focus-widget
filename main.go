// Focus Widget: a desktop countdown widget with a segmented dial and quick timers.
//
// Go owns the timer, the alarm sound, the settings, the tray and the menus. The page in
// renderer/ only draws the state it is sent and passes input back through Widget.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const appName = "Focus Widget"

//go:embed renderer
var renderer embed.FS

//go:embed assets/trayTemplate@2x.png
var trayTemplateIcon []byte

//go:embed assets/tray@2x.png
var trayIcon []byte

// measureHook drives the app from stdin in measurement builds (measure.go).
var measureHook func(*App)

// instanceID keeps the app single-instance; measurement builds use their own.
var instanceID = "io.github.chameerar.focuswidget"

func main() {
	store, err := NewStore()
	if err != nil {
		log.Fatal(err)
	}
	set := store.Load()
	page, err := fs.Sub(renderer, "renderer")
	if err != nil {
		log.Fatal(err)
	}

	a := &App{canLogin: release && runtime.GOOS != "linux"}
	a.app = application.New(application.Options{
		Name:        appName,
		Description: "A desktop countdown widget with a segmented dial and quick timers.",
		Services:    []application.Service{application.NewService(&Widget{a: a})},
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(page)},
		Mac:         application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		Windows:     application.WindowsOptions{WebviewUserDataPath: webviewDataPath()},
		Linux:       application.LinuxOptions{ProgramName: "focus-widget"},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               instanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { a.showWidget() },
		},
	})
	a.core = NewCore(set, store, a, &Chime{})
	a.createWindow(set)
	a.createTray()
	if runtime.GOOS == "darwin" {
		a.app.Menu.Set(macMenu())
	}
	a.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.core.Start()
		if measureHook != nil {
			measureHook(a)
		}
	})

	if err := a.app.Run(); err != nil {
		log.Fatal(err)
	}
}

// webviewDataPath keeps WebView2's data under %LocalAppData%\Focus Widget on Windows.
func webviewDataPath() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, appName, "WebView2")
}
