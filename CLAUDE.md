# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go run -tags private_mac_apis .   # run on macOS (the tag makes the window transparent)
go run .                          # run on Windows
go run -tags gtk3 .               # run on Linux (needs libgtk-3-dev libwebkit2gtk-4.1-dev)
go test ./...                     # unit tests; add -tags gtk3 on Linux
go vet ./...
build/macos/package.sh arm64      # or amd64; DMG in dist/
build/windows/package.sh          # Git Bash with NSIS; installer in dist/
build/linux/package.sh            # .deb and .tar.gz in dist/
```

There is no bundler or npm step: `renderer/` is embedded as is with `go:embed`. `renderer/index.html` can be opened directly in a browser for a visual check: `window.widget` is absent there and the page shows a static idle dial.

Build tags: `production` for installers (no DevTools, enables "Open at login"), `private_mac_apis` on macOS (transparent webview), `gtk3` on Linux (GTK 3 and WebKitGTK 4.1 instead of GTK 4), `measure` for a build driven from stdin (see `measure.go`; `FOCUS_WIDGET_CONFIG` points settings at a test folder).

## Architecture

A Wails v3 app: one Go `main` package at the repository root plus the page in `renderer/` (plain JavaScript, `'use strict'`, a single IIFE, no dependencies).

**Go owns all state.** `core.go` holds the `Timer` (`timer.go`: `idle | running | paused | done`, pure methods that take `now`), the `Settings` (`store.go`), the sound (`sound.go`) and the one wake-up timer. The page draws snapshots and sends input. `app.go` is the Wails layer: window, tray, menus, notifications and the bound `Widget` service. `Core` talks to it only through the `UI` interface, and to sound through `Sound`, so `core_test.go` runs on a fake clock without a window.

**Locking:** every change goes through `Core.mu`, and the core calls `UI` methods while holding it, so the page, tray and menus see changes in order. `App.mu` is only ever taken after `Core.mu`, never before; UI methods must not call back into the core. Wails delivers menu clicks, page calls and window events on goroutines, never the main thread, which is what makes this safe.

**The page's calls are a fixed, narrow contract** (`Widget` in `app.go`, wrapped by `renderer/bridge.js`):
- Page to Go: `GetState()`, `Ready()` (shows the window after the first draw), `Act({type, minutes})`, `OpenMenu(x, y)`. Each checks the caller is the `main` window. `Core.Act` refuses unknown types; `preset` must be one of `Presets`, `set` must be 1 to 99.
- Go to page: the `widget:state` event with a `Snapshot` (`mode`, durations in ms, `endAt` in Unix ms, `sound`, `event`, `seq`). The page drops a snapshot with a lower `seq` and uses `event` for announcements and the start sweep.
- Actions: `primary`, `reset`, `again`, `plus`, `minus`, `sound`, `preset`, `set`. Menu items call `Core.Act` directly.
- New calls must be added in `app.go`, `bridge.js` and `widget.js`, and validated in Go.

**Values duplicated across Go and the page** that must stay in sync: `Presets` (`timer.go`) and `PRESETS` (`widget.js`; keys 1 to 7 index into it), the mode names, and the keyboard help text in `index.html` and the README.

**Persistence:** one file, `settings.json`, in `os.UserConfigDir()/Focus Widget` (the folder the Electron versions used): size, keep on top, position, sound, last length, today's tally. On first run it imports size, keep on top and position from the Electron `widget-state.json` and never changes that file. Writes go through a temp file and rename.

**Timing model:** countdowns use wall-clock time with the monotonic reading stripped (`Core.now`), so a run that ends during sleep is done on wake. `Core.schedule` sets a single `time.AfterFunc`: while running, the next tray-title change (whole seconds), `armAhead` (1 s) before zero to start the chime, and zero; otherwise an hourly check for the day's tally. Nothing wakes more often while idle. Waits are capped at an hour because the clock timers run on stops during sleep. While running on macOS, `stayAwake` holds an `NSProcessInfo` activity so App Nap doesn't delay the wake-ups when the widget is hidden.

**Sound:** `Chime` synthesizes the alarm (880, 880 and 1175 Hz beeps with exponential envelopes, 12 rounds 2.2 s apart) and plays it with oto. `Ring(at)` starts a player with leading silence so the chime starts at `at`. The audio output opens on first use and is suspended whenever nothing plays. `Cancel` stops the alarm but lets a confirm tone finish.

**Keep it idle when nothing changes:** a transparent window that repaints costs CPU even when hidden. The page renders with one timeout to the next visible change (the countdown's next whole second, or the clock's next minute) and nothing while `document.hidden`. While running the ring changes once a second with no transition. The paused and done blinks toggle `.dim` on `#device` from `blink()` instead of CSS animations. The tray title updates once a second while running, which is the main remaining running cost.

**UI:** the whole dial is SVG generated in `widget.js` in a 300x300 viewBox. Visual state is CSS driven by `data-mode` and `data-sound` on `#device`. `.face` is the drag region (`--wails-draggable: drag`) and `.bezel` opts out. Right-click opens the Go menu through `--custom-contextmenu: widget` on `.device`; the menu key and Shift+F10 call `OpenMenu`. `#screen` sizes itself with `aspect-ratio`, because WebKit resolved `height:100%` against the bezel's padded height and made it oval.

**Window behavior:** closing hides the window (only `App.quit` sets `quitting`). The app lives in the tray, hides the macOS Dock icon (accessory policy and `LSUIElement`) and is single-instance; a second launch shows the window. "Open at login" uses Wails autostart and is offered only in `production` builds on macOS and Windows. Notifications start on first use (macOS needs an app bundle and the user's permission).

## Security constraints to preserve

- The page gets only the four `Widget` calls. Validate every argument in Go; never bind another service (the notifications service is started by hand so it isn't exposed to the page).
- Wails has no permission system for its own runtime calls (window, clipboard, dialogs and so on are callable from page script) and no navigation hook. The defences are the CSP and the page having no links, forms or remote content. Keep it that way.
- The CSP in `index.html` is `default-src 'none'` with `'self'` for scripts, styles and `connect-src` (the Wails runtime calls Go with `fetch`), plus `base-uri 'none'` and `form-action 'none'`. No inline scripts or styles: set styles via classes or `element.style` from JS.
- Web permission requests (camera, microphone, location, notifications, clipboard read) are denied in the window options.
- Go modules are pinned in `go.mod`/`go.sum`, go-winres is a pinned `tool`, and GitHub Actions are pinned to commit SHAs with a version comment. Dependabot uses a 14-day cooldown; only pick releases at least 14 days old. Release builds run with the Go module cache off.

## Releasing

Bump `VERSION`, commit, then push a `vX.Y.Z` tag that matches it. `.github/workflows/release.yml` tests, then builds DMGs (arm64, x64), an NSIS installer (x64, on windows-2022, which has NSIS) and a .deb plus .tar.gz (x64, on ubuntu-22.04 for an older glibc), and publishes a GitHub release. Running the workflow manually builds without publishing. Builds are ad-hoc signed on macOS and unsigned on Windows.
