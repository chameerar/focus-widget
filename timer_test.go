package main

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 25, 9, 0, 0, 0, time.Local)

func TestNewTimerClampsLength(t *testing.T) {
	for in, want := range map[int]int{0: 1, -5: 1, 25: 25, 99: 99, 150: 99} {
		if got := NewTimer(in).Minutes(); got != want {
			t.Errorf("NewTimer(%d).Minutes() = %d, want %d", in, got, want)
		}
	}
}

func TestPrimaryCycle(t *testing.T) {
	tm := NewTimer(25)
	if ev := tm.Primary(t0); ev != EvStart || tm.Mode != Running || !tm.EndAt.Equal(t0.Add(25*time.Minute)) {
		t.Fatalf("start: ev=%q mode=%q endAt=%v", ev, tm.Mode, tm.EndAt)
	}
	if ev := tm.Primary(t0.Add(10 * time.Minute)); ev != EvPause || tm.Remaining != 15*time.Minute {
		t.Fatalf("pause: ev=%q remaining=%v", ev, tm.Remaining)
	}
	// Paused time doesn't count.
	if ev := tm.Primary(t0.Add(20 * time.Minute)); ev != EvResume || !tm.EndAt.Equal(t0.Add(35*time.Minute)) {
		t.Fatalf("resume: ev=%q endAt=%v", ev, tm.EndAt)
	}
	if tm.Due(t0.Add(35*time.Minute - time.Millisecond)) {
		t.Fatal("due before the end")
	}
	if !tm.Due(t0.Add(35 * time.Minute)) {
		t.Fatal("not due at the end")
	}
	if ev := tm.Finish(); ev != EvFinish || tm.Mode != Done {
		t.Fatalf("finish: ev=%q mode=%q", ev, tm.Mode)
	}
	if ev := tm.Primary(t0); ev != EvReset || tm.Mode != Idle || tm.Remaining != 25*time.Minute {
		t.Fatalf("dismiss: ev=%q mode=%q remaining=%v", ev, tm.Mode, tm.Remaining)
	}
}

func TestPauseAfterTheEndKeepsZero(t *testing.T) {
	tm := NewTimer(1)
	tm.Start(t0)
	tm.Pause(t0.Add(2 * time.Minute))
	if tm.Remaining != 0 {
		t.Fatalf("remaining = %v, want 0", tm.Remaining)
	}
}

func TestActionsIgnoredInTheWrongMode(t *testing.T) {
	tm := NewTimer(5)
	if tm.Pause(t0) != EvNone || tm.Resume(t0) != EvNone || tm.Finish() != EvNone {
		t.Fatal("pause, resume or finish acted while idle")
	}
	tm.Start(t0)
	if tm.Start(t0) != EvNone || tm.SetMinutes(10) != EvNone {
		t.Fatal("start or set acted while running")
	}
}

func TestAdjustIdle(t *testing.T) {
	tm := NewTimer(98)
	if ev := tm.Adjust(1, t0); ev != EvAdjust || tm.Minutes() != 99 {
		t.Fatalf("98+1: ev=%q minutes=%d", ev, tm.Minutes())
	}
	if ev := tm.Adjust(1, t0); ev != EvNone || tm.Minutes() != 99 {
		t.Fatalf("99+1: ev=%q minutes=%d", ev, tm.Minutes())
	}
	tm = NewTimer(1)
	if ev := tm.Adjust(-1, t0); ev != EvNone || tm.Minutes() != 1 {
		t.Fatalf("1-1: ev=%q minutes=%d", ev, tm.Minutes())
	}
}

func TestAdjustRunning(t *testing.T) {
	tm := NewTimer(5)
	tm.Start(t0)
	now := t0.Add(4*time.Minute + 30*time.Second) // 30 s left
	tm.Adjust(1, now)
	if got := tm.EndAt.Sub(now); got != 90*time.Second {
		t.Fatalf("+1 left %v, want 1m30s", got)
	}
	if tm.SessionTotal != 6*time.Minute {
		t.Fatalf("+1 session total %v, want 6m", tm.SessionTotal)
	}
	tm.Adjust(-1, now)
	tm.Adjust(-1, now)
	if got := tm.EndAt.Sub(now); got != time.Second {
		t.Fatalf("-2 left %v, want the 1 s floor", got)
	}

	tm = NewTimer(99)
	tm.Start(t0)
	tm.Adjust(5, t0)
	if got := tm.EndAt.Sub(t0); got != maxLength {
		t.Fatalf("cap: left %v, want %v", got, maxLength)
	}
}

