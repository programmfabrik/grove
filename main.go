// Command grove serves a local dashboard over a directory full of git
// repositories, in three panes: the repos, the worktrees of the selected one,
// and a viewer for the selected worktree — its diff or everything else known
// about it.
//
// Started with no arguments it takes the working directory, or its repo's
// parent when started inside a checkout, so both of these list everything next
// to myrepo:
//
//	cd ~/src && grove
//	cd ~/src/myrepo && grove
//
// The dashboard reads git and nothing else. It never starts, stops or
// reconfigures anything. The single exception is the tree's context menu —
// unstage and discard, on files in view, behind a confirmation (revert.go).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type options struct {
	dir     string
	base    string
	refresh time.Duration
}

// repoState is one repository's scan: its worktrees and the branch they are
// compared against. Cached per repo and refreshed when a request finds it
// stale — with dozens of repos in the directory, scanning all of them on a
// timer would be waste.
type repoState struct {
	checkouts []Checkout
	base      string
	gitAt     time.Time
	err       string
}

type grove struct {
	opt options

	mu      sync.RWMutex
	repos   []Repo
	reposAt time.Time
	state   map[string]*repoState // keyed by repo path
}

func main() {
	var (
		addr = flag.String("addr", ":80", "listen address (\":80\" makes the dashboard http://localhost)")
		opt  options
		open = flag.Bool("open", false, "open the dashboard in the browser once it listens")
	)
	flag.StringVar(&opt.dir, "dir", "", "directory holding the repositories (default: the working directory, or its repo's parent)")
	flag.StringVar(&opt.base, "base", "", "branch the checkouts are compared against (default: the branch of the main checkout)")
	flag.DurationVar(&opt.refresh, "refresh", 20*time.Second, "how often the worktrees are re-scanned with git")
	flag.Parse()

	dir, err := startDir(opt.dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grove: %v\n", err)
		os.Exit(1)
	}
	opt.dir = dir

	d := &grove{opt: opt, state: map[string]*repoState{}}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// One scan before binding a port: an empty directory is worth reporting
	// now rather than as an empty first page.
	repos := d.reposList(ctx)
	if len(repos) == 0 {
		fmt.Fprintf(os.Stderr, "grove: no git repository in %s — pass -dir\n", dir)
		os.Exit(1)
	}

	srv := &http.Server{Addr: *addr, Handler: d.routes()}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grove: cannot listen on %s: %v\n", *addr, err)
		if strings.Contains(err.Error(), "address already in use") {
			fmt.Fprintf(os.Stderr, "  something else holds the port. Try -addr :8000\n")
		}
		os.Exit(1)
	}
	url := "http://localhost"
	if _, port, err := net.SplitHostPort(*addr); err == nil && port != "80" {
		url += ":" + port
	}
	fmt.Printf("grove: %s  —  %d repositories in %s\n", url, len(repos), dir)
	if *open {
		go openBrowser(url)
	}

	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(sc)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "grove: %v\n", err)
		os.Exit(1)
	}
}

func (d *grove) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repos", d.handleRepos)
	mux.HandleFunc("GET /api/state", d.handleState)
	mux.HandleFunc("GET /api/scopes", d.handleScopes)
	mux.HandleFunc("GET /api/diff", d.handleDiff)
	mux.HandleFunc("GET /api/blob", d.handleBlob)
	mux.HandleFunc("GET /api/lines", d.handleLines)
	mux.HandleFunc("POST /api/refresh", d.handleRefresh)
	mux.HandleFunc("POST /api/revert", d.handleRevert)
	mux.Handle("/", uiHandler())
	return mux
}

// State is one repository's payload — one request per refresh tick.
type State struct {
	Repo      string     `json:"repo"` // path
	Name      string     `json:"name"`
	Checkouts []Checkout `json:"checkouts"`
	Base      string     `json:"base"` // branch the ahead/behind and the diffs compare against
	GitAt     string     `json:"git_at"`
	GitError  string     `json:"git_error,omitempty"`
}

// reposList serves the repo list, rescanning it when it has gone stale.
func (d *grove) reposList(ctx context.Context) []Repo {
	d.mu.RLock()
	fresh := time.Since(d.reposAt) < d.opt.refresh && d.repos != nil
	repos := d.repos
	d.mu.RUnlock()
	if fresh {
		return repos
	}
	repos = scanRepos(ctx, d.opt.dir)
	d.mu.Lock()
	d.repos, d.reposAt = repos, time.Now()
	d.mu.Unlock()
	return repos
}

func (d *grove) handleRepos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"dir": d.opt.dir, "repos": d.reposList(r.Context())})
}

// repoStateFor scans a repository's worktrees when its cache has expired.
func (d *grove) repoStateFor(ctx context.Context, repo string) *repoState {
	d.mu.Lock()
	st := d.state[repo]
	if st == nil {
		st = &repoState{}
		d.state[repo] = st
	}
	d.mu.Unlock()

	if time.Since(st.gitAt) >= d.opt.refresh {
		d.refreshGit(ctx, repo, st)
	}
	return st
}

func (d *grove) handleState(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		if repos := d.reposList(r.Context()); len(repos) > 0 {
			repo = repos[0].Path
		}
	}
	st := d.repoStateFor(r.Context(), repo)

	d.mu.RLock()
	defer d.mu.RUnlock()
	writeJSON(w, http.StatusOK, State{
		Repo:      repo,
		Name:      filepath.Base(repo),
		Checkouts: st.checkouts,
		Base:      st.base,
		GitAt:     stamp(st.gitAt),
		GitError:  st.err,
	})
}

// handleRefresh drops the caches so the next reads rescan — the UI's explicit
// refresh button.
func (d *grove) handleRefresh(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	d.reposAt = time.Time{}
	for _, st := range d.state {
		st.gitAt = time.Time{}
	}
	d.mu.Unlock()
	d.handleState(w, r)
}

// checkout finds a worktree by name. Names are the directory basename, unique
// enough in practice, and the caller (a diff, a revert) always names one it
// just saw.
//
// The scanned repos are only what somebody has looked at, so a cold cache is
// normal: a deep link opens straight into a diff, and its request can arrive
// before — or instead of — the /api/state that would have scanned the repo.
// Rather than 404 in that race, find which repo owns the name and scan that
// one. `git worktree list` per repo is one cheap call, and only on a miss.
func (d *grove) checkout(name string) (Checkout, bool) {
	if c, ok := d.cachedCheckout(name); ok {
		return c, true
	}
	for _, r := range d.reposList(context.Background()) {
		out, err := git(r.Path, "worktree", "list", "--porcelain")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			p, ok := strings.CutPrefix(line, "worktree ")
			if !ok || filepath.Base(p) != name {
				continue
			}
			d.repoStateFor(context.Background(), r.Path)
			return d.cachedCheckout(name)
		}
	}
	return Checkout{}, false
}

func (d *grove) cachedCheckout(name string) (Checkout, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, st := range d.state {
		for _, c := range st.checkouts {
			if c.Name == name {
				return c, true
			}
		}
	}
	return Checkout{}, false
}

// baseFor is the branch a checkout's diffs compare against: the branch of the
// main checkout of ITS repository.
func (d *grove) baseFor(path string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, st := range d.state {
		for _, c := range st.checkouts {
			if c.Path == path {
				return st.base
			}
		}
	}
	return ""
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func openBrowser(url string) {
	time.Sleep(200 * time.Millisecond)
	if runtime.GOOS == "darwin" {
		exec.Command("open", url).Run()
	}
}
