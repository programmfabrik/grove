package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The first pane: every git repository in the start directory. A directory
// full of clones is how the work is actually laid out — the main project next
// to the libraries it uses next to two dozen plugin repos — and which of them
// is interesting changes by the hour, so the list sorts by how much is checked
// out (a repo with worktrees is a repo being worked on) and then by when it
// was last touched.
//
// Only the top level is scanned. Linked worktrees are found through the repo
// itself: `git worktree list` reports them wherever they live, a sibling
// directory of worktrees and checkouts nested inside another one included.

type Repo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Branch    string `json:"branch,omitempty"` // of the main checkout
	Worktrees int    `json:"worktrees"`        // incl. the main checkout
	Dirty     bool   `json:"dirty,omitempty"`  // the main checkout has changes
	LastUsed  string `json:"last_used,omitempty"`
}

// scanRepos lists the repositories directly inside dir.
func scanRepos(ctx context.Context, dir string) []Repo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		repos []Repo
		seen  = map[string]bool{}
	)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := readRepo(path)
			mu.Lock()
			defer mu.Unlock()
			// a linked worktree resolves to its main repo; count it once
			if seen[r.Path] {
				return
			}
			seen[r.Path] = true
			repos = append(repos, r)
		}()
	}
	wg.Wait()

	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Worktrees != repos[j].Worktrees {
			return repos[i].Worktrees > repos[j].Worktrees
		}
		if repos[i].LastUsed != repos[j].LastUsed {
			return repos[i].LastUsed > repos[j].LastUsed // RFC3339 sorts lexically
		}
		return repos[i].Name < repos[j].Name
	})
	return repos
}

func readRepo(path string) Repo {
	r := Repo{Path: path, Name: filepath.Base(path)}
	// resolve to the main repo, so scanning a directory that holds a linked
	// worktree does not list it as a repository of its own
	if common, err := git(path, "rev-parse", "--git-common-dir"); err == nil {
		if !filepath.IsAbs(common) {
			common = filepath.Join(path, common)
		}
		if abs, err := filepath.Abs(filepath.Dir(common)); err == nil {
			r.Path, r.Name = abs, filepath.Base(abs)
		}
	}
	r.Branch, _ = git(r.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if out, err := git(r.Path, "worktree", "list", "--porcelain"); err == nil {
		r.Worktrees = strings.Count(out, "\nworktree ") + 1
	}
	if s, err := git(r.Path, "status", "--porcelain"); err == nil && s != "" {
		r.Dirty = true
	}
	r.LastUsed = lastUsed(r.Path)
	return r
}

// lastUsed is when this repo was last WORKED in. The .git directory's mtime
// is the obvious candidate and the wrong one: reading a repo touches it (a
// `git status` rewrites the index), so a dashboard that scans every repo makes
// them all look freshly used — which is exactly what it must not do.
//
// So: the newest of the last commit and .git/HEAD's mtime. Committing,
// checking out a branch and switching worktrees all move one of the two, and
// none of them is disturbed by reading.
func lastUsed(repo string) string {
	newest := time.Time{}
	if out, err := git(repo, "log", "-1", "--format=%cI"); err == nil && out != "" {
		if t, err := time.Parse(time.RFC3339, out); err == nil {
			newest = t
		}
	}
	if st, err := os.Stat(filepath.Join(repo, ".git", "HEAD")); err == nil && st.ModTime().After(newest) {
		newest = st.ModTime()
	} else if st, err := os.Stat(filepath.Join(repo, ".git")); err == nil && newest.IsZero() {
		// a linked worktree's .git is a file, and a bare-ish layout has no
		// HEAD where we looked — fall back rather than report nothing
		newest = st.ModTime()
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}

// startDir is where the repo list is scanned. An explicit -dir wins; otherwise
// the working directory, or its repo's parent when grove is started from
// inside a checkout — `cd myrepo && grove` should list everything next to
// it, not the repo alone.
func startDir(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if common, err := git(wd, "rev-parse", "--git-common-dir"); err == nil {
		if !filepath.IsAbs(common) {
			common = filepath.Join(wd, common)
		}
		if repo, err := filepath.Abs(filepath.Dir(common)); err == nil {
			return filepath.Dir(repo), nil
		}
	}
	return filepath.Abs(wd)
}
