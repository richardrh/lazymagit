package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStage2QueryHelpers(t *testing.T) {
	if _, _, err := cherryLimit(-1); err == nil {
		t.Fatal("negative cherry limit accepted")
	}
	if n, truncated, err := cherryLimit(0); err != nil || n != 1000 || truncated {
		t.Fatalf("cherryLimit default = %d, %t, %v", n, truncated, err)
	}
	if n, truncated, err := cherryLimit(inspectionItemLimit + 1); err != nil || n != inspectionItemLimit || !truncated {
		t.Fatalf("cherryLimit cap = %d, %t, %v", n, truncated, err)
	}
	if defaultCherryHead("") != "HEAD" || defaultCherryHead("topic") != "topic" {
		t.Fatal("defaultCherryHead")
	}
	if _, err := cherryOutputLimit(-1); err == nil {
		t.Fatal("negative cherry output accepted")
	}
	if n, err := cherryOutputLimit(0); err != nil || n != inspectionOutputLimit {
		t.Fatalf("cherry output default = %d, %v", n, err)
	}
	oid := strings.Repeat("a", 40)
	items, err := parseCherryOutput([]byte("+ "+oid+" subject\n- "+oid+"\n"), false)
	if err != nil || len(items) != 2 || items[0].Equivalent || !items[1].Equivalent {
		t.Fatalf("parseCherryOutput = %#v, %v", items, err)
	}
	if _, err := parseCherryOutput([]byte("bad\n"), false); err == nil {
		t.Fatal("malformed cherry accepted")
	}

	if _, _, err := reflogLimit(-1); err == nil {
		t.Fatal("negative reflog limit accepted")
	}
	if n, truncated, err := reflogLimit(0); err != nil || n != 256 || truncated {
		t.Fatalf("reflog default = %d, %t, %v", n, truncated, err)
	}
	if n, truncated, err := reflogLimit(inspectionItemLimit + 1); err != nil || n != inspectionItemLimit || !truncated {
		t.Fatalf("reflog cap = %d, %t, %v", n, truncated, err)
	}
	if _, err := reflogOutputLimit(inspectionOutputLimit + 1); err == nil {
		t.Fatal("large reflog output accepted")
	}
	if got := reflogArgs(true, "", 2); got[len(got)-2] != "--all" {
		t.Fatalf("all args = %#v", got)
	}
	if got := reflogArgs(false, "HEAD", 2); got[len(got)-2] != "HEAD" {
		t.Fatalf("ref args = %#v", got)
	}
}

func TestStage2CommitAndRebaseHelpers(t *testing.T) {
	if err := validateCommitArgsOptions("test", CommitOptions{ReuseMessage: "a", ReeditMessage: "b"}); err == nil {
		t.Fatal("combined messages accepted")
	}
	if err := validateCommitArgsOptions("test", CommitOptions{Sign: true}); !errors.Is(err, ErrInteractiveRequired) {
		t.Fatalf("signing error = %v", err)
	}
	opts := CommitOptions{All: true, AllowEmpty: true, NoVerify: true, ResetAuthor: true, Signoff: true, Author: "A", Date: "now", Sign: true, AllowInteractiveSigning: true}
	args := appendCommitBooleanArgs([]string{"commit"}, opts)
	args = appendCommitValueArgs(args, opts)
	args = appendCommitSigningArg(args, opts)
	for _, want := range []string{"--all", "--allow-empty", "--no-verify", "--reset-author", "--signoff", "--author=A", "--date=now", "--gpg-sign"} {
		if !containsString(args, want) {
			t.Fatalf("%q missing from %#v", want, args)
		}
	}
	if got := appendCommitSigningArg(nil, CommitOptions{SigningKey: "key"}); !reflect.DeepEqual(got, []string{"--gpg-sign=key"}) {
		t.Fatalf("key signing = %#v", got)
	}

	if err := validateInteractiveRebaseOptions(RebaseOptions{}); err == nil {
		t.Fatal("empty upstream accepted")
	}
	if err := validateInteractiveRebaseOptions(RebaseOptions{Upstream: "main", RebaseMerges: true}); err == nil {
		t.Fatal("merge topology accepted")
	}
	rargs := interactiveRebaseArgs(RebaseOptions{KeepEmpty: true, UpdateRefs: true, Autostash: true, ForceRebase: true, Strategy: "ort", Signoff: true})
	for _, want := range []string{"--keep-empty", "--update-refs", "--autostash", "--force-rebase", "--strategy=ort", "--signoff"} {
		if !containsString(rargs, want) {
			t.Fatalf("%q missing from %#v", want, rargs)
		}
	}
}

func TestStage2PatchSelectionHelpers(t *testing.T) {
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

func TestStage2HistoryStateHelpers(t *testing.T) {
	if got := nonEmptyNULFields([]byte("b\x00\x00a\x00")); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("fields = %#v", got)
	}
	dir := t.TempDir()
	repo := &Repository{workTree: dir}
	if err := os.WriteFile(filepath.Join(dir, "regular"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	for _, path := range []string{"missing", "regular", "link"} {
		if _, err := repo.hashHistoryUIPath(h, path, 0); err != nil {
			t.Fatalf("hash %s: %v", path, err)
		}
	}
	if _, err := hashHistoryUIRegular(h, filepath.Join(dir, "regular"), 17<<20, 0); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestStage2OperationHelpers(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	oid := strings.Repeat("a", 40)
	if err := os.WriteFile(filepath.Join(repo.gitDir, "MERGE_HEAD"), []byte(oid+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var state OperationState
	if err := repo.appendOperationHead(context.Background(), &state, "MERGE_HEAD", OperationMerge); err != nil || len(state.Items) != 1 {
		t.Fatalf("append head = %#v, %v", state, err)
	}
	if err := repo.appendDetailOperation(context.Background(), &state, "missing", OperationBisect); err != nil {
		t.Fatal(err)
	}
}
