//go:build !darwin

package main

// stayAwake does nothing here: App Nap is macOS only.
func stayAwake(bool) {}

// controlClick is only asked on macOS, where Control-click is a right-click.
func controlClick() bool { return false }
