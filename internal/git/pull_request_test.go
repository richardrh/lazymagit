package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeGH(t *testing.T, body string) (logPath, inputPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "args.log")
	inputPath = filepath.Join(dir, "body.txt")
	script := filepath.Join(dir, "gh")
	contents := "#!/bin/sh\n"
	contents += "printf '%s\\n' \"$*\" >> \"$GH_TEST_LOG\"\n"
	contents += "if [ \"$1\" = repo ]; then printf '%s' '{\"nameWithOwner\":\"owner/repo\",\"defaultBranchRef\":{\"name\":\"main\"}}'; exit 0; fi\n"
	contents += body
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_TEST_LOG", logPath)
	t.Setenv("GH_TEST_BODY", inputPath)
	return logPath, inputPath
}

func TestPullRequestForUINewDefaultsAndTemplate(t *testing.T) {
	logPath, _ := fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = list ]; then printf '%s' '[]'; exit 0; fi\n")
	r := newTestRepo(t)
	r.write("tracked.txt", "tracked\n")
	r.write(".github/pull_request_template.md", "template body\n")
	r.commitAll("subject line\n\ncommit body")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.PullRequestForUI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 0 || got.Title != "subject line" || got.Body != "template body" || got.Base != "main" || got.Head != "main" {
		t.Fatalf("new pull request defaults = %#v", got)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "commit body") {
		t.Fatal("read-only gh invocation leaked commit body into arguments")
	}
}

