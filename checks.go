package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Whether GitHub is testing what you pushed.
//
// grove reads git, and git does not know: a check run lives on GitHub and
// nowhere else. So this is the one place grove talks to a service rather than
// a program — and only ever to read, only about commits that are already
// pushed, and only when it can find a credential you already have.
//
// It never asks for one. Whatever `gh` is signed in as, or GITHUB_TOKEN, or
// what the git credential helper already holds for github.com — in that order,
// and if none of them answers, the column simply says nothing. A dashboard
// that demanded a personal access token before it would draw a dot would be a
// worse dashboard.

// Checks is what GitHub says about one commit.
type Checks struct {
	// State is one of: success, pending, failure, none. "none" means GitHub
	// has the commit and nothing has run for it; an empty Checks means grove
	// could not ask.
	State string     `json:"state"`
	Total int        `json:"total"`
	Runs  []CheckRun `json:"runs,omitempty"`
	// Started is when the earliest run began and Finished when the last one
	// ended — empty while anything is still going, which is what lets the row
	// say "running for 4m" rather than a time that has not happened yet.
	Started  string `json:"started,omitempty"`
	Finished string `json:"finished,omitempty"`
	URL      string `json:"url,omitempty"` // the commit's checks page
	Sha      string `json:"sha,omitempty"`
}

type CheckRun struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	URL         string `json:"url,omitempty"`
}

// ghRemote is the owner and repository a remote URL points at, for the two
// spellings a GitHub remote comes in.
var ghRemote = regexp.MustCompile(`(?:git@github\.com:|https://(?:[^@/]+@)?github\.com/)([^/]+)/(.+?)(?:\.git)?$`)

func githubRepo(remoteURL string) (owner, name string, ok bool) {
	m := ghRemote.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// tokenSource says where a credential came from, so the settings can tell you
// which one is being used and the others can be ruled out.
type tokenSource struct {
	Token string `json:"-"`
	From  string `json:"from,omitempty"` // gh | env | credential helper
	Err   string `json:"error,omitempty"`
}

// findToken asks, in order, the places a developer's GitHub credential
// already lives. It never prompts: `git credential fill` is given a closed
// stdin so a helper that wants to ask a question fails instead of hanging a
// dashboard on an invisible prompt.
func findToken() tokenSource {
	if gh, err := lookPath("gh"); err == nil {
		if out, err := exec.Command(gh, "auth", "token").Output(); err == nil {
			if t := strings.TrimSpace(string(out)); t != "" {
				return tokenSource{Token: t, From: "gh"}
			}
		}
	}
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(k)); t != "" {
			return tokenSource{Token: t, From: k}
		}
	}
	cmd := exec.Command(gitExe, "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	out, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if t, ok := strings.CutPrefix(line, "password="); ok && strings.TrimSpace(t) != "" {
				return tokenSource{Token: strings.TrimSpace(t), From: "git credential helper"}
			}
		}
	}
	return tokenSource{Err: "no GitHub credential found"}
}

// checker holds what GitHub has said, keyed by commit. A concluded run does
// not change, and one still going is worth asking about again shortly.
type checker struct {
	mu    sync.Mutex
	byRef map[string]cached
	tok   tokenSource
	tokAt time.Time
}

type cached struct {
	checks Checks
	at     time.Time
	err    string
}

func newChecker() *checker { return &checker{byRef: map[string]cached{}} }

const (
	checksSettled = 5 * time.Minute
	checksRunning = 20 * time.Second
)

func (c *checker) token() tokenSource {
	c.mu.Lock()
	defer c.mu.Unlock()
	// a token can be revoked or expire; re-ask now and then rather than
	// remembering a refusal for the life of the process
	if c.tok.Token == "" && time.Since(c.tokAt) < time.Minute {
		return c.tok
	}
	if c.tok.Token != "" && time.Since(c.tokAt) < 30*time.Minute {
		return c.tok
	}
	c.tok, c.tokAt = findToken(), time.Now()
	return c.tok
}

