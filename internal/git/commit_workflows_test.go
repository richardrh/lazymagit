package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitWorkflowsCreateExtendAndReword(t *testing.T) {
	tr := newTestRepo(t)
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tr.write("a", "one\n")
	tr.git("add", "a")
	if _, err := repo.CreateCommit(ctx, "initial secret", CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	tr.write("a", "two\n")
	tr.git("add", "a")
	if _, err := repo.ExtendCommit(ctx, CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := tr.git("log", "-1", "--format=%s"); got != "initial secret" {
		t.Fatalf("extend subject = %q", got)
	}

	tr.write("staged", "kept\n")
	tr.git("add", "staged")
	if _, err := repo.RewordCommit(ctx, "renamed", CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := tr.git("log", "-1", "--format=%s"); got != "renamed" {
		t.Fatalf("reword subject = %q", got)
	}
	if got := tr.git("diff", "--cached", "--name-only"); got != "staged" {
		t.Fatalf("staged tree after reword = %q", got)
	}
}

func TestCommitOptionsHooksNoVerifyEmptyAndRecorderRedaction(t *testing.T) {
	tr := newTestRepo(t)
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(tr.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	tr.write("a", "a")
	tr.git("add", "a")
	if _, err := repo.CreateCommit(ctx, "do not record me", CommitOptions{}); err == nil {
		t.Fatal("hook should reject commit")
	}
	if _, err := repo.CreateCommit(ctx, "do not record me", CommitOptions{NoVerify: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateCommit(ctx, "empty", CommitOptions{AllowEmpty: true, NoVerify: true}); err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if strings.Contains(strings.Join(record.Args, " "), "do not record me") {
			t.Fatalf("message leaked in %#v", record.Args)
		}
	}
}

func TestCommitWorkflowAutosquashVariantsAndErrors(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("base", "base")
	base := tr.commitAll("base")
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tr.write("fix", "x")
	tr.git("add", "fix")
	if _, err := repo.FixupCommit(ctx, base, CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := tr.git("log", "-1", "--format=%s"); got != "fixup! base" {
		t.Fatalf("fixup subject = %q", got)
	}

	tr.write("staged", "still staged")
	tr.git("add", "staged")
	if _, err := repo.ReviseCommit(ctx, base, "replacement", CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := tr.git("diff", "--cached", "--name-only"); got != "staged" {
		t.Fatalf("revise consumed index: %q", got)
	}
	if got := tr.git("log", "-1", "--format=%B"); !strings.Contains(got, "replacement") || !strings.HasPrefix(got, "amend! base") {
		t.Fatalf("revise message = %q", got)
	}

	if _, err := repo.FixupCommit(ctx, "--help", CommitOptions{}); err == nil {
		t.Fatal("unsafe revision accepted")
	} else {
		var invalid *InvalidRevisionError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %T, want InvalidRevisionError", err)
		}
	}
	if _, err := repo.AugmentCommit(ctx, base, "", CommitOptions{}); err == nil {
		t.Fatal("missing editor error")
	} else {
		var editor *EditorRequiredError
		if !errors.As(err, &editor) {
			t.Fatalf("error = %T, want EditorRequiredError", err)
		}
	}
}

func TestCommitWorkflowUnbornErrors(t *testing.T) {
	tr := newTestRepo(t)
	repo, err := Discover(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExtendCommit(context.Background(), CommitOptions{}); err == nil {
		t.Fatal("extend on unborn repository succeeded")
	}
}
