//go:build !darwin

package main

// stayAwake does nothing here: App Nap is macOS only.
func stayAwake(bool) {}