func TestSubmitPullRequestCreateUsesSafeArgsAndStdin(t *testing.T) {
	_, inputPath := fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = create ]; then cat > \"$GH_TEST_BODY\"; printf '%s' 'https://github.com/owner/repo/pull/7'; exit 0; fi\nif [ \"$1\" = pr ] && [ \"$2\" = view ]; then printf '%s' '{\"number\":7,\"url\":\"https://github.com/owner/repo/pull/7\",\"title\":\"safe title\",\"body\":\"private body\",\"baseRefName\":\"main\",\"headRefName\":\"main\",\"isDraft\":true}'; exit 0; fi\n")
	r := newTestRepo(t)
	r.write("tracked.txt", "tracked\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var records []ProcessRecord
	ctx := WithProcessRecorder(context.Background(), func(record ProcessRecord) { records = append(records, record) })
	title := "title with spaces; $(touch should-not-exist)"
	body := "private body\nwith newlines"
	got, err := repo.SubmitPullRequest(ctx, PullRequest{Title: title, Body: body, Base: "main", Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 7 || got.URL != "https://github.com/owner/repo/pull/7" || got.Head != "main" || !got.Draft {
		t.Fatalf("created pull request = %#v", got)
	}
	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != body {
		t.Fatalf("body stdin = %q, want %q", input, body)
	}
	if len(records) != 1 {
		t.Fatalf("mutation records = %#v, want one create record", records)
	}
	serialized := strings.Join(records[0].Args, "\x00") + records[0].Stdout + records[0].Stderr
	if strings.Contains(serialized, title) || strings.Contains(serialized, body) {
		t.Fatalf("process record leaked PR message: %#v", records[0])
	}
	if !strings.Contains(serialized, "[REDACTED]") {
		t.Fatalf("title was not redacted in process record: %#v", records[0])
	}
	if _, err := os.Stat(filepath.Join(r.dir, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("title caused shell execution: %v", err)
	}
}

func TestSubmitPullRequestRejectsBranchMismatchBeforeMutation(t *testing.T) {
	logPath, _ := fakeGH(t, "if [ \"$1\" = pr ]; then printf '%s' 'unexpected mutation' >&2; exit 1; fi\n")
	r := newTestRepo(t)
	r.write("tracked.txt", "tracked\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.SubmitPullRequest(context.Background(), PullRequest{Title: "title", Base: "main", Head: "other"})
	if err == nil || !strings.Contains(err.Error(), "current branch") {
		t.Fatalf("branch mismatch error = %v", err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(log), "pr") {
		t.Fatalf("branch mismatch launched PR mutation: %s", log)
	}
}

func TestCreatedPullRequestKeepsIdentityWhenRefreshFails(t *testing.T) {
	fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = create ]; then printf '%s' 'https://github.com/owner/repo/pull/42'; exit 0; fi\nif [ \"$1\" = pr ] && [ \"$2\" = view ]; then printf 'network unavailable' >&2; exit 1; fi\n")
	r := newTestRepo(t)
	r.write("tracked", "text\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repo.SubmitPullRequest(context.Background(), PullRequest{Title: "Preserve identity", Head: "main", Base: "release"})
	if err == nil || result.Number != 42 || result.URL != "https://github.com/owner/repo/pull/42" {
		t.Fatalf("partial success lost the published PR identity: %#v, %v", result, err)
	}
}

func TestPullRequestForUIRejectsAmbiguousOpenPullRequests(t *testing.T) {
	logPath, _ := fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = list ]; then printf '%s' '[{\"number\":1,\"url\":\"u1\",\"headRefName\":\"main\"},{\"number\":2,\"url\":\"u2\",\"headRefName\":\"main\"}]'; exit 0; fi\n")
	r := newTestRepo(t)
	r.write("tracked", "text\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.PullRequestForUI(context.Background())
	if err == nil || !strings.Contains(err.Error(), "multiple open PRs") {
		t.Fatalf("ambiguous pull request error = %v", err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(log), "pr edit") {
		t.Fatalf("ambiguous lookup attempted mutation: %s", log)
	}
}

func TestPullRequestForUIRejectsReturnedHeadMismatch(t *testing.T) {
	fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = list ]; then printf '%s' '[{\"number\":1,\"url\":\"u1\",\"headRefName\":\"other\"}]'; exit 0; fi\n")
	r := newTestRepo(t)
	r.write("tracked", "text\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.PullRequestForUI(context.Background())
	if err == nil || !strings.Contains(err.Error(), "current branch") {
		t.Fatalf("head mismatch error = %v", err)
	}
}

func TestSubmitPullRequestEditUsesReviewedIdentity(t *testing.T) {
	logPath, _ := fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = view ]; then printf '%s' '{\"number\":7,\"url\":\"https://github.com/owner/repo/pull/7\",\"title\":\"old\",\"body\":\"old\",\"baseRefName\":\"main\",\"headRefName\":\"main\",\"isDraft\":false}'; exit 0; fi\nif [ \"$1\" = pr ] && [ \"$2\" = edit ]; then cat >/dev/null; exit 0; fi\n")
	r := newTestRepo(t)
	r.write("tracked", "text\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.SubmitPullRequest(context.Background(), PullRequest{
		Number: 7, Title: "new title", Body: "new body", Base: "main", Head: "main",
	})
	if err != nil || got.Number != 7 || got.URL != "https://github.com/owner/repo/pull/7" {
		t.Fatalf("edited pull request = %#v, %v", got, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	viewIndex, editIndex := -1, -1
	for index, line := range lines {
		if strings.HasPrefix(line, "pr view 7 ") && viewIndex == -1 {
			viewIndex = index
		}
		if strings.HasPrefix(line, "pr edit 7 ") && editIndex == -1 {
			editIndex = index
		}
	}
	if viewIndex == -1 || editIndex == -1 || viewIndex >= editIndex {
		t.Fatalf("reviewed view did not precede edit: %q", lines)
	}
	if !strings.Contains(lines[editIndex], "pr edit 7 --repo owner/repo --title new title --body-file - --base main") {
		t.Fatalf("edit invocation missing reviewed values: %s", lines[editIndex])
	}
}

func TestSubmitPullRequestEditRejectsStaleHeadBeforeMutation(t *testing.T) {
	logPath, _ := fakeGH(t, "if [ \"$1\" = pr ] && [ \"$2\" = view ]; then printf '%s' '{\"number\":7,\"url\":\"https://github.com/owner/repo/pull/7\",\"title\":\"old\",\"body\":\"old\",\"baseRefName\":\"main\",\"headRefName\":\"other\",\"isDraft\":false}'; exit 0; fi\nif [ \"$1\" = pr ] && [ \"$2\" = edit ]; then printf '%s' > \"$GH_TEST_BODY\"; exit 0; fi\n")
	r := newTestRepo(t)
	r.write("tracked", "text\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.SubmitPullRequest(context.Background(), PullRequest{Number: 7, Title: "title", Body: "body", Base: "main", Head: "main"})
	if err == nil || !strings.Contains(err.Error(), "belongs to") {
		t.Fatalf("stale pull request error = %v", err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "pr edit 7") {
		t.Fatalf("stale pull request was mutated: %s", log)
	}
}
