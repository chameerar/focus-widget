//go:build measure

package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
)

// Measurement hook: drive the app from stdin and log state and sound timing. Built only with
// -tags measure. FOCUS_WIDGET_CONFIG points the settings at another folder.
//
//	hide | show          hide or show the widget
//	eval:<js>            run script in the page
//	act:<type>[:<min>]   run an action, as a menu would
func init() {
	debugf = log.Printf
	configDir = os.Getenv("FOCUS_WIDGET_CONFIG")
	instanceID += ".measure" // runs beside an installed copy
	measureHook = func(a *App) {
		go func() {
			s := bufio.NewScanner(os.Stdin)
			s.Buffer(make([]byte, 64*1024), 1024*1024)
			for s.Scan() {
				line := strings.TrimSpace(s.Text())
				switch {
				case line == "hide":
					a.win.Hide()
				case line == "show":
					a.win.Show()
				case strings.HasPrefix(line, "eval:"):
					a.win.ExecJS(line[5:])
				case strings.HasPrefix(line, "act:"):
					t, m, _ := strings.Cut(line[4:], ":")
					minutes, _ := strconv.Atoi(m)
					a.act(Action{Type: t, Minutes: minutes})
				}
			}
		}()
	}
}
