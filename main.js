'use strict';

const { app, BrowserWindow, Menu, Tray, Notification, ipcMain, screen, nativeImage, session } = require('electron');
const path = require('node:path');
const fs = require('node:fs');

// Timers can be started from the tray or the right-click menu, not only by clicking
// inside the widget, so the chime must be allowed to play without a page gesture.
app.commandLine.appendSwitch('autoplay-policy', 'no-user-gesture-required');

const SIZES = { small: 180, medium: 300, large: 380 };
const PRESETS = [5, 10, 15, 25, 30, 45, 60];
const MODES = ['idle', 'running', 'paused', 'done'];

let win = null;
let tray = null;
let quitting = false;
let settings = { size: 'medium', alwaysOnTop: true, x: null, y: null };
let widgetState = { mode: 'idle', sound: true, stats: '' };

/* ---------- Saved window settings ---------- */
const statePath = () => path.join(app.getPath('userData'), 'widget-state.json');

function readSettings() {
  try {
    const raw = JSON.parse(fs.readFileSync(statePath(), 'utf8'));
    return raw && typeof raw === 'object' ? raw : {};
  } catch {
    return {};
  }
}

function writeSettings() {
  try {
    fs.writeFileSync(statePath(), JSON.stringify(settings, null, 2));
  } catch (err) {
    console.error('Could not save widget settings:', err.message);
  }
}

// Put the widget back where it was, unless that spot is no longer on any screen.
function visiblePosition(x, y, side) {
  if (Number.isFinite(x) && Number.isFinite(y)) {
    const onScreen = screen.getAllDisplays().some(({ workArea: a }) =>
      x + side - 40 > a.x && x + 40 < a.x + a.width &&
      y + side - 40 > a.y && y + 40 < a.y + a.height);
    if (onScreen) return { x, y };
  }
  const a = screen.getPrimaryDisplay().workArea;
  return { x: a.x + a.width - side - 24, y: a.y + a.height - side - 24 };
}

/* ---------- Window ---------- */
function createWindow() {
  const side = SIZES[settings.size];
  const pos = visiblePosition(settings.x, settings.y, side);

  win = new BrowserWindow({
    width: side,
    height: side,
    x: pos.x,
    y: pos.y,
    title: 'Focus Widget',
    frame: false,
    transparent: true,
    backgroundColor: '#00000000',
    hasShadow: false,
    roundedCorners: false,
    resizable: false,
    maximizable: false,
    fullscreenable: false,
    skipTaskbar: true,
    alwaysOnTop: settings.alwaysOnTop,
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      backgroundThrottling: false, // keep the countdown live while other windows cover it
      spellcheck: false,
      devTools: !app.isPackaged
    }
  });

  if (settings.alwaysOnTop) win.setAlwaysOnTop(true, 'floating');

  win.loadFile(path.join(__dirname, 'renderer', 'index.html'));
  win.once('ready-to-show', () => win.show());

  // The widget only ever shows its own local page.
  win.webContents.on('will-navigate', (e) => e.preventDefault());
  win.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));

  let moveTimer = null;
  win.on('move', () => {
    clearTimeout(moveTimer);
    moveTimer = setTimeout(() => {
      if (!win || win.isDestroyed()) return;
      const [x, y] = win.getPosition();
      settings.x = x;
      settings.y = y;
      writeSettings();
    }, 400);
  });

  // Windows and Linux: right-click on the draggable frame opens our menu, not the system one.
  win.on('system-context-menu', (e) => {
    e.preventDefault();
    popMenu();
  });

  // Closing the window (for example Alt+F4) hides it; Quit is in the menu and tray.
  win.on('close', (e) => {
    if (!quitting) {
      e.preventDefault();
      win.hide();
      refreshTray();
    }
  });
  win.on('closed', () => { win = null; });
}

function toggleVisible() {
  if (!win) return;
  if (win.isVisible()) win.hide();
  else win.show();
  refreshTray();
}

function setOnTop(on) {
  settings.alwaysOnTop = on;
  writeSettings();
  if (win) win.setAlwaysOnTop(on, 'floating');
  refreshTray();
}

function setSize(name) {
  if (!SIZES[name] || !win) return;
  const side = SIZES[name];
  const b = win.getBounds();
  const cx = b.x + b.width / 2;
  const cy = b.y + b.height / 2;
  settings.size = name;
  win.setResizable(true); // some window managers ignore size changes on fixed-size windows
  win.setBounds({ x: Math.round(cx - side / 2), y: Math.round(cy - side / 2), width: side, height: side });
  win.setResizable(false);
  const [x, y] = win.getPosition();
  settings.x = x;
  settings.y = y;
  writeSettings();
  refreshTray();
}

/* ---------- Menu (right-click and tray) ---------- */
function send(cmd) {
  if (win && !win.isDestroyed()) win.webContents.send('widget:command', cmd);
}

