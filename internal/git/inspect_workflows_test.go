package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestInspectionReflogShortlogAndMergedRefs(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	base := r.commitAll("base")
	r.git("branch", "merged", base)
	r.git("switch", "-c", "unmerged")
	r.write("topic.txt", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("next.txt", "next\n")
	r.commitAll("next")
	repo, _ := Discover(r.dir)

	reflog, err := repo.QueryReflog(ctx, ReflogQuery{Revision: "HEAD", Limit: 2, OutputLimit: 512})
	if err != nil || len(reflog.Items) != 2 || reflog.Items[0].ID == "" || reflog.Items[0].Selector == "" {
		t.Fatalf("QueryReflog = %#v, %v", reflog, err)
	}
	if all, err := repo.QueryReflog(ctx, ReflogQuery{All: true, Limit: 2, OutputLimit: 512}); err != nil || len(all.Items) == 0 {
		t.Fatalf("all reflogs = %#v, %v", all, err)
	}
	if _, err := repo.QueryReflog(ctx, ReflogQuery{All: true, Revision: "HEAD"}); err == nil {
		t.Fatal("ambiguous reflog selectors were accepted")
	}
	if _, err := repo.QueryReflog(ctx, ReflogQuery{Revision: "--all"}); err == nil {
		t.Fatal("option-like reflog revision was accepted")
	}

	shortlog, err := repo.QueryShortlog(ctx, ShortlogQuery{Revision: "HEAD", Summary: true, Numbered: true, Email: true, WrapWidth: 80, WrapIndent1: 4, WrapIndent2: 8, WrapIndent1Set: true, WrapIndent2Set: true, OutputLimit: 512})
	if err != nil || shortlog.Detail == "" || !strings.Contains(shortlog.Detail, "backend-test@example.invalid") {
		t.Fatalf("QueryShortlog = %#v, %v", shortlog, err)
	}
	if _, err := repo.QueryShortlog(ctx, ShortlogQuery{Revision: "HEAD", WrapWidth: 80, WrapIndent2: 4, WrapIndent2Set: true}); err == nil {
		t.Fatal("shortlog accepted a second indent without the first")
	}
	if _, err := repo.QueryShortlog(ctx, ShortlogQuery{Range: base + "..HEAD", OutputLimit: 512}); err != nil {
		t.Fatalf("shortlog range: %v", err)
	}
	if _, err := repo.QueryShortlog(ctx, ShortlogQuery{Range: "--all..HEAD"}); err == nil {
		t.Fatal("option-like shortlog range endpoint was accepted")
	}
	if _, err := repo.QueryShortlog(ctx, ShortlogQuery{Revision: "--all"}); err == nil {
		t.Fatal("option-like shortlog revision was accepted")
	}

	contained, err := repo.QueryRefs(ctx, RefQuery{Focus: "main", Contains: "main", Sort: RefSortNameReverse})
	if err != nil || !refResultHasName(contained, "main") || refResultHasName(contained, "merged") || refResultHasName(contained, "unmerged") {
		t.Fatalf("contained refs = %#v, %v", contained, err)
	}
	if _, err := repo.QueryRefs(ctx, RefQuery{Contains: "--all"}); err == nil {
		t.Fatal("option-like contains revision was accepted")
	}
	if _, err := repo.QueryRefs(ctx, RefQuery{Sort: RefSort(99)}); err == nil {
		t.Fatal("unknown ref sort was accepted")
	}

	merged, err := repo.QueryRefs(ctx, RefQuery{Focus: "main", MergedTo: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !refResultHasName(merged, "merged") || refResultHasName(merged, "unmerged") {
		t.Fatalf("merged refs = %#v", merged)
	}
	notMerged, err := repo.QueryRefs(ctx, RefQuery{Focus: "main", NoMergedTo: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !refResultHasName(notMerged, "unmerged") || refResultHasName(notMerged, "merged") {
		t.Fatalf("not-merged refs = %#v", notMerged)
	}
}

func TestInspectionConflictDiffIsReadOnlyAndRevisionFree(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("conflict.txt", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "topic")
	r.write("conflict.txt", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("conflict.txt", "main\n")
	r.commitAll("main")
	cmd := exec.Command("git", "-C", r.dir, "merge", "topic")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("merge unexpectedly succeeded: %s", out)
	}
	repo, _ := Discover(r.dir)

	result, err := repo.QueryDiff(ctx, DiffQuery{Kind: DiffConflicts, Files: []string{"conflict.txt"}})
	if err != nil || !strings.Contains(result.Detail, "diff --cc conflict.txt") {
		t.Fatalf("conflict diff = %#v, %v", result, err)
	}
	if _, err := repo.QueryDiff(ctx, DiffQuery{Kind: DiffConflicts, Base: "HEAD"}); err == nil {
		t.Fatal("conflict diff accepted a revision")
	}
	if got := r.git("ls-files", "-u", "--", "conflict.txt"); got == "" {
		t.Fatal("conflict inspection changed the index")
	}
}

func refResultHasName(result RefResult, name string) bool {
	for _, refs := range [][]Ref{result.Local, result.Remote, result.Tags} {
		for _, ref := range refs {
			if ref.Name == name {
				return true
			}
		}
	}
	return false
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
