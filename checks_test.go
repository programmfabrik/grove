package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The two spellings a GitHub remote comes in, and everything that is not one.
func TestGithubRepoFromRemote(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:programmfabrik/grove.git":              "programmfabrik/grove",
		"git@github.com:programmfabrik/grove":                  "programmfabrik/grove",
		"https://github.com/programmfabrik/grove.git":          "programmfabrik/grove",
		"https://github.com/programmfabrik/grove":              "programmfabrik/grove",
		"https://token@github.com/programmfabrik/grove":        "programmfabrik/grove",
		"git@github.com:programmfabrik/easydb-webfrontend.git": "programmfabrik/easydb-webfrontend",
	} {
		owner, name, ok := githubRepo(url)
		if !ok || owner+"/"+name != want {
			t.Errorf("githubRepo(%q) = %q/%q (%v), want %q", url, owner, name, ok, want)
		}
	}
	for _, url := range []string{
		"git@gitlab.com:someone/thing.git",
		"https://example.com/a/b.git",
		"/a/local/path",
		"",
	} {
		if _, _, ok := githubRepo(url); ok {
			t.Errorf("githubRepo(%q) claimed to be a GitHub remote", url)
		}
	}
}

// The dot has room for one word, so the worst state has to win: a green dot
// beside a failed job is exactly the lie a dashboard exists to prevent.
func TestCombineTakesTheWorstState(t *testing.T) {
	for _, c := range []struct {
		name  string
		total int
		runs  []CheckRun
		want  string
	}{
		{"nothing ran", 0, nil, "none"},
		{"all green", 2, []CheckRun{
			{Status: "completed", Conclusion: "success"},
			{Status: "completed", Conclusion: "success"},
		}, "success"},
		{"one still going", 2, []CheckRun{
			{Status: "completed", Conclusion: "success"},
			{Status: "in_progress"},
		}, "pending"},
		{"one failed among green", 2, []CheckRun{
			{Status: "completed", Conclusion: "success"},
			{Status: "completed", Conclusion: "failure"},
		}, "failure"},
		{"a failure outranks anything still running", 2, []CheckRun{
			{Status: "in_progress"},
			{Status: "completed", Conclusion: "timed_out"},
		}, "failure"},
		{"skipped and neutral are not failures", 2, []CheckRun{
			{Status: "completed", Conclusion: "skipped"},
			{Status: "completed", Conclusion: "neutral"},
		}, "success"},
		{"cancelled counts against", 1, []CheckRun{
			{Status: "completed", Conclusion: "cancelled"},
		}, "failure"},
	} {
		if got := combine(c.total, c.runs).State; got != c.want {
			t.Errorf("%s: combine = %q, want %q", c.name, got, c.want)
		}
	}
}

// fakeGitHub serves a commit's check runs and the workflow runs behind them —
// shaped like fylr's main tip on the day it said "passed an hour ago · 9h 46m":
// one push-triggered CI run, a scheduled cleanup, and a branch-deletion run for
// every branch anybody deleted, all filed under the same commit.
func fakeGitHub(t *testing.T, actions int) *httptest.Server {
	t.Helper()
	type run struct {
		Name        string `json:"name"`
		Status      string `json:"status"`
		Conclusion  string `json:"conclusion"`
		StartedAt   string `json:"started_at"`
		CompletedAt string `json:"completed_at"`
		CheckSuite  struct {
			ID int64 `json:"id"`
		} `json:"check_suite"`
	}
	mk := func(name, from, to string, suite int64) run {
		r := run{Name: name, Status: "completed", Conclusion: "success", StartedAt: from, CompletedAt: to}
		r.CheckSuite.ID = suite
		return r
	}
	runs := []run{
		mk("test the go code", "2026-09-23T01:34:00Z", "2026-09-23T01:37:00Z", 1),
		mk("apitests (postgres, 3)", "2026-09-23T01:40:00Z", "2026-09-23T03:13:00Z", 1),
		mk("update k8s deployments", "2026-09-23T03:16:00Z", "2026-09-23T03:16:30Z", 1),
		mk("cleanup-old-tar-files", "2026-09-23T02:03:00Z", "2026-09-23T02:05:00Z", 2),
		mk("remove the deleted branch's supervisor", "2026-09-23T05:43:00Z", "2026-09-23T05:43:20Z", 3),
		mk("remove the deleted branch's supervisor", "2026-09-23T12:25:00Z", "2026-09-23T12:25:20Z", 4),
	}
	workflows := []map[string]any{
		{"event": "push", "check_suite_id": 1},
		{"event": "schedule", "check_suite_id": 2},
		{"event": "delete", "check_suite_id": 3},
		{"event": "delete", "check_suite_id": 4},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/actions/runs"):
			if actions != http.StatusOK {
				w.WriteHeader(actions)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"workflow_runs": workflows})
		case strings.Contains(r.URL.Path, "/check-runs"):
			json.NewEncoder(w).Encode(map[string]any{"total_count": len(runs), "check_runs": runs})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestChecksAreTheCIOfTheCommitNotEverythingFiledUnderIt(t *testing.T) {
	srv := fakeGitHub(t, http.StatusOK)
	defer srv.Close()
	was := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = was }()

	c, err := fetchChecks(context.Background(), "token", "o", "r", "sha")
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 3 {
		t.Errorf("counted %d runs, want the push run's 3 — cleanup and branch deletion are not CI", c.Total)
	}
	if c.Started != "2026-09-23T01:34:00Z" || c.Finished != "2026-09-23T03:16:30Z" {
		t.Errorf("span %s → %s, want the push run's 01:34 → 03:16:30 — not the last branch deleted, at 12:25",
			c.Started, c.Finished)
	}
}

