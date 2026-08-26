package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFetchUpstreamResolvesConfiguredAndPrimaryRemotes(t *testing.T) {
	ctx := context.Background()

	t.Run("configured branch remote", func(t *testing.T) {
		local := newTestRepo(t)
		one, oneID := seededBareRemote(t, "one")
		two, twoID := seededBareRemote(t, "two")
		local.git("remote", "add", "one", one.dir)
		local.git("remote", "add", "two", two.dir)
		local.git("config", "branch.main.remote", "two")
		repo, _ := Discover(local.dir)

		if err := repo.FetchUpstream(ctx); err != nil {
			t.Fatalf("FetchUpstream: %v", err)
		}
		assertOnlyRemoteFetched(t, local, "two", twoID)
		if oneID == twoID {
			t.Fatal("test remotes unexpectedly have the same tip")
		}
	})

	t.Run("sole remote", func(t *testing.T) {
		local := newTestRepo(t)
		remote, id := seededBareRemote(t, "sole")
		local.git("remote", "add", "company", remote.dir)
		repo, _ := Discover(local.dir)

		if err := repo.FetchUpstream(ctx); err != nil {
			t.Fatalf("FetchUpstream: %v", err)
		}
		assertOnlyRemoteFetched(t, local, "company", id)
	})

	t.Run("origin among multiple remotes", func(t *testing.T) {
		local := newTestRepo(t)
		origin, originID := seededBareRemote(t, "origin")
		other, _ := seededBareRemote(t, "other")
		local.git("remote", "add", "other", other.dir)
		local.git("remote", "add", "origin", origin.dir)
		repo, _ := Discover(local.dir)

		if err := repo.FetchUpstream(ctx); err != nil {
			t.Fatalf("FetchUpstream: %v", err)
		}
		assertOnlyRemoteFetched(t, local, "origin", originID)
	})

	t.Run("configured primary among multiple remotes", func(t *testing.T) {
		local := newTestRepo(t)
		primary, primaryID := seededBareRemote(t, "primary")
		other, _ := seededBareRemote(t, "other")
		local.git("remote", "add", "company", primary.dir)
		local.git("remote", "add", "other", other.dir)
		local.git("config", "magit.primaryRemote", "company")
		repo, _ := Discover(local.dir)

		if err := repo.FetchUpstream(ctx); err != nil {
			t.Fatalf("FetchUpstream: %v", err)
		}
		assertOnlyRemoteFetched(t, local, "company", primaryID)
	})

	t.Run("ambiguous remotes", func(t *testing.T) {
		local := newTestRepo(t)
		one, _ := seededBareRemote(t, "one")
		two, _ := seededBareRemote(t, "two")
		local.git("remote", "add", "one", one.dir)
		local.git("remote", "add", "two", two.dir)
		repo, _ := Discover(local.dir)

		err := repo.FetchUpstream(ctx)
		if err == nil || !strings.Contains(err.Error(), "fetch remote") {
			t.Fatalf("FetchUpstream error = %v, want a clear fetch-remote error", err)
		}
	})

	t.Run("explicit dot remote is not replaced", func(t *testing.T) {
		local := newTestRepo(t)
		fallback, _ := seededBareRemote(t, "fallback")
		local.git("remote", "add", "origin", fallback.dir)
		local.git("config", "branch.main.remote", ".")
		repo, _ := Discover(local.dir)

		err := repo.FetchUpstream(ctx)
		var commandErr *CommandError
		if !errors.As(err, &commandErr) || len(commandErr.Args) < 3 || commandErr.Args[len(commandErr.Args)-1] != "." {
			t.Fatalf("FetchUpstream error = %v, want Git error from fetching explicit dot remote", err)
		}
		if got := local.git("for-each-ref", "--format=%(refname)", "refs/remotes"); got != "" {
			t.Fatalf("fallback remote was fetched: %q", got)
		}
	})
}

