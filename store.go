package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
)

// Settings are what the app remembers between runs.
type Settings struct {
	Size        string `json:"size"` // a key of Sizes
	AlwaysOnTop bool   `json:"alwaysOnTop"`
	X           *int   `json:"x,omitempty"` // window position, unset until the window is moved
	Y           *int   `json:"y,omitempty"`
	Sound       bool   `json:"sound"`
	LastMinutes int    `json:"lastMinutes"`
	Today       Tally  `json:"today"`
}

// Sizes are the window's side lengths.
var Sizes = map[string]int{"small": 180, "medium": 300, "large": 380}

func defaultSettings() Settings {
	return Settings{Size: "medium", AlwaysOnTop: true, Sound: true, LastMinutes: 25}
}

const (
	settingsFile = "settings.json"
	electronFile = "widget-state.json" // saved by the Electron versions (0.1.x)
)

// Store keeps Settings in one JSON file in the user's config folder, the same folder the
// Electron versions used: ~/Library/Application Support/Focus Widget on macOS,
// %AppData%\Focus Widget on Windows and ~/.config/Focus Widget on Linux.
type Store struct{ dir string }

// configDir overrides the folder in measurement builds, so tests leave real settings alone.
var configDir string

func NewStore() (*Store, error) {
	if configDir != "" {
		return &Store{dir: configDir}, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{dir: filepath.Join(base, appName)}, nil
}

// Load reads the settings. On first run it imports the window settings the Electron version
// saved, without changing that file. A file that can't be read gives the defaults.
func (s *Store) Load() Settings {
	data, err := os.ReadFile(filepath.Join(s.dir, settingsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return s.importElectron()
	}
	set := defaultSettings()
	if err == nil {
		err = json.Unmarshal(data, &set)
	}
	if err != nil {
		log.Printf("settings: using defaults: %v", err)
		return defaultSettings()
	}
	set.normalize()
	return set
}

func (s *Store) importElectron() Settings {
	set := defaultSettings()
	data, err := os.ReadFile(filepath.Join(s.dir, electronFile))
	if err != nil {
		return set
	}
	var old struct {
		Size        *string  `json:"size"`
		AlwaysOnTop *bool    `json:"alwaysOnTop"`
		X           *float64 `json:"x"`
		Y           *float64 `json:"y"`
	}
	if json.Unmarshal(data, &old) != nil {
		return set
	}
	if old.Size != nil {
		set.Size = *old.Size
	}
	if old.AlwaysOnTop != nil {
		set.AlwaysOnTop = *old.AlwaysOnTop
	}
	if old.X != nil && old.Y != nil {
		x, y := int(math.Round(*old.X)), int(math.Round(*old.Y))
		set.X, set.Y = &x, &y
	}
	set.normalize()
	return set
}

func (set *Settings) normalize() {
	if _, ok := Sizes[set.Size]; !ok {
		set.Size = "medium"
	}
	set.LastMinutes = clampMinutes(set.LastMinutes)
	if (set.X == nil) != (set.Y == nil) {
		set.X, set.Y = nil, nil
	}
	if set.Today.Count < 0 || set.Today.Minutes < 0 || math.IsNaN(set.Today.Minutes) {
		set.Today = Tally{}
	}
}

// Save writes the settings through a temporary file, so a crash never leaves half a file.
func (s *Store) Save(set Settings) error {
	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".settings-*.json")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o600)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(s.dir, settingsFile))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
