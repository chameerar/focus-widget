(() => {
  'use strict';

  // Present inside the desktop app; absent if the page is opened in a normal browser for a preview.
  const api = window.widget || null;

  const SVGNS = 'http://www.w3.org/2000/svg';
  const PRESETS = [5, 10, 15, 25, 30, 45, 60];
  const MAX_MIN = 99;
  const N = 24;                // ring segments
  const OFF = 0.13;            // opacity of an unlit segment
  const ARM_AHEAD = 15000;     // ms before zero when the alarm audio starts
  const COLORS = ['#F2433B', '#F7A51E', '#58CF4A', '#3EAEF0']; // 12 to 3, 3 to 6, 6 to 9, 9 to 12 o'clock
  const STORE = 'focus-widget:v1';

  const $ = (id) => document.getElementById(id);
  const pad2 = (n) => String(n).padStart(2, '0');
  const clampMin = (m) => Math.min(MAX_MIN, Math.max(1, Math.round(m)));

  function el(tag, attrs, parent) {
    const e = document.createElementNS(SVGNS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    if (parent) parent.appendChild(e);
    return e;
  }

  /* ---------- Saved preferences and today's tally ---------- */
  function load() {
    try {
      const v = JSON.parse(localStorage.getItem(STORE));
      return v && typeof v === 'object' ? v : {};
    } catch (e) { return {}; }
  }
  function save() {
    try { localStorage.setItem(STORE, JSON.stringify(P)); } catch (e) { /* storage unavailable */ }
  }
  const P = Object.assign({ sound: true, lastMinutes: 25, day: '', count: 0, minutes: 0 }, load());
  P.sound = P.sound !== false;

  const firstMin = clampMin(Number(P.lastMinutes) || 25);
  const S = {
    mode: 'idle',                    // idle | running | paused | done
    duration: firstMin * 60000,      // the length you set
    remaining: firstMin * 60000,     // used while idle or paused
    sessionTotal: firstMin * 60000,  // full length of the current run (ring scale)
    endAt: 0,
    isExtension: false,
    dragging: false,
    lastDrag: null
  };

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

  /* ---------- Sound, scheduled ahead so it rings on time ---------- */
  let ctx = null;
  let alarmNodes = [];
  let quietAt = 0;   // audio clock time when the last scheduled sound ends
  let prevAudioTime = Infinity; // currentTime on the previous arming tick

  function ensureAudio() {
    try {
      if (!ctx) {
        const AC = window.AudioContext || window.webkitAudioContext;
        if (!AC) return null;
        ctx = new AC();
      }
      ctx.resume();  // unconditional: also undoes a syncAudio() suspend that hasn't settled yet
      return ctx;
    } catch (e) { return null; }
  }
  // Runs on the render tick. A running context renders silence nonstop, so it only runs from
  // ARM_AHEAD before zero until the chimes end. Once armed, the chime rings on the audio clock.
  function syncAudio() {
    const rem = S.endAt - Date.now();
    if (S.mode === 'running' && P.sound && !alarmNodes.length && rem <= ARM_AHEAD) {
      const c = ensureAudio();
      if (!c) return;
      // Schedule once the clock is seen moving: a context that is still starting its
      // audio device can report 'running' while currentTime stands still.
      const moving = c.state === 'running' && c.currentTime > prevAudioTime;
      prevAudioTime = c.currentTime;
      if (moving) scheduleAlarm(rem);
    } else if (ctx && ctx.state === 'running' && ctx.currentTime > quietAt) {
      ctx.suspend();
    }
  }
  function beep(at, freq, dur, vol) {
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = 'sine';
    o.frequency.value = freq;
    g.gain.setValueAtTime(0.0001, at);
    g.gain.exponentialRampToValueAtTime(vol, at + 0.012);
    g.gain.exponentialRampToValueAtTime(0.0001, at + dur);
    o.connect(g);
    g.connect(ctx.destination);
    o.start(at);
    o.stop(at + dur + 0.03);
    quietAt = Math.max(quietAt, at + dur + 0.03);
    return o;
  }
  function cancelAlarm() {
    alarmNodes.forEach((o) => { try { o.disconnect(); } catch (e) {} try { o.stop(0); } catch (e) {} });
    alarmNodes = [];
    if (ctx) quietAt = Math.min(quietAt, ctx.currentTime + 0.25); // let a confirm chime finish
  }
  function scheduleAlarm(msFromNow) {
    cancelAlarm();
    if (!P.sound) return;
    const c = ensureAudio();
    if (!c) return;
    const t0 = c.currentTime + Math.max(0, msFromNow) / 1000;
    for (let r = 0; r < 12; r++) {          // about 26 seconds of chimes unless dismissed
      const base = t0 + r * 2.2;
      alarmNodes.push(beep(base, 880, 0.12, 0.28));
      alarmNodes.push(beep(base + 0.18, 880, 0.12, 0.28));
      alarmNodes.push(beep(base + 0.36, 1175, 0.2, 0.28));
    }
  }
  function confirmChime() {
    const c = ensureAudio();
    if (c) beep(c.currentTime + 0.02, 1175, 0.12, 0.18);
  }

  /* ---------- Timer actions ---------- */
  const live = $('live');
  const announce = (msg) => { live.textContent = ''; setTimeout(() => { live.textContent = msg; }, 30); };

  function setDuration(min) {
    const m = clampMin(min);
    S.duration = m * 60000;
    S.remaining = S.duration;
    S.sessionTotal = S.duration;
    P.lastMinutes = m;
    save();
    render();
  }
  function start() {
    if (S.mode !== 'idle') return;
    S.remaining = S.duration;
    S.sessionTotal = S.duration;
    S.isExtension = false;
    S.endAt = Date.now() + S.duration;
    S.mode = 'running';
    sweep();
    announce(`Timer started, ${S.duration / 60000} minutes.`);
    stateChanged();
  }
  function pause() {
    if (S.mode !== 'running') return;
    S.remaining = Math.max(0, S.endAt - Date.now());
    S.mode = 'paused';
    cancelAlarm();
    announce('Paused.');
    stateChanged();
  }
  function resume() {
    if (S.mode !== 'paused') return;
    S.endAt = Date.now() + S.remaining;
    S.mode = 'running';
    announce('Resumed.');
    stateChanged();
  }
  function reset() {
    cancelAlarm();
    S.mode = 'idle';
    S.remaining = S.duration;
    S.sessionTotal = S.duration;
    announce('Timer reset.');
    stateChanged();
  }
  function finish() {
    S.mode = 'done';
    recordFinish();
    if (P.sound && alarmNodes.length === 0) scheduleAlarm(0);
    announce('Time’s up.');
    if (api) {
      api.notify(S.isExtension
        ? 'Your extra minute is over.'
        : `Your ${Math.round(S.sessionTotal / 60000)} minute timer has finished.`);
    }
    stateChanged();
  }
  function startPreset(m) {
    cancelAlarm();
    S.mode = 'idle';
    setDuration(m);
    start();
  }
  function startAgain() {
    cancelAlarm();
    S.mode = 'idle';
    start();
  }
  function primaryAction() {
    if (S.mode === 'idle') start();
    else if (S.mode === 'running') pause();
    else if (S.mode === 'paused') resume();
    else reset();
  }
  function adjust(delta) {
    const step = delta * 60000;
    const cap = MAX_MIN * 60000 + 59000;
    if (S.mode === 'idle') {
      setDuration(S.duration / 60000 + delta);
    } else if (S.mode === 'running') {
      const rem = S.endAt - Date.now();
      const next = Math.min(cap, Math.max(1000, rem + step));
      const diff = next - rem;
      S.endAt += diff;
      S.sessionTotal = Math.max(next, S.sessionTotal + diff);
      cancelAlarm();                         // syncAudio() re-arms it for the new end
      render();
    } else if (S.mode === 'paused') {
      const next = Math.min(cap, Math.max(1000, S.remaining + step));
      S.sessionTotal = Math.max(next, S.sessionTotal + (next - S.remaining));
      S.remaining = next;
      render();
    } else if (S.mode === 'done' && delta > 0) {
      cancelAlarm();
      S.sessionTotal = 60000;
      S.isExtension = true;
      S.endAt = Date.now() + 60000;
      S.mode = 'running';
      announce('One more minute.');
      stateChanged();
    }
  }
  function toggleSound() {
    P.sound = !P.sound;
    save();
    if (P.sound) confirmChime();
    else cancelAlarm();
    device.dataset.sound = P.sound ? 'on' : 'off';
    announce(P.sound ? 'Sound on.' : 'Sound off.');
    reportState();
  }

  /* ---------- Today's tally ---------- */
  function todayKey() {
    const d = new Date();
    return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
  }
  function fmtMinutes(m) {
    m = Math.round(m);
    if (m < 60) return `${m} min`;
    const h = Math.floor(m / 60), r = m % 60;
    return r ? `${h} h ${r} min` : `${h} h`;
  }
  function recordFinish() {
    const day = todayKey();
    if (P.day !== day) { P.day = day; P.count = 0; P.minutes = 0; }
    P.minutes += S.sessionTotal / 60000;
    if (!S.isExtension) P.count += 1;
    save();
  }
  function statsText() {
    const fresh = P.day !== todayKey();
    const count = fresh ? 0 : P.count;
    const minutes = fresh ? 0 : P.minutes;
    if (count === 0 && minutes < 1) return 'No timers finished today yet.';
    return `Today: ${count} ${count === 1 ? 'timer' : 'timers'}, ${fmtMinutes(minutes)} focused`;
  }

  /* ---------- Rendering ---------- */
  let lastMain = '', lastClock = '', lastTitle = null, lastHandle = 0;

  function renderRing(ratio) {
    const lit = Math.max(0, Math.min(1, ratio)) * N;
    const full = Math.floor(lit), frac = lit - full;
    for (let i = 0; i < N; i++) {
      const v = i < full ? 1 : (i === full ? frac : 0);
      segs[i].style.opacity = (OFF + (1 - OFF) * v).toFixed(3);
    }
  }
  function positionHandle() {
    if (S.duration === lastHandle) return;   // rewriting cx/cy forces a layout even when unchanged
    lastHandle = S.duration;
    const [x, y] = pt(131, Math.min(S.duration, 3600000) / 3600000 * 360);
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
  function render() {
    const t = Date.now();
    if (S.mode === 'running' && t >= S.endAt) { finish(); return; }

    let secs, ratio;
    if (S.mode === 'done') {
      secs = Math.min(5999, Math.floor((t - S.endAt) / 1000)); // time past zero
      ratio = 1;
    } else {
      const rem = S.mode === 'running' ? S.endAt - t : S.remaining;
      secs = Math.ceil(rem / 1000);
      // Whole seconds, so the ring repaints together with the digits, once a second.
      ratio = S.mode === 'idle' ? Math.min(S.duration, 3600000) / 3600000 : secs * 1000 / S.sessionTotal;
    }
    const mm = Math.floor(secs / 60), ss = secs % 60;
    const str = pad2(mm) + pad2(ss);
    if (str !== lastMain) {
      lastMain = str;
      str.split('').forEach((c, i) => mainDigits[i].set(c));
    }
    renderRing(ratio);
    renderClock();
    if (S.mode === 'idle') positionHandle();

    // Shown next to the tray icon (macOS menu bar) and in its tooltip
    const title = S.mode === 'done' ? 'Time’s up'
      : S.mode === 'idle' ? ''
      : S.mode === 'paused' ? `Paused ${pad2(mm)}:${pad2(ss)}` : `${pad2(mm)}:${pad2(ss)}`;
    if (title !== lastTitle) {
      lastTitle = title;
      document.title = title || 'Focus Widget';
      if (api) api.setTitle(title);
    }
  }

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
    if (!half) return;
    const flip = () => { device.classList.toggle('dim'); blinkTimer = setTimeout(flip, half); };
    blinkTimer = setTimeout(flip, half);
  }

  function reportState() {
    if (api) api.reportState({ mode: S.mode, sound: P.sound, stats: statsText() });
  }
  function stateChanged() {
    device.dataset.mode = S.mode;
    blink();
    const label = { idle: 'Start (Space)', running: 'Pause (Space)', paused: 'Resume (Space)', done: 'Dismiss (Space)' }[S.mode];
    primaryB.title.textContent = label;
    digitsTitle.textContent = label;
    resetB.title.textContent = S.mode === 'done' ? 'Start again' : 'Reset (R)';
    render();
    reportState();
  }

  function act(action, value) {
    switch (action) {
      case 'primary': primaryAction(); break;
      case 'reset': if (S.mode === 'done') startAgain(); else reset(); break;
      case 'again': startAgain(); break;
      case 'plus': adjust(1); break;
      case 'sound': toggleSound(); break;
      case 'preset': {
        const m = Number(value);
        if (PRESETS.includes(m)) startPreset(m);
        break;
      }
      default: break;
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
    if (m === 0) m = S.lastDrag !== null && S.lastDrag < 30 ? 1 : 60;
    if (S.lastDrag !== null) {               // stop the value jumping across 12 o'clock
      if (S.lastDrag >= 45 && m <= 15) m = 60;
      else if (S.lastDrag <= 15 && m >= 45) m = 1;
    }
    m = Math.max(1, m);
    S.lastDrag = m;
    if (m !== S.duration / 60000) setDuration(m);
  }

  screen.addEventListener('click', (e) => {
    const target = e.target.closest('[data-action]');
    if (target) act(target.dataset.action, target.dataset.min);
  });
  screen.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 || S.mode !== 'idle') return;
    if (e.target.closest('[data-action]')) return;
    if (polar(e).dist < 110) return;         // only the ring band sets the time
    S.dragging = true;
    S.lastDrag = null;
    try { screen.setPointerCapture(e.pointerId); } catch (err) {}
    device.classList.add('dragging');
    dragTo(e);
    e.preventDefault();
  });
  screen.addEventListener('pointermove', (e) => { if (S.dragging) dragTo(e); });
  const endDrag = () => {
    if (!S.dragging) return;
    S.dragging = false;
    device.classList.remove('dragging');
    reportState();
  };
  screen.addEventListener('pointerup', endDrag);
  screen.addEventListener('pointercancel', endDrag);

  let wheelAcc = 0;
  screen.addEventListener('wheel', (e) => {
    if (S.mode !== 'idle') return;
    e.preventDefault();
    wheelAcc += e.deltaMode === 1 ? e.deltaY * 16 : e.deltaY;
    while (Math.abs(wheelAcc) >= 40) {
      adjust(wheelAcc < 0 ? 1 : -1);
      wheelAcc -= 40 * Math.sign(wheelAcc);
    }
  }, { passive: false });

  document.addEventListener('contextmenu', (e) => {
    if (!api) return;                        // keep the normal menu in a browser preview
    e.preventDefault();
    api.openMenu();
  });

  /* ---------- Keyboard (when the widget has focus) ---------- */
  document.addEventListener('keydown', (e) => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const k = e.key;
    if (k === ' ' || k === 'Spacebar' || k === 'Enter') { e.preventDefault(); primaryAction(); }
    else if (k === 'r' || k === 'R') reset();
    else if (k === 'ArrowUp' || k === 'ArrowRight') { e.preventDefault(); adjust(1); }
    else if (k === 'ArrowDown' || k === 'ArrowLeft') { e.preventDefault(); adjust(-1); }
    else if (k === 'm' || k === 'M') toggleSound();
    else if (k === 'Escape') { if (S.mode === 'done') reset(); }
    else if (k === 'ContextMenu' || (k === 'F10' && e.shiftKey)) { if (api) { e.preventDefault(); api.openMenu(); } }
    else if (/^[1-7]$/.test(k)) startPreset(PRESETS[Number(k) - 1]);
  });

  /* ---------- Commands from the tray and right-click menu ---------- */
  if (api) {
    api.onCommand((cmd) => {
      if (!cmd || typeof cmd.type !== 'string') return;
      act(cmd.type, cmd.minutes);
    });
  }

  /* ---------- Go ---------- */
  device.dataset.sound = P.sound ? 'on' : 'off';
  stateChanged();
  setInterval(() => { render(); syncAudio(); }, 200);
  setInterval(reportState, 60000);           // keeps the day's tally in the menu current after midnight
})();
