package main

import "testing"

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
