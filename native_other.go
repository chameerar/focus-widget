//go:build !darwin

package main

// stayAwake does nothing here: App Nap is macOS only.
func stayAwake(bool) {}

// controlClick is only asked on macOS, where Control-click is a right-click.
func controlClick() bool { return false }

// displayAsleep is only asked on macOS, where a dark wake has no audio.
func displayAsleep() bool { return false }
