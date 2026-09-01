package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTransferWorkflowFetchPushAndRecording(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	repo, _ := Discover(local.dir)
	var records []ProcessRecord
	ctx = WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	if err := repo.PushWithArgs(ctx, PushArgs{Target: PushElsewhere, Remote: "origin", Source: "main", Destination: "published", SetUpstream: true}); err != nil {
		t.Fatalf("push: %v", err)
	}
	if got := remote.git("rev-parse", "refs/heads/published"); got != local.git("rev-parse", "HEAD") {
		t.Fatalf("published tip = %q", got)
	}
	if err := repo.FetchWithArgs(ctx, FetchArgs{Remote: "origin", Prune: true, Refspec: "+refs/heads/published:refs/remotes/origin/copied"}); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got := local.git("rev-parse", "refs/remotes/origin/copied"); got == "" {
		t.Fatal("fetch did not create tracking ref")
	}
	if len(records) != 2 || records[0].Args[0] != "push" || records[1].Args[0] != "fetch" {
		t.Fatalf("process records = %#v", records)
	}
}

func TestTransferWorkflowRemoteCleanupAndPrunePlan(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "origin", "main:stale")
	local.git("fetch", "origin")
	remote.git("update-ref", "-d", "refs/heads/stale")
	local.git("config", "remote.pushDefault", "origin")
	local.git("config", "branch.main.pushRemote", "origin")
	repo, _ := Discover(local.dir)
	plan, err := repo.RemotePruneComparison(ctx, "origin")
	if err != nil || len(plan.StaleTrackingRefs) != 1 {
		t.Fatalf("prune plan = %#v, %v", plan, err)
	}
	if _, err := repo.PruneRemote(ctx, plan, plan.Token); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := repo.RemoveRemoteWithArgs(ctx, RemoveRemoteArgs{Remote: "origin"}); err == nil {
		t.Fatal("remove without confirmation succeeded")
	}
	p, err := repo.RemoveRemoteWithArgs(ctx, RemoveRemoteArgs{Remote: "origin", Confirm: true})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !p.UsesRemotePushDefault || len(p.BranchPushRemotes) != 1 {
		t.Fatalf("remove preflight = %#v", p)
	}
	if got := local.git("config", "--list"); strings.Contains(got, "remote.pushdefault=") || strings.Contains(got, "branch.main.pushremote=") {
		t.Fatalf("push configuration remains in %q", got)
	}
}

