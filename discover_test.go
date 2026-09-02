package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNaturalCompare(t *testing.T) {
	// the point of it: wt2 before wt10, which byte order gets backwards
	cases := []struct {
		a, b string
		want int
	}{
		{"wt2", "wt10", -1},
		{"wt10", "wt2", 1},
		{"wt2", "wt2", 0},
		{"myrepo", "myrepo2", -1},
		{"a1b", "a1c", -1},
		{"a01", "a1", 0}, // leading zeros are the same number
		{"", "a", -1},
	}
	for _, c := range cases {
		if got := sign(naturalCompare(c.a, c.b)); got != c.want {
			t.Errorf("naturalCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func TestLeadingInt(t *testing.T) {
	for _, c := range []struct {
		in  string
		n   int
		end int
	}{{"12ab", 12, 2}, {"7", 7, 1}, {"ab", 0, 0}, {"007x", 7, 3}} {
		n, end := leadingInt(c.in)
		if n != c.n || end != c.end {
			t.Errorf("leadingInt(%q) = (%d, %d), want (%d, %d)", c.in, n, end, c.n, c.end)
		}
	}
}

// The main checkout is found by comparing the path git printed against the
// path grove built, and the two are spelled differently whenever anything
// stands between them: separators on Windows, a symlink anywhere. This walks
// the real chain — scanRepos builds the repo path, worktreePaths gets its
// worktrees out of git — through a symlinked start directory, which is the
// same disagreement the Windows one is and can be provoked on any platform.
func TestScanFindsMainCheckoutThroughSymlink(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	start := filepath.Join(dir, "link")
	if err := os.Symlink(real, start); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	repo := initRepo(t, filepath.Join(real, "myrepo"))
	run(t, repo, "worktree", "add", "-q", "-b", "feature", filepath.Join(real, "myrepo2"))

	repos := scanRepos(context.Background(), normPath(start))
	if len(repos) != 1 {
		t.Fatalf("scanRepos found %d repositories, want 1: %+v", len(repos), repos)
	}
	if repos[0].Name != "myrepo" {
		t.Errorf("repo name = %q, want myrepo", repos[0].Name)
	}
	if repos[0].Worktrees != 2 {
		t.Errorf("worktrees = %d, want 2", repos[0].Worktrees)
	}

	paths, err := worktreePaths(repos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("worktreePaths = %v, want 2", paths)
	}
	if !samePath(paths[0], repos[0].Path) {
		t.Errorf("main checkout is not first: got %q, want %q", paths[0], repos[0].Path)
	}
	main := scanCheckout(context.Background(), repos[0].Path, paths[0], "main")
	if !main.IsMain {
		t.Errorf("IsMain false for the main checkout (%q vs repo %q)", paths[0], repos[0].Path)
	}
	linked := scanCheckout(context.Background(), repos[0].Path, paths[1], "main")
	if linked.IsMain {
		t.Errorf("IsMain true for the linked worktree %q", paths[1])
	}
	if linked.Branch != "feature" {
		t.Errorf("linked branch = %q, want feature", linked.Branch)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if err := findGit(); err != nil {
		t.Skipf("no usable git: %v", err)
	}
}

// initRepo makes a repository with one commit on main and returns its path,
// normalised the way grove holds paths.
func initRepo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, path, "init", "-q")
	run(t, path, "symbolic-ref", "HEAD", "refs/heads/main") // predates `init -b`
	// the machine running this may sign commits or have no identity at all
	run(t, path, "config", "user.email", "grove@example.com")
	run(t, path, "config", "user.name", "grove")
	run(t, path, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(path, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, path, "add", "a.txt")
	run(t, path, "commit", "-q", "-m", "first")
	return normPath(path)
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitExe, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
