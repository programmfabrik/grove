package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// The one place grove writes. Everything else reads: git status, git diff, a
// port probe. Undoing a change from the diff you are looking at is worth the
// exception, but it stays deliberately narrow — the two actions GitHub Desktop
// offers, on paths the caller can see in that same view, and never anything
// that touches a commit.
//
//	POST /api/revert  {"name":"myrepo3","repo":"myrepo","action":"discard","paths":["a.go"]}
//
//	unstage  git restore --staged   — the index goes back to HEAD, the file's
//	                                  working-tree content is untouched
//	discard  git restore --worktree — the working tree goes back to the index;
//	                                  an UNTRACKED file is in no index, so
//	                                  discarding it means deleting it, which is
//	                                  the one step with no git-side undo
//
// A committed change has neither: undoing it is a rebase or a revert commit,
// not a file operation, and the UI does not offer it.

type revertRequest struct {
	Name   string   `json:"name"`
	Repo   string   `json:"repo"`
	Action string   `json:"action"` // unstage | discard
	Paths  []string `json:"paths"`
	// Untracked paths are deleted rather than restored; the caller marks them
	// so the server never has to guess which of the two it is doing.
	Untracked []string `json:"untracked"`
}

func (d *grove) handleRevert(w http.ResponseWriter, r *http.Request) {
	var req revertRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return
	}
	c, ok := d.checkout(req.Name)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", req.Name))
		return
	}
	want := req.Repo
	if want == "" {
		want = primaryRepo(c)
	}
	root := ""
	for _, repo := range diffRepos(c) {
		if repo.Name == want {
			root = repo.Path
		}
	}
	if root == "" {
		writeErr(w, http.StatusNotFound, fmt.Errorf("%s has no %s checkout", c.Name, want))
		return
	}
	if req.Action != "unstage" && req.Action != "discard" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown action %q", req.Action))
		return
	}
	for _, p := range append(append([]string{}, req.Paths...), req.Untracked...) {
		if !safeRepoPath(p) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("path must stay inside the checkout: %q", p))
			return
		}
	}

	done := 0
	if len(req.Paths) > 0 {
		flag := "--staged"
		if req.Action == "discard" {
			flag = "--worktree"
		}
		args := append([]string{"restore", flag, "--"}, req.Paths...)
		if out, err := git(root, args...); err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("git restore: %w: %s", err, out))
			return
		}
		done += len(req.Paths)
	}

	// an untracked file has nothing to restore from; discarding it is deleting
	if req.Action == "discard" {
		for _, p := range req.Untracked {
			if err := os.Remove(filepath.Join(root, filepath.Clean(p))); err != nil && !os.IsNotExist(err) {
				writeErr(w, http.StatusInternalServerError, fmt.Errorf("remove %s: %w", p, err))
				return
			}
			done++
		}
	}

	// the caches describe a tree that just changed underneath them
	d.mu.Lock()
	for _, st := range d.state {
		st.gitAt = time.Time{}
	}
	d.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"reverted": done})
}
