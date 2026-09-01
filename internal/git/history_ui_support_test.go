package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneHistoryUIRequestCopiesSlices(t *testing.T) {
	original := HistoryUIRequest{
		Reset:     ResetOptions{Paths: []string{"reset"}},
		Revisions: []string{"revision"},
		Bisect:    BisectStartOptions{Paths: []string{"bisect"}},
	}
	cloned := cloneHistoryUIRequest(original)
	cloned.Reset.Paths[0] = "changed reset"
	cloned.Revisions[0] = "changed revision"
	cloned.Bisect.Paths[0] = "changed bisect"
	if original.Reset.Paths[0] != "reset" || original.Revisions[0] != "revision" || original.Bisect.Paths[0] != "bisect" {
		t.Fatalf("clone mutated original: %#v", original)
	}
}

func TestHistoryUIActionClassifiers(t *testing.T) {
	if !isRebaseHistoryUIAction(HistoryUIRebaseStart) {
		t.Fatal("rebase start must be classified as a rebase action")
	}
	if !isSequencerHistoryUIAction(HistoryUIRevertAbort) {
		t.Fatal("revert abort must be classified as a sequencer action")
	}
	if !isBisectHistoryUIAction(HistoryUIBisectGood) {
		t.Fatal("bisect good must be classified as a bisect action")
	}
	if isRebaseHistoryUIAction("unknown") || isSequencerHistoryUIAction("unknown") || isBisectHistoryUIAction("unknown") {
		t.Fatal("unknown action was classified")
	}
}

func TestReviewedHistoryUIActionIsCurrent(t *testing.T) {
	request := HistoryUIRequest{Action: HistoryUIBisectReset}
	state := HistoryUIState{HEAD: "head", Operation: "bisect"}
	reviewed := ReviewedHistoryUIAction{Request: request, State: state}
	reviewed.Token = NewConfirmationToken(historyUIIdentity(reviewed))
	if !reviewedHistoryUIActionIsCurrent(reviewed, reviewed) {
		t.Fatal("identical reviewed actions were stale")
	}
	current := reviewed
	current.State.HEAD = "moved"
	if reviewedHistoryUIActionIsCurrent(reviewed, current) {
		t.Fatal("state change was accepted")
	}
}

func TestExecuteReviewedRebaseUIAction(t *testing.T) {
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		request   HistoryUIRequest
		handled   bool
		wantError error
		wantText  string
	}{
		{name: "start", request: HistoryUIRequest{Action: HistoryUIRebaseStart}, handled: true, wantText: "rebase upstream is empty"},
		{name: "continue", request: HistoryUIRequest{Action: HistoryUIRebaseContinue}, handled: true, wantError: ErrWorkflowNotActive},
		{name: "skip", request: HistoryUIRequest{Action: HistoryUIRebaseSkip}, handled: true, wantError: ErrWorkflowNotActive},
		{name: "abort", request: HistoryUIRequest{Action: HistoryUIRebaseAbort}, handled: true, wantError: ErrWorkflowNotActive},
		{name: "interactive", request: HistoryUIRequest{Action: HistoryUIRebaseInteractive}, handled: true, wantText: "rebase upstream is empty"},
		{name: "todo", request: HistoryUIRequest{Action: HistoryUIRebaseTodo}, handled: true, wantError: ErrWorkflowNotActive},
		{name: "unknown", request: HistoryUIRequest{Action: "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handled, err := repo.executeReviewedRebaseUIAction(context.Background(), tt.request)
			if handled != tt.handled {
				t.Fatalf("handled = %v, want %v", handled, tt.handled)
			}
			if tt.wantError != nil && !errors.Is(err, tt.wantError) {
				t.Fatalf("error = %v, want %v", err, tt.wantError)
			}
			if tt.wantText != "" && (err == nil || err.Error() != tt.wantText) {
				t.Fatalf("error = %v, want %q", err, tt.wantText)
			}
			if tt.wantError == nil && tt.wantText == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("reset confirms reviewed request", func(t *testing.T) {
		request := HistoryUIRequest{Action: HistoryUIReset, Reset: ResetOptions{Mode: ResetSoft, Target: "HEAD"}}
		if err := repo.Reset(context.Background(), request.Reset); !errors.Is(err, ErrConfirmationRequired) {
			t.Fatalf("unreviewed reset error = %v, want ErrConfirmationRequired", err)
		}
		var records []ProcessRecord
		ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) {
			records = append(records, record)
		})
		handled, err := repo.executeReviewedRebaseUIAction(ctx, request)
		if !handled || err != nil {
			t.Fatalf("execute reset = (%v, %v), want (true, nil)", handled, err)
		}
		if len(records) != 1 || len(records[0].Args) < 2 || records[0].Args[0] != "reset" || records[0].Args[1] != "--soft" {
			t.Fatalf("reset records = %#v", records)
		}
	})
}

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