func TestTransferWorkflowPullAndRemoteConfiguration(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base", "base\n")
	seed.commitAll("base")
	seed.git("remote", "add", "origin", remote.dir)
	seed.git("push", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	local := cloneTestRepo(t, remote.dir)
	peer := cloneTestRepo(t, remote.dir)
	peer.write("peer", "peer\n")
	peer.commitAll("peer")
	peer.git("push", "origin", "main")
	repo, _ := Discover(local.dir)
	if err := repo.PullWithArgs(ctx, PullArgs{Target: PullRemoteBranch, Remote: "origin", Branch: "main", Mode: PullFFOnly}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if local.git("rev-parse", "HEAD") != peer.git("rev-parse", "HEAD") {
		t.Fatal("pull did not fast-forward")
	}
	url := remote.dir
	pushURL := remote.dir
	tagOpt := RemoteTagsNone
	if err := repo.ConfigureRemote(ctx, RemoteConfigArgs{
		Remote: "origin", FetchURL: &url, PushURL: &pushURL,
		FetchRefspecs: []string{"+refs/heads/*:refs/remotes/origin/*"},
		PushRefspecs:  []string{"refs/heads/main:refs/heads/published"}, TagOpt: &tagOpt,
	}); err != nil {
		t.Fatalf("configure remote: %v", err)
	}
	if got := local.git("config", "--get", "remote.origin.tagOpt"); got != "--no-tags" {
		t.Fatalf("tagOpt = %q", got)
	}
	allTags := RemoteTagsAll
	if err := repo.ConfigureRemote(ctx, RemoteConfigArgs{Remote: "origin", TagOpt: &allTags}); err != nil {
		t.Fatal(err)
	}
	if got := local.git("config", "--get", "remote.origin.tagOpt"); got != "--tags" {
		t.Fatalf("all tagOpt = %q", got)
	}
	defaultTags := RemoteTagsDefault
	if err := repo.ConfigureRemote(ctx, RemoteConfigArgs{Remote: "origin", TagOpt: &defaultTags}); err != nil {
		t.Fatal(err)
	}
	if _, set, err := repo.configValue(ctx, "remote.origin.tagOpt"); err != nil || set {
		t.Fatalf("default tagOpt remained set: %v, %v", set, err)
	}
	p, err := repo.RenameRemote(ctx, RenameRemoteArgs{Old: "origin", New: "mirror"})
	if err != nil {
		t.Fatalf("rename remote: %v", err)
	}
	if p.Remote != "origin" || local.git("remote") != "mirror" {
		t.Fatalf("rename result = %#v, remotes %q", p, local.git("remote"))
	}
}

func TestConfigureRemoteExtractedHelpersPreserveNilEmptyAndTagOptions(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", newBareTestRepo(t).dir)
	local.git("config", "--add", "remote.origin.fetch", "old-fetch")
	local.git("config", "--add", "remote.origin.push", "old-push")
	repo, _ := Discover(local.dir)

	if err := repo.replaceRemoteConfigValues(ctx, "remote.origin.fetch", nil); err != nil {
		t.Fatal(err)
	}
	if got := local.git("config", "--get-all", "remote.origin.fetch"); !strings.Contains(got, "old-fetch") {
		t.Fatalf("nil replacement changed fetch values: %q", got)
	}
	if err := repo.replaceRemoteConfigValues(ctx, "remote.origin.push", []string{}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.configValue(ctx, "remote.origin.push"); err != nil || ok {
		t.Fatalf("empty replacement did not clear push values: set=%v err=%v", ok, err)
	}

	all := RemoteTagsAll
	if err := repo.configureRemoteTagOpt(ctx, "origin", &all); err != nil {
		t.Fatal(err)
	}
	if got := local.git("config", "--get", "remote.origin.tagOpt"); got != "--tags" {
		t.Fatalf("tag option = %q", got)
	}
	defaultTags := RemoteTagsDefault
	if err := repo.configureRemoteTagOpt(ctx, "origin", &defaultTags); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.configValue(ctx, "remote.origin.tagOpt"); err != nil || ok {
		t.Fatalf("default tag option remained set: set=%v err=%v", ok, err)
	}
}

func TestResolvePullTargetSelectsUpstreamPushRemoteAndExplicitBranch(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base", "base\n")
	seed.commitAll("base")
	seed.git("remote", "add", "origin", remote.dir)
	seed.git("push", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	local := cloneTestRepo(t, remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args PullArgs
	}{
		{"upstream", PullArgs{Target: PullUpstream}},
		{"explicit", PullArgs{Target: PullRemoteBranch, Remote: "origin", Branch: "main"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote, branch, err := repo.resolvePullTarget(ctx, test.args)
			if err != nil || remote != "origin" || branch != "main" {
				t.Fatalf("resolve %s = (%q, %q, %v)", test.name, remote, branch, err)
			}
		})
	}

	local.git("config", "remote.pushDefault", "origin")
	remoteName, branch, err := repo.resolvePullTarget(ctx, PullArgs{Target: PullPushRemote})
	if err != nil || remoteName != "origin" || branch != "main" {
		t.Fatalf("resolve push remote = (%q, %q, %v)", remoteName, branch, err)
	}
	if _, _, err := repo.resolvePullTarget(ctx, PullArgs{Target: PullRemoteBranch, Remote: "origin"}); err == nil {
		t.Fatal("explicit pull without a branch was accepted")
	}
	if _, _, err := repo.resolvePullTarget(ctx, PullArgs{Target: PullTarget(99)}); err == nil {
		t.Fatal("unknown pull target was accepted")
	}
	local.git("checkout", "--detach")
	if _, _, err := repo.resolvePullTarget(ctx, PullArgs{Target: PullPushRemote}); err == nil {
		t.Fatal("pulling a detached HEAD from the push remote was accepted")
	}

	missing := newTestRepo(t)
	missing.write("base", "base\n")
	missing.commitAll("base")
	missingRepo, err := Discover(missing.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := missingRepo.resolvePullTarget(ctx, PullArgs{Target: PullUpstream}); !errors.Is(err, ErrNoUpstream) {
		t.Fatalf("missing upstream error = %v", err)
	}
}

func TestResolvePushTargetSelectsUpstreamPushRemoteAndExplicitRemote(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base", "base\n")
	seed.commitAll("base")
	seed.git("remote", "add", "origin", remote.dir)
	seed.git("push", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	local := cloneTestRepo(t, remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		target PushTarget
		remote string
	}{
		{"upstream", PushToUpstream, "origin"},
		{"elsewhere", PushElsewhere, "origin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := repo.resolvePushTarget(ctx, test.target, test.remote)
			if err != nil || got != "origin" {
				t.Fatalf("resolve %s = (%q, %v)", test.name, got, err)
			}
		})
	}

	local.git("config", "remote.pushDefault", "origin")
	if got, err := repo.resolvePushTarget(ctx, PushToPushRemote, ""); err != nil || got != "origin" {
		t.Fatalf("resolve push remote = (%q, %v)", got, err)
	}
	if _, err := repo.resolvePushTarget(ctx, PushElsewhere, ""); err == nil {
		t.Fatal("empty explicit push remote was accepted")
	}
	if _, err := repo.resolvePushTarget(ctx, PushTarget(99), "origin"); err == nil {
		t.Fatal("unknown push target was accepted")
	}
}

func TestPushSpecCompilesEverySupportedSelector(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("tag", "v1")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "-u", "origin", "main")
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name     string
		args     PushArgs
		wantSpec string
		allTags  bool
	}{
		{"refspec", PushArgs{Refspec: "main:copy"}, "main:copy", false},
		{"source destination", PushArgs{Source: "main", Destination: "copy"}, "main:copy", false},
		{"source", PushArgs{Source: "main"}, "main", false},
		{"matching", PushArgs{Matching: true}, ":", false},
		{"tag", PushArgs{Tag: "v1"}, "refs/tags/v1", false},
		{"all tags", PushArgs{AllTags: true}, "", true},
		{"notes", PushArgs{Notes: true}, "refs/notes/*:refs/notes/*", false},
		{"current branch", PushArgs{Target: PushElsewhere}, "main", false},
		{"upstream branch", PushArgs{Target: PushToUpstream}, "main:main", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec, allTags, err := repo.pushSpec(ctx, test.args)
			if err != nil || spec != test.wantSpec || allTags != test.allTags {
				t.Fatalf("push spec = (%q, %t, %v), want (%q, %t)", spec, allTags, err, test.wantSpec, test.allTags)
			}
		})
	}
	if _, _, err := repo.pushSpec(ctx, PushArgs{Destination: "copy"}); err == nil {
		t.Fatal("destination without a source was accepted")
	}
}

func TestTransferWorkflowTagsAndMergeSafety(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	repo, _ := Discover(r.dir)
	if _, err := repo.CreateTagWithArgs(ctx, CreateTagArgs{Name: "v1", Annotated: true, Message: "release secret"}); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	tags, err := repo.ListTags(ctx)
	if err != nil || len(tags) != 1 || !tags[0].Annotated || tags[0].TargetID != base {
		t.Fatalf("tags = %#v, %v", tags, err)
	}
	r.git("switch", "-c", "topic")
	r.write("topic", "topic\n")
	r.commitAll("topic")
	r.git("switch", "main")
	r.write("dirty", "dirty\n")
	p, err := repo.MergeWithArgs(ctx, MergeArgs{Target: "topic", Mode: MergeFFOnly})
	if err == nil || !p.RequiresDirtyConfirmation {
		t.Fatalf("dirty merge = %#v, %v", p, err)
	}
	if _, err := repo.MergeWithArgs(ctx, MergeArgs{Target: "topic", Mode: MergeFFOnly, ConfirmDirty: true}); err != nil {
		t.Fatalf("confirmed merge: %v", err)
	}
	if got := r.git("rev-parse", "HEAD"); got == base {
		t.Fatal("merge did not advance HEAD")
	}
	if _, err := repo.DeleteTags(ctx, DeleteTagsArgs{Names: []string{"v1"}}); err == nil {
		t.Fatal("tag deletion without confirmation succeeded")
	}
	if _, err := repo.DeleteTags(ctx, DeleteTagsArgs{Names: []string{"v1"}, Confirm: true}); err != nil {
		t.Fatalf("delete tag: %v", err)
	}
	state, err := repo.MergeState(ctx)
	if err != nil || state.InProgress {
		t.Fatalf("merge state = %#v, %v", state, err)
	}
}

func TestTransferWorkflowRejectsOptionLikeNamesBeforeMutation(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	err := repo.FetchWithArgs(context.Background(), FetchArgs{Remote: "--all"})
	if err == nil || errors.As(err, new(*CommandError)) {
		t.Fatalf("error = %T %v; want validation error", err, err)
	}
}
