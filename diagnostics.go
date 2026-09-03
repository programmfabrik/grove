package main

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

// What grove is standing on, and what it is missing.
//
// grove runs other programs, and when one of them is absent a feature quietly
// does not happen — the CI column stays empty and nobody is told whether that
// means "nothing is running" or "I could not ask". This says which, for each
// thing, along with what it is for and what is lost without it.

// Tool is one program grove runs, or would if it were there.
type Tool struct {
	Name    string `json:"name"`
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Needed is what grove uses it for; Missing is what goes without it.
	Needed   string `json:"needed"`
	Missing  string `json:"missing,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type Diagnostics struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Dir      string `json:"dir"`
	Tools    []Tool `json:"tools"`

	// GitHub is the credential the check column runs on, and whether it works.
	GitHub struct {
		From    string `json:"from,omitempty"`
		Working bool   `json:"working"`
		Detail  string `json:"detail"`
	} `json:"github"`
}

func (d *grove) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	var dg Diagnostics
	dg.Version = version
	dg.Platform = runtime.GOOS + "/" + runtime.GOARCH
	dg.Dir = d.dir()

	git := Tool{
		Name:     "git",
		Needed:   "everything. Every repository, worktree, diff and commit grove shows is git answering a question.",
		Required: true,
		Missing:  "grove does not start at all.",
	}
	git.Path = gitExe
	git.Found = true
	if v, err := exec.Command(gitExe, "--version").Output(); err == nil {
		git.Version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(v)), "git version "))
	}
	dg.Tools = append(dg.Tools, git)

	gh := Tool{
		Name:    "gh",
		Needed:  "a GitHub credential, so grove can ask whether what you pushed is passing its checks. It is only ever read from — grove never signs you in or asks for a token.",
		Missing: "the check column stays empty, unless GITHUB_TOKEN is set or your git credential helper already holds one for github.com.",
	}
	if p, err := exec.LookPath("gh"); err == nil {
		gh.Found, gh.Path = true, p
		if v, err := exec.Command(p, "--version").Output(); err == nil {
			gh.Version = strings.TrimSpace(strings.SplitN(strings.TrimPrefix(string(v), "gh version "), "\n", 2)[0])
		}
	}
	dg.Tools = append(dg.Tools, gh)

	tok := d.checks.token()
	dg.GitHub.From = tok.From
	switch {
	case tok.Token == "":
		dg.GitHub.Detail = "No credential found. grove looked at `gh auth token`, then GITHUB_TOKEN and GH_TOKEN, then your git credential helper for github.com. Sign in with `gh auth login` and the check column starts working — nothing needs restarting."
	default:
		// prove it rather than assume it: a token that is present and refused
		// looks exactly like one that works until something asks
		if err := pingGitHub(r.Context(), tok.Token); err != nil {
			dg.GitHub.Detail = "A credential from " + tok.From + " was found and GitHub refused it: " + err.Error()
		} else {
			dg.GitHub.Working = true
			dg.GitHub.Detail = "Working, using the credential from " + tok.From + "."
		}
	}
	writeJSON(w, http.StatusOK, dg)
}

func pingGitHub(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "grove/"+version)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errFromStatus(res.Status)
	}
	return nil
}

type statusErr string

func (e statusErr) Error() string { return string(e) }

func errFromStatus(s string) error { return statusErr(s) }
