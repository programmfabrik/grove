package main

import "testing"

func TestPreviewKind(t *testing.T) {
	for in, want := range map[string]string{
		"docs/grove.PNG": "image", // the extension is matched case-insensitively
		"a/b.svg":        "image",
		"paper.pdf":      "pdf",
		"clip.mov":       "video",
		"note.m4a":       "audio",
		"main.go":        "", // show the diff instead
		"Makefile":       "",
	} {
		if got := previewKind(in); got != want {
			t.Errorf("previewKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPreviewTypeCoversEveryKind(t *testing.T) {
	// /api/blob refuses anything previewKind does not admit, so a kind without
	// a Content-Type would be served as a download nobody asked for
	for _, ext := range []string{
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp", ".ico", ".svg",
		".pdf", ".mp4", ".webm", ".ogv", ".mov", ".mp3", ".wav", ".ogg", ".m4a", ".flac",
	} {
		if previewKind("f"+ext) == "" {
			t.Errorf("previewKind admits nothing for %s", ext)
		}
		if previewType("f"+ext) == "" {
			t.Errorf("previewType has no Content-Type for %s", ext)
		}
	}
}

// Both sides of a preview resolve through the scope, so that an expanded line
// and the image beside it never disagree about which revision is on screen.
func TestBlobRev(t *testing.T) {
	for _, c := range []struct {
		name         string
		spec         scopeSpec
		before       bool
		rev          string
		fromWorktree bool
	}{
		{"commit before", scopeSpec{kind: "commit", sha: "abc1234", from: "abc1234^"}, true, "abc1234^", false},
		{"commit after", scopeSpec{kind: "commit", sha: "abc1234", from: "abc1234^"}, false, "abc1234", false},
		{"staged before", scopeSpec{kind: "staged"}, true, "HEAD", false},
		{"staged after", scopeSpec{kind: "staged"}, false, ":", false},
		{"unstaged before", scopeSpec{kind: "unstaged"}, true, ":", false},
		{"unstaged after", scopeSpec{kind: "unstaged"}, false, "", true},
		{"range before", scopeSpec{kind: "range", from: "forkpoint"}, true, "forkpoint", false},
		{"range after", scopeSpec{kind: "range", from: "forkpoint"}, false, "", true},
	} {
		rev, wt := blobRev(c.spec, c.before)
		if rev != c.rev || wt != c.fromWorktree {
			t.Errorf("%s: blobRev = (%q, %v), want (%q, %v)", c.name, rev, wt, c.rev, c.fromWorktree)
		}
	}
}
