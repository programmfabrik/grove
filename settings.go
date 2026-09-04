package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// What the app has to remember and the command never does. A command is
// started in the directory it is meant to show — that is what `cd ~/src &&
// grove` means — while an application is started from an icon, with a working
// directory of `/`, and can only know where to look because it was told once
// and wrote it down.
//
// It is deliberately one file with one field. Everything else the dashboard
// remembers — pane widths, folds, the theme — lives in the page's own
// localStorage, which is why the desktop build keeps a stable port.

type settings struct {
	Dir string `json:"dir"` // the directory of repositories last opened

	// The three things grove does that reach off the machine, each of which
	// somebody may reasonably not want. Stored as negatives so that the zero
	// value — a settings file that has never been written — means everything
	// on, which is what a first run should be.
	// Which program to hand a link, a directory or a file to. Empty means
	// whatever this machine would do on its own.
	Browser  string `json:"browser,omitempty"`
	Terminal string `json:"terminal,omitempty"`
	Editor   string `json:"editor,omitempty"`

	NoChecks      bool `json:"no_checks,omitempty"`
	NoAutoFetch   bool `json:"no_auto_fetch,omitempty"`
	NoUpdateCheck bool `json:"no_update_check,omitempty"`
	NoNotify      bool `json:"no_notify,omitempty"`

	// RefreshSeconds is how often a repository's worktrees are re-scanned.
	// Zero means whatever -refresh said, which is what it always was.
	RefreshSeconds int `json:"refresh_seconds,omitempty"`
	// Recent is the directories grove has been pointed at, newest first, so
	// going back to one is a click rather than a folder dialog.
	Recent []string `json:"recent,omitempty"`

	// LoginItem is not stored here — it IS the LaunchAgent file, and one
	// truth beats two that can disagree. See loginitem.go.
}

// settingsPath is the OS's own place for it: Application Support on macOS,
// %AppData% on Windows, $XDG_CONFIG_HOME on Linux.
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "grove", "settings.json"), nil
}

// loadSettings never fails loudly: a missing or unreadable file means nothing
// was remembered, which is the same thing a first launch means.
func loadSettings() settings {
	var s settings
	path, err := settingsPath()
	if err != nil {
		return s
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	json.Unmarshal(buf, &s)
	return s
}

func (s settings) save() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(buf, '\n'), 0o644)
}

// likelyStartDir is where to point the folder chooser when there is nothing
// remembered and nothing to show. An application launched from an icon has a
// working directory of "/", and a first window that opens on the filesystem
// root, finds nothing, and says nothing is the worst first impression grove
// could make.
//
// It suggests rather than decides: the directories below are where people
// actually keep checkouts, and the first one that HOLDS a repository is
// offered — offered, in a dialog somebody confirms, because guessing wrong and
// silently opening somewhere unexpected is its own kind of rude.
func likelyStartDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{
		"src", "Projects", "projects", "code", "Code", "dev", "Developer",
		"git", "repos", "workspace", "work",
		filepath.Join("go", "src", "github.com"),
	} {
		if p := filepath.Join(home, name); hasRepos(p) {
			return p
		}
	}
	// nothing recognisable: the home directory is a better place to start
	// looking from than the root of the disk
	return home
}

// hasRepos says whether a directory holds a git checkout at its top level —
// the same thing the repo list looks for, stopping at the first one found.
func hasRepos(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), ".git")); err == nil {
			return true
		}
	}
	return false
}

// handleSettings reads and writes the ones somebody can change. The window
// showing them and the window using them are the same origin, so a change here
// is a change everywhere as soon as either asks again.
func (d *grove) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, loadSettings())
		return
	}
	var in settings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return
	}
	cur := loadSettings()
	// Dir and Recent are not settable from here: what grove watches is always
	// somewhere somebody actually pointed at, through the folder picker
	in.Dir, in.Recent = cur.Dir, cur.Recent
	if err := in.save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// remember writes down where grove is looking WITHOUT taking anything else
// with it. Building a fresh settings value and saving it discards every other
// preference in the file, which is how a launch from a directory that happened
// to hold repositories quietly reset somebody's editor, terminal and browser.
func remember(dir string) {
	s := loadSettings()
	if s.Dir == dir {
		return
	}
	s.Dir = dir
	s.save()
}
