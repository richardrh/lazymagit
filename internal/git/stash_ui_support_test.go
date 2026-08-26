package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReviewedStashApplyAndBranchTargetExactOIDAcrossReflogMovement(t *testing.T) {
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
	var records []ProcessRecord
	recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	if err := repo.ApplyReviewedStash(recorded, reviewed, StashApplyOptions{Index: true}); err != nil {
		t.Fatal(err)
	}
	if got := r.read("file"); got != "old\n" {
		t.Fatalf("exact reviewed apply targeted wrong stash: %q", got)
	}
	stashes, _ := repo.Stashes(ctx)
	if len(stashes) != 2 {
		t.Fatalf("exact apply removed a stash: %#v", stashes)
	}
	assertRecordedArgs(t, records, []string{"stash", "apply", "--index", reviewed.Stash.ID})

	r.git("reset", "--hard", "HEAD")
	records = nil
	if err := repo.BranchReviewedStash(recorded, "from-reviewed", reviewed); err != nil {
		t.Fatal(err)
	}
	if got := r.git("branch", "--show-current"); got != "from-reviewed" || r.read("file") != "old\n" {
		t.Fatalf("exact reviewed branch targeted wrong stash: branch=%q file=%q", got, r.read("file"))
	}
	stashes, _ = repo.Stashes(ctx)
	if len(stashes) != 2 {
		t.Fatalf("OID-form stash branch removed a stash: %#v", stashes)
	}
	assertRecordedArgs(t, records, []string{"stash", "branch", "from-reviewed", reviewed.Stash.ID})
}

func TestReviewedStashRemovalFailsClosedAndStaleTokensStillFail(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file", "stash\n")
	if err := repo.StashPush(ctx, StashPushOptions{}); err != nil {
		t.Fatal(err)
	}
	reviewed, err := repo.ReviewStash(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"pop":  repo.PopReviewedStash(ctx, reviewed, StashApplyOptions{}),
		"drop": repo.DropReviewedStash(ctx, reviewed),
	} {
		if !errors.Is(err, ErrReviewedStashRemovalUnsupported) {
			t.Fatalf("reviewed %s = %v", name, err)
		}
	}
	if stashes, _ := repo.Stashes(ctx); len(stashes) != 1 || r.read("file") != "base\n" {
		t.Fatalf("fail-closed removal mutated state: %#v file=%q", stashes, r.read("file"))
	}
	if err := repo.StashDrop(ctx, reviewed.Stash.ID, ConfirmationOptions{Token: reviewed.token}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyReviewedStash(ctx, reviewed, StashApplyOptions{}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale apply = %v", err)
	}
	if err := repo.PopReviewedStash(ctx, reviewed, StashApplyOptions{}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale pop = %v", err)
	}
	if err := repo.DropReviewedStash(ctx, reviewed); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale drop = %v", err)
	}
	if err := repo.BranchReviewedStash(ctx, "stale-branch", reviewed); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale branch = %v", err)
	}
}

func TestReviewedStashApplyConflictRetainsStash(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file", "stash\n")
	_ = repo.StashPush(ctx, StashPushOptions{})
	reviewed, _ := repo.ReviewStash(ctx, "")
	r.write("file", "head\n")
	r.commitAll("conflicting head")
	if err := repo.ApplyReviewedStash(ctx, reviewed, StashApplyOptions{}); err == nil {
		t.Fatal("conflicting reviewed apply succeeded")
	}
	stashes, _ := repo.Stashes(ctx)
	if len(stashes) != 1 || stashes[0].ID != reviewed.Stash.ID {
		t.Fatalf("conflicting reviewed apply lost stash: %#v", stashes)
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

func TestReviewedStashClearFailsClosedForUnchangedSet(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file", "stash\n")
	_ = repo.StashPush(ctx, StashPushOptions{})
	plan, err := repo.ReviewStashClear(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ClearReviewedStashes(ctx, plan); !errors.Is(err, ErrReviewedStashRemovalUnsupported) {
		t.Fatalf("reviewed clear = %v", err)
	}
	if stashes, _ := repo.Stashes(ctx); len(stashes) != 1 {
		t.Fatalf("reviewed clear mutated stashes: %#v", stashes)
	}
}

func assertRecordedArgs(t *testing.T, records []ProcessRecord, want []string) {
	t.Helper()
	wanted := strings.Join(want, "\x00")
	for _, record := range records {
		if strings.Join(record.Args, "\x00") == wanted {
			return
		}
	}
	t.Fatalf("process records do not contain %#v: %#v", want, records)
}
