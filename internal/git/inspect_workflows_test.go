package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectionDiffIsLiteralTypedAndBounded(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("-option.txt", "base\n")
	r.write("other.txt", "base\n")
	r.commitAll("base")
	r.write("-option.txt", strings.Repeat("changed\n", 100))
	r.write("other.txt", "unrelated\n")
	repo, _ := Discover(r.dir)

	got, err := repo.QueryDiff(ctx, DiffQuery{Kind: DiffWorktree, Files: []string{"-option.txt"}, OutputLimit: 128})
	if err != nil {
		t.Fatalf("QueryDiff: %v", err)
	}
	if !got.Truncated || len(got.Detail) != 128 {
		t.Fatalf("bounded diff = %d bytes, truncated %v", len(got.Detail), got.Truncated)
	}
	if strings.Contains(got.Detail, "other.txt") {
		t.Fatalf("literal path diff included unrelated path: %q", got.Detail)
	}

	if _, err := repo.QueryDiff(ctx, DiffQuery{Kind: DiffRevision, Base: "--stat"}); err == nil {
		t.Fatal("option-like revision was accepted")
	}
}

func TestInspectionLogParsesGraphFiltersAndTruncation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("one.txt", "one\n")
	r.commitAll("first \x1e and \x1f subject")
	r.write("two.txt", "two\n")
	r.commitAll("second")
	repo, _ := Discover(r.dir)

	result, err := repo.QueryLog(ctx, LogQuery{Limit: 1, Graph: true, Decorations: true})
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Subject != "second" || !result.Truncated {
		t.Fatalf("log result = %#v", result)
	}
	if result.Items[0].ID == "" || result.Items[0].AuthorEmail != "backend-test@example.invalid" {
		t.Fatalf("parsed record = %#v", result.Items[0])
	}

	filtered, err := repo.QueryLog(ctx, LogQuery{Grep: "first \x1e", Files: []string{"one.txt"}})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Subject != "first \x1e and \x1f subject" {
		t.Fatalf("filtered log = %#v, %v", filtered, err)
	}
	if _, err := repo.QueryLog(ctx, LogQuery{Revision: "--all"}); err == nil {
		t.Fatal("option-like log revision was accepted")
	}
}

func TestInspectionRefsCherryAndRevisionNavigation(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	base := r.commitAll("base")
	r.git("remote", "add", "origin", remote.dir)
	r.git("push", "-u", "origin", "main")
	r.git("switch", "-c", "topic")
	r.write("patch.txt", "same patch\n")
	topic := r.commitAll("topic patch")
	r.git("switch", "main")
	r.write("main-only.txt", "make cherry-pick parent distinct\n")
	r.commitAll("main-only")
	r.git("cherry-pick", topic)
	r.write("ahead.txt", "ahead\n")
	r.commitAll("ahead")
	repo, _ := Discover(r.dir)

	refs, err := repo.QueryRefs(ctx, RefQuery{Focus: "main"})
	if err != nil {
		t.Fatalf("QueryRefs: %v", err)
	}
	if refs.Focus == nil || refs.Focus.Name != "main" || refs.Focus.Upstream != "origin/main" || refs.Ahead != 3 || refs.Behind != 0 {
		t.Fatalf("refs focus = %#v", refs)
	}
	cherry, err := repo.QueryCherry(ctx, CherryQuery{Upstream: "main", Head: "topic"})
	if err != nil {
		t.Fatalf("QueryCherry: %v", err)
	}
	if len(cherry.Items) != 1 || !cherry.Items[0].Equivalent || cherry.Items[0].ID != topic {
		t.Fatalf("cherry = %#v", cherry)
	}

	revision, err := repo.ResolveRevision(ctx, "topic")
	if err != nil || revision.ID != topic || len(revision.ParentIDs) != 1 {
		t.Fatalf("ResolveRevision = %#v, %v", revision, err)
	}
	parent, err := repo.RevisionParent(ctx, topic, 1)
	if err != nil || parent.ID != base {
		t.Fatalf("RevisionParent = %#v, %v", parent, err)
	}
	show, err := repo.QueryShowRevision(ctx, ShowRevisionQuery{Revision: topic, Stat: true, Patch: true, OutputLimit: 64})
	if err != nil || !show.Truncated || len(show.Detail) != 64 {
		t.Fatalf("QueryShowRevision = %#v, %v", show, err)
	}
}

func TestInspectionOperationStateUsesStableSentinels(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	head := r.commitAll("base")
	repo, _ := Discover(r.dir)
	writeAdmin := func(name, value string) {
		t.Helper()
		path := filepath.Join(repo.GitDir(), filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeAdmin("MERGE_HEAD", head+"\n")
	writeAdmin("BISECT_START", "refs/heads/main\n")
	writeAdmin("NOTES_MERGE_REF", "refs/notes/commits\n")
	writeAdmin("sequencer/todo", "revert "+head+" subject\n")

	state, err := repo.QueryOperationState(ctx)
	if err != nil {
		t.Fatalf("QueryOperationState: %v", err)
	}
	for _, kind := range []OperationKind{OperationMerge, OperationRevert, OperationBisect, OperationNotesMerge} {
		if !hasOperation(state.Items, kind) {
			t.Errorf("state omitted operation %v: %#v", kind, state)
		}
	}
	writeAdmin("MERGE_HEAD", "--not-an-oid\n")
	if _, err := repo.QueryOperationState(ctx); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed sentinel error = %v", err)
	}
}
