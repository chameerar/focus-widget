//go:build spike

package main

import (
	"bufio"
	"os"
	"strings"
)

// Measurement hook: drive the page from stdin. Built only with -tags spike.
func init() {
	spikeHook = func() {
		go func() {
			s := bufio.NewScanner(os.Stdin)
			s.Buffer(make([]byte, 64*1024), 1024*1024)
			for s.Scan() {
				line := strings.TrimSpace(s.Text())
				switch {
				case line == "hide":
					win.Hide()
				case line == "show":
					win.Show()
				case strings.HasPrefix(line, "eval:"):
					win.ExecJS(line[5:])
				case strings.HasPrefix(line, "cmd:"):
					app.Event.Emit("widget:command", map[string]any{"type": line[4:]})
				}
			}
		}()
	}
}