// A token without Actions access cannot say which run was which. Counting
// everything, as grove always did, beats showing nothing at all.
func TestChecksWithoutActionsAccessCountEverything(t *testing.T) {
	srv := fakeGitHub(t, http.StatusForbidden)
	defer srv.Close()
	was := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = was }()

	c, err := fetchChecks(context.Background(), "token", "o", "r", "sha")
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 6 {
		t.Errorf("counted %d runs, want all 6 when the events cannot be read", c.Total)
	}
}

// Which kind of problem it is decides how loudly the dashboard says it, so the
// kinds have to come out right — a rate limit in particular arrives as a 403,
// and reporting it as a refused credential sends somebody off to renew a token
// that is fine.
func TestGitHubFailuresAreNamed(t *testing.T) {
	for _, c := range []struct {
		name   string
		status int
		header string
		body   string
		want   error
	}{
		{"expired token", 401, "", `{"message":"Bad credentials"}`, errRefused},
		{"no access", 403, "", `{"message":"Resource not accessible"}`, errRefused},
		{"hourly limit", 403, "0", `{"message":"API rate limit exceeded"}`, errRateLimited},
		{"secondary limit", 403, "", `{"message":"You have exceeded a secondary rate limit"}`, errRateLimited},
		{"too many", 429, "", ``, errRateLimited},
		{"gone", 404, "", ``, errNotOnGitHub},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.header != "" {
					w.Header().Set("X-RateLimit-Remaining", c.header)
				}
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			var into any
			if err := ghGet(context.Background(), "t", srv.URL, &into); !errors.Is(err, c.want) {
				t.Errorf("%d %s: got %v, want %v", c.status, c.body, err, c.want)
			}
		})
	}
	kinds := map[error]string{
		errNoToken: "credential", errRefused: "credential", errRateLimited: "limited",
		context.DeadlineExceeded: "unreachable",
	}
	for err, want := range kinds {
		if got := problemOf(err).Kind; got != want {
			t.Errorf("problemOf(%v).Kind = %q, want %q", err, got, want)
		}
	}
}

// A failed refresh is not an answer. It keeps the last one on screen with the
// problem beside it, is tried again at the running pace rather than cached for
// the settled five minutes, and a refused credential is dropped so a renewed
// one is used at the very next ask.
func TestAFailedRefreshKeepsTheLastAnswerAndSaysWhy(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[]}`))
		default:
			w.Write([]byte(`{"total_count":1,"check_runs":[{"name":"ci","status":"completed","conclusion":"success",` +
				`"started_at":"2026-09-25T08:00:00Z","completed_at":"2026-09-25T08:10:00Z","check_suite":{"id":1}}]}`))
		}
	}))
	defer srv.Close()
	was := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = was }()

	c := newChecker()
	c.tok, c.tokAt = tokenSource{Token: "t", From: "test"}, time.Now()

	first, p := c.get(context.Background(), "o", "r", "sha")
	if p != nil || first.State != "success" {
		t.Fatalf("first answer: %+v, %+v", first, p)
	}

	// the settled answer goes stale, and the refresh is refused
	status = http.StatusUnauthorized
	c.byRef["o/r@sha"] = cached{checks: first, at: time.Now().Add(-checksSettled - time.Second)}
	got, p := c.get(context.Background(), "o", "r", "sha")
	if p == nil || p.Kind != "credential" {
		t.Fatalf("problem = %+v, want a credential problem", p)
	}
	if got.State != "success" {
		t.Errorf("the last answer went missing on a failed refresh: %+v", got)
	}
	if c.tok.Token != "" {
		t.Error("the refused token is still being used")
	}
	if e := c.byRef["o/r@sha"]; e.err == nil {
		t.Error("the failure was not remembered")
	} else if checksRunning >= checksSettled {
		t.Error("a failure must be retried sooner than a settled answer")
	}
}

// Checks switched on and a GitHub remote is a configuration, and a missing
// credential is an error in it — said even when nothing has been pushed yet,
// which is exactly when nothing else would ever have asked for one.
func TestAMissingCredentialIsAProblemWhenChecksAreOn(t *testing.T) {
	requireGit(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // default settings: checks on
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	gitRun(t, repo, "remote", "add", "origin", "git@github.com:o/r.git")

	d := &grove{state: map[string]*repoState{}, checks: newChecker()}
	d.checks.tokAt = time.Now() // "looked a moment ago and found nothing"

	w := httptest.NewRecorder()
	d.handleChecks(w, httptest.NewRequest("GET", "/api/checks?repo="+url.QueryEscape(repo), nil))
	var res struct {
		Problem *checksProblem `json:"problem"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Problem == nil || res.Problem.Kind != "credential" {
		t.Fatalf("problem = %+v, want a credential problem", res.Problem)
	}

	// not a GitHub remote: there was never anything to ask, and nothing to say
	gitRun(t, repo, "remote", "set-url", "origin", "git@gitlab.example.com:o/r.git")
	w = httptest.NewRecorder()
	d.handleChecks(w, httptest.NewRequest("GET", "/api/checks?repo="+url.QueryEscape(repo), nil))
	res.Problem = nil
	json.NewDecoder(w.Body).Decode(&res)
	if res.Problem != nil {
		t.Errorf("a remote that is not GitHub is a choice, not a problem: %+v", res.Problem)
	}
}
