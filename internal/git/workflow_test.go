package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStageAndUnstageFilesIncludingUnbornBranch(t *testing.T) {
	ctx := context.Background()
	t.Run("ordinary and option-like paths", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("tracked.txt", "base\n")
		r.commitAll("base")
		r.write("tracked.txt", "changed\n")
		r.write("-new file.txt", "new\n")
		repo, err := Discover(r.dir)
		if err != nil {
			t.Fatal(err)
		}
		paths := []string{"tracked.txt", "-new file.txt"}
		if err := repo.Stage(ctx, paths); err != nil {
			t.Fatalf("Stage: %v", err)
		}
		if got := r.git("diff", "--cached", "--name-only"); got != "-new file.txt\ntracked.txt" {
			t.Fatalf("staged paths = %q", got)
		}
		if err := repo.Unstage(ctx, paths); err != nil {
			t.Fatalf("Unstage: %v", err)
		}
		if got := r.git("diff", "--cached", "--name-only"); got != "" {
			t.Fatalf("cached diff after unstage = %q, want empty", got)
		}
	})

	t.Run("unborn branch", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("first.txt", "first\n")
		repo, err := Discover(r.dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Stage(ctx, []string{"first.txt"}); err != nil {
			t.Fatalf("Stage: %v", err)
		}
		if got := r.git("diff", "--cached", "--name-only"); got != "first.txt" {
			t.Fatalf("staged paths = %q", got)
		}
		if err := repo.Unstage(ctx, []string{"first.txt"}); err != nil {
			t.Fatalf("Unstage on unborn branch: %v", err)
		}
		if got := r.git("ls-files"); got != "" {
			t.Fatalf("index after unstage = %q, want empty", got)
		}
		if got := r.read("first.txt"); got != "first\n" {
			t.Fatalf("unstage changed worktree contents: %q", got)
		}
	})

	t.Run("deleted tracked file", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("deleted.txt", "base\n")
		r.commitAll("base")
		if err := os.Remove(filepath.Join(r.dir, "deleted.txt")); err != nil {
			t.Fatal(err)
		}
		repo, _ := Discover(r.dir)
		if err := repo.Stage(ctx, []string{"deleted.txt"}); err != nil {
			t.Fatalf("Stage deletion: %v", err)
		}
		if got := r.git("diff", "--cached", "--name-status"); got != "D\tdeleted.txt" {
			t.Fatalf("staged deletion = %q", got)
		}
	})
}

func TestPathArgumentsAreAlwaysLiteral(t *testing.T) {
	// Repository commands must override inherited pathspec behavior rather than
	// merely appending a duplicate environment variable.
	t.Setenv("GIT_LITERAL_PATHSPECS", "0")
	t.Setenv("GIT_GLOB_PATHSPECS", "1")
	ctx := context.Background()
	for _, test := range []struct {
		name      string
		path      string
		unrelated string
	}{
		{name: "pathspec magic", path: ":(glob)*", unrelated: "unrelated.txt"},
		{name: "wildcard", path: "a*", unrelated: "also.txt"},
	} {
		t.Run(test.name+"/stage", func(t *testing.T) {
			r := newTestRepo(t)
			r.write(test.path, "base\n")
			r.write(test.unrelated, "base\n")
			r.commitAll("base")
			r.write(test.path, "target changed\n")
			r.write(test.unrelated, "unrelated changed\n")
			repo, _ := Discover(r.dir)

			if err := repo.Stage(ctx, []string{test.path}); err != nil {
				t.Fatalf("Stage(%q): %v", test.path, err)
			}
			if got := r.git("diff", "--cached", "--name-only"); got != test.path {
				t.Fatalf("staged paths = %q, want only %q", got, test.path)
			}
		})

		t.Run(test.name+"/unstage", func(t *testing.T) {
			r := newTestRepo(t)
			r.write(test.path, "base\n")
			r.write(test.unrelated, "base\n")
			r.commitAll("base")
			r.write(test.path, "target changed\n")
			r.write(test.unrelated, "unrelated changed\n")
			r.git("add", "--all")
			repo, _ := Discover(r.dir)

			if err := repo.Unstage(ctx, []string{test.path}); err != nil {
				t.Fatalf("Unstage(%q): %v", test.path, err)
			}
			if got := r.git("diff", "--cached", "--name-only"); got != test.unrelated {
				t.Fatalf("staged paths = %q, want only %q", got, test.unrelated)
			}
		})

		t.Run(test.name+"/discard", func(t *testing.T) {
			r := newTestRepo(t)
			r.write("tracked.txt", "base\n")
			r.commitAll("base")
			r.write(test.path, "discard me\n")
			r.write(test.unrelated, "keep me\n")
			repo, _ := Discover(r.dir)

			if err := repo.Discard(ctx, []string{test.path}); err != nil {
				t.Fatalf("Discard(%q): %v", test.path, err)
			}
			if _, err := os.Stat(filepath.Join(r.dir, test.path)); !os.IsNotExist(err) {
				t.Fatalf("discarded path still exists or cannot be checked: %v", err)
			}
			if got := r.read(test.unrelated); got != "keep me\n" {
				t.Fatalf("unrelated file changed to %q", got)
			}
		})
	}
}

