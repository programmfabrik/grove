package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

// A submodule kept as its own clone, with a linked worktree inside every
// checkout of the parent, is one repository whose checkouts are all called the
// same: each sits in a folder named after the submodule. Keyed by name, picking
// one picked all of them, and every request answered for whichever came first.
func TestWorktreesWithOneNameAreEachThemselves(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	lib := initRepo(t, filepath.Join(dir, "lib"))
	one := filepath.Join(dir, "work", "one", "lib")
	two := filepath.Join(dir, "work", "two", "lib")
	gitRun(t, lib, "worktree", "add", "-q", "-b", "one", one)
	gitRun(t, lib, "worktree", "add", "-q", "-b", "two", two)
	write(t, two, "only-in-two.txt", "x\n")

	d := &grove{
		opt:   options{dir: normPath(dir), refresh: time.Minute},
		state: map[string]*repoState{},
	}
	srv := httptest.NewServer(d.routes(true))
	defer srv.Close()
	get := func(path string, into any) {
		t.Helper()
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %s", path, res.Status)
		}
		if err := json.NewDecoder(res.Body).Decode(into); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
	}

	var state State
	get("/api/state?repo="+url.QueryEscape(lib), &state)
	if len(state.Checkouts) != 3 {
		t.Fatalf("checkouts = %+v, want three", state.Checkouts)
	}
	ids := map[string]bool{}
	for _, c := range state.Checkouts {
		if c.Name != "lib" {
			t.Errorf("%s is shown as %q, want the folder's name, lib", c.Path, c.Name)
		}
		if ids[c.ID] {
			t.Errorf("two checkouts share the ID %q", c.ID)
		}
		ids[c.ID] = true
	}

	// each asks about itself, and only the one with the new file has it
	for _, c := range state.Checkouts {
		var files DiffList
		get("/api/diff?name="+url.QueryEscape(c.ID)+"&repo=lib&scope=unstaged", &files)
		has := false
		for _, f := range files.Files {
			has = has || f.Path == "only-in-two.txt"
		}
		if want := samePath(c.Path, two); has != want {
			t.Errorf("%s: only-in-two.txt listed = %v, want %v", c.Path, has, want)
		}
	}

	// and a request that arrives before anything was scanned finds it too
	cold := &grove{opt: d.opt, state: map[string]*repoState{}}
	if c, ok := cold.checkout(normPath(two)); !ok || !samePath(c.Path, two) {
		t.Errorf("cold lookup of %s = %+v, %v", two, c, ok)
	}
	if _, ok := cold.checkout("lib"); ok {
		t.Error("a bare name, which three checkouts share, was taken to mean one of them")
	}
}
