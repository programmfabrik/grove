package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Starting with the machine.
//
// Wails has no service for this, so it is a LaunchAgent — a file in your own
// Library that says "run this at login", which is what macOS does underneath
// every other way of asking. Turning the setting off deletes it again: a
// dashboard that left something behind in the login sequence after being told
// not to would be a poor guest.

const loginLabel = "com.programmfabrik.grove"

func loginAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", loginLabel+".plist"), nil
}

// bundlePath is the .app this binary is inside, if it is inside one. Only a
// bundle is worth starting at login: the bare executable has no dock icon and
// no way to be quit.
func bundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	// …/Grove.app/Contents/MacOS/Grove
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i > 0 {
		return exe[:i+4]
	}
	return ""
}

func loginItemSet(on bool) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("grove only knows how to do this on macOS")
	}
	path, err := loginAgentPath()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	app := bundlePath()
	if app == "" {
		return fmt.Errorf("this is not the bundled Grove.app, so there is nothing to start")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/open</string>
		<string>-a</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`, loginLabel, app)
	return os.WriteFile(path, []byte(plist), 0o644)
}

func loginItemOn() bool {
	path, err := loginAgentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