func TestAdjustPaused(t *testing.T) {
	tm := NewTimer(10)
	tm.Start(t0)
	tm.Pause(t0.Add(8 * time.Minute)) // 2 min left
	tm.Adjust(1, t0)
	if tm.Remaining != 3*time.Minute || tm.SessionTotal != 11*time.Minute {
		t.Fatalf("remaining=%v session=%v", tm.Remaining, tm.SessionTotal)
	}
	tm.Adjust(-5, t0)
	if tm.Remaining != time.Second {
		t.Fatalf("floor: remaining=%v", tm.Remaining)
	}
}

func TestExtraMinuteAfterFinishing(t *testing.T) {
	tm := NewTimer(25)
	tm.Start(t0)
	end := t0.Add(25 * time.Minute)
	tm.Finish()
	if ev := tm.Adjust(-1, end); ev != EvNone || tm.Mode != Done {
		t.Fatalf("-1 when done: ev=%q mode=%q", ev, tm.Mode)
	}
	if ev := tm.Adjust(1, end); ev != EvExtend || tm.Mode != Running || !tm.IsExtension || tm.SessionTotal != time.Minute {
		t.Fatalf("+1 when done: ev=%q %+v", ev, tm)
	}
	if tm.FinishedText() != "Your extra minute is over." {
		t.Fatalf("text = %q", tm.FinishedText())
	}
	// Starting again uses the set length, not the extra minute.
	tm.StartAgain(end)
	if tm.IsExtension || tm.SessionTotal != 25*time.Minute || tm.FinishedText() != "Your 25 minute timer has finished." {
		t.Fatalf("again: %+v %q", tm, tm.FinishedText())
	}
}

func TestStartPresetOnlyAcceptsPresets(t *testing.T) {
	tm := NewTimer(25)
	tm.Start(t0)
	if ev := tm.StartPreset(7, t0); ev != EvNone || tm.Minutes() != 25 {
		t.Fatalf("7 is not a preset: ev=%q minutes=%d", ev, tm.Minutes())
	}
	if ev := tm.StartPreset(45, t0); ev != EvStart || tm.Minutes() != 45 || !tm.EndAt.Equal(t0.Add(45*time.Minute)) {
		t.Fatalf("45: ev=%q %+v", ev, tm)
	}
}

func TestTitleAndNextTick(t *testing.T) {
	tm := NewTimer(25)
	if tm.Title(t0) != "" {
		t.Fatalf("idle title %q", tm.Title(t0))
	}
	tm.Start(t0)
	now := t0.Add(1500 * time.Millisecond) // 24:58.5 left, shown as 24:59
	if got := tm.Title(now); got != "24:59" {
		t.Fatalf("title %q, want 24:59", got)
	}
	if got := tm.NextTick(now); !got.Equal(t0.Add(2 * time.Second)) {
		t.Fatalf("next tick %v, want 2 s after start", got.Sub(t0))
	}
	last := t0.Add(25*time.Minute - 400*time.Millisecond)
	if got := tm.NextTick(last); !got.Equal(tm.EndAt) {
		t.Fatalf("last second ticks at %v, want the end", got.Sub(t0))
	}
	tm.Pause(now)
	if got := tm.Title(now); got != "Paused 24:59" {
		t.Fatalf("paused title %q", got)
	}
	tm.Resume(now)
	tm.Finish()
	if got := tm.Title(now); got != "Time’s up" {
		t.Fatalf("done title %q", got)
	}
}

func TestTally(t *testing.T) {
	var y Tally
	if got := y.Text(t0); got != "No timers finished today yet." {
		t.Fatalf("empty: %q", got)
	}
	y.Record(t0, 25*time.Minute, false)
	if got := y.Text(t0); got != "Today: 1 timer, 25 min focused" {
		t.Fatalf("one: %q", got)
	}
	y.Record(t0, 50*time.Minute, false)
	y.Record(t0, time.Minute, true)
	if got := y.Text(t0); got != "Today: 2 timers, 1 h 16 min focused" {
		t.Fatalf("three: %q", got)
	}
	tomorrow := t0.Add(24 * time.Hour)
	if got := y.Text(tomorrow); got != "No timers finished today yet." {
		t.Fatalf("next day: %q", got)
	}
	y.Record(tomorrow, 60*time.Minute, false)
	if y.Count != 1 || y.Text(tomorrow) != "Today: 1 timer, 1 h focused" {
		t.Fatalf("rollover: %+v %q", y, y.Text(tomorrow))
	}
}
