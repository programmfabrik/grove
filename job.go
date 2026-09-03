package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Pushing and pulling take seconds, sometimes tens of them, and a button that
// says "Pulling…" for that long is indistinguishable from one that has hung.
// So the work runs as a job and the page watches it: every git command as it
// is started, and what it said when it finished.
//
// It is a transcript, not a log. What is shown is exactly what was run, in the
// order it ran, so that somebody who wants to know what grove did to their
// repository can read it — and reproduce it in a terminal if they would rather
// do it themselves next time.

// Line is one entry in a job's transcript.
type Line struct {
	// Kind is what to make of it: "cmd" is a command about to run, "out" what
	// it said, "ok" a step that worked, "err" one that did not, "note" grove
	// speaking for itself.
	Kind string `json:"kind"`
	Dir  string `json:"dir,omitempty"` // which repository, for a cmd
	Text string `json:"text"`
}

type Job struct {
	mu      sync.Mutex
	lines   []Line
	done    bool
	results []RepoResult
	repos   []RemoteRepo
	at      time.Time
}

func (j *Job) add(l Line) {
	j.mu.Lock()
	j.lines = append(j.lines, l)
	j.at = time.Now()
	j.mu.Unlock()
}

func (j *Job) cmd(dir string, args []string) {
	j.add(Line{Kind: "cmd", Dir: dir, Text: "git " + strings.Join(args, " ")})
}
func (j *Job) out(text string) {
	if text = strings.TrimRight(text, "\n"); text != "" {
		j.add(Line{Kind: "out", Text: text})
	}
}
func (j *Job) note(text string) { j.add(Line{Kind: "note", Text: text}) }

func (j *Job) finish(results []RepoResult, repos []RemoteRepo) {
	j.mu.Lock()
	j.results, j.repos, j.done, j.at = results, repos, true, time.Now()
	j.mu.Unlock()
}

// snapshot is what the page is told, from `after` onwards — only the lines it
// has not seen, so watching a long job does not mean re-sending it each time.
func (j *Job) snapshot(after int) map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	from := after
	if from < 0 || from > len(j.lines) {
		from = len(j.lines)
	}
	m := map[string]any{"lines": append([]Line{}, j.lines[from:]...), "next": len(j.lines), "done": j.done}
	if j.done {
		m["results"], m["repos"] = j.results, j.repos
	}
	return m
}

// jobs are kept only long enough for the page to read the end of one.
type jobs struct {
	mu sync.Mutex
	m  map[string]*Job
}

func newJobs() *jobs { return &jobs{m: map[string]*Job{}} }

func (js *jobs) start() (string, *Job) {
	b := make([]byte, 8)
	rand.Read(b)
	id := hex.EncodeToString(b)
	j := &Job{at: time.Now()}
	js.mu.Lock()
	js.m[id] = j
	// anything finished and unread for a quarter of an hour is nobody's
	for k, old := range js.m {
		old.mu.Lock()
		stale := old.done && time.Since(old.at) > 15*time.Minute
		old.mu.Unlock()
		if stale {
			delete(js.m, k)
		}
	}
	js.mu.Unlock()
	return id, j
}

func (js *jobs) get(id string) *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.m[id]
}

// handleRun starts a push or a pull and answers with the job to watch.
func (d *grove) handleRun(w http.ResponseWriter, r *http.Request) {
	var req remoteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return
	}
	switch req.Action {
	case "push", "rebase", "merge", "ff":
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("%q is not something to watch", req.Action))
		return
	}
	c, ok := d.checkout(req.Name)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", req.Name))
		return
	}
	id, job := d.jobs.start()
	go d.runRemote(c, req, job)
	writeJSON(w, http.StatusOK, map[string]any{"job": id})
}

// handleJob is the page watching one.
func (d *grove) handleJob(w http.ResponseWriter, r *http.Request) {
	j := d.jobs.get(r.URL.Query().Get("job"))
	if j == nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such job — it may have finished a while ago"))
		return
	}
	after := 0
	fmt.Sscanf(r.URL.Query().Get("after"), "%d", &after)
	writeJSON(w, http.StatusOK, j.snapshot(after))
}

// runRemote is one push or pull, narrated. It does what handleRemoteAction
// does and says so as it goes: fetch everything under the checkout, work out
// where that leaves things, then act on the one repository chosen.
func (d *grove) runRemote(c Checkout, req remoteRequest, job *Job) {
	all := diffRepos(c)
	var results []RepoResult

	job.note("Fetching every repository under " + c.Name + ", because whether the parent may push depends on what the submodules' remotes hold.")
	for _, s := range all {
		if out, err := say(job, s.Name, s.Path, "fetch", "--prune"); err != nil {
			results = append(results, RepoResult{Repo: s.Name, Detail: "could not reach the remote", Git: out})
		}
	}

	standing := map[string]RemoteRepo{}
	for _, rr := range remoteRepos(c) {
		standing[rr.Name] = rr
	}

	want := map[string]bool{}
	for _, n := range req.Repos {
		want[n] = true
	}
	// submodules before the parent: pushing a submodule is what makes the
	// parent's pointer to it followable
	for i := len(all) - 1; i >= 0; i-- {
		t := all[i]
		if len(want) > 0 && !want[t.Name] {
			continue
		}
		job.note(verbFor(req.Action) + " " + t.Name + "…")
		results = append(results, doRemote(t, standing[t.Name], req.Action, req.Remote, job))
	}

	d.dropCaches()
	after := remoteRepos(c)
	for _, r := range results {
		if !r.Ok {
			job.add(Line{Kind: "err", Text: r.Repo + ": " + r.Detail})
			continue
		}
		job.add(Line{Kind: "ok", Text: r.Repo + ": " + r.Detail})
	}
	job.finish(results, after)
}

func verbFor(action string) string {
	switch action {
	case "push":
		return "Pushing"
	case "rebase":
		return "Rebasing"
	case "merge":
		return "Merging"
	}
	return "Fast-forwarding"
}
