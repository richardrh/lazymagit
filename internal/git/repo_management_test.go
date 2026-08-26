package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func managementRepo(t *testing.T) (*testRepo, *Repository) {
	t.Helper()
	r := newTestRepo(t)
	r.write("tracked.txt", "base\n")
	r.write("dir/kept.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, repo
}

func TestRepositoryIgnoreAndIndexFlags(t *testing.T) {
	r, repo := managementRepo(t)
	ctx := context.Background()

	if _, err := repo.AddIgnoreRule(ctx, "*.top", "", IgnoreTopLevel); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddIgnoreRule(ctx, "*.local", "dir", IgnoreSubdirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddIgnoreRule(ctx, "private/", "", IgnoreRepositoryExclude); err != nil {
		t.Fatal(err)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != ".gitignore\ndir/.gitignore" {
		t.Fatalf("staged ignore files = %q", got)
	}
	if !strings.Contains(r.read("dir/.gitignore"), "*.local\n") {
		t.Fatal("subdirectory ignore rule was not written")
	}

	global := filepath.Join(t.TempDir(), "global ignore")
	r.git("config", "core.excludesFile", global)
	if got, err := repo.AddIgnoreRule(ctx, "*.global", "", IgnoreGlobalExclude); err != nil || got != global {
		t.Fatalf("global ignore = %q, %v", got, err)
	}

	for _, flag := range []IndexFlag{SkipWorktree, AssumeUnchanged} {
		if err := repo.SetIndexFlag(ctx, flag, []string{"tracked.txt"}, true); err != nil {
			t.Fatal(err)
		}
		paths, err := repo.ListIndexFlag(ctx, flag)
		if err != nil || !slices.Contains(paths, "tracked.txt") {
			t.Fatalf("ListIndexFlag(%v) = %q, %v", flag, paths, err)
		}
		if err := repo.SetIndexFlag(ctx, flag, []string{"tracked.txt"}, false); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUntrackAndRenamePreserveAndProtectPaths(t *testing.T) {
	r, repo := managementRepo(t)
	ctx := context.Background()
	if err := repo.Untrack(ctx, []string{"tracked.txt"}); err != nil {
		t.Fatal(err)
	}
	if r.read("tracked.txt") != "base\n" || r.git("ls-files", "--", "tracked.txt") != "" {
		t.Fatal("untrack did not preserve the worktree and remove the index entry")
	}

	r.write("loose ; name", "loose\n")
	if err := repo.RenamePath(ctx, "loose ; name", "renamed ; name"); err != nil {
		t.Fatal(err)
	}
	if got := r.read("renamed ; name"); got != "loose\n" {
		t.Fatalf("renamed contents = %q", got)
	}
	if err := repo.RenamePath(ctx, "dir/kept.txt", "tracked moved.txt"); err != nil {
		t.Fatal(err)
	}
	if got := r.git("diff", "--cached", "--name-status"); !strings.Contains(got, "dir/kept.txt") || !strings.Contains(got, "tracked moved.txt") {
		t.Fatalf("tracked rename was not staged: %q", got)
	}
	r.write("occupied", "do not replace\n")
	if err := repo.RenamePath(ctx, "renamed ; name", "occupied"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("unsafe rename error = %v", err)
	}
}

func TestWorktreePreflightAndSparseCheckout(t *testing.T) {
	r, repo := managementRepo(t)
	ctx := context.Background()
	linked := filepath.Join(t.TempDir(), "linked worktree")
	if err := repo.AddWorktree(ctx, linked, "HEAD", WorktreeAddOptions{Detach: true}); err != nil {
		t.Fatal(err)
	}
	worktrees, err := repo.Worktrees(ctx)
	if err != nil || len(worktrees) != 2 || !worktrees[0].Primary || worktrees[1].Primary {
		t.Fatalf("worktrees = %#v, %v", worktrees, err)
	}
	if err := os.WriteFile(filepath.Join(linked, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveWorktree(ctx, linked, NotConfirmed); !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("unconfirmed dirty remove = %v", err)
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatalf("preflight mutated linked worktree: %v", err)
	}
	if err := repo.RemoveWorktree(ctx, linked, Confirmed); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveWorktree(ctx, r.dir, Confirmed); !errors.Is(err, ErrPrimaryWorktree) {
		t.Fatalf("primary remove error = %v", err)
	}

	if err := repo.EnableSparseCheckoutCone(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSparseCheckout(ctx, []string{"dir"}); err != nil {
		t.Fatal(err)
	}
	state, err := repo.SparseCheckoutState(ctx)
	if err != nil || !state.Enabled || !state.Cone {
		t.Fatalf("sparse state = %#v, %v", state, err)
	}
	if err := repo.ReapplySparseCheckout(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.DisableSparseCheckout(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCloneRawArgvCaptureAndRedaction(t *testing.T) {
	r, repo := managementRepo(t)
	ctx := context.Background()
	destination := filepath.Join(t.TempDir(), "clone target")
	beforeTree, beforeGit := repo.WorkTree(), repo.GitDir()
	if err := CloneRepository(ctx, r.dir, destination, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	if repo.WorkTree() != beforeTree || repo.GitDir() != beforeGit {
		t.Fatal("clone altered the current Repository")
	}
	if _, err := os.Stat(filepath.Join(destination, ".git")); err != nil {
		t.Fatal(err)
	}
	nonempty := t.TempDir()
	if err := os.WriteFile(filepath.Join(nonempty, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CloneRepository(ctx, r.dir, nonempty, CloneOptions{}); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("clone to nonempty destination = %v", err)
	}

	marker := filepath.Join(t.TempDir(), "must-not-exist")
	var records []ProcessRecord
	recorded := WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	capability := NewAllowUnsafeExecution()
	result, err := repo.UnsafeRunGitCommand(recorded, capability, []string{"rev-parse", "--verify", "HEAD;touch " + marker})
	if err == nil || result.ExitCode == 0 {
		t.Fatal("invalid revision unexpectedly succeeded")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("raw argv was interpreted by a shell: %v", err)
	}
	secretURL := "https://user:secret@example.invalid/repo.git"
	_, _ = repo.UnsafeRunGitCommand(recorded, capability, []string{"ls-remote", secretURL})
	if len(records) < 2 || strings.Contains(strings.Join(records[len(records)-1].Args, " "), "secret") {
		t.Fatalf("process record leaked credentials: %#v", records)
	}
}

func TestTypedManagementOptionValidation(t *testing.T) {
	_, repo := managementRepo(t)
	ctx := context.Background()
	if err := repo.UpdateSubmodules(ctx, nil, SubmoduleUpdateOptions{Merge: true, Rebase: true}); err == nil {
		t.Fatal("conflicting submodule modes accepted")
	}
	if err := repo.RemoveSubmodule(ctx, "module", NotConfirmed); !errors.Is(err, ErrDestructiveConfirmationRequired) {
		t.Fatalf("unconfirmed submodule removal = %v", err)
	}
	if err := CloneRepository(ctx, "source", filepath.Join(t.TempDir(), "clone"), CloneOptions{Bare: true, Mirror: true}); err == nil {
		t.Fatal("conflicting clone modes accepted")
	}
	if _, err := repo.AddIgnoreRule(ctx, "bad\nrule", "", IgnoreTopLevel); err == nil {
		t.Fatal("multiline ignore rule accepted")
	}
}

func TestSubmoduleLifecycleAndRemovalPreflight(t *testing.T) {
	sub := newTestRepo(t)
	sub.write("content.txt", "submodule\n")
	sub.commitAll("submodule content")
	_, repo := managementRepo(t)
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	ctx := context.Background()
	path := "modules/sub module"

	if err := repo.AddSubmodule(ctx, sub.dir, path, SubmoduleAddOptions{Name: "fixture module"}); err != nil {
		t.Fatal(err)
	}
	modules, err := repo.Submodules(ctx)
	if err != nil || len(modules) != 1 || modules[0].Name != "fixture module" || modules[0].Path != path || !modules[0].Initialized {
		t.Fatalf("submodules = %#v, %v", modules, err)
	}
	if err := repo.SyncSubmodules(ctx, []string{path}, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeinitSubmodules(ctx, []string{path}, Confirmed); err != nil {
		t.Fatal(err)
	}
	if err := repo.InitSubmodules(ctx, []string{path}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateSubmodules(ctx, []string{path}, SubmoduleUpdateOptions{Init: true}); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(repo.WorkTree(), filepath.FromSlash(path))
	if err := os.WriteFile(filepath.Join(full, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveSubmodule(ctx, path, Confirmed); !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("dirty submodule removal = %v", err)
	}
	if err := os.Remove(filepath.Join(full, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveSubmodule(ctx, path, Confirmed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(full); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed submodule still exists: %v", err)
	}
}
