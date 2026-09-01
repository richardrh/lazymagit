package git

import (
	"context"
	"errors"
	"testing"
)

func TestReviewedRemoteBranchPushTrackingCheckoutAndDeleteWithBareRemote(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	oid := local.commitAll("base")
	remote := newBareTestRepo(t)
	local.git("remote", "add", "origin", remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	push, err := repo.ReviewRemoteBranchPush(ctx, "main", "origin", "topic", true)
	if err != nil {
		t.Fatal(err)
	}
	if push.LocalOID != oid || push.RemoteOID != "" {
		t.Fatalf("push review = %#v", push)
	}
	if err := repo.PushRemoteBranchReviewed(ctx, push); err != nil {
		t.Fatal(err)
	}
	if got := remote.git("rev-parse", "refs/heads/topic"); got != oid {
		t.Fatalf("remote topic = %q, want %q", got, oid)
	}
	if got := local.git("config", "--get", "branch.main.remote"); got != "origin" {
		t.Fatalf("branch.main.remote = %q", got)
	}
	if got := local.git("config", "--get", "branch.main.merge"); got != "refs/heads/topic" {
		t.Fatalf("branch.main.merge = %q", got)
	}

	local.git("fetch", "origin")
	local.git("switch", "-c", "other")
	if err := repo.CheckoutRemoteTrackingBranch(ctx, "origin/topic", "tracked-topic", CheckoutOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := local.git("symbolic-ref", "--short", "HEAD"); got != "tracked-topic" {
		t.Fatalf("checked out branch = %q", got)
	}
	if got := local.git("rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/topic" {
		t.Fatalf("tracking upstream = %q", got)
	}

	deletion, err := repo.ReviewRemoteBranchDelete(ctx, "origin", "topic")
	if err != nil {
		t.Fatal(err)
	}
	if deletion.OID != oid {
		t.Fatalf("delete review OID = %q", deletion.OID)
	}
	if err := repo.DeleteRemoteBranchReviewed(ctx, deletion); err != nil {
		t.Fatal(err)
	}
	if got := remote.git("for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("remote branch remained: %q", got)
	}
}

func TestReviewedRemoteBranchChangesRejectStalePlans(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	remote := newBareTestRepo(t)
	local.git("remote", "add", "origin", remote.dir)
	repo, _ := Discover(local.dir)

	push, err := repo.ReviewRemoteBranchPush(ctx, "main", "origin", "topic", false)
	if err != nil {
		t.Fatal(err)
	}
	local.write("next", "next\n")
	local.commitAll("move main")
	if err := repo.PushRemoteBranchReviewed(ctx, push); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale push error = %v", err)
	}

	local.git("push", "origin", "main:topic")
	deletion, err := repo.ReviewRemoteBranchDelete(ctx, "origin", "topic")
	if err != nil {
		t.Fatal(err)
	}
	local.write("third", "third\n")
	local.commitAll("move remote")
	local.git("push", "origin", "main:topic")
	if err := repo.DeleteRemoteBranchReviewed(ctx, deletion); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale delete error = %v", err)
	}
}
