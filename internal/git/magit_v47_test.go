package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestLogsIncludeRefDecorations(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base.txt", "base\n")
	local.commitAll("base")
	local.git("tag", "v1")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "-u", "origin", "main", "--tags")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")

	peer := cloneTestRepo(t, remote.dir)
	peer.write("remote.txt", "remote\n")
	peer.commitAll("remote change")
	peer.git("push", "origin", "main")
	local.git("fetch", "origin")
	local.write("local.txt", "local\n")
	local.commitAll("local change")

	repo, _ := Discover(local.dir)
	recent, err := repo.RecentLog(ctx, 10)
	if err != nil {
		t.Fatalf("RecentLog: %v", err)
	}
	var decorated bool
	for _, commit := range recent {
		if commit.Subject == "base" && strings.Contains(commit.Refs, "tag: v1") {
			decorated = true
		}
	}
	if !decorated {
		t.Fatalf("RecentLog omitted tag decoration: %#v", recent)
	}

	ranges, err := repo.UpstreamLog(ctx)
	if err != nil {
		t.Fatalf("UpstreamLog: %v", err)
	}
	if len(ranges.Ahead) != 1 || !strings.Contains(ranges.Ahead[0].Refs, "main") {
		t.Fatalf("ahead decorations = %#v", ranges.Ahead)
	}
	if len(ranges.Behind) != 1 || !strings.Contains(ranges.Behind[0].Refs, "origin/main") {
		t.Fatalf("behind decorations = %#v", ranges.Behind)
	}
}

func TestParseCommitsPreservesStructuredFieldsAndRawDecorations(t *testing.T) {
	date := "2026-08-26T10:11:12+00:00"
	out := []byte("full-id\x00short-id\x00HEAD -> main, tag: v1\x00subject\x00author\x00" + date + "\x00")
	commits, err := parseCommits(out)
	if err != nil {
		t.Fatalf("parseCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("commits = %#v", commits)
	}
	got := commits[0]
	if got.ID != "full-id" || got.ShortID != "short-id" || got.Refs != "HEAD -> main, tag: v1" || got.Subject != "subject" || got.Author != "author" {
		t.Fatalf("commit = %#v", got)
	}
}

func TestShowCommitIncludesMetadataStatAndPatch(t *testing.T) {
	r := newTestRepo(t)
	r.write("notes.txt", "first\n")
	parent := r.commitAll("base")
	r.write("notes.txt", "first\nsecond\n")
	id := r.commitAll("add second line")
	repo, _ := Discover(r.dir)

	shown, err := repo.ShowCommit(context.Background(), id)
	if err != nil {
		t.Fatalf("ShowCommit: %v", err)
	}
	for _, want := range []string{
		"Author:", "AuthorDate:", "Commit:", "CommitDate:",
		parent, "notes.txt", "1 file changed", "diff --git a/notes.txt b/notes.txt", "+second",
	} {
		if !strings.Contains(shown, want) {
			t.Errorf("ShowCommit output omitted %q:\n%s", want, shown)
		}
	}
	if _, err := repo.ShowCommit(context.Background(), "--stat"); err == nil {
		t.Fatal("ShowCommit accepted an option as a revision")
	}
}

func TestShowCommitMergeIncludesPatchAgainstFirstParent(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.git("switch", "-c", "feature")
	r.write("merged.txt", "line from second parent\n")
	r.commitAll("feature change")
	r.git("switch", "main")
	r.write("main.txt", "line from first parent\n")
	r.commitAll("main change")
	r.git("merge", "--no-ff", "feature", "-m", "merge feature")
	mergeID := r.git("rev-parse", "HEAD")
	if fields := strings.Fields(r.git("rev-list", "--parents", "-n", "1", mergeID)); len(fields) != 3 {
		t.Fatalf("merge has %d parents, want 2: %v", len(fields)-1, fields)
	}
	repo, _ := Discover(r.dir)

	shown, err := repo.ShowCommit(context.Background(), mergeID)
	if err != nil {
		t.Fatalf("ShowCommit: %v", err)
	}
	for _, want := range []string{"diff --git a/merged.txt b/merged.txt", "+++ b/merged.txt", "+line from second parent"} {
		if !strings.Contains(shown, want) {
			t.Errorf("ShowCommit merge output omitted first-parent patch %q:\n%s", want, shown)
		}
	}
}

func TestShowCommitTruncatesCapturedStdoutAtLimit(t *testing.T) {
	r := newTestRepo(t)
	var contents strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&contents, "line %03d: enough patch data to exceed the test limit\n", i)
	}
	r.write("large.txt", contents.String())
	id := r.commitAll("large patch")
	repo, _ := Discover(r.dir)

	const limit = 256
	shown, err := repo.showCommitLimit(context.Background(), id, limit)
	if err != nil {
		t.Fatalf("showCommitLimit: %v", err)
	}
	if !strings.HasSuffix(shown, showCommitTruncationMarker) {
		t.Fatalf("truncated output lacks marker: %q", shown)
	}
	if got, want := len(strings.TrimSuffix(shown, showCommitTruncationMarker)), limit; got != want {
		t.Fatalf("captured output length = %d, want %d before marker", got, want)
	}
}

