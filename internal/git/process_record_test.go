package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProcessRecorderCapturesRunResults(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) {
		records = append(records, record)
	})
	before := time.Now()
	successArgs := []string{"hash-object", "--stdin"}
	if err := repo.runInput(ctx, []byte("recorded contents\n"), successArgs...); err != nil {
		t.Fatalf("successful runInput: %v", err)
	}
	successArgs[0] = "changed-after-callback"

	if len(records) != 1 {
		t.Fatalf("records after success = %d, want 1", len(records))
	}
	success := records[0]
	if success.Dir != r.dir {
		t.Errorf("success Dir = %q, want %q", success.Dir, r.dir)
	}
	if got := strings.Join(success.Args, " "); got != "hash-object --stdin" {
		t.Errorf("success Args = %q", got)
	}
	if success.Started.Before(before) || success.Started.After(time.Now()) {
		t.Errorf("success Started = %v, outside command interval", success.Started)
	}
	if success.Duration < 0 {
		t.Errorf("success Duration = %v, want non-negative", success.Duration)
	}
	if success.ExitCode != 0 {
		t.Errorf("success ExitCode = %d, want 0", success.ExitCode)
	}
	if strings.TrimSpace(success.Stdout) == "" {
		t.Error("success Stdout is empty")
	}
	if success.Stderr != "" {
		t.Errorf("success Stderr = %q, want empty", success.Stderr)
	}

	err = repo.run(ctx, "rev-parse", "--verify", "refs/heads/definitely-missing")
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("failed run error = %v, want CommandError", err)
	}
	if len(records) != 2 {
		t.Fatalf("records after failure = %d, want 2", len(records))
	}
	failed := records[1]
	if got := strings.Join(failed.Args, " "); got != "rev-parse --verify refs/heads/definitely-missing" {
		t.Errorf("failed Args = %q", got)
	}
	if failed.Duration < 0 {
		t.Errorf("failed Duration = %v, want non-negative", failed.Duration)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("failed run error = %v, want exec.ExitError", err)
	}
	if failed.ExitCode != exit.ExitCode() || failed.ExitCode <= 0 {
		t.Errorf("failed ExitCode = %d, want actual code %d", failed.ExitCode, exit.ExitCode())
	}
	if failed.Stdout != "" {
		t.Errorf("failed Stdout = %q, want captured empty output", failed.Stdout)
	}
	if failed.Stderr == "" || commandErr.Stderr != strings.TrimSpace(failed.Stderr) {
		t.Errorf("failed Stderr = %q, CommandError Stderr = %q", failed.Stderr, commandErr.Stderr)
	}

	r.write("tracked.txt", "base\n")
	r.commitAll("base")
	r.write("tracked.txt", "changed\n")
	if err := repo.run(ctx, "diff", "--exit-code", "--", "tracked.txt"); err == nil {
		t.Fatal("diff --exit-code with a change succeeded")
	}
	if len(records) != 3 {
		t.Fatalf("records after stdout-producing failure = %d, want 3", len(records))
	}
	diffFailure := records[2]
	if diffFailure.ExitCode != 1 || diffFailure.Stdout == "" || diffFailure.Stderr != "" {
		t.Errorf("diff failure = %#v, want exit 1 with stdout and empty stderr", diffFailure)
	}
}

func TestProcessRecorderExcludesQueriesAndRecordsCancellation(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) {
		records = append(records, record)
	})

	if _, err := repo.Status(ctx); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Status produced %d process records, want none", len(records))
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	err = repo.run(canceled, "status", "--short")
	if err == nil {
		t.Fatal("run with canceled context succeeded")
	}
	if len(records) != 1 || records[0].ExitCode != -1 {
		t.Fatalf("canceled records = %#v, want one with exit code -1", records)
	}

	if got := WithProcessRecorder(ctx, nil); got != ctx {
		t.Fatal("nil recorder changed the context")
	}
}

