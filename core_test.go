package main

import (
	"strings"
	"testing"
	"time"
)

// fakeUI and fakeSound record what the core asks for.
type fakeUI struct {
	states   []Snapshot
	titles   []string
	menus    []MenuState
	finished []string
	onTop    []bool
	sizes    []int
}

func (u *fakeUI) State(s Snapshot)     { u.states = append(u.states, s) }
func (u *fakeUI) Title(t string)       { u.titles = append(u.titles, t) }
func (u *fakeUI) Menus(m MenuState)    { u.menus = append(u.menus, m) }
func (u *fakeUI) Finished(body string) { u.finished = append(u.finished, body) }
func (u *fakeUI) OnTop(on bool)        { u.onTop = append(u.onTop, on) }
func (u *fakeUI) Resize(side int)      { u.sizes = append(u.sizes, side) }
func (u *fakeUI) last() Snapshot       { return u.states[len(u.states)-1] }
func (u *fakeUI) lastTitle() string    { return u.titles[len(u.titles)-1] }
func (u *fakeUI) lastMenu() MenuState  { return u.menus[len(u.menus)-1] }

type fakeSound struct{ log []string }

func (s *fakeSound) Ring(at time.Time) { s.log = append(s.log, "ring "+at.Format("15:04:05.000")) }
func (s *fakeSound) Cancel()           { s.log = append(s.log, "cancel") }
func (s *fakeSound) Confirm()          { s.log = append(s.log, "confirm") }
func (s *fakeSound) rings() int {
	n := 0
	for _, l := range s.log {
		if strings.HasPrefix(l, "ring") {
			n++
		}
	}
	return n
}

// harness runs the core on a fake clock. Wake-ups are recorded and fired by advance.
type harness struct {
	*Core
	ui    *fakeUI
	sound *fakeSound
	clock time.Time
	due   time.Time
	awake []bool
}

func newHarness(t *testing.T, set Settings) *harness {
	h := &harness{ui: &fakeUI{}, sound: &fakeSound{}, clock: t0}
	h.Core = NewCore(set, &Store{dir: t.TempDir()}, h.ui, h.sound)
	h.now = func() time.Time { return h.clock }
	h.after = func(d time.Duration, f func()) *time.Timer {
		h.due = h.clock.Add(d)
		return time.NewTimer(time.Hour) // never fires; advance calls tick
	}
	h.Core.awake = func(on bool) { h.awake = append(h.awake, on) }
	h.Start()
	return h
}

// advance moves the clock forward, firing each wake-up on the way.
func (h *harness) advance(d time.Duration) {
	end := h.clock.Add(d)
	for !h.due.After(end) {
		h.clock = h.due
		h.tick()
	}
	h.clock = end
}

func (h *harness) act(t *testing.T, typ string, minutes int) {
	t.Helper()
	if err := h.Act(Action{Type: typ, Minutes: minutes}); err != nil {
		t.Fatal(err)
	}
}

func TestRunRingsOnTimeAndFinishes(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.act(t, "preset", 5)
	if s := h.ui.last(); s.Mode != Running || s.Event != EvStart || s.EndAt != t0.Add(5*time.Minute).UnixMilli() {
		t.Fatalf("after start: %+v", s)
	}
	if h.ui.lastTitle() != "05:00" {
		t.Fatalf("title %q", h.ui.lastTitle())
	}
	h.advance(4*time.Minute + 58*time.Second)
	if h.ui.lastTitle() != "00:02" || h.sound.rings() != 0 {
		t.Fatalf("2 s left: title %q, sound %v", h.ui.lastTitle(), h.sound.log)
	}
	h.advance(1500 * time.Millisecond)
	if h.sound.rings() != 1 || !strings.HasSuffix(h.sound.log[len(h.sound.log)-1], "09:05:00.000") {
		t.Fatalf("armed %v before zero: %v", armAhead, h.sound.log)
	}
	h.advance(time.Second)
	if s := h.ui.last(); s.Mode != Done || s.Event != EvFinish {
		t.Fatalf("after zero: %+v", s)
	}
	if h.sound.rings() != 1 {
		t.Fatalf("rang again at zero: %v", h.sound.log)
	}
	if len(h.ui.finished) != 1 || h.ui.finished[0] != "Your 5 minute timer has finished." {
		t.Fatalf("finished %v", h.ui.finished)
	}
	if h.ui.lastTitle() != "Time’s up" || h.ui.lastMenu().Stats != "Today: 1 timer, 5 min focused" {
		t.Fatalf("title %q, stats %q", h.ui.lastTitle(), h.ui.lastMenu().Stats)
	}
	if want := []bool{true, false}; len(h.awake) != 2 || h.awake[0] != want[0] || h.awake[1] != want[1] {
		t.Fatalf("awake %v, want %v", h.awake, want)
	}
	if got := h.store.Load(); got.Today.Count != 1 || got.LastMinutes != 5 {
		t.Fatalf("saved %+v", got)
	}
}

func TestLateWakeRingsAtOnce(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.act(t, "preset", 5)
	// The computer slept through the end: the first wake-up is long after it.
	h.clock = t0.Add(20 * time.Minute)
	h.tick()
	if h.ui.last().Mode != Done || h.sound.rings() != 1 {
		t.Fatalf("mode %q, sound %v", h.ui.last().Mode, h.sound.log)
	}
}

