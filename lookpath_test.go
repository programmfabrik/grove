package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A GUI application does not inherit the shell's PATH, so a lookup that only
// consults the process environment finds nothing a package manager installed —
// and grove then tells somebody to install what they already have.
func TestLookPathReachesBeyondTheProcessEnvironment(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "grove-test-tool")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// the environment a Dock-launched app gets: this directory is not in it
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SHELL", "/bin/sh")
	// and the shell it would ask says the directory IS on the path
	resetSearchPath(t, []string{dir, "/usr/bin", "/bin"})

	got, err := lookPath("grove-test-tool")
	if err != nil {
		t.Fatalf("not found, though the shell would have found it: %v", err)
	}
	if got != bin {
		t.Errorf("found %q, want %q", got, bin)
	}
	if _, err := lookPath("grove-test-tool-that-is-not-there"); err == nil {
		t.Error("found something that does not exist")
	}
}

// A directory, or a file nobody may run, is not a program.
func TestLookPathWantsSomethingRunnable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "adirectory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notrunnable"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resetSearchPath(t, []string{dir})
	for _, name := range []string{"adirectory", "notrunnable"} {
		if p, err := lookPath(name); err == nil {
			t.Errorf("lookPath(%q) = %q, want not found", name, p)
		}
	}
}

// resetSearchPath replaces the once-resolved search path for one test and puts
// it back afterwards.
func resetSearchPath(t *testing.T, dirs []string) {
	t.Helper()
	oldDirs := pathDirs
	pathOnce.Do(func() {}) // make sure the once has fired, then override
	pathDirs = dirs
	t.Cleanup(func() { pathDirs = oldDirs })
}