func TestUnstageStagedRenameRestoresBothSides(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("old.txt", "contents\n")
	r.commitAll("base")
	if err := os.Rename(filepath.Join(r.dir, "old.txt"), filepath.Join(r.dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--all")
	repo, _ := Discover(r.dir)

	if err := repo.Unstage(ctx, []string{"new.txt"}); err != nil {
		t.Fatalf("Unstage rename: %v", err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("cached diff after unstage = %q, want empty", got)
	}
	status, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]FileStatus, len(status.Files))
	for _, file := range status.Files {
		byPath[file.Path] = file
	}
	if got := byPath["old.txt"]; got.Staged != ChangeNone || got.Unstaged != ChangeDeleted {
		t.Errorf("old.txt status after unstage = %#v, want unstaged deletion", got)
	}
	if got := byPath["new.txt"]; got.Staged != ChangeNone || got.Unstaged != ChangeUntracked {
		t.Errorf("new.txt status after unstage = %#v, want untracked", got)
	}
}

func TestStageDetectedRenameExpandsBothSides(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("old.txt", "contents\n")
	r.commitAll("base")
	if err := os.Rename(filepath.Join(r.dir, "old.txt"), filepath.Join(r.dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--all")
	repo, _ := Discover(r.dir)

	// The source path no longer exists, but Status still exposes it as one side
	// of the detected rename. Stage must update both sides together.
	if err := repo.Stage(ctx, []string{"old.txt"}); err != nil {
		t.Fatalf("Stage rename source: %v", err)
	}
	if got := r.git("diff", "--cached", "--name-status"); got != "R100\told.txt\tnew.txt" {
		t.Fatalf("cached diff after stage = %q, want rename", got)
	}
}

func TestDiscardStagedOnlyChanges(t *testing.T) {
	ctx := context.Background()
	t.Run("modified", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("file.txt", "base\n")
		r.commitAll("base")
		r.write("file.txt", "staged\n")
		r.git("add", "--", "file.txt")
		repo, _ := Discover(r.dir)

		if err := repo.Discard(ctx, []string{"file.txt"}); err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if got := r.read("file.txt"); got != "base\n" {
			t.Fatalf("contents after discard = %q, want base", got)
		}
		if got := r.git("status", "--porcelain"); got != "" {
			t.Fatalf("status after discard = %q, want clean", got)
		}
	})

	t.Run("added", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("base.txt", "base\n")
		r.write(".gitignore", "added.txt\n")
		r.commitAll("base")
		r.write("added.txt", "added\n")
		// Force-add verifies discard removes the worktree file even when clean
		// would ordinarily preserve it because it is ignored.
		r.git("add", "-f", "--", "added.txt")
		repo, _ := Discover(r.dir)

		if err := repo.Discard(ctx, []string{"added.txt"}); err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if _, err := os.Stat(filepath.Join(r.dir, "added.txt")); !os.IsNotExist(err) {
			t.Fatalf("discarded added file still exists or cannot be checked: %v", err)
		}
		if got := r.git("status", "--porcelain"); got != "" {
			t.Fatalf("status after discard = %q, want clean", got)
		}
	})
}

func TestDiscardStagedRenameRestoresBothSidesToHEAD(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("old.txt", "contents\n")
	r.commitAll("base")
	if err := os.Rename(filepath.Join(r.dir, "old.txt"), filepath.Join(r.dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--all")
	repo, _ := Discover(r.dir)

	if err := repo.Discard(ctx, []string{"new.txt"}); err != nil {
		t.Fatalf("Discard rename: %v", err)
	}
	if got := r.read("old.txt"); got != "contents\n" {
		t.Fatalf("restored source contents = %q", got)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("rename destination still exists or cannot be checked: %v", err)
	}
	if got := r.git("status", "--porcelain"); got != "" {
		t.Fatalf("status after discard = %q, want clean", got)
	}
}

func TestDiscardStagedRenameRejectsRecreatedSource(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("old.txt", "original\n")
	r.commitAll("base")
	if err := os.Rename(filepath.Join(r.dir, "old.txt"), filepath.Join(r.dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--all")
	r.write("old.txt", "new untracked work\n")
	repo, _ := Discover(r.dir)

	err := repo.Discard(ctx, []string{"new.txt"})
	if !errors.Is(err, ErrMixedState) {
		t.Fatalf("Discard error = %v, want ErrMixedState", err)
	}
	if got := r.read("old.txt"); got != "new untracked work\n" {
		t.Fatalf("rejected discard overwrote recreated source with %q", got)
	}
	if got := r.read("new.txt"); got != "original\n" {
		t.Fatalf("rejected discard changed destination to %q", got)
	}
	if got := r.git("diff", "--cached", "--name-status"); got != "R100\told.txt\tnew.txt" {
		t.Fatalf("rejected discard changed index to %q", got)
	}
}

func TestDiscardStagedDeletionRejectsIgnoredRecreatedPathBeforeMutation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write(".gitignore", "deleted.txt\n")
	r.write("deleted.txt", "original\n")
	r.git("add", "-f", "--", "deleted.txt")
	r.git("add", "--", ".gitignore")
	r.git("commit", "-m", "base")
	if err := os.Remove(filepath.Join(r.dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--all")
	r.write("deleted.txt", "recreated ignored content\n")
	indexBefore := r.git("write-tree")
	repo, _ := Discover(r.dir)

	err := repo.Discard(ctx, []string{"deleted.txt"})
	if !errors.Is(err, ErrMixedState) {
		t.Fatalf("Discard error = %v, want ErrMixedState", err)
	}
	if got := r.read("deleted.txt"); got != "recreated ignored content\n" {
		t.Fatalf("rejected discard changed recreated content to %q", got)
	}
	if got := r.git("write-tree"); got != indexBefore {
		t.Fatalf("rejected discard changed index tree from %q to %q", indexBefore, got)
	}
}

func TestDiscardUnstagedDeletionRejectsIgnoredReplacementDirectoryBeforeMutation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write(".gitignore", "deleted.txt/\n")
	r.write("deleted.txt", "original\n")
	r.write("earlier.txt", "original\n")
	r.commitAll("base")
	r.write("earlier.txt", "keep this unstaged change\n")
	if err := os.Remove(filepath.Join(r.dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	r.write("deleted.txt/ignored.txt", "ignored replacement content\n")
	indexBefore := r.git("write-tree")
	repo, _ := Discover(r.dir)

	err := repo.Discard(ctx, []string{"earlier.txt", "deleted.txt"})
	if !errors.Is(err, ErrMixedState) {
		t.Fatalf("Discard error = %v, want ErrMixedState", err)
	}
	if got := r.read("earlier.txt"); got != "keep this unstaged change\n" {
		t.Fatalf("rejected discard changed earlier path to %q", got)
	}
	if got := r.read("deleted.txt/ignored.txt"); got != "ignored replacement content\n" {
		t.Fatalf("rejected discard changed replacement directory content to %q", got)
	}
	if got := r.git("write-tree"); got != indexBefore {
		t.Fatalf("rejected discard changed index tree from %q to %q", indexBefore, got)
	}
}

func TestDiscardRejectsStagedCopyBeforeMutation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("source.txt", "base\n")
	r.write("a-earlier.txt", "base\n")
	r.commitAll("base")
	r.write("a-earlier.txt", "keep this change\n")
	r.write("copy.txt", "base\n")
	r.write("source.txt", "base\nmodified\n")
	r.git("add", "--all")
	r.git("config", "status.renames", "copies")
	repo, _ := Discover(r.dir)

	err := repo.Discard(ctx, []string{"a-earlier.txt", "copy.txt"})
	if !errors.Is(err, ErrUnsupportedStagedState) {
		t.Fatalf("Discard error = %v, want ErrUnsupportedStagedState", err)
	}
	if got := r.read("a-earlier.txt"); got != "keep this change\n" {
		t.Fatalf("rejected discard changed earlier path to %q", got)
	}
	if got := r.read("copy.txt"); got != "base\n" {
		t.Fatalf("rejected discard changed copy to %q", got)
	}
	if got := r.git("show", ":copy.txt"); got != "base" {
		t.Fatalf("rejected discard changed index copy to %q", got)
	}
}

func TestDiscardRestoresUnstagedFileAndRejectsMixedState(t *testing.T) {
	ctx := context.Background()
	t.Run("unstaged", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("file.txt", "base\n")
		r.commitAll("base")
		r.write("file.txt", "worktree\n")
		repo, _ := Discover(r.dir)
		if err := repo.Discard(ctx, []string{"file.txt"}); err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if got := r.read("file.txt"); got != "base\n" {
			t.Fatalf("contents after discard = %q", got)
		}
	})

	t.Run("mixed staged and unstaged", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("file.txt", "base\n")
		r.commitAll("base")
		r.write("file.txt", "staged\n")
		r.git("add", "--", "file.txt")
		r.write("file.txt", "unstaged\n")
		repo, _ := Discover(r.dir)
		err := repo.Discard(ctx, []string{"file.txt"})
		if !errors.Is(err, ErrMixedState) {
			t.Fatalf("Discard error = %v, want ErrMixedState", err)
		}
		if got := r.read("file.txt"); got != "unstaged\n" {
			t.Fatalf("rejected discard changed worktree to %q", got)
		}
		if got := r.git("show", ":file.txt"); got != "staged" {
			t.Fatalf("rejected discard changed index to %q", got)
		}
	})

	t.Run("mixed-state preflight does not mutate earlier paths", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("first.txt", "base\n")
		r.write("mixed.txt", "base\n")
		r.commitAll("base")
		r.write("first.txt", "keep this unstaged change\n")
		r.write("mixed.txt", "staged\n")
		r.git("add", "--", "mixed.txt")
		r.write("mixed.txt", "keep this mixed change\n")
		repo, _ := Discover(r.dir)

		err := repo.Discard(ctx, []string{"first.txt", "mixed.txt"})
		if !errors.Is(err, ErrMixedState) {
			t.Fatalf("Discard error = %v, want ErrMixedState", err)
		}
		if got := r.read("first.txt"); got != "keep this unstaged change\n" {
			t.Fatalf("preflight changed earlier path to %q", got)
		}
		if got := r.read("mixed.txt"); got != "keep this mixed change\n" {
			t.Fatalf("preflight changed mixed worktree path to %q", got)
		}
		if got := r.git("show", ":mixed.txt"); got != "staged" {
			t.Fatalf("preflight changed mixed index path to %q", got)
		}
	})
}

func TestUpstreamLogReportsDivergedRanges(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base.txt", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "-u", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")

	peer := cloneTestRepo(t, remote.dir)
	peer.write("remote.txt", "remote\n")
	peer.commitAll("remote change")
	peer.git("push", "origin", "main")
	local.git("fetch", "origin")
	local.write("local.txt", "local\n")
	local.commitAll("local change")

	repo, _ := Discover(local.dir)
	ranges, err := repo.UpstreamLog(ctx)
	if err != nil {
		t.Fatalf("UpstreamLog: %v", err)
	}
	if len(ranges.Ahead) != 1 || ranges.Ahead[0].Subject != "local change" {
		t.Fatalf("Ahead = %#v", ranges.Ahead)
	}
	if len(ranges.Behind) != 1 || ranges.Behind[0].Subject != "remote change" {
		t.Fatalf("Behind = %#v", ranges.Behind)
	}
}

func TestRecentAndUpstreamLogsUseGitShortObjectIDs(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.git("config", "core.abbrev", "12")
	local.write("base.txt", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "-u", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	local.write("local.txt", "local\n")
	local.commitAll("local")

	repo, _ := Discover(local.dir)
	recent, err := repo.RecentLog(ctx, 1)
	if err != nil {
		t.Fatalf("RecentLog: %v", err)
	}
	if len(recent) != 1 || recent[0].ShortID != recent[0].ID[:12] {
		t.Fatalf("RecentLog short ID = %#v, want Git's 12-character abbreviation", recent)
	}
	ranges, err := repo.UpstreamLog(ctx)
	if err != nil {
		t.Fatalf("UpstreamLog: %v", err)
	}
	if len(ranges.Ahead) != 1 || ranges.Ahead[0].ShortID != ranges.Ahead[0].ID[:12] {
		t.Fatalf("UpstreamLog short ID = %#v, want Git's 12-character abbreviation", ranges.Ahead)
	}
}

func TestUpstreamLogLimitCapsEachDivergedRange(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base.txt", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "-u", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")

	peer := cloneTestRepo(t, remote.dir)
	for i := 1; i <= 3; i++ {
		peer.write("remote.txt", string(rune('0'+i))+"\n")
		peer.commitAll("remote change " + string(rune('0'+i)))
	}
	peer.git("push", "origin", "main")
	local.git("fetch", "origin")
	for i := 1; i <= 3; i++ {
		local.write("local.txt", string(rune('0'+i))+"\n")
		local.commitAll("local change " + string(rune('0'+i)))
	}

	repo, _ := Discover(local.dir)
	ranges, err := repo.UpstreamLogLimit(ctx, 2)
	if err != nil {
		t.Fatalf("UpstreamLogLimit: %v", err)
	}
	if len(ranges.Ahead) != 2 || len(ranges.Behind) != 2 {
		t.Fatalf("limited ranges = %#v, want two commits per range", ranges)
	}
	if ranges.Ahead[0].Subject != "local change 3" || ranges.Behind[0].Subject != "remote change 3" {
		t.Fatalf("limited ranges have unexpected tips: %#v", ranges)
	}
	unlimited, err := repo.UpstreamLog(ctx)
	if err != nil {
		t.Fatalf("UpstreamLog: %v", err)
	}
	if len(unlimited.Ahead) != 3 || len(unlimited.Behind) != 3 {
		t.Fatalf("unlimited ranges = %#v, want three commits per range", unlimited)
	}
}

func TestBranchListingCommitCreateAndSwitch(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.git("remote", "add", "origin", remote.dir)
	r.git("push", "-u", "origin", "main")
	r.git("branch", "local-only")
	r.git("push", "origin", "main:remote-only")
	r.git("fetch", "origin")

	repo, _ := Discover(r.dir)
	if err := repo.CreateBranch(ctx, "topic", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := repo.SwitchBranch(ctx, "topic"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	r.write("topic.txt", "topic\n")
	if err := repo.Stage(ctx, []string{"topic.txt"}); err != nil {
		t.Fatal(err)
	}
	commit, err := repo.Commit(ctx, "topic commit")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if commit.ID == "" || commit.Subject != "topic commit" {
		t.Fatalf("Commit result = %#v", commit)
	}
	if got := r.git("branch", "--show-current"); got != "topic" {
		t.Fatalf("current branch = %q, want topic", got)
	}

	branches, err := repo.Branches(ctx)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	type key struct {
		name   string
		remote bool
	}
	got := make(map[key]Branch, len(branches))
	for _, branch := range branches {
		got[key{branch.Name, branch.Remote}] = branch
	}
	for _, want := range []key{
		{"main", false}, {"local-only", false}, {"topic", false},
		{"origin/main", true}, {"origin/remote-only", true},
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("Branches omitted %#v; got %#v", want, branches)
		}
	}
	if !got[key{"topic", false}].Current {
		t.Errorf("topic not marked current: %#v", got[key{"topic", false}])
	}
}