func TestSoundOffNeverRings(t *testing.T) {
	set := defaultSettings()
	set.Sound = false
	h := newHarness(t, set)
	h.act(t, "preset", 5)
	h.advance(6 * time.Minute)
	if h.ui.last().Mode != Done || h.sound.rings() != 0 {
		t.Fatalf("mode %q, sound %v", h.ui.last().Mode, h.sound.log)
	}
}

func TestPauseCancelsAndResumeRearms(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.act(t, "preset", 5)
	h.advance(4*time.Minute + 59*time.Second + 500*time.Millisecond)
	h.act(t, "primary", 0) // pause with half a second left, after arming
	if h.sound.log[len(h.sound.log)-1] != "cancel" {
		t.Fatalf("pause didn't cancel: %v", h.sound.log)
	}
	h.advance(time.Hour)
	if h.ui.last().Mode != Paused || h.ui.lastTitle() != "Paused 00:01" {
		t.Fatalf("paused: %q %q", h.ui.last().Mode, h.ui.lastTitle())
	}
	h.act(t, "primary", 0)
	h.advance(time.Second)
	if h.ui.last().Mode != Done || h.sound.rings() != 2 {
		t.Fatalf("resumed run: mode %q, sound %v", h.ui.last().Mode, h.sound.log)
	}
}

func TestAddMinuteNearTheEndMovesTheChime(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.act(t, "preset", 5)
	h.advance(4*time.Minute + 59*time.Second + 500*time.Millisecond)
	h.act(t, "plus", 0)
	h.advance(59 * time.Second)
	if h.ui.last().Mode != Running {
		t.Fatalf("finished early")
	}
	h.advance(1500 * time.Millisecond)
	last := h.sound.log[len(h.sound.log)-1]
	if h.ui.last().Mode != Done || last != "ring 09:06:00.000" {
		t.Fatalf("mode %q, sound %v", h.ui.last().Mode, h.sound.log)
	}
}

func TestDoneThenPlusOneIsAnExtraMinute(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.act(t, "preset", 10)
	h.advance(10 * time.Minute)
	h.act(t, "plus", 0)
	if s := h.ui.last(); s.Event != EvExtend || !s.IsExtension {
		t.Fatalf("extend: %+v", s)
	}
	h.advance(time.Minute)
	if h.ui.finished[1] != "Your extra minute is over." || h.ui.lastMenu().Stats != "Today: 1 timer, 11 min focused" {
		t.Fatalf("finished %v, stats %q", h.ui.finished, h.ui.lastMenu().Stats)
	}
}

func TestActValidates(t *testing.T) {
	h := newHarness(t, defaultSettings())
	for _, a := range []Action{{Type: "preset", Minutes: 7}, {Type: "set", Minutes: 0}, {Type: "set", Minutes: 100}, {Type: "quit"}} {
		if err := h.Act(a); err == nil {
			t.Errorf("%+v was accepted", a)
		}
	}
	h.act(t, "set", 42)
	if s := h.ui.last(); s.Duration != 42*60000 || s.Event != EvAdjust {
		t.Fatalf("set: %+v", s)
	}
	if h.store.Load().LastMinutes != 42 {
		t.Fatal("last length not saved")
	}
}

func TestSoundToggle(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.act(t, "sound", 0)
	if s := h.ui.last(); s.Sound || s.Event != EvSound || h.ui.lastMenu().Sound {
		t.Fatalf("off: %+v", s)
	}
	h.act(t, "sound", 0)
	if !h.ui.last().Sound || h.sound.log[1] != "confirm" { // log[0] is the cancel from turning it off
		t.Fatalf("on: %+v %v", h.ui.last(), h.sound.log)
	}
}

func TestIdleWakesOnlyForMidnight(t *testing.T) {
	h := newHarness(t, defaultSettings())
	if want := t0.Add(time.Hour); !h.due.Equal(want) {
		t.Fatalf("idle wake at %v, want an hourly check", h.due.Sub(t0))
	}
	titles := len(h.ui.titles)
	h.advance(15 * time.Hour)
	if len(h.ui.titles) != titles {
		t.Fatal("idle wake-ups changed the title")
	}
	if h.ui.lastMenu().Stats != "No timers finished today yet." {
		t.Fatalf("stats %q", h.ui.lastMenu().Stats)
	}
}

func TestSettingsChanges(t *testing.T) {
	h := newHarness(t, defaultSettings())
	h.SetSize("large")
	h.SetSize("huge")
	h.SetOnTop(false)
	h.MovedTo(10, 20)
	if len(h.ui.sizes) != 1 || h.ui.sizes[0] != 380 || len(h.ui.onTop) != 1 || h.ui.onTop[0] {
		t.Fatalf("sizes %v, onTop %v", h.ui.sizes, h.ui.onTop)
	}
	got := h.store.Load()
	if got.Size != "large" || got.AlwaysOnTop || *got.X != 10 || *got.Y != 20 {
		t.Fatalf("saved %+v", got)
	}
}
