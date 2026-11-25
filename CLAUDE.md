# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
npm ci            # install (Node 20+; CI uses Node 22)
npm start         # run the app from source (electron .)
npm run dist      # build an installer for the current OS into dist/
```

There is no test suite, linter, bundler or build step. The source files are what ships. Verify changes by running `npm start`. On Linux without a compositor: `npm start -- --enable-transparent-visuals --disable-gpu`.

`renderer/index.html` can also be opened directly in a browser for a quick visual check: `window.widget` is absent there, and the renderer skips all app calls (`if (api)`).

## Architecture

A frameless, transparent Electron app with three source files plus CSS. Plain JavaScript, `'use strict'`, CommonJS in the main process, a single IIFE in the renderer, no dependencies at runtime.

**The renderer owns all timer state.** `renderer/widget.js` holds the mode (`idle | running | paused | done`), durations, alarm scheduling and today's tally. The main process only knows a summary (`mode`, `sound`, `stats`) that the renderer pushes to it so it can label the tray and right-click menus.

**IPC is a fixed, narrow contract** defined in `preload.js`:
- Renderer to main: `widget:state`, `widget:title`, `widget:menu`, `widget:notify`. Every `ipcMain` handler in `main.js` checks the sender with `fromWidget(e)`, validates types, and truncates strings. New messages must follow the same pattern and be added in all three places (preload, main, renderer).
- Main to renderer: `widget:command` with `{ type, minutes? }`. Menu items in `buildMenu()` send these, and the renderer routes them through `act()` (`primary`, `reset`, `again`, `plus`, `sound`, `preset`). Clicks inside the SVG use the same `act()` via `data-action` attributes, so menu and on-screen actions share one code path.

**Values duplicated across processes** that must stay in sync: `PRESETS` (in both `main.js` and `widget.js`; keys 1 to 7 index into it), the mode list (`MODES` in main), and the keyboard help text in `index.html` and the README.

**Persistence is split:**
- Main: `widget-state.json` in `app.getPath('userData')` stores `size`, `alwaysOnTop`, `x`, `y`.
- Renderer: `localStorage` key `focus-widget:v1` stores `sound`, `lastMinutes`, and the daily tally (`day`, `count`, `minutes`).

**Timing model:** countdowns use wall clock (`S.endAt = Date.now() + ...`) and `render()` polls every 200 ms, so accuracy doesn't depend on timers firing on time. A running AudioContext renders silence nonstop, so it stays suspended until `ARM_AHEAD` (15 s) before zero. `syncAudio()`, on the same 200 ms tick, then resumes it and, once a later tick sees `currentTime` advancing (a starting context can report `running` with a stalled clock), schedules the chime in Web Audio (`scheduleAlarm`) so it rings exactly at zero. It suspends the context again once the chimes end. Start, resume, adjust and sound toggles only `cancelAlarm()`; the tick re-arms against the new end time, and `finish()` rings at once if nothing was armed. This relies on `backgroundThrottling: false` and the `autoplay-policy` switch in `main.js`; keep both.

**Keep it idle when nothing changes:** a transparent window that repaints every frame costs several percent CPU, even when hidden (because of `backgroundThrottling: false`). While running, the ring changes once a second with no transition. The paused and done blinks toggle `.dim` on `#device` from `blink()` instead of using CSS animations. `render()` skips DOM writes when values are unchanged (`lastMain`, `lastHandle`, ...).

**UI:** the whole dial (ring segments, seven-segment digits, buttons, presets) is generated as SVG in `widget.js` inside a 300x300 viewBox. Visual state is CSS driven by `data-mode` and `data-sound` on `#device`; hover on `#screen` swaps the clock for the control pad and, when idle, shows presets. `.device` is the window drag region (`app-region: drag`) and `.bezel` opts out, so the frame moves the window and the dial stays interactive. On Windows/Linux a right-click on the drag region fires `system-context-menu`, which `main.js` redirects to the app menu.

**Window behavior:** closing hides the window (only `before-quit` sets `quitting`); the app lives in the tray, hides the macOS Dock icon, and holds a single-instance lock. "Open at login" is offered only in packaged builds on macOS/Windows; DevTools only in unpackaged runs.

## Security constraints to preserve

- The page runs with `sandbox: true`, `contextIsolation: true`, `nodeIntegration: false`. Never expose raw `ipcRenderer`.
- The CSP in `index.html` is `default-src 'none'` with `'self'` for scripts and styles: no inline scripts or styles, no remote resources. Set styles via classes or `element.style` from JS, not `style=""` attributes in HTML.
- Navigation, `window.open`, and all permission requests are denied in `main.js`. The app makes no network requests.
- Dependencies in `package.json` are pinned to exact versions, and GitHub Actions are pinned to commit SHAs with a version comment. Dependabot uses a 14-day cooldown. Keep new pins in the same form.

## Releasing

Bump `version` in `package.json`, commit, then push a `vX.Y.Z` tag. `.github/workflows/release.yml` builds DMG (arm64, x64), NSIS (x64) and AppImage (x64) and publishes a GitHub release. Running the workflow manually builds installers without publishing. Builds are unsigned (`identity: "-"`, no notarization), which is why macOS notifications don't appear in release builds.
