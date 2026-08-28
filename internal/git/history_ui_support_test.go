package git

import (
	"context"
	"errors"
	"testing"
)

func TestReviewedHistoryResetBindsHEADIndexAndWorktree(t *testing.T) {
	ctx := context.Background()
	for _, mutate := range []struct {
		name string
		do   func(*testRepo)
	}{
		{"HEAD", func(r *testRepo) { r.write("head.txt", "new\n"); r.commitAll("move head") }},
		{"index", func(r *testRepo) { r.write("file.txt", "staged\n"); r.git("add", "--", "file.txt") }},
		{"worktree", func(r *testRepo) { r.write("file.txt", "unstaged replacement\n") }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			r := newTestRepo(t)
			r.write("file.txt", "base\n")
			r.commitAll("base")
			repo, err := Discover(r.dir)
			if err != nil {
				t.Fatal(err)
			}
			review, err := repo.ReviewHistoryUIAction(ctx, HistoryUIRequest{Action: HistoryUIReset, Reset: ResetOptions{Mode: ResetHard, Target: "HEAD"}})
			if err != nil {
				t.Fatal(err)
			}
			mutate.do(r)
			if err := repo.ExecuteReviewedHistoryUIAction(ctx, review); !errors.Is(err, ErrStalePlan) {
				t.Fatalf("execute after %s mutation = %v, want ErrStalePlan", mutate.name, err)
			}
		})
	}
}

func TestReviewedHistoryResetSuccessPreservesExactTargetState(t *testing.T) {
	r := newTestRepo(t)
	r.write("file.txt", "one\n")
	one := r.commitAll("one")
	r.write("file.txt", "two\n")
	r.commitAll("two")
	r.write("file.txt", "dirty\n")
	r.write("untracked.txt", "untracked\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	review, err := repo.ReviewHistoryUIAction(context.Background(), HistoryUIRequest{Action: HistoryUIReset, Reset: ResetOptions{Mode: ResetHard, Target: one}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedHistoryUIAction(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if got := r.git("rev-parse", "HEAD"); got != one {
		t.Fatalf("HEAD = %s, want %s", got, one)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index diff = %q", got)
	}
	if got := r.git("diff", "--", "file.txt"); got != "" {
		t.Fatalf("worktree diff = %q", got)
	}
	if got := r.read("file.txt"); got != "one\n" {
		t.Fatalf("file = %q", got)
	}
	if got := r.read("untracked.txt"); got != "untracked\n" {
		t.Fatalf("hard reset changed untracked file: %q", got)
	}
}

func TestReviewedHistoryCherryPickBindsResolvedCommitAndWorktree(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "source")
	r.write("picked.txt", "picked\n")
	picked := r.commitAll("picked")
	r.git("switch", "main")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	review, err := repo.ReviewHistoryUIAction(context.Background(), HistoryUIRequest{Action: HistoryUICherryStart, Pick: PickOptions{NoCommit: true, NoEdit: true}, Revisions: []string{picked}})
	if err != nil {
		t.Fatal(err)
	}
	if review.Request.Revisions[0] != picked || len(review.Plan) < 2 {
		t.Fatalf("review = %#v", review)
	}
	r.write("base.txt", "changed after review\n")
	if err := repo.ExecuteReviewedHistoryUIAction(context.Background(), review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("execute stale cherry-pick = %v", err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("stale cherry-pick changed index = %q", got)
	}
}

func TestReviewedHistoryActionBindsOperationState(t *testing.T) {
	r := newTestRepo(t)
	r.write("file.txt", "one\n")
	good := r.commitAll("one")
	r.write("file.txt", "two\n")
	r.commitAll("two")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	review, err := repo.ReviewHistoryUIAction(context.Background(), HistoryUIRequest{Action: HistoryUIReset, Reset: ResetOptions{Mode: ResetMixed, Target: "HEAD"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.BisectStart(context.Background(), BisectStartOptions{Bad: "HEAD", Good: good}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedHistoryUIAction(context.Background(), review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("execute after operation-state change = %v, want ErrStalePlan", err)
	}
}
