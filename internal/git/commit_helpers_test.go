package git

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCommitUIInvocationsAndSquashCommitValidation(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base", "base\n")
	base := r.commitAll("base")
	repo, _ := Discover(r.dir)

	for _, variant := range []CommitUIVariant{CommitUIFixup, CommitUISquash, CommitUIAugment, CommitUIAlter, CommitUIRevise} {
		message := ""
		if variant != CommitUIFixup && variant != CommitUISquash {
			message = "replacement"
		}
		invocation, err := repo.autosquashCommitUIInvocation(ctx, variant, "HEAD", message, CommitOptions{})
		if err != nil || len(invocation.args) == 0 {
			t.Errorf("autosquash invocation %s = %#v, %v", variant, invocation, err)
		}
	}
	if _, err := repo.autosquashCommitUIInvocation(ctx, "unknown", "HEAD", "", CommitOptions{}); err == nil {
		t.Fatal("unknown autosquash workflow accepted")
	}
	if _, err := repo.squashCommitUIInvocation(ctx, CommitUISquash, base, "", CommitOptions{ReuseMessage: "HEAD"}); err == nil {
		t.Fatal("squash reuse-message accepted")
	}
	if _, err := repo.squashCommitUIInvocation(ctx, CommitUIAugment, base, "", CommitOptions{}); err == nil {
		t.Fatal("empty augment message accepted")
	}
	if _, err := repo.structuredFixupCommitUIInvocation(ctx, CommitUIRevise, base, "replacement", CommitOptions{All: true}); err == nil {
		t.Fatal("revised all invocation accepted")
	}

	r.write("squash", "content\n")
	r.git("add", "squash")
	commit, err := repo.SquashCommit(ctx, "HEAD", "extra", CommitOptions{})
	if err != nil || commit.ID == "" {
		t.Fatalf("SquashCommit() = %#v, %v", commit, err)
	}
	if _, err := repo.SquashCommit(ctx, "HEAD", "", CommitOptions{ReuseMessage: "HEAD"}); err == nil {
		t.Fatal("SquashCommit accepted reuse-message")
	}
}

func TestCommitAndInteractiveRebaseArgumentValidation(t *testing.T) {
	if err := validateCommitArgsOptions("test", CommitOptions{ReuseMessage: "a", ReeditMessage: "b"}); err == nil {
		t.Fatal("combined messages accepted")
	}
	if err := validateCommitArgsOptions("test", CommitOptions{Sign: true}); !errors.Is(err, ErrInteractiveRequired) {
		t.Fatalf("signing error = %v", err)
	}
	opts := CommitOptions{All: true, AllowEmpty: true, NoVerify: true, ResetAuthor: true, Signoff: true, Author: "A", Date: "now", Sign: true, AllowInteractiveSigning: true}
	args := appendCommitBooleanArgs([]string{"commit"}, opts)
	args = appendCommitValueArgs(args, opts)
	args = appendCommitSigningArg(args, opts)
	for _, want := range []string{"--all", "--allow-empty", "--no-verify", "--reset-author", "--signoff", "--author=A", "--date=now", "--gpg-sign"} {
		if !containsString(args, want) {
			t.Fatalf("%q missing from %#v", want, args)
		}
	}
	if got := appendCommitSigningArg(nil, CommitOptions{SigningKey: "key"}); !reflect.DeepEqual(got, []string{"--gpg-sign=key"}) {
		t.Fatalf("key signing = %#v", got)
	}

	if err := validateInteractiveRebaseOptions(RebaseOptions{}); err == nil {
		t.Fatal("empty upstream accepted")
	}
	if err := validateInteractiveRebaseOptions(RebaseOptions{Upstream: "main", RebaseMerges: true}); err == nil {
		t.Fatal("merge topology accepted")
	}
	rargs := interactiveRebaseArgs(RebaseOptions{KeepEmpty: true, UpdateRefs: true, Autostash: true, ForceRebase: true, Strategy: "ort", Signoff: true})
	for _, want := range []string{"--keep-empty", "--update-refs", "--autostash", "--force-rebase", "--strategy=ort", "--signoff"} {
		if !containsString(rargs, want) {
			t.Fatalf("%q missing from %#v", want, rargs)
		}
	}
}
