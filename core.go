package main

import (
	"fmt"
	"log"
	"slices"
	"sync"
	"time"
)

// armAhead is how long before zero the audio output starts, so the chime rings on time.
const armAhead = time.Second

// Snapshot is the timer state the page draws. Times are milliseconds; EndAt is Unix time.
type Snapshot struct {
	Mode         Mode  `json:"mode"`
	Duration     int64 `json:"duration"`
	Remaining    int64 `json:"remaining"`
	SessionTotal int64 `json:"sessionTotal"`
	EndAt        int64 `json:"endAt"`
	IsExtension  bool  `json:"isExtension"`
	Sound        bool  `json:"sound"`
	Event        Event `json:"event,omitempty"`
	Seq          int   `json:"seq"` // grows with every change, so the page can drop a stale one
}

// MenuState is what the tray and right-click menus show.
type MenuState struct {
	Mode        Mode
	Sound       bool
	AlwaysOnTop bool
	Size        string
	Stats       string
}

// UI is the window, tray and menus. The core calls it while holding its lock, so UI methods
// must not call back into the core.
type UI interface {
	State(Snapshot)       // to the page
	Title(string)         // tray title and tooltip
	Menus(MenuState)      // tray and right-click menus
	Finished(body string) // notification, and show the widget if hidden
	OnTop(bool)
	Resize(side int)
}

// Sound plays the chime. Ring schedules it for a moment, usually just ahead. Cancel stops the
// chime but lets a Confirm tone finish.
type Sound interface {
	Ring(at time.Time)
	Cancel()
	Confirm()
}

// Core owns the timer, the settings and the schedule. Every change goes through its lock,
// so the page, tray and menus see changes in order.
type Core struct {
	mu    sync.Mutex
	timer Timer
	set   Settings
	store *Store
	ui    UI
	sound Sound
	now   func() time.Time
	after func(time.Duration, func()) *time.Timer
	wake  *time.Timer
	armed time.Time // the end the chime is scheduled for, if any
	title string
	menu  MenuState
	seq   int
	awake func(bool) // keeps timers precise while a run is on (macOS App Nap)
	busy  bool
}

func NewCore(set Settings, store *Store, ui UI, sound Sound) *Core {
	return &Core{
		timer: NewTimer(set.LastMinutes),
		set:   set,
		store: store,
		ui:    ui,
		sound: sound,
		// Wall-clock time without the monotonic reading, so a run that ends while the computer
		// sleeps is over on wake, as with Date.now() before.
		now:   func() time.Time { return time.Now().Round(0) },
		after: time.AfterFunc,
		awake: stayAwake,
	}
}

// Action is a request from the page or a menu.
type Action struct {
	Type    string `json:"type"`
	Minutes int    `json:"minutes,omitempty"`
}

// Act runs one action. Unknown actions and lengths outside the allowed set are refused.
func (c *Core) Act(a Action) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	t := &c.timer
	var ev Event
	switch a.Type {
	case "primary":
		ev = t.Primary(now)
	case "reset":
		ev = t.Reset()
	case "again":
		ev = t.StartAgain(now)
	case "plus":
		ev = t.Adjust(1, now)
	case "minus":
		ev = t.Adjust(-1, now)
	case "preset":
		if !slices.Contains(Presets, a.Minutes) {
			return fmt.Errorf("not a quick timer: %d", a.Minutes)
		}
		ev = t.StartPreset(a.Minutes, now)
	case "set":
		if a.Minutes < 1 || a.Minutes > maxMinutes {
			return fmt.Errorf("length out of range: %d", a.Minutes)
		}
		ev = t.SetMinutes(a.Minutes)
	case "sound":
		c.set.Sound = !c.set.Sound
		if c.set.Sound {
			c.sound.Confirm()
		}
		c.save()
		c.changed(EvSound, now)
		return nil
	default:
		return fmt.Errorf("unknown action %q", a.Type)
	}
	if ev != EvNone {
		c.changed(ev, now)
	}
	return nil
}

// State is the current snapshot, for the page's first draw.
func (c *Core) State() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshot(EvNone)
}

// Start draws the first state into the tray and menus.
func (c *Core) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.menu = MenuState{}
	c.refresh(c.now())
	c.schedule(c.now())
}

