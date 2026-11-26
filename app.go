package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// App is the window, tray and menus around the core. It implements UI.
type App struct {
	app  *application.App
	win  *application.WebviewWindow
	tray *application.SystemTray
	core *Core

	mu       sync.Mutex // guards the fields below; taken after the core's lock, never before
	state    MenuState
	menus    []*appMenu
	canLogin bool // "Open at login" points at the installed app, so release builds only

	shown    sync.Once
	ready    atomic.Bool // the window has been placed, so moves are the user's
	quitting atomic.Bool
	moveWait *time.Timer

	notesOnce sync.Once
	notes     *notifications.NotificationService
}

// appMenu is one copy of the menu: the tray's or the right-click one. Both show the same state.
type appMenu struct {
	menu                                *application.Menu
	primary, reset, sound, onTop, login *application.MenuItem
	stats, toggle                       *application.MenuItem
	sizes                               map[string]*application.MenuItem
}

func (a *App) buildMenu(m *application.Menu) *appMenu {
	act := func(t string, minutes int) func(*application.Context) {
		return func(*application.Context) { a.act(Action{Type: t, Minutes: minutes}) }
	}
	am := &appMenu{menu: m, sizes: map[string]*application.MenuItem{}}
	am.primary = m.Add("Start").OnClick(act("primary", 0))
	am.reset = m.Add("Reset").OnClick(func(*application.Context) {
		a.mu.Lock()
		done := a.state.Mode == Done
		a.mu.Unlock()
		if done {
			a.act(Action{Type: "again"})
		} else {
			a.act(Action{Type: "reset"})
		}
	})
	m.Add("Add 1 minute").OnClick(act("plus", 0))
	quick := m.AddSubmenu("Quick timers")
	for _, p := range Presets {
		quick.Add(fmt.Sprintf("%d minutes", p)).OnClick(act("preset", p))
	}
	m.AddSeparator()
	am.sound = m.AddCheckbox("Sound", true).OnClick(act("sound", 0))
	am.onTop = m.AddCheckbox("Keep on top", true).OnClick(func(ctx *application.Context) {
		a.core.SetOnTop(ctx.IsChecked())
	})
	size := m.AddSubmenu("Size")
	for _, sz := range []struct{ name, label string }{{"small", "Small"}, {"medium", "Medium"}, {"large", "Large"}} {
		am.sizes[sz.name] = size.AddRadio(sz.label, false).OnClick(func(*application.Context) { a.core.SetSize(sz.name) })
	}
	if a.canLogin {
		am.login = m.AddCheckbox("Open at login", false).OnClick(func(ctx *application.Context) {
			a.setLogin(ctx.IsChecked())
		})
	}
	m.AddSeparator()
	am.stats = m.Add("").SetEnabled(false)
	m.AddSeparator()
	am.toggle = m.Add("Hide widget").OnClick(func(*application.Context) { a.toggleVisible() })
	m.Add("Quit Focus Widget").OnClick(func(*application.Context) { a.quit() })
	return am
}

func (a *App) act(ac Action) {
	if err := a.core.Act(ac); err != nil {
		log.Printf("action: %v", err)
	}
}

// updateMenus writes the state into both menus. Call with a.mu held.
func (a *App) updateMenus() {
	m := a.state
	primary := map[Mode]string{Idle: "Start", Running: "Pause", Paused: "Resume", Done: "Dismiss"}[m.Mode]
	reset := "Reset"
	if m.Mode == Done {
		reset = "Start again"
	}
	visible := a.win.IsVisible()
	toggle := "Show widget"
	if visible {
		toggle = "Hide widget"
	}
	login := false
	if a.canLogin {
		login, _ = a.app.Autostart.IsEnabled()
	}
	for _, am := range a.menus {
		am.primary.SetLabel(primary)
		am.reset.SetLabel(reset).SetEnabled(m.Mode != Idle)
		am.sound.SetChecked(m.Sound)
		am.onTop.SetChecked(m.AlwaysOnTop)
		for name, item := range am.sizes {
			item.SetChecked(name == m.Size)
		}
		if am.login != nil {
			am.login.SetChecked(login)
		}
		am.stats.SetLabel(m.Stats)
		am.toggle.SetLabel(toggle)
		am.menu.Update()
	}
}

func (a *App) refreshMenus() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updateMenus()
}

func (a *App) toggleVisible() {
	if a.win.IsVisible() {
		a.win.Hide()
		a.refreshMenus()
	} else {
		a.showWidget()
	}
}

// showWidget brings the widget to the front and gives it the keyboard.
func (a *App) showWidget() {
	if !a.ready.Load() {
		return // Ready shows the window once the page has drawn
	}
	if runtime.GOOS == "darwin" && !a.win.IsVisible() {
		// Hidden, covered or on another desktop (Space). Activating the app doesn't switch
		// desktops, but a window that is hidden and shown again opens on the current one.
		a.win.Hide()
	}
	a.win.Show()
	a.win.Focus()
	a.refreshMenus()
}

