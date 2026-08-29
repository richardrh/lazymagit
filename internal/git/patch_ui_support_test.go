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

func TestReviewedApplyPatchRejectsChangedPatchAndRepository(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	r.commitAll("base")
	r.write("file.txt", "patched\n")
	r.git("diff", "--binary", "--full-index", "--output=change.patch")
	r.git("checkout", "--", "file.txt")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	review, err := repo.ReviewApplyPatch(ctx, "change.patch", ApplyPatchOptions{})
	if err != nil {
		t.Fatalf("ReviewApplyPatch: %v", err)
	}
	r.write("change.patch", "not a patch\n")
	if err := repo.ExecuteReviewedApplyPatch(ctx, review); err == nil {
		t.Fatal("changed reviewed patch was applied")
	}

	// Restoring the real patch permits review, but a worktree change after that
	// review is checked again before Git is allowed to mutate it.
	r.git("diff", "--binary", "--full-index", "--output=change.patch", "HEAD")
	// The previous checkout made HEAD clean; recreate the known patch bytes.
	r.write("file.txt", "patched\n")
	r.git("diff", "--binary", "--full-index", "--output=change.patch")
	r.git("checkout", "--", "file.txt")
	review, err = repo.ReviewApplyPatch(ctx, "change.patch", ApplyPatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.write("file.txt", "different\n")
	if err := repo.ExecuteReviewedApplyPatch(ctx, review); err == nil {
		t.Fatal("repository change after review was applied")
	}
	r.git("checkout", "--", "file.txt")
	review, err = repo.ReviewApplyPatch(ctx, "change.patch", ApplyPatchOptions{Index: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedApplyPatch(ctx, review); err != nil {
		t.Fatalf("reviewed apply: %v", err)
	}
	if got := r.git("show", ":file.txt"); got != "patched" {
		t.Fatalf("reviewed apply index = %q", got)
	}
}

func TestReviewedAMStartRejectsChangedMailBeforeCreatingCommits(t *testing.T) {
	ctx := context.Background()
	source := newTestRepo(t)
	source.write("file.txt", "base\n")
	base := source.commitAll("base")
	source.write("file.txt", "from mail\n")
	source.commitAll("mail change")
	sourceRepo, err := Discover(source.dir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(source.dir, "mail")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	patches, err := sourceRepo.FormatPatch(ctx, base+"..HEAD", FormatPatchOptions{OutputDirectory: out})
	if err != nil || len(patches) != 1 {
		t.Fatalf("FormatPatch = %#v, %v", patches, err)
	}

	target := newTestRepo(t)
	target.write("file.txt", "base\n")
	target.commitAll("base")
	targetRepo, err := Discover(target.dir)
	if err != nil {
		t.Fatal(err)
	}
	review, err := targetRepo.ReviewAMStart(patches, AMOptions{Signoff: true})
	if err != nil {
		t.Fatal(err)
	}
	before := target.git("rev-parse", "HEAD")
	data, err := os.ReadFile(patches[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patches[0], append(data, []byte("\nreview changed\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := targetRepo.ExecuteReviewedAMStart(ctx, review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale reviewed am = %v", err)
	}
	if got := target.git("rev-parse", "HEAD"); got != before {
		t.Fatalf("stale am changed HEAD from %q to %q", before, got)
	}
}

func TestReviewedFormatPatchPublishesExactNewFilesAndRejectsStaleDirectory(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	base := r.commitAll("base")
	r.write("topic.txt", "topic\n")
	r.commitAll("topic")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(r.dir, "patches")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}

	review, err := repo.ReviewFormatPatchUI(ctx, base+"..HEAD", FormatPatchOptions{OutputDirectory: out, RerollCount: 2, To: []string{"dev@example.test"}, Cc: []string{"review@example.test"}})
	if err != nil || len(review.Files) != 1 {
		t.Fatalf("ReviewFormatPatchUI = %#v, %v", review, err)
	}
	if _, err := os.Stat(filepath.Join(out, review.Files[0].Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("review published output early: %v", err)
	}
	if err := os.WriteFile(filepath.Join(out, "arrived-after-review"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedFormatPatch(ctx, review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale format-patch execution = %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, review.Files[0].Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale execution published output: %v", err)
	}
	if err := os.Remove(filepath.Join(out, "arrived-after-review")); err != nil {
		t.Fatal(err)
	}

	review, err = repo.ReviewFormatPatchUI(ctx, base+"..HEAD", FormatPatchOptions{OutputDirectory: out, RerollCount: 2, To: []string{"dev@example.test"}, Cc: []string{"review@example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedFormatPatch(ctx, review); err != nil {
		t.Fatalf("ExecuteReviewedFormatPatch: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, review.Files[0].Name))
	if err != nil || !strings.Contains(string(data), "Subject: [PATCH v2]") || !strings.Contains(string(data), "To: dev@example.test") || !strings.Contains(string(data), "Cc: review@example.test") {
		t.Fatalf("published reviewed patch = %q, %v", data, err)
	}
}

func TestReviewedFormatPatchMailMetadataAndEditableCoverLetter(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	base := r.commitAll("base")
	r.write("topic.txt", "topic\n")
	r.commitAll("topic")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(r.dir, "patches")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	options := FormatPatchOptions{
		OutputDirectory: out,
		CoverLetterBody: "A terminal-edited cover letter.\n\nIt never opens an external editor.",
		RFC:             true,
		ThreadStyle:     "deep",
		From:            "Series Author <author@example.test>",
		InReplyTo:       "<previous-series@example.test>",
		Base:            base,
		To:              []string{"list@example.test"},
		Cc:              []string{"review@example.test"},
	}
	review, err := repo.ReviewFormatPatchUI(ctx, base+"..HEAD", options)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Files) != 2 { // a cover letter plus one patch
		t.Fatalf("review files = %#v", review.Files)
	}
	// Every mail argument and the terminal-edited body are bound to the review
	// token, not merely represented by the staged file digest.
	changed := review
	changed.Options.CoverLetterBody = "changed after review"
	if err := repo.ExecuteReviewedFormatPatch(ctx, changed); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("changed metadata execution = %v", err)
	}
	if entries, err := os.ReadDir(out); err != nil || len(entries) != 0 {
		t.Fatalf("stale metadata published output: %v, %v", entries, err)
	}
	review, err = repo.ReviewFormatPatchUI(ctx, base+"..HEAD", options)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedFormatPatch(ctx, review); err != nil {
		t.Fatal(err)
	}
	var cover, patch string
	for _, file := range review.Files {
		data, err := os.ReadFile(filepath.Join(out, file.Name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "A terminal-edited cover letter.") {
			cover = string(data)
		} else {
			patch = string(data)
		}
	}
	for _, want := range []string{"A terminal-edited cover letter.", "It never opens an external editor.", "From: Series Author <author@example.test>", "In-Reply-To: <previous-series@example.test>", "To: list@example.test", "Cc: review@example.test", "Subject: [RFC PATCH 0/1]", "base-commit: " + base} {
		if !strings.Contains(cover, want) {
			t.Errorf("cover letter lacks %q:\n%s", want, cover)
		}
	}
	for _, want := range []string{"From: Series Author <author@example.test>", "References: <previous-series@example.test>"} {
		if !strings.Contains(patch, want) {
			t.Errorf("patch lacks %q:\n%s", want, patch)
		}
	}
}

func TestFormatPatchMailOptionsHaveDistinctReviewIdentities(t *testing.T) {
	options := FormatPatchOptions{
		ThreadStyle: "deep", RFC: true, From: "author@example.test", InReplyTo: "<series@example.test>", Base: "HEAD~1", To: []string{"to@example.test"}, Cc: []string{"cc@example.test"}, CoverLetterBody: "overview\n",
	}
	token := NewConfirmationToken(formatPatchOptionsIdentity(options))
	variants := []FormatPatchOptions{
		func() FormatPatchOptions { out := options; out.ThreadStyle = "shallow"; return out }(),
		func() FormatPatchOptions { out := options; out.RFC = false; return out }(),
		func() FormatPatchOptions { out := options; out.From = "other@example.test"; return out }(),
		func() FormatPatchOptions { out := options; out.InReplyTo = "<other@example.test>"; return out }(),
		func() FormatPatchOptions { out := options; out.Base = "HEAD~2"; return out }(),
		func() FormatPatchOptions {
			out := options
			out.To = []string{"to@example.test", "extra@example.test"}
			return out
		}(),
		func() FormatPatchOptions { out := options; out.Cc = nil; return out }(),
		func() FormatPatchOptions { out := options; out.CoverLetterBody = "changed"; return out }(),
	}
	for _, variant := range variants {
		if token.validFor(formatPatchOptionsIdentity(variant)) {
			t.Errorf("changed mail option retained its confirmation identity: %#v", variant)
		}
	}
	if formatPatchOptionsIdentity(FormatPatchOptions{To: []string{"a", "b"}}) == formatPatchOptionsIdentity(FormatPatchOptions{To: []string{"a"}, Cc: []string{"b"}}) {
		t.Fatal("recipient-list boundaries collide in format-patch review identity")
	}
}

func TestFormatPatchRejectsUnboundedOrInjectedMailHeaders(t *testing.T) {
	invalid := []FormatPatchOptions{
		{OutputDirectory: ".", From: "sender@example.test\r\nBcc: injected@example.test"},
		{OutputDirectory: ".", To: []string{"not an address"}},
		{OutputDirectory: ".", InReplyTo: "previous@example.test"},
		{OutputDirectory: ".", CoverLetterBody: strings.Repeat("x", maxFormatPatchCoverBody+1)},
		{OutputDirectory: ".", ThreadStyle: "arbitrary"},
	}
	for _, options := range invalid {
		if err := validateFormatPatchOptions(options); err == nil {
			t.Errorf("invalid mail options accepted: %#v", options)
		}
	}
}

func TestEditableCoverLetterRefusesSymlinkOutput(t *testing.T) {
	dir := t.TempDir()
	external := filepath.Join(dir, "external")
	if err := os.WriteFile(external, []byte("*** BLURB HERE ***"), 0o600); err != nil {
		t.Fatal(err)
	}
	cover := filepath.Join(dir, "0000-cover-letter.patch")
	if err := os.Symlink(external, cover); err != nil {
		t.Fatal(err)
	}
	if err := replaceFormatPatchCoverLetter([]string{cover}, "must not be written"); err == nil {
		t.Fatal("symlink cover letter was accepted")
	}
	if got, err := os.ReadFile(external); err != nil || string(got) != "*** BLURB HERE ***" {
		t.Fatalf("symlink replacement wrote external file: %q, %v", got, err)
	}
}

func TestReviewFormatPatchRejectsInvalidOrEmptyInputBeforePublication(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(r.dir, "patches")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, input := range []struct {
		name, revision, directory string
	}{
		{"unsafe revision", "-HEAD", out},
		{"empty range", "HEAD..HEAD", out},
		{"output is a file", "HEAD", filepath.Join(r.dir, "base.txt")},
	} {
		t.Run(input.name, func(t *testing.T) {
			review, err := repo.ReviewFormatPatchUI(ctx, input.revision, FormatPatchOptions{OutputDirectory: input.directory})
			if err == nil {
				repo.DiscardReviewedFormatPatch(review)
				t.Fatal("ReviewFormatPatchUI unexpectedly succeeded")
			}
		})
	}
	if _, err := repo.ReviewFormatPatchUI(ctx, "HEAD", FormatPatchOptions{OutputDirectory: out, RerollCount: -1}); err == nil {
		t.Fatal("negative format-patch option was accepted")
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed review left output behind: entries=%v err=%v", entries, err)
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
