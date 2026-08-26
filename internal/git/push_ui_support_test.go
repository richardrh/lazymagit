package git

import (
	"context"
	"os/exec"
	"reflect"
	"testing"
)

func TestPushUIExactMultipleRefspecsAndOptions(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	remote.git("config", "receive.advertisePushOptions", "true")
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	repo, _ := Discover(local.dir)
	var records []ProcessRecord
	ctx = WithProcessRecorder(ctx, func(record ProcessRecord) { records = append(records, record) })
	in := PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", NoVerify: true, SetUpstream: true, FollowTags: true, PushOptions: []string{"ci.skip", "topic=two words"}}, Refspecs: []string{"main:one", "main:two"}}
	if err := repo.PushWithUIArgs(ctx, in); err != nil {
		t.Fatalf("push multiple refspecs: %v", err)
	}
	want := []string{"push", "--no-verify", "--set-upstream", "--follow-tags", "--push-option=ci.skip", "--push-option=topic=two words", "--", "origin", "main:one", "main:two"}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
		t.Fatalf("process args = %#v, want %#v", records, want)
	}
	for _, branch := range []string{"one", "two"} {
		if remote.git("rev-parse", "refs/heads/"+branch) != local.git("rev-parse", "HEAD") {
			t.Fatalf("remote branch %s was not pushed", branch)
		}
	}
}

func TestPushUISingularNotesDryRunAndLease(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	base := local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	local.git("notes", "--ref=review", "add", "-m", "reviewed", base)
	repo, _ := Discover(local.dir)
	refs, err := repo.ListNotesRefs(ctx)
	if err != nil || !reflect.DeepEqual(refs, []string{"refs/notes/review"}) {
		t.Fatalf("notes refs = %#v, %v", refs, err)
	}
	if err := repo.PushWithUIArgs(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin"}, NotesRef: refs[0]}); err != nil {
		t.Fatalf("push one notes ref: %v", err)
	}
	if remote.git("rev-parse", "refs/notes/review") == "" {
		t.Fatal("selected notes ref was not pushed")
	}
	if err := repo.PushWithUIArgs(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", Source: "main", Destination: "dry-only", DryRun: true}}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if testRefExists(remote.dir, "refs/heads/dry-only") {
		t.Fatal("dry run mutated remote")
	}
	if err := repo.PushWithUIArgs(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", Source: "main", Destination: "leased"}}); err != nil {
		t.Fatalf("seed lease branch: %v", err)
	}
	local.git("fetch", "origin")
	local.write("next", "next\n")
	local.commitAll("next")
	if err := repo.PushWithUIArgs(ctx, PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin", Source: "main", Destination: "leased", Force: PushForceWithLease}}); err != nil {
		t.Fatalf("lease push: %v", err)
	}
}

func TestValidatePushUIArgsHasNoMutation(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	repo, _ := Discover(local.dir)
	in := PushUIArgs{PushArgs: PushArgs{Target: PushElsewhere, Remote: "origin"}, Refspecs: []string{"main:validated", "main:also-validated"}}
	if err := repo.ValidatePushUIArgs(ctx, in); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if testRefExists(remote.dir, "refs/heads/validated") {
		t.Fatal("validation ran push")
	}
}

func testRefExists(dir, ref string) bool {
	return exec.Command("git", "-C", dir, "show-ref", "--verify", "--quiet", ref).Run() == nil
}