func TestHistoryUIReviewCanonicalizesExtendedOptions(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.write("first.txt", "first\n")
	r.commitAll("first")
	r.write("second.txt", "second\n")
	r.commitAll("second")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	rebase, err := repo.ReviewHistoryUIAction(ctx, HistoryUIRequest{Action: HistoryUIRebaseStart, Rebase: RebaseOptions{
		Upstream: "HEAD~1", Onto: "HEAD", KeepEmpty: true, RebaseMerges: true, UpdateRefs: true,
		Autostash: true, ForceRebase: true, Strategy: "recursive", Signoff: true,
	}})
	if err != nil {
		t.Fatalf("review rebase: %v", err)
	}
	if len(rebase.Plan) != 10 || rebase.Request.Rebase.Upstream == "HEAD~1" || rebase.Request.Rebase.Onto == "HEAD" {
		t.Fatalf("rebase review = %#v", rebase)
	}

	for _, request := range []HistoryUIRequest{
		{Action: HistoryUICherryStart, Revisions: []string{"HEAD"}, Pick: PickOptions{NoCommit: true, Mainline: 1, Strategy: "recursive", Signoff: true, NoEdit: true}},
		{Action: HistoryUIRevertStart, Revisions: []string{"HEAD"}, Pick: PickOptions{NoEdit: true}},
		{Action: HistoryUIBisectStart, Bisect: BisectStartOptions{Bad: "HEAD", Good: "HEAD~1", NoCheckout: true, FirstParent: true}},
		{Action: HistoryUIBisectSkip},
	} {
		review, err := repo.ReviewHistoryUIAction(ctx, request)
		if err != nil {
			t.Fatalf("review %s: %v", request.Action, err)
		}
		if len(review.Plan) == 0 {
			t.Fatalf("review %s has no plan", request.Action)
		}
	}

	for _, request := range []HistoryUIRequest{
		{Action: HistoryUICherryStart},
		{Action: HistoryUIRevertStart, Revisions: []string{"HEAD"}, Pick: PickOptions{Mainline: -1}},
		{Action: HistoryUICherryStart, Revisions: []string{"HEAD"}, Pick: PickOptions{Strategy: "bad strategy"}},
	} {
		if _, err := repo.ReviewHistoryUIAction(ctx, request); err == nil {
			t.Errorf("review %s = nil error for %#v", request.Action, request)
		}
	}
}

func TestReviewedHistoryStartsCherryPickAndRevert(t *testing.T) {
	ctx := context.Background()
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

	review, err := repo.ReviewHistoryUIAction(ctx, HistoryUIRequest{Action: HistoryUICherryStart, Revisions: []string{picked}, Pick: PickOptions{NoCommit: true, NoEdit: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedHistoryUIAction(ctx, review); err != nil {
		t.Fatalf("execute reviewed cherry-pick: %v", err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "picked.txt" {
		t.Fatalf("cherry-pick index = %q", got)
	}
	r.git("reset", "--hard", "HEAD")

	r.write("reverted.txt", "revert me\n")
	commit := r.commitAll("revert me")
	review, err = repo.ReviewHistoryUIAction(ctx, HistoryUIRequest{Action: HistoryUIRevertStart, Revisions: []string{commit}, Pick: PickOptions{NoEdit: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedHistoryUIAction(ctx, review); err != nil {
		t.Fatalf("execute reviewed revert: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "reverted.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revert retained file: %v", err)
	}
}

func TestReviewedHistoryRejectsInactiveContinuationActions(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []HistoryUIAction{
		HistoryUIRebaseContinue,
		HistoryUICherryContinue,
		HistoryUIRevertContinue,
		HistoryUIBisectSkip,
	} {
		review, err := repo.ReviewHistoryUIAction(context.Background(), HistoryUIRequest{Action: action})
		if err != nil {
			t.Fatalf("review %s: %v", action, err)
		}
		if err := repo.ExecuteReviewedHistoryUIAction(context.Background(), review); err == nil {
			t.Errorf("execute inactive %s succeeded", action)
		}
	}
}

func TestReviewedHistoryOptionsBindConfirmationIdentity(t *testing.T) {
	r := newTestRepo(t)
	r.write("file.txt", "one\n")
	r.commitAll("one")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	review, err := repo.ReviewHistoryUIAction(context.Background(), HistoryUIRequest{Action: HistoryUICherryStart, Revisions: []string{"HEAD"}, Pick: PickOptions{FastForward: true, RecordOrigin: true, NoEdit: true}})
	if err != nil {
		t.Fatal(err)
	}
	mutated := review
	mutated.Request.Pick.FastForward = false
	if err := repo.ExecuteReviewedHistoryUIAction(context.Background(), mutated); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("option-mutated execute error = %v, want ErrStalePlan", err)
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
