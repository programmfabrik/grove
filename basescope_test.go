package main

import (
	"path/filepath"
	"testing"
)

// "vs main" belongs to every checkout that is not main itself — including one
// whose branch has no commits yet. A branch cut this morning and worked in all
// day has everything uncommitted and nothing committed, and that scope is the
// only one showing the lot; it was the one scope missing from it, because the
// test for "this checkout IS the base branch" was really a test for "the fork
// point is HEAD", which is equally true of a branch nobody has committed on.
func TestBaseScopeIsThereForABranchWithNoCommitsOfItsOwn(t *testing.T) {
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))

	scopeIDs := func() []string {
		t.Helper()
		var ids []string
		for _, s := range repoScopes(repo, "r", "main", "r").Scopes {
			if s.Kind != "commit" {
				ids = append(ids, s.ID)
			}
		}
		return ids
	}
	has := func(ids []string, want string) bool {
		for _, id := range ids {
			if id == want {
				return true
			}
		}
		return false
	}

	// on main itself there is nothing to compare against
	if ids := scopeIDs(); has(ids, "base") {
		t.Errorf("the base checkout offers a scope against itself: %v", ids)
	}

	// a branch of its own, not one commit on it, and a day's work in the tree
	gitRun(t, repo, "checkout", "-q", "-b", "work")
	write(t, repo, "a.txt", "a day of it\n")
	write(t, repo, "new.txt", "and something new\n")

	ids := scopeIDs()
	if !has(ids, "base") {
		t.Fatalf("no vs-main scope on a branch with uncommitted work: %v", ids)
	}
	r := repoScopes(repo, "r", "main", "r")
	if r.Base != "main" {
		t.Errorf("Base = %q, want main — the UI names the branch from this", r.Base)
	}
	for _, s := range r.Scopes {
		if s.ID != "base" {
			continue
		}
		if s.Label != "vs main" {
			t.Errorf("label = %q, want %q", s.Label, "vs main")
		}
		// both the tracked edit and the untracked file, which is what makes
		// this scope worth having over staged and unstaged separately
		if s.Files != 2 {
			t.Errorf("vs main covers %d files, want 2", s.Files)
		}
	}

	// and it survives the branch gaining a commit
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "now committed")
	if ids := scopeIDs(); !has(ids, "base") {
		t.Errorf("vs main went away once the branch had a commit: %v", ids)
	}
}
