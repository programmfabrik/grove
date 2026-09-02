package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
