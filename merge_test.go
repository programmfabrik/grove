package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// A merge is what it brought into its branch. `git show` gives a merge a
// combined diff — only what differs from EVERY parent, the files the merge had
// to reconcile — so a merge of a whole branch into main listed almost nothing,
// and `git log --numstat` gives it no counts at all.
func TestAMergeShowsWhatItBroughtIn(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	gitRun(t, repo, "checkout", "-q", "-b", "feature")
	write(t, repo, "one.txt", "1\n2\n")
	gitRun(t, repo, "add", "one.txt")
	gitRun(t, repo, "commit", "-q", "-m", "one")
	write(t, repo, "two.txt", "3\n")
	gitRun(t, repo, "add", "two.txt")
	gitRun(t, repo, "commit", "-q", "-m", "two")
	gitRun(t, repo, "checkout", "-q", "main")
	write(t, repo, "own.txt", "main's own\n") // so the merge is a real one
	gitRun(t, repo, "add", "own.txt")
	gitRun(t, repo, "commit", "-q", "-m", "own")
	gitRun(t, repo, "merge", "-q", "--no-edit", "feature")

	merge, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	var row *Scope
	for _, s := range commitScopes(repo, "main") {
		if s.Sha == merge[:9] {
			row = &s
			break
		}
	}
	if row == nil {
		t.Fatal("the merge is not in the commit list")
	}
	if row.Files != 2 || row.Added != 3 {
		t.Errorf("the merge's row counts %d files, +%d; want 2 files, +3", row.Files, row.Added)
	}

	spec, err := resolveScope(repo, "r", "main", "r", "commit:"+merge)
	if err != nil {
		t.Fatal(err)
	}
	files, err := scopeFiles(repo, spec, ignoring{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Path)
	}
	if strings.Join(got, " ") != "one.txt two.txt" {
		t.Errorf("the merge lists %v, want what feature brought: one.txt two.txt", got)
	}
	text, _, err := fileDiff(repo, "one.txt", false, spec, ignoring{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "+1\n+2\n") {
		t.Errorf("one.txt in the merge reads\n%s\nwant its two added lines", text)
	}
}

// A merge of main that is staged but not yet committed is not this branch's
// work. Measured from the old fork point, everything main gained since then
// was listed under "vs main" as uncommitted — a whole release of other people's
// commits that `git diff main` does not show, because the tree has all of it.
func TestAMergeInProgressIsNotTheBranchsOwnWork(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	gitRun(t, repo, "checkout", "-q", "-b", "feature")
	write(t, repo, "mine.txt", "the branch's own\n")
	gitRun(t, repo, "add", "mine.txt")
	gitRun(t, repo, "commit", "-q", "-m", "mine")
	gitRun(t, repo, "checkout", "-q", "main")
	write(t, repo, "theirs.txt", "main moved on\n")
	gitRun(t, repo, "add", "theirs.txt")
	gitRun(t, repo, "commit", "-q", "-m", "theirs")
	gitRun(t, repo, "checkout", "-q", "feature")
	gitRun(t, repo, "merge", "-q", "--no-commit", "--no-ff", "main")

	spec, err := resolveScope(repo, "r", "main", "r", "base")
	if err != nil {
		t.Fatal(err)
	}
	files, err := scopeFiles(repo, spec, ignoring{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Path)
	}
	if strings.Join(got, " ") != "mine.txt" {
		t.Errorf("vs main mid-merge lists %v, want only the branch's own mine.txt", got)
	}
	for _, s := range repoScopes(repo, "r", "main", "r").Scopes {
		if s.ID == "base" && s.Files != 1 {
			t.Errorf("vs main mid-merge counts %d files, want 1", s.Files)
		}
	}
}
