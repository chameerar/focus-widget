// Maps the page's window.widget calls onto the Wails runtime. Under Electron, preload.js provides window.widget.
(() => {
  'use strict';
  const W = window.wails;
  if (!W) return;
  const call = (method, ...args) => W.Call.ByName(`main.Widget.${method}`, ...args);
  // The right-click menu opens where the pointer is.
  let x = 0, y = 0;
  document.addEventListener('contextmenu', (e) => { x = e.clientX; y = e.clientY; }, true);
  window.widget = {
    onCommand(cb) { W.Events.On('widget:command', (e) => cb(e.data)); },
    reportState(state) { call('ReportState', state); },
    setTitle(text) { call('SetTitle', text); },
    openMenu() { call('OpenMenu', x, y); },
    notify(body) { call('Notify', body); }
  };
})();
