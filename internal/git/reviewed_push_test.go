package git

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestReviewedPushFreezesExplicitSourceAndRejectsStalePlan(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	oid := local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := repo.ReviewPush(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", Source: "main", Destination: "reviewed"}})
	if err != nil {
		t.Fatalf("review push: %v", err)
	}
	wantRefspec := oid + ":refs/heads/reviewed"
	if plan.Remote != "origin" || plan.SourceRef != "main" || plan.SourceOID != oid || !reflect.DeepEqual(plan.Refspecs, []string{wantRefspec}) {
		t.Fatalf("review plan = %#v", plan)
	}
	if !reflect.DeepEqual(plan.Sources, []ReviewedPushSource{{Ref: "main", OID: oid}}) {
		t.Fatalf("review sources = %#v", plan.Sources)
	}
	if got := plan.Argv[len(plan.Argv)-1]; got != wantRefspec {
		t.Fatalf("frozen argv refspec = %q, want %q", got, wantRefspec)
	}
	if err := repo.ExecuteReviewedPushWithPushRemote(ctx, plan, "other"); err == nil {
		t.Fatal("mismatched reviewed push remote was accepted")
	}
	if err := repo.ExecuteReviewedPushWithPushRemote(ctx, plan, "origin"); err != nil {
		t.Fatalf("execute reviewed push: %v", err)
	}
	if got := local.git("config", "--get", "branch.main.pushRemote"); got != "origin" {
		t.Fatalf("configured push remote = %q", got)
	}
	if got := remote.git("rev-parse", "refs/heads/reviewed"); got != oid {
		t.Fatalf("reviewed remote tip = %q, want %q", got, oid)
	}

	local.write("next", "next\n")
	local.commitAll("next")
	if err := repo.ExecuteReviewedPush(ctx, plan); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale reviewed push error = %v", err)
	}
}

func TestReviewedPushBindsEveryExplicitNonPrimarySource(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("tag", "v1")
	local.git("remote", "add", "origin", remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := repo.ReviewPush(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin"}, Refspecs: []string{"main:reviewed", "refs/tags/v1:refs/tags/v1"}})
	if err != nil {
		t.Fatalf("review multi-source push: %v", err)
	}
	if !reflect.DeepEqual(plan.Sources, []ReviewedPushSource{{Ref: "refs/tags/v1", OID: local.git("rev-parse", "refs/tags/v1")}}) {
		t.Fatalf("reviewed explicit sources = %#v", plan.Sources)
	}
	if got := plan.Refspecs[1]; got != plan.Sources[0].OID+":refs/tags/v1" {
		t.Fatalf("frozen tag refspec = %q", got)
	}
}

func TestReviewedPushFreezesTagNamespace(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("tag", "v1")
	local.git("remote", "add", "origin", remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := repo.ReviewPush(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", AllTags: true}})
	if err != nil {
		t.Fatalf("review all tags: %v", err)
	}
	if plan.SourceNamespace != "refs/tags" || len(plan.Sources) != 1 || plan.Sources[0].Ref != "refs/tags/v1" || plan.Sources[0].OID == "" {
		t.Fatalf("tag review plan = %#v", plan)
	}
	if err := repo.ExecuteReviewedPush(ctx, plan); err != nil {
		t.Fatalf("execute reviewed tags: %v", err)
	}
	if got := remote.git("rev-parse", "refs/tags/v1"); got != plan.Sources[0].OID {
		t.Fatalf("reviewed tag = %q, want %q", got, plan.Sources[0].OID)
	}
}
