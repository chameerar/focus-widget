package main

import (
	"encoding/binary"
	"io"
	"log"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const sampleRate = 48000

// tone is one beep: a sine wave at seconds after the chime starts.
type tone struct{ at, freq, dur, vol float64 }

// alarmTones are three rising beeps, repeated 12 times 2.2 s apart: about 26 s unless dismissed.
func alarmTones() []tone {
	var ts []tone
	for r := range 12 {
		base := float64(r) * 2.2
		ts = append(ts, tone{base, 880, 0.12, 0.28}, tone{base + 0.18, 880, 0.12, 0.28}, tone{base + 0.36, 1175, 0.2, 0.28})
	}
	return ts
}

// confirmTones play when sound is turned on.
var confirmTones = []tone{{0.02, 1175, 0.12, 0.18}}

// chime renders tones after a stretch of silence, as mono 32-bit float samples.
type chime struct {
	tones   []tone
	silence int // samples before the first tone
	pos     int // next sample
	end     int
}

func newChime(tones []tone, delay time.Duration) *chime {
	var length float64
	for _, t := range tones {
		length = max(length, t.at+t.dur+0.03)
	}
	silence := int(max(0, delay.Seconds()) * sampleRate)
	return &chime{tones: tones, silence: silence, end: silence + int(length*sampleRate)}
}

func (c *chime) Read(p []byte) (int, error) {
	if c.pos >= c.end {
		return 0, io.EOF
	}
	n := 0
	for ; n+4 <= len(p) && c.pos < c.end; n += 4 {
		binary.LittleEndian.PutUint32(p[n:], math.Float32bits(c.sample(c.pos)))
		c.pos++
	}
	return n, nil
}

func (c *chime) sample(i int) float32 {
	if i < c.silence {
		return 0
	}
	t := float64(i-c.silence) / sampleRate
	var v float64
	for _, tn := range c.tones {
		if dt := t - tn.at; dt >= 0 && dt < tn.dur {
			v += envelope(dt, tn.dur, tn.vol) * math.Sin(2*math.Pi*tn.freq*dt)
		}
	}
	return float32(v)
}

// envelope matches the earlier Web Audio chime: an exponential rise from 0.0001 to vol in
// 12 ms, then an exponential fall back to 0.0001 at dur.
func envelope(dt, dur, vol float64) float64 {
	const floor, attack = 0.0001, 0.012
	if dt < attack {
		return floor * math.Pow(vol/floor, dt/attack)
	}
	return vol * math.Pow(floor/vol, (dt-attack)/(dur-attack))
}

// Chime plays through the system audio output. The output opens on first use and is suspended
// whenever nothing plays, so it costs nothing between alarms.
type Chime struct {
	once    sync.Once
	ctx     *oto.Context
	err     error
	mu      sync.Mutex
	gen     int // changes on every Ring and Cancel, so a slow start can tell it was overtaken
	alarm   *oto.Player
	players []*oto.Player
	settle  *time.Timer
}

func (s *Chime) context() (*oto.Context, error) {
	s.once.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:      sampleRate,
			ChannelCount:    1,
			Format:          oto.FormatFloat32LE,
			ApplicationName: appName,
		})
		if err == nil {
			<-ready
			err = ctx.Err()
		}
		if err != nil {
			s.err = err
			log.Printf("sound: no audio output: %v", err)
			return
		}
		s.ctx = ctx
	})
	return s.ctx, s.err
}

// Ring plays the alarm so that it starts at the given moment.
func (s *Chime) Ring(at time.Time) {
	s.mu.Lock()
	s.gen++
	gen := s.gen
	s.stopAlarm()
	s.mu.Unlock()
	go func() {
		ctx, err := s.context()
		if err != nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if gen == s.gen {
			s.alarm = s.play(ctx, alarmTones(), time.Until(at))
			debugf("sound: alarm armed %v ahead of %s", time.Until(at).Round(time.Millisecond), at.Format(time.TimeOnly))
		}
	}()
}

// Cancel stops the alarm. A confirm tone that is playing finishes.
func (s *Chime) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	s.stopAlarm()
}

// Confirm plays a short tone now.
func (s *Chime) Confirm() {
	go func() {
		ctx, err := s.context()
		if err != nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.play(ctx, confirmTones, 0)
	}()
}

// play starts a player. Call with the lock held.
func (s *Chime) play(ctx *oto.Context, tones []tone, delay time.Duration) *oto.Player {
	if err := ctx.Resume(); err != nil {
		log.Printf("sound: %v", err)
	}
	p := ctx.NewPlayer(newChime(tones, delay))
	p.Play()
	s.players = append(s.players, p)
	s.settleLater()
	return p
}

// stopAlarm pauses the alarm and lets settle close it. Call with the lock held.
func (s *Chime) stopAlarm() {
	if s.alarm != nil {
		s.alarm.Pause()
		s.alarm = nil
		s.settleLater()
	}
}

func (s *Chime) settleLater() {
	if s.settle != nil {
		s.settle.Stop()
	}
	s.settle = time.AfterFunc(time.Second, s.closeFinished)
}

// closeFinished closes players that are done and suspends the output once none are left.
func (s *Chime) closeFinished() {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := s.players[:0]
	for _, p := range s.players {
		if p.IsPlaying() {
			live = append(live, p)
		} else {
			p.Close()
		}
	}
	clear(s.players[len(live):])
	s.players = live
	if len(live) > 0 {
		s.settleLater()
	} else if s.ctx != nil {
		if err := s.ctx.Suspend(); err != nil {
			log.Printf("sound: %v", err)
		}
	}
}

// debugf logs in measurement builds only.
var debugf = func(string, ...any) {}
