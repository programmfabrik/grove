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
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// version is stamped in by the release build (-ldflags "-X main.version=…").
// A build from a working tree says so rather than claiming a number nobody
// tagged.
var version = "dev"

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
		addr = flag.String("addr", defaultAddr, "listen address (loopback, so nothing off this machine can reach it)")
		opt  options
		open = flag.Bool("open", false, "open the dashboard in the browser once it listens")
	)
	flag.StringVar(&opt.dir, "dir", "", "directory holding the repositories (default: the working directory, or its repo's parent)")
	flag.StringVar(&opt.base, "base", "", "branch the checkouts are compared against (default: the branch of the main checkout)")
	flag.DurationVar(&opt.refresh, "refresh", 20*time.Second, "how often the worktrees are re-scanned with git")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("grove %s\n", version)
		return
	}

	// Everything grove shows comes out of git, startDir included, so resolve it
	// before the first call rather than serving a dashboard of empty panes.
	if err := findGit(); err != nil {
		fmt.Fprintf(os.Stderr, "grove: %v\n", err)
		os.Exit(1)
	}

	dir, err := startDir(opt.dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grove: %v\n", err)
		os.Exit(1)
	}
	opt.dir = dir

	d := &grove{opt: opt, state: map[string]*repoState{}}

	// Everything above is the dashboard; run is the front door, and there are
	// two of them. The default build serves it to a browser and prints where;
	// `-tags desktop` puts it in a window of its own (front_cli.go,
	// front_desktop.go). Both hold the same *grove and the same routes.
	if err := run(d, *addr, isSet("addr"), *open); err != nil {
		fmt.Fprintf(os.Stderr, "grove: %v\n", err)
		os.Exit(1)
	}
}

// dir is the directory the repo list is scanned in. Behind the lock because
// the desktop front door can point the dashboard somewhere else while it runs.
func (d *grove) dir() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.opt.dir
}

// setDir points the dashboard at another directory. The caches describe the
// old one entirely, so all of them go. A directory with nothing in it is
// refused rather than swapped in: an empty dashboard cannot say why it is
// empty, and the caller can.
func (d *grove) setDir(dir string) error {
	repos := scanRepos(context.Background(), dir)
	if len(repos) == 0 {
		return fmt.Errorf("no git repository in %s", dir)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.opt.dir = dir
	d.repos, d.reposAt = repos, time.Now()
	d.state = map[string]*repoState{}
	return nil
}

// dropCaches makes the next read rescan everything.
func (d *grove) dropCaches() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reposAt = time.Time{}
	for _, st := range d.state {
		st.gitAt = time.Time{}
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
	repos = scanRepos(ctx, d.dir())
	d.mu.Lock()
	d.repos, d.reposAt = repos, time.Now()
	d.mu.Unlock()
	return repos
}

func (d *grove) handleRepos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"dir": d.dir(), "repos": d.reposList(r.Context())})
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
	d.dropCaches()
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
			if !ok || filepath.Base(normPath(p)) != name {
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
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Run()
	case "windows":
		// not `cmd /c start`: it takes the first quoted argument as the window
		// title, and a URL with an & in it ends the command there
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
	default:
		exec.Command("xdg-open", url).Run()
	}
}

// defaultAddr binds the loopback interface, not the wildcard: grove reads
// every repository on the machine and has one endpoint that deletes untracked
// files, so a dashboard reachable from the network is not a default anybody
// asked for. `-addr :80` restores it for whoever wants it.
//
// Port 80 is what makes the address http://localhost. macOS lets an
// unprivileged process bind it; the other two often do not, which is what
// fallbackAddr is for.
const (
	defaultAddr  = "127.0.0.1:80"
	fallbackAddr = "127.0.0.1:7433"
)

// isSet reports whether a flag was given on the command line, as opposed to
// holding its default.
func isSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// listen binds addr, and lets the DEFAULT move. Port 80 belongs to root on
// Linux and http.sys can hold it on Windows, and refusing to start is a worse
// answer than starting somewhere else and saying so. An explicit -addr is
// taken literally: the user named a port, and quietly using another would be a
// lie about where the dashboard is.
func listen(addr, fallback string, explicit bool) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil || explicit {
		return ln, err
	}
	if alt, err2 := net.Listen("tcp", fallback); err2 == nil {
		return alt, nil
	}
	return nil, err // the first failure is the one worth reporting
}

// dashboardURL is what to type into a browser, read off the listener rather
// than off the requested address — the port may have moved, and a port of 0
// does not name one at all until it is bound.
func dashboardURL(ln net.Listener) string {
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "http://localhost"
	}
	switch host {
	case "", "0.0.0.0", "::", "127.0.0.1", "::1":
		host = "localhost"
	}
	if port == "80" {
		return "http://" + host
	}
	return "http://" + net.JoinHostPort(host, port)
}
