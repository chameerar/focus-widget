// Spike: a Wails v3 shell around the existing page, the starting point for the Go version of the app.
package main

import (
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var (
	app      *application.App
	win      *application.WebviewWindow
	tray     *application.SystemTray
	showOnce sync.Once
	toggles  []*application.MenuItem
	menus    []interface{ Update() }
)

// Widget holds the only calls the page can make.
type Widget struct{}

func (Widget) ReportState(state map[string]any) {
	showOnce.Do(func() { placeWindow(); win.Show(); refreshMenus() })
}

func (Widget) SetTitle(text string) {
	t := truncate(text, 24)
	if runtime.GOOS == "darwin" {
		if t == "" {
			tray.SetLabel("")
		} else {
			tray.SetLabel(" " + t)
		}
	}
	if t == "" {
		tray.SetTooltip("Focus Widget")
	} else {
		tray.SetTooltip("Focus Widget: " + t)
	}
}

func (Widget) OpenMenu(x, y int) {
	win.OpenContextMenu(&application.ContextMenuData{Id: "widget", X: x, Y: y})
}

func (Widget) Notify(body string) {}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

const side = 300

// placeWindow puts the widget at the bottom right of the primary screen. It runs just before the first
// show: until then the native window doesn't exist and SetPosition does nothing.
func placeWindow() {
	if s := app.Screen.GetPrimary(); s != nil {
		a := s.WorkArea
		win.SetPosition(a.X+a.Width-side-24, a.Y+a.Height-side-24)
	}
}

func toggleVisible() {
	if win.IsVisible() {
		win.Hide()
	} else {
		win.Show()
	}
	refreshMenus()
}

func refreshMenus() {
	label := "Show widget"
	if win.IsVisible() {
		label = "Hide widget"
	}
	for _, t := range toggles {
		t.SetLabel(label)
	}
	for _, m := range menus {
		m.Update()
	}
}

func fillMenu(m *application.Menu) {
	toggles = append(toggles, m.Add("Hide widget").OnClick(func(*application.Context) { toggleVisible() }))
	m.AddSeparator()
	m.Add("Quit Focus Widget").OnClick(func(*application.Context) { app.Quit() })
}

var spikeHook func()

func main() {
	app = application.New(application.Options{
		Name:     "Focus Widget",
		Services: []application.Service{application.NewService(&Widget{})},
		Assets:   application.AssetOptions{Handler: application.BundledAssetFileServer(os.DirFS("../renderer"))},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
	})

	win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "main",
		Title:                      "Focus Widget",
		Width:                      side,
		Height:                     side,
		Frameless:                  true,
		BackgroundType:             application.BackgroundTypeTransparent,
		BackgroundColour:           application.NewRGBA(0, 0, 0, 0),
		AlwaysOnTop:                true,
		DisableResize:              true,
		Hidden:                     true,
		DefaultContextMenuDisabled: true,
		MinimiseButtonState:        application.ButtonHidden,
		MaximiseButtonState:        application.ButtonHidden,
		URL:                        "/",
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropTransparent,
			DisableShadow: true,
			WindowLevel:   application.MacWindowLevelFloating,
		},
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
	})
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
		refreshMenus()
	})

	trayMenu := application.NewMenu()
	fillMenu(trayMenu)
	ctxMenu := app.ContextMenu.New()
	fillMenu(ctxMenu.Menu)
	app.ContextMenu.Add("widget", ctxMenu)
	menus = append(menus, trayMenu, ctxMenu)

	tray = app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		icon, _ := os.ReadFile("../assets/trayTemplate@2x.png")
		tray.SetTemplateIcon(icon)
	} else {
		icon, _ := os.ReadFile("../assets/tray@2x.png")
		tray.SetIcon(icon)
	}
	tray.SetTooltip("Focus Widget")
	tray.SetMenu(trayMenu)

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if spikeHook != nil {
			spikeHook()
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
