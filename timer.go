package main

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// Mode is the timer's state.
type Mode string

const (
	Idle    Mode = "idle"
	Running Mode = "running"
	Paused  Mode = "paused"
	Done    Mode = "done"
)

// Presets are the quick timers in minutes. Keys 1 to 7 on the page index into them.
var Presets = []int{5, 10, 15, 25, 30, 45, 60}

const (
	maxMinutes = 99
	maxLength  = maxMinutes*time.Minute + 59*time.Second // cap when adding time to a run
	minLength  = time.Second                             // floor when taking time off a run
)

// Event names what an action changed. The page uses it for announcements and the start sweep.
type Event string

const (
	EvNone   Event = ""
	EvStart  Event = "start"
	EvPause  Event = "pause"
	EvResume Event = "resume"
	EvReset  Event = "reset"
	EvFinish Event = "finish"
	EvExtend Event = "extend" // one more minute after finishing
	EvAdjust Event = "adjust"
	EvSound  Event = "sound" // sound turned on or off
)

// Timer is the countdown state machine. Methods take the current time so tests can drive them.
// Countdowns use the wall clock, so a timer that ends while the computer sleeps shows as done on wake.
type Timer struct {
	Mode         Mode
	Duration     time.Duration // the length you set
	Remaining    time.Duration // while idle or paused
	SessionTotal time.Duration // full length of the current run, the ring's scale
	EndAt        time.Time     // when a run ends, or ended once done
	IsExtension  bool          // the run is the extra minute added after finishing
}

func clampMinutes(m int) int { return min(maxMinutes, max(1, m)) }

// NewTimer returns an idle timer set to the given length.
func NewTimer(minutes int) Timer {
	var t Timer
	t.Mode = Idle
	t.setMinutes(minutes)
	return t
}

func (t *Timer) setMinutes(m int) {
	t.Duration = time.Duration(clampMinutes(m)) * time.Minute
	t.Remaining = t.Duration
	t.SessionTotal = t.Duration
}

// Minutes is the set length in whole minutes.
func (t Timer) Minutes() int { return int(t.Duration / time.Minute) }

// SetMinutes changes the set length while idle.
func (t *Timer) SetMinutes(m int) Event {
	if t.Mode != Idle || clampMinutes(m) == t.Minutes() {
		return EvNone
	}
	t.setMinutes(m)
	return EvAdjust
}

func (t *Timer) Start(now time.Time) Event {
	if t.Mode != Idle {
		return EvNone
	}
	t.Remaining = t.Duration
	t.SessionTotal = t.Duration
	t.IsExtension = false
	t.EndAt = now.Add(t.Duration)
	t.Mode = Running
	return EvStart
}

func (t *Timer) Pause(now time.Time) Event {
	if t.Mode != Running {
		return EvNone
	}
	t.Remaining = max(0, t.EndAt.Sub(now))
	t.Mode = Paused
	return EvPause
}

func (t *Timer) Resume(now time.Time) Event {
	if t.Mode != Paused {
		return EvNone
	}
	t.EndAt = now.Add(t.Remaining)
	t.Mode = Running
	return EvResume
}

func (t *Timer) Reset() Event {
	t.Mode = Idle
	t.Remaining = t.Duration
	t.SessionTotal = t.Duration
	return EvReset
}

// Due reports whether a run has reached zero and should finish.
func (t Timer) Due(now time.Time) bool {
	return t.Mode == Running && !now.Before(t.EndAt)
}

func (t *Timer) Finish() Event {
	if t.Mode != Running {
		return EvNone
	}
	t.Mode = Done
	return EvFinish
}

// StartPreset sets one of the Presets and starts it. Other lengths are refused.
func (t *Timer) StartPreset(m int, now time.Time) Event {
	if !slices.Contains(Presets, m) {
		return EvNone
	}
	t.Mode = Idle
	t.setMinutes(m)
	return t.Start(now)
}

// StartAgain restarts the set length, for example after finishing.
func (t *Timer) StartAgain(now time.Time) Event {
	t.Mode = Idle
	return t.Start(now)
}

