# Focus Widget

A small countdown timer that sits on your desktop, inspired by physical time-cube timers. The whole widget is the dial: a ring of colored segments that empties as time runs out, with the controls tucked inside it.

![Focus Widget at rest, with the controls showing, and while running](docs/preview.png)

Works on macOS, Windows and Linux.

## Features

- Quick timers (5, 10, 15, 25, 30, 45 and 60 minutes) that start with one click
- Set any length from 1 to 99 minutes by dragging around the ring, scrolling, or using the arrow keys
- Stays on top of other windows, and you can move it anywhere by dragging the frame
- A chime and a system notification when time is up, then a count of the time past zero
- Three sizes, a tray or menu bar icon, and a count of the timers you finish each day
- Works offline and collects no data

## Install

Download the installer for your system from the [Releases](../../releases) page:

| System | File |
| --- | --- |
| macOS (Apple chip) | `focus-widget-<version>-mac-arm64.dmg` |
| macOS (Intel) | `focus-widget-<version>-mac-x64.dmg` |
| Windows | `focus-widget-<version>-win-x64.exe` |
| Linux | `focus-widget-<version>-linux-x86_64.AppImage` |

The installers aren't signed with a paid developer certificate, so your system will warn you the first time:

- **macOS:** open the app once, then go to System Settings, Privacy & Security, and choose Open Anyway.
- **Windows:** when SmartScreen appears, choose More info, then Run anyway.
- **Linux:** make the file executable (`chmod +x focus-widget-*.AppImage`), then run it.

## Using the widget

- **Move it:** drag the black frame.
- **Controls:** move the pointer over the screen. The current time gives way to three buttons: reset, start or pause, and +1 minute. Clicking the big countdown digits also starts or pauses.
- **Quick timers:** when the timer isn't running, the hover view shows the quick timers at their own spot on the dial. Click one to start it.
- **Your own length:** drag around the colored ring or scroll over the dial.
- **Sound:** click the bell on the screen.
- **Menu:** right-click the widget, or use the tray icon (menu bar on macOS), for quick timers, sound, keep on top, size, today's tally, hide and quit.

When time is up, the ring flashes, the digits turn red and count the time past zero, and a chime plays for about 25 seconds. Click the digits to dismiss it, or press +1 for one more minute.

Closing the window (for example with Alt+F4) only hides it. Use **Show widget** or **Quit Focus Widget** in the tray menu.

### Keyboard shortcuts

These work after you click the widget.

| Key | Action |
| --- | --- |
| Space or Enter | Start, pause, resume, or dismiss the alarm |
| R | Reset |
| Up and down arrows | One minute more or less |
| 1 to 7 | Start a quick timer |
| M | Sound on or off |
| Escape | Dismiss the alarm |

## Run from source

You need [Node.js](https://nodejs.org) 20 or newer.

```bash
git clone <this repository's URL>
cd focus-widget
npm ci
npm start
```

To build an installer for the system you're on, run `npm run dist`. The files appear in the `dist` folder.

### Project layout

| Path | What it does |
| --- | --- |
| `main.js` | Creates the widget window, tray icon and menu, and saves the window position |
| `preload.js` | The small, fixed set of messages the widget page can send to the app |
| `renderer/` | The widget itself: the dial, timer logic and styles |
| `assets/` | App and tray icons |
| `.github/workflows/release.yml` | Builds the installers and publishes releases |

## Releasing a new version

1. Update `version` in `package.json` and commit.
2. Tag the commit and push the tag, for example `git tag v1.1.0` and `git push origin v1.1.0`.
3. The **Build and release** workflow builds the installers on macOS, Windows and Linux and attaches them to a new GitHub release.

You can also run the workflow by hand from the Actions tab to build installers without publishing a release.

## Troubleshooting

- **Linux shows a black box instead of a see-through window:** transparency needs a compositing window manager. Most desktops have one turned on. If yours doesn't, try `npm start -- --enable-transparent-visuals --disable-gpu`.
- **Linux has no tray icon:** some desktops (GNOME in particular) need the AppIndicator extension.
- **No notification on macOS:** macOS shows notifications only for apps signed with an Apple Developer ID. The chime and the flashing dial still work.
- **No sound:** check that the bell on the dial is lit and that your system volume is up.

## Privacy and security

- The widget makes no network requests and collects no data.
- Settings stay on your computer: window position and size in `widget-state.json` in the app's user-data folder, and timer preferences and today's tally in the app's local storage.
- The page runs sandboxed, with context isolation and no Node.js access. The main process checks every message it receives.
- A strict Content Security Policy allows only the app's own files. Navigation, new windows and browser permission requests are all blocked.
- Dependencies and GitHub Actions are pinned to exact versions, and Dependabot proposes updates once they're at least two weeks old.
- `npm audit` reports moderate issues in a `minimatch` dependency of `electron-builder`. That code runs only while building installers and isn't included in the app.

To report a security problem, please use GitHub's private vulnerability reporting on this repository rather than a public issue.

## Contributing

Issues and pull requests are welcome. Please test your change with `npm start` on at least one system and describe what you tried.

## License

[MIT](LICENSE)