func TestMutationCaptureIsBoundedAndFullyDrained(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) {
		records = append(records, record)
	})
	alias := "alias.capture=!dd if=/dev/zero bs=1048576 count=1 2>/dev/null; dd if=/dev/zero bs=1048576 count=1 >&2 2>/dev/null; exit 7"
	err = repo.run(ctx, "-c", alias, "capture")
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("run error = %v, want CommandError", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if !record.StdoutTruncated || !record.StderrTruncated || !commandErr.StderrTruncated {
		t.Fatalf("truncation state: stdout=%v/%d stderr=%v/%d command stderr=%v/%d", record.StdoutTruncated, len(record.Stdout), record.StderrTruncated, len(record.Stderr), commandErr.StderrTruncated, len(commandErr.Stderr))
	}
	for name, output := range map[string]string{"stdout": record.Stdout, "stderr": record.Stderr, "command stderr": commandErr.Stderr} {
		if len(output) > mutationCaptureLimit {
			t.Errorf("%s retained %d bytes, cap %d", name, len(output), mutationCaptureLimit)
		}
		if !strings.Contains(output, strings.TrimSpace(mutationTruncationMarker)) {
			t.Errorf("%s has no truncation marker", name)
		}
	}
}

func TestProcessRecorderRedactsMutationSecrets(t *testing.T) {
	r := newTestRepo(t)
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) {
		records = append(records, record)
	})

	commitSecret := "commit-message-token-8462"
	r.write("secret.txt", "content\n")
	if err := repo.Stage(ctx, []string{"secret.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(ctx, commitSecret); err != nil {
		t.Fatal(err)
	}
	userinfoURL := "https://token-user:token-password@example.invalid/repo.git"
	queryURL := "https://example.invalid/repo.git?access_token=query-token-7251"
	if err := repo.AddRemote(ctx, "userinfo", userinfoURL, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRemote(ctx, "query", queryURL, false); err != nil {
		t.Fatal(err)
	}
	setURL := "https://set-token@example.invalid/new.git?key=set-query-token"
	if err := repo.run(ctx, "remote", "set-url", "query", setURL); err != nil {
		t.Fatal(err)
	}
	failureURL := "https://failure-user:failure-password@example.invalid/repo.git?token=failure-query"
	err = repo.AddRemote(ctx, "userinfo", failureURL, false)
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("duplicate AddRemote error = %v, want CommandError", err)
	}
	if strings.Contains(commandErr.Error(), "failure-") || !strings.Contains(strings.Join(commandErr.Args, " "), redactionMarker) {
		t.Fatalf("CommandError did not redact URL: %#v", commandErr)
	}

	serialized := strings.Builder{}
	for _, record := range records {
		serialized.WriteString(strings.Join(record.Args, "\x00"))
		serialized.WriteString(record.Stdout)
		serialized.WriteString(record.Stderr)
	}
	got := serialized.String()
	for _, secret := range []string{commitSecret, "token-user", "token-password", "query-token-7251", "set-token", "set-query-token", "failure-user", "failure-password", "failure-query"} {
		if strings.Contains(got, secret) {
			t.Errorf("process records contain secret %q", secret)
		}
	}
	if !strings.Contains(got, "commit\x00-m\x00"+redactionMarker) || !strings.Contains(got, "remote\x00add") {
		t.Errorf("redacted records lost useful command identity: %q", got)
	}

	echoed := "before " + userinfoURL + " middle " + queryURL + " after"
	redacted, truncated := redactCaptured(echoed, []string{userinfoURL, queryURL})
	if truncated || strings.Contains(redacted, "token-") || strings.Contains(redacted, "query-token") {
		t.Fatalf("redacted echoed output = %q, truncated=%v", redacted, truncated)
	}

	// Network commands usually receive only a remote name, but Git can echo the
	// stored URL in their diagnostics. Redaction must not depend on the URL being
	// present in the current command's arguments.
	storedDiagnostic := "fatal: unable to access 'https://stored-user:stored-password@example.invalid/repo.git?token=stored-query': denied"
	redacted, truncated = redactCaptured(storedDiagnostic, nil)
	if truncated || strings.Contains(redacted, "stored-") || !strings.Contains(redacted, redactionMarker) {
		t.Fatalf("stored remote diagnostic was not redacted: %q, truncated=%v", redacted, truncated)
	}
	publicDiagnostic := "fatal: unable to access 'https://example.invalid/public.git': denied"
	redacted, _ = redactCaptured(publicDiagnostic, nil)
	if redacted != publicDiagnostic {
		t.Fatalf("public URL was unnecessarily redacted: %q", redacted)
	}
}
