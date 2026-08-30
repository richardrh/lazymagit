package git

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRemoteChangePlanLinesDescribeRenameAndRemovalDeterministically(t *testing.T) {
	preflight := RemoteChangePreflight{
		Remote:                "origin",
		TrackingRefs:          []string{"refs/remotes/origin/main"},
		UsesRemotePushDefault: true,
		BranchPushRemotes:     []string{"topic"},
		BranchRemotes:         []string{"main"},
		BranchMerges:          []string{"branch.main.merge=refs/heads/main"},
		RemoteConfig:          []string{"remote.origin.url\x00https://example.test/repo"},
	}
	rename := RemoteChangePlanLines(ReviewedRemoteChange{Preflight: preflight, NewName: "upstream"})
	for _, want := range []string{
		"remote: origin",
		"new name: upstream",
		"tracking ref: refs/remotes/origin/main -> refs/remotes/upstream/main",
		"config remote.pushDefault: origin -> upstream",
		"config branch.topic.pushRemote: origin -> upstream",
		"config branch.main.remote: origin -> upstream",
		"upstream merge relationship: branch.main.merge=refs/heads/main",
		"remote config: remote.origin.url = https://example.test/repo",
	} {
		if !slices.Contains(rename, want) {
			t.Errorf("rename plan omitted %q: %#v", want, rename)
		}
	}
	removal := RemoteChangePlanLines(ReviewedRemoteChange{Preflight: preflight})
	for _, want := range []string{
		"delete tracking ref: refs/remotes/origin/main",
		"config remote.pushDefault: origin -> <unset>",
		"config branch.topic.pushRemote: origin -> <unset>",
		"config branch.main.remote: origin -> <unset>",
	} {
		if !slices.Contains(removal, want) {
			t.Errorf("removal plan omitted %q: %#v", want, removal)
		}
	}
}

func TestSameRemotePreflightListsChecksEveryList(t *testing.T) {
	base := RemoteChangePreflight{
		TrackingRefs:       []string{"tracking"},
		TrackingRefOIDs:    []string{"oid"},
		TrackingRefSymbols: []string{"symbol"},
		BranchPushRemotes:  []string{"push"},
		BranchRemotes:      []string{"remote"},
		BranchMerges:       []string{"merge"},
		RemoteConfig:       []string{"config"},
	}
	if !sameRemotePreflightLists(base, base) {
		t.Fatal("identical preflight lists did not match")
	}
	mutations := []func(*RemoteChangePreflight){
		func(p *RemoteChangePreflight) { p.TrackingRefs = []string{"changed"} },
		func(p *RemoteChangePreflight) { p.TrackingRefOIDs = []string{"changed"} },
		func(p *RemoteChangePreflight) { p.TrackingRefSymbols = []string{"changed"} },
		func(p *RemoteChangePreflight) { p.BranchPushRemotes = []string{"changed"} },
		func(p *RemoteChangePreflight) { p.BranchRemotes = []string{"changed"} },
		func(p *RemoteChangePreflight) { p.BranchMerges = []string{"changed"} },
		func(p *RemoteChangePreflight) { p.RemoteConfig = []string{"changed"} },
	}
	for i, mutate := range mutations {
		changed := base
		mutate(&changed)
		if sameRemotePreflightLists(base, changed) {
			t.Errorf("list mutation %d was treated as equal", i)
		}
	}
}

func TestRollbackRemoteChangeRestoresTrackingRefsAndSymbolicHead(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	oid := local.commitAll("base")
	local.git("remote", "add", "origin", newBareTestRepo(t).dir)
	local.git("update-ref", "refs/remotes/origin/main", oid)
	local.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}
	configPath, configBefore, configExisted, err := repo.snapshotConfigFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := RemoteChangePreflight{
		Remote:             "origin",
		TrackingRefOIDs:    []string{"refs/remotes/origin/main=" + oid},
		TrackingRefSymbols: []string{"refs/remotes/origin/HEAD=refs/remotes/origin/main"},
	}
	cause := errors.New("simulated remote mutation failure")
	if err := repo.rollbackRemoteChange(ctx, cause, configPath, configBefore, configExisted, before, "renamed"); !errors.Is(err, cause) {
		t.Fatalf("rollback result = %v", err)
	}
	if got := local.git("rev-parse", "refs/remotes/origin/main"); got != oid {
		t.Fatalf("restored tracking ref = %q, want %q", got, oid)
	}
	if got := local.git("symbolic-ref", "refs/remotes/origin/HEAD"); got != "refs/remotes/origin/main" {
		t.Fatalf("restored symbolic head = %q", got)
	}
}

