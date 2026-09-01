package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFetchChoicesDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	oid := r.commitAll("base")
	r.git("remote", "add", "origin", "https://example.invalid/origin")
	r.git("remote", "add", "team/origin", "https://example.invalid/team")
	r.git("update-ref", "refs/remotes/origin/main", oid)
	r.git("update-ref", "refs/remotes/team/origin/topic", oid)
	r.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	choices, err := repo.FetchChoices(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	wantBranches := []FetchRemoteBranch{{Remote: "origin", Branch: "main"}, {Remote: "team/origin", Branch: "topic"}}
	if !reflect.DeepEqual(choices.RemoteBranches, wantBranches) {
		t.Fatalf("remote branches = %#v, want %#v", choices.RemoteBranches, wantBranches)
	}
	if _, err := repo.FetchChoices(context.Background(), 0); err == nil {
		t.Fatal("non-positive fetch choice limit succeeded")
	}
}

func TestReviewedBisectUIDispatchHandlesEveryAction(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	requests := []HistoryUIRequest{
		{Action: HistoryUIBisectStart, Bisect: BisectStartOptions{Good: "HEAD"}},
		{Action: HistoryUIBisectGood, Revision: "missing"},
		{Action: HistoryUIBisectBad, Revision: "missing"},
		{Action: HistoryUIBisectSkip, Revision: "missing"},
		{Action: HistoryUIBisectReset},
	}
	for _, request := range requests {
		if handled, _ := repo.executeReviewedBisectUIAction(ctx, request); !handled {
			t.Fatalf("bisect action %q was not handled", request.Action)
		}
	}
	if handled, err := repo.executeReviewedBisectUIAction(ctx, HistoryUIRequest{}); handled || err != nil {
		t.Fatalf("invalid bisect action = handled %t, err %v", handled, err)
	}
}

func TestRemoteFollowRemoteHEADStringDirect(t *testing.T) {
	for value, want := range map[RemoteFollowRemoteHEAD]string{
		RemoteFollowRemoteHEADDefault: "default",
		RemoteFollowRemoteHEADNever:   "never",
		RemoteFollowRemoteHEADCreate:  "create",
		RemoteFollowRemoteHEADWarn:    "warn",
		RemoteFollowRemoteHEADAlways:  "always",
		RemoteFollowRemoteHEAD(255):   "invalid",
	} {
		if got := value.String(); got != want {
			t.Errorf("RemoteFollowRemoteHEAD(%d).String() = %q, want %q", value, got, want)
		}
	}
}

func TestValidateRemoteConfigArgsDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	r.git("remote", "add", "origin", "https://example.invalid/origin")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fetchURL, pushURL := "https://example.invalid/fetch", "ssh://example.invalid/push"
	tags, follow := RemoteTagsAll, RemoteFollowRemoteHEADWarn
	valid := RemoteConfigArgs{
		Remote:           "origin",
		FetchURL:         &fetchURL,
		PushURL:          &pushURL,
		FetchRefspecs:    []string{"refs/heads/*:refs/remotes/origin/*"},
		PushRefspecs:     []string{"main:reviewed"},
		TagOpt:           &tags,
		FollowRemoteHEAD: &follow,
	}
	if err := repo.validateRemoteConfigArgs(ctx, valid); err != nil {
		t.Fatalf("valid remote config: %v", err)
	}
	badURL := "bad\nurl"
	badTags := RemoteTagOpt(255)
	badFollow := RemoteFollowRemoteHEAD(255)
	for name, mutate := range map[string]func(*RemoteConfigArgs){
		"remote":        func(in *RemoteConfigArgs) { in.Remote = "missing" },
		"url":           func(in *RemoteConfigArgs) { in.FetchURL = &badURL },
		"fetch refspec": func(in *RemoteConfigArgs) { in.FetchRefspecs = []string{"main"} },
		"push refspec":  func(in *RemoteConfigArgs) { in.PushRefspecs = []string{"missing:reviewed"} },
		"tag option":    func(in *RemoteConfigArgs) { in.TagOpt = &badTags },
		"follow option": func(in *RemoteConfigArgs) { in.FollowRemoteHEAD = &badFollow },
	} {
		t.Run(name, func(t *testing.T) {
			in := valid
			mutate(&in)
			if err := repo.validateRemoteConfigArgs(ctx, in); err == nil {
				t.Fatal("invalid remote config succeeded")
			}
		})
	}
}

