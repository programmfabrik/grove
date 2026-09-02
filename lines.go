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
//	GET /api/lines?…&file=…&to=0&side=before
//
// Line numbers are 1-based and refer to the AFTER side, the same numbering the
// diff's right-hand gutter shows. `total` lets the viewer know where the file
// ends, so the expander below the last hunk can stop offering more.
//
// `to=0` is the whole file, and `side=before` the revision the diff compares
// against instead: that is what the syntax colouring and the rendered
// markdown read. A diff is not a file — a comment opened in one hunk closes in
// the next — so each side is highlighted whole and the diff lines look their
// colours up by number.

// linesMax caps one ranged request. The expander asks for a screenful at a
// time, so this only bounds a hand-written URL. A whole-file read is bounded
// by wholeMax instead: past that the viewer shows the diff plain.
const linesMax = 500
const wholeMax = 400 << 10

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

	body, err := fileAt(root, file, spec, q.Get("side") == "before")
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
	switch {
	case to == 0: // the whole file
		if len(body) > wholeMax {
			writeErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf("file larger than %d KB", wholeMax>>10))
			return
		}
		to = len(all)
	case to < from:
		to = from
	case to-from >= linesMax:
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

// fileAt reads the whole file as one side of the scope sees it: the AFTER
// side — the working tree for a range or the index, the commit itself for a
// commit scope — or, with before, the revision the diff compares against. The
// same resolution the media preview uses, so an expanded line, a coloured
// line and a preview never disagree about which revision is on screen.
func fileAt(root, path string, spec scopeSpec, before bool) (string, error) {
	rev, fromWorktree := blobRev(spec, before)
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
