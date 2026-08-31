package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReviewedGitCommandBlocksConfigAliasesAndGlobalOptions(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.run(context.Background(), "config", "alias.ui-owned", "!echo must-not-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReviewGitCommand(context.Background(), []string{"ui-owned"}, true); !errors.Is(err, ErrRawCommandAlias) {
		t.Fatalf("alias review error = %v", err)
	}
	for _, argv := range [][]string{{"-c", "alias.x=!false", "x"}, {"--git-dir=/tmp/x", "status"}, {"git", "status"}} {
		if _, err := repo.ReviewGitCommand(context.Background(), argv, false); err == nil {
			t.Fatalf("global argv was accepted: %#v", argv)
		}
	}
	if _, err := repo.ReviewGitCommand(context.Background(), []string{"bisect", "run", "true"}, false); err == nil || !strings.Contains(err.Error(), "history-owned") {
		t.Fatalf("bisect run escaped its history owner: %v", err)
	}
}

func TestReviewedGitCommandRequiresSeparateExternalConfirmation(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReviewGitCommand(context.Background(), []string{"definitely-not-a-builtin"}, false); !errors.Is(err, ErrExternalGitCommand) {
		t.Fatalf("external review error = %v", err)
	}
	plan, err := repo.ReviewGitCommand(context.Background(), []string{"definitely-not-a-builtin"}, true)
	if err != nil || !plan.ExternalGit() {
		t.Fatalf("separately confirmed external plan = %#v, %v", plan, err)
	}
}

func TestValidateReviewedGitSubcommandDirect(t *testing.T) {
	for _, argv := range [][]string{{"-c", "x=y", "status"}, {"git", "status"}, {"credential"}, {"bisect", "run", "true"}} {
		if err := validateReviewedGitSubcommand(argv); err == nil {
			t.Fatalf("unsupported subcommand was accepted: %#v", argv)
		}
	}
	if err := validateReviewedGitSubcommand([]string{"status", "--short"}); err != nil {
		t.Fatalf("status was rejected: %v", err)
	}
}

func TestRejectReviewedGitAliasDirect(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.rejectReviewedGitAlias(ctx, "status"); err != nil {
		t.Fatalf("missing alias rejected: %v", err)
	}
	if err := repo.run(ctx, "config", "alias.direct-test", "status --short"); err != nil {
		t.Fatal(err)
	}
	if err := repo.rejectReviewedGitAlias(ctx, "direct-test"); !errors.Is(err, ErrRawCommandAlias) {
		t.Fatalf("configured alias error = %v", err)
	}
}

func TestDirectRunRecordsLiteralMetacharactersAndRedacts(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	secret := "token=raw-command-secret-9284"
	literal := ";$(printf should-not-expand)"
	plan, err := ReviewRunCommand([]string{"/usr/bin/printf", "%s %s", literal, secret})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ExecuteReviewedCommand(context.Background(), AllowUnsafeExecution{}, plan); !errors.Is(err, ErrUnsafeExecution) {
		t.Fatalf("zero capability error = %v", err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	if err := repo.ExecuteReviewedCommand(ctx, NewAllowUnsafeExecution(), plan); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d", len(records))
	}
	serialized := strings.Join(records[0].Args, "\x00") + records[0].Stdout + records[0].Stderr
	if !strings.Contains(records[0].Stdout, literal) {
		t.Fatalf("metacharacters were not literal: %q", records[0].Stdout)
	}
	if strings.Contains(serialized, "raw-command-secret") || !strings.Contains(serialized, redactionMarker) {
		t.Fatalf("credential was not redacted: %q", serialized)
	}
	if shown := strings.Join(plan.Args(), " "); strings.Contains(shown, "raw-command-secret") {
		t.Fatalf("review args leaked credential: %q", shown)
	}
}

func TestDirectRunRejectsShellAndGitPolicyBypasses(t *testing.T) {
	for _, argv := range [][]string{{"/bin/sh", "-c", "true"}, {"env", "bash"}, {"git", "-c", "alias.x=!false", "x"}, {"git-custom-helper"}} {
		if _, err := ReviewRunCommand(argv); err == nil {
			t.Fatalf("policy bypass was accepted: %#v", argv)
		}
	}
}
