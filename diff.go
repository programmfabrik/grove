package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The diff tab shows everything a worktree holds that the base branch does not
// — the commits on its branch AND the uncommitted work, in one tree. They are
// not separate modes: a file is listed once and marked with where its change
// lives (branch / uncommitted / both), because "what is in this worktree" is
// the question, and whether a line is committed yet is a detail of it.
//
// That makes one diff command cover every case: the fork point (`git
// merge-base <base> HEAD`) against the working tree. For a checkout on the
// base branch, or a detached one, the fork point IS HEAD, so the same command
// degrades to the plain uncommitted diff with nothing special-cased.
//
// A checkout with submodules is several repositories — itself, and every
// submodule checked out under it, nested ones included (diffRepos below).
// Work on a ticket usually spans more than one, so every checked-out repo is
// scanned, each file tagged with the `repo` it came from.
//
//	GET /api/diff?name=myrepo1                          -> the repos and their changed files
//	GET /api/diff?name=myrepo1&repo=<submodule>&file=…  -> the unified diff of one

// commentIgnores are the patterns behind "ignore comments". git does the work:
// -I drops a hunk whose changed lines ALL match, so a file whose only change is
// a reworded comment disappears from the diff entirely. Its own regex engine,
// hence POSIX classes rather than \s.
//
// Block-comment continuation lines ("  * more text") are deliberately absent:
// they are indistinguishable from a Markdown bullet, and hiding a real content
// change would be worse than showing a comment. Go and TypeScript here use //
// almost throughout.
var commentIgnores = []string{
	`^[[:space:]]*//`,   // Go, TypeScript, JavaScript, C-like
	`^[[:space:]]*#`,    // YAML, shell, Python, CoffeeScript, Makefile
	`^[[:space:]]*--`,   // SQL
	`^[[:space:]]*/\*`,  // block comment opener
	`^[[:space:]]*\*/`,  // block comment closer
	`^[[:space:]]*<!--`, // HTML, XML
}

// ignoreArgs are the -I flags for a git invocation, or nothing.
func ignoreArgs(ignore bool) []string {
	if !ignore {
		return nil
	}
	args := make([]string, 0, len(commentIgnores)*2)
	for _, re := range commentIgnores {
		args = append(args, "-I", re)
	}
	return args
}

// diffMax caps one file's diff. A generated file or a vendored blob would
// otherwise push megabytes into the browser for nothing.
const diffMax = 400 << 10

type DiffFile struct {
	Repo   string `json:"repo"` // the checkout itself, or one of its submodules
	Path   string `json:"path"`
	Status string `json:"status"` // modified | added | deleted | renamed | new | …
	// Origin says where this file's change lives: "branch" (committed on this
	// branch since the fork point), "working" (uncommitted only) or "both".
	Origin string `json:"origin"`
	// Preview is set when a browser can render this file itself (image, pdf,
	// video, audio) — the viewer then shows it instead of "Binary files differ".
	Preview   string `json:"preview,omitempty"`
	Added     int    `json:"added"`
	Deleted   int    `json:"deleted"`
	Untracked bool   `json:"untracked,omitempty"`
	// Merged marks a committed file whose change the branch this range is
	// measured against already holds, found by content (markLanded).
	Merged bool `json:"merged,omitempty"`
}

type DiffList struct {
	Name  string     `json:"name"`
	Repo  string     `json:"repo"`
	Files []DiffFile `json:"files"`
}

// subRepo is one of the repositories a checkout spans, named for the scope
// and diff lists.
type subRepo struct{ Name, Path string }

// primaryRepo names a checkout's own repository in the scope and diff lists.
func primaryRepo(c Checkout) string {
	if c.Repo != "" {
		return c.Repo
	}
	return "repo"
}

