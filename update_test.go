package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The whole notice hangs off this, and a false positive nags somebody about a
// release they already have.
func TestNewerVersion(t *testing.T) {
	newer := [][2]string{
		{"v0.2.0", "v0.1.0"},
		{"v1.0.0", "v0.9.9"},
		{"v0.1.1", "v0.1.0"},
		{"v0.10.0", "v0.9.0"}, // ten is after nine, whatever a string sort says
		{"0.2.0", "v0.1.0"},   // the v is optional on either side
	}
	for _, c := range newer {
		if !newerVersion(c[0], c[1]) {
			t.Errorf("newerVersion(%q, %q) = false, want true", c[0], c[1])
		}
	}
	same := [][2]string{
		{"v0.1.0", "v0.1.0"},
		{"v0.1.0", "v0.2.0"}, // older
		{"v0.1.0", "dev"},    // a build from a working tree is never out of date
		{"dev", "v0.1.0"},
		{"", "v0.1.0"},
		{"v0.1.0-rc1", "v0.1.0"}, // a pre-release of what is already installed
		{"garbage", "v0.1.0"},
		{"v1.2.3.4", "v0.1.0"}, // not a shape this understands: say nothing
	}
	for _, c := range same {
		if newerVersion(c[0], c[1]) {
			t.Errorf("newerVersion(%q, %q) = true, want false", c[0], c[1])
		}
	}
}

// The point of naming the asset is that a notice offers the file that replaces
// this build, so the name has to be one the release actually carries.
func TestAssetNameMatchesTheRelease(t *testing.T) {
	published := map[string]bool{
		"Grove-macos-universal.zip":        true,
		"Grove-windows-amd64.zip":          true,
		"grove-cli-macos-universal.tar.gz": true,
		"grove-cli-windows-amd64.zip":      true,
		"grove-cli-linux-amd64.tar.gz":     true,
		"grove-cli-linux-arm64.tar.gz":     true,
	}
	old := distKind
	defer func() { distKind = old }()

	distKind = "cli"
	if got := assetName(); !published[got] {
		t.Errorf("the command on %s/%s wants %q, which no release carries", runtime.GOOS, runtime.GOARCH, got)
	}
	distKind = "app"
	got := assetName()
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if !published[got] {
			t.Errorf("the app on %s wants %q, which no release carries", runtime.GOOS, got)
		}
	} else if got != "" {
		t.Errorf("the app names %q on %s, where there is no app to download", got, runtime.GOOS)
	}
}

// -no-update-check has to mean it: no network, and an endpoint that still
// answers with what is running.
func TestNoUpdateCheckMakesNoRequest(t *testing.T) {
	d := &grove{state: map[string]*repoState{}} // up is nil: the flag was given
	srv := httptest.NewServer(d.routes(true))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/version")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var u Update
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	if u.Version != version {
		t.Errorf("version = %q, want %q", u.Version, version)
	}
	if u.Available || u.Latest != "" || u.URL != "" {
		t.Errorf("an update was reported with the check off: %+v", u)
	}
}

// A dev build must never check, whatever is published.
func TestDevBuildNeverChecks(t *testing.T) {
	if version != "dev" {
		t.Skip("this binary carries a release version")
	}
	hit := false
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer stub.Close()

	u := newUpdater()
	u.check(context.Background())
	if hit {
		t.Error("a dev build asked about releases")
	}
	if got := u.get(); got.Available {
		t.Errorf("a dev build reported an update: %+v", got)
	}
}

// The first version of this looked at the executable's own path, which is
// wrong: a cask moves the app to /Applications and keeps only bookkeeping in
// the Caskroom, so the path it runs from says nothing about how it got there.
func TestBrewInstalledLooksAtTheCaskroom(t *testing.T) {
	old := distKind
	defer func() { distKind = old }()

	prefix := t.TempDir()
	t.Setenv("HOMEBREW_PREFIX", prefix)

	distKind = "app"
	if brewInstalled() {
		t.Error("no Caskroom entry, yet it claimed Homebrew manages this")
	}
	if err := os.MkdirAll(filepath.Join(prefix, "Caskroom", "grove", "0.1.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !brewInstalled() {
		t.Error("a Caskroom entry for grove exists and it was not noticed")
	}
	// the command is not what a cask installs, so a cask record says nothing
	// about it — only living under the Cellar would
	distKind = "cli"
	if brewInstalled() {
		t.Error("a cask record made the command claim to be brew-managed")
	}
}
