package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPushSetUpstream(t *testing.T) {
	local := newTestRepo(t)
	local.write("tracked.txt", "base\n")
	wantID := local.commitAll("base")
	remote := newBareTestRepo(t)
	local.git("remote", "add", "company", remote.dir)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) {
		records = append(records, record)
	})
	if err := repo.PushSetUpstream(ctx, "company"); err != nil {
		t.Fatalf("PushSetUpstream: %v", err)
	}

	if got := remote.git("rev-parse", "refs/heads/main"); got != wantID {
		t.Fatalf("remote main = %q, want %q", got, wantID)
	}
	if got := local.git("config", "--get", "branch.main.remote"); got != "company" {
		t.Fatalf("branch remote = %q, want company", got)
	}
	if got := local.git("config", "--get", "branch.main.merge"); got != "refs/heads/main" {
		t.Fatalf("branch merge ref = %q, want refs/heads/main", got)
	}
	if len(records) != 1 {
		t.Fatalf("process records = %#v, want one push", records)
	}
	if got := strings.Join(records[0].Args, " "); got != "push --set-upstream -- company main" {
		t.Fatalf("push args = %q", got)
	}
	if records[0].ExitCode != 0 {
		t.Fatalf("push record = %#v, want success", records[0])
	}
}

func TestPushSetUpstreamRejectsDetachedAndUnbornHEAD(t *testing.T) {
	t.Run("detached", func(t *testing.T) {
		local := newTestRepo(t)
		local.write("tracked.txt", "base\n")
		local.commitAll("base")
		local.git("checkout", "--detach")
		remote := newBareTestRepo(t)
		local.git("remote", "add", "company", remote.dir)
		repo, _ := Discover(local.dir)

		if err := repo.PushSetUpstream(context.Background(), "company"); err == nil || !strings.Contains(err.Error(), "detached") {
			t.Fatalf("PushSetUpstream error = %v, want detached HEAD error", err)
		}
	})

	t.Run("unborn", func(t *testing.T) {
		local := newTestRepo(t)
		remote := newBareTestRepo(t)
		local.git("remote", "add", "company", remote.dir)
		repo, _ := Discover(local.dir)

		if err := repo.PushSetUpstream(context.Background(), "company"); err == nil || !strings.Contains(err.Error(), "unborn") {
			t.Fatalf("PushSetUpstream error = %v, want unborn branch error", err)
		}
	})

	t.Run("unconfigured remote", func(t *testing.T) {
		local := newTestRepo(t)
		local.write("tracked.txt", "base\n")
		local.commitAll("base")
		repo, _ := Discover(local.dir)

		if err := repo.PushSetUpstream(context.Background(), "missing"); !errors.Is(err, ErrNoFetchRemote) {
			t.Fatalf("PushSetUpstream error = %v, want ErrNoFetchRemote", err)
		}
	})
}
