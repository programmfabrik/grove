package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Finding the programs a person actually has.
//
// A GUI application does not inherit the PATH from your shell. Launched from
// the Dock it gets launchd's, which on a normal Mac is roughly
// /usr/local/bin:/bin:/usr/bin — no /opt/homebrew/bin, and therefore no gh, no
// nvim, no hx. grove told somebody with gh installed and signed in that gh was
// not installed, which is worse than not having looked: the instructions it
// then offered were to install a thing they already had.
//
// So the search is the login shell's PATH — the one every instruction on the
// internet assumes — asked for once, with the usual package manager
// directories behind it in case even that fails.

var (
	pathOnce sync.Once
	pathDirs []string
)

// searchPath is where to look for a program, in order: what the login shell
// would use, then this process's own PATH, then the places things get
// installed. Resolved once; a login shell reads rc files and that is not a
// thing to do per lookup.
func searchPath() []string {
	pathOnce.Do(func() {
		seen := map[string]bool{}
		add := func(list string) {
			for _, dir := range filepath.SplitList(list) {
				if dir == "" || seen[dir] {
					continue
				}
				seen[dir] = true
				pathDirs = append(pathDirs, dir)
			}
		}
		add(loginShellPath())
		add(os.Getenv("PATH"))
		home, _ := os.UserHomeDir()
		for _, dir := range []string{
			"/opt/homebrew/bin", "/opt/homebrew/sbin", // Apple Silicon Homebrew
			"/usr/local/bin", "/usr/local/sbin", // Intel Homebrew, and everyone else
			"/opt/local/bin", // MacPorts
			"/usr/bin", "/bin",
			filepath.Join(home, "bin"),
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "go", "bin"),
			filepath.Join(home, ".cargo", "bin"),
		} {
			if dir != "" && home != "" || !strings.HasPrefix(dir, home) {
				add(dir)
			}
		}
	})
	return pathDirs
}

// loginShellPath asks the shell what its PATH is, the way it would be for a
// terminal. It is given a deadline: an rc file that waits for something would
// otherwise hold up the first lookup for as long as it liked.
func loginShellPath() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// -l so the profile is read; the command prints nothing else
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", "printf %s \"$PATH\"")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// lookPath is exec.LookPath over that search path.
func lookPath(name string) (string, error) {
	if filepath.IsAbs(name) {
		if usable(name) {
			return name, nil
		}
		return "", exec.ErrNotFound
	}
	for _, dir := range searchPath() {
		p := filepath.Join(dir, name)
		if usable(p) {
			return p, nil
		}
	}
	// let exec have the last word, for the extensions Windows cares about
	return exec.LookPath(name)
}

func usable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}
