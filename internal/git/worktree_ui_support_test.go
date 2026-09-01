package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewedWorktreeAddMoveLockUnlockAndRemove(t *testing.T) {
	_, repo := managementRepo(t)
	ctx := context.Background()
	root := t.TempDir()
	linked := filepath.Join(root, "linked")

	add, err := repo.ReviewWorktreeAdd(ctx, linked, "HEAD", "topic", WorktreeAddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AddWorktreeReviewed(ctx, add); err != nil {
		t.Fatal(err)
	}
	if err := repo.LockWorktree(ctx, linked, "test lock"); err != nil {
		t.Fatal(err)
	}
	all, err := repo.Worktrees(ctx)
	if err != nil || len(all) != 2 || !all[1].Locked || all[1].LockReason != "test lock" {
		t.Fatalf("locked worktrees = %#v, %v", all, err)
	}
	if err := repo.UnlockWorktree(ctx, linked); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(root, "moved")
	move, err := repo.ReviewWorktreeMutation(ctx, linked, moved, NotConfirmed)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveWorktreeReviewed(ctx, move); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("moved worktree: %v", err)
	}
	remove, err := repo.ReviewWorktreeMutation(ctx, moved, "", NotConfirmed)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveWorktreeReviewed(ctx, remove); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedWorktreeAddSupportsNoCheckoutAndLockOnCreate(t *testing.T) {
	_, repo := managementRepo(t)
	ctx := context.Background()
	linked := filepath.Join(t.TempDir(), "advanced")
	checkout := false
	review, err := repo.ReviewWorktreeAdd(ctx, linked, "HEAD", "advanced-topic", WorktreeAddOptions{
		Checkout: &checkout, Lock: true, LockReason: "offline disk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AddWorktreeReviewed(ctx, review); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(linked, "tracked.txt")); !os.IsNotExist(err) {
		t.Fatalf("no-checkout worktree populated tracked files: %v", err)
	}
	all, err := repo.Worktrees(ctx)
	if err != nil || len(all) != 2 || !all[1].Locked || all[1].LockReason != "offline disk" || all[1].Branch != "advanced-topic" {
		t.Fatalf("advanced worktree = %#v, %v", all, err)
	}
	if _, err := repo.ReviewWorktreeAdd(ctx, filepath.Join(t.TempDir(), "invalid"), "HEAD", "", WorktreeAddOptions{LockReason: "missing lock"}); err == nil {
		t.Fatal("lock reason without lock-on-create was accepted")
	}
}

func TestReviewedWorktreeRemovalRejectsStaleStateAndUnsafePaths(t *testing.T) {
	r, repo := managementRepo(t)
	ctx := context.Background()
	linked := filepath.Join(t.TempDir(), "linked")
	if err := repo.AddWorktree(ctx, linked, "HEAD", WorktreeAddOptions{Detach: true}); err != nil {
		t.Fatal(err)
	}
	review, err := repo.ReviewWorktreeMutation(ctx, linked, "", Confirmed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linked, "stale.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveWorktreeReviewed(ctx, review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale removal error = %v", err)
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatalf("stale review removed worktree: %v", err)
	}
	if _, err := repo.ReviewWorktreeMutation(ctx, r.dir, "", Confirmed); !errors.Is(err, ErrPrimaryWorktree) {
		t.Fatalf("primary review error = %v", err)
	}
	if _, err := repo.ReviewWorktreeAdd(ctx, string(filepath.Separator), "HEAD", "", WorktreeAddOptions{}); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("root add review error = %v", err)
	}
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReviewWorktreeAdd(ctx, symlink, "HEAD", "", WorktreeAddOptions{}); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("symlink add review error = %v", err)
	}
}

func TestWorktreePrunePlanRejectsStaleSetAndExecutesExactSet(t *testing.T) {
	_, repo := managementRepo(t)
	ctx := context.Background()
	root := t.TempDir()
	first := filepath.Join(root, "first")
	if err := repo.AddWorktree(ctx, first, "HEAD", WorktreeAddOptions{Detach: true}); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(root, "second")
	if err := repo.AddWorktree(ctx, second, "HEAD", WorktreeAddOptions{Detach: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	// "now" makes the administrative records eligible independent of their
	// filesystem timestamp and of Git's configured default expiry.
	plan, err := repo.ReviewWorktreePrune(ctx, "now")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(second); err != nil {
		t.Fatal(err)
	}
	changed, err := repo.ReviewWorktreePrune(ctx, "now")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Output == plan.Output {
		t.Fatalf("prune fixture did not change: %q", plan.Output)
	}
	if err := repo.PruneWorktreesReviewed(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale prune error = %v; before=%q after=%q", err, plan.Output, changed.Output)
	}
	current, err := repo.ReviewWorktreePrune(ctx, "now")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PruneWorktreesReviewed(ctx, current); err != nil {
		t.Fatal(err)
	}
	all, err := repo.Worktrees(ctx)
	if err != nil || len(all) != 1 || !all[0].Primary {
		t.Fatalf("worktrees after prune = %#v, %v", all, err)
	}
}
