package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteCommitUIRefusesSigningAndDisablesInheritedSigning(t *testing.T) {
	tr := newTestRepo(t)
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	tr.git("config", "commit.gpgSign", "true")
	tr.git("config", "gpg.program", "/definitely/not/a/signer")
	tr.write("a", "a")
	tr.git("add", "a")
	if _, err := repo.ExecuteCommitUI(context.Background(), CommitUICreate, "", "unsigned", CommitOptions{}, false); err != nil {
		t.Fatalf("inherited commit.gpgSign was not suppressed: %v", err)
	}
	tr.write("b", "b")
	tr.git("add", "b")
	if _, err := repo.ExecuteCommitUI(context.Background(), CommitUICreate, "", "signed", CommitOptions{Sign: true}, false); !errors.Is(err, ErrCommitSigningConsentRequired) {
		t.Fatalf("signing without consent = %v", err)
	}
}

func TestExecuteCommitUIReeditsWithPrivateEditor(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("base", "base\n")
	tr.git("add", "base")
	tr.git("commit", "-m", "source message")
	source := tr.git("rev-parse", "HEAD")
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if message, err := repo.CommitMessageForUI(ctx, source); err != nil || message != "source message\n\n" {
		t.Fatalf("source message = %q, %v", message, err)
	}
	for _, test := range []struct {
		variant CommitUIVariant
		staged  bool
	}{
		{CommitUICreate, true},
		{CommitUIExtend, true},
		{CommitUIAmend, true},
		{CommitUIReword, false},
	} {
		if test.staged {
			tr.write(string(test.variant), string(test.variant)+"\n")
			tr.git("add", string(test.variant))
		}
		message := "reedit " + string(test.variant)
		if _, err := repo.ExecuteCommitUI(ctx, test.variant, "", message, CommitOptions{ReeditMessage: source}, false); err != nil {
			t.Fatalf("%s: %v", test.variant, err)
		}
		if got := tr.git("log", "-1", "--format=%B"); got != message {
			t.Fatalf("%s message = %q", test.variant, got)
		}
	}
	if _, err := repo.ExecuteCommitUI(ctx, CommitUIFixup, source, "", CommitOptions{ReeditMessage: source}, false); err == nil {
		t.Fatal("fixup accepted reedit-message")
	}
	if _, err := repo.ExecuteCommitUI(ctx, CommitUICreate, "", "conflict", CommitOptions{ReuseMessage: source, ReeditMessage: source}, false); err == nil {
		t.Fatal("create combined reuse-message and reedit-message")
	}
	if _, err := repo.commitArgs(ctx, "test", nil, CommitOptions{ReeditMessage: "missing"}); err == nil {
		t.Fatal("commit arguments accepted a missing reedit source")
	}
}

func TestExecuteCommitUIHooksNoVerifyAndMessageRedaction(t *testing.T) {
	tr := newTestRepo(t)
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(tr.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	tr.write("a", "a")
	tr.git("add", "a")
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	if _, err := repo.ExecuteCommitUI(ctx, CommitUICreate, "", "private message", CommitOptions{}, false); err == nil {
		t.Fatal("pre-commit hook was bypassed without no-verify")
	}
	if _, err := repo.ExecuteCommitUI(ctx, CommitUICreate, "", "private message", CommitOptions{NoVerify: true}, false); err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if strings.Contains(strings.Join(record.Args, " "), "private message") || strings.Contains(record.Stdout, "private message") || strings.Contains(record.Stderr, "private message") {
			t.Fatalf("commit message leaked in process record: %+v", record)
		}
	}
}

func TestPrepareCommitUIOptions(t *testing.T) {
	options := CommitOptions{Sign: true}
	requested, err := prepareCommitUIOptions(CommitUICreate, &options, true)
	if err != nil || !requested || !options.AllowInteractiveSigning {
		t.Fatalf("consented signing = %v, %#v, %v", requested, options, err)
	}

	options = CommitOptions{SigningKey: "key"}
	if _, err := prepareCommitUIOptions(CommitUICreate, &options, false); !errors.Is(err, ErrCommitSigningConsentRequired) {
		t.Fatalf("unconsented signing = %v", err)
	}

	options = CommitOptions{ReeditMessage: "HEAD"}
	if _, err := prepareCommitUIOptions(CommitUIFixup, &options, false); err == nil {
		t.Fatal("fixup reedit was accepted")
	}
}

func TestAllowsCommitUIReedit(t *testing.T) {
	for _, variant := range []CommitUIVariant{CommitUICreate, CommitUIExtend, CommitUIAmend, CommitUIReword} {
		if !allowsCommitUIReedit(variant) {
			t.Errorf("allowsCommitUIReedit(%q) = false", variant)
		}
	}
	for _, variant := range []CommitUIVariant{CommitUIFixup, CommitUISquash, CommitUIAlter, CommitUIAugment, CommitUIRevise, "unknown"} {
		if allowsCommitUIReedit(variant) {
			t.Errorf("allowsCommitUIReedit(%q) = true", variant)
		}
	}
}

func TestValidateStructuredFixupInvocationHelper(t *testing.T) {
	if err := validateStructuredFixupInvocation(CommitUIAlter, "message", CommitOptions{}); err != nil {
		t.Fatalf("valid alter invocation: %v", err)
	}
	if err := validateStructuredFixupInvocation(CommitUIRevise, "message", CommitOptions{All: true}); err == nil {
		t.Fatal("revise --all was accepted")
	}
	if err := validateStructuredFixupInvocation(CommitUIAlter, "", CommitOptions{}); err == nil {
		t.Fatal("empty structured message was accepted")
	}
	if err := validateStructuredFixupInvocation(CommitUIAlter, "message", CommitOptions{ReuseMessage: "HEAD"}); err == nil {
		t.Fatal("reuse-message was accepted")
	}
}

func TestReviewedCommitUIRejectsStaleIndexAndExecutesExactFixup(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	target := r.git("rev-parse", "HEAD")
	r.write("change", "change\n")
	r.git("add", "--", "change")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	request := CommitUIRequest{Variant: CommitUIFixup, Target: "HEAD"}
	review, err := repo.ReviewCommitUI(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if review.Request.Target != target || len(review.Plan) < 2 {
		t.Fatalf("canonical review = %#v", review)
	}
	r.write("later", "later\n")
	r.git("add", "--", "later")
	if _, err := repo.ExecuteReviewedCommitUI(ctx, review); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("stale reviewed fixup = %v", err)
	}
	r.git("reset", "--", "later")
	if err := os.Remove(filepath.Join(r.dir, "later")); err != nil {
		t.Fatal(err)
	}
	review, err = repo.ReviewCommitUI(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExecuteReviewedCommitUI(ctx, review); err != nil {
		t.Fatal(err)
	}
	if got := r.git("log", "-1", "--format=%s"); got != "fixup! base" {
		t.Fatalf("fixup subject = %q", got)
	}
}
