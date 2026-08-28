package git

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReviewedBranchCreationDefaultsRejectStaleConfiguration(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	request := BranchConfigUpdate{Branch: "main", Description: ConfigUpdate{Action: ConfigKeep}, Upstream: ConfigUpdate{Action: ConfigKeep}, Rebase: ConfigUpdate{Action: ConfigKeep}, PushRemote: ConfigUpdate{Action: ConfigKeep}, PullRebase: ConfigUpdate{Action: ConfigKeep}, RemotePushDefault: ConfigUpdate{Action: ConfigKeep}, AutoSetupMerge: ConfigUpdate{Action: ConfigSet, Value: "simple"}, AutoSetupRebase: ConfigUpdate{Action: ConfigSet, Value: "remote"}}
	review, err := repo.ReviewBranchConfigUpdate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	r.git("config", "branch.autoSetupMerge", "always")
	if err := repo.ExecuteBranchConfigUpdate(ctx, review); err != ErrStalePlan {
		t.Fatalf("stale defaults update = %v", err)
	}
	if got, err := repo.workflowConfigValue(ctx, "branch.autoSetupRebase"); err != nil || got.Set {
		t.Fatalf("stale review changed rebase default: %#v, %v", got, err)
	}

	review, err = repo.ReviewBranchConfigUpdate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteBranchConfigUpdate(ctx, review); err != nil {
		t.Fatal(err)
	}
	if got := r.git("config", "--get", "branch.autoSetupMerge"); got != "simple" {
		t.Fatalf("autoSetupMerge = %q", got)
	}
	if got := r.git("config", "--get", "branch.autoSetupRebase"); got != "remote" {
		t.Fatalf("autoSetupRebase = %q", got)
	}
}

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
