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
func identify(t testing.TB, path string) {
	t.Helper()
	gitRun(t, path, "config", "user.email", "grove@example.com")
	gitRun(t, path, "config", "user.name", "grove")
	gitRun(t, path, "config", "commit.gpgsign", "false")
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
	if res := doRemote(self, rr, "push", ""); !res.Ok {
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
	if res := doRemote(self, rr, "rebase", ""); !res.Ok {
		t.Fatalf("rebase failed: %s", res.Detail)
	}
	rr = readRemote(work)
	rr.CanPush, rr.Blocked = pushable(rr)
	if !rr.CanPush {
		t.Fatalf("still cannot push after rebasing: %s", rr.Blocked)
	}
	if res := doRemote(self, rr, "push", ""); !res.Ok {
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
	if res := doRemote(subRepo{Name: "work", Path: work}, rr, "ff", ""); !res.Ok {
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
	res := doRemote(subRepo{Name: "work", Path: work}, rr, "rebase", "")
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
	res := doRemote(subRepo{Name: "work", Path: work}, rr, "rebase", "")
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
