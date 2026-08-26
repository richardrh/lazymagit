package git

import (
	"context"
	"errors"
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