// get answers from what it knows, and asks GitHub only when that has gone
// stale. It never blocks on the network for longer than the caller's context.
func (c *checker) get(ctx context.Context, owner, repo, sha string) (Checks, string) {
	key := owner + "/" + repo + "@" + sha
	c.mu.Lock()
	if e, ok := c.byRef[key]; ok {
		fresh := checksSettled
		if e.checks.State == "pending" {
			fresh = checksRunning
		}
		if time.Since(e.at) < fresh {
			c.mu.Unlock()
			return e.checks, e.err
		}
	}
	c.mu.Unlock()

	tok := c.token()
	if tok.Token == "" {
		return Checks{}, tok.Err
	}
	checks, err := fetchChecks(ctx, tok.Token, owner, repo, sha)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	c.mu.Lock()
	c.byRef[key] = cached{checks: checks, at: time.Now(), err: msg}
	c.mu.Unlock()
	return checks, msg
}

func fetchChecks(ctx context.Context, token, owner, repo, sha string) (Checks, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/check-runs?per_page=30", owner, repo, sha)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Checks{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "grove/"+version)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Checks{}, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		// the commit is not on GitHub, or this account cannot see the repo
		return Checks{}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return Checks{}, fmt.Errorf("GitHub refused the credential (%s)", res.Status)
	default:
		return Checks{}, fmt.Errorf("GitHub: %s", res.Status)
	}
	var body struct {
		Total     int `json:"total_count"`
		CheckRuns []struct {
			Name        string `json:"name"`
			Status      string `json:"status"`
			Conclusion  string `json:"conclusion"`
			StartedAt   string `json:"started_at"`
			CompletedAt string `json:"completed_at"`
			HTMLURL     string `json:"html_url"`
		} `json:"check_runs"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Checks{}, err
	}
	runs := make([]CheckRun, 0, len(body.CheckRuns))
	for _, r := range body.CheckRuns {
		runs = append(runs, CheckRun{
			Name: r.Name, Status: r.Status, Conclusion: r.Conclusion,
			StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, URL: r.HTMLURL,
		})
	}
	c := combine(body.Total, runs)
	c.Sha = sha
	if c.Total > 0 {
		c.URL = fmt.Sprintf("https://github.com/%s/%s/commit/%s/checks", owner, repo, sha)
	}
	return c, nil
}

// combine reduces a commit's runs to the one thing the list has room for. The
// worst state wins: a green dot beside a failed job would be a lie of exactly
// the kind a dashboard exists to prevent.
func combine(total int, runs []CheckRun) Checks {
	c := Checks{State: "none", Total: total, Runs: runs}
	if total == 0 {
		return c
	}
	state := "success"
	for _, r := range runs {
		if r.Status != "completed" {
			if state != "failure" {
				state = "pending"
			}
			continue
		}
		switch r.Conclusion {
		case "success", "neutral", "skipped":
		case "":
			if state != "failure" {
				state = "pending"
			}
		default: // failure, timed_out, cancelled, action_required, stale
			state = "failure"
		}
	}
	c.State = state

	// when the whole thing started, and when it ended — the latter only once
	// everything has, since "finished at" while a job is still running is not
	// a time that has happened
	for _, r := range runs {
		if r.StartedAt != "" && (c.Started == "" || r.StartedAt < c.Started) {
			c.Started = r.StartedAt
		}
	}
	if state != "pending" {
		for _, r := range runs {
			if r.CompletedAt > c.Finished {
				c.Finished = r.CompletedAt
			}
		}
	}
	return c
}

// handleChecks answers with what GitHub says about each checkout of one
// repository, keyed by checkout name.
//
// It asks about the commit the REMOTE has, not the one on disk: "is what I
// pushed being tested" is the question, and a local commit nobody has seen is
// not being tested by anybody. A checkout with no upstream is simply absent
// from the answer.
// pushedTips is what each local branch has ON THE REMOTE, by branch name.
//
// `@{upstream}` is not that, and taking it for that is how a branch nobody had
// ever pushed came to wear main's green tick. `git checkout -b work
// origin/main` sets branch.work.merge to refs/heads/main, so work's upstream
// tip is MAIN's commit — GitHub answers about main, truthfully, and the answer
// gets shown against work. A branch is only being tested if its OWN remote
// branch exists, which is what an upstream whose remoteref is refs/heads/<its
// own name> means. %(upstream:remoteref) says so directly, rather than by
// cutting a remote's name off the front of a string that may contain slashes
// on both sides of the join.
//
// The rarity this turns away is a branch pushed under another name
// (`git push origin work:other`), which loses its badge. That is the safe
// direction to be wrong in: a missing tick is looked into, a green one that
// belongs to another branch is believed.
//
// Read once for the repository, not once per checkout: refs are shared by
// every worktree of a repo, so this is two git calls where it used to be one
// per checkout on a thirty-second timer.
func pushedTips(repo string) map[string]string {
	heads, err := git(repo, "for-each-ref", "--format=%(refname:short)\x1f%(upstream:remoteref)\x1f%(upstream:short)", "refs/heads/")
	if err != nil {
		return nil
	}
	want := map[string]string{} // branch -> the remote-tracking ref that is ITS own
	for _, line := range strings.Split(heads, "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 3 || f[2] == "" {
			continue // no upstream at all: never pushed
		}
		if strings.TrimPrefix(f[1], "refs/heads/") != f[0] {
			continue // tracks another branch, whose checks are not this one's
		}
		want[f[0]] = f[2]
	}
	if len(want) == 0 {
		return nil
	}
	remotes, err := git(repo, "for-each-ref", "--format=%(refname:short)\x1f%(objectname)", "refs/remotes/")
	if err != nil {
		return nil
	}
	at := map[string]string{}
	for _, line := range strings.Split(remotes, "\n") {
		if name, sha, ok := strings.Cut(line, "\x1f"); ok {
			at[name] = sha
		}
	}
	tips := map[string]string{}
	for branch, ref := range want {
		if sha := at[ref]; sha != "" {
			tips[branch] = sha
		}
	}
	return tips
}