function buildMenu() {
  const s = widgetState;
  const primary = { idle: 'Start', running: 'Pause', paused: 'Resume', done: 'Dismiss' }[s.mode];
  const template = [
    { label: primary, click: () => send({ type: 'primary' }) },
    {
      label: s.mode === 'done' ? 'Start again' : 'Reset',
      enabled: s.mode !== 'idle',
      click: () => send({ type: s.mode === 'done' ? 'again' : 'reset' })
    },
    { label: 'Add 1 minute', click: () => send({ type: 'plus' }) },
    {
      label: 'Quick timers',
      submenu: PRESETS.map((m) => ({ label: `${m} minutes`, click: () => send({ type: 'preset', minutes: m }) }))
    },
    { type: 'separator' },
    { label: 'Sound', type: 'checkbox', checked: s.sound, click: () => send({ type: 'sound' }) },
    { label: 'Keep on top', type: 'checkbox', checked: settings.alwaysOnTop, click: (item) => setOnTop(item.checked) },
    {
      label: 'Size',
      submenu: Object.keys(SIZES).map((k) => ({
        label: k[0].toUpperCase() + k.slice(1),
        type: 'radio',
        checked: settings.size === k,
        click: () => setSize(k)
      }))
    }
  ];

  // Login items point at the installed app, so only offer this in a packaged build.
  if (app.isPackaged && process.platform !== 'linux') {
    template.push({
      label: 'Open at login',
      type: 'checkbox',
      checked: app.getLoginItemSettings().openAtLogin,
      click: (item) => app.setLoginItemSettings({ openAtLogin: item.checked })
    });
  }

  template.push(
    { type: 'separator' },
    { label: s.stats || 'No timers finished today yet.', enabled: false },
    { type: 'separator' },
    { label: win && win.isVisible() ? 'Hide widget' : 'Show widget', click: toggleVisible },
    { label: 'Quit Focus Widget', click: () => app.quit() }
  );
  return Menu.buildFromTemplate(template);
}

function popMenu() {
  if (win && !win.isDestroyed()) buildMenu().popup({ window: win });
}

/* ---------- Tray ---------- */
function createTray() {
  const isMac = process.platform === 'darwin';
  const img = nativeImage.createFromPath(path.join(__dirname, 'assets', isMac ? 'trayTemplate.png' : 'tray.png'));
  if (isMac) img.setTemplateImage(true);
  tray = new Tray(img);
  tray.setToolTip('Focus Widget');
  if (process.platform === 'win32') tray.on('click', toggleVisible);
  refreshTray();
}

function refreshTray() {
  if (tray) tray.setContextMenu(buildMenu());
}

/* ---------- Messages from the widget page ---------- */
const fromWidget = (e) => win && !win.isDestroyed() && e.sender === win.webContents;

ipcMain.on('widget:state', (e, s) => {
  if (!fromWidget(e) || !s || typeof s !== 'object') return;
  widgetState = {
    mode: MODES.includes(s.mode) ? s.mode : 'idle',
    sound: s.sound === true,
    stats: typeof s.stats === 'string' ? s.stats.slice(0, 120) : ''
  };
  refreshTray();
});

ipcMain.on('widget:title', (e, text) => {
  if (!fromWidget(e) || typeof text !== 'string' || !tray) return;
  const t = text.slice(0, 24);
  if (process.platform === 'darwin') tray.setTitle(t ? ` ${t}` : '');
  tray.setToolTip(t ? `Focus Widget: ${t}` : 'Focus Widget');
});

ipcMain.on('widget:menu', (e) => {
  if (fromWidget(e)) popMenu();
});

ipcMain.on('widget:notify', (e, body) => {
  if (!fromWidget(e) || typeof body !== 'string') return;
  if (win && !win.isVisible()) win.showInactive();
  if (!Notification.isSupported()) return;
  const n = new Notification({ title: 'Time’s up', body: body.slice(0, 140), silent: true });
  n.on('failed', () => { /* macOS shows notifications only for signed builds */ });
  n.show();
});

/* ---------- App lifecycle ---------- */
function lockDownSession() {
  // The page needs no browser permissions (camera, location, and so on).
  session.defaultSession.setPermissionRequestHandler((_wc, _perm, cb) => cb(false));
  session.defaultSession.setPermissionCheckHandler(() => false);
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (win) {
      win.show();
      refreshTray();
    }
  });

  app.whenReady().then(() => {
    settings = Object.assign(settings, readSettings());
    if (!SIZES[settings.size]) settings.size = 'medium';
    settings.alwaysOnTop = settings.alwaysOnTop !== false;
    if (process.platform === 'darwin' && app.dock) app.dock.hide();
    lockDownSession();
    createWindow();
    createTray();
  });

  app.on('before-quit', () => { quitting = true; });
  app.on('window-all-closed', () => { /* keep running in the tray */ });
}
