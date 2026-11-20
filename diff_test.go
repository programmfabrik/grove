package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if got := (ignoring{}).args(); got != nil {
		t.Errorf("nothing ignored gives %v, want no flags", got)
	}
	got := ignoring{comments: true}.args()
	if len(got) != len(commentIgnores)*2 {
		t.Fatalf("comments give %d args, want %d", len(got), len(commentIgnores)*2)
	}
	for i := 0; i < len(got); i += 2 {
		if got[i] != "-I" {
			t.Errorf("arg %d = %q, want -I", i, got[i])
		}
	}
	if got := (ignoring{whitespace: true}).args(); len(got) != 1 || got[0] != "-w" {
		t.Errorf("whitespace gives %v, want [-w]", got)
	}
	both := ignoring{comments: true, whitespace: true}
	if got := both.args(); len(got) != len(commentIgnores)*2+1 {
		t.Errorf("both give %d args, want %d", len(got), len(commentIgnores)*2+1)
	}
	if !both.any() || (ignoring{}).any() {
		t.Error("any() does not say whether anything is ignored")
	}
}

// Ignoring whitespace takes a file whose only change is whitespace out of the
// list, the way ignoring comments does for comments — and it has to, because
// --name-status lists it regardless and only --numstat knows it has nothing
// left. A trailing space and a line ending are both whitespace to -w.
func TestIgnoreWhitespaceDropsAWhitespaceOnlyFile(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	write(t, repo, "spaced.txt", "one\ntwo \nthree\n")
	write(t, repo, "real.txt", "x\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "base")

	write(t, repo, "spaced.txt", "one\r\ntwo\nthree\n") // a line ending and a trailing space
	write(t, repo, "real.txt", "x\ny\n")                // and one real change

	paths := func(ig ignoring) []string {
		t.Helper()
		files, err := scopeFiles(repo, scopeSpec{kind: "unstaged"}, ig)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range files {
			out = append(out, f.Path)
		}
		return out
	}

	if got := paths(ignoring{}); len(got) != 2 {
		t.Fatalf("without ignoring anything: %v, want both files", got)
	}
	got := paths(ignoring{whitespace: true})
	if len(got) != 1 || got[0] != "real.txt" {
		t.Errorf("ignoring whitespace: %v, want only real.txt", got)
	}

	// and the diff text of the one that is left is its real change alone
	text, _, err := fileDiff(repo, "spaced.txt", false, scopeSpec{kind: "unstaged"}, ignoring{whitespace: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "@@") {
		t.Errorf("a whitespace-only file still has a hunk under -w:\n%s", text)
	}
}

// A file the branch touched and the working tree put back is not a change:
// between the fork point and the tree there is nothing, and a row for it had
// no counts and an empty diff — fylr.default.yml, five lines added in a commit
// and taken out again uncommitted. Nor does it depend on what is ignored: it
// once vanished only when a flag was on, 27 files at a time, for a reason that
// had nothing to do with the flag. Absent either way, and every real
// difference without a line in it — a mode, a binary, an untracked file —
// still listed.
func TestAFileChangedAndChangedBackIsNotAChange(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	write(t, repo, "kept.yml", "a: 1\n")
	write(t, repo, "run.sh", "echo\n")
	write(t, repo, "blob.bin", "\x00\x01")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "files")
	base, _ := git(repo, "rev-parse", "HEAD")
	gitRun(t, repo, "checkout", "-q", "-b", "work")
	write(t, repo, "added.txt", "on the branch\n")
	write(t, repo, "kept.yml", "a: 1\nb: 2\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "add them")
	gitRun(t, repo, "rm", "-q", "added.txt") // taken out again, staged
	write(t, repo, "kept.yml", "a: 1\n")     // put back, unstaged
	// a mode, no lines: on disk, and in the index for a git that does not
	// read modes from disk (core.filemode is off on Windows)
	if err := os.Chmod(filepath.Join(repo, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "update-index", "--chmod=+x", "run.sh")
	write(t, repo, "blob.bin", "\x00\x02")   // a binary, no lines
	write(t, repo, "new.txt", "untracked\n") // in no diff at all

	for _, ig := range []ignoring{{}, {whitespace: true}, {comments: true}} {
		files, err := scopeFiles(repo, scopeSpec{kind: "range", from: base}, ig)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range files {
			got = append(got, f.Path)
		}
		if want := "blob.bin new.txt run.sh"; strings.Join(got, " ") != want {
			t.Errorf("%+v: listed %v, want %s", ig, got, want)
		}
	}
}
