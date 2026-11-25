'use strict';

const { contextBridge, ipcRenderer } = require('electron');

// Only these narrow calls are available to the page. Raw ipcRenderer is never exposed.
contextBridge.exposeInMainWorld('widget', {
  onCommand(callback) {
    ipcRenderer.on('widget:command', (_event, cmd) => callback(cmd));
  },
  reportState(state) {
    ipcRenderer.send('widget:state', state);
  },
  setTitle(text) {
    ipcRenderer.send('widget:title', text);
  },
  openMenu() {
    ipcRenderer.send('widget:menu');
  },
  notify(body) {
    ipcRenderer.send('widget:notify', body);
  }
});
