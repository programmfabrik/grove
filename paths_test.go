package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormPathSeparators(t *testing.T) {
	// git's spelling of a path it just printed, on the platform running this
	got := normPath("a/b/../c")
	want := filepath.Join("a", "c")
	if got != want {
		t.Errorf("normPath(a/b/../c) = %q, want %q", got, want)
	}
	if normPath("") != "" {
		t.Errorf("normPath(\"\") = %q, want empty", normPath(""))
	}
}

// A repository reached through a symlink is the case git and the OS disagree
// about on every platform: `git worktree list` reports where the directory
// really is, os.Getwd reports the way in. macOS puts /tmp behind such a link.
func TestNormPathResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err) // Windows without developer mode
	}
	if got := normPath(link); got != normPath(real) {
		t.Errorf("normPath(%q) = %q, want %q", link, got, normPath(real))
	}
	if !samePath(link, real) {
		t.Errorf("samePath(%q, %q) = false, want true", link, real)
	}
}

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "src", "repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// the same directory, spelled the way git prints it and the way Go builds it
	if !samePath(filepath.ToSlash(sub), sub) {
		t.Errorf("samePath disagreed about %q spelled with slashes", sub)
	}
	if samePath(sub, dir) {
		t.Errorf("samePath(%q, %q) = true, want false", sub, dir)
	}
	// case folds on Windows and only there
	if runtime.GOOS == "windows" && !samePath(sub, filepath.Join(dir, "SRC", "REPO")) {
		t.Error("samePath should fold case on Windows")
	}
}
