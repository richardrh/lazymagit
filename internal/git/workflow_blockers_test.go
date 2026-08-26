package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBranchDeleteConfirmationRejectsMovedRef(t *testing.T) {
	r := newTestRepo(t)
	r.write("a", "one\n")
	base := r.commitAll("base")
	r.git("switch", "-c", "victim")
	r.write("a", "two\n")
	r.commitAll("topic")
	r.git("switch", "main")
	repo, _ := Discover(r.dir)
	plan, err := repo.LocalBranchDeletePreflight(context.Background(), "victim")
	if err != nil {
		t.Fatal(err)
	}
	r.git("branch", "-f", "victim", base)
	if _, err := repo.DeleteLocalBranchConfirmed(context.Background(), "victim", plan.Token); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("moved branch error = %v", err)
	}
	if got := r.git("rev-parse", "victim"); got != base {
		t.Fatalf("victim changed: %s", got)
	}
}

func TestStashDropConfirmationRejectsReflogRenumber(t *testing.T) {
	r := newTestRepo(t)
	r.write("a", "base\n")
	r.commitAll("base")
	r.write("a", "first\n")
	repo, _ := Discover(r.dir)
	if err := repo.StashPush(context.Background(), StashPushOptions{Message: "first"}); err != nil {
		t.Fatal(err)
	}
	stashes, _ := repo.Stashes(context.Background())
	r.write("a", "second\n")
	if err := repo.StashPush(context.Background(), StashPushOptions{Message: "second"}); err != nil {
		t.Fatal(err)
	}
	err := repo.StashDrop(context.Background(), "stash@{0}", ConfirmationOptions{Token: NewConfirmationToken(stashes[0].ID)})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("renumbered stash error = %v", err)
	}
	if got, _ := repo.Stashes(context.Background()); len(got) != 2 {
		t.Fatalf("stash count = %d", len(got))
	}
}

func TestSaveDiffPatchNoOverwriteConcurrentDestination(t *testing.T) {
	r := newTestRepo(t)
	r.write("a", "base\n")
	r.commitAll("base")
	r.write("a", "changed\n")
	repo, _ := Discover(r.dir)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if repo.SaveDiffPatch(context.Background(), "out.patch", DiffPatchOptions{}) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful writers = %d", successes.Load())
	}
	b, err := os.ReadFile(filepath.Join(r.dir, "out.patch"))
	if err != nil || len(b) == 0 {
		t.Fatalf("installed patch: %v, %d bytes", err, len(b))
	}
}

func TestConfigureRemoteValidatesLateFieldBeforeMutation(t *testing.T) {
	r := newTestRepo(t)
	r.git("remote", "add", "origin", "before")
	repo, _ := Discover(r.dir)
	after := "after"
	invalid := RemoteTagOpt(99)
	err := repo.ConfigureRemote(context.Background(), RemoteConfigArgs{Remote: "origin", FetchURL: &after, TagOpt: &invalid})
	if err == nil {
		t.Fatal("invalid tag option accepted")
	}
	if got := r.git("remote", "get-url", "origin"); got != "before" {
		t.Fatalf("URL mutated to %q", got)
	}
}

func TestAddIgnoreRuleStagesOnlyAppendedRule(t *testing.T) {
	r := newTestRepo(t)
	r.write(".gitignore", "base\n")
	r.commitAll("base")
	r.write(".gitignore", "base\nunrelated\n")
	repo, _ := Discover(r.dir)
	if _, err := repo.AddIgnoreRule(context.Background(), "added", "", IgnoreTopLevel); err != nil {
		t.Fatal(err)
	}
	if got := r.git("show", ":.gitignore"); got != "base\nadded" {
		t.Fatalf("staged ignore = %q", got)
	}
	if got := r.read(".gitignore"); got != "base\nunrelated\nadded\n" {
		t.Fatalf("worktree ignore = %q", got)
	}
}

func TestCommitMutationReportsCreatedOIDWhenPostQueryFails(t *testing.T) {
	r := newTestRepo(t)
	r.write("a", "base\n")
	r.commitAll("base")
	r.write("a", "next\n")
	r.git("add", "a")
	repo, _ := Discover(r.dir)
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithProcessRecorder(ctx, func(ProcessRecord) { cancel() })
	commit, err := repo.CreateCommit(ctx, "next", CommitOptions{})
	var partial *CommitMutationError
	if !errors.As(err, &partial) || partial.CommitOID == "" || commit.ID != partial.CommitOID {
		t.Fatalf("commit = %#v, error = %#v", commit, err)
	}
	if got := r.git("rev-parse", "HEAD"); got != partial.CommitOID {
		t.Fatalf("HEAD = %s", got)
	}
}
