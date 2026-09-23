package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
