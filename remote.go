package main

import (
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
	Name     string   `json:"name"`
	Branch   string   `json:"branch,omitempty"`
	Detached bool     `json:"detached,omitempty"`
	Upstream string   `json:"upstream,omitempty"` // origin/branch, when it has one
	Remote   string   `json:"remote,omitempty"`   // where a push goes by default
	Remotes  []string `json:"remotes,omitempty"`  // all of them, when there is a choice
	Ahead    int      `json:"ahead"`              // commits the upstream has not
	Behind   int      `json:"behind"`             // commits it has and we have not
	Dirty    int      `json:"dirty"`

	// Gitlink is the commit the PARENT records for this submodule, and
	// GitlinkUnknown says no remote branch of the submodule contains it.
	Gitlink        string `json:"gitlink,omitempty"`
	GitlinkUnknown bool   `json:"gitlink_unknown,omitempty"`

	CanPush bool   `json:"can_push"`
	Blocked string `json:"blocked,omitempty"` // why not, in one line

	CanPull     bool   `json:"can_pull"`
	PullBlocked string `json:"pull_blocked,omitempty"`
	// PullMode is the way in that makes sense here — see pullMode.
	PullMode  string `json:"pull_mode,omitempty"`
	FetchedAt string `json:"fetched_at,omitempty"` // when this repo last heard from its remote
}

type remoteState struct {
	Name string `json:"name"` // the checkout
	// Repo is the repository the checkout belongs to — the directory holding
	// the real .git. Auto-fetch is keyed by it, because a repository's
	// worktrees share one object store and one fetch serves them all.
	Repo  string       `json:"repo,omitempty"`
	Repos []RemoteRepo `json:"repos"`
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
	writeJSON(w, http.StatusOK, remoteState{
		Name:  c.Name,
		Repo:  repoOf(c.Path),
		Repos: remoteRepos(c),
	})
}

// remoteRepos reads the standing of the checkout and every checked-out
// submodule under it, in the order the diff lists them.
func remoteRepos(c Checkout) []RemoteRepo {
	subs := diffRepos(c)
	out := make([]RemoteRepo, 0, len(subs))
	byParent := map[string][]string{} // parent path -> submodules it cannot publish
	for _, s := range subs {
		rr := readRemote(s.Path)
		rr.Name = s.Name
		if s.Parent != "" {
			// A submodule: what its parent records for it is what a push of
			// THAT parent would publish. The parent of a nested submodule is
			// the submodule above it, not the checkout — using the checkout
			// for all of them asked fylr for a path only easydb-webfrontend
			// has, got nothing, and quietly checked nothing.
			rr.Gitlink, rr.GitlinkUnknown = gitlinkStanding(s.Parent, s.Path)
			if rr.GitlinkUnknown {
				byParent[s.Parent] = append(byParent[s.Parent], s.Name)
			}
		}
		rr.CanPush, rr.Blocked = pushable(rr)
		rr.CanPull, rr.PullBlocked, rr.PullMode = pullable(rr)
		out = append(out, rr)
	}
	// An unknown gitlink stops the repository that RECORDS it, not the one that
	// owns the commit. The submodule is free to push — that is the cure — but
	// whoever points at it must not publish a tree naming a commit that
	// submodule's remote has never heard of. With nesting, that is not always
	// the checkout: easydb-webfrontend is stopped by easydb-library.
	for i := range out {
		names := byParent[subs[i].Path]
		if len(names) == 0 {
			continue
		}
		out[i].CanPush = false
		out[i].Blocked = "not on its remote: " + strings.Join(names, ", ") +
			" — push " + plural(len(names), "that submodule", "those submodules") + " first"
	}
	return out
}

// rebaseWithStash rebases a tree that has uncommitted work in it, by setting
// that work aside and putting it back.
//
// The whole point is that there is no in-between state to be left in. A rebase
// that stops halfway, or a stash that will not reapply, is worse than not
// having started: you are somewhere you did not ask to be, holding two things
// to reconcile by hand. So every step that can fail is followed by the step
// that undoes it, and there are exactly two outcomes — the rebase happened and
// your changes are back on top of it, or nothing happened at all and your
// changes are where they were.
//
// The one thing that cannot be undone is losing them, so they stay in the
// stash if they will not come back cleanly, and the message says so.
func rebaseWithStash(root, upstream string) (string, error) {
	start, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot read HEAD: %w", err)
	}

	dirty, _ := git(root, "status", "--porcelain", "-uall")
	stashed := false
	if dirty != "" {
		// --include-untracked: a file you have not added can still be in the
		// way of one the rebase wants to create
		if out, err := git(root, "stash", "push", "--include-untracked",
			"-m", "grove: rebasing onto "+upstream); err != nil {
			return "", fmt.Errorf("could not set your changes aside, so nothing was done: %v %s", err, out)
		}
		stashed = true
	}

	if out, err := git(root, "rebase", upstream); err != nil {
		// back to where we started, and hand the changes back
		git(root, "rebase", "--abort")
		if stashed {
			git(root, "stash", "pop")
		}
		return "", fmt.Errorf("the rebase stopped and was undone — nothing changed: %v %s", err, out)
	}

	if !stashed {
		return "rebased onto " + upstream, nil
	}

	if out, err := git(root, "stash", "pop"); err != nil {
		// The rebase worked and the changes will not sit on top of it. A
		// rebased branch plus a stash nobody asked for is exactly the
		// in-between state this exists to avoid, so put it all back.
		git(root, "reset", "--hard", start)
		if _, again := git(root, "stash", "pop"); again != nil {
			return "", fmt.Errorf("your changes conflict with %s. The rebase was undone and they are "+
				"waiting in the stash — `git stash pop` when you are ready: %v %s", upstream, err, out)
		}
		return "", fmt.Errorf("your changes conflict with %s, so the rebase was undone "+
			"and nothing changed: %v %s", upstream, err, out)
	}
	return "rebased onto " + upstream + ", your changes back on top", nil
}

