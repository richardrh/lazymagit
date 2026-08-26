package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestOperationCollectorFailureOpensProcessPaneAndRendersStderr(t *testing.T) {
	repo, err := gitbackend.Init(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	m := New(repo)
	m.width, m.height, m.loading = 100, 24, false
	cmd := m.startOperation("push", func(ctx context.Context) error { return repo.Push(ctx) })
	msg := cmd().(operationMsg)
	if msg.opErr == nil || len(msg.records) != 1 {
		t.Fatalf("failed push result err=%v records=%d", msg.opErr, len(msg.records))
	}
	if got := msg.records[0]; len(got.Args) == 0 || got.Args[0] != "push" || got.ExitCode != 128 || got.Stderr == "" {
		t.Fatalf("push process record = %+v", got)
	}
	_, _ = m.Update(msg)
	plain := ansi.Strip(m.render())
	if m.mode != modeProcess || !strings.Contains(m.processTranscript(), strings.TrimSpace(msg.records[0].Stderr)) || !strings.Contains(plain, "Git processes") || !strings.Contains(plain, "git push") {
		t.Fatalf("failure pane/transcript missing: mode=%d render=%q", m.mode, plain)
	}
}

func TestProcessPanelLayoutScrollCloseAndEmptyCopy(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {120, 30}} {
		m := New(&gitbackend.Repository{})
		m.width, m.height, m.mode = size[0], size[1], modeProcess
		panel := m.renderProcessPanel(m.width, m.processPanelHeight())
		if w, h := lipgloss.Width(panel), lipgloss.Height(panel); w != size[0] || h != m.processPanelHeight() {
			t.Fatalf("process panel %dx%d rendered %dx%d", size[0], m.processPanelHeight(), w, h)
		}
		if full := m.render(); lipgloss.Width(full) != size[0] || lipgloss.Height(full) != size[1] {
			t.Fatalf("process layout dimensions changed")
		}
		if !strings.Contains(ansi.Strip(panel), "No Git process output recorded.") {
			t.Fatalf("empty process state unclear: %q", panel)
		}
		_, cmd := m.Update(keyMsg("y"))
		if cmd != nil {
			t.Fatal("empty transcript emitted clipboard command")
		}
	}

	m := New(&gitbackend.Repository{})
	m.width, m.height, m.mode = 80, 16, modeProcess
	m.processBatches = []processBatch{{text: strings.Join([]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, "\n")}}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.processOffset == 0 {
		t.Fatal("PageDown did not scroll process transcript")
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.processOffset >= m.processMaximumOffset() {
		t.Fatal("Up did not scroll process transcript")
	}
	_, cmd := m.Update(keyMsg("y"))
	if cmd == nil || m.message != "Clipboard copy requested" {
		t.Fatalf("non-empty copy cmd=%v message=%q", cmd != nil, m.message)
	}
	_, _ = m.Update(keyMsg("$"))
	if m.mode != modeStatus {
		t.Fatal("$ did not close process pane")
	}
}

func TestProcessFormattingSanitizationQuotingAndBounds(t *testing.T) {
	record := gitbackend.ProcessRecord{
		Dir: "/tmp/a dir", Args: []string{"push", "a'b", "$(not-run)"}, ExitCode: 7,
		Duration: 123 * time.Millisecond, Stdout: "ok\x1b[2J", Stderr: strings.Repeat("x", maxProcessStreamBytes+100),
	}
	text := formatProcessBatch("operation", []gitbackend.ProcessRecord{record}, errors.New("failed"))
	for _, want := range []string{"$ git -C '/tmp/a dir' push 'a'\\''b' '$(not-run)'", "exit 7 · 123ms", "stdout:", "stderr:", "process output truncated"} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted transcript omitted %q: %q", want, text)
		}
	}
	if strings.ContainsRune(text, '\x1b') {
		t.Fatal("formatted transcript retained terminal escape")
	}
	if got := humanQuote(""); got != "''" {
		t.Fatalf("empty human quote = %q", got)
	}
	truncated := truncateProcessText("diagnostic-start "+strings.Repeat("m", 100)+" diagnostic-end", 80)
	for _, want := range []string{"diagnostic-start", "process output truncated", "diagnostic-end"} {
		if !strings.Contains(truncated, want) {
			t.Fatalf("head/tail truncation omitted %q: %q", want, truncated)
		}
	}

	m := New(nil)
	for i := 0; i < maxProcessBatches+5; i++ {
		m.appendProcessBatch("op", []gitbackend.ProcessRecord{{Stdout: strings.Repeat("z", 20<<10)}}, nil)
	}
	if len(m.processBatches) > maxProcessBatches || len(m.processTranscript()) > maxProcessHistoryBytes {
		t.Fatalf("history unbounded: batches=%d bytes=%d", len(m.processBatches), len(m.processTranscript()))
	}
}