// diffRepos lists the repositories a checkout spans: itself, then every
// submodule checked out under it, nested ones included, in .gitmodules order.
// Each is a git checkout of its own and needs its own `git diff`. A submodule
// that was never initialised is an empty directory without a .git and is
// left out. Names are the submodule directory's base name; a name that
// repeats at another depth keeps its full relative path instead.
func diffRepos(c Checkout) []subRepo {
	out := []subRepo{{primaryRepo(c), c.Path}}
	seen := map[string]bool{out[0].Name: true}
	var walk func(root, prefix string, depth int)
	walk = func(root, prefix string, depth int) {
		if depth > 4 {
			return
		}
		for _, rel := range submodulePaths(root) {
			path := filepath.Join(root, rel)
			if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
				continue
			}
			name := filepath.Base(rel)
			if seen[name] {
				name = filepath.ToSlash(filepath.Join(prefix, rel))
			}
			seen[name] = true
			out = append(out, subRepo{name, path})
			walk(path, filepath.Join(prefix, rel), depth+1)
		}
	}
	walk(c.Path, "", 1)
	return out
}

// submodulePaths reads the submodule paths a checkout declares. No
// .gitmodules is the common case and makes git exit non-zero: no submodules.
func submodulePaths(root string) []string {
	out, err := git(root, "config", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if _, p, ok := strings.Cut(line, " "); ok && p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

type DiffText struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
	// Total is how many lines the file has on the scope's AFTER side, so the
	// viewer knows from the first render whether anything is hidden below the
	// last hunk — without it the "more" expander has to appear on every file
	// and only discovers it was pointing at nothing once clicked. 0 means the
	// after side has no file at all (a deletion).
	Total     int  `json:"total"`
	Truncated bool `json:"truncated,omitempty"`
}

func (d *grove) handleDiff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := d.checkout(q.Get("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", q.Get("name")))
		return
	}
	want := q.Get("repo")
	if want == "" {
		want = primaryRepo(c)
	}
	root := ""
	for _, repo := range diffRepos(c) {
		if repo.Name == want {
			root = repo.Path
		}
	}
	if root == "" {
		writeErr(w, http.StatusNotFound, fmt.Errorf("%s has no %s checkout", c.Name, want))
		return
	}
	spec, err := resolveScope(root, want, d.baseFor(c.Path), primaryRepo(c), q.Get("scope"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	ignore := q.Get("ignore_comments") == "1"
	if file := q.Get("file"); file != "" {
		if !safeRepoPath(file) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("path must stay inside the checkout"))
			return
		}
		text, truncated, err := fileDiff(root, file, q.Get("untracked") == "1", spec, ignore)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		total := 0
		if body, err := fileAt(root, file, spec, false); err == nil && body != "" {
			total = strings.Count(strings.TrimSuffix(body, "\n"), "\n") + 1
		}
		writeJSON(w, http.StatusOK, DiffText{Path: file, Diff: text, Total: total, Truncated: truncated})
		return
	}

	files, err := scopeFiles(root, spec, ignore)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	for i := range files {
		files[i].Repo = want
	}
	writeJSON(w, http.StatusOK, DiffList{Name: c.Name, Repo: want, Files: files})
}

// scopeFiles lists the files a scope covers. Only the range scopes merge
// committed and uncommitted work — the others are one side of git by
// definition, and their origin marker says which.
func scopeFiles(root string, spec scopeSpec, ignore bool) ([]DiffFile, error) {
	switch spec.kind {
	case "range":
		files, err := changedFiles(root, spec.from, ignore)
		if err == nil && spec.ref != "" {
			markLanded(root, spec.ref, files)
		}
		return files, err
	case "staged":
		return nameStatus(root, "staged", ignore, "diff", "--cached", "--name-status")
	case "unstaged":
		files, err := nameStatus(root, "working", ignore, "diff", "--name-status")
		if err != nil {
			return nil, err
		}
		// untracked files are unstaged too — git just leaves them out of a diff
		out, err := git(root, "ls-files", "--others", "--exclude-standard")
		if err == nil && out != "" {
			for _, path := range strings.Split(out, "\n") {
				if path == "" {
					continue
				}
				files = append(files, DiffFile{
					Path: path, Status: "new", Origin: "working",
					Untracked: true, Preview: previewKind(path),
				})
			}
		}
		slices.SortFunc(files, func(a, b DiffFile) int { return strings.Compare(a.Path, b.Path) })
		return files, nil
	case "commit":
		return nameStatus(root, "branch", ignore, "show", "--format=", "--name-status", spec.sha)
	}
	return nil, fmt.Errorf("unknown scope kind %q", spec.kind)
}

