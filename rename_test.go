package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every way --numstat writes a path: plain, a rename whose names share a
// directory pulled outside braces, one sharing a suffix too, one sharing
// nothing, and a move into or out of a directory, where one side of the braces
// is empty.
func TestRenamePaths(t *testing.T) {
	for in, want := range map[string][2]string{
		"internal/openapi/gitbook.go":                {"internal/openapi/gitbook.go", "internal/openapi/gitbook.go"},
		"internal/openapi/{gitbook.go => render.go}": {"internal/openapi/gitbook.go", "internal/openapi/render.go"},
		"src/{old => new}/main.go":                   {"src/old/main.go", "src/new/main.go"},
		"a.go => b.go":                               {"a.go", "b.go"},
		"{ => sub}/x.go":                             {"x.go", "sub/x.go"},
		"pkg/{sub => }/x.go":                         {"pkg/sub/x.go", "pkg/x.go"},
		"docs/{guide.md => guides/intro.md}":         {"docs/guide.md", "docs/guides/intro.md"},
	} {
		old, path := renamePaths(in)
		if old != want[0] || path != want[1] {
			t.Errorf("renamePaths(%q) = %q, %q; want %q, %q", in, old, path, want[0], want[1])
		}
	}
}

// A file renamed and cut down, the way gitbook.go became render.go: half its
// lines kept, the rest removed. It must read as a rename, with its counts, and
// its diff must be the lines that changed — not the whole file added anew.
func TestARenameIsARenameEverywhere(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	var body strings.Builder
	for i := 0; i < 40; i++ {
		body.WriteString("line " + strings.Repeat("x", i) + "\n")
	}
	if err := os.MkdirAll(filepath.Join(repo, "internal", "openapi"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, repo, "internal/openapi/gitbook.go", body.String())
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "the generator")
	base, _ := git(repo, "rev-parse", "HEAD")

	gitRun(t, repo, "mv", "internal/openapi/gitbook.go", "internal/openapi/render.go")
	kept := strings.Join(strings.Split(body.String(), "\n")[:30], "\n") + "\n"
	write(t, repo, "internal/openapi/render.go", kept) // ten lines removed
	gitRun(t, repo, "commit", "-q", "-am", "the generator goes, render.go stays")
	sha, _ := git(repo, "rev-parse", "HEAD")

	for name, spec := range map[string]scopeSpec{
		"commit": {kind: "commit", sha: sha, from: sha + "^"},
		"range":  {kind: "range", from: base},
	} {
		t.Run(name, func(t *testing.T) {
			files, err := scopeFiles(repo, spec, ignoring{})
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 {
				t.Fatalf("files = %+v, want the one renamed file", files)
			}
			f := files[0]
			if f.Path != "internal/openapi/render.go" || f.OldPath != "internal/openapi/gitbook.go" {
				t.Errorf("path %q from %q, want render.go from gitbook.go", f.Path, f.OldPath)
			}
			if f.Added != 0 || f.Deleted != 10 {
				t.Errorf("counts +%d -%d, want +0 -10 — the counts of a rename were being lost", f.Added, f.Deleted)
			}
			text, _, err := fileDiff(repo, f.Path, false, spec, ignoring{}, f.OldPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "rename from internal/openapi/gitbook.go") {
				t.Errorf("the diff does not show a rename:\n%s", firstLines(text, 8))
			}
			if strings.Contains(text, "new file mode") {
				t.Errorf("the diff shows render.go as new:\n%s", firstLines(text, 8))
			}
		})
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
