package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompileDiffQuery(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	base := r.commitAll("base")
	repo, _ := Discover(r.dir)
	args, limit, err := repo.compileDiffQuery(ctx, DiffQuery{Kind: DiffRevisionRange, Base: base, Target: "HEAD", TripleDot: true, Context: 7, ContextSet: true, Algorithm: DiffAlgorithmHistogram, Stat: true, Files: []string{"file.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--unified=7", "--histogram", "--stat", base + "..." + base, "-- file.txt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compiled diff %q missing %q", joined, want)
		}
	}
	if limit != inspectionOutputLimit {
		t.Fatalf("limit = %d", limit)
	}
	if _, _, err := repo.compileDiffQuery(ctx, DiffQuery{Kind: DiffKind(99)}); err == nil {
		t.Fatal("unknown diff kind compiled")
	}
}

func TestCompileShortlogQuery(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	head := r.commitAll("base")
	repo, _ := Discover(r.dir)
	args, limit, err := repo.compileShortlogQuery(ctx, ShortlogQuery{Revision: "HEAD", Numbered: true, Summary: true, Email: true, Group: "author", Format: "%s", WrapWidth: 72, WrapIndent1: 4, WrapIndent2: 8, WrapIndent1Set: true, WrapIndent2Set: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--numbered", "--summary", "--email", "--group=author", "--format=%s", "-w72,4,8", head} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compiled shortlog %q missing %q", joined, want)
		}
	}
	if limit != inspectionOutputLimit {
		t.Fatalf("limit = %d", limit)
	}
	if _, _, err := repo.compileShortlogQuery(ctx, ShortlogQuery{Group: "invalid"}); err == nil {
		t.Fatal("invalid group compiled")
	}
}

func TestCompileLogQuery(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	head := r.commitAll("base")
	repo, _ := Discover(r.dir)
	since := time.Date(2020, 1, 2, 3, 4, 5, 0, time.FixedZone("test", 3600))
	args, limit, truncated, err := repo.compileLogQuery(ctx, LogQuery{Revision: "HEAD", Limit: inspectionItemLimit + 1, Graph: true, Decorations: true, FirstParent: true, Order: LogOrderTopo, Author: "name", Grep: "subject", Since: &since, Files: []string{"file.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--graph", "--decorate=short", "--first-parent", "--topo-order", "--author=name", "--fixed-strings", "--grep=subject", "--since=2020-01-02T02:04:05Z", head, "-- file.txt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compiled log %q missing %q", joined, want)
		}
	}
	if limit != inspectionItemLimit || !truncated {
		t.Fatalf("limit/truncated = %d/%v", limit, truncated)
	}
	if _, _, _, err := repo.compileLogQuery(ctx, LogQuery{MergesOnly: true, NoMerges: true}); err == nil {
		t.Fatal("incompatible merge filters compiled")
	}
}

func TestCompileRefQueryAndBuildRefResult(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file.txt", "base\n")
	head := r.commitAll("base")
	repo, _ := Discover(r.dir)
	args, limit, byteLimit, truncated, err := repo.compileRefQuery(ctx, RefQuery{Contains: "HEAD", MergedTo: "HEAD", NoMergedTo: "HEAD", Sort: RefSortNameReverse, Limit: inspectionItemLimit + 1})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--sort=-refname", "--contains=" + head, "--merged=" + head, "--no-merged=" + head, "refs/heads refs/remotes refs/tags"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compiled refs %q missing %q", joined, want)
		}
	}
	if limit != inspectionItemLimit || byteLimit != inspectionOutputLimit || !truncated {
		t.Fatalf("limits/truncated = %d/%d/%v", limit, byteLimit, truncated)
	}
	refs := []Ref{{Kind: RefLocal, Name: "main", FullName: "refs/heads/main", ID: head, Current: true}, {Kind: RefRemote, Name: "origin/main", ID: head}, {Kind: RefTag, Name: "v1", ID: head}}
	result, err := repo.buildRefResult(ctx, RefQuery{Focus: "main"}, refs, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Focus == nil || result.Focus.Name != "main" || len(result.Local) != 1 || len(result.Remote) != 1 || len(result.Tags) != 1 || !result.Truncated {
		t.Fatalf("built refs = %#v", result)
	}
}

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

