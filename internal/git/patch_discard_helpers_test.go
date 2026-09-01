package git

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFormatPatchExtractionHelpers(t *testing.T) {
	options := FormatPatchOptions{Numbered: true, CoverLetter: true, Signoff: true, ThreadStyle: "deep", RFC: true, SubjectPrefix: "PATCH v2", RerollCount: 2, StartNumber: 3, From: "A <a@example.com>", InReplyTo: "<id@example.com>", Base: "HEAD~2", To: []string{"B <b@example.com>"}, Cc: []string{"C <c@example.com>"}}
	args := formatPatchArgs("out", "HEAD~2..HEAD", options)
	joined := strings.Join(args, " ")
	for _, value := range []string{"--numbered", "--cover-letter", "--signoff", "--thread=deep", "--rfc", "--subject-prefix=PATCH v2", "--reroll-count=2", "--start-number=3", "--from=A <a@example.com>", "--in-reply-to=<id@example.com>", "--base=HEAD~2", "--to=B <b@example.com>", "--cc=C <c@example.com>", "HEAD~2..HEAD"} {
		if !strings.Contains(joined, value) {
			t.Errorf("formatPatchArgs %q omit %q", joined, value)
		}
	}
	dir := t.TempDir()
	before := map[string]bool{"existing.patch": true}
	for name, contents := range map[string]string{"existing.patch": "old", "0001-new.patch": "new", "notes.txt": "ignore"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	created, err := newFormatPatchFiles(dir, before)
	if err != nil || !reflect.DeepEqual(created, []string{filepath.Join(dir, "0001-new.patch")}) {
		t.Fatalf("newFormatPatchFiles = %#v, %v", created, err)
	}
	if err := os.Remove(filepath.Join(dir, "0001-new.patch")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("notes.txt", filepath.Join(dir, "0002-link.patch")); err != nil {
		t.Fatal(err)
	}
	if _, err := newFormatPatchFiles(dir, before); err == nil {
		t.Fatal("non-regular format patch output accepted")
	}
}

func TestDiscardPlanningHelpers(t *testing.T) {
	repo := &Repository{workTree: t.TempDir()}
	status := Status{Files: []FileStatus{
		{Path: "tracked", Unstaged: ChangeModified},
		{Path: "untracked", Unstaged: ChangeUntracked},
		{Path: "added", Staged: ChangeAdded},
		{Path: "new", OriginalPath: "old", Staged: ChangeRenamed},
		{Path: "ignored", Unstaged: ChangeModified},
	}}
	plan, err := repo.planDiscard(status, []string{"tracked", "untracked", "added", "old"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.tracked, []string{"tracked"}) || !reflect.DeepEqual(plan.untracked, []string{"untracked"}) ||
		!reflect.DeepEqual(plan.stagedAdded, []string{"added"}) || !reflect.DeepEqual(plan.stagedTracked, []string{"old", "new"}) {
		t.Fatalf("discard plan = %#v", plan)
	}
	if !supportedDiscardState(ChangeModified) || supportedDiscardState(Change(255)) {
		t.Fatal("supportedDiscardState classified states incorrectly")
	}
	if got := conflictSubject("old", "rename source %s was recreated"); got != "rename source old" {
		t.Fatalf("conflictSubject = %q", got)
	}
}

func TestValidateDiscardFileRejectsUnsafeStates(t *testing.T) {
	dir := t.TempDir()
	repo := &Repository{workTree: dir}
	if err := os.WriteFile(filepath.Join(dir, "deleted"), []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.validateDiscardFile(FileStatus{Path: "deleted", Unstaged: ChangeDeleted}); !errors.Is(err, ErrMixedState) {
		t.Fatalf("replacement error = %v", err)
	}
	if err := repo.validateDiscardFile(FileStatus{Path: "mixed", Staged: ChangeModified, Unstaged: ChangeModified}); !errors.Is(err, ErrMixedState) {
		t.Fatalf("mixed error = %v", err)
	}
	if err := repo.validateDiscardFile(FileStatus{Path: "bad", Staged: Change(255)}); !errors.Is(err, ErrUnsupportedStagedState) {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestDiffPatchHelpers(t *testing.T) {
	dir := t.TempDir()
	repo := &Repository{commandDir: dir}
	path, err := repo.diffPatchOutputPath("out.patch")
	if err != nil || path != filepath.Join(dir, "out.patch") {
		t.Fatalf("diffPatchOutputPath = %q, %v", path, err)
	}
	if _, err := repo.diffPatchOutputPath(""); err == nil {
		t.Fatal("empty output path accepted")
	}
	args := diffPatchArgs(DiffPatchOptions{Cached: true, Range: "HEAD^", Paths: []string{"a b"}})
	wantSuffix := []string{"--cached", "HEAD^", "--", "a b"}
	if !reflect.DeepEqual(args[len(args)-len(wantSuffix):], wantSuffix) {
		t.Fatalf("diffPatchArgs = %#v", args)
	}
	for _, overwrite := range []bool{false, true} {
		tmp := filepath.Join(dir, "tmp")
		destination := filepath.Join(dir, "installed")
		_ = os.Remove(destination)
		if err := os.WriteFile(tmp, []byte("patch"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := installDiffPatch(tmp, destination, "installed", overwrite); err != nil {
			t.Fatalf("installDiffPatch(overwrite=%t): %v", overwrite, err)
		}
		if data, err := os.ReadFile(destination); err != nil || string(data) != "patch" {
			t.Fatalf("installed patch = %q, %v", data, err)
		}
	}
}

func TestPatchSelectionSupportsFileHunkLineAndExplicitSelections(t *testing.T) {
	doc, err := ParseUnifiedDiff([]byte("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@ heading\n-old\n+new\n"))
	if err != nil {
		t.Fatal(err)
	}
	file := &doc.Files[0]
	for _, request := range []InteractiveChangeRequest{{Scope: InteractiveChangeFile}, {Scope: InteractiveChangeHunk, Hunk: 0}, {Scope: InteractiveChangeLines, Hunk: 0, Start: 0, End: 2}, {Scope: InteractiveChangeSelections, Selections: []InteractiveChangeSelection{{Hunk: 0, WholeHunk: true}}}} {
		patch, headings, changed, err := selectedPatchFromDocument(doc, file, request)
		if err != nil || len(patch) == 0 || len(headings) != 1 || changed != 2 {
			t.Fatalf("selection %#v = %q, %#v, %d, %v", request, patch, headings, changed, err)
		}
	}
	if _, _, _, err := selectedPatchFromDocument(doc, file, InteractiveChangeRequest{Scope: 99}); err == nil {
		t.Fatal("unknown scope accepted")
	}
	if got := selectedHunkChangeCount(file.Hunks[0], []InteractiveChangeSelection{{Start: 0, End: 1}}); got != 1 {
		t.Fatalf("region count = %d", got)
	}
}
