package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Talking to the remote — the second place grove writes, and the first that
// leaves the machine.
//
// `git push` publishes. grove cannot take it back, and neither can you without
// rewriting somebody else's history, so the answer here is not to be clever:
// no force, no lease, no inventing a branch. It pushes the branch that is
// checked out to the upstream it already has, and refuses in every case it is
// not certain about.
//
// It refuses four of them:
//
//	detached HEAD          there is no branch to push
//	the remote has moved   pushing would fail anyway; rebase or merge first
//	nothing to push        being ahead by nothing is not an error, just a no-op
//	an unknown submodule   the parent records a commit for a submodule that no
//	                       remote branch of that submodule contains, so pushing
//	                       the parent publishes a pointer nobody else can follow
//
// The last one is the reason this file knows about submodules at all. A
// checkout that carries them is several repositories, each with its own remote
// and its own idea of what has been pushed, and the one that breaks a
// colleague's clone is the one nobody looks at.

// RemoteRepo is one repository's standing with its remote: the checkout
// itself, or one of the submodules under it.
type RemoteRepo struct {
	Name     string `json:"name"`
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached,omitempty"`
	Upstream string `json:"upstream,omitempty"` // origin/branch, when it has one
	Remote   string `json:"remote,omitempty"`   // where a first push would go
	Ahead    int    `json:"ahead"`              // commits the upstream has not
	Behind   int    `json:"behind"`             // commits it has and we have not
	Dirty    int    `json:"dirty"`

	// Gitlink is the commit the PARENT records for this submodule, and
	// GitlinkUnknown says no remote branch of the submodule contains it.
	Gitlink        string `json:"gitlink,omitempty"`
	GitlinkUnknown bool   `json:"gitlink_unknown,omitempty"`

	CanPush   bool   `json:"can_push"`
	Blocked   string `json:"blocked,omitempty"`    // why not, in one line
	FetchedAt string `json:"fetched_at,omitempty"` // when this repo last heard from its remote
}

type remoteState struct {
	Name string `json:"name"` // the checkout
	// Repo is the repository the checkout belongs to — the directory holding
	// the real .git. Auto-fetch is keyed by it, because a repository's
	// worktrees share one object store and one fetch serves them all.
	Repo      string       `json:"repo,omitempty"`
	AutoFetch bool         `json:"auto_fetch,omitempty"`
	Repos     []RemoteRepo `json:"repos"`
}

// repoOf is the repository a checkout belongs to.
func repoOf(checkout string) string {
	common, err := git(checkout, "rev-parse", "--git-common-dir")
	if err != nil {
		return ""
	}
	common = normPath(common)
	if !filepath.IsAbs(common) {
		common = filepath.Join(checkout, common)
	}
	return normPath(filepath.Dir(common))
}

// handleRemote reports where every repository under a checkout stands with its
// remote. A read: it never fetches, so what it says is as fresh as the last
// fetch and FetchedAt says when that was.
func (d *grove) handleRemote(w http.ResponseWriter, r *http.Request) {
	c, ok := d.checkout(r.URL.Query().Get("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", r.URL.Query().Get("name")))
		return
	}
	repo := repoOf(c.Path)
	writeJSON(w, http.StatusOK, remoteState{
		Name:      c.Name,
		Repo:      repo,
		AutoFetch: loadSettings().AutoFetch[repo],
		Repos:     remoteRepos(c),
	})
}

