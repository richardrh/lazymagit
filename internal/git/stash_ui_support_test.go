package git

import (
	"context"
	"errors"
	"testing"
)

func TestReviewedStashTargetsOIDAcrossReflogMovement(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file", "old\n")
	if err := repo.StashPush(ctx, StashPushOptions{Message: "old"}); err != nil {
		t.Fatal(err)
	}
	reviewed, err := repo.ReviewStash(ctx, "stash@{0}")
	if err != nil {
		t.Fatal(err)
	}
	r.write("file", "new\n")
	if err := repo.StashPush(ctx, StashPushOptions{Message: "new"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DropReviewedStash(ctx, reviewed); err != nil {
		t.Fatal(err)
	}
	stashes, _ := repo.Stashes(ctx)
	if len(stashes) != 1 || stashes[0].ID == reviewed.Stash.ID {
		t.Fatalf("wrong stash dropped: %#v", stashes)
	}
}

func TestReviewedStashPopRejectsStaleAndConflictRetainsStash(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file", "stash\n")
	if err := repo.StashPush(ctx, StashPushOptions{}); err != nil {
		t.Fatal(err)
	}
	reviewed, _ := repo.ReviewStash(ctx, "")
	if err := repo.DropReviewedStash(ctx, reviewed); err != nil {
		t.Fatal(err)
	}
	if err := repo.PopReviewedStash(ctx, reviewed, StashApplyOptions{}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale pop = %v", err)
	}

	r.write("file", "stash\n")
	_ = repo.StashPush(ctx, StashPushOptions{})
	reviewed, _ = repo.ReviewStash(ctx, "")
	r.write("file", "head\n")
	r.commitAll("conflicting head")
	if err := repo.PopReviewedStash(ctx, reviewed, StashApplyOptions{}); err == nil {
		t.Fatal("conflicting pop succeeded")
	}
	stashes, _ := repo.Stashes(ctx)
	if len(stashes) != 1 || stashes[0].ID != reviewed.Stash.ID {
		t.Fatalf("conflicting pop lost stash: %#v", stashes)
	}
}

func TestReviewedStashClearRejectsChangedSet(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file", "one\n")
	_ = repo.StashPush(ctx, StashPushOptions{})
	plan, err := repo.ReviewStashClear(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.write("file", "two\n")
	_ = repo.StashPush(ctx, StashPushOptions{})
	if err := repo.ClearReviewedStashes(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("changed clear plan = %v", err)
	}
	if stashes, _ := repo.Stashes(ctx); len(stashes) != 2 {
		t.Fatalf("stale clear mutated stashes: %#v", stashes)
	}
}
