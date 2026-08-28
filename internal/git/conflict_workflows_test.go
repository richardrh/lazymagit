package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestConflictWorkflowListsInspectsResolvesAndStages(t *testing.T) {
	ctx := context.Background()
	r, repo := conflictedRepository(t)

	paths, err := repo.UnmergedPaths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0].Path != "conflict.txt" {
		t.Fatalf("UnmergedPaths = %#v", paths)
	}
	if got, want := paths[0].Stages, []ConflictStage{ConflictBase, ConflictOurs, ConflictTheirs}; !sameConflictStages(got, want) {
		t.Fatalf("stages = %v, want %v", got, want)
	}

	inspection, err := repo.InspectConflict(ctx, "conflict.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := conflictBlobText(inspection, ConflictBase), "base\n"; got != want {
		t.Fatalf("base = %q, want %q", got, want)
	}
	if got, want := conflictBlobText(inspection, ConflictOurs), "main\n"; got != want {
		t.Fatalf("ours = %q, want %q", got, want)
	}
	if got, want := conflictBlobText(inspection, ConflictTheirs), "topic\n"; got != want {
		t.Fatalf("theirs = %q, want %q", got, want)
	}
	if _, err := repo.ReviewConflictResolution(ctx, "conflict.txt", ResolveBase); !errors.Is(err, ErrConflictResolutionUnsupported) {
		t.Fatalf("base review error = %v, want unsupported", err)
	}

	review, err := repo.ReviewConflictResolution(ctx, "conflict.txt", ResolveTheirs)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedConflictResolution(ctx, review); err != nil {
		t.Fatalf("resolve theirs: %v", err)
	}
	if got, want := r.read("conflict.txt"), "topic\n"; got != want {
		t.Fatalf("worktree = %q, want %q", got, want)
	}
	if status := r.git("status", "--porcelain"); strings.Contains(status, "UU") || status != "M  conflict.txt" {
		t.Fatalf("status after resolution = %q, want staged path", status)
	}
}

func TestConflictResolutionReviewRejectsStaleWorktreeAndIndex(t *testing.T) {
	ctx := context.Background()
	r, repo := conflictedRepository(t)

	review, err := repo.ReviewConflictResolution(ctx, "conflict.txt", ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	r.write("conflict.txt", "manual resolution\n")
	if err := repo.ExecuteReviewedConflictResolution(ctx, review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("worktree-stale execute error = %v, want ErrStalePlan", err)
	}
	if got := r.read("conflict.txt"); got != "manual resolution\n" {
		t.Fatalf("stale execution overwrote worktree: %q", got)
	}

	// Recreate the reviewed state, then alter an index stage without changing
	// the path name. The token must not stage a newly supplied resolution.
	r.git("checkout", "--conflict=merge", "--", "conflict.txt")
	review, err = repo.ReviewConflictResolution(ctx, "conflict.txt", ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	r.git("update-index", "--cacheinfo", "100644,"+r.git("rev-parse", "HEAD:conflict.txt")+",conflict.txt")
	if err := repo.ExecuteReviewedConflictResolution(ctx, review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("index-stale execute error = %v, want ErrStalePlan", err)
	}
}

func conflictedRepository(t *testing.T) (*testRepo, *Repository) {
	t.Helper()
	r := newTestRepo(t)
	r.write("conflict.txt", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("conflict.txt", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("conflict.txt", "main\n")
	r.commitAll("main")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeWithArgs(context.Background(), MergeArgs{Target: "topic", Mode: MergePlain}); err == nil {
		t.Fatal("conflicting merge unexpectedly succeeded")
	}
	return r, repo
}

func conflictBlobText(inspection ConflictInspection, stage ConflictStage) string {
	for _, blob := range inspection.Blobs {
		if blob.Stage == stage {
			return string(blob.Content)
		}
	}
	return ""
}

func sameConflictStages(left, right []ConflictStage) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
