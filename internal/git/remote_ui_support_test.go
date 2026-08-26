package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReviewedRemoteRenameAndRemovalRejectStaleConfiguration(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", newBareTestRepo(t).dir)
	repo, _ := Discover(local.dir)

	rename, err := repo.ReviewRemoteRename(ctx, "origin", "upstream")
	if err != nil {
		t.Fatal(err)
	}
	local.git("config", "remote.pushDefault", "origin")
	if err := repo.RenameRemoteReviewed(ctx, rename); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("rename stale error = %v", err)
	}
	if got := local.git("remote"); got != "origin" {
		t.Fatalf("stale rename mutated remote: %q", got)
	}

	remove, err := repo.ReviewRemoteRemoval(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	local.git("config", "branch.main.pushRemote", "origin")
	if err := repo.RemoveRemoteReviewed(ctx, remove); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("remove stale error = %v", err)
	}
	if got := local.git("remote"); got != "origin" {
		t.Fatalf("stale removal mutated remote: %q", got)
	}
}

func TestReviewedRemoteConfigurationPreservesNilEmptyAndRejectsStalePlan(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", newBareTestRepo(t).dir)
	local.git("config", "--add", "remote.origin.push", "refs/heads/main:refs/heads/main")
	repo, _ := Discover(local.dir)

	plan, err := repo.ReviewRemoteConfiguration(ctx, RemoteConfigArgs{Remote: "origin", FetchRefspecs: nil, PushRefspecs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Args.FetchRefspecs != nil || plan.Args.PushRefspecs == nil {
		t.Fatalf("nil/empty distinction lost: %#v", plan.Args)
	}
	local.git("config", "remote.origin.tagopt", "--no-tags")
	if err := repo.ConfigureRemoteReviewed(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("configuration stale error = %v", err)
	}
	if got := local.git("config", "--get-all", "remote.origin.push"); !strings.Contains(got, "refs/heads/main") {
		t.Fatalf("stale configuration cleared push refspec: %q", got)
	}

	plan, err = repo.ReviewRemoteConfiguration(ctx, RemoteConfigArgs{Remote: "origin", PushRefspecs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ConfigureRemoteReviewed(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if out, err := repo.configValues(ctx, "remote.origin.push"); err != nil || out != nil {
		t.Fatalf("clear result = %#v, %v", out, err)
	}
}

func TestReviewedRemoteDefaultBranchUsesAdvertisedSymbolicHead(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base", "base\n")
	seed.commitAll("base")
	seed.git("remote", "add", "origin", remote.dir)
	seed.git("push", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	local := cloneTestRepo(t, remote.dir)
	repo, _ := Discover(local.dir)
	local.git("symbolic-ref", "-d", "refs/remotes/origin/HEAD")

	plan, err := repo.ReviewRemoteDefaultBranch(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewRef != "refs/remotes/origin/main" {
		t.Fatalf("new ref = %q", plan.NewRef)
	}
	if err := repo.UpdateRemoteDefaultBranch(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if got := local.git("symbolic-ref", "refs/remotes/origin/HEAD"); got != plan.NewRef {
		t.Fatalf("tracking HEAD = %q", got)
	}
}