func (c *Core) snapshot(ev Event) Snapshot {
	t := c.timer
	s := Snapshot{
		Mode:         t.Mode,
		Duration:     t.Duration.Milliseconds(),
		Remaining:    t.Remaining.Milliseconds(),
		SessionTotal: t.SessionTotal.Milliseconds(),
		IsExtension:  t.IsExtension,
		Sound:        c.set.Sound,
		Event:        ev,
		Seq:          c.seq,
	}
	if !t.EndAt.IsZero() {
		s.EndAt = t.EndAt.UnixMilli()
	}
	return s
}

// changed applies the side effects of a timer or sound change. Call with the lock held.
func (c *Core) changed(ev Event, now time.Time) {
	// Any change to a run moves or ends its chime; the schedule arms it again.
	if ev != EvFinish {
		c.sound.Cancel()
		c.armed = time.Time{}
	}
	switch ev {
	case EvFinish:
		c.set.Today.Record(now, c.timer.SessionTotal, c.timer.IsExtension)
		c.save()
		if c.set.Sound && !c.armed.Equal(c.timer.EndAt) {
			c.sound.Ring(now) // not armed in time, for example after sleep: ring at once
		}
		c.armed = time.Time{}
		c.ui.Finished(c.timer.FinishedText())
	}
	if m := c.timer.Minutes(); m != c.set.LastMinutes {
		c.set.LastMinutes = m
		c.save()
	}
	c.seq++
	c.ui.State(c.snapshot(ev))
	c.refresh(now)
	c.schedule(now)
}

// refresh updates the tray title and the menus when they change. Call with the lock held.
func (c *Core) refresh(now time.Time) {
	if title := c.timer.Title(now); title != c.title {
		c.title = title
		c.ui.Title(title)
	}
	m := MenuState{
		Mode:        c.timer.Mode,
		Sound:       c.set.Sound,
		AlwaysOnTop: c.set.AlwaysOnTop,
		Size:        c.set.Size,
		Stats:       c.set.Today.Text(now),
	}
	if m != c.menu {
		c.menu = m
		c.ui.Menus(m)
	}
	if busy := c.timer.Mode == Running; busy != c.busy {
		c.busy = busy
		c.awake(busy)
	}
}

// schedule sets the single wake-up: the next title change, arming the chime, or zero while
// running, and otherwise the next midnight for the day's tally. Call with the lock held.
func (c *Core) schedule(now time.Time) {
	next := nextMidnight(now)
	if c.timer.Mode == Running {
		next = c.timer.NextTick(now)
		if arm := c.timer.EndAt.Add(-armAhead); c.set.Sound && c.armed.IsZero() && arm.Before(next) {
			next = arm
		}
	}
	// Sleep pauses the clock timers run on, so never wait long on a wall-clock target.
	d := min(next.Sub(now), time.Hour)
	if c.wake != nil {
		c.wake.Stop()
	}
	c.wake = c.after(max(0, d), c.tick)
}

func nextMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}

func (c *Core) tick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.timer.Mode == Running && c.set.Sound && c.armed.IsZero() && !now.Before(c.timer.EndAt.Add(-armAhead)) {
		c.armed = c.timer.EndAt
		c.sound.Ring(c.timer.EndAt)
	}
	if c.timer.Due(now) {
		c.changed(c.timer.Finish(), now)
		return
	}
	c.refresh(now)
	c.schedule(now)
}

func (c *Core) save() {
	if c.store == nil {
		return
	}
	if err := c.store.Save(c.set); err != nil {
		log.Printf("settings: %v", err)
	}
}

// SetOnTop keeps the widget above other windows, or not.
func (c *Core) SetOnTop(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set.AlwaysOnTop = on
	c.save()
	c.ui.OnTop(on)
	c.refresh(c.now())
}

// SetSize resizes the widget to one of Sizes.
func (c *Core) SetSize(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	side, ok := Sizes[name]
	if !ok {
		return
	}
	c.set.Size = name
	c.ui.Resize(side)
	c.save()
	c.refresh(c.now())
}

// MovedTo remembers where the widget is.
func (c *Core) MovedTo(x, y int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.set.X != nil && *c.set.X == x && *c.set.Y == y {
		return
	}
	c.set.X, c.set.Y = &x, &y
	c.save()
}

// Settings returns a copy of the current settings.
func (c *Core) Settings() Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.set
}
