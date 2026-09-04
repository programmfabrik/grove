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

// Writing down where grove is looking must not take everything else with it.
// Constructing a fresh settings value and saving it discards every other
// preference, which is how a launch from a directory that happened to hold
// repositories quietly reset somebody's editor, terminal and browser.
func TestRememberingADirectoryKeepsTheRest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	before := settings{
		Dir: "/somewhere", Browser: "safari", Terminal: "warp", Editor: "vscode",
		NoChecks: true, RefreshSeconds: 60, Recent: []string{"/somewhere"},
	}
	if err := before.save(); err != nil {
		t.Fatal(err)
	}

	remember("/elsewhere")

	got := loadSettings()
	if got.Dir != "/elsewhere" {
		t.Errorf("dir = %q, want /elsewhere", got.Dir)
	}
	for _, c := range []struct{ name, got, want string }{
		{"browser", got.Browser, "safari"},
		{"terminal", got.Terminal, "warp"},
		{"editor", got.Editor, "vscode"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q — remembering a directory wiped it", c.name, c.got, c.want)
		}
	}
	if !got.NoChecks || got.RefreshSeconds != 60 || len(got.Recent) != 1 {
		t.Errorf("the rest was lost: %+v", got)
	}
}