// nameStatus lists files from any git command that prints --name-status, with
// the stats of the matching --numstat run.
func nameStatus(root, origin string, ignore bool, args ...string) ([]DiffFile, error) {
	out, err := git(root, append(args, ignoreArgs(ignore)...)...)
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	numArgs := make([]string, len(args))
	copy(numArgs, args)
	for i, a := range numArgs {
		if a == "--name-status" {
			numArgs[i] = "--numstat"
		}
	}
	stats := numstatArgs(root, append(numArgs, ignoreArgs(ignore)...)...)

	var files []DiffFile
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		path := f[len(f)-1] // "R100\told\tnew" -> the new name
		file := DiffFile{
			Path: path, Status: statusWord(f[0][:1]),
			Origin: origin, Preview: previewKind(path),
		}
		if s, ok := stats[path]; ok {
			file.Added, file.Deleted = s[0], s[1]
		} else if ignore {
			// --name-status ignores -I, --numstat honours it: a file missing
			// from the stats has nothing left once comments are dropped
			continue
		}
		files = append(files, file)
	}
	return files, nil
}

// repoBase is the branch a repo's commits are measured against. The checkout's
// own repo uses the dashboard's base (the main checkout's branch — never a
// hardcoded name); a submodule is a repository with a history of its own, so
// it uses its remote's default branch.
func repoBase(root, name, dashboardBase string, primary string) string {
	if name == primary {
		return dashboardBase
	}
	if b, err := git(root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && b != "" {
		return b
	}
	return ""
}

// mergeBaseOf is the commit this checkout forked from — the point everything is
// measured against. It is HEAD itself when there is nothing to fork from: a
// detached checkout, one sitting ON the base branch, or a base that does not
// resolve here. The uncommitted work is then the whole diff, with nothing
// special-cased anywhere else.
func mergeBaseOf(root, base string) string {
	if base == "" {
		return "HEAD"
	}
	mb, err := git(root, "merge-base", base, "HEAD")
	if err != nil || mb == "" {
		return "HEAD"
	}
	if head, err := git(root, "rev-parse", "HEAD"); err == nil && head == mb {
		return "HEAD"
	}
	return mb
}

// changedFiles lists everything the checkout holds that forkPoint does not:
// the files its commits touched plus the uncommitted ones, merged into one
// list, each marked with where its change lives. -uall expands untracked
// directories to their files, so a fresh directory is a list of files in the
// tree instead of one unopenable entry.
func changedFiles(root, forkPoint string, ignore bool) ([]DiffFile, error) {
	committed := map[string]string{} // path -> status letter
	if forkPoint != "HEAD" {
		out, err := git(root, append([]string{"diff", "--name-status", forkPoint + "..HEAD"}, ignoreArgs(ignore)...)...)
		if err != nil {
			return nil, fmt.Errorf("git diff --name-status: %w", err)
		}
		for _, line := range strings.Split(out, "\n") {
			f := strings.Split(line, "\t")
			if len(f) < 2 {
				continue
			}
			committed[f[len(f)-1]] = f[0][:1] // "R100\told\tnew" -> the new name
		}
	}

	out, err := git(root, "status", "--porcelain", "-uall")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	working := map[string]string{}
	var order []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		code, path := line[:2], line[3:]
		// a rename reads "R  old -> new"; the new name is the one to diff
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		path = strings.Trim(path, `"`)
		working[path] = code
		order = append(order, path)
	}
	for path := range committed {
		if _, dup := working[path]; !dup {
			order = append(order, path)
		}
	}
	slices.Sort(order)

	// one numstat over the whole span covers every case: a committed-only file
	// gets its branch counts, an uncommitted-only file its working counts, and
	// a file that is both gets the sum a reviewer actually cares about
	stats := numstat(root, forkPoint, ignore)
	files := make([]DiffFile, 0, len(order))
	for _, path := range order {
		code, dirty := working[path]
		status, onBranch := committed[path]
		f := DiffFile{Path: path, Untracked: code == "??", Preview: previewKind(path)}
		switch {
		case onBranch && dirty:
			f.Origin, f.Status = "both", statusWord(code)
		case onBranch:
			f.Origin, f.Status = "branch", statusWord(status)
		default:
			f.Origin, f.Status = "working", statusWord(code)
		}
		if s, ok := stats[path]; ok {
			f.Added, f.Deleted = s[0], s[1]
		} else if ignore && !f.Untracked {
			// nothing of this file survives once comments are dropped (an
			// untracked file is in no diff at all, so it is never in stats)
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

// markLanded flags the committed files whose change ref already holds. The
// range is measured from the fork point, so a file the base branch picked up
// through another commit — a change lifted into a review commit, say — stays
// in it, and `git cherry` keeps the commit "not in base" because it compares
// whole commits. So the content is asked instead: git merge-tree merges HEAD
// onto ref in memory (no worktree, no index) and the files that merge would
// still change on ref are the ones ref lacks. A committed file it would not
// change is already there. Uncommitted work is never on ref and stays
// unmarked, as does everything when the merge cannot be formed at all.
func markLanded(root, ref string, files []DiffFile) {
	cmd := exec.Command("git", "merge-tree", "--write-tree", ref, "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// exit 1 is a merge with conflicts: the tree is still written, and a
		// conflicted file still differs from ref, which reads as not there
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
			return
		}
	}
	tree, _, _ := strings.Cut(string(out), "\n")
	if !plainSha(tree) {
		return
	}
	still, err := git(root, "diff", "--name-only", ref, tree)
	if err != nil {
		return
	}
	differs := map[string]bool{}
	for _, p := range strings.Split(still, "\n") {
		differs[p] = true
	}
	for i := range files {
		if files[i].Origin == "branch" && !differs[files[i].Path] {
			files[i].Merged = true
		}
	}
}

// numstat maps path -> {added, deleted} from a ref to the working tree.
func numstat(root, from string, ignore bool) map[string][2]int {
	return numstatArgs(root, append([]string{"diff", from, "--numstat"}, ignoreArgs(ignore)...)...)
}

// numstatArgs is numstat over any git invocation that prints --numstat lines,
// so staged/unstaged/commit scopes share the parsing.
func numstatArgs(root string, args ...string) map[string][2]int {
	stats := map[string][2]int{}
	out, err := git(root, args...)
	if err != nil {
		return stats
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0]) // "-" for binary files -> 0
		d, _ := strconv.Atoi(f[1])
		path := f[2]
		if i := strings.Index(path, " => "); i >= 0 { // rename
			path = strings.TrimSuffix(path[i+4:], "}")
		}
		stats[path] = [2]int{a, d}
	}
	return stats
}

