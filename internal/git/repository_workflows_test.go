package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryCompareRemoteTagsDirect(t *testing.T) {
	ctx := context.Background()
	remote := newBareTestRepo(t)
	local := newTestRepo(t)
	local.write("file", "base\n")
	head := local.commitAll("base")
	local.git("tag", "common")
	local.git("tag", "local-only")
	local.git("remote", "add", "origin", remote.dir)
	local.git("push", "origin", "main", "refs/tags/common")
	remote.git("update-ref", "refs/tags/remote-only", head)
	repo, err := Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	comparison, err := repo.CompareRemoteTags(ctx, "origin")
	if err != nil {
		t.Fatalf("CompareRemoteTags: %v", err)
	}
	want := RemoteTagComparison{
		Remote:               "origin",
		LocalOnly:            []string{"local-only"},
		RemoteOnly:           []string{"remote-only"},
		Common:               []string{"common"},
		RequiresConfirmation: true,
	}
	if !reflect.DeepEqual(comparison, want) {
		t.Fatalf("comparison = %#v, want %#v", comparison, want)
	}
	if _, err := repo.CompareRemoteTags(ctx, "--invalid"); err == nil {
		t.Fatal("option-like remote was accepted")
	}

	if got := tagNames([]byte("b\n\na\n")); !reflect.DeepEqual(got, map[string]bool{"a": true, "b": true}) {
		t.Fatalf("tagNames = %#v", got)
	}
	remoteOutput := []byte(strings.Repeat("a", 40) + "\trefs/tags/v2\nmalformed\n" + strings.Repeat("b", 40) + "\trefs/tags/v1\n")
	if got := remoteTagNames(remoteOutput); !reflect.DeepEqual(got, map[string]bool{"v1": true, "v2": true}) {
		t.Fatalf("remoteTagNames = %#v", got)
	}
}

func TestRepositoryResolveRevisionRangeDirect(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("file", "base\n")
	base := r.commitAll("base")
	r.write("file", "head\n")
	head := r.commitAll("head")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "revision", value: " HEAD ", want: head},
		{name: "two dot", value: "HEAD^..HEAD", want: base + ".." + head},
		{name: "three dot", value: "HEAD^...HEAD", want: base + "..." + head},
		{name: "default left", value: "..HEAD", want: head + ".." + head},
		{name: "default right", value: "HEAD^...", want: base + "..." + head},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := repo.resolveRevisionRange(ctx, test.value)
			if err != nil || got != test.want {
				t.Fatalf("resolveRevisionRange(%q) = %q, %v; want %q", test.value, got, err, test.want)
			}
		})
	}

	for _, value := range []string{"", "--all", "HEAD\nHEAD", "HEAD\x00HEAD", "HEAD..HEAD..HEAD", "HEAD..HEAD...HEAD", "missing..HEAD", "HEAD..missing"} {
		t.Run("reject "+strings.ReplaceAll(value, "\n", "newline"), func(t *testing.T) {
			if _, err := repo.resolveRevisionRange(ctx, value); err == nil {
				t.Fatalf("resolveRevisionRange(%q) succeeded", value)
			}
		})
	}
}

func TestRepositoryResetDirectCommands(t *testing.T) {
	for _, test := range []struct {
		name string
		mode ResetMode
		path string
		want func(string) []string
	}{
		{name: "mixed", mode: ResetMixed, want: func(target string) []string { return []string{"reset", "--mixed", target} }},
		{name: "soft", mode: ResetSoft, want: func(target string) []string { return []string{"reset", "--soft", target} }},
		{name: "hard", mode: ResetHard, want: func(target string) []string { return []string{"reset", "--hard", target} }},
		{name: "keep", mode: ResetKeep, want: func(target string) []string { return []string{"reset", "--keep", target} }},
		{name: "index", mode: ResetIndex, want: func(target string) []string { return []string{"reset", target, "--", "."} }},
		{name: "worktree", mode: ResetWorktree, want: func(target string) []string {
			return []string{"restore", "--worktree", "--source=" + target, "--", "."}
		}},
		{name: "file", mode: ResetFile, path: "file", want: func(target string) []string {
			return []string{"restore", "--staged", "--worktree", "--source=" + target, "--", "file"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := newTestRepo(t)
			r.write("file", "base\n")
			base := r.commitAll("base")
			r.write("file", "head\n")
			r.commitAll("head")
			repo, err := Discover(r.dir)
			if err != nil {
				t.Fatal(err)
			}
			var records []ProcessRecord
			ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
			opts := ResetOptions{Mode: test.mode, Target: base, ConfirmOptions: ConfirmOptions{Confirmed: true}}
			if test.path != "" {
				opts.Paths = []string{test.path}
			}
			if err := repo.Reset(ctx, opts); err != nil {
				t.Fatalf("Reset: %v", err)
			}
			want := test.want(base)
			if len(records) != 1 || !reflect.DeepEqual(records[0].Args, want) {
				t.Fatalf("records = %#v, want args %#v", records, want)
			}
		})
	}

	if _, err := resetArgs(ResetMode(255), DestructivePreflight{}); err == nil {
		t.Fatal("invalid reset mode produced arguments")
	}

	r := newTestRepo(t)
	r.write("file", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	err = repo.Reset(context.Background(), ResetOptions{Mode: ResetHard})
	var required *HistoryConfirmationRequiredError
	if !errors.As(err, &required) {
		t.Fatalf("unconfirmed Reset error = %T %v", err, err)
	}
}