func TestReviewedRemoteRenameAndRemovalRejectStaleConfiguration(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", newBareTestRepo(t).dir)
	repo, _ := Discover(local.dir)

	rename, err := repo.ReviewRemoteRename(ctx, "origin", "upstream")
	if err != nil {
		t.Fatal(err)
	}
	local.git("config", "remote.pushDefault", "origin")
	if err := repo.RenameRemoteReviewed(ctx, rename); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("rename stale error = %v", err)
	}
	if got := local.git("remote"); got != "origin" {
		t.Fatalf("stale rename mutated remote: %q", got)
	}

	remove, err := repo.ReviewRemoteRemoval(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	local.git("config", "branch.main.pushRemote", "origin")
	if err := repo.RemoveRemoteReviewed(ctx, remove); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("remove stale error = %v", err)
	}
	if got := local.git("remote"); got != "origin" {
		t.Fatalf("stale removal mutated remote: %q", got)
	}
}

func TestReviewedRemoteConfigurationPreservesNilEmptyAndRejectsStalePlan(t *testing.T) {
	ctx := context.Background()
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", newBareTestRepo(t).dir)
	local.git("config", "--add", "remote.origin.push", "refs/heads/main:refs/heads/main")
	repo, _ := Discover(local.dir)

	plan, err := repo.ReviewRemoteConfiguration(ctx, RemoteConfigArgs{Remote: "origin", FetchRefspecs: nil, PushRefspecs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Args.FetchRefspecs != nil || plan.Args.PushRefspecs == nil {
		t.Fatalf("nil/empty distinction lost: %#v", plan.Args)
	}
	local.git("config", "remote.origin.tagopt", "--no-tags")
	if err := repo.ConfigureRemoteReviewed(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("configuration stale error = %v", err)
	}
	if got := local.git("config", "--get-all", "remote.origin.push"); !strings.Contains(got, "refs/heads/main") {
		t.Fatalf("stale configuration cleared push refspec: %q", got)
	}

	follow := RemoteFollowRemoteHEADAlways
	plan, err = repo.ReviewRemoteConfiguration(ctx, RemoteConfigArgs{Remote: "origin", PushRefspecs: []string{}, FollowRemoteHEAD: &follow})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Changes, "\n"), "followRemoteHEAD: <unset> -> always") {
		t.Fatalf("followRemoteHEAD review = %#v", plan.Changes)
	}
	if err := repo.ConfigureRemoteReviewed(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if out, err := repo.configValues(ctx, "remote.origin.push"); err != nil || out != nil {
		t.Fatalf("clear result = %#v, %v", out, err)
	}
	if got, err := repo.RemoteConfiguration(ctx, "origin"); err != nil || got.FollowRemoteHEAD == nil || *got.FollowRemoteHEAD != RemoteFollowRemoteHEADAlways {
		t.Fatalf("followRemoteHEAD = %#v, %v", got.FollowRemoteHEAD, err)
	}
	defaultFollow := RemoteFollowRemoteHEADDefault
	if err := repo.ConfigureRemote(ctx, RemoteConfigArgs{Remote: "origin", FollowRemoteHEAD: &defaultFollow}); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.RemoteConfiguration(ctx, "origin"); err != nil || got.FollowRemoteHEAD != nil {
		t.Fatalf("cleared followRemoteHEAD = %#v, %v", got.FollowRemoteHEAD, err)
	}
	invalidFollow := RemoteFollowRemoteHEAD(99)
	if err := repo.ConfigureRemote(ctx, RemoteConfigArgs{Remote: "origin", FollowRemoteHEAD: &invalidFollow}); err == nil {
		t.Fatal("invalid followRemoteHEAD was accepted")
	}
	if _, err := repo.ReviewRemoteConfiguration(ctx, RemoteConfigArgs{Remote: "origin", FollowRemoteHEAD: &invalidFollow}); err == nil {
		t.Fatal("review accepted invalid followRemoteHEAD")
	}
}

func TestReviewedRemoteDefaultBranchUsesAdvertisedSymbolicHead(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base", "base\n")
	seed.commitAll("base")
	seed.git("remote", "add", "origin", remote.dir)
	seed.git("push", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	local := cloneTestRepo(t, remote.dir)
	repo, _ := Discover(local.dir)
	local.git("symbolic-ref", "-d", "refs/remotes/origin/HEAD")

	plan, err := repo.ReviewRemoteDefaultBranch(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if plan.NewRef != "refs/remotes/origin/main" {
		t.Fatalf("new ref = %q", plan.NewRef)
	}
	if err := repo.UpdateRemoteDefaultBranch(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if got := local.git("symbolic-ref", "refs/remotes/origin/HEAD"); got != plan.NewRef {
		t.Fatalf("tracking HEAD = %q", got)
	}
}