func TestFailedProcessBatchAddsGenericErrorOnlyWithoutCommandOutput(t *testing.T) {
	err := errors.New("launch failed")
	withoutOutput := formatProcessBatch("push", []gitbackend.ProcessRecord{{Args: []string{"push"}, ExitCode: -1}}, err)
	if !strings.Contains(withoutOutput, "error:\nlaunch failed") {
		t.Fatalf("generic failure diagnostic missing: %q", withoutOutput)
	}
	withStderr := formatProcessBatch("push", []gitbackend.ProcessRecord{{Args: []string{"push"}, ExitCode: 128, Stderr: "fatal: rejected"}}, errors.New("fatal: rejected"))
	if strings.Count(withStderr, "fatal: rejected") != 1 || strings.Contains(withStderr, "\nerror:\n") {
		t.Fatalf("stderr was duplicated by operation error: %q", withStderr)
	}
}

func TestSuccessfulRecordStdoutDoesNotSuppressLaterOperationError(t *testing.T) {
	text := formatProcessBatch("commit", []gitbackend.ProcessRecord{{
		Args: []string{"commit", "-m", "done"}, ExitCode: 0, Stdout: "created commit",
	}}, errors.New("follow-up query failed"))
	for _, want := range []string{"stdout:\ncreated commit", "error:\nfollow-up query failed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("batch omitted %q: %q", want, text)
		}
	}
}

func TestRefreshFailureCreatesDistinctVisibleFailedBatch(t *testing.T) {
	m := New(nil)
	m.width, m.height, m.loading, m.busy = 60, 20, false, true
	m.operationRequest = 4
	_, _ = m.Update(operationMsg{
		request: 4,
		name:    "push",
		records: []gitbackend.ProcessRecord{{Args: []string{"push"}, ExitCode: 0}},
		loadErr: errors.New("status query unavailable"),
	})
	if m.mode != modeProcess || len(m.processBatches) != 2 {
		t.Fatalf("refresh failure mode=%d batches=%d", m.mode, len(m.processBatches))
	}
	transcript := m.processTranscript()
	for _, want := range []string{"== push — complete ==", "== refresh after push — failed ==", "status query unavailable"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("refresh diagnostics omitted %q: %q", want, transcript)
		}
	}
	if pane := ansi.Strip(m.renderProcessPanel(m.width, m.processPanelHeight())); !strings.Contains(pane, "status query unavailable") {
		t.Fatalf("refresh failure is not visible at newest process output: %q", pane)
	}
}

