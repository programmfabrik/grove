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

	// GitHub is the one question "is gh installed" and "do the checks work"
	// were each answering half of.
	GitHub struct {
		Tool        Tool     `json:"tool"`
		From        string   `json:"from,omitempty"`
		Working     bool     `json:"working"`
		Detail      string   `json:"detail"`
		Fix         []string `json:"fix,omitempty"`
		Alternative string   `json:"alternative,omitempty"`
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

	dg.Tools = append(dg.Tools, git)

	ghPath, ghErr := exec.LookPath("gh")
	ghVersion := ""
	if ghErr == nil {
		if v, err := exec.Command(ghPath, "--version").Output(); err == nil {
			ghVersion = strings.TrimSpace(strings.SplitN(strings.TrimPrefix(string(v), "gh version "), "\n", 2)[0])
		}
	}

	// gh and "GitHub checks" were two boxes saying overlapping halves of one
	// thing. There is one question — can grove ask GitHub about your commits —
	// and gh is only the commonest answer to it.
	dg.GitHub.Tool = Tool{Name: "gh", Found: ghErr == nil, Path: ghPath, Version: ghVersion}
	if loadSettings().NoChecks {
		dg.GitHub.Detail = "Switched off below. grove makes no request to GitHub at all, and the branch names carry no colour."
		writeJSON(w, http.StatusOK, dg)
		return
	}

	tok := d.checks.token()
	dg.GitHub.From = tok.From
	switch {
	case tok.Token == "" && ghErr != nil:
		// nothing to sign in with, so say what to install first
		dg.GitHub.Detail = "No GitHub credential, and `gh` is not installed. grove does not sign you in — it uses a credential you already have."
		dg.GitHub.Fix = []string{
			installGh() + "  — install the GitHub CLI",
			"gh auth login  — sign in once; grove picks it up with no restart",
		}
		dg.GitHub.Alternative = "Or set GITHUB_TOKEN to a token with `repo` read access, or let your git credential helper hold one for github.com. grove looks in all three, in that order."
	case tok.Token == "":
		dg.GitHub.Detail = "`gh` is installed and not signed in, and nothing else offered a credential."
		dg.GitHub.Fix = []string{"gh auth login  — sign in once; grove picks it up with no restart"}
		dg.GitHub.Alternative = "Or set GITHUB_TOKEN, or let your git credential helper hold one for github.com."
	default:
		// prove it rather than assume it: a token that is present and refused
		// looks exactly like one that works until something asks
		if err := pingGitHub(r.Context(), tok.Token); err != nil {
			dg.GitHub.Detail = "A credential from " + tok.From + " was found and GitHub refused it: " + err.Error()
			dg.GitHub.Fix = []string{"gh auth login  — sign in again"}
		} else {
			dg.GitHub.Working = true
			dg.GitHub.Detail = "Working, using the credential from " + tok.From + "."
		}
	}
	writeJSON(w, http.StatusOK, dg)
}

// installGh is the line to type on this machine, since "install gh" is not
// the same sentence everywhere.
func installGh() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install gh"
	case "windows":
		return "winget install --id GitHub.cli"
	default:
		return "see https://github.com/cli/cli#installation"
	}
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
