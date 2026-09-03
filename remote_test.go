package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The case the whole submodule check exists for: the parent records a commit
// for a submodule that the submodule's own remote has never seen. Push the
// parent and a colleague's clone breaks — git fetches the parent, asks the
// submodule's remote for that id, and nobody has it.
func TestParentIsBlockedByAnUnknownSubmoduleCommit(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	// two bare repositories standing in for origin
	subRemote := filepath.Join(dir, "sub.git")
	parentRemote := filepath.Join(dir, "parent.git")
	for _, r := range []string{subRemote, parentRemote} {
		gitRun(t, dir, "init", "-q", "--bare", "-b", "main", r)
	}

	// a submodule with one pushed commit
	sub := initRepo(t, filepath.Join(dir, "sub"))
	gitRun(t, sub, "remote", "add", "origin", subRemote)
	gitRun(t, sub, "push", "-q", "-u", "origin", "main")

	// a parent that carries it
	parent := initRepo(t, filepath.Join(dir, "parent"))
	gitRun(t, parent, "remote", "add", "origin", parentRemote)
	gitRun(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", subRemote, "vendor/sub")
	gitRun(t, parent, "commit", "-q", "-m", "add the submodule")
	gitRun(t, parent, "push", "-q", "-u", "origin", "main")

	// `submodule add` clones the submodule afresh under the parent, and that
	// clone has none of the identity initRepo set on the original — a machine
	// without a global one (every CI runner) cannot commit in it
	subIn := filepath.Join(parent, "vendor", "sub")
	identify(t, subIn)
	c := Checkout{Name: "parent", Path: normPath(parent), Repo: "parent"}

	// Nothing unusual yet. The parent has nothing to push, which is its own
	// honest refusal — what matters is that it is not refused over a submodule.
	if rr := byName(remoteRepos(c), "parent"); strings.Contains(rr.Blocked, "remote") {
		t.Fatalf("a clean parent was blocked over its submodule: %s", rr.Blocked)
	}

	// a commit in the submodule that its remote knows nothing about, and the
	// parent bumped to point at it
	write(t, subIn, "new.txt", "local only\n")
	gitRun(t, subIn, "add", "new.txt")
	gitRun(t, subIn, "commit", "-q", "-m", "not pushed anywhere")
	gitRun(t, parent, "add", "vendor/sub")
	gitRun(t, parent, "commit", "-q", "-m", "bump the submodule")

	repos := remoteRepos(c)
	p, s := byName(repos, "parent"), byName(repos, "sub")
	if !s.GitlinkUnknown {
		t.Errorf("the submodule's recorded commit is on no remote and was not noticed: %+v", s)
	}
	if p.CanPush {
		t.Error("the parent was allowed to publish a pointer nobody can follow")
	}
	if p.Blocked == "" {
		t.Error("the parent was blocked without saying why")
	}
	// and the submodule itself is free to push — that is the cure, not the crime
	if !s.CanPush {
		t.Errorf("the submodule was blocked from the very push that fixes this: %s", s.Blocked)
	}

	// push the submodule, and the parent is free
	gitRun(t, subIn, "push", "-q", "origin", "HEAD:main")
	gitRun(t, subIn, "fetch", "-q", "origin")
	if p := byName(remoteRepos(c), "parent"); !p.CanPush {
		t.Errorf("the parent is still blocked after the submodule was pushed: %s", p.Blocked)
	}
}

// The other refusals, which are about the checkout itself.
func TestPushableRefusals(t *testing.T) {
	for _, c := range []struct {
		name string
		rr   RemoteRepo
		want string
	}{
		{"detached", RemoteRepo{Detached: true}, "a detached HEAD is a commit, not a branch: nothing to push"},
		{"behind", RemoteRepo{Branch: "b", Upstream: "origin/b", Behind: 3, Ahead: 1},
			"origin/b is 3 ahead: rebase or merge first"},
		{"nothing to push", RemoteRepo{Branch: "b", Upstream: "origin/b"}, "nothing to push"},
		{"no remote at all", RemoteRepo{Branch: "b"}, "no remote to push to"},
	} {
		ok, why := pushable(c.rr)
		if ok || why != c.want {
			t.Errorf("%s: pushable = (%v, %q), want (false, %q)", c.name, ok, why, c.want)
		}
	}
	// ahead of an upstream that has not moved is the one case that goes
	if ok, why := pushable(RemoteRepo{Branch: "b", Upstream: "origin/b", Ahead: 2}); !ok {
		t.Errorf("a branch two ahead was refused: %s", why)
	}
	// a first push, with a remote but no upstream yet
	if ok, why := pushable(RemoteRepo{Branch: "b", Remote: "origin"}); !ok {
		t.Errorf("a branch with no upstream was refused: %s", why)
	}
}

// identify gives a repository an author, which a clone does not inherit and a
// machine with no global git config cannot supply.
//
// Turning core.autocrlf off has to be followed by re-checking-out the tracked
// files. A clone on Windows has already written them with CRLF under the
// global setting, and flipping the setting afterwards leaves every one of them
// looking modified against the LF in the index — which is a dirty tree the
// fixture never asked for, and it failed a `git checkout` two steps later.
func identify(t testing.TB, path string) {
	t.Helper()
	gitRun(t, path, "config", "core.autocrlf", "false")
	gitRun(t, path, "config", "user.email", "grove@example.com")
	gitRun(t, path, "config", "user.name", "grove")
	gitRun(t, path, "config", "commit.gpgsign", "false")
	if _, err := git(path, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		gitRun(t, path, "checkout", "--", ".")
	}
}

func byName(repos []RemoteRepo, name string) RemoteRepo {
	for _, r := range repos {
		if r.Name == name {
			return r
		}
	}
	return RemoteRepo{}
}

// The paths that actually run git and change something: push when it is
// allowed, and the two ways out when the remote has moved.
func TestPushAndRebaseAgainstARealRemote(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	remote := filepath.Join(dir, "origin.git")
	gitRun(t, dir, "init", "-q", "--bare", "-b", "main", remote)

	work := initRepo(t, filepath.Join(dir, "work"))
	gitRun(t, work, "remote", "add", "origin", remote)
	gitRun(t, work, "push", "-q", "-u", "origin", "main")
	self := subRepo{Name: "work", Path: work}

	// a commit of our own: push should take it
	write(t, work, "mine.txt", "mine\n")
	gitRun(t, work, "add", "mine.txt")
	gitRun(t, work, "commit", "-q", "-m", "mine")

	rr := readRemote(work)
	rr.CanPush, rr.Blocked = pushable(rr)
	if res := doRemote(self, rr, "push", "", nil); !res.Ok {
		t.Fatalf("push refused: %s", res.Detail)
	}
	if out, _ := git(remote, "log", "--oneline", "-1", "main"); !strings.Contains(out, "mine") {
		t.Errorf("the remote did not get the commit: %q", out)
	}

	// somebody else pushes, so we are behind and must not push over them
	other := filepath.Join(dir, "other")
	gitRun(t, dir, "clone", "-q", remote, other)
	identify(t, other)
	write(t, other, "theirs.txt", "theirs\n")
	gitRun(t, other, "add", "theirs.txt")
	gitRun(t, other, "commit", "-q", "-m", "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")

	write(t, work, "mine2.txt", "mine again\n")
	gitRun(t, work, "add", "mine2.txt")
	gitRun(t, work, "commit", "-q", "-m", "mine again")
	gitRun(t, work, "fetch", "-q", "origin")

	rr = readRemote(work)
	rr.CanPush, rr.Blocked = pushable(rr)
	if rr.CanPush {
		t.Error("push was offered while the remote was ahead")
	}
	if !strings.Contains(rr.Blocked, "rebase or merge first") {
		t.Errorf("blocked for the wrong reason: %q", rr.Blocked)
	}

	// rebase takes us onto it, and then the push goes
	if res := doRemote(self, rr, "rebase", "", nil); !res.Ok {
		t.Fatalf("rebase failed: %s", res.Detail)
	}
	rr = readRemote(work)
	rr.CanPush, rr.Blocked = pushable(rr)
	if !rr.CanPush {
		t.Fatalf("still cannot push after rebasing: %s", rr.Blocked)
	}
	if res := doRemote(self, rr, "push", "", nil); !res.Ok {
		t.Fatalf("push after rebase refused: %s", res.Detail)
	}
	if out, _ := git(remote, "log", "--oneline", "main"); !strings.Contains(out, "mine again") || !strings.Contains(out, "theirs") {
		t.Errorf("the remote lost somebody's work: %q", out)
	}
}

// A dirty tree is not a refusal. The case only arises when there is something
// to pull at all, since otherwise "nothing to do" is the honest answer
// whatever the tree looks like.
func TestADirtyTreeCanStillPull(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	remote := filepath.Join(dir, "origin.git")
	gitRun(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	work := initRepo(t, filepath.Join(dir, "work"))
	gitRun(t, work, "remote", "add", "origin", remote)
	gitRun(t, work, "push", "-q", "-u", "origin", "main")

	// somebody else moves the branch on, so we are genuinely behind
	other := filepath.Join(dir, "other")
	gitRun(t, dir, "clone", "-q", remote, other)
	identify(t, other)
	write(t, other, "theirs.txt", "theirs\n")
	gitRun(t, other, "add", "theirs.txt")
	gitRun(t, other, "commit", "-q", "-m", "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitRun(t, work, "fetch", "-q", "origin")

	write(t, work, "a.txt", "changed but not committed\n")

	rr := readRemote(work)
	// A dirty tree does not stop a pull, and no longer stops a rebase either:
	// the changes are set aside and put back.
	ok, _, _ := pullable(rr)
	if !ok {
		t.Error("a dirty tree was refused a pull it could have had")
	}
	// the fast-forward it can have goes through, edit and all
	if res := doRemote(subRepo{Name: "work", Path: work}, rr, "ff", "", nil); !res.Ok {
		t.Errorf("a fast-forward past an unrelated edit was refused: %s", res.Detail)
	}
	if out, _ := git(work, "status", "--porcelain"); !strings.Contains(out, "a.txt") {
		t.Errorf("the uncommitted edit did not survive the pull: %q", out)
	}
}

// A rebase over uncommitted work: set aside, rebase, put back — and the
// changes are still there afterwards, on top of the new history.
func TestRebaseCarriesUncommittedWorkOver(t *testing.T) {
	requireGit(t)
	work, _ := twoWaysApart(t, "keep.txt", "theirs\n")
	write(t, work, "wip.txt", "work in progress\n") // untracked, and in nobody's way

	rr := readRemote(work)
	res := doRemote(subRepo{Name: "work", Path: work}, rr, "rebase", "", nil)
	if !res.Ok {
		t.Fatalf("the rebase was refused: %s", res.Detail)
	}
	if got, _ := os.ReadFile(filepath.Join(work, "wip.txt")); string(got) != "work in progress\n" {
		t.Errorf("the uncommitted file did not come back: %q", got)
	}
	if _, err := os.Stat(filepath.Join(work, "keep.txt")); err != nil {
		t.Error("their commit is not here, so the rebase did not happen")
	}
	if out, _ := git(work, "stash", "list"); out != "" {
		t.Errorf("a stash was left behind: %q", out)
	}
	if out, _ := git(work, "rev-list", "--count", "--merges", "HEAD"); out != "0" {
		t.Errorf("a rebase made %s merge commits", out)
	}
}

// The case worth having: the stashed work collides with what is coming in.
// The rebase must be undone rather than left standing beside a stash nobody
// asked for, and the working tree must be exactly where it started.
func TestARebaseThatCannotBePutBackUndoesItself(t *testing.T) {
	requireGit(t)
	// they change a file, and so do we, without committing
	work, _ := twoWaysApart(t, "shared.txt", "theirs\n")
	write(t, work, "shared.txt", "mine, uncommitted\n")

	before, _ := git(work, "rev-parse", "HEAD")
	rr := readRemote(work)
	res := doRemote(subRepo{Name: "work", Path: work}, rr, "rebase", "", nil)
	if res.Ok {
		t.Fatal("a rebase that could not be put back reported success")
	}
	if !strings.Contains(res.Detail, "undone") {
		t.Errorf("the message does not say it was undone: %q", res.Detail)
	}

	after, _ := git(work, "rev-parse", "HEAD")
	if after != before {
		t.Errorf("HEAD moved: %s -> %s", before, after)
	}
	got, err := os.ReadFile(filepath.Join(work, "shared.txt"))
	if err != nil || string(got) != "mine, uncommitted\n" {
		t.Errorf("the uncommitted edit is not where it was: %q (%v)", got, err)
	}
	if out, _ := git(work, "status", "--porcelain"); !strings.Contains(out, "shared.txt") {
		t.Errorf("the tree is not dirty any more, so the edit was lost: %q", out)
	}
	// no half-finished rebase left lying about
	if out, _ := git(work, "rev-parse", "--git-path", "rebase-merge"); out != "" {
		if _, err := os.Stat(out); err == nil {
			t.Error("a rebase is still in progress")
		}
	}
}

// twoWaysApart returns a clean checkout that is one commit behind its upstream
// and one ahead of it, with the remote's commit touching the named file.
func twoWaysApart(t testing.TB, name, theirs string) (work, remote string) {
	t.Helper()
	dir := t.TempDir()
	remote = filepath.Join(dir, "origin.git")
	gitRun(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	work = initRepo(t, filepath.Join(dir, "work"))
	gitRun(t, work, "remote", "add", "origin", remote)
	gitRun(t, work, "push", "-q", "-u", "origin", "main")

	other := filepath.Join(dir, "other")
	gitRun(t, dir, "clone", "-q", remote, other)
	identify(t, other)
	write(t, other, name, theirs)
	gitRun(t, other, "add", name)
	gitRun(t, other, "commit", "-q", "-m", "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")

	write(t, work, "ours.txt", "ours\n")
	gitRun(t, work, "add", "ours.txt")
	gitRun(t, work, "commit", "-q", "-m", "ours")
	gitRun(t, work, "fetch", "-q", "origin")
	return work, remote
}

// Which way in grove suggests, and why.
func TestPullModeFitsWhereTheBranchStands(t *testing.T) {
	for _, c := range []struct {
		name string
		rr   RemoteRepo
		ok   bool
		mode string
	}{
		{"nothing to pull", RemoteRepo{Branch: "b", Upstream: "origin/b"}, false, ""},
		{"only theirs — a fast-forward",
			RemoteRepo{Branch: "b", Upstream: "origin/b", Behind: 3}, true, "ff"},
		{"ours on top — keep it linear",
			RemoteRepo{Branch: "b", Upstream: "origin/b", Behind: 3, Ahead: 2}, true, "rebase"},
		{"dirty, ours on top — still rebase: the changes are stashed and put back",
			RemoteRepo{Branch: "b", Upstream: "origin/b", Behind: 1, Ahead: 1, Dirty: 4}, true, "rebase"},
		{"dirty, nothing of ours — a fast-forward is still fine",
			RemoteRepo{Branch: "b", Upstream: "origin/b", Behind: 1, Dirty: 4}, true, "ff"},
		{"detached", RemoteRepo{Detached: true, Behind: 1}, false, ""},
		{"no upstream", RemoteRepo{Branch: "b", Remote: "origin", Behind: 1}, false, ""},
	} {
		ok, _, mode := pullable(c.rr)
		if ok != c.ok || mode != c.mode {
			t.Errorf("%s: pullable = (%v, %q), want (%v, %q)", c.name, ok, mode, c.ok, c.mode)
		}
	}
}

// Submodules nest, and the flat list grove shows does not. easydb-library is
// declared by easydb-webfrontend, which is declared by fylr — and only
// easydb-webfrontend records a commit for it. Checking every submodule against
// the CHECKOUT asked the wrong repository for a path it does not have, got
// nothing back, and quietly checked nothing at all.
func TestANestedSubmoduleStopsItsOwnParent(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	remotes := map[string]string{}
	for _, n := range []string{"leaf", "mid", "top"} {
		remotes[n] = filepath.Join(dir, n+".git")
		gitRun(t, dir, "init", "-q", "--bare", "-b", "main", remotes[n])
	}
	build := func(name string) string {
		p := initRepo(t, filepath.Join(dir, name))
		gitRun(t, p, "remote", "add", "origin", remotes[name])
		gitRun(t, p, "push", "-q", "-u", "origin", "main")
		return p
	}
	leaf, mid, top := build("leaf"), build("mid"), build("top")
	add := func(parent, url, at string) {
		gitRun(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", url, at)
		gitRun(t, parent, "commit", "-q", "-m", "add "+at)
		identify(t, filepath.Join(parent, at))
	}
	add(mid, remotes["leaf"], "leaf")
	gitRun(t, mid, "push", "-q", "origin", "main")
	add(top, remotes["mid"], "mid")
	gitRun(t, top, "push", "-q", "origin", "main")
	// `submodule add` does not recurse, so mid's own submodule is a bare
	// directory inside top until it is asked for
	gitRun(t, filepath.Join(top, "mid"), "-c", "protocol.file.allow=always",
		"submodule", "update", "--init", "-q")
	identify(t, filepath.Join(top, "mid", "leaf"))

	c := Checkout{Name: "top", Path: normPath(top), Repo: "top"}
	if got := len(remoteRepos(c)); got != 3 {
		t.Fatalf("found %d repositories, want top, mid and leaf", got)
	}

	// a commit in the leaf that its remote does not have, recorded by MID
	leafIn := filepath.Join(top, "mid", "leaf")
	write(t, leafIn, "x.txt", "local only\n")
	gitRun(t, leafIn, "add", "x.txt")
	gitRun(t, leafIn, "commit", "-q", "-m", "unpushed")
	midIn := filepath.Join(top, "mid")
	gitRun(t, midIn, "add", "leaf")
	gitRun(t, midIn, "commit", "-q", "-m", "bump leaf")

	repos := remoteRepos(c)
	if !byName(repos, "leaf").GitlinkUnknown {
		t.Error("the leaf's unpushed commit was not noticed")
	}
	if byName(repos, "mid").CanPush {
		t.Error("mid was allowed to publish a pointer to a commit leaf's remote lacks")
	}
	// and the checkout is untouched by it: top records mid, not leaf, and what
	// it records for mid is still on mid's remote
	if got := byName(repos, "top"); !got.CanPush && strings.Contains(got.Blocked, "leaf") {
		t.Errorf("top was blamed for the leaf: %s", got.Blocked)
	}
	_ = leaf
}

// A submodule sitting at a different commit than the parent records leaves the
// parent modified, and `git stash` cannot clean that: it stores the recorded
// pointer and leaves the submodule's own checkout where it is. This used to
// stop the rebase outright. It does not any more — the submodule is moved to
// what the parent records, the rebase runs, and it is put back — and this
// walks the whole of that.
func TestARebaseMovesASubmoduleOutOfTheWayAndBack(t *testing.T) {
	top, subIn, first := parentWithMovedSubmodule(t)
	before, _ := git(top, "rev-parse", "HEAD")

	behindBefore, _ := git(top, "rev-list", "--count", "HEAD..@{upstream}")
	res := doRemote(subRepo{Name: "top", Path: top}, readRemote(top), "rebase", "", nil)
	if !res.Ok {
		t.Fatalf("the rebase was refused over a submodule that could simply be moved: %s\n%s\n%s",
			res.Detail, res.Why, res.Git)
	}

	// the parent moved on
	if after, _ := git(top, "rev-parse", "HEAD"); after == before {
		t.Error("HEAD did not move, so the rebase did not happen")
	}
	if out, _ := git(top, "rev-list", "--count", "HEAD..@{upstream}"); out != "0" {
		t.Errorf("still %s behind after rebasing (was %s)", out, behindBefore)
	}
	// and the submodule is back exactly where it was, which is what made the
	// parent look modified in the first place
	if at, _ := git(subIn, "rev-parse", "HEAD"); at != first {
		t.Errorf("the submodule was left at %s, not the %s it was on", at, first)
	}
	if out, _ := git(top, "status", "--porcelain"); !strings.Contains(out, "vendor/sub") {
		t.Errorf("the submodule was quietly bumped instead of put back: %q", out)
	}
	if out, _ := git(top, "stash", "list"); out != "" {
		t.Errorf("a stash was left behind: %q", out)
	}
}

// A submodule with uncommitted work inside it cannot be moved out of the way,
// and grove must not overwrite it to get a rebase through. That one is
// refused, by name, with the tree exactly as it was.
func TestASubmoduleWithItsOwnWorkIsNotMovedForARebase(t *testing.T) {
	requireGit(t)
	top, subIn, _ := parentWithMovedSubmodule(t)
	// An edit git cannot carry across the move: two.txt exists at the commit
	// the parent records and not at the one the submodule is on, so checking
	// it out would have to delete a file with uncommitted changes in it.
	gitRun(t, subIn, "checkout", "--quiet", "main")
	write(t, subIn, "two.txt", "edited, not committed\n")

	before, _ := git(top, "rev-parse", "HEAD")
	res := doRemote(subRepo{Name: "top", Path: top}, readRemote(top), "rebase", "", nil)
	if res.Ok {
		t.Fatal("a submodule holding uncommitted work was moved anyway")
	}
	if !strings.Contains(res.Why, "uncommitted work inside") {
		t.Errorf("the explanation does not say what is in the way: %q", res.Why)
	}
	if after, _ := git(top, "rev-parse", "HEAD"); after != before {
		t.Errorf("HEAD moved: %s -> %s", before, after)
	}
	if got, err := os.ReadFile(filepath.Join(subIn, "two.txt")); err != nil || string(got) != "edited, not committed\n" {
		t.Errorf("the submodule's own work is gone: %q (%v)", got, err)
	}
}

// A failure must carry what git said. Output() keeps stderr on the ExitError
// and throws it away everywhere else, which is how a rebase that stopped for a
// nameable reason arrived on screen as "exit status 1".
func TestGitFailuresCarryGitsWords(t *testing.T) {
	requireGit(t)
	repo := initRepo(t, filepath.Join(t.TempDir(), "r"))
	_, err := git(repo, "rev-parse", "--verify", "refs/heads/nope")
	if err == nil {
		t.Fatal("expected a failure")
	}
	if strings.Contains(err.Error(), "exit status") {
		t.Errorf("the error is a status code and nothing else: %q", err)
	}
	out, err := gitSays(repo, "checkout", "no-such-branch")
	if err == nil || out == "" {
		t.Errorf("gitSays lost what git said: %q (%v)", out, err)
	}
}

// parentWithMovedSubmodule is a checkout one commit behind its upstream whose
// submodule sits at an earlier commit than the parent records — the state that
// makes `git status` say ` M vendor/sub` and a plain stash useless.
func parentWithMovedSubmodule(t testing.TB) (top, subIn, first string) {
	requireGit(t)
	dir := t.TempDir()
	subRemote := filepath.Join(dir, "sub.git")
	topRemote := filepath.Join(dir, "top.git")
	for _, r := range []string{subRemote, topRemote} {
		gitRun(t, dir, "init", "-q", "--bare", "-b", "main", r)
	}
	sub := initRepo(t, filepath.Join(dir, "sub"))
	gitRun(t, sub, "remote", "add", "origin", subRemote)
	gitRun(t, sub, "push", "-q", "-u", "origin", "main")
	// the commit the submodule will be moved back to, named outright: HEAD~1
	// is one more thing that can resolve differently somewhere else
	var err error
	first, err = git(sub, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	write(t, sub, "two.txt", "two\n")
	gitRun(t, sub, "add", "two.txt")
	gitRun(t, sub, "commit", "-q", "-m", "second")
	gitRun(t, sub, "push", "-q", "origin", "main")

	top = initRepo(t, filepath.Join(dir, "top"))
	gitRun(t, top, "remote", "add", "origin", topRemote)
	gitRun(t, top, "-c", "protocol.file.allow=always", "submodule", "add", "-q", subRemote, "vendor/sub")
	gitRun(t, top, "commit", "-q", "-m", "add submodule")
	gitRun(t, top, "push", "-q", "-u", "origin", "main")
	subIn = filepath.Join(top, "vendor", "sub")
	identify(t, subIn)

	// somebody moves the parent's branch on, so there is something to rebase onto
	other := filepath.Join(dir, "other")
	gitRun(t, dir, "clone", "-q", topRemote, other)
	identify(t, other)
	write(t, other, "theirs.txt", "theirs\n")
	gitRun(t, other, "add", "theirs.txt")
	gitRun(t, other, "commit", "-q", "-m", "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitRun(t, top, "fetch", "-q", "origin")

	// and the submodule is moved back one, without committing the parent
	if out, err := gitSays(subIn, "log", "--oneline"); err != nil || !strings.Contains(out, "second") {
		t.Fatalf("the submodule clone is not where the fixture expects: %q (%v)", out, err)
	}
	if out, err := gitSays(subIn, "checkout", "-q", first); err != nil {
		t.Fatalf("could not move the submodule to %s: %v\n%s", first, err, out)
	}

	if out, _ := git(top, "status", "--porcelain"); !strings.Contains(out, "vendor/sub") {
		t.Fatalf("the fixture is not in the state under test: %q", out)
	}

	return top, subIn, first
}

// Submodules nest, and so does being out of position. easydb-library and
// coffeescript-ui are declared by easydb-webfrontend, which is declared by
// fylr, and moving easydb-webfrontend to what fylr records changes what
// coffeescript-ui is supposed to be — so the outer one has to move first.
// Parking only the top level left the tree dirty and the rebase refused.
func TestParkingReachesNestedSubmodules(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	remotes := map[string]string{}
	for _, n := range []string{"leaf", "mid", "top"} {
		remotes[n] = filepath.Join(dir, n+".git")
		gitRun(t, dir, "init", "-q", "--bare", "-b", "main", remotes[n])
	}
	build := func(name string) string {
		p := initRepo(t, filepath.Join(dir, name))
		gitRun(t, p, "remote", "add", "origin", remotes[name])
		gitRun(t, p, "push", "-q", "-u", "origin", "main")
		return p
	}
	leaf, mid, top := build("leaf"), build("mid"), build("top")
	// a second commit in the leaf, so it has somewhere else to be
	write(t, leaf, "two.txt", "two\n")
	gitRun(t, leaf, "add", "two.txt")
	gitRun(t, leaf, "commit", "-q", "-m", "leaf second")
	gitRun(t, leaf, "push", "-q", "origin", "main")

	gitRun(t, mid, "-c", "protocol.file.allow=always", "submodule", "add", "-q", remotes["leaf"], "leaf")
	gitRun(t, mid, "commit", "-q", "-m", "add leaf")
	gitRun(t, mid, "push", "-q", "origin", "main")
	gitRun(t, top, "-c", "protocol.file.allow=always", "submodule", "add", "-q", remotes["mid"], "mid")
	gitRun(t, top, "commit", "-q", "-m", "add mid")
	gitRun(t, top, "push", "-q", "-u", "origin", "main")
	gitRun(t, filepath.Join(top, "mid"), "-c", "protocol.file.allow=always",
		"submodule", "update", "--init", "-q")

	midIn := filepath.Join(top, "mid")
	leafIn := filepath.Join(midIn, "leaf")
	identify(t, midIn)
	identify(t, leafIn)

	// somebody moves top on, so there is something to rebase onto
	other := filepath.Join(dir, "other")
	gitRun(t, dir, "clone", "-q", remotes["top"], other)
	identify(t, other)
	write(t, other, "theirs.txt", "theirs\n")
	gitRun(t, other, "add", "theirs.txt")
	gitRun(t, other, "commit", "-q", "-m", "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitRun(t, top, "fetch", "-q", "origin")

	// and the NESTED submodule is moved, which makes mid dirty, which makes
	// top dirty — two levels down from the repository being rebased
	firstLeaf, err := git(leafIn, "rev-parse", "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, leafIn, "checkout", "--quiet", firstLeaf)
	if out, _ := git(top, "status", "--porcelain"); !strings.Contains(out, "mid") {
		t.Fatalf("the fixture is not in the state under test: %q", out)
	}

	wasMid, _ := git(midIn, "rev-parse", "HEAD")
	res := doRemote(subRepo{Name: "top", Path: top}, readRemote(top), "rebase", "", nil)
	if !res.Ok {
		t.Fatalf("a nested submodule stopped the rebase: %s\n%s\n%s", res.Detail, res.Why, res.Git)
	}
	if out, _ := git(top, "rev-list", "--count", "HEAD..@{upstream}"); out != "0" {
		t.Errorf("still %s behind", out)
	}
	// everything put back, at both depths
	if at, _ := git(leafIn, "rev-parse", "HEAD"); at != firstLeaf {
		t.Errorf("the nested submodule is at %s, not the %s it was on", at, firstLeaf)
	}
	if at, _ := git(midIn, "rev-parse", "HEAD"); at != wasMid {
		t.Errorf("the middle submodule moved: %s -> %s", wasMid, at)
	}
}
