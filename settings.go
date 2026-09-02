package main

import (
	"encoding/json"
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