// Primary is Space or a click on the digits: start, pause, resume, or dismiss.
func (t *Timer) Primary(now time.Time) Event {
	switch t.Mode {
	case Idle:
		return t.Start(now)
	case Running:
		return t.Pause(now)
	case Paused:
		return t.Resume(now)
	default:
		return t.Reset()
	}
}

// Adjust adds or removes whole minutes. Once finished, +1 starts one extra minute.
func (t *Timer) Adjust(delta int, now time.Time) Event {
	step := time.Duration(delta) * time.Minute
	clamp := func(d time.Duration) time.Duration { return min(maxLength, max(minLength, d)) }
	switch t.Mode {
	case Idle:
		return t.SetMinutes(t.Minutes() + delta)
	case Running:
		rem := t.EndAt.Sub(now)
		next := clamp(rem + step)
		diff := next - rem
		t.EndAt = t.EndAt.Add(diff)
		t.SessionTotal = max(next, t.SessionTotal+diff)
		return EvAdjust
	case Paused:
		next := clamp(t.Remaining + step)
		t.SessionTotal = max(next, t.SessionTotal+(next-t.Remaining))
		t.Remaining = next
		return EvAdjust
	case Done:
		if delta <= 0 {
			return EvNone
		}
		t.SessionTotal = time.Minute
		t.IsExtension = true
		t.EndAt = now.Add(time.Minute)
		t.Mode = Running
		return EvExtend
	}
	return EvNone
}

// left is the time still to go, in whole seconds rounded up, as the digits show it.
func (t Timer) left(now time.Time) int {
	rem := t.Remaining
	if t.Mode == Running {
		rem = t.EndAt.Sub(now)
	}
	return max(0, int(math.Ceil(rem.Seconds())))
}

// Title is shown next to the tray icon (macOS menu bar) and in its tooltip.
func (t Timer) Title(now time.Time) string {
	switch t.Mode {
	case Idle:
		return ""
	case Done:
		return "Time’s up"
	}
	s := t.left(now)
	clock := fmt.Sprintf("%02d:%02d", s/60, s%60)
	if t.Mode == Paused {
		return "Paused " + clock
	}
	return clock
}

// NextTick is when the title next changes while running: the next whole second, or zero.
func (t Timer) NextTick(now time.Time) time.Time {
	s := t.left(now)
	if s <= 1 {
		return t.EndAt
	}
	return t.EndAt.Add(-time.Duration(s-1) * time.Second)
}

// FinishedText is the notification body when a run ends.
func (t Timer) FinishedText() string {
	if t.IsExtension {
		return "Your extra minute is over."
	}
	return fmt.Sprintf("Your %d minute timer has finished.", int(math.Round(t.SessionTotal.Minutes())))
}

// Tally counts today's finished timers.
type Tally struct {
	Day     string  `json:"day"` // local date, YYYY-MM-DD
	Count   int     `json:"count"`
	Minutes float64 `json:"minutes"`
}

func dayKey(now time.Time) string { return now.Format(time.DateOnly) }

// Record adds a finished run. The extra minute adds time but not a timer.
func (y *Tally) Record(now time.Time, length time.Duration, extension bool) {
	if day := dayKey(now); y.Day != day {
		*y = Tally{Day: day}
	}
	y.Minutes += length.Minutes()
	if !extension {
		y.Count++
	}
}

// Text is the summary line in the menu.
func (y Tally) Text(now time.Time) string {
	count, minutes := y.Count, y.Minutes
	if y.Day != dayKey(now) {
		count, minutes = 0, 0
	}
	if count == 0 && minutes < 1 {
		return "No timers finished today yet."
	}
	noun := "timers"
	if count == 1 {
		noun = "timer"
	}
	return fmt.Sprintf("Today: %d %s, %s focused", count, noun, fmtMinutes(minutes))
}

func fmtMinutes(m float64) string {
	n := int(math.Round(m))
	if n < 60 {
		return fmt.Sprintf("%d min", n)
	}
	if r := n % 60; r != 0 {
		return fmt.Sprintf("%d h %d min", n/60, r)
	}
	return fmt.Sprintf("%d h", n/60)
}
