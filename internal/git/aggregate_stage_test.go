package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageAllStagesTrackedChangesAndExcludesUntracked(t *testing.T) {
	r := newTestRepo(t)
	for _, path := range []string{"one.txt", "two.txt", "delete.txt", "-tracked.txt", ":(glob)*"} {
		r.write(path, "base\n")
	}
	r.commitAll("base")
	r.write("one.txt", "one changed\n")
	r.write("two.txt", "two changed\n")
	r.write("-tracked.txt", "option-like changed\n")
	r.write(":(glob)*", "literal changed\n")
	if err := os.Remove(filepath.Join(r.dir, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	r.write("untracked.txt", "leave me alone\n")
	r.write("-untracked.txt", "also leave me alone\n")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })

	changed, err := repo.StageAll(ctx)
	if err != nil {
		t.Fatalf("StageAll: %v", err)
	}
	if !changed {
		t.Fatal("StageAll did not report tracked changes")
	}
	if len(records) != 1 || len(records[0].Args) != 2 || records[0].Args[0] != "add" || records[0].Args[1] != "--update" {
		t.Fatalf("StageAll process = %#v, want git add --update", records)
	}
	if got, want := r.git("diff", "--cached", "--name-only"), "-tracked.txt\n:(glob)*\ndelete.txt\none.txt\ntwo.txt"; got != want {
		t.Fatalf("staged paths = %q, want %q", got, want)
	}
	if got, want := r.git("diff", "--cached", "--name-status", "--", "delete.txt"), "D\tdelete.txt"; got != want {
		t.Fatalf("staged deletion = %q, want %q", got, want)
	}
	if got, want := r.git("status", "--porcelain", "--", "untracked.txt", "-untracked.txt"), "?? -untracked.txt\n?? untracked.txt"; got != want {
		t.Fatalf("untracked status = %q, want %q", got, want)
	}
}

func TestUnstageAllHandlesMixedChangesAndPreservesWorktree(t *testing.T) {
	r := newTestRepo(t)
	r.write("mixed.txt", "base\n")
	r.write("delete.txt", "base\n")
	r.write("-literal.txt", "base\n")
	r.commitAll("base")
	r.write("mixed.txt", "staged version\n")
	r.git("add", "--", "mixed.txt")
	if err := os.Remove(filepath.Join(r.dir, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("add", "--all")
	r.write("mixed.txt", "worktree version\n")
	r.write("-literal.txt", "literal worktree version\n")
	r.git("add", "--", "-literal.txt")
	r.write(":(top)added", "new staged file\n")
	r.git("--literal-pathspecs", "add", "--", ":(top)added")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })

	if err := repo.UnstageAll(ctx); err != nil {
		t.Fatalf("UnstageAll: %v", err)
	}
	if len(records) != 1 || strings.Join(records[0].Args, " ") != "reset --mixed --quiet HEAD" {
		t.Fatalf("UnstageAll process = %#v, want no-path mixed reset", records)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("cached paths after UnstageAll = %q, want empty", got)
	}
	if got := r.read("mixed.txt"); got != "worktree version\n" {
		t.Fatalf("mixed worktree contents = %q", got)
	}
	if got := r.read("-literal.txt"); got != "literal worktree version\n" {
		t.Fatalf("literal worktree contents = %q", got)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("UnstageAll restored deleted worktree path: %v", err)
	}
	if got := r.read(":(top)added"); got != "new staged file\n" {
		t.Fatalf("added worktree contents = %q", got)
	}
}

func TestUnstageAllOnUnbornBranchAndAggregateNoOps(t *testing.T) {
	ctx := context.Background()
	t.Run("unborn", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("first.txt", "first\n")
		r.write("-option.txt", "option\n")
		r.write(":(glob)*", "literal\n")
		r.git("add", "--all")
		repo, _ := Discover(r.dir)
		var records []ProcessRecord
		recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })

		if err := repo.UnstageAll(recorded); err != nil {
			t.Fatalf("UnstageAll: %v", err)
		}
		if len(records) != 1 || strings.Join(records[0].Args, " ") != "read-tree --empty" {
			t.Fatalf("unborn UnstageAll process = %#v, want no-path empty index", records)
		}
		if got := r.git("ls-files"); got != "" {
			t.Fatalf("unborn index = %q, want empty", got)
		}
		for path, want := range map[string]string{"first.txt": "first\n", "-option.txt": "option\n", ":(glob)*": "literal\n"} {
			if got := r.read(path); got != want {
				t.Errorf("worktree %q = %q, want %q", path, got, want)
			}
		}
	})

	t.Run("clean", func(t *testing.T) {
		r := newTestRepo(t)
		r.write("clean.txt", "clean\n")
		r.commitAll("base")
		repo, _ := Discover(r.dir)
		var records []ProcessRecord
		recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })

		changed, err := repo.StageAll(recorded)
		if err != nil {
			t.Fatalf("StageAll clean: %v", err)
		}
		if changed {
			t.Fatal("StageAll clean reported a change")
		}
		if err := repo.UnstageAll(recorded); err != nil {
			t.Fatalf("UnstageAll clean: %v", err)
		}
		if len(records) != 0 {
			t.Fatalf("clean aggregate operations launched %d mutating commands", len(records))
		}
	})
}