func (a *App) setLogin(on bool) {
	var err error
	if on {
		err = a.app.Autostart.Enable()
	} else {
		err = a.app.Autostart.Disable()
	}
	if err != nil {
		log.Printf("open at login: %v", err)
	}
	a.refreshMenus()
}

func (a *App) quit() {
	a.quitting.Store(true)
	a.app.Quit()
}

// UI, called by the core with its lock held.

func (a *App) State(s Snapshot) {
	debugf("state: %s %s", s.Mode, s.Event)
	a.app.Event.Emit("widget:state", s)
}

func (a *App) Title(t string) {
	if runtime.GOOS == "darwin" {
		a.tray.SetLabel(t) // the icon's padding spaces it from the icon
	}
	if t == "" {
		a.tray.SetTooltip(appName)
	} else {
		a.tray.SetTooltip(appName + ": " + t)
	}
}

func (a *App) Menus(m MenuState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = m
	a.updateMenus()
}

func (a *App) Finished(body string) {
	// The mode change that follows refreshes the menus, including Show or Hide widget.
	if !a.win.IsVisible() {
		a.win.Show()
	}
	go a.notify(body)
}

func (a *App) OnTop(on bool) { a.win.SetAlwaysOnTop(on) }

// Resize keeps the widget's centre where it is.
func (a *App) Resize(side int) {
	b := a.win.Bounds()
	r := application.Rect{X: b.X + b.Width/2 - side/2, Y: b.Y + b.Height/2 - side/2, Width: side, Height: side}
	linux := runtime.GOOS == "linux"
	if linux {
		a.win.SetResizable(true) // some window managers ignore size changes on fixed-size windows
	}
	a.win.SetBounds(r)
	if linux {
		a.win.SetResizable(false)
	}
}

// notify shows "Time's up". The notification service starts on first use: on macOS it needs
// an app bundle and the user's permission, and on Windows it registers the app for toasts.
func (a *App) notify(body string) {
	a.notesOnce.Do(func() {
		ns := notifications.New()
		if err := ns.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
			log.Printf("notifications: %v", err)
			return
		}
		if runtime.GOOS == "darwin" {
			if ok, err := ns.RequestNotificationAuthorization(); !ok {
				log.Printf("notifications: not allowed: %v", err)
				return
			}
		}
		a.notes = ns
	})
	if a.notes == nil {
		return
	}
	err := a.notes.SendNotification(notifications.NotificationOptions{
		ID:    "finished",
		Title: "Time’s up",
		Body:  truncate(body, 140),
		Sound: &notifications.NotificationSound{Silent: true}, // the chime already plays
	})
	if err != nil {
		log.Printf("notifications: %v", err)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// placeWindow puts the widget back where it was, unless that spot is no longer on any screen,
// and otherwise at the bottom right of the primary screen.
func (a *App) placeWindow() {
	set := a.core.Settings()
	side := Sizes[set.Size]
	x, y, ok := visiblePosition(set.X, set.Y, side, a.app.Screen.GetAll())
	if !ok {
		if s := a.app.Screen.GetPrimary(); s != nil {
			w := s.WorkArea
			x, y = w.X+w.Width-side-24, w.Y+w.Height-side-24
		}
	}
	a.win.SetPosition(x, y)
}

func visiblePosition(px, py *int, side int, screens []*application.Screen) (int, int, bool) {
	if px == nil || py == nil {
		return 0, 0, false
	}
	x, y := *px, *py
	for _, s := range screens {
		w := s.WorkArea
		if x+side-40 > w.X && x+40 < w.X+w.Width && y+side-40 > w.Y && y+40 < w.Y+w.Height {
			return x, y, true
		}
	}
	return 0, 0, false
}

// moved saves the position once the window stops moving.
func (a *App) moved() {
	if !a.ready.Load() {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.moveWait != nil {
		a.moveWait.Stop()
	}
	a.moveWait = time.AfterFunc(400*time.Millisecond, func() {
		x, y := a.win.Position()
		a.core.MovedTo(x, y)
	})
}

// Widget holds the only calls the page can make. Each checks that it comes from the widget window.
type Widget struct{ a *App }

var errNotWidget = errors.New("not the widget window")

func fromWidget(ctx context.Context) bool {
	w, ok := ctx.Value(application.WindowKey).(application.Window)
	return ok && w.Name() == "main"
}

// GetState returns the timer state for the page's first draw.
func (w *Widget) GetState(ctx context.Context) (Snapshot, error) {
	if !fromWidget(ctx) {
		return Snapshot{}, errNotWidget
	}
	return w.a.core.State(), nil
}

// Ready shows the window once the page has drawn, so it never appears blank.
func (w *Widget) Ready(ctx context.Context) error {
	if !fromWidget(ctx) {
		return errNotWidget
	}
	w.a.shown.Do(func() {
		w.a.placeWindow()
		w.a.ready.Store(true)
		w.a.win.Show()
		w.a.refreshMenus()
	})
	return nil
}

// Act runs one timer action, such as {type: "preset", minutes: 25}.
func (w *Widget) Act(ctx context.Context, a Action) error {
	if !fromWidget(ctx) {
		return errNotWidget
	}
	return w.a.core.Act(a)
}

// OpenMenu opens the right-click menu at a point in the window, for the keyboard menu key.
func (w *Widget) OpenMenu(ctx context.Context, x, y int) error {
	if !fromWidget(ctx) {
		return errNotWidget
	}
	side := Sizes[w.a.core.Settings().Size]
	x, y = min(max(0, x), side), min(max(0, y), side)
	w.a.win.OpenContextMenu(&application.ContextMenuData{Id: "widget", X: x, Y: y})
	return nil
}

func (a *App) createWindow(set Settings) {
	side := Sizes[set.Size]
	a.win = a.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "main",
		Title:                      appName,
		Width:                      side,
		Height:                     side,
		Frameless:                  true,
		BackgroundType:             application.BackgroundTypeTransparent,
		BackgroundColour:           application.NewRGBA(0, 0, 0, 0),
		AlwaysOnTop:                set.AlwaysOnTop,
		DisableResize:              true,
		Hidden:                     true,
		DefaultContextMenuDisabled: true,
		MinimiseButtonState:        application.ButtonHidden,
		MaximiseButtonState:        application.ButtonHidden,
		URL:                        "/",
		// The page needs none of these. App notifications go through Go, not the web API.
		Permissions: map[application.PermissionType]application.Permission{
			application.PermissionMicrophone:    application.PermissionDeny,
			application.PermissionCamera:        application.PermissionDeny,
			application.PermissionGeolocation:   application.PermissionDeny,
			application.PermissionNotifications: application.PermissionDeny,
			application.PermissionClipboardRead: application.PermissionDeny,
		},
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropTransparent,
			DisableShadow: true,
		},
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
	})
	// Closing the window (for example Alt+F4) hides it. Quit is in the menus.
	a.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if a.quitting.Load() {
			return
		}
		e.Cancel()
		a.win.Hide()
		go a.refreshMenus()
	})
	a.win.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { a.moved() })
	if runtime.GOOS == "darwin" {
		// IsVisible follows the window's occlusion state on macOS. It changes a moment after
		// Show, and when the widget is covered or on another Space, so Show or Hide widget is
		// relabelled when it does.
		for _, ev := range []events.WindowEventType{events.Mac.WindowShow, events.Mac.WindowHide} {
			a.win.OnWindowEvent(ev, func(*application.WindowEvent) { a.refreshMenus() })
		}
	}
}

