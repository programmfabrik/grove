package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// One pass over the endpoints the dashboard actually calls, against a real
// repository. It is here for the platforms: the git plumbing underneath is the
// same everywhere in theory and the spellings around it are not, and an
// untracked file is diffed against /dev/null — a name Windows does not have,
// which git special-cases and this proves.
func TestAPIOverARealRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	repo := initRepo(t, filepath.Join(dir, "myrepo"))
	write(t, repo, "committed.txt", "one\ntwo\nthree\n")
	gitRun(t, repo, "add", "committed.txt")
	gitRun(t, repo, "commit", "-q", "-m", "second")
	write(t, repo, "committed.txt", "one\ntwo\nCHANGED\n") // unstaged
	write(t, repo, "staged.txt", "s\n")
	gitRun(t, repo, "add", "staged.txt")
	write(t, repo, "untracked.txt", "u1\nu2\n") // in no commit and no index

	d := &grove{
		opt:   options{dir: normPath(dir), refresh: time.Minute},
		state: map[string]*repoState{},
	}
	srv := httptest.NewServer(d.routes(true))
	defer srv.Close()

	get := func(path string, into any) {
		t.Helper()
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %s", path, res.Status)
		}
		if err := json.NewDecoder(res.Body).Decode(into); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
	}

	var repos struct {
		Dir   string `json:"dir"`
		Repos []Repo `json:"repos"`
	}
	get("/api/repos", &repos)
	if len(repos.Repos) != 1 || repos.Repos[0].Name != "myrepo" {
		t.Fatalf("/api/repos = %+v, want one myrepo", repos.Repos)
	}
	if !repos.Repos[0].Dirty {
		t.Error("/api/repos: myrepo has uncommitted work and did not say so")
	}

	var state State
	get("/api/state?repo="+repo, &state)
	if state.GitError != "" {
		t.Fatalf("/api/state: %s", state.GitError)
	}
	if len(state.Checkouts) != 1 || !state.Checkouts[0].IsMain {
		t.Fatalf("/api/state checkouts = %+v, want one main checkout", state.Checkouts)
	}
	if state.Base != "main" {
		t.Errorf("base = %q, want main", state.Base)
	}
	// committed.txt modified, staged.txt added, untracked.txt new
	if state.Checkouts[0].Dirty != 3 {
		t.Errorf("dirty = %d, want 3", state.Checkouts[0].Dirty)
	}

	var scopes struct {
		Name  string      `json:"name"`
		Repos []ScopeRepo `json:"repos"`
	}
	get("/api/scopes?name=myrepo", &scopes)
	if len(scopes.Repos) != 1 {
		t.Fatalf("/api/scopes = %+v, want one repo section", scopes.Repos)
	}
	have := map[string]Scope{}
	for _, s := range scopes.Repos[0].Scopes {
		have[s.ID] = s
	}
	for _, id := range []string{"staged", "unstaged"} {
		if _, ok := have[id]; !ok {
			t.Errorf("no %q scope in %+v", id, scopes.Repos[0].Scopes)
		}
	}
	if got := have["staged"].Files; got != 1 {
		t.Errorf("staged holds %d files, want 1", got)
	}

	var files struct {
		Files []DiffFile `json:"files"`
	}
	get("/api/diff?name=myrepo&repo=myrepo&scope=unstaged", &files)
	byPath := map[string]DiffFile{}
	for _, f := range files.Files {
		byPath[f.Path] = f
	}
	if _, ok := byPath["committed.txt"]; !ok {
		t.Errorf("unstaged is missing committed.txt: %+v", files.Files)
	}
	u, ok := byPath["untracked.txt"]
	if !ok {
		t.Fatalf("unstaged is missing untracked.txt: %+v", files.Files)
	}
	if !u.Untracked || u.Status != "new" {
		t.Errorf("untracked.txt = %+v, want untracked and new", u)
	}

	// the /dev/null diff: an untracked file has to read as all additions
	var one struct {
		Diff string `json:"diff"`
	}
	get("/api/diff?name=myrepo&repo=myrepo&scope=unstaged&file=untracked.txt&untracked=1", &one)
	if !strings.Contains(one.Diff, "+u1") || !strings.Contains(one.Diff, "+u2") {
		t.Errorf("untracked diff did not come out as additions:\n%s", one.Diff)
	}

	// and a tracked file's diff, at the same scope
	get("/api/diff?name=myrepo&repo=myrepo&scope=unstaged&file=committed.txt", &one)
	if !strings.Contains(one.Diff, "-three") || !strings.Contains(one.Diff, "+CHANGED") {
		t.Errorf("unstaged diff of committed.txt is wrong:\n%s", one.Diff)
	}

	// the hunk expanders read whole files at the scope's own revision
	var lines struct {
		Lines []string `json:"lines"`
	}
	get("/api/lines?name=myrepo&repo=myrepo&scope=unstaged&file=committed.txt&side=after", &lines)
	if strings.Join(lines.Lines, "\n") != "one\ntwo\nCHANGED" {
		t.Errorf("/api/lines after = %q, want the working tree", lines.Lines)
	}
	get("/api/lines?name=myrepo&repo=myrepo&scope=unstaged&file=committed.txt&side=before", &lines)
	if strings.Join(lines.Lines, "\n") != "one\ntwo\nthree" {
		t.Errorf("/api/lines before = %q, want the index", lines.Lines)
	}
	// both sides that resolve to the index: git spells it ":path", and joining
	// a revision of ":" to the path with another colon does not
	get("/api/lines?name=myrepo&repo=myrepo&scope=staged&file=staged.txt&side=after", &lines)
	if strings.Join(lines.Lines, "\n") != "s" {
		t.Errorf("/api/lines staged after = %q, want the staged blob", lines.Lines)
	}
}

func write(t testing.TB, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
