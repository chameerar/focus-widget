package main

import (
	"encoding/binary"
	"io"
	"math"
	"testing"
	"time"
)

func readAll(t *testing.T, r io.Reader) []float32 {
	t.Helper()
	var out []float32
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		for i := 0; i+4 <= n; i += 4 {
			out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))
		}
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestChimeStartsAfterTheDelay(t *testing.T) {
	s := readAll(t, newChime(alarmTones(), 500*time.Millisecond))
	silence := sampleRate / 2
	for i, v := range s[:silence] {
		if v != 0 {
			t.Fatalf("sound at sample %d, before the delay", i)
		}
	}
	// The first beep has risen within its 12 ms attack.
	if peak := maxAbs(s[silence : silence+sampleRate*12/1000+1]); peak < 0.2 {
		t.Fatalf("attack peak %.3f", peak)
	}
	// 12 rounds 2.2 s apart, the last ending 0.59 s into its round.
	if want := silence + int((11*2.2+0.59)*sampleRate); len(s) != want {
		t.Fatalf("%d samples, want %d", len(s), want)
	}
	if peak := maxAbs(s); peak > 0.281 {
		t.Fatalf("peak %.3f is above the beep volume", peak)
	}
}

func TestConfirmToneIsShort(t *testing.T) {
	s := readAll(t, newChime(confirmTones, 0))
	if d := time.Duration(len(s)) * time.Second / sampleRate; d > 200*time.Millisecond {
		t.Fatalf("confirm tone lasts %v", d)
	}
}

func maxAbs(s []float32) float32 {
	var m float32
	for _, v := range s {
		m = max(m, float32(math.Abs(float64(v))))
	}
	return m
}
