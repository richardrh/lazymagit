package git

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFetchUIWrappersPreserveOptionsAndRecordExactArgs(t *testing.T) {
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("base", "base\n")
	local.commitAll("base")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "-u", "origin", "main")
	local.git("config", "remote.pushDefault", "origin")
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	options := FetchArgs{Prune: true, Tags: FetchAllTags, Force: true}
	tests := []struct {
		name string
		run  func() error
		want []string
	}{
		{"push", func() error { return repo.FetchPushWithArgs(ctx, options) }, []string{"fetch", "--prune", "--tags", "--force", "--", "origin"}},
		{"upstream", func() error { return repo.FetchUpstreamWithArgs(ctx, options) }, []string{"fetch", "--prune", "--tags", "--force", "--", "origin"}},
		{"all", func() error { return repo.FetchAllWithArgs(ctx, options) }, []string{"fetch", "--prune", "--tags", "--force", "--all"}},
		{"modules", func() error { return repo.FetchModulesWithArgs(ctx, options) }, []string{"fetch", "--prune", "--tags", "--force", "--recurse-submodules=yes"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := len(records)
			if err := test.run(); err != nil {
				t.Fatal(err)
			}
			if len(records) != before+1 || !reflect.DeepEqual(records[before].Args, test.want) {
				t.Fatalf("records = %#v; want args %#v", records[before:], test.want)
			}
		})
	}
}

func TestFetchUIUnshallowOptionAgainstShallowBareRemote(t *testing.T) {
	remote := newBareTestRepo(t)
	seed := newTestRepo(t)
	seed.write("base", "base\n")
	seed.commitAll("base")
	seed.git("remote", "add", "origin", remote.dir)
	seed.git("push", "origin", "main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	parent := t.TempDir()
	shallowDir := filepath.Join(parent, "shallow")
	runGit(t, parent, "clone", "--depth=1", "file://"+remote.dir, shallowDir)
	repo, err := Discover(shallowDir)
	if err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	if err := repo.FetchUpstreamWithArgs(ctx, FetchArgs{Unshallow: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch", "--unshallow", "--", "origin"}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
		t.Fatalf("process records = %#v; want %#v", records, want)
	}
}

func TestFetchUISupportValidatesBeforeMutation(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, _ := Discover(r.dir)
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	if err := repo.FetchAllWithArgs(ctx, FetchArgs{Refspec: "refs/heads/main:refs/remotes/origin/main"}); err == nil {
		t.Fatal("fetch all accepted a refspec")
	}
	if err := repo.FetchModulesWithArgs(ctx, FetchArgs{Branch: "--bad"}); err == nil {
		t.Fatal("module fetch accepted a branch selector")
	}
	if len(records) != 0 {
		t.Fatalf("validation launched mutations: %#v", records)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repo.FetchUpstreamWithArgs(cancelled, FetchArgs{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}
