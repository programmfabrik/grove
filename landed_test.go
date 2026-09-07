package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A squash merge is the case no count can see. The branch's commits never
// become ancestors of the base, so it goes on reporting N ahead forever, while
// the base holds every line of them under one commit of its own. landedInto is
// what tells that apart from work nobody has merged.
func TestLandedIntoSeesASquashMerge(t *testing.T) {
	dir := t.TempDir()
	repo := initRepo(t, filepath.Join(dir, "r"))
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// a branch with two commits of its own
	gitRun(t, repo, "checkout", "-q", "-b", "work")
	write("b.txt", "one\n")
	gitRun(t, repo, "add", "b.txt")
	gitRun(t, repo, "commit", "-q", "-m", "b one")
	write("b.txt", "one\ntwo\n")
	gitRun(t, repo, "commit", "-q", "-am", "b two")

	if landedInto(repo, "main", treeOf(repo, "main")) {
		t.Error("work is not in main yet, but landedInto says it is")
	}

	// main takes the CONTENT as one commit of its own, and moves on
	gitRun(t, repo, "checkout", "-q", "main")
	gitRun(t, repo, "merge", "-q", "--squash", "work")
	gitRun(t, repo, "commit", "-q", "-m", "the work, squashed")
	write("c.txt", "unrelated\n")
	gitRun(t, repo, "add", "c.txt")
	gitRun(t, repo, "commit", "-q", "-m", "and main moves on")
	gitRun(t, repo, "checkout", "-q", "work")

	// the counts still read as unmerged work — that is the whole problem
	if out, err := git(repo, "rev-list", "--count", "main..HEAD"); err != nil || out != "2" {
		t.Fatalf("ahead = %q (%v), want 2 — the fixture is not the case under test", out, err)
	}
	if err := gitErr(repo, "merge-base", "--is-ancestor", "HEAD", "main"); err == nil {
		t.Fatal("HEAD became an ancestor of main; that is a real merge, not a squash")
	}

	if !landedInto(repo, "main", treeOf(repo, "main")) {
		t.Error("main holds every line of this branch, but landedInto says it does not")
	}

	// one more commit on the branch and it is no longer all in main
	write("b.txt", "one\ntwo\nthree\n")
	gitRun(t, repo, "commit", "-q", "-am", "b three")
	if landedInto(repo, "main", treeOf(repo, "main")) {
		t.Error("a commit main has never seen still reads as landed")
	}
}

func TestLandedIntoWithoutABase(t *testing.T) {
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	if landedInto(repo, "", "") {
		t.Error("no base is not a landing")
	}
	if landedInto(repo, "nosuchbranch", treeOf(repo, "nosuchbranch")) {
		t.Error("a base that does not exist is not a landing")
	}
	if got := treeOf(repo, "nosuchbranch"); got != "" {
		t.Errorf("treeOf(missing) = %q, want empty", got)
	}
}

// gitErr runs git for its exit status alone.
func gitErr(dir string, args ...string) error {
	_, err := git(dir, args...)
	return err
}