func pastTense(action string) string {
	switch action {
	case "rebase":
		return "rebased onto"
	case "merge":
		return "merged"
	}
	return "fast-forwarded to"
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
	// every remote, and which one a push goes to unless told otherwise
	if remote, err := git(root, "remote"); err == nil && remote != "" {
		rr.Remotes = strings.Split(remote, "\n")
		rr.Remote = rr.Remotes[0]
		for _, n := range rr.Remotes {
			if n == "origin" {
				rr.Remote = "origin"
			}
		}
		// an upstream names its own remote, and that beats the guess
		if up, _, ok := strings.Cut(rr.Upstream, "/"); ok && up != "" {
			for _, n := range rr.Remotes {
				if n == up {
					rr.Remote = n
				}
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
		return false, "a detached HEAD is a commit, not a branch: nothing to push"
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

// pullable decides how to take in what the remote has, and picks the way that
// suits where this branch stands rather than making the reader choose blind:
//
//	nothing behind   there is nothing to take in
//	nothing of ours  a fast-forward — no merge commit for work that does not exist
//	ours on top      rebase, which keeps the history of a branch you are still
//	                 writing linear. Merge is offered beside it; it is the right
//	                 answer once the branch is shared, and grove cannot know that
//
// An uncommitted change is NOT a refusal. A merge or a fast-forward only fails
// when what is coming in touches a file you have edited, and git says so
// plainly when it does — being told "commit first" while holding an edit to an
// unrelated file is a wrong answer. A rebase does want a clean tree, and that
// is a reason to set the changes aside and put them back (rebaseWithStash),
// not a reason to refuse.
func pullable(rr RemoteRepo) (bool, string, string) {
	switch {
	case rr.Detached:
		return false, "a detached HEAD is a commit, not a branch: check one out to pull into", ""
	case rr.Upstream == "":
		return false, "no upstream to pull from", ""
	case rr.Behind == 0:
		return false, "already up to date", ""
	case rr.Ahead == 0:
		return true, "", "ff"
	}
	return true, "", "rebase"
}

type remoteRequest struct {
	Name   string   `json:"name"`   // the checkout
	Repos  []string `json:"repos"`  // which of its repositories to act on
	Action string   `json:"action"` // fetch | push | rebase | merge | ff
	Remote string   `json:"remote"` // push: which remote, when there is a choice
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
	case "fetch", "push", "rebase", "merge", "ff":
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
			results = append(results, doRemote(t, standing[t.Name], req.Action, req.Remote))
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
func doRemote(t subRepo, rr RemoteRepo, action, remote string) RepoResult {
	res := RepoResult{Repo: t.Name}
	switch action {
	case "rebase", "merge", "ff":
		if ok, why, _ := pullable(rr); !ok {
			if rr.Behind == 0 {
				res.Ok, res.Detail = true, "already up to date"
				return res
			}
			return RepoResult{Repo: t.Name, Detail: why}
		}

		if action == "rebase" {
			// a rebase wants a clean tree; getting one is this function's job
			detail, err := rebaseWithStash(t.Path, rr.Upstream)
			if err != nil {
				return RepoResult{Repo: t.Name, Detail: err.Error()}
			}
			res.Ok, res.Detail = true, detail
			return res
		}
		args := []string{"merge", "--ff-only", rr.Upstream}
		if action == "merge" {
			// --no-ff is not forced: when it can fast-forward, it should
			args = []string{"merge", rr.Upstream}
		}
		// no autostash and no strategy flags: a conflict should stop and be
		// dealt with in a terminal, not be papered over from a dashboard
		if out, err := git(t.Path, args...); err != nil {
			return RepoResult{Repo: t.Name, Detail: fmt.Sprintf("git %s: %v %s", strings.Join(args, " "), err, out)}
		}
		res.Ok, res.Detail = true, pastTense(action)+" "+rr.Upstream
		return res

	default: // push
		// re-checked against the freshly fetched state, not against whatever
		// the page was showing when the button was clicked
		if ok, why := pushable(rr); !ok {
			return RepoResult{Repo: t.Name, Detail: why}
		}
		// never --force, and never --force-with-lease: grove does not rewrite
		// anybody's history, including yours
		args := []string{"push"}
		to := rr.Remote
		if remote != "" {
			to = remote
		}
		if rr.Upstream == "" || (remote != "" && !strings.HasPrefix(rr.Upstream, to+"/")) {
			args = append(args, "--set-upstream", to, rr.Branch)
		}
		if out, err := git(t.Path, args...); err != nil {
			return RepoResult{Repo: t.Name, Detail: fmt.Sprintf("git push: %v %s", err, out)}
		}
		where := rr.Upstream
		if remote != "" {
			where = remote + "/" + rr.Branch
		} else if where == "" {
			where = rr.Remote + "/" + rr.Branch
		}
		res.Ok, res.Detail = true, "pushed to "+where
		return res
	}
}
