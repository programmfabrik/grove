package main

import (
	"runtime"
	"testing"
)

// safeRepoPath guards the two endpoints that take a path from the browser and
// hand it to git, so its failures are the interesting cases.
func TestSafeRepoPath(t *testing.T) {
	ok := []string{"a.go", "ui/src/App.tsx", "a/b/c.txt", "./a.go", "dir/-notaflag"}
	for _, p := range ok {
		if !safeRepoPath(p) {
			t.Errorf("safeRepoPath(%q) = false, want true", p)
		}
	}
	bad := []string{
		"",                   // nothing to diff
		"../outside.go",      // out of the checkout
		"a/../../outside.go", // …the long way round
		"--output=/tmp/x",    // an option, not a path
		"-x",                 //
	}
	// The shapes that escape a checkout are not the same on both platforms,
	// and the Windows ones are the reason this guard is IsLocal: `\etc\passwd`
	// is rooted but names no drive, so IsAbs called it relative and let it by.
	if runtime.GOOS == "windows" {
		bad = append(bad, `\etc\passwd`, `C:\Windows\System32`, `C:relative`, "NUL", "COM1")
	} else {
		bad = append(bad, "/etc/passwd")
	}
	for _, p := range bad {
		if safeRepoPath(p) {
			t.Errorf("safeRepoPath(%q) = true, want false", p)
		}
	}
}

func TestStatusWord(t *testing.T) {
	for in, want := range map[string]string{
		"??":   "new",
		"M":    "modified",
		"MM":   "modified",
		"A":    "added",
		"D":    "deleted",
		"R":    "renamed",
		"R100": "renamed",
		" M":   "modified",
		"T":    "T", // a type change has no word of its own; it keeps its code
	} {
		if got := statusWord(in); got != want {
			t.Errorf("statusWord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIgnoreArgs(t *testing.T) {
	if got := ignoreArgs(false); got != nil {
		t.Errorf("ignoreArgs(false) = %v, want nil", got)
	}
	got := ignoreArgs(true)
	if len(got) != len(commentIgnores)*2 {
		t.Fatalf("ignoreArgs(true) has %d args, want %d", len(got), len(commentIgnores)*2)
	}
	for i := 0; i < len(got); i += 2 {
		if got[i] != "-I" {
			t.Errorf("arg %d = %q, want -I", i, got[i])
		}
	}
}
