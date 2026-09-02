package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// A diff of a jpg is "Binary files … differ", which tells you nothing. For the
// file types a browser can render itself, the viewer shows the file instead —
// before and after where both exist — a repository full of test fixtures
// changes images as often as code.
//
//	GET /api/blob?name=myrepo1&repo=myrepo&scope=<id>&file=<path>&side=before|after
//
// Both sides are resolved through the SCOPE, so "before" means the same thing
// here as in the diff next to it: the fork point for a range, HEAD for the
// index, the commit's parent for a commit.

// previewKind says whether a browser can display this file, and as what. The
// empty string means "show the diff instead".
func previewKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp", ".ico", ".svg":
		return "image"
	case ".pdf":
		return "pdf"
	case ".mp4", ".webm", ".ogv", ".mov":
		return "video"
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac":
		return "audio"
	}
	return ""
}

// previewType is the Content-Type the preview is served with. Only types
// previewKind admits reach here, so the list stays short and explicit rather
// than trusting mime.TypeByExtension on this machine.
func previewType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".mp4", ".mov":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ogv":
		return "video/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".flac":
		return "audio/flac"
	}
	return "application/octet-stream"
}

func (d *grove) handleBlob(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := d.checkout(q.Get("name"))
	if !ok {
		http.Error(w, "no such checkout", http.StatusNotFound)
		return
	}
	file := q.Get("file")
	if !safeRepoPath(file) || previewKind(file) == "" {
		http.Error(w, "not a previewable path", http.StatusBadRequest)
		return
	}
	want := q.Get("repo")
	if want == "" {
		want = primaryRepo(c)
	}
	root := ""
	for _, repo := range diffRepos(c) {
		if repo.Name == want {
			root = repo.Path
		}
	}
	if root == "" {
		http.Error(w, "no such repo in this checkout", http.StatusNotFound)
		return
	}

	// The browser asks for these in an <img>/<video>, which it may re-request
	// on every render; the underlying file changes as the user works, so it
	// must not be cached.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", previewType(file))

	spec, err := resolveScope(root, want, d.baseFor(c.Path), primaryRepo(c), q.Get("scope"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rev, fromWorktree := blobRev(spec, q.Get("side") == "before")
	if !fromWorktree {
		blob, err := gitBlob(root, rev, file)
		if err != nil {
			http.Error(w, "not in that revision", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, filepath.Base(file), time.Time{}, bytes.NewReader(blob))
		return
	}

	// the working tree — a plain file, so ServeContent handles range requests
	// and a <video> can seek
	full := filepath.Join(root, filepath.Clean(file))
	f, err := os.Open(full)
	if err != nil {
		// committed but no longer on disk: fall back to what HEAD holds
		blob, err := gitBlob(root, "HEAD", file)
		if err != nil {
			http.Error(w, "no such file", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, filepath.Base(file), time.Time{}, bytes.NewReader(blob))
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "cannot stat", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, filepath.Base(file), st.ModTime(), f)
}

// blobRev picks the revision a preview side comes from. The second return
// says to read the working tree instead of the object store.
func blobRev(spec scopeSpec, before bool) (rev string, fromWorktree bool) {
	switch spec.kind {
	case "commit":
		if before {
			return spec.from, false // the commit's parent
		}
		return spec.sha, false
	case "staged":
		if before {
			return "HEAD", false
		}
		return ":", false // ":path" is the staged blob
	case "unstaged":
		if before {
			return ":", false
		}
		return "", true
	default: // range
		if before {
			return spec.from, false
		}
		return "", true
	}
}

// gitBlob reads one path at one revision out of the object store.
func gitBlob(root, rev, path string) ([]byte, error) {
	// rev ":" is the index, and git spells that ":path" — ONE colon. Joining
	// it to the path with another produces "::path", which git rejects, so the
	// index needs the case it looks like it does not.
	spec := rev + ":" + path
	if rev == ":" {
		spec = ":" + path
	}
	cmd := exec.Command(gitExe, "show", spec)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s: %w", spec, err)
	}
	return out, nil
}
