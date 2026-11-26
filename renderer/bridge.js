// Connects the page to the app through the Wails runtime. Absent in a browser preview.
// These are the only calls the page can make; Widget in app.go checks each one.
(() => {
  'use strict';
  const W = window.wails;
  if (!W) return;
  const call = (method, ...args) => W.Call.ByName(`main.Widget.${method}`, ...args);
  const quiet = (p) => p.catch((err) => console.warn(err));
  window.widget = {
    getState: () => call('GetState'),
    ready: () => quiet(call('Ready')),
    act: (type, minutes) => quiet(call('Act', { type, minutes: minutes || 0 })),
    openMenu: (x, y) => quiet(call('OpenMenu', Math.round(x), Math.round(y))),
    onState: (cb) => W.Events.On('widget:state', (e) => cb(e.data))
  };
})();
