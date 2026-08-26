package git

import (
	"context"
	"errors"
	"testing"
)

func TestBranchWorkflowCheckoutCreateOrphanAndValidation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	base := r.commitAll("base")
	repo, _ := Discover(r.dir)

	if err := repo.CreateBranchOnly(ctx, "next", "HEAD"); err != nil {
		t.Fatalf("CreateBranchOnly: %v", err)
	}
	if got := r.git("branch", "--show-current"); got != "main" {
		t.Fatalf("create-only changed current branch to %q", got)
	}
	if err := repo.CheckoutBranch(ctx, "next"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	if err := repo.CheckoutRevision(ctx, base); err != nil {
		t.Fatalf("CheckoutRevision: %v", err)
	}
	if got := r.git("branch", "--show-current"); got != "" {
		t.Fatalf("revision checkout was not detached: %q", got)
	}
	if err := repo.CreateAndCheckoutBranch(ctx, "topic", base); err != nil {
		t.Fatalf("CreateAndCheckoutBranch from detached HEAD: %v", err)
	}
	r.write("dirty.txt", "preserved\n")
	if err := repo.CheckoutBranch(ctx, "main"); err != nil {
		t.Fatalf("CheckoutBranch with nonconflicting dirty tree: %v", err)
	}
	if got := r.read("dirty.txt"); got != "preserved\n" {
		t.Fatalf("checkout changed dirty file to %q", got)
	}
	if err := repo.CreateOrphanBranch(ctx, "empty-root"); err != nil {
		t.Fatalf("CreateOrphanBranch: %v", err)
	}
	if got := r.git("branch", "--show-current"); got != "empty-root" {
		t.Fatalf("orphan branch = %q", got)
	}

	for _, operation := range []func() error{
		func() error { return repo.CreateBranchOnly(ctx, "--bad", base) },
		func() error { return repo.CreateBranchOnly(ctx, "@{-1}", base) },
		func() error { return repo.CheckoutRevision(ctx, "--bad") },
	} {
		if err := operation(); err == nil {
			t.Fatal("operation accepted an option-like name/revision")
		}
	}

	unborn := newTestRepo(t)
	unbornRepo, _ := Discover(unborn.dir)
	if err := unbornRepo.CheckoutRevision(ctx, "HEAD"); err == nil {
		t.Fatal("CheckoutRevision accepted HEAD in an unborn repository")
	}
}

func TestBranchWorkflowRenameUpstreamAndConfiguration(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.git("remote", "add", "origin", remote.dir)
	r.git("push", "origin", "main")
	r.git("fetch", "origin")
	r.git("branch", "topic")
	repo, _ := Discover(r.dir)

	if err := repo.SetBranchUpstream(ctx, "topic", "origin/main"); err != nil {
		t.Fatalf("SetBranchUpstream: %v", err)
	}
	if err := repo.SetBranchDescription(ctx, "topic", "work in progress"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBranchRebase(ctx, "topic", RebaseMerges); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBranchPushRemote(ctx, "topic", "origin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RenameBranch(ctx, "topic", "renamed"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}
	configuration, err := repo.BranchConfiguration(ctx, "renamed")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Description.Value != "work in progress" || configuration.Rebase.Value != "merges" || configuration.PushRemote.Value != "origin" {
		t.Fatalf("renamed configuration = %#v", configuration)
	}
	if got, err := repo.workflowConfigValue(ctx, "branch.topic.pushRemote"); err != nil || got.Set {
		t.Fatalf("old pushRemote remains configured: %#v, %v", got, err)
	}
	if err := repo.UnsetBranchUpstream(ctx, "renamed"); err != nil {
		t.Fatalf("UnsetBranchUpstream: %v", err)
	}
	if err := repo.SetPullRebase(ctx, RebaseInteractive); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetRemotePushDefault(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.PullRebase(ctx); got != (ConfiguredValue{Value: "interactive", Set: true}) {
		t.Fatalf("pull.rebase = %#v", got)
	}
	if got, _ := repo.RemotePushDefault(ctx); got != (ConfiguredValue{Value: "origin", Set: true}) {
		t.Fatalf("remote.pushDefault = %#v", got)
	}
	if err := repo.UnsetBranchDescription(ctx, "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnsetBranchRebase(ctx, "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnsetBranchPushRemote(ctx, "renamed"); err != nil {
		t.Fatal(err)
	}
}

func TestBranchWorkflowDeletePreflightResetAndRemoteDelete(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	base := r.commitAll("base")
	r.git("branch", "merged")
	r.git("switch", "-c", "unmerged")
	r.write("topic.txt", "topic\n")
	topic := r.commitAll("topic")
	r.git("switch", "main")
	r.git("branch", "reset-me", topic)
	r.git("remote", "add", "origin", remote.dir)
	r.git("push", "origin", "main:remote-topic")
	repo, _ := Discover(r.dir)

	deleted, err := repo.DeleteLocalBranch(ctx, "merged")
	if err != nil || !deleted.Deleted || !deleted.Merged || deleted.Unmerged {
		t.Fatalf("delete merged = %#v, %v", deleted, err)
	}
	preflight, err := repo.DeleteLocalBranch(ctx, "unmerged")
	if err != nil || preflight.Deleted || !preflight.Unmerged || preflight.Confirmation != BranchDeleteConfirmUnmerged {
		t.Fatalf("unmerged preflight = %#v, %v", preflight, err)
	}
	if got := r.git("rev-parse", "refs/heads/unmerged"); got != topic {
		t.Fatalf("preflight mutated unmerged branch to %q", got)
	}
	confirmed, err := repo.DeleteLocalBranchConfirmed(ctx, "unmerged", preflight.Token)
	if err != nil || !confirmed.Deleted {
		t.Fatalf("confirmed delete = %#v, %v", confirmed, err)
	}
	current, err := repo.DeleteLocalBranch(ctx, "main")
	if err != nil || current.Confirmation != BranchDeleteSwitchCurrent || !current.Current {
		t.Fatalf("current preflight = %#v, %v", current, err)
	}
	if err := repo.ResetBranch(ctx, "main", base); !errors.Is(err, ErrCurrentBranch) {
		t.Fatalf("current ResetBranch error = %v", err)
	}
	if err := repo.ResetBranch(ctx, "reset-me", base); err != nil {
		t.Fatalf("ResetBranch: %v", err)
	}
	if got := r.git("rev-parse", "refs/heads/reset-me"); got != base {
		t.Fatalf("reset branch = %q, want %q", got, base)
	}
	if err := repo.DeleteRemoteBranch(ctx, "origin", "remote-topic"); err != nil {
		t.Fatalf("DeleteRemoteBranch: %v", err)
	}
	if got := remote.git("for-each-ref", "--format=%(refname)", "refs/heads/remote-topic"); got != "" {
		t.Fatalf("remote branch remains: %q", got)
	}
}

func TestBranchWorkflowMutationsAreRecorded(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	if err := repo.CreateBranchOnly(ctx, "recorded", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(records[0].Args) < 2 || records[0].Args[0] != "branch" {
		t.Fatalf("process records = %#v", records)
	}
	if _, err := repo.LocalBranchDeletePreflight(ctx, "recorded"); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("read-only preflight recorded a process: %#v", records)
	}
}