func statusWord(code string) string {
	switch {
	case code == "??":
		return "new"
	case strings.ContainsAny(code, "R"):
		return "renamed"
	case strings.ContainsAny(code, "A"):
		return "added"
	case strings.ContainsAny(code, "D"):
		return "deleted"
	case strings.ContainsAny(code, "M"):
		return "modified"
	default:
		return strings.TrimSpace(code)
	}
}

// fileDiff renders one file from the fork point to the working tree, so a file
// that is partly committed and partly not shows as one diff. An untracked file
// is in no commit and no index, so it is compared to /dev/null — which makes
// git print it as all additions, exactly how a new file should read.
func fileDiff(root, path string, untracked bool, spec scopeSpec, ignore bool) (string, bool, error) {
	var args []string
	switch {
	case untracked:
		// in no commit and no index, so there is nothing to diff against
		args = []string{"diff", "--no-index", "--", "/dev/null", path}
	case spec.kind == "staged":
		args = []string{"diff", "--cached", "--", path}
	case spec.kind == "unstaged":
		args = []string{"diff", "--", path}
	case spec.kind == "commit":
		args = []string{"show", "--format=", spec.sha, "--", path}
	default:
		args = []string{"diff", spec.from, "--", path}
	}
	if !untracked {
		args = append(args[:len(args)-2], append(ignoreArgs(ignore), args[len(args)-2:]...)...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	// --no-index exits 1 whenever there IS a difference, which is the normal
	// case here; only a missing stdout is a real failure
	if err != nil && len(out) == 0 {
		return "", false, fmt.Errorf("git diff: %w", err)
	}
	if len(out) > diffMax {
		return string(out[:diffMax]), true, nil
	}
	return string(out), false, nil
}

// safeRepoPath keeps the file parameter inside the checkout: relative, no
// parent-directory hops, and not an option git could interpret.
func safeRepoPath(p string) bool {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "-") {
		return false
	}
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Clean(p)), "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}