func TestPushRemoteResolutionAndFetch(t *testing.T) {
	for _, test := range []struct {
		name       string
		configure  func(*testRepo)
		wantRemote string
	}{
		{
			name: "branch pushRemote",
			configure: func(local *testRepo) {
				local.git("config", "branch.main.remote", "upstream")
				local.git("config", "remote.pushDefault", "default")
				local.git("config", "branch.main.pushRemote", "push")
			},
			wantRemote: "push",
		},
		{
			name: "pushDefault",
			configure: func(local *testRepo) {
				local.git("config", "branch.main.remote", "upstream")
				local.git("config", "remote.pushDefault", "default")
			},
			wantRemote: "default",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			local := newTestRepo(t)
			ids := make(map[string]string)
			for _, name := range []string{"upstream", "default", "push"} {
				remote, id := seededBareRemote(t, test.name+name)
				local.git("remote", "add", name, remote.dir)
				ids[name] = id
			}
			test.configure(local)
			repo, _ := Discover(local.dir)

			got, err := repo.PushRemote(context.Background())
			if err != nil || got != test.wantRemote {
				t.Fatalf("PushRemote = %q, %v; want %q", got, err, test.wantRemote)
			}
			if err := repo.FetchPush(context.Background()); err != nil {
				t.Fatalf("FetchPush: %v", err)
			}
			assertOnlyRemoteFetched(t, local, test.wantRemote, ids[test.wantRemote])
		})
	}

	t.Run("unset does not fall back to upstream", func(t *testing.T) {
		local := newTestRepo(t)
		remote, _ := seededBareRemote(t, "upstream-only")
		local.git("remote", "add", "upstream", remote.dir)
		local.git("config", "branch.main.remote", "upstream")
		repo, _ := Discover(local.dir)
		if _, err := repo.PushRemote(context.Background()); !errors.Is(err, ErrNoFetchRemote) {
			t.Fatalf("PushRemote error = %v, want ErrNoFetchRemote", err)
		}
		if err := repo.FetchPush(context.Background()); !errors.Is(err, ErrNoFetchRemote) {
			t.Fatalf("FetchPush error = %v, want ErrNoFetchRemote", err)
		}
	})

	t.Run("invalid configured remote", func(t *testing.T) {
		local := newTestRepo(t)
		local.git("config", "branch.main.pushRemote", "missing")
		repo, _ := Discover(local.dir)
		if _, err := repo.PushRemote(context.Background()); !errors.Is(err, ErrNoFetchRemote) {
			t.Fatalf("PushRemote error = %v, want ErrNoFetchRemote", err)
		}
	})

	t.Run("detached branch", func(t *testing.T) {
		local := newTestRepo(t)
		local.write("base", "base\n")
		local.commitAll("base")
		remote, _ := seededBareRemote(t, "default")
		local.git("remote", "add", "default", remote.dir)
		local.git("config", "remote.pushDefault", "default")
		local.git("checkout", "--detach")
		repo, _ := Discover(local.dir)
		if got, err := repo.PushRemote(context.Background()); err != nil || got != "default" {
			t.Fatalf("PushRemote = %q, %v; want global default while detached", got, err)
		}
	})

	t.Run("set validates and configures current branch", func(t *testing.T) {
		local := newTestRepo(t)
		remote, _ := seededBareRemote(t, "set")
		local.git("remote", "add", "push", remote.dir)
		repo, _ := Discover(local.dir)
		if err := repo.SetPushRemote(context.Background(), "missing"); !errors.Is(err, ErrNoFetchRemote) {
			t.Fatalf("SetPushRemote invalid error = %v, want ErrNoFetchRemote", err)
		}
		if err := repo.SetPushRemote(context.Background(), "push"); err != nil {
			t.Fatalf("SetPushRemote: %v", err)
		}
		if got := local.git("config", "--get", "branch.main.pushRemote"); got != "push" {
			t.Fatalf("configured push remote = %q", got)
		}
	})
}

func TestFetchResolutionPreservesCancellation(t *testing.T) {
	local := newTestRepo(t)
	repo, _ := Discover(local.dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.FetchUpstream(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchUpstream error = %v, want context cancellation", err)
	}
}

func TestFetchUpstreamPreservesGitError(t *testing.T) {
	local := newTestRepo(t)
	local.git("remote", "add", "broken", t.TempDir())
	local.git("config", "branch.main.remote", "broken")
	repo, _ := Discover(local.dir)

	err := repo.FetchUpstream(context.Background())
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("FetchUpstream error = %v, want CommandError", err)
	}
}

func seededBareRemote(t *testing.T, label string) (*testRepo, string) {
	t.Helper()
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write(label+".txt", label+"\n")
	id := seed.commitAll(label)
	seed.git("push", remote.dir, "main")
	return remote, id
}

func assertOnlyRemoteFetched(t *testing.T, local *testRepo, remote, wantID string) {
	t.Helper()
	if got := local.git("rev-parse", "refs/remotes/"+remote+"/main"); got != wantID {
		t.Fatalf("fetched %s tip = %q, want %q", remote, got, wantID)
	}
	wantRef := "refs/remotes/" + remote + "/main"
	if got := local.git("for-each-ref", "--format=%(refname)", "refs/remotes"); got != wantRef {
		t.Fatalf("fetched remote refs = %q, want only %q", got, wantRef)
	}
}
