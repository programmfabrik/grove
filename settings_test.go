package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	// UserConfigDir reads the environment, so the test gets its own
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if got := loadSettings().Dir; got != "" {
		t.Errorf("a first launch remembered %q", got)
	}
	want := filepath.Join(t.TempDir(), "src")
	if err := (settings{Dir: want}).save(); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings().Dir; got != want {
		t.Errorf("loadSettings().Dir = %q, want %q", got, want)
	}
}

// Nothing about a missing or broken file should stop the app starting: it says
// the same thing a first launch says.
func TestSettingsSurvivesRubbish(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	path, err := settingsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings().Dir; got != "" {
		t.Errorf("rubbish parsed as %q", got)
	}
}
