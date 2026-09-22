package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A submodule's "change" is that it sits on a different commit than the parent
// records. `git restore --worktree` walks that path, declines to enter the
// submodule, and exits 0 having done nothing — so discard reported success and
// changed nothing at all. Only --recurse-submodules actually puts it back.
func TestDiscardMovesASubmoduleBack(t *testing.T) {
	dir := t.TempDir()

	lib := initRepo(t, filepath.Join(dir, "lib")) // commits a.txt as "hi"
	write(t, lib, "a.txt", "two\n")
	gitRun(t, lib, "commit", "-q", "-am", "two")
	second, _ := git(lib, "rev-parse", "HEAD")
	first, _ := git(lib, "rev-parse", "HEAD~1")

	parent := initRepo(t, filepath.Join(dir, "parent"))
	gitRun(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib")
	sub := filepath.Join(parent, "lib")
	gitRun(t, sub, "checkout", "-q", first) // the parent is to record the FIRST
	gitRun(t, parent, "commit", "-q", "-am", "lib at first")

	// move the submodule forward: the state the dashboard shows as modified
	gitRun(t, sub, "checkout", "-q", second)
	if st, _ := git(parent, "status", "--porcelain"); st == "" {
		t.Fatal("the parent sees no change, so there is nothing to discard — bad fixture")
	}

	// The file list has to say this is a submodule, or the dialog cannot warn.
	// Every scope that can show one, not just the range scope: the uncommitted
	// view is where this change appears and where it was acted on.
	for _, kind := range []string{"unstaged", "range"} {
		spec := scopeSpec{kind: kind}
		if kind == "range" {
			spec.from = "HEAD"
		}
		files, err := scopeFiles(parent, spec, false)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		var found *DiffFile
		for i := range files {
			if files[i].Path == "lib" {
				found = &files[i]
			}
		}
		if found == nil {
			t.Fatalf("%s: lib is not in the file list", kind)
		}
		if !found.Submodule {
			t.Errorf("%s: lib is a gitlink and is not marked as one", kind)
		}
	}

	discardVia(t, dir, "parent", "lib")

	if at, _ := git(sub, "rev-parse", "HEAD"); at != first {
		t.Errorf("submodule is at %s, want the recorded %s — discard did nothing", at[:8], first[:8])
	}
	if st, _ := git(parent, "status", "--porcelain"); st != "" {
		t.Errorf("parent still dirty after discarding the submodule: %q", st)
	}
}

// An ordinary file must go on behaving as it did — --recurse-submodules is
// carried by every discard, including in repositories that have none.
func TestDiscardAnOrdinaryFileWithoutSubmodules(t *testing.T) {
	dir := t.TempDir()
	repo := initRepo(t, filepath.Join(dir, "r"))
	write(t, repo, "a.txt", "changed\n")

	discardVia(t, dir, "r", "a.txt")

	got, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi\n" {
		t.Errorf("a.txt = %q, want the committed %q", got, "hi\n")
	}
}

// discardVia goes through handleRevert, so what is under test is the command
// grove builds rather than one the test wrote out for it.
func discardVia(t testing.TB, dir, checkout string, paths ...string) {
	t.Helper()
	d := &grove{
		opt:   options{dir: normPath(dir), refresh: time.Minute},
		state: map[string]*repoState{},
	}
	body, err := json.Marshal(revertRequest{Name: checkout, Action: "discard", Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	d.handleRevert(w, httptest.NewRequest(http.MethodPost, "/api/revert", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/revert: %d %s", w.Code, w.Body.String())
	}
}
