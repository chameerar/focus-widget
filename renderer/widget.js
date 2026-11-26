(() => {
  'use strict';

  // Present inside the app; absent if the page is opened in a normal browser for a preview.
  // The app owns the timer, the sound and the settings. This page draws the state it is sent.
  const api = window.widget || null;

  const SVGNS = 'http://www.w3.org/2000/svg';
  const PRESETS = [5, 10, 15, 25, 30, 45, 60];
  const N = 24;                // ring segments
  const OFF = 0.13;            // opacity of an unlit segment
  const COLORS = ['#F2433B', '#F7A51E', '#58CF4A', '#3EAEF0']; // 12 to 3, 3 to 6, 6 to 9, 9 to 12 o'clock

  const $ = (id) => document.getElementById(id);
  const pad2 = (n) => String(n).padStart(2, '0');

  function el(tag, attrs, parent) {
    const e = document.createElementNS(SVGNS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    if (parent) parent.appendChild(e);
    return e;
  }

  // The latest snapshot from the app. A browser preview shows an idle 25-minute dial.
  let S = { mode: 'idle', duration: 1500000, remaining: 1500000, sessionTotal: 1500000,
            endAt: 0, isExtension: false, sound: true, seq: -1 };
  let dragging = false;
  let dragMin = null;          // minutes under the pointer while dragging the ring, until the app confirms
  let lastDrag = null;

  /* ---------- Seven-segment digits ---------- */
  const SEGMAP = { '0': 'abcdef', '1': 'bc', '2': 'abged', '3': 'abgcd', '4': 'fgbc', '5': 'afgcd',
                   '6': 'afgedc', '7': 'abc', '8': 'abcdefg', '9': 'abcdfg', ' ': '' };
  const hSeg = (x1, x2, y, t) => { const h = t / 2; return [[x1, y], [x1 + h, y - h], [x2 - h, y - h], [x2, y], [x2 - h, y + h], [x1 + h, y + h]]; };
  const vSeg = (x, y1, y2, t) => { const h = t / 2; return [[x, y1], [x + h, y1 + h], [x + h, y2 - h], [x, y2], [x - h, y2 - h], [x - h, y1 + h]]; };

  function makeDigit(parent, x, y, w, h, t) {
    const g = el('g', { transform: `translate(${x} ${y})` }, parent);
    const gap = Math.max(0.5, t * 0.16);
    const L = t / 2, R = w - t / 2, T = t / 2, M = h / 2, B = h - t / 2;
    const shapes = {
      a: hSeg(L + gap, R - gap, T, t), b: vSeg(R, T + gap, M - gap, t), c: vSeg(R, M + gap, B - gap, t),
      d: hSeg(L + gap, R - gap, B, t), e: vSeg(L, M + gap, B - gap, t), f: vSeg(L, T + gap, M - gap, t),
      g: hSeg(L + gap, R - gap, M, t)
    };
    const polys = {};
    for (const k in shapes) {
      polys[k] = el('polygon', { class: 'seg7', points: shapes[k].map((p) => p.map((n) => n.toFixed(2)).join(',')).join(' ') }, g);
    }
    return { set(ch) { const on = SEGMAP[ch] || ''; for (const k in polys) polys[k].classList.toggle('on', on.includes(k)); } };
  }

  /* ---------- Build the screen ---------- */
  const device = $('device');
  const screen = $('screen');
  const pt = (r, deg) => { const a = (deg - 90) * Math.PI / 180; return [150 + r * Math.cos(a), 150 + r * Math.sin(a)]; };
  const ptStr = (r, deg) => pt(r, deg).map((n) => n.toFixed(2)).join(' ');

  // Ring
  const ringG = el('g', { class: 'ring' }, screen);
  const segs = [];
  const R0 = 121, R1 = 141, STEP = 360 / N, GAP = 1.4;
  for (let i = 0; i < N; i++) {
    const a0 = i * STEP + GAP, a1 = (i + 1) * STEP - GAP;
    const d = `M${ptStr(R1, a0)} A${R1} ${R1} 0 0 1 ${ptStr(R1, a1)} L${ptStr(R0, a1)} A${R0} ${R0} 0 0 0 ${ptStr(R0, a0)}Z`;
    const p = el('path', { d, fill: COLORS[Math.floor(i / (N / 4))], class: 'seg' }, ringG);
    p.style.setProperty('--i', i);
    segs.push(p);
  }
  const handle = el('circle', { r: 6.5, cx: 150, cy: 19, class: 'handle' }, screen);

  // Status icons: the bell doubles as the sound switch
  const bell = el('g', { class: 'bell', 'data-action': 'sound', transform: 'translate(117 78) scale(1.15)' }, screen);
  el('title', {}, bell).textContent = 'Sound on or off (M)';
  el('rect', { class: 'hit', x: -4, y: -4, width: 24, height: 24 }, bell);
  el('path', { class: 'bell-body', d: 'M8 1.6c-2.4 0-4.1 1.9-4.1 4.3v2.8L2.6 11h10.8l-1.3-2.3V5.9C12.1 3.5 10.4 1.6 8 1.6z' }, bell);
  el('path', { class: 'bell-clapper', d: 'M6.2 12.2a1.9 1.9 0 0 0 3.6 0z' }, bell);
  el('path', { class: 'bell-slash', d: 'M1.6 1.8l12.8 12' }, bell);
  const hg = el('g', { class: 'hourglass', transform: 'translate(165 78) scale(1.15)' }, screen);
  el('path', { d: 'M3.5 1.5h9v1.4c0 2-1.3 3.6-3 4.5L8 8l-1.5-.6c-1.7-.9-3-2.5-3-4.5zM3.5 14.5h9v-1.4c0-2-1.3-3.6-3-4.5L8 8l-1.5.6c-1.7.9-3 2.5-3 4.5z' }, hg);

  // Countdown digits, clickable to start or pause
  const mainG = el('g', { class: 'main-digits' }, screen);
  const MW = 31, MH = 58, MT = 7.4, MX = 72, MY = 112;
  const mainDigits = [0, 36, 89, 125].map((dx) => makeDigit(mainG, MX + dx, MY, MW, MH, MT));
  el('rect', { class: 'colon', x: 146.75, y: (MY + MH * 0.3 - 3.25).toFixed(2), width: 6.5, height: 6.5 }, mainG);
  el('rect', { class: 'colon', x: 146.75, y: (MY + MH * 0.7 - 3.25).toFixed(2), width: 6.5, height: 6.5 }, mainG);
  const digitsHit = el('rect', { class: 'digits-hit', 'data-action': 'primary', x: 66, y: 106, width: 168, height: 70, rx: 10 }, screen);
  const digitsTitle = el('title', {}, digitsHit);

  // Current time, like the device. It gives way to the buttons on hover.
  const clockG = el('g', { class: 'clock' }, screen);
  const CW = 12, CH = 22, CT = 3.1, CX = 113, CY = 190;
  const clockDigits = [0, 15, 38, 53].map((dx) => makeDigit(clockG, CX + dx, CY, CW, CH, CT));
  el('rect', { class: 'colon', x: CX + 31.1, y: (CY + CH * 0.3 - 1.4).toFixed(2), width: 2.8, height: 2.8 }, clockG);
  el('rect', { class: 'colon', x: CX + 31.1, y: (CY + CH * 0.7 - 1.4).toFixed(2), width: 2.8, height: 2.8 }, clockG);
  const ampm = el('text', { class: 'ampm', x: CX + 53 + CW + 5, y: CY + CH - 1 }, clockG);

  // Buttons tucked in where the clock sits
  const padG = el('g', { class: 'pad' }, screen);
  function padBtn(x, action, label, cls) {
    const g = el('g', { class: `pad-btn ${cls}`, 'data-action': action, transform: `translate(${x} 201)` }, padG);
    const title = el('title', {}, g);
    title.textContent = label;
    el('circle', { class: 'pad-bg', r: 15 }, g);
    return { g, title };
  }
  const resetB = padBtn(110, 'reset', 'Reset (R)', 'reset');
  el('path', { class: 'icon-stroke', d: 'M6.5 0A6.5 6.5 0 1 1 3.25 -5.63' }, resetB.g);
  el('path', { class: 'icon', d: 'M6.6 -3.7L1.6 -2.9L4.9 -8.4Z' }, resetB.g);
  const primaryB = padBtn(150, 'primary', 'Start (Space)', 'primary');
  el('path', { class: 'icon play', d: 'M-4.5 -7.5L7.5 0L-4.5 7.5Z' }, primaryB.g);
  el('path', { class: 'icon pause', d: 'M-6 -7h4.2v14H-6zM1.8 -7H6v14H1.8z' }, primaryB.g);
  el('path', { class: 'icon-stroke stop', d: 'M-5.5 -5.5L5.5 5.5M5.5 -5.5L-5.5 5.5' }, primaryB.g);
  const plusB = padBtn(190, 'plus', 'Add 1 minute (Up arrow)', 'plus');
  el('text', { class: 'pad-text', 'text-anchor': 'middle', 'dominant-baseline': 'central', y: 0.5 }, plusB.g).textContent = '+1';

  // Quick timers placed around the dial at their own minute mark (shown on hover when idle)
  const presetsG = el('g', { class: 'presets' }, screen);
  PRESETS.forEach((m, idx) => {
    const [x, y] = pt(100, (m % 60) * 6);
    const g = el('g', { class: 'preset', 'data-action': 'preset', 'data-min': m, transform: `translate(${x.toFixed(1)} ${y.toFixed(1)})` }, presetsG);
    el('title', {}, g).textContent = `Start ${m} minutes (${idx + 1})`;
    el('circle', { class: 'preset-bg', r: 13 }, g);
    el('text', { class: 'preset-label', 'text-anchor': 'middle', 'dominant-baseline': 'central', y: 0.5 }, g).textContent = String(m);
  });

  /* ---------- Announcements ---------- */
  const live = $('live');
  const announce = (msg) => { live.textContent = ''; setTimeout(() => { live.textContent = msg; }, 30); };
  const SAY = {
    start: (s) => `Timer started, ${Math.round(s.duration / 60000)} minutes.`,
    pause: () => 'Paused.',
    resume: () => 'Resumed.',
    reset: () => 'Timer reset.',
    finish: () => 'Time’s up.',
    extend: () => 'One more minute.',
    sound: (s) => (s.sound ? 'Sound on.' : 'Sound off.')
  };

  /* ---------- Rendering ---------- */
  let lastMain = '', lastClock = '', lastHandle = 0;

  function renderRing(ratio) {
    const lit = Math.max(0, Math.min(1, ratio)) * N;
    const full = Math.floor(lit), frac = lit - full;
    for (let i = 0; i < N; i++) {
      const v = i < full ? 1 : (i === full ? frac : 0);
      segs[i].style.opacity = (OFF + (1 - OFF) * v).toFixed(3);
    }
  }
  function positionHandle(duration) {
    if (duration === lastHandle) return;     // rewriting cx/cy forces a layout even when unchanged
    lastHandle = duration;
    const [x, y] = pt(131, Math.min(duration, 3600000) / 3600000 * 360);
    handle.setAttribute('cx', x.toFixed(2));
    handle.setAttribute('cy', y.toFixed(2));
  }
  function renderClock() {
    const d = new Date();
    const pm = d.getHours() >= 12;
    const h = d.getHours() % 12 || 12;
    const s = (h < 10 ? ' ' : String(Math.floor(h / 10))) + String(h % 10) + pad2(d.getMinutes());
    if (s === lastClock) return;
    lastClock = s;
    s.split('').forEach((c, i) => clockDigits[i].set(c));
    ampm.textContent = pm ? 'pm' : 'am';
  }

  // Draws S as of now, then waits for the next visible change: the countdown's next whole
  // second, or the clock's next minute. Nothing runs while the window is hidden.
  let renderTimer = 0;
  function render() {
    clearTimeout(renderTimer);
    const t = Date.now();
    const duration = dragMin !== null ? dragMin * 60000 : S.duration;
    let secs, ratio;
    if (S.mode === 'done') {
      secs = Math.min(5999, Math.floor((t - S.endAt) / 1000)); // time past zero
      ratio = 1;
    } else {
      const rem = S.mode === 'running' ? Math.max(0, S.endAt - t) : S.mode === 'idle' ? duration : S.remaining;
      secs = Math.ceil(rem / 1000);
      // Whole seconds, so the ring repaints together with the digits, once a second.
      ratio = S.mode === 'idle' ? Math.min(duration, 3600000) / 3600000 : secs * 1000 / S.sessionTotal;
    }
    const str = pad2(Math.floor(secs / 60)) + pad2(secs % 60);
    if (str !== lastMain) {
      lastMain = str;
      str.split('').forEach((c, i) => mainDigits[i].set(c));
    }
    renderRing(ratio);
    renderClock();
    if (S.mode === 'idle') positionHandle(duration);

    if (document.hidden) return;
    let wait = 60000 - t % 60000;
    if (S.mode === 'running') wait = S.endAt > t ? (S.endAt - t) % 1000 || 1000 : 1000;
    else if (S.mode === 'done') wait = 1000 - (t - S.endAt) % 1000;
    renderTimer = setTimeout(render, wait + 5);
  }
  document.addEventListener('visibilitychange', () => { render(); blink(); });

  let sweepTimer = 0;
  function sweep() {
    ringG.classList.add('sweep');
    clearTimeout(sweepTimer);
    sweepTimer = setTimeout(() => ringG.classList.remove('sweep'), N * 22 + 400);
  }

  // Paused digits and the finished ring blink. A CSS animation would redraw every frame, so flip a class instead.
  let blinkTimer = 0;
  function blink() {
    clearTimeout(blinkTimer);
    device.classList.remove('dim');
    const half = { paused: 700, done: 500 }[S.mode];
    if (!half || document.hidden) return;   // a hidden window still redraws, so don't blink there
    const flip = () => { device.classList.toggle('dim'); blinkTimer = setTimeout(flip, half); };
    blinkTimer = setTimeout(flip, half);
  }

  const MODES = ['idle', 'running', 'paused', 'done'];
  function apply(s) {
    if (!s || !MODES.includes(s.mode) || s.seq < S.seq) return; // an older snapshot arrived late
    const modeChanged = s.mode !== S.mode;
    S = s;
    if (!dragging) dragMin = null;
    device.dataset.mode = S.mode;
    device.dataset.sound = S.sound ? 'on' : 'off';
    if (modeChanged) blink();
    const label = { idle: 'Start (Space)', running: 'Pause (Space)', paused: 'Resume (Space)', done: 'Dismiss (Space)' }[S.mode];
    primaryB.title.textContent = label;
    digitsTitle.textContent = label;
    resetB.title.textContent = S.mode === 'done' ? 'Start again' : 'Reset (R)';
    if (S.event === 'start') sweep();
    if (SAY[S.event]) announce(SAY[S.event](S));
    render();
  }

  function act(type, minutes) {
    if (api) api.act(type, minutes);
  }
  function onAction(action, value) {
    switch (action) {
      case 'reset': act(S.mode === 'done' ? 'again' : 'reset'); break;
      case 'preset': act('preset', Number(value)); break;
      default: act(action); break;  // primary, plus, sound
    }
  }

  /* ---------- Pointer: buttons, plus drag or scroll on the ring to set a time ---------- */
  function polar(e) {
    const r = screen.getBoundingClientRect();
    const x = (e.clientX - r.left) / r.width * 300 - 150;
    const y = (e.clientY - r.top) / r.height * 300 - 150;
    let a = Math.atan2(x, -y) * 180 / Math.PI;
    if (a < 0) a += 360;
    return { a, dist: Math.hypot(x, y) };
  }
  function dragTo(e) {
    let m = Math.round(polar(e).a / 6);
    if (m === 0) m = lastDrag !== null && lastDrag < 30 ? 1 : 60;
    if (lastDrag !== null) {                 // stop the value jumping across 12 o'clock
      if (lastDrag >= 45 && m <= 15) m = 60;
      else if (lastDrag <= 15 && m >= 45) m = 1;
    }
    m = Math.max(1, m);
    lastDrag = m;
    if (m !== dragMin) {
      dragMin = m;
      render();
    }
  }

  screen.addEventListener('click', (e) => {
    const target = e.target.closest('[data-action]');
    if (target) onAction(target.dataset.action, target.dataset.min);
  });
  screen.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 || S.mode !== 'idle') return;
    if (e.target.closest('[data-action]')) return;
    if (polar(e).dist < 110) return;         // only the ring band sets the time
    dragging = true;
    lastDrag = null;
    try { screen.setPointerCapture(e.pointerId); } catch (err) {}
    device.classList.add('dragging');
    dragTo(e);
    e.preventDefault();
  });
  screen.addEventListener('pointermove', (e) => { if (dragging) dragTo(e); });
  const endDrag = () => {
    if (!dragging) return;
    dragging = false;
    device.classList.remove('dragging');
    if (dragMin !== null && dragMin * 60000 !== S.duration) act('set', dragMin);
    else dragMin = null;
  };
  screen.addEventListener('pointerup', endDrag);
  screen.addEventListener('pointercancel', endDrag);

  let wheelAcc = 0;
  screen.addEventListener('wheel', (e) => {
    if (S.mode !== 'idle') return;
    e.preventDefault();
    wheelAcc += e.deltaMode === 1 ? e.deltaY * 16 : e.deltaY;
    while (Math.abs(wheelAcc) >= 40) {
      act(wheelAcc < 0 ? 'plus' : 'minus');
      wheelAcc -= 40 * Math.sign(wheelAcc);
    }
  }, { passive: false });

  // Right-click opens the app's menu through the --custom-contextmenu rule in widget.css.
  function openMenu() {
    const r = screen.getBoundingClientRect();
    api.openMenu(r.left + r.width / 2, r.top + r.height / 2);
  }

  /* ---------- Keyboard (when the widget has focus) ---------- */
  document.addEventListener('keydown', (e) => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const k = e.key;
    if (k === ' ' || k === 'Spacebar' || k === 'Enter') { e.preventDefault(); act('primary'); }
    else if (k === 'r' || k === 'R') act('reset');
    else if (k === 'ArrowUp' || k === 'ArrowRight') { e.preventDefault(); act('plus'); }
    else if (k === 'ArrowDown' || k === 'ArrowLeft') { e.preventDefault(); act('minus'); }
    else if (k === 'm' || k === 'M') act('sound');
    else if (k === 'Escape') { if (S.mode === 'done') act('reset'); }
    else if (k === 'ContextMenu' || (k === 'F10' && e.shiftKey)) { if (api) { e.preventDefault(); openMenu(); } }
    else if (/^[1-7]$/.test(k)) act('preset', PRESETS[Number(k) - 1]);
  });

  /* ---------- Go ---------- */
  if (api) {
    api.onState(apply);
    // The window shows once the first state is drawn. Not in an animation frame: a hidden
    // window gets none.
    api.getState().then((s) => {
      apply(s);
      api.ready();
    });
  } else {
    apply(Object.assign({}, S, { seq: 0 }));
  }
})();