func TestDiffsTruncateCapturedStdoutAtLimit(t *testing.T) {
	r := newTestRepo(t)
	r.write("notes.txt", "base\n")
	r.commitAll("base")
	r.write("notes.txt", "staged line one\nstaged line two\n")
	r.git("add", "notes.txt")
	r.write("notes.txt", "unstaged line one\nunstaged line two\n")
	repo, _ := Discover(r.dir)

	const limit = 32
	tests := []struct {
		name string
		diff func(context.Context, string, int) (string, error)
	}{
		{name: "unstaged", diff: repo.diffLimit},
		{name: "staged", diff: repo.diffStagedLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.diff(context.Background(), "notes.txt", limit)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if !strings.HasSuffix(got, diffTruncationMarker) {
				t.Fatalf("truncated diff lacks marker: %q", got)
			}
			if captured := strings.TrimSuffix(got, diffTruncationMarker); len(captured) != limit {
				t.Fatalf("captured diff length = %d, want %d", len(captured), limit)
			}
		})
	}
}

func TestAutomaticPatchLoadingDisablesConfiguredDiffCommandsAndColor(t *testing.T) {
	for _, hostile := range []struct {
		name      string
		configure func(*testRepo)
	}{
		{
			name: "external diff",
			configure: func(r *testRepo) {
				r.git("config", "diff.external", "false")
			},
		},
		{
			name: "textconv",
			configure: func(r *testRepo) {
				r.git("config", "diff.hostile.textconv", "false")
			},
		},
	} {
		t.Run(hostile.name, func(t *testing.T) {
			r := newTestRepo(t)
			r.write(".gitattributes", "notes.txt diff=hostile\n")
			r.write("notes.txt", "base\n")
			r.commitAll("base")
			r.write("notes.txt", "committed\n")
			id := r.commitAll("change notes")
			r.write("notes.txt", "staged\n")
			r.git("add", "notes.txt")
			r.write("notes.txt", "unstaged\n")
			r.git("config", "color.ui", "always")
			hostile.configure(r)
			repo, _ := Discover(r.dir)

			operations := []struct {
				name string
				load func() (string, error)
			}{
				{name: "Diff", load: func() (string, error) { return repo.Diff(context.Background(), "notes.txt") }},
				{name: "DiffStaged", load: func() (string, error) { return repo.DiffStaged(context.Background(), "notes.txt") }},
				{name: "ShowCommit", load: func() (string, error) { return repo.ShowCommit(context.Background(), id) }},
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					got, err := operation.load()
					if err != nil {
						t.Fatalf("%s ran configured %s command: %v", operation.name, hostile.name, err)
					}
					if got == "" {
						t.Fatalf("%s returned no patch", operation.name)
					}
					if strings.Contains(got, "\x1b[") {
						t.Fatalf("%s returned colored output: %q", operation.name, got)
					}
				})
			}
		})
	}
}

func TestRemotesAndAddRemote(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base.txt", "base\n")
	seed.commitAll("base")
	seed.git("push", remote.dir, "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")

	local := newTestRepo(t)
	repo, _ := Discover(local.dir)
	if err := repo.AddRemote(ctx, "origin", remote.dir, true); err != nil {
		t.Fatalf("AddRemote(fetch=true): %v", err)
	}
	if got := local.git("rev-parse", "refs/remotes/origin/main"); got == "" {
		t.Fatal("AddRemote(fetch=true) did not fetch the remote branch")
	}
	remotes, err := repo.Remotes(ctx)
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" || remotes[0].FetchURL != remote.dir || remotes[0].PushURL != remote.dir {
		t.Fatalf("Remotes = %#v", remotes)
	}

	for _, name := range []string{"", " \t\n"} {
		if err := repo.AddRemote(ctx, name, remote.dir, false); err == nil {
			t.Errorf("AddRemote accepted name %q", name)
		}
	}
	if err := repo.AddRemote(ctx, "other", "", false); err == nil {
		t.Error("AddRemote accepted an empty URL")
	}
	err = repo.AddRemote(ctx, "origin", remote.dir, false)
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("duplicate AddRemote error = %v, want preserved CommandError", err)
	}
}

func TestRemotesAllowsMissingURLAndAddRemotePreservesURL(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.git("config", "remote.url-less.fetch", "+refs/heads/*:refs/remotes/url-less/*")
	repo, _ := Discover(local.dir)

	remotes, err := repo.Remotes(ctx)
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(remotes) != 1 || remotes[0] != (Remote{Name: "url-less"}) {
		t.Fatalf("Remotes = %#v, want URL-less remote with empty URL fields", remotes)
	}

	const exactURL = "  relative path with spaces  "
	if err := repo.AddRemote(ctx, "  spaced-name  ", exactURL, false); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	stored, err := repo.output(ctx, "config", "--get", "remote.spaced-name.url")
	if err != nil {
		t.Fatalf("read stored URL: %v", err)
	}
	if got := trimLine(stored); got != exactURL {
		t.Fatalf("stored URL = %q, want exact %q", got, exactURL)
	}
}

func TestFetchAllFetchesEveryRemote(t *testing.T) {
	ctx := context.Background()
	one, two := newBareTestRepo(t), newBareTestRepo(t)
	seedOne, seedTwo := newTestRepo(t), newTestRepo(t)
	seedOne.write("one.txt", "one\n")
	oneID := seedOne.commitAll("one")
	seedOne.git("push", one.dir, "main")
	seedTwo.write("two.txt", "two\n")
	twoID := seedTwo.commitAll("two")
	seedTwo.git("push", two.dir, "main")
	local := newTestRepo(t)
	repo, _ := Discover(local.dir)
	local.git("remote", "add", "one", one.dir)
	local.git("remote", "add", "two", two.dir)
	if err := repo.FetchAll(ctx); err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if got := local.git("rev-parse", "refs/remotes/one/main"); got != oneID {
		t.Fatalf("remote one ref = %q, want %q", got, oneID)
	}
	if got := local.git("rev-parse", "refs/remotes/two/main"); got != twoID {
		t.Fatalf("remote two ref = %q, want %q", got, twoID)
	}
}