func TestPushDestinationCollectorPersistsOnlyAfterReviewedPush(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	remotePath := filepath.Join(root, "remote.git")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	if out, err := exec.Command("git", "init", "--bare", remotePath).CombinedOutput(); err != nil {
		t.Fatalf("init bare remote: %v: %s", err, out)
	}
	repo, err := gitbackend.Init(context.Background(), work)
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	for _, setting := range [][2]string{{"user.name", "Test User"}, {"user.email", "test@example.test"}} {
		if out, err := exec.Command("git", "-C", work, "config", setting[0], setting[1]).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v: %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(work, "tracked.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	ctx := context.Background()
	if err := repo.Stage(ctx, []string{"tracked.txt"}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := repo.Commit(ctx, "initial"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := repo.AddRemote(ctx, "origin", remotePath, false); err != nil {
		t.Fatalf("add remote: %v", err)
	}
	summary, err := repo.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	remotes, err := repo.Remotes(ctx)
	if err != nil {
		t.Fatalf("remotes: %v", err)
	}

	m := New(repo)
	m.loading = false
	m.install(snapshot{summary: summary, remotes: remotes})
	_, _ = m.Update(keyMsg("P"))
	_, cmd := m.Update(keyMsg("p"))
	if cmd == nil {
		t.Fatal("push without pushRemote did not load its reviewed chooser")
	}
	_, _ = m.Update(cmd())
	if m.mode != modeWorkflow || m.workflow == nil {
		t.Fatalf("push chooser mode=%d workflow=%v message=%q", m.mode, m.workflow != nil, m.message)
	}
	assertUnset := func(stage string) {
		t.Helper()
		check := exec.Command("git", "-C", work, "config", "--get", "branch."+summary.Branch+".pushRemote")
		if out, err := check.CombinedOutput(); err == nil || len(out) != 0 {
			t.Fatalf("%s prematurely persisted pushRemote: out=%q err=%v", stage, out, err)
		}
	}
	assertUnset("dialog load")
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("first submit did not start review preflight")
	}
	_, _ = m.Update(cmd())
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatal("push preflight did not produce a distinct review")
	}
	if plan := strings.Join(m.workflow.review.Plan, "\n"); !strings.Contains(plan, "Persist branch pushRemote as origin") {
		t.Fatalf("review omitted persistent pushRemote semantics: %q", plan)
	}
	assertUnset("review")
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("confirmed review did not start push")
	}
	msg := cmd().(operationMsg)
	if msg.opErr != nil || len(msg.records) == 0 {
		t.Fatalf("reviewed push err=%v records=%+v", msg.opErr, msg.records)
	}
	_, _ = m.Update(msg)
	if got := strings.TrimSpace(string(mustCommandOutput(t, exec.Command("git", "-C", work, "config", "--get", "branch."+summary.Branch+".pushRemote")))); got != "origin" {
		t.Fatalf("persisted pushRemote = %q", got)
	}
	if got := strings.TrimSpace(string(mustCommandOutput(t, exec.Command("git", "--git-dir", remotePath, "rev-parse", "refs/heads/"+summary.Branch)))); got != summary.Head {
		t.Fatalf("reviewed push remote ref = %q, want %q", got, summary.Head)
	}
	if len(m.processBatches) != 1 {
		t.Fatalf("commands split across %d process batches", len(m.processBatches))
	}
	transcript := m.processTranscript()
	if !strings.Contains(transcript, " push ") {
		t.Fatalf("process batch omitted reviewed push: %q", transcript)
	}
}

func mustCommandOutput(t *testing.T, command *exec.Cmd) []byte {
	t.Helper()
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v: %s", strings.Join(command.Args, " "), err, out)
	}
	return out
}

func TestProcessPanelWrapsLongLinesAndAllContinuationsAreReachable(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height, m.mode = 32, 20, modeProcess
	m.appendProcessBatch("push", []gitbackend.ProcessRecord{{
		Dir:      "/a/very/long/worktree/location",
		Args:     []string{"push", "destination-with-long-command-argument"},
		ExitCode: 1,
		Stderr:   "diagnostic-prefix-with-many-characters DIAGNOSTIC-END",
	}}, errors.New("diagnostic-prefix-with-many-characters DIAGNOSTIC-END"))
	transcript := m.processTranscript()
	if strings.Contains(transcript, "destination-with-long-\n") {
		t.Fatalf("plain transcript was unexpectedly wrapped: %q", transcript)
	}
	lines := m.processLinesAtWidth(m.width - 2)
	if len(lines) <= len(strings.Split(transcript, "\n")) || m.processMaximumOffset() == 0 {
		t.Fatalf("narrow transcript did not wrap/scroll: lines=%d offset=%d", len(lines), m.processMaximumOffset())
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > m.width-2 {
			t.Fatalf("wrapped line width %d exceeds panel: %q", ansi.StringWidth(line), line)
		}
	}
	for _, target := range []string{"argument", "DIAGNOSTIC-END"} {
		reachable := false
		for offset := 0; offset <= m.processMaximumOffset(); offset++ {
			m.processOffset = offset
			if strings.Contains(ansi.Strip(m.renderProcessPanel(m.width, m.processPanelHeight())), target) {
				reachable = true
				break
			}
		}
		if !reachable {
			t.Fatalf("wrapped continuation %q was not reachable", target)
		}
	}
	if m.processPanelHeight() < 7 {
		t.Fatalf("common process pane remains too short: %d", m.processPanelHeight())
	}
}