func TestReadMutatedHEADDirect(t *testing.T) {
	oid := strings.Repeat("a", 40)
	t.Run("detached", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte(oid+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := (&Repository{gitDir: dir}).readMutatedHEAD()
		if err != nil || got != oid {
			t.Fatalf("readMutatedHEAD = %q, %v", got, err)
		}
	})
	t.Run("symbolic", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "refs", "heads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "refs", "heads", "main"), []byte(oid+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := (&Repository{gitDir: dir}).readMutatedHEAD()
		if err != nil || got != oid {
			t.Fatalf("readMutatedHEAD = %q, %v", got, err)
		}
	})
	for name, head := range map[string]string{"unsafe symbolic": "ref: ../outside", "invalid oid": "not-an-oid"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte(head), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := (&Repository{gitDir: dir}).readMutatedHEAD(); err == nil {
				t.Fatal("invalid HEAD succeeded")
			}
		})
	}
}

func TestRenameRemoteDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	r.git("remote", "add", "origin", "https://example.invalid/origin")
	r.git("config", "remote.pushDefault", "origin")
	r.git("config", "branch.main.pushRemote", "origin")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := repo.RenameRemote(context.Background(), RenameRemoteArgs{Old: "origin", New: "mirror"})
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.UsesRemotePushDefault || len(preflight.BranchPushRemotes) != 1 {
		t.Fatalf("rename preflight = %#v", preflight)
	}
	if got := strings.TrimSpace(r.git("config", "remote.pushDefault")); got != "mirror" {
		t.Fatalf("remote.pushDefault = %q", got)
	}
	if _, err := repo.RenameRemote(context.Background(), RenameRemoteArgs{Old: "missing", New: "other"}); err == nil {
		t.Fatal("missing remote rename succeeded")
	}
}

func TestEnsurePathParentWithinDirect(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "new", "child")
	if err := ensurePathParentWithin(root, inside); err != nil {
		t.Fatalf("inside path rejected: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := ensurePathParentWithin(root, filepath.Join(root, "escape", "child")); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("symlink escape error = %v", err)
	}
}

func TestRenamePathDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("tracked", "tracked\n")
	r.commitAll("base")
	r.write("untracked", "untracked\n")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(r.dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.RenamePath(ctx, "tracked", "nested/tracked"); err != nil {
		t.Fatalf("rename tracked path: %v", err)
	}
	if err := repo.RenamePath(ctx, "untracked", "nested/untracked"); err != nil {
		t.Fatalf("rename untracked path: %v", err)
	}
	if err := repo.RenamePath(ctx, "missing", "other"); err == nil {
		t.Fatal("missing source rename succeeded")
	}
}

func TestReviewPushDirect(t *testing.T) {
	remote := newBareTestRepo(t)
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	r.git("remote", "add", "origin", remote.dir)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repo.ReviewPush(context.Background(), PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", Source: "main", Destination: "reviewed"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Remote != "origin" || plan.SourceOID == "" || len(plan.Refspecs) != 1 || plan.Token == (ConfirmationToken{}) {
		t.Fatalf("reviewed push = %#v", plan)
	}
}

func TestRebaseStartDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RebaseStart(context.Background(), RebaseOptions{}); err == nil {
		t.Fatal("empty upstream rebase succeeded")
	}
	if err := repo.RebaseStart(context.Background(), RebaseOptions{Upstream: "HEAD", Strategy: "bad\nstrategy"}); err == nil {
		t.Fatal("invalid strategy rebase succeeded")
	}
	if err := repo.RebaseStart(context.Background(), RebaseOptions{Upstream: "missing"}); err == nil {
		t.Fatal("missing upstream rebase succeeded")
	}
	if err := repo.RebaseStart(context.Background(), RebaseOptions{Upstream: "HEAD"}); err != nil {
		t.Fatalf("no-op rebase: %v", err)
	}
}

func TestReviewGitCommandDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plan, err := repo.ReviewGitCommand(ctx, []string{"status", "--short"}, false)
	if err != nil || plan.ExternalGit() {
		t.Fatalf("review builtin = %#v, %v", plan, err)
	}
	r.git("config", "alias.mine", "status")
	if _, err := repo.ReviewGitCommand(ctx, []string{"mine"}, true); !errors.Is(err, ErrRawCommandAlias) {
		t.Fatalf("alias error = %v", err)
	}
	if _, err := repo.ReviewGitCommand(ctx, []string{"unknown-helper"}, false); !errors.Is(err, ErrExternalGitCommand) {
		t.Fatalf("external error = %v", err)
	}
	for _, argv := range [][]string{{}, {"-c"}, {"git"}, {"credential"}, {"bisect", "run", "true"}} {
		if _, err := repo.ReviewGitCommand(ctx, argv, false); err == nil {
			t.Errorf("invalid argv %#v succeeded", argv)
		}
	}
}

func TestResetPreflightDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	oid := r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, mode := range []ResetMode{ResetMixed, ResetSoft, ResetHard, ResetKeep, ResetIndex, ResetWorktree, ResetFile} {
		opts := ResetOptions{Mode: mode, Target: "HEAD"}
		if mode == ResetFile {
			opts.Paths = []string{"base"}
		}
		preflight, err := repo.ResetPreflight(ctx, opts)
		if err != nil || preflight.Target != oid {
			t.Errorf("ResetPreflight(%d) = %#v, %v", mode, preflight, err)
		}
	}
	for _, opts := range []ResetOptions{{Mode: ResetMode(255)}, {Mode: ResetFile}, {Mode: ResetHard, Paths: []string{"base"}}} {
		if _, err := repo.ResetPreflight(ctx, opts); err == nil {
			t.Errorf("invalid reset %#v succeeded", opts)
		}
	}
}

func TestValidateRefspecDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, test := range []struct {
		spec  string
		fetch bool
		valid bool
	}{
		{"refs/heads/main:refs/remotes/origin/main", true, true},
		{"main:reviewed", false, true},
		{":reviewed", false, true},
		{"main", true, false},
		{"refs/heads/main:", true, false},
		{"missing:reviewed", false, false},
		{"main:bad..branch", false, false},
		{"a:b:c", false, false},
	} {
		if err := repo.validateRefspec(ctx, test.spec, test.fetch); (err == nil) != test.valid {
			t.Errorf("validateRefspec(%q, %t) = %v", test.spec, test.fetch, err)
		}
	}
}

func TestCloneRepositoryForUIDirect(t *testing.T) {
	source := newTestRepo(t)
	source.write("base", "base\n")
	source.commitAll("base")
	destination := filepath.Join(t.TempDir(), "clone")
	if err := CloneRepositoryForUI(context.Background(), source.dir, destination, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "base")); err != nil {
		t.Fatalf("cloned file: %v", err)
	}
	if err := CloneRepositoryForUI(context.Background(), source.dir, destination, CloneOptions{}); err == nil {
		t.Fatal("clone into non-empty destination succeeded")
	}
}

func TestAddWorktreeDirect(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	checkout := false
	path := filepath.Join(t.TempDir(), "linked")
	opts := WorktreeAddOptions{Detach: true, Force: Confirmed, Checkout: &checkout, Lock: true, LockReason: "reviewed"}
	if err := repo.AddWorktree(context.Background(), path, "HEAD", opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("worktree destination: %v", err)
	}
	if err := repo.AddWorktree(context.Background(), path, "HEAD", WorktreeAddOptions{}); err == nil {
		t.Fatal("duplicate worktree destination succeeded")
	}
}
