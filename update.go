package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The one thing grove talks to besides git.
//
// Everything else it shows comes from running the commands you would have run
// yourself. This does not: once a day it asks GitHub what the latest release
// is, so that somebody who downloaded a file six months ago finds out there is
// a newer one. A download is otherwise a dead end — Homebrew can upgrade what
// it installed and nothing can upgrade a zip.
//
// It sends nothing. It is a plain unauthenticated GET of a public URL, with no
// identifier of any kind attached; GitHub sees an address and a user agent, as
// it would for anyone fetching the same page. Nothing is downloaded and
// nothing is installed — the answer is a link. `-no-update-check` turns it off
// and then grove makes no network request at all.

const (
	updateAPI   = "https://api.github.com/repos/programmfabrik/grove/releases/latest"
	updateEvery = 24 * time.Hour
)

// distKind is which download this build is, so the notice can point at the one
// file that replaces it rather than at a release page holding seven. Set by
// whichever front door is compiled in.
var distKind = "cli"

// Update is what the dashboard is told about itself.
type Update struct {
	Version   string `json:"version"` // what is running
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available,omitempty"`
	URL       string `json:"url,omitempty"`       // the asset that replaces THIS build
	NotesURL  string `json:"notes_url,omitempty"` // the release itself
	// Homebrew installs are upgraded by Homebrew. Handing one a zip would put
	// a second copy beside the managed one and leave brew describing a version
	// that is no longer there.
	Homebrew bool `json:"homebrew,omitempty"`
}

type updater struct {
	mu      sync.RWMutex
	state   Update
	checked time.Time
}

func newUpdater() *updater {
	return &updater{state: Update{Version: version, Homebrew: brewInstalled()}}
}

func (u *updater) get() Update {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.state
}

// watch checks now and then once a day, in the background: the answer is never
// worth making a page wait for, and a machine that is offline should be no
// slower than one that is not.
func (u *updater) watch(ctx context.Context) {
	for {
		u.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(updateEvery):
		}
	}
}

func (u *updater) check(ctx context.Context) {
	// a build from a working tree has no version to compare against, and
	// telling a developer their own build is out of date is just noise
	if version == "dev" {
		return
	}
	rel, err := latestRelease(ctx)
	if err != nil {
		return // offline, rate-limited, or no release yet: say nothing
	}
	st := Update{Version: version, Homebrew: brewInstalled()}
	if newerVersion(rel.TagName, version) {
		st.Latest, st.Available, st.NotesURL = rel.TagName, true, rel.HTMLURL
		want := assetName()
		for _, a := range rel.Assets {
			if a.Name == want {
				st.URL = a.BrowserDownloadURL
			}
		}
		if st.URL == "" {
			st.URL = rel.HTMLURL // no asset for this platform; the page will do
		}
	}
	u.mu.Lock()
	u.state, u.checked = st, time.Now()
	u.mu.Unlock()
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func latestRelease(ctx context.Context) (*release, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", updateAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "grove/"+version) // GitHub refuses a request without one
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s", res.Status)
	}
	var rel release
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// assetName is the file that replaces this build. The app is one universal
// bundle per platform; the command is per architecture.
func assetName() string {
	if distKind == "app" {
		switch runtime.GOOS {
		case "darwin":
			return "Grove-macos-universal.zip"
		case "windows":
			return "Grove-windows-amd64.zip"
		}
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return "grove-cli-macos-universal.tar.gz"
	case "windows":
		return "grove-cli-windows-" + runtime.GOARCH + ".zip"
	default:
		return "grove-cli-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	}
}

// brewInstalled says this build is managed by Homebrew, in which case the
// answer to a new release is `brew upgrade` and not a download.
//
// Not by looking at its own path, which was the first guess and was wrong: a
// cask MOVES the app to /Applications and keeps only bookkeeping in the
// Caskroom, so the running executable sits at
// /Applications/Grove.app/Contents/MacOS/Grove and says nothing whatever about
// how it got there. What does say so is a Caskroom entry for grove. A command
// installed as a formula is the other way round — it really does live under
// the Cellar — so both are asked.
//
// A hand-built app on a machine that also has the cask would answer yes here
// and be told to brew upgrade. It never gets that far: a build from a working
// tree carries no version and never checks at all.
func brewInstalled() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if strings.Contains(filepath.ToSlash(exe), "/Cellar/") {
		return true // a formula, reached through its link in bin
	}
	if distKind != "app" {
		return false
	}
	for _, prefix := range brewPrefixes() {
		if st, err := os.Stat(filepath.Join(prefix, "Caskroom", "grove")); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

// brewPrefixes is where Homebrew keeps itself: Apple Silicon, Intel, and
// whatever HOMEBREW_PREFIX says when somebody has moved it.
func brewPrefixes() []string {
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		return []string{p}
	}
	return []string{"/opt/homebrew", "/usr/local"}
}

// newerVersion compares two v-prefixed dotted numbers. Anything it cannot read
// — "dev", a hand-built binary, a tag in some other shape — is not newer, so
// an unreadable version can never produce a notice.
func newerVersion(latest, current string) bool {
	l, ok1 := parseVersion(latest)
	c, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return v, false
	}
	// a pre-release suffix is not part of the ordering here; 1.2.3-rc1 sorts
	// as 1.2.3, which is close enough for "is there something newer"
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
