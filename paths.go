package main

import (
	"path/filepath"
	"runtime"
	"strings"
)

// A path reaches grove from two directions and the two do not spell it the
// same way. Twice over:
//
// Separators. git prints forward slashes on every platform, so a worktree on
// Windows comes back as `C:/Users/x/src/repo` while os.Getwd and filepath.Join
// produce `C:\Users\x\src\repo`. The same directory, and `==` says otherwise.
//
// Symlinks. git reports the PHYSICAL path: a repository reached through a link
// comes out of `git worktree list` at its real location, while grove built its
// own path from the directory it was pointed at and still holds the link.
// macOS puts /tmp behind such a link, and a ~/src on a second volume is a
// common enough layout.
//
// Neither is cosmetic. The main checkout is recognised by comparing the
// worktree path git printed against the repo path grove built (discover.go),
// so either mismatch makes a repository look as though it had no main
// checkout: nothing sorted first, IsMain false everywhere, and the base branch
// taken off the wrong tree.
//
// So a path is normalised the moment it enters, whichever side it came from,
// and everything downstream compares normalised paths.

// normPath is the one spelling grove keeps a path in: the OS's separators,
// cleaned, symlinks resolved — the same spelling git uses, which is the point.
//
// A path that will not resolve keeps its cleaned form rather than disappearing:
// it may have been deleted between the scan and here, or sit behind a directory
// this process may not read, and a worktree that is merely gone should still be
// reported by name.
func normPath(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(filepath.FromSlash(p))
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

// samePath compares two paths that reached grove from different sides. Case
// matters on Linux and does not on Windows, where the drive letter alone can be
// spelled either way: `git worktree list` says `C:/src/x` while a shell started
// in `c:\src` makes Go say `c:\src\x`.
//
// macOS is case-insensitive by default too, but not always — an APFS volume can
// be formatted case-sensitive, and there two files really can differ by case.
// Windows is the only platform where the mismatch is grove's own doing, so it
// is the only one where the comparison folds.
func samePath(a, b string) bool {
	a, b = normPath(a), normPath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
