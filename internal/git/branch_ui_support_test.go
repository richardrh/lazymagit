package git

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAddWorktreeWithNewBranchValidatesNameRevisionDestinationAndDoesNotForce(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.AddWorktreeWithNewBranch(ctx, filepath.Join(t.TempDir(), "bad"), "--bad", "HEAD"); err == nil {
		t.Fatal("option-like branch was accepted")
	}
	if err := repo.AddWorktreeWithNewBranch(ctx, filepath.Join(t.TempDir(), "missing"), "missing-start", "does-not-exist"); err == nil {
		t.Fatal("missing start point was accepted")
	}
	destination := filepath.Join(t.TempDir(), "linked")
	if err := repo.AddWorktreeWithNewBranch(ctx, destination, "linked-topic", "HEAD"); err != nil {
		t.Fatalf("AddWorktreeWithNewBranch: %v", err)
	}
	if got := r.git("-C", destination, "branch", "--show-current"); got != "linked-topic" {
		t.Fatalf("new worktree branch = %q", got)
	}
	if err := repo.AddWorktreeWithNewBranch(ctx, destination, "other-topic", "HEAD"); err == nil {
		t.Fatal("existing destination was implicitly forced")
	}
}
