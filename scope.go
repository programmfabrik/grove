package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// A scope is WHAT the diff tab shows for one repo. "What changed here" has
// several honest answers and a worktree needs all of them:
//
//	base       everything the checkout holds that the base branch does not,
//	           committed and uncommitted together (the fork point → worktree).
//	           Omitted in a checkout that IS the base branch — it compares to
//	           itself.
//	origin     the same against the branch's upstream: what has not been
//	           pushed yet, plus what is not committed yet. Omitted for a branch
//	           with no upstream — nothing to compare against.
//	staged     the index against HEAD, and
//	unstaged   the worktree against the index — the two halves of "dirty",
//	           which matters the moment you are assembling a commit.
//	commit     one commit on its own, marked with how far it has travelled:
//	           not pushed, pushed but not in the base branch, or landed.
//
// Every scope resolves to a scopeSpec, and the file list, the per-file diff
// and the media preview all read that one struct — so a new scope is a case
// here and nothing else.

const commitScopeLimit = 20

type Scope struct {
	ID      string `json:"id"` // base | origin | staged | unstaged | commit:<sha>
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Hint    string `json:"hint,omitempty"`   // the ref it compares against
	Sha     string `json:"sha,omitempty"`    // commit scopes
	Pushed  bool   `json:"pushed,omitempty"` // commit scopes: reachable from a remote
	Merged  bool   `json:"merged,omitempty"` // commit scopes: the base branch has it
	Date    string `json:"date,omitempty"`   // commit scopes
	Author  string `json:"author,omitempty"` // commit scopes
	Body    string `json:"body,omitempty"`   // commit scopes: the message below the subject
	Files   int    `json:"files"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

// ScopeRepo is one repo's section in the scope list — the "splitter headline"
// and the scopes under it.
type ScopeRepo struct {
	Name     string  `json:"name"`
	Branch   string  `json:"branch,omitempty"`
	Base     string  `json:"base,omitempty"`
	Upstream string  `json:"upstream,omitempty"`
	Scopes   []Scope `json:"scopes"`
}

// scopeSpec is the resolved scope: how to list its files and how to diff one.
type scopeSpec struct {
	kind string // range | staged | unstaged | commit
	from string // range: the fork point; commit: the commit's parent
	sha  string // commit
	ref  string // range vs the base: the branch compared against (markLanded)
}

// resolveScope turns a scope id back into its spec. Ids carry everything
// needed except the fork points, which are recomputed from the repo.
func resolveScope(root, repoName, dashboardBase, primary, id string) (scopeSpec, error) {
	switch {
	case id == "" || id == "base":
		base := repoBase(root, repoName, dashboardBase, primary)
		return scopeSpec{kind: "range", from: mergeBaseOf(root, base), ref: base}, nil
	case id == "origin":
		from := upstreamForkPoint(root)
		if from == "" {
			return scopeSpec{}, fmt.Errorf("no upstream for this branch")
		}
		return scopeSpec{kind: "range", from: from}, nil
	case id == "staged":
		return scopeSpec{kind: "staged"}, nil
	case id == "unstaged":
		return scopeSpec{kind: "unstaged"}, nil
	case strings.HasPrefix(id, "commit:"):
		sha := strings.TrimPrefix(id, "commit:")
		if !plainSha(sha) {
			return scopeSpec{}, fmt.Errorf("bad commit id")
		}
		// a root commit has no parent; the empty-tree hash stands in for one
		parent := sha + "^"
		if _, err := git(root, "rev-parse", "--verify", "--quiet", parent+"^{commit}"); err != nil {
			parent = emptyTree
		}
		return scopeSpec{kind: "commit", sha: sha, from: parent}, nil
	}
	return scopeSpec{}, fmt.Errorf("unknown scope %q", id)
}

// emptyTree is git's fixed hash of the empty tree — the parent stand-in that
// makes a root commit diffable like any other.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func plainSha(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// upstreamForkPoint is where the branch left its upstream: everything after it
// is unpushed. Empty when the branch has no upstream.
func upstreamForkPoint(root string) string {
	up, err := git(root, "rev-parse", "--verify", "--quiet", "--abbrev-ref", "@{upstream}")
	if err != nil || up == "" {
		return ""
	}
	mb, err := git(root, "merge-base", up, "HEAD")
	if err != nil || mb == "" {
		return ""
	}
	return mb
}

// repoScopes builds one repo's section: the comparisons, then its commits.
func repoScopes(root, repoName, dashboardBase, primary string) ScopeRepo {
	out := ScopeRepo{Name: repoName}
	out.Branch, _ = git(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	base := repoBase(root, repoName, dashboardBase, primary)
	out.Upstream, _ = git(root, "rev-parse", "--verify", "--quiet", "--abbrev-ref", "@{upstream}")

	// vs the base branch — omitted when this checkout IS the base branch
	if forkPoint := mergeBaseOf(root, base); forkPoint != "HEAD" {
		out.Base = base
		s := Scope{ID: "base", Kind: "range", Label: "vs " + base, Hint: "committed + uncommitted"}
		fill(&s, numstat(root, forkPoint, false))
		out.Scopes = append(out.Scopes, s)
	}

	// vs the upstream — what is not pushed yet, plus what is not committed yet
	if from := upstreamForkPoint(root); from != "" {
		s := Scope{ID: "origin", Kind: "range", Label: "vs " + out.Upstream, Hint: "unpushed + uncommitted"}
		fill(&s, numstat(root, from, false))
		out.Scopes = append(out.Scopes, s)
	}

	staged := Scope{ID: "staged", Kind: "staged", Label: "staged", Hint: "index vs HEAD"}
	fill(&staged, numstatArgs(root, "diff", "--cached", "--numstat"))
	unstaged := Scope{ID: "unstaged", Kind: "unstaged", Label: "unstaged", Hint: "worktree vs index"}
	fill(&unstaged, numstatArgs(root, "diff", "--numstat"))
	if n, err := git(root, "ls-files", "--others", "--exclude-standard"); err == nil && n != "" {
		unstaged.Files += len(strings.Split(n, "\n")) // untracked files live here too
	}
	out.Scopes = append(out.Scopes, staged, unstaged)

	out.Scopes = append(out.Scopes, commitScopes(root, base)...)
	return out
}

func fill(s *Scope, stats map[string][2]int) {
	s.Files = len(stats)
	for _, v := range stats {
		s.Added += v[0]
		s.Deleted += v[1]
	}
}

// commitScopes lists the last commits of HEAD with their stats, marking the
// ones no remote has yet and the ones the base branch does not have yet. One
// `git log --numstat` carries all of it, rather than a diff call per commit.
func commitScopes(root, base string) []Scope {
	// "pushed" means the branch's OWN upstream has it. Asking whether any
	// remote-tracking ref contains the commit gets this wrong in the common
	// case: a feature branch cut from main and pushed makes main's unpushed
	// commits look pushed, though `git push` here would still send them.
	// Without an upstream there is nothing to compare to, so fall back to
	// "no remote has it at all".
	args := []string{"rev-list", "HEAD", "--not", "--remotes"}
	if up, err := git(root, "rev-parse", "--verify", "--quiet", "--abbrev-ref", "@{upstream}"); err == nil && up != "" {
		args = []string{"rev-list", up + "..HEAD"}
	}
	unpushed := map[string]bool{}
	if out, err := git(root, args...); err == nil {
		for _, sha := range strings.Fields(out) {
			unpushed[sha] = true
		}
	}

	// "merged" means the base branch already has this work, which is what
	// separates the branch being worked on from the history it sits on —
	// twenty commits where four are the branch and sixteen are main.
	//
	// `git cherry` rather than `rev-list base..HEAD`, because landing here
	// rewrites shas: a commit cherry-picked or rebased onto the base keeps its
	// patch and loses its identity, and ancestry alone would keep calling it
	// unmerged forever. cherry answers by patch-id, so `-` is "the base has
	// this change under another sha" and `+` is genuinely still outstanding.
	// Its two known edges are both quiet: two commits with byte-identical
	// diffs share a patch-id and one can vouch for the other, and a squash
	// merge produces a patch no single branch commit matches, so a squashed
	// branch stays marked unmerged.
	unmerged := map[string]bool{}
	if base != "" {
		if out, err := git(root, "cherry", base, "HEAD"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				if sign, sha, ok := strings.Cut(line, " "); ok && sign == "+" {
					unmerged[sha] = true
				}
			}
		}
	}

	// date AND time: a day's work is several commits, and "which of today's
	// five" is exactly what the list has to answer
	// The message body is multi-line, so it cannot share the line-based
	// numstat parsing: %x1e closes the header fields and everything after it
	// in the block is numstat.
	out, err := git(root, "log", "-n", strconv.Itoa(commitScopeLimit),
		"--numstat", "--date=format:%Y-%m-%d %H:%M", "--format=%x00%H%x1f%s%x1f%ad%x1f%an%x1f%b%x1e")
	if err != nil {
		return nil
	}
	var scopes []Scope
	for _, block := range strings.Split(out, "\x00") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		part := strings.SplitN(block, "\x1e", 2)
		head := strings.Split(part[0], "\x1f")
		if len(head) != 5 {
			continue
		}
		sha := head[0]
		s := Scope{
			ID: "commit:" + sha, Kind: "commit", Sha: sha[:9],
			Label: head[1], Date: head[2], Author: head[3],
			Body:   strings.TrimSpace(head[4]),
			Pushed: !unpushed[sha], Merged: !unmerged[sha],
		}
		var lines []string
		if len(part) == 2 {
			lines = strings.Split(part[1], "\n")
		}
		seen := map[string]bool{}
		for _, l := range lines {
			f := strings.Split(l, "\t")
			if len(f) != 3 {
				continue
			}
			a, _ := strconv.Atoi(f[0])
			d, _ := strconv.Atoi(f[1])
			s.Added += a
			s.Deleted += d
			if !seen[f[2]] {
				seen[f[2]] = true
				s.Files++
			}
		}
		scopes = append(scopes, s)
	}
	return scopes
}

func (d *grove) handleScopes(w http.ResponseWriter, r *http.Request) {
	c, ok := d.checkout(r.URL.Query().Get("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", r.URL.Query().Get("name")))
		return
	}
	repos := []ScopeRepo{}
	for _, repo := range diffRepos(c) {
		repos = append(repos, repoScopes(repo.Path, repo.Name, d.baseFor(c.Path), primaryRepo(c)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": c.Name, "repos": repos})
}
