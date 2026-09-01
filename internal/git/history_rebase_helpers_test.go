package git

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseRebaseNumber(t *testing.T) {
	if got, err := parseRebaseNumber(" 12\n"); err != nil || got != 12 {
		t.Fatalf("parseRebaseNumber(valid) = %d, %v", got, err)
	}
	for _, input := range []string{"", "nope", "-1"} {
		if _, err := parseRebaseNumber(input); err == nil {
			t.Errorf("parseRebaseNumber(%q) succeeded", input)
		}
	}
}

func TestHistoryResolversAndRebaseTodoPlanning(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	head := r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got, err := repo.resolveHistoryBranch(ctx, "main"); err != nil || got != "main" {
		t.Fatalf("resolveHistoryBranch(main) = %q, %v", got, err)
	}
	for _, branch := range []string{"", " main", "-main", "bad\x00name", "missing"} {
		if _, err := repo.resolveHistoryBranch(ctx, branch); err == nil {
			t.Errorf("resolveHistoryBranch(%q) succeeded", branch)
		}
	}
	for _, symmetric := range []bool{false, true} {
		got, err := repo.resolveLogRange(ctx, LogQuery{From: "HEAD", To: "main", Symmetric: symmetric})
		separator := ".."
		if symmetric {
			separator = "..."
		}
		if err != nil || !reflect.DeepEqual(got, []string{head + separator + head}) {
			t.Fatalf("resolveLogRange(%v) = %#v, %v", symmetric, got, err)
		}
	}
	if _, err := repo.resolveLogRange(ctx, LogQuery{From: "missing", To: "HEAD"}); err == nil {
		t.Fatal("missing range start resolved")
	}
	if _, err := repo.resolveLogRange(ctx, LogQuery{From: "HEAD", To: "missing"}); err == nil {
		t.Fatal("missing range end resolved")
	}
	if got, want := rebaseTodoPlan("\n# comment\n pick abc \n\n squash def\n"), []string{"pick abc", "squash def"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rebaseTodoPlan() = %#v, want %#v", got, want)
	}
}

func TestRebaseStateReadingAndTodoReview(t *testing.T) {
	r := newTestRepo(t)
	r.write("file", "base\n")
	head := r.commitAll("base")
	repo, _ := Discover(r.dir)
	directory := filepath.Join(r.dir, ".git", "rebase-merge")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"head-name": "refs/heads/main\n", "onto": head + "\n", "msgnum": "2\n", "end": "4\n", "git-rebase-todo": "pick " + head + "\n"}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	op, err := repo.rebaseState(context.Background(), "rebase-merge", false)
	if err != nil || op.Kind != OperationRebase || op.Branch != "refs/heads/main" || op.Onto != head || op.Current != 2 || op.Total != 4 {
		t.Fatalf("rebaseState() = %#v, %v", op, err)
	}
	op, err = repo.rebaseState(context.Background(), "rebase-merge", true)
	if err != nil || op.Kind != OperationApplyMailbox {
		t.Fatalf("mailbox rebaseState() = %#v, %v", op, err)
	}
	q := HistoryUIRequest{Rebase: RebaseOptions{Todo: "pick " + head + "\n"}}
	canonical, plan, err := repo.canonicalRebaseTodoHistoryUIRequest(context.Background(), q)
	if err != nil || canonical.Rebase.Todo == "" || len(plan) != 3 {
		t.Fatalf("canonical todo = %#v, %#v, %v", canonical, plan, err)
	}
	if err := os.WriteFile(filepath.Join(directory, "msgnum"), []byte("bad\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.rebaseState(context.Background(), "rebase-merge", false); err == nil {
		t.Fatal("invalid rebase progress was accepted")
	}
}

func TestCherryAndReflogQueryLimitsParsingAndArguments(t *testing.T) {
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

func TestHistoryStateFieldsAndPathHashing(t *testing.T) {
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

func TestOperationStateReadsHeadAndOptionalDetails(t *testing.T) {
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
