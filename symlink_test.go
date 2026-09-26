package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A symlink is stored as the path it points to, and that is what its diff and
// its lines have to show. A new link to a DIRECTORY could not be opened at all:
// git diff --no-index follows it, takes the directory for the place to diff
// /dev/null into, and fails looking for <link>/null.
func TestANewSymlinkShowsWhereItPoints(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	if err := os.Mkdir(filepath.Join(repo, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, repo, "target.txt", "the real content\n")
	for link, target := range map[string]string{"_models": "models", "to-file": "target.txt", "dangling": "nowhere"} {
		if err := os.Symlink(target, filepath.Join(repo, link)); err != nil {
			t.Skipf("cannot make symlinks here: %v", err) // Windows without the privilege
		}
	}

	for link, target := range map[string]string{"_models": "models", "to-file": "target.txt", "dangling": "nowhere"} {
		t.Run(link, func(t *testing.T) {
			text, _, err := fileDiff(repo, link, true, scopeSpec{kind: "unstaged"}, ignoring{})
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if !strings.Contains(text, "new file mode 120000") || !strings.Contains(text, "\n+"+target+"\n") {
				t.Errorf("diff does not show a link to %q:\n%s", target, text)
			}
			// the text grove colours and expands is the link's, not the target's
			got, err := fileAt(repo, link, scopeSpec{kind: "unstaged"}, false)
			if err != nil {
				t.Fatalf("lines: %v", err)
			}
			if got != target {
				t.Errorf("lines = %q, want the link text %q — not whatever it points at", got, target)
			}
		})
	}

	// and it is exactly what git records once the link is added
	gitRun(t, repo, "add", "_models")
	want, err := git(repo, "diff", "--cached", "--", "_models")
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "reset", "-q", "--", "_models")
	got, _, _ := fileDiff(repo, "_models", true, scopeSpec{kind: "unstaged"}, ignoring{})
	strip := func(s string) string { // the index line names blobs, and ours has none
		var out []string
		for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
			if !strings.HasPrefix(l, "index ") {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
	if strip(got) != strip(want) {
		t.Errorf("spelled out:\n%s\n\ngit records:\n%s", got, want)
	}
}