// remoteRepos reads the standing of the checkout and every checked-out
// submodule under it, in the order the diff lists them.
func remoteRepos(c Checkout) []RemoteRepo {
	subs := diffRepos(c)
	out := make([]RemoteRepo, 0, len(subs))
	var unknown []string
	for i, s := range subs {
		rr := readRemote(s.Path)
		rr.Name = s.Name
		if i > 0 {
			// a submodule: what the parent records for it is what a push of
			// the parent would publish
			rr.Gitlink, rr.GitlinkUnknown = gitlinkStanding(c.Path, s.Path)
			if rr.GitlinkUnknown {
				unknown = append(unknown, s.Name)
			}
		}
		rr.CanPush, rr.Blocked = pushable(rr)
		out = append(out, rr)
	}
	// An unknown gitlink stops the PARENT, not the submodule. The submodule is
	// free to push — that is the cure — but the parent must not publish a tree
	// pointing at a commit the submodule's remote has never heard of.
	if len(unknown) > 0 && len(out) > 0 {
		out[0].CanPush = false
		out[0].Blocked = "not on its remote: " + strings.Join(unknown, ", ") +
			" — push " + plural(len(unknown), "that submodule", "those submodules") + " first"
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func readRemote(root string) RemoteRepo {
	var rr RemoteRepo
	if b, err := git(root, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && b != "" {
		rr.Branch = b
	} else {
		rr.Detached = true
	}
	if up, err := git(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		rr.Upstream = up
	}
	// where a branch with no upstream would go on its first push
	if remote, err := git(root, "remote"); err == nil && remote != "" {
		names := strings.Split(remote, "\n")
		rr.Remote = names[0]
		for _, n := range names {
			if n == "origin" {
				rr.Remote = "origin"
			}
		}
	}
	if rr.Upstream != "" {
		// left = only the upstream has it (behind), right = only we have it
		if s, err := git(root, "rev-list", "--left-right", "--count", rr.Upstream+"...HEAD"); err == nil {
			if f := strings.Fields(s); len(f) == 2 {
				rr.Behind, _ = strconv.Atoi(f[0])
				rr.Ahead, _ = strconv.Atoi(f[1])
			}
		}
	}
	if s, err := git(root, "status", "--porcelain", "-uall"); err == nil && s != "" {
		rr.Dirty = len(strings.Split(s, "\n"))
	}
	rr.FetchedAt = lastFetch(root)
	return rr
}

// lastFetch is when this repository last heard from a remote. git stamps
// FETCH_HEAD on every fetch and nothing else touches it, which makes its mtime
// the honest answer to "how stale is the number you are showing me".
func lastFetch(root string) string {
	dir, err := git(root, "rev-parse", "--git-dir")
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	st, err := os.Stat(filepath.Join(dir, "FETCH_HEAD"))
	if err != nil {
		return ""
	}
	return st.ModTime().UTC().Format(time.RFC3339)
}

// gitlinkStanding is the commit the parent records for a submodule, and
// whether any remote branch of the submodule contains it. A commit that is
// only local is the one that breaks somebody else's clone: they get the
// parent, git asks the submodule's remote for that id, and nobody has it.
func gitlinkStanding(parent, subPath string) (sha string, unknown bool) {
	rel, err := filepath.Rel(parent, subPath)
	if err != nil {
		return "", false
	}
	sha, err = git(parent, "rev-parse", "HEAD:"+filepath.ToSlash(rel))
	if err != nil || sha == "" {
		return "", false // not recorded in HEAD: nothing a push would publish
	}
	// --contains needs the commit to be present locally; if it is not, no
	// remote branch can be said to hold it either
	out, err := git(subPath, "branch", "-r", "--contains", sha)
	return sha, err != nil || strings.TrimSpace(out) == ""
}

// pushable is the one place that decides, so the button and the endpoint can
// never disagree about why something is refused.
func pushable(rr RemoteRepo) (bool, string) {
	switch {
	case rr.Detached:
		return false, "detached HEAD: no branch to push"
	// GitlinkUnknown is deliberately not here. It stops the PARENT, which
	// remoteRepos handles, and must never stop the submodule: pushing the
	// submodule is exactly the cure, and refusing it would leave no way out.
	case rr.Behind > 0:
		return false, fmt.Sprintf("%s is %d ahead: rebase or merge first", rr.Upstream, rr.Behind)
	case rr.Upstream == "" && rr.Remote == "":
		return false, "no remote to push to"
	case rr.Upstream != "" && rr.Ahead == 0:
		return false, "nothing to push"
	}
	return true, ""
}

type remoteRequest struct {
	Name   string   `json:"name"`   // the checkout
	Repos  []string `json:"repos"`  // which of its repositories to act on
	Action string   `json:"action"` // fetch | push | rebase | merge
}

// RepoResult is what one repository did, or would not.
type RepoResult struct {
	Repo   string `json:"repo"`
	Ok     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func (d *grove) handleRemoteAction(w http.ResponseWriter, r *http.Request) {
	var req remoteRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return
	}
	switch req.Action {
	case "fetch", "push", "rebase", "merge":
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown action %q", req.Action))
		return
	}
	c, ok := d.checkout(req.Name)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", req.Name))
		return
	}
	want := map[string]bool{}
	for _, n := range req.Repos {
		want[n] = true
	}
	var targets []subRepo
	for _, s := range diffRepos(c) {
		if len(want) == 0 || want[s.Name] {
			targets = append(targets, s)
		}
	}
	if len(targets) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no repository selected"))
		return
	}

	// Fetch EVERY repository under the checkout, not only the selected ones:
	// whether the parent may push depends on what the submodules' remotes hold,
	// and that question cannot be answered with stale remote refs.
	all := diffRepos(c)
	results := make([]RepoResult, 0, len(all))
	for _, s := range all {
		if out, err := git(s.Path, "fetch", "--prune"); err != nil {
			results = append(results, RepoResult{Repo: s.Name,
				Detail: fmt.Sprintf("git fetch: %v %s", err, out)})
		}
	}

	if req.Action != "fetch" {
		// Now the standing is fresh, and it knows about gitlinks — which the
		// per-repository read cannot, since only the parent records them.
		standing := map[string]RemoteRepo{}
		for _, rr := range remoteRepos(c) {
			standing[rr.Name] = rr
		}
		// Submodules before the parent. Pushing a submodule is what makes the
		// parent's pointer to it followable, so doing the parent first would
		// publish exactly the broken state this refuses to publish.
		for i := len(targets) - 1; i >= 0; i-- {
			t := targets[i]
			results = append(results, doRemote(t, standing[t.Name], req.Action))
		}
	}
	// every one of these changes what a scan would find
	d.dropCaches()
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"repos":   remoteRepos(c), // the standing after, so the header can settle
	})
}

