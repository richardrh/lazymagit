package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestParseUnifiedDiffStructureAndRegion(t *testing.T) {
	patch := []byte("diff --git a/note.txt b/note.txt\n" +
		"index 1111111..2222222 100644\n--- a/note.txt\n+++ b/note.txt\n" +
		"@@ -1,5 +1,6 @@ title\n keep\n-old one\n+new one\n middle\n-old two\n+new two\n+extra\n end\n")
	doc, err := ParseUnifiedDiff(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Files) != 1 || doc.Files[0].OldPath != "note.txt" || doc.Files[0].NewPath != "note.txt" {
		t.Fatalf("file = %#v", doc.Files)
	}
	h := doc.Files[0].Hunks[0]
	if h.OldRange != (DiffRange{Start: 1, Count: 5}) || h.NewRange != (DiffRange{Start: 1, Count: 6}) {
		t.Fatalf("ranges = %#v %#v", h.OldRange, h.NewRange)
	}
	if h.Lines[1].OldLine != 2 || h.Lines[1].NewLine != 0 || h.Lines[2].NewLine != 2 {
		t.Fatalf("line positions = %#v %#v", h.Lines[1], h.Lines[2])
	}

	selected, err := doc.ChangedLineRegionPatch(0, 0, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := "@@ -1,5 +1,5 @@ title\n"
	if !strings.Contains(string(selected), wantHeader) {
		t.Fatalf("selected patch lacks corrected range %q:\n%s", wantHeader, selected)
	}
	if strings.Contains(string(selected), "+new two") || strings.Contains(string(selected), "+extra") {
		t.Fatalf("selected patch retained unselected additions:\n%s", selected)
	}
	if !strings.Contains(string(selected), " old two\n") {
		t.Fatalf("unselected deletion was not made context:\n%s", selected)
	}
	if _, err := ParseUnifiedDiff(selected); err != nil {
		t.Fatalf("reconstructed patch is invalid: %v\n%s", err, selected)
	}
}

func TestParseUnifiedDiffMetadataNoNewlineAndFailures(t *testing.T) {
	patch := []byte("diff --git \"a/é file\" \"b/é file\"\n" +
		"deleted file mode 100644\nindex 1111111..0000000\n--- \"a/é file\"\n+++ /dev/null\n" +
		"@@ -1 +0,0 @@\n-last\n\\ No newline at end of file\n")
	doc, err := ParseUnifiedDiff(patch)
	if err != nil {
		t.Fatal(err)
	}
	f := doc.Files[0]
	if f.OldPath != "é file" || f.NewPath != "" || !f.Deleted || !f.Hunks[0].Lines[0].NoNewline {
		t.Fatalf("metadata = %#v, line = %#v", f, f.Hunks[0].Lines[0])
	}

	bad := []string{
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n", // truncated hunk
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ nonsense\n",
		"not a patch\n",
		"diff --git a/x b/x\n--- a/x\n+++ b/x", // partial physical line
	}
	for _, input := range bad {
		if _, err := ParseUnifiedDiff([]byte(input)); !errors.Is(err, ErrMalformedChangePatch) {
			t.Errorf("ParseUnifiedDiff(%q) error = %v", input, err)
		}
	}
}

func TestParseDiffHunkDirectlyPreservesCoordinatesAndRejectsMalformedLines(t *testing.T) {
	lines := []string{
		"@@ -2,2 +4,3 @@ heading",
		" keep",
		"-old",
		"+new",
		"+extra",
		`\ No newline at end of file`,
		"next",
	}
	hunk, next, err := parseDiffHunk(lines, 0)
	if err != nil {
		t.Fatal(err)
	}
	if next != 6 || hunk.Heading != " heading" || len(hunk.Lines) != 4 {
		t.Fatalf("parsed hunk = %#v, next %d", hunk, next)
	}
	if got := hunk.Lines[0]; got.OldLine != 2 || got.NewLine != 4 {
		t.Fatalf("context coordinates = %#v", got)
	}
	if got := hunk.Lines[1]; got.OldLine != 3 || got.NewLine != 0 {
		t.Fatalf("deletion coordinates = %#v", got)
	}
	if got := hunk.Lines[3]; got.NewLine != 6 || !got.NoNewline {
		t.Fatalf("final addition = %#v", got)
	}

	bad := [][]string{
		{"not a header"},
		{"@@ -1 +1 @@", ""},
		{"@@ -1 +1 @@", "?bad"},
		{"@@ -0,0 +1,1 @@", " context"},
	}
	for _, input := range bad {
		if _, _, err := parseDiffHunk(input, 0); !errors.Is(err, ErrMalformedChangePatch) {
			t.Errorf("parseDiffHunk(%q) error = %v", input, err)
		}
	}
}

func TestCanonicalChangeSelectionsDirectlyMergesAndValidates(t *testing.T) {
	hunks := []DiffHunk{{Lines: make([]DiffLine, 6)}, {Lines: make([]DiffLine, 2)}}
	canonical, err := canonicalChangeSelections(hunks, []InteractiveChangeSelection{
		{Hunk: 0, Start: 3, End: 5},
		{Hunk: 1, Start: 0, End: 1},
		{Hunk: 0, Start: 1, End: 3},
		{Hunk: 1, WholeHunk: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := canonical[0]; len(got) != 1 || got[0].Start != 1 || got[0].End != 5 {
		t.Fatalf("merged selections = %#v", got)
	}
	if got := canonical[1]; len(got) != 1 || !got[0].WholeHunk {
		t.Fatalf("whole-hunk selection = %#v", got)
	}

	invalid := [][]InteractiveChangeSelection{
		nil,
		{{Hunk: -1, Start: 0, End: 1}},
		{{Hunk: 2, Start: 0, End: 1}},
		{{Hunk: 0, WholeHunk: true, End: 1}},
		{{Hunk: 0, Start: -1, End: 1}},
		{{Hunk: 0, Start: 1, End: 7}},
		{{Hunk: 0, Start: 1, End: 1}},
	}
	for _, selections := range invalid {
		if _, err := canonicalChangeSelections(hunks, selections); !errors.Is(err, ErrInvalidChangePatchRegion) {
			t.Errorf("canonicalChangeSelections(%#v) error = %v", selections, err)
		}
	}
}

func TestChangePatchExactRepositoryState(t *testing.T) {
	r := newTestRepo(t)
	r.write("-option é.txt", "one\ntwo\nthree\nfour\n")
	r.commitAll("base")
	r.write("-option é.txt", "one\nTWO\nthree\nFOUR\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := repo.LoadUnstagedDiffDocument(context.Background(), "-option é.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Files) != 1 || len(doc.Files[0].Hunks) != 1 {
		t.Fatalf("document = %#v", doc)
	}
	h := doc.Files[0].Hunks[0]
	start, end := changedBlockContaining(t, h, "TWO")
	partial, err := doc.ChangedLineRegionPatch(0, 0, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StageChangePatch(context.Background(), partial); err != nil {
		t.Fatal(err)
	}
	if got, want := r.git("show", ":-option é.txt"), "one\nTWO\nthree\nfour"; got != want {
		t.Fatalf("index after partial stage = %q, want %q", got, want)
	}
	if got, want := r.read("-option é.txt"), "one\nTWO\nthree\nFOUR\n"; got != want {
		t.Fatalf("worktree changed = %q, want %q", got, want)
	}

	staged, err := repo.LoadStagedDiffDocument(context.Background(), "-option é.txt")
	if err != nil {
		t.Fatal(err)
	}
	whole, err := staged.HunkPatch(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UnstageChangePatch(context.Background(), whole); err != nil {
		t.Fatal(err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index still changed: %q", got)
	}

	unstaged, err := repo.LoadUnstagedDiffDocument(context.Background(), "-option é.txt")
	if err != nil {
		t.Fatal(err)
	}
	all, err := unstaged.HunkPatch(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ReverseWorktreeChangePatch(context.Background(), all); err != nil {
		t.Fatal(err)
	}
	if got, want := r.read("-option é.txt"), "one\ntwo\nthree\nfour\n"; got != want {
		t.Fatalf("worktree after reverse = %q, want %q", got, want)
	}
	if err := repo.ApplyWorktreeChangePatch(context.Background(), all); err != nil {
		t.Fatal(err)
	}
	if got, want := r.read("-option é.txt"), "one\nTWO\nthree\nFOUR\n"; got != want {
		t.Fatalf("worktree after apply = %q, want %q", got, want)
	}
}

func TestChangePatchAddDeleteAndUnsupported(t *testing.T) {
	r := newTestRepo(t)
	r.write("gone", "old")
	r.commitAll("base")
	r.write("new", "new")
	if err := r.gitRemove("gone"); err != nil {
		t.Fatal(err)
	}
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.LoadUnstagedDiffDocument(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	// Untracked files are not part of git diff; stage the deletion and add the
	// new file independently to exercise /dev/null in both directions.
	if len(doc.Files) != 1 || !doc.Files[0].Deleted {
		t.Fatalf("deletion metadata = %#v", doc.Files)
	}
	deletion, _ := doc.HunkPatch(0, 0)
	if err := repo.StageChangePatch(context.Background(), deletion); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--", "new")
	staged, err := repo.LoadStagedDiffDocument(context.Background(), "new")
	if err != nil || len(staged.Files) != 1 || !staged.Files[0].NewFile {
		t.Fatalf("addition metadata = %#v, err %v", staged, err)
	}

	rename := []byte("diff --git a/a b/b\nsimilarity index 100%\nrename from a\nrename to b\n")
	parsed, err := ParseUnifiedDiff(rename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parsed.HunkPatch(0, 0); !errors.Is(err, ErrInvalidChangePatchRegion) {
		t.Fatalf("rename hunk error = %v", err)
	}
	if err := repo.ApplyChangePatch(context.Background(), rename, ChangePatchStage); !errors.Is(err, ErrUnsupportedChangePatch) {
		t.Fatalf("rename apply error = %v", err)
	}
	binary := []byte("diff --git a/x b/x\nindex 1..2 100644\nBinary files a/x and b/x differ\n")
	if err := repo.ApplyChangePatch(context.Background(), binary, ChangePatchStage); !errors.Is(err, ErrUnsupportedChangePatch) {
		t.Fatalf("binary apply error = %v", err)
	}
}

func TestInteractiveChangeReviewStageUnstageDiscardAndStale(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	baseLines := make([]string, 120)
	for i := range baseLines {
		baseLines[i] = fmt.Sprintf("line %03d", i+1)
	}
	r.write("file.txt", strings.Join(baseLines, "\n")+"\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	changedLines := append([]string(nil), baseLines...)
	changedLines[4] = "FIRST CHANGE"
	changedLines[94] = "SECOND CHANGE"
	r.write("file.txt", strings.Join(changedLines, "\n")+"\n")
	doc, err := repo.LoadUnstagedDiffDocument(ctx, "file.txt")
	if err != nil || len(doc.Files) != 1 || len(doc.Files[0].Hunks) != 2 {
		t.Fatalf("unstaged document = %#v, %v", doc, err)
	}
	filePatch, err := doc.FilePatch(0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(filePatch), "diff --git ") != 1 || strings.Count(string(filePatch), "@@ -") != 2 {
		t.Fatalf("file patch did not retain one header and two hunks:\n%s", filePatch)
	}

	stageRequest := InteractiveChangeRequest{Action: InteractiveChangeStage, Scope: InteractiveChangeHunk, Path: "file.txt", Hunk: 0}
	stageReview, err := repo.ReviewInteractiveChange(ctx, stageRequest)
	if err != nil || stageReview.ChangedLines != 2 || stageReview.PatchHash == "" {
		t.Fatalf("stage review = %#v, %v", stageReview, err)
	}
	var records []ProcessRecord
	recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	if err := repo.ExecuteReviewedInteractiveChange(recorded, stageReview); err != nil {
		t.Fatal(err)
	}
	assertRecordedArgs(t, records, []string{"apply", "--cached", "--whitespace=nowarn", "-"})
	wantIndex := append([]string(nil), baseLines...)
	wantIndex[4] = "FIRST CHANGE"
	if got, want := r.git("show", ":file.txt"), strings.Join(wantIndex, "\n"); got != want {
		t.Fatalf("index after hunk stage = %q, want %q", got, want)
	}
	if got := r.read("file.txt"); !strings.Contains(got, "SECOND CHANGE") {
		t.Fatalf("hunk stage changed remaining worktree hunk: %q", got)
	}

	unstageReview, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeUnstage, Scope: InteractiveChangeHunk, Path: "file.txt", Hunk: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, unstageReview); err != nil {
		t.Fatal(err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index still changed after reviewed unstage: %q", got)
	}

	discardReview, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeDiscardUnstaged, Scope: InteractiveChangeHunk, Path: "file.txt", Hunk: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, discardReview); err != nil {
		t.Fatal(err)
	}
	if got := r.read("file.txt"); strings.Contains(got, "FIRST CHANGE") || !strings.Contains(got, "SECOND CHANGE") {
		t.Fatalf("reviewed discard targeted wrong hunk: %q", got)
	}

	stale, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeStage, Scope: InteractiveChangeHunk, Path: "file.txt", Hunk: 0})
	if err != nil {
		t.Fatal(err)
	}
	r.write("file.txt", strings.Replace(r.read("file.txt"), "SECOND CHANGE", "CHANGED AGAIN", 1))
	beforeIndex := r.git("show", ":file.txt")
	if err := repo.ExecuteReviewedInteractiveChange(ctx, stale); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale reviewed change = %v", err)
	}
	if got := r.git("show", ":file.txt"); got != beforeIndex {
		t.Fatalf("stale reviewed change mutated index: %q", got)
	}
}

func TestInteractiveChangeLineRangeAndStagedDiscard(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "one\ntwo\nthree\nfour\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	r.write("file.txt", "one\nTWO\nthree\nFOUR\n")

	doc, err := repo.LoadUnstagedDiffDocument(ctx, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	start, end := changedBlockContaining(t, doc.Files[0].Hunks[0], "TWO")
	request := InteractiveChangeRequest{Action: InteractiveChangeStage, Scope: InteractiveChangeLines, Path: "file.txt", Hunk: 0, Start: start, End: end}
	review, err := repo.ReviewInteractiveChange(ctx, request)
	if err != nil || review.ChangedLines == 0 {
		t.Fatalf("line review = %#v, %v", review, err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, review); err != nil {
		t.Fatal(err)
	}
	if got, want := r.git("show", ":file.txt"), "one\nTWO\nthree\nfour"; got != want {
		t.Fatalf("line-range stage = %q, want %q", got, want)
	}

	// Remove the remaining unstaged hunk so index and worktree match; staged
	// discard can then update both atomically with git apply --index --reverse.
	remaining, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeDiscardUnstaged, Scope: InteractiveChangeHunk, Path: "file.txt", Hunk: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, remaining); err != nil {
		t.Fatal(err)
	}
	if got := r.git("diff", "--name-only"); got != "" {
		t.Fatalf("remaining unstaged discard left worktree diff: %q\nworktree=%q\nindex=%q", got, r.read("file.txt"), r.git("show", ":file.txt"))
	}
	stagedDiscard, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeDiscardStaged, Scope: InteractiveChangeHunk, Path: "file.txt", Hunk: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, stagedDiscard); err != nil {
		t.Fatal(err)
	}
	if got := r.read("file.txt"); got != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("staged discard worktree = %q", got)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("staged discard left index changes: %q", got)
	}
}

func TestChangePatchNoFinalNewlineAndContextRefusal(t *testing.T) {
	r := newTestRepo(t)
	r.write("no-newline", "first\nlast")
	r.write("context", "one\ntwo\nthree\n")
	r.commitAll("base")
	r.write("no-newline", "first\nLAST")
	r.write("context", "one\nTWO\nthree\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := repo.LoadUnstagedDiffDocument(context.Background(), "no-newline")
	if err != nil {
		t.Fatal(err)
	}
	noNewlineMarkers := 0
	for _, line := range doc.Files[0].Hunks[0].Lines {
		if line.NoNewline {
			noNewlineMarkers++
		}
	}
	if noNewlineMarkers != 2 {
		t.Fatalf("no-final-newline markers = %d, want 2", noNewlineMarkers)
	}
	patch, err := doc.HunkPatch(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StageChangePatch(context.Background(), patch); err != nil {
		t.Fatal(err)
	}
	if got, want := r.git("rev-parse", ":no-newline"), r.git("hash-object", "no-newline"); got != want {
		t.Fatalf("index blob = %s, worktree blob = %s", got, want)
	}

	stale, err := repo.LoadUnstagedDiffDocument(context.Background(), "context")
	if err != nil {
		t.Fatal(err)
	}
	stalePatch, err := stale.HunkPatch(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	r.write("context", "different context\nTWO\nthree\n")
	before := r.read("context")
	if err := repo.ReverseWorktreeChangePatch(context.Background(), stalePatch); err == nil {
		t.Fatal("reverse with stale context unexpectedly succeeded")
	}
	if got := r.read("context"); got != before {
		t.Fatalf("failed apply partially mutated worktree: got %q, want %q", got, before)
	}
}

func TestInteractiveChangeMultiSelectionsStageUnstageAndDiscard(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	base := make([]string, 160)
	for i := range base {
		base[i] = fmt.Sprintf("line %03d", i+1)
	}
	r.write("file.txt", strings.Join(base, "\n")+"\n")
	r.commitAll("base")
	changed := append([]string(nil), base...)
	changed[4], changed[74], changed[144] = "FIRST", "SECOND", "THIRD"
	r.write("file.txt", strings.Join(changed, "\n")+"\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	// The first and third hunks are deliberately noncontiguous. One reviewed
	// patch carries both hunks under a single file header.
	request := InteractiveChangeRequest{Action: InteractiveChangeStage, Scope: InteractiveChangeSelections, Path: "file.txt", Selections: []InteractiveChangeSelection{
		{Hunk: 2, WholeHunk: true}, {Hunk: 0, WholeHunk: true},
	}}
	review, err := repo.ReviewInteractiveChange(ctx, request)
	if err != nil || len(review.HunkHeadings) != 2 || review.ChangedLines != 4 {
		t.Fatalf("multi-hunk review = %#v, %v", review, err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, review); err != nil {
		t.Fatal(err)
	}
	index := r.git("show", ":file.txt")
	if !strings.Contains(index, "FIRST") || strings.Contains(index, "SECOND") || !strings.Contains(index, "THIRD") {
		t.Fatalf("multi-hunk stage index:\n%s", index)
	}

	unstage, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeUnstage, Scope: InteractiveChangeSelections, Path: "file.txt", Selections: []InteractiveChangeSelection{
		{Hunk: 1, WholeHunk: true}, {Hunk: 0, WholeHunk: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, unstage); err != nil {
		t.Fatal(err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("multi-hunk unstage left index changes: %q", got)
	}

	discard, err := repo.ReviewInteractiveChange(ctx, InteractiveChangeRequest{Action: InteractiveChangeDiscardUnstaged, Scope: InteractiveChangeSelections, Path: "file.txt", Selections: []InteractiveChangeSelection{
		{Hunk: 2, WholeHunk: true}, {Hunk: 0, WholeHunk: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedInteractiveChange(ctx, discard); err != nil {
		t.Fatal(err)
	}
	worktree := r.read("file.txt")
	if strings.Contains(worktree, "FIRST") || !strings.Contains(worktree, "SECOND") || strings.Contains(worktree, "THIRD") {
		t.Fatalf("multi-hunk discard worktree:\n%s", worktree)
	}
}

func TestInteractiveChangeRejectsUnresolvedConflict(t *testing.T) {
	r := newTestRepo(t)
	r.write("conflict.txt", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("conflict.txt", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("conflict.txt", "main\n")
	r.commitAll("main")
	merge := exec.Command("git", "-C", r.dir, "merge", "topic")
	if out, err := merge.CombinedOutput(); err == nil {
		t.Fatalf("conflicting merge unexpectedly succeeded: %s", out)
	}
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ReviewInteractiveChange(context.Background(), InteractiveChangeRequest{
		Action: InteractiveChangeStage, Scope: InteractiveChangeHunk, Path: "conflict.txt", Hunk: 0,
	})
	if !errors.Is(err, ErrInteractiveChangeConflict) {
		t.Fatalf("conflict review error = %v", err)
	}
}

func TestChangedLineSelectionsPatchSupportsDisjointRegions(t *testing.T) {
	r := newTestRepo(t)
	r.write("file.txt", "one\ntwo\nthree\nfour\nfive\n")
	r.commitAll("base")
	r.write("file.txt", "one\nTWO\nthree\nFOUR\nfive\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.LoadUnstagedDiffDocument(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	hunk := doc.Files[0].Hunks[0]
	firstStart, firstEnd := changedBlockContaining(t, hunk, "TWO")
	secondStart, secondEnd := changedBlockContaining(t, hunk, "FOUR")
	patch, err := doc.ChangedLineSelectionsPatch(0, []InteractiveChangeSelection{
		{Hunk: 0, Start: secondStart, End: secondEnd}, {Hunk: 0, Start: firstStart, End: firstEnd},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(patch), "@@ -") != 1 {
		t.Fatalf("disjoint patch was not one refined hunk:\n%s", patch)
	}
	if err := repo.StageChangePatch(context.Background(), patch); err != nil {
		t.Fatal(err)
	}
	if got, want := r.git("show", ":file.txt"), "one\nTWO\nthree\nFOUR\nfive"; got != want {
		t.Fatalf("disjoint regions stage = %q, want %q", got, want)
	}
}

func changedBlockContaining(t *testing.T, h DiffHunk, text string) (int, int) {
	t.Helper()
	for i, line := range h.Lines {
		if line.Text != text || (line.Kind != DiffLineAdded && line.Kind != DiffLineDeleted) {
			continue
		}
		start, end := i, i+1
		for start > 0 && h.Lines[start-1].Kind != DiffLineContext {
			start--
		}
		for end < len(h.Lines) && h.Lines[end].Kind != DiffLineContext {
			end++
		}
		return start, end
	}
	t.Fatalf("changed line %q not found", text)
	return 0, 0
}

func (r *testRepo) gitRemove(path string) error {
	r.t.Helper()
	r.git("rm", "--", path)
	// Put the deletion back in the worktree state rather than staged state.
	r.git("reset", "HEAD", "--", path)
	return nil
}
