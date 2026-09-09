package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A branch is only being tested if it has been pushed AS ITSELF. Cutting a
// branch from origin/main leaves it tracking origin/main, and taking that
// upstream for "what this branch pushed" hands main's checks to a branch
// nobody has ever pushed — a green tick on work that does not exist anywhere
// but this disk.
func TestPushedTipsIgnoresABranchTrackingAnother(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	gitRun(t, dir, "init", "-q", "--bare", origin)

	repo := initRepo(t, filepath.Join(dir, "r"))
	gitRun(t, repo, "remote", "add", "origin", origin)
	gitRun(t, repo, "push", "-q", "-u", "origin", "main")

	// the branch this is about: cut from origin/main, never pushed
	gitRun(t, repo, "checkout", "-q", "-b", "work", "origin/main")
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "b.txt")
	gitRun(t, repo, "commit", "-q", "-m", "work of my own")

	// the fixture is only worth anything if it reproduces the trap
	if up, err := git(repo, "rev-parse", "--abbrev-ref", "@{upstream}"); err != nil || up != "origin/main" {
		t.Fatalf("upstream = %q (%v), want origin/main — no trap to test", up, err)
	}

	tips := pushedTips(repo)
	if _, ok := tips["work"]; ok {
		t.Errorf("work has never been pushed, but pushedTips gives it %q", tips["work"])
	}
	mainSha, _ := git(repo, "rev-parse", "origin/main")
	if tips["main"] != mainSha {
		t.Errorf("main = %q, want its pushed tip %q", tips["main"], mainSha)
	}

	// push it properly and it earns its own tip — the pushed one, not the local
	gitRun(t, repo, "push", "-q", "-u", "origin", "work")
	pushed, _ := git(repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("newer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "commit", "-q", "-am", "not pushed yet")

	tips = pushedTips(repo)
	if tips["work"] != pushed {
		t.Errorf("work = %q, want the tip it actually pushed %q", tips["work"], pushed)
	}
	if head, _ := git(repo, "rev-parse", "HEAD"); tips["work"] == head {
		t.Error("the local tip was never pushed, so no CI can have seen it")
	}
}

// A branch with no remote at all has nothing to ask about.
func TestPushedTipsWithoutARemote(t *testing.T) {
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	if tips := pushedTips(repo); len(tips) != 0 {
		t.Errorf("pushedTips = %v, want nothing: no remote, nothing pushed", tips)
	}
}