func TestInspectionRequestPullValidatesAndBuildsBoundedDetail(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("one.txt", "one\n")
	start := r.commitAll("first")
	r.write("two.txt", "two\n")
	r.commitAll("second")
	remote := newBareTestRepo(t)
	r.git("remote", "add", "origin", remote.dir)
	r.git("push", "origin", "main")
	repo, _ := Discover(r.dir)

	for _, query := range []RequestPullQuery{
		{URL: ""},
		{URL: "--upload-pack=bad"},
		{URL: remote.dir, Start: "HEAD", OutputLimit: -1},
	} {
		if _, err := repo.QueryRequestPull(ctx, query); err == nil {
			t.Fatalf("invalid request-pull query was accepted: %#v", query)
		}
	}
	result, err := repo.QueryRequestPull(ctx, RequestPullQuery{Start: start, URL: remote.dir, OutputLimit: 4096})
	if err != nil {
		t.Fatalf("request-pull: %v", err)
	}
	if result.Detail == "" || result.Truncated {
		t.Fatalf("request-pull result = %#v", result)
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

func TestDiffQueryPureHelpersDirectly(t *testing.T) {
	if err := validateDiffQuery(DiffQuery{Context: -1}); err == nil {
		t.Fatal("negative context was accepted")
	}
	options, err := diffQueryOptions(DiffQuery{ContextSet: true, Context: 4, Algorithm: DiffAlgorithmPatience, WordDiff: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(options, " ")
	for _, want := range []string{"--unified=4", "--patience", "--word-diff=plain"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("options %q missing %q", joined, want)
		}
	}
	if _, err := diffQueryOptions(DiffQuery{Algorithm: DiffAlgorithm(99)}); err == nil {
		t.Fatal("unknown algorithm was accepted")
	}
	if got, err := inspectionByteLimit("diff", 0); err != nil || got != inspectionOutputLimit {
		t.Fatalf("default byte limit = %d, %v", got, err)
	}
}

func TestLogQueryPureHelpersDirectly(t *testing.T) {
	if err := validateLogQuery(LogQuery{MergesOnly: true, NoMerges: true}); err == nil {
		t.Fatal("conflicting merge filters were accepted")
	}
	if got, truncated := logItemLimit(inspectionItemLimit + 1); got != inspectionItemLimit || !truncated {
		t.Fatalf("logItemLimit = %d, %v", got, truncated)
	}
	options, err := logQueryOptions(LogQuery{Graph: true, Decorations: true, Order: LogOrderTopo, Author: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(options, " ")
	for _, want := range []string{"--graph", "--decorate=short", "--topo-order", "--author=Ada"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("options %q missing %q", joined, want)
		}
	}
	if !invalidLogSelectors(LogQuery{Revision: "HEAD", From: "main", To: "topic"}) {
		t.Fatal("ambiguous log selectors were accepted")
	}
	if got, err := singleSelector("abc", nil); err != nil || len(got) != 1 || got[0] != "abc" {
		t.Fatalf("singleSelector = %#v, %v", got, err)
	}
	since := time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("offset", 3600))
	until := since.Add(time.Hour)
	options, err = logQueryOptions(LogQuery{
		All: true, Reflog: true, FirstParent: true, MergesOnly: true, Reverse: true,
		Order: LogOrderAuthorDate, Grep: "literal", Since: &since, Until: &until, BranchPattern: "release/*",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(options, " ")
	for _, want := range []string{
		"--no-decorate", "--all", "--reflog", "--first-parent", "--merges", "--reverse",
		"--author-date-order", "--fixed-strings", "--grep=literal",
		"--since=" + since.UTC().Format(time.RFC3339Nano), "--until=" + until.UTC().Format(time.RFC3339Nano),
		"HEAD", "--branches=release/*",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("options %q missing %q", joined, want)
		}
	}
	if options, err = logQueryOptions(LogQuery{NoMerges: true, Order: LogOrderDate, TagPattern: "v*"}); err != nil || !strings.Contains(strings.Join(options, " "), "--tags=v*") {
		t.Fatalf("tag options = %#v, %v", options, err)
	}
	if _, err := logQueryOptions(LogQuery{Order: LogOrder(99)}); err == nil {
		t.Fatal("unknown log order was accepted")
	}
}

func TestShortlogQueryPureHelpersDirectly(t *testing.T) {
	options, err := shortlogQueryOptions(ShortlogQuery{Numbered: true, Group: "author", Format: "%s", WrapWidth: 72, WrapIndent1: 4, WrapIndent1Set: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(options, " ")
	for _, want := range []string{"--numbered", "--group=author", "--format=%s", "-w72,4"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("options %q missing %q", joined, want)
		}
	}
	if _, err := shortlogWrapOption(ShortlogQuery{WrapWidth: -1}); err == nil {
		t.Fatal("negative wrap width was accepted")
	}
}

func TestRefResultPureHelpersDirectly(t *testing.T) {
	refs := []Ref{
		{Kind: RefLocal, Name: "main", FullName: "refs/heads/main", ID: "aaa", Current: true},
		{Kind: RefRemote, Name: "origin/main", FullName: "refs/remotes/origin/main", ID: "aaa"},
		{Kind: RefTag, Name: "v1", FullName: "refs/tags/v1", ID: "bbb"},
	}
	result := categorizedRefResult(refs, true)
	if len(result.Local) != 1 || len(result.Remote) != 1 || len(result.Tags) != 1 || !result.Truncated {
		t.Fatalf("categorized result = %#v", result)
	}
	if focus := selectRefFocus(refs, "", "aaa"); focus == nil || focus.Name != "main" {
		t.Fatalf("default focus = %#v", focus)
	}
	if focus := selectRefFocus(refs, "origin/main", "aaa"); focus == nil || focus.Name != "origin/main" {
		t.Fatalf("named focus = %#v", focus)
	}
}
