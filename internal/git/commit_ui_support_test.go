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
