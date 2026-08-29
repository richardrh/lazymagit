package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReviewedMergeExecutesAndRejectsHeadOrConfigChanges(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("topic", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	var records []ProcessRecord
	ctx = WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	advanced := MergeArgs{Target: "topic", Mode: MergeNoFF, Strategy: "ort", StrategyOptions: []string{"ignore-space-change"}, Signoff: true}
	plan, err := repo.ReviewMerge(ctx, advanced)
	if err != nil {
		t.Fatal(err)
	}
	r.git("config", "merge.stat", "false")
	if _, err := repo.ExecuteReviewedMerge(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("config-changed execute error = %v, want ErrStalePlan", err)
	}
	plan, err = repo.ReviewMerge(ctx, advanced)
	if err != nil {
		t.Fatal(err)
	}
	mutated := plan
	mutated.Args.Signoff = false
	if _, err := repo.ExecuteReviewedMerge(ctx, mutated); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("option-mutated execute error = %v, want ErrStalePlan", err)
	}
	if _, err := repo.ExecuteReviewedMerge(ctx, plan); err != nil {
		t.Fatalf("execute reviewed merge: %v", err)
	}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args[:5], []string{"merge", "--no-ff", "--no-edit", "--strategy=ort", "--strategy-option=ignore-space-change"}) || records[0].Args[5] != "--signoff" {
		t.Fatalf("advanced merge argv = %#v", records)
	}
	if parents := r.git("rev-list", "--parents", "-n", "1", "HEAD"); len(strings.Fields(parents)) != 3 {
		t.Fatalf("advanced merge did not create a merge commit: %s", parents)
	}
}

func TestReviewedMergeContinueBindsPreparedResolution(t *testing.T) {
	ctx := context.Background()
	r, repo := conflictedRepository(t)

	r.write("conflict.txt", "resolved one\n")
	r.git("add", "--", "conflict.txt")
	reviewed, err := repo.ReviewMergeContinue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.write("conflict.txt", "resolved two\n")
	r.git("add", "--", "conflict.txt")
	if err := repo.ExecuteReviewedMergeContinue(ctx, reviewed); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("index-stale continue error = %v, want ErrStalePlan", err)
	}
	reviewed, err = repo.ReviewMergeContinue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.git("config", "merge.stat", "false")
	if err := repo.ExecuteReviewedMergeContinue(ctx, reviewed); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("config-stale continue error = %v, want ErrStalePlan", err)
	}
	reviewed, err = repo.ReviewMergeContinue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedMergeContinue(ctx, reviewed); err != nil {
		t.Fatalf("continue reviewed merge: %v", err)
	}
	if got := r.git("status", "--porcelain"); got != "" {
		t.Fatalf("status after continue = %q", got)
	}
}

func TestReviewedMergeAbortIsDestructiveAndStaleSafe(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("conflict", "base\n")
	base := r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("conflict", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("conflict", "main\n")
	r.commitAll("main")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeWithArgs(ctx, MergeArgs{Target: "topic", Mode: MergePlain}); err == nil {
		t.Fatal("conflicting merge unexpectedly succeeded")
	}

	plan, err := repo.ReviewMergeAbort(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.git("config", "merge.conflictStyle", "diff3")
	if err := repo.ExecuteReviewedMergeAbort(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale abort error = %v, want ErrStalePlan", err)
	}
	plan, err = repo.ReviewMergeAbort(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedMergeAbort(ctx, plan); err != nil {
		t.Fatalf("abort reviewed merge: %v", err)
	}
	if got := r.git("merge-base", "--is-ancestor", base, "HEAD"); got != "" {
		t.Fatalf("unexpected merge-base output %q", got)
	}
	state, err := repo.QueryOperationState(ctx)
	if err != nil || state.InProgress() {
		t.Fatalf("operation state after abort = %#v, %v", state, err)
	}
}
