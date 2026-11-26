package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaultsWithoutFiles(t *testing.T) {
	s := &Store{dir: t.TempDir()}
	if got := s.Load(); got != defaultSettings() {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadImportsElectronSettings(t *testing.T) {
	dir := t.TempDir()
	old := `{"size":"large","alwaysOnTop":false,"x":120.4,"y":-30}`
	writeFile(t, filepath.Join(dir, electronFile), old)
	got := (&Store{dir: dir}).Load()
	if got.Size != "large" || got.AlwaysOnTop || got.X == nil || *got.X != 120 || *got.Y != -30 {
		t.Fatalf("got %+v", got)
	}
	if !got.Sound || got.LastMinutes != 25 {
		t.Fatalf("page settings should start at their defaults: %+v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, electronFile))
	if string(data) != old {
		t.Fatal("the Electron file was changed")
	}
}

func TestLoadIgnoresElectronFileOnceSaved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, electronFile), `{"size":"large"}`)
	s := &Store{dir: dir}
	set := defaultSettings()
	set.Size = "small"
	if err := s.Save(set); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(); got.Size != "small" {
		t.Fatalf("size %q, want small", got.Size)
	}
}

func TestSaveRoundTripAndMode(t *testing.T) {
	s := &Store{dir: filepath.Join(t.TempDir(), "Focus Widget")}
	x, y := 10, 20
	want := Settings{Size: "large", AlwaysOnTop: false, X: &x, Y: &y, Sound: false, LastMinutes: 45,
		Today: Tally{Day: "2026-11-25", Count: 2, Minutes: 50}}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if got.Size != want.Size || got.AlwaysOnTop || *got.X != 10 || *got.Y != 20 || got.Sound ||
		got.LastMinutes != 45 || got.Today != want.Today {
		t.Fatalf("got %+v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(s.dir, settingsFile))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, err %v", info.Mode().Perm(), err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(s.dir, ".settings-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func TestLoadRepairsBadValues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, settingsFile),
		`{"size":"huge","lastMinutes":500,"x":5,"today":{"day":"2026-11-25","count":-1,"minutes":3}}`)
	got := (&Store{dir: dir}).Load()
	if got.Size != "medium" || got.LastMinutes != 99 || got.X != nil || got.Today != (Tally{}) {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadCorruptFileGivesDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, settingsFile), `{"size":`)
	if got := (&Store{dir: dir}).Load(); got != defaultSettings() {
		t.Fatalf("got %+v", got)
	}
}
