package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// grove is a program that spawns git, and little else: a repo scan is five
// invocations and a state refresh is one plus four per worktree, so a
// directory of twenty repositories with five checkouts each is around five
// hundred processes per cycle. That is free on Unix and is not on Windows,
// where process creation costs an order of magnitude more and every spawn goes
// through the anti-virus filter driver.
//
// These two benchmarks are run on all three platforms in CI so the difference
// is a number rather than a worry. BenchmarkGitSpawn is the floor — what it
// costs to start git at all — and BenchmarkScanCheckout is what grove actually
// does per checkout, four calls including a status.

func BenchmarkGitSpawn(b *testing.B) {
	requireGit(b)
	dir := initRepo(b, filepath.Join(b.TempDir(), "myrepo"))
	b.ResetTimer()
	for b.Loop() {
		cmd := exec.Command(gitExe, "rev-parse", "--git-dir")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanCheckout(b *testing.B) {
	requireGit(b)
	dir := initRepo(b, filepath.Join(b.TempDir(), "myrepo"))
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		if c := scanCheckout(ctx, dir, dir, "main"); c.Branch != "main" {
			b.Fatalf("branch = %q", c.Branch)
		}
	}
}