func (a *App) createTray() {
	a.tray = a.app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		a.tray.SetTemplateIcon(menuBarIcon(trayTemplateIcon))
	} else {
		a.tray.SetIcon(trayIcon)
	}
	a.tray.SetTooltip(appName)
	trayMenu := application.NewMenu()
	a.menus = append(a.menus, a.buildMenu(trayMenu))
	a.tray.SetMenu(trayMenu)
	// Tray clicks arrive on the main thread, which the core may be waiting for while it holds
	// its lock, so they hand off to a goroutine. A right-click opens the menu.
	switch runtime.GOOS {
	case "darwin":
		a.tray.OnClick(func() {
			if controlClick() {
				a.tray.OpenMenu()
				return
			}
			go a.showWidget()
		})
	case "windows":
		a.tray.OnClick(func() { go a.toggleVisible() })
	}

	ctx := a.app.ContextMenu.New()
	a.menus = append(a.menus, a.buildMenu(ctx.Menu))
	a.app.ContextMenu.Add("widget", ctx)
}

// macMenu is the macOS application menu. It never shows, since the app has no menu bar, but
// its shortcuts work while the widget has focus. The default menu's Minimize, zoom and full
// screen would leave the widget in a state its own menus can't undo, so only Close (which
// hides the widget) and Quit are kept.
func macMenu() *application.Menu {
	m := application.NewMenu()
	app := m.AddSubmenu(appName)
	app.AddRole(application.CloseWindow)
	app.AddRole(application.Quit)
	return m
}

// menuBarIcon pads the 16 pt tray icon to 22 pt. Wails scales the image to the menu bar's
// height (22 pt), which drew the icon larger than other menu bar icons.
func menuBarIcon(icon []byte) []byte {
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		log.Printf("tray icon: %v", err)
		return icon
	}
	b := src.Bounds()
	side := b.Dx() * 22 / 16
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	at := image.Pt((side-b.Dx())/2, (side-b.Dy())/2)
	draw.Draw(dst, b.Sub(b.Min).Add(at), src, b.Min, draw.Src)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		log.Printf("tray icon: %v", err)
		return icon
	}
	return out.Bytes()
}