// doRemote runs one action in one repository, against a standing read AFTER
// the fetch — never against whatever the page happened to be showing when the
// button was clicked.
func doRemote(t subRepo, rr RemoteRepo, action string) RepoResult {
	res := RepoResult{Repo: t.Name}
	switch action {
	case "rebase", "merge":
		if rr.Upstream == "" {
			return RepoResult{Repo: t.Name, Detail: "no upstream to " + action + " onto"}
		}
		if rr.Dirty > 0 {
			return RepoResult{Repo: t.Name, Detail: "uncommitted changes: commit or discard them first"}
		}
		if rr.Behind == 0 {
			res.Ok, res.Detail = true, "already up to date"
			return res
		}
		// no autostash and no strategy flags: a conflict should stop and be
		// dealt with in a terminal, not be papered over from a dashboard
		if out, err := git(t.Path, action, rr.Upstream); err != nil {
			return RepoResult{Repo: t.Name, Detail: fmt.Sprintf("git %s %s: %v %s", action, rr.Upstream, err, out)}
		}
		res.Ok, res.Detail = true, action+"d onto "+rr.Upstream
		return res

	default: // push
		// re-checked against the freshly fetched state, not against whatever
		// the page was showing when the button was clicked
		if ok, why := pushable(rr); !ok {
			return RepoResult{Repo: t.Name, Detail: why}
		}
		args := []string{"push"}
		if rr.Upstream == "" {
			args = append(args, "--set-upstream", rr.Remote, rr.Branch)
		}
		if out, err := git(t.Path, args...); err != nil {
			return RepoResult{Repo: t.Name, Detail: fmt.Sprintf("git push: %v %s", err, out)}
		}
		where := rr.Upstream
		if where == "" {
			where = rr.Remote + "/" + rr.Branch
		}
		res.Ok, res.Detail = true, "pushed to "+where
		return res
	}
}

// ── fetching on its own ──────────────────────────────────────────────────
//
// Off for every repository until somebody turns it on, and remembered per
// repository rather than globally: one project whose remote you care about
// being current is not a reason to reach out to seventy-three others.
//
// A repository's worktrees share one object store, so one fetch there brings
// every worktree's remote-tracking refs up to date at once. Submodules are
// repositories of their own and do not come along, so each checked-out one is
// fetched too — which is what makes the parent's "is this gitlink known"
// answer worth anything.

const autoFetchEvery = 5 * time.Minute

func (d *grove) handleAutoFetch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo string `json:"repo"` // repository path
		On   bool   `json:"on"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return
	}
	if req.Repo == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no repository named"))
		return
	}
	s := loadSettings()
	if s.AutoFetch == nil {
		s.AutoFetch = map[string]bool{}
	}
	if req.On {
		s.AutoFetch[normPath(req.Repo)] = true
	} else {
		delete(s.AutoFetch, normPath(req.Repo))
	}
	if err := s.save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repo": req.Repo, "on": req.On})
}

// autoFetchLoop keeps the enabled repositories' remote refs current. It reads
// the setting each round rather than caching it, so turning it on takes effect
// without a restart.
func (d *grove) autoFetchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(autoFetchEvery):
		}
		for repo, on := range loadSettings().AutoFetch {
			if !on {
				continue
			}
			// the repository itself: one fetch, every worktree's refs
			git(repo, "fetch", "--prune")
			// and each checkout's submodules, which are their own repositories
			for _, p := range func() []string { ps, _ := worktreePaths(repo); return ps }() {
				c := Checkout{Name: filepath.Base(p), Path: p, Repo: filepath.Base(repo)}
				for i, s := range diffRepos(c) {
					if i > 0 {
						git(s.Path, "fetch", "--prune")
					}
				}
			}
		}
		d.dropCaches()
	}
}
