package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedDiffPatchCreateOverwriteAndStaleReview(t *testing.T) {
	r := newTestRepo(t)
	r.write("tracked.txt", "base\n")
	r.git("add", "--", "tracked.txt")
	r.git("commit", "-m", "base")
	r.write("tracked.txt", "changed\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	review, err := repo.ReviewDiffPatch("change.patch", DiffPatchOptions{})
	if err != nil || review.Exists {
		t.Fatalf("ReviewDiffPatch(create) = %+v, %v", review, err)
	}
	if err := repo.ExecuteReviewedDiffPatch(context.Background(), review); err != nil {
		t.Fatalf("ExecuteReviewedDiffPatch(create): %v", err)
	}
	patch := r.read("change.patch")
	if !strings.Contains(patch, "-base") || !strings.Contains(patch, "+changed") {
		t.Fatalf("created patch did not contain the exact change:\n%s", patch)
	}

	options := DiffPatchOptions{Overwrite: true}
	review, err = repo.ReviewDiffPatch("change.patch", options)
	if err != nil || !review.Exists || review.Digest == "" {
		t.Fatalf("ReviewDiffPatch(overwrite) = %+v, %v", review, err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, "change.patch"), []byte("changed after review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedDiffPatch(context.Background(), review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale ExecuteReviewedDiffPatch error = %v", err)
	}
	if got := r.read("change.patch"); got != "changed after review\n" {
		t.Fatalf("stale execution replaced destination with %q", got)
	}

	review, err = repo.ReviewDiffPatch("change.patch", options)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedDiffPatch(context.Background(), review); err != nil {
		t.Fatalf("reviewed overwrite: %v", err)
	}
	if got := r.read("change.patch"); !strings.Contains(got, "+changed") {
		t.Fatalf("reviewed overwrite content:\n%s", got)
	}
}

func TestReviewDiffPatchRejectsSymlinkAndOversizedExistingFile(t *testing.T) {
	r := newTestRepo(t)
	r.write("target", "do not replace\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(r.dir, "link.patch")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReviewDiffPatch("link.patch", DiffPatchOptions{Overwrite: true}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink review error = %v", err)
	}

	f, err := os.Create(filepath.Join(r.dir, "large.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxReviewedPatch + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReviewDiffPatch("large.patch", DiffPatchOptions{Overwrite: true}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized review error = %v", err)
	}
}