func (d *grove) handleChecks(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no repository named"))
		return
	}
	if loadSettings().NoChecks {
		writeJSON(w, http.StatusOK, map[string]any{"checks": map[string]Checks{}, "off": true})
		return
	}
	origin, err := git(repo, "remote", "get-url", "origin")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"checks": map[string]Checks{}})
		return
	}
	owner, name, ok := githubRepo(origin)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"checks": map[string]Checks{},
			"note":   "origin is not a GitHub remote, so there is nothing to ask",
		})
		return
	}

	d.mu.RLock()
	st := d.state[repo]
	var checkouts []Checkout
	if st != nil {
		checkouts = append(checkouts, st.checkouts...)
	}
	d.mu.RUnlock()

	type answer struct {
		name   string
		checks Checks
		err    string
	}
	tips := pushedTips(repo)
	ch := make(chan answer, len(checkouts))
	asked := 0
	for _, c := range checkouts {
		sha := tips[c.Branch]
		if c.Detached || sha == "" {
			continue // nothing this checkout has pushed as itself
		}
		asked++
		go func(checkout, sha string) {
			checks, msg := d.checks.get(r.Context(), owner, name, sha)
			ch <- answer{checkout, checks, msg}
		}(c.Name, sha)
	}
	out := map[string]Checks{}
	var firstErr string
	for i := 0; i < asked; i++ {
		a := <-ch
		if a.err != "" && firstErr == "" {
			firstErr = a.err
		}
		if a.checks.State != "" {
			out[a.name] = a.checks
		}
	}
	res := map[string]any{"checks": out}
	if firstErr != "" {
		res["error"] = firstErr
	}
	writeJSON(w, http.StatusOK, res)
}
