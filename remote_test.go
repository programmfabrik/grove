package main

import (
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

	subIn := filepath.Join(parent, "vendor", "sub")
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
		{"detached", RemoteRepo{Detached: true}, "detached HEAD: no branch to push"},
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
	if res := doRemote(self, rr, "push"); !res.Ok {
		t.Fatalf("push refused: %s", res.Detail)
	}
	if out, _ := git(remote, "log", "--oneline", "-1", "main"); !strings.Contains(out, "mine") {
		t.Errorf("the remote did not get the commit: %q", out)
	}

	// somebody else pushes, so we are behind and must not push over them
	other := filepath.Join(dir, "other")
	gitRun(t, dir, "clone", "-q", remote, other)
	gitRun(t, other, "config", "user.email", "o@example.com")
	gitRun(t, other, "config", "user.name", "other")
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
	if res := doRemote(self, rr, "rebase"); !res.Ok {
		t.Fatalf("rebase failed: %s", res.Detail)
	}
	rr = readRemote(work)
	rr.CanPush, rr.Blocked = pushable(rr)
	if !rr.CanPush {
		t.Fatalf("still cannot push after rebasing: %s", rr.Blocked)
	}
	if res := doRemote(self, rr, "push"); !res.Ok {
		t.Fatalf("push after rebase refused: %s", res.Detail)
	}
	if out, _ := git(remote, "log", "--oneline", "main"); !strings.Contains(out, "mine again") || !strings.Contains(out, "theirs") {
		t.Errorf("the remote lost somebody's work: %q", out)
	}
}

// A dirty tree is not something to rebase over somebody's back.
func TestRebaseRefusesADirtyTree(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	remote := filepath.Join(dir, "origin.git")
	gitRun(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	work := initRepo(t, filepath.Join(dir, "work"))
	gitRun(t, work, "remote", "add", "origin", remote)
	gitRun(t, work, "push", "-q", "-u", "origin", "main")
	write(t, work, "a.txt", "changed but not committed\n")

	rr := readRemote(work)
	res := doRemote(subRepo{Name: "work", Path: work}, rr, "rebase")
	if res.Ok || !strings.Contains(res.Detail, "uncommitted") {
		t.Errorf("a dirty tree was rebased: %+v", res)
	}
}
