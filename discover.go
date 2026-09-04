package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"` // RFC3339
}

// Checkout is one git worktree of a repository, the main checkout included.
type Checkout struct {
	Name     string `json:"name"` // directory base name, the stable key
	Path     string `json:"path"`
	Repo     string `json:"repo"` // the repository this is a worktree of
	IsMain   bool   `json:"is_main"`
	Branch   string `json:"branch"`
	Detached bool   `json:"detached"`

	Head   Commit `json:"head"`
	Ahead  int    `json:"ahead"`  // commits HEAD has and the base branch has not
	Behind int    `json:"behind"` // commits the base branch has and HEAD has not
	Dirty  int    `json:"dirty"`  // changed files incl. untracked
}

// baseBranch is what every worktree's commits are compared against: the
// branch checked out in the MAIN repo (the checkout holding the real .git).
// That is `main` today, but nothing here assumes the name — put a release
// branch in the main checkout and the whole dashboard follows it. Falls back
// to the remote's default branch (origin/HEAD), then to "main" for a checkout
// that has neither.
func baseBranch(repo string) string {
	if b, err := git(repo, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && b != "" {
		return b
	}
	if b, err := git(repo, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && b != "" {
		return b
	}
	return "main"
}

// refreshGit re-scans one repository's worktrees. Each checkout's git calls
// run in their own goroutine — `git status` alone takes ~0.2s and a repo can
// have two dozen checkouts.
func (d *grove) refreshGit(ctx context.Context, repo string, st *repoState) {
	paths, err := worktreePaths(repo)
	if err != nil {
		d.mu.Lock()
		st.err, st.gitAt = err.Error(), time.Now()
		d.mu.Unlock()
		return
	}
	base := d.opt.base
	if base == "" {
		base = baseBranch(repo)
	}
	out := make([]Checkout, len(paths))
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = scanCheckout(ctx, repo, p, base)
		}()
	}
	wg.Wait()
	d.mu.Lock()
	st.checkouts, st.base, st.gitAt, st.err = out, base, time.Now(), ""
	d.mu.Unlock()
}

// worktreePaths lists the main repo and every linked worktree, main first and
// the rest by name — numbers in a name compare by value, so wt2 comes before
// wt10 — so the dashboard's order is stable across refreshes.
func worktreePaths(repo string) ([]string, error) {
	out, err := git(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, normPath(p)) // git's spelling, in the OS's
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("git worktree list returned no checkout")
	}
	slices.SortStableFunc(paths, func(a, b string) int {
		if samePath(a, repo) {
			return -1 // main first
		}
		if samePath(b, repo) {
			return 1
		}
		return naturalCompare(filepath.Base(a), filepath.Base(b))
	})
	return paths, nil
}

// naturalCompare orders names the way they are read: a run of digits compares
// by its value, everything else byte by byte.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			na, ia := leadingInt(a)
			nb, ib := leadingInt(b)
			if c := cmp.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[ia:], b[ib:]
			continue
		}
		if c := cmp.Compare(a[0], b[0]); c != 0 {
			return c
		}
		a, b = a[1:], b[1:]
	}
	return cmp.Compare(len(a), len(b))
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// leadingInt is the digit run s starts with, and where it ends.
func leadingInt(s string) (n, end int) {
	for end < len(s) && isDigit(s[end]) {
		end++
	}
	n, _ = strconv.Atoi(s[:end])
	return n, end
}

func scanCheckout(ctx context.Context, repo, path, base string) Checkout {
	c := Checkout{
		Name:   filepath.Base(path),
		Path:   path,
		Repo:   filepath.Base(repo),
		IsMain: samePath(path, repo),
	}
	if b, err := git(path, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && b != "" {
		c.Branch = b
	} else {
		c.Detached = true
	}
	if s, err := git(path, "log", "-1", "--format=%h%x1f%s%x1f%an%x1f%cI"); err == nil {
		f := strings.Split(s, "\x1f")
		if len(f) == 4 {
			c.Head = Commit{Hash: f[0], Subject: f[1], Author: f[2], Date: f[3]}
		}
	}
	// left = commits only the base has (behind), right = only HEAD has (ahead)
	if s, err := git(path, "rev-list", "--left-right", "--count", base+"...HEAD"); err == nil {
		f := strings.Fields(s)
		if len(f) == 2 {
			c.Behind, _ = strconv.Atoi(f[0])
			c.Ahead, _ = strconv.Atoi(f[1])
		}
	}
	// -uall so the count matches the file list the diff sidebar opens: without
	// it an untracked directory counts as one entry, however many files it holds
	if s, err := git(path, "status", "--porcelain", "-uall"); err == nil && s != "" {
		c.Dirty = len(strings.Split(s, "\n"))
	}
	return c
}

// gitExe is the git binary every call runs, resolved once by findGit. A PATH
// lookup per invocation is waste when a refresh makes hundreds of them, and
// resolving up front is what lets grove say "no git" once and clearly.
var gitExe = "git"

// findGit resolves git and proves it runs. Without git grove has nothing to
// show, and the failure has to be legible: the repo scan only stats .git, so a
// machine without git produced a full list of repositories and empty
// everything else — a dashboard that looks broken rather than one that says
// what is missing.
func findGit() error {
	p, err := lookPath("git")
	if err != nil {
		if p = fallbackGit(); p == "" {
			return fmt.Errorf("git is not on the PATH%s", installHint())
		}
	}
	// LookPath found a file; only running it proves it works. On macOS
	// /usr/bin/git is a stub that fails until the command line tools are there.
	if err := exec.Command(p, "--version").Run(); err != nil {
		return fmt.Errorf("%s does not run: %w%s", p, err, installHint())
	}
	gitExe = p
	return nil
}

// fallbackGit is where Windows keeps git when it is not on the PATH. Git for
// Windows installs into Program Files, and a process started from an icon
// rather than a shell does not always inherit the PATH entry that puts it
// there — which is exactly the case the desktop build will be in.
func fallbackGit() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		dir := os.Getenv(env)
		if dir == "" {
			continue // filepath.Join would make the candidate relative to the cwd
		}
		p := filepath.Join(dir, "Git", "cmd", "git.exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// installHint names the usual cure, per platform.
func installHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "\n  install the command line tools: xcode-select --install"
	case "windows":
		return "\n  install Git for Windows: https://git-scm.com/download/win"
	default:
		return "\n  install git with your package manager"
	}
}

// git runs a git command in dir and returns its trimmed stdout.
//
// A failure carries what git said about it. Output() keeps stderr on the
// ExitError and throws it away everywhere else, which is how a rebase that
// stopped for a nameable reason reached the screen as "exit status 1".
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command(gitExe, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(bytes.TrimSpace(ee.Stderr)) > 0 {
			return "", fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// gitSays runs a git command and returns everything it said, stdout and stderr
// together, whether it worked or not. The commands that CHANGE something are
// read by a person when they go wrong, and git explains itself far better than
// any message grove could invent for it.
func gitSays(dir string, args ...string) (string, error) {
	cmd := exec.Command(gitExe, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}
