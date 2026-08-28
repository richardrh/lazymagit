package git

import (
	"context"
	"strings"
	"testing"
)

func TestReviewedTagMutationsRejectStaleObject(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "one\n")
	first := r.commitAll("one")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := repo.CreateTagWithArgs(ctx, CreateTagArgs{Name: "bad", Target: first, LocalUser: "key\nunsafe"}); err == nil {
		t.Fatal("tag signing identity with newline was accepted")
	}
	if _, err := repo.CreateTagWithArgs(ctx, CreateTagArgs{Name: "v1", Target: first}); err != nil {
		t.Fatal(err)
	}

	deleteReview, err := repo.ReviewTagDelete(ctx, []string{"v1"})
	if err != nil {
		t.Fatal(err)
	}
	r.write("file", "two\n")
	second := r.commitAll("two")
	r.git("tag", "--force", "v1", second)
	if err := repo.DeleteTagsReviewed(ctx, deleteReview); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale delete = %v", err)
	}
	if got := r.git("rev-parse", "v1"); got != second {
		t.Fatalf("stale review changed tag to %q", got)
	}

	createReview, err := repo.ReviewTagCreate(ctx, CreateTagArgs{Name: "v1", Target: first, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	r.git("tag", "--force", "v1", first)
	if err := repo.CreateTagReviewed(ctx, createReview); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale replacement = %v", err)
	}
	if got := r.git("rev-parse", "v1"); got != first {
		t.Fatalf("stale replacement changed tag to %q", got)
	}
}

func TestReviewedRemoteTagPruneBindsRemoteAndObjects(t *testing.T) {
	local := newTestRepo(t)
	local.write("file", "one\n")
	first := local.commitAll("one")
	remote := newBareTestRepo(t)
	local.git("remote", "add", "origin", remote.dir)
	local.git("tag", "keep", first)
	local.git("tag", "stale", first)
	local.git("push", "origin", "refs/tags/keep")
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	review, err := repo.ReviewRemoteTagPrune(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(review.Comparison.LocalOnly, ",") != "stale" || review.ObjectIDs["stale"] != first {
		t.Fatalf("review = %#v", review)
	}
	local.write("file", "two\n")
	second := local.commitAll("two")
	local.git("tag", "--force", "stale", second)
	if err := repo.PruneRemoteTagsReviewed(ctx, review); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale prune = %v", err)
	}
	if got := local.git("rev-parse", "stale"); got != second {
		t.Fatalf("stale prune changed tag to %q", got)
	}
}

func TestNotesMergeStrategyIsValidatedAndPassedToGit(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "one\n")
	oid := r.commitAll("one")
	r.git("notes", "--ref=source", "add", "-m", "source note", oid)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.NotesMergeStart(ctx, "dest", "refs/notes/source", "union"); err != nil {
		t.Fatal(err)
	}
	if got := r.git("notes", "--ref=dest", "show", oid); got != "source note" {
		t.Fatalf("merged note = %q", got)
	}
	if err := repo.NotesMergeStart(ctx, "dest", "refs/notes/source", "--unsafe"); err == nil {
		t.Fatal("option-like merge strategy was accepted")
	}
}

func TestNotesUIMessageAndReviewedRemoval(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "one\n")
	oid := r.commitAll("one")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.NotesWriteMessage(ctx, "review", oid, "first\nsecond\n", false); err != nil {
		t.Fatal(err)
	}
	message, exists, err := repo.NotesMessage(ctx, "review", oid)
	if err != nil || !exists || message != "first\nsecond\n" {
		t.Fatalf("message=%q exists=%v err=%v", message, exists, err)
	}
	if err := repo.NotesWriteMessage(ctx, "review", oid, strings.Repeat("x", MaxNotesUIMessageBytes+1), true); err == nil {
		t.Fatal("oversized note was accepted")
	}
	review, err := repo.ReviewNotesRemoval(ctx, "review", []string{oid})
	if err != nil {
		t.Fatal(err)
	}
	r.git("notes", "--ref=review", "add", "--force", "-m", "changed", oid)
	if err := repo.RemoveNotesReviewed(ctx, review); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale notes removal = %v", err)
	}
	if got := r.git("notes", "--ref=review", "show", oid); got != "changed" {
		t.Fatalf("stale removal changed note: %q", got)
	}
}
