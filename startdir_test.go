package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasRepos(t *testing.T) {
	dir := t.TempDir()
	if hasRepos(dir) {
		t.Error("an empty directory claimed to hold repositories")
	}
	if err := os.MkdirAll(filepath.Join(dir, "notarepo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if hasRepos(dir) {
		t.Error("a plain directory was taken for a checkout")
	}
	if err := os.MkdirAll(filepath.Join(dir, "myrepo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasRepos(dir) {
		t.Error("a directory holding a checkout was not recognised")
	}
	if hasRepos(filepath.Join(dir, "nowhere")) {
		t.Error("a directory that does not exist claimed to hold repositories")
	}
	// a dotted directory is skipped by the repo scan, so it must not count here
	dotted := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dotted, ".hidden", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if hasRepos(dotted) {
		t.Error("a hidden directory counted as a repository the list would show")
	}
}

// An app launched from an icon has a working directory of "/", so the chooser
// has to be pointed somewhere better than the root of the disk.
func TestLikelyStartDirPrefersSourceOverHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows reads this one

	// nothing recognisable yet: the home directory beats "/"
	if got := likelyStartDir(); got != home {
		t.Errorf("likelyStartDir() = %q, want the home directory %q", got, home)
	}
	// a src/ with something in it is the answer
	src := filepath.Join(home, "src")
	if err := os.MkdirAll(filepath.Join(src, "myrepo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := likelyStartDir(); got != src {
		t.Errorf("likelyStartDir() = %q, want %q", got, src)
	}
	// an empty src/ is not an answer — the list is of directories that HOLD
	// repositories, not of names that happen to exist
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("USERPROFILE", empty)
	if err := os.MkdirAll(filepath.Join(empty, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := likelyStartDir(); got != empty {
		t.Errorf("an empty src/ was offered: got %q, want %q", got, empty)
	}
}
