package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A diff shows three lines of context and hides the rest, which is exactly
// wrong when the question is "what does this function look like around the
// change". The viewer's hunk headers are expanders, and this is what they
// read: any line range of the file at the scope's own revision.
//
//	GET /api/lines?name=myrepo1&repo=myrepo&scope=base&file=…&from=120&to=170
//
// Line numbers are 1-based and refer to the AFTER side, the same numbering the
// diff's right-hand gutter shows. `total` lets the viewer know where the file
// ends, so the expander below the last hunk can stop offering more.

// linesMax caps one request. The expander asks for a screenful at a time, so
// this only bounds a hand-written URL.
const linesMax = 500

func (d *grove) handleLines(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := d.checkout(q.Get("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", q.Get("name")))
		return
	}
	file := q.Get("file")
	if !safeRepoPath(file) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("path must stay inside the checkout"))
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
		writeErr(w, http.StatusNotFound, fmt.Errorf("%s has no %s checkout", c.Name, want))
		return
	}
	spec, err := resolveScope(root, want, d.baseFor(c.Path), primaryRepo(c), q.Get("scope"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	body, err := fileAt(root, file, spec)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	all := strings.Split(strings.TrimSuffix(body, "\n"), "\n")

	from, _ := strconv.Atoi(q.Get("from"))
	to, _ := strconv.Atoi(q.Get("to"))
	if from < 1 {
		from = 1
	}
	if to < from {
		to = from
	}
	if to-from >= linesMax {
		to = from + linesMax - 1
	}
	if from > len(all) {
		writeJSON(w, http.StatusOK, map[string]any{"lines": []string{}, "from": from, "total": len(all)})
		return
	}
	if to > len(all) {
		to = len(all)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lines": all[from-1 : to],
		"from":  from,
		"total": len(all),
	})
}

// fileAt reads the whole file as the scope's AFTER side sees it — the working
// tree for a range or the index, the commit itself for a commit scope. The
// same resolution the media preview uses, so an expanded line and a preview
// never disagree about which revision is on screen.
func fileAt(root, path string, spec scopeSpec) (string, error) {
	rev, fromWorktree := blobRev(spec, false)
	if fromWorktree {
		buf, err := os.ReadFile(filepath.Join(root, filepath.Clean(path)))
		if err == nil {
			return string(buf), nil
		}
		// committed but gone from disk: fall back to what HEAD holds
		rev = "HEAD"
	}
	blob, err := gitBlob(root, rev, path)
	if err != nil {
		return "", fmt.Errorf("no such file at %s", rev)
	}
	return string(blob), nil
}
