package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

// A word when something you were waiting for finishes.
//
// The checks on a branch take minutes and sometimes an hour, and the only way
// to know how they went is to keep looking at the column. So when a run that
// was going turns green or red, grove says so once and stops.
//
// It goes through the operating system's own notifier rather than Wails'. The
// Wails one wants a signed bundle to register with the notification centre,
// and grove is not signed yet; osascript works today, works for the command as
// well as the window, and asks nothing of anybody.

func notify(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		// AppleScript string literals: quotes and backslashes are the only
		// things that can end one early
		esc := func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			return strings.ReplaceAll(s, `"`, `\"`)
		}
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, esc(body), esc(title))
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		return exec.Command("notify-send", title, body).Run()
	}
	return fmt.Errorf("no notifier on %s", runtime.GOOS)
}

// handleNotify is the page saying something finished. It carries no link and
// opens nothing: the worst it can do is put a line on the screen.
func (d *grove) handleNotify(w http.ResponseWriter, r *http.Request) {
	if loadSettings().NoNotify {
		writeJSON(w, http.StatusOK, map[string]any{"sent": false, "off": true})
		return
	}
	var req struct{ Title, Body string }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Title == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("nothing to say"))
		return
	}
	if err := notify(req.Title, req.Body); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}
