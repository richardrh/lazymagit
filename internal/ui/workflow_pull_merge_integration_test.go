package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPullMergeKeyDrivenIntegration(t *testing.T) {
	local, _, bare := newPushWorkflowE2E(t)
	local.git("push", "-u", "origin", "main")
	peer := newUIE2ERepo(t)
	peer.git("remote", "add", "origin", bare)
	peer.git("fetch", "origin")
	peer.git("reset", "--hard", "origin/main")
	peer.write("upstream", "upstream\n")
	peer.git("add", "upstream")
	peer.git("commit", "-m", "upstream")
	peer.git("push", "origin", "main")

	m := newE2EModel(t, local)
	sendE2EKey(t, m, keyMsg("F"))
	sendE2EKey(t, m, keyMsg("u"))
	if got := local.git("rev-parse", "HEAD"); got != peer.git("rev-parse", "main") {
		t.Fatalf("upstream pull HEAD = %s", got)
	}

	peer.git("switch", "-c", "elsewhere")
	peer.write("elsewhere", "elsewhere\n")
	peer.git("add", "elsewhere")
	peer.git("commit", "-m", "elsewhere")
	peer.git("push", "origin", "elsewhere")
	local.git("fetch", "origin")
	sendE2EKey(t, m, keyMsg("g"))
	sendE2EKey(t, m, keyMsg("F"))
	sendE2EKey(t, m, keyMsg("e"))
	selectWorkflowValue(t, m, "target", "0") // origin/elsewhere sorts before origin/main.
	submitWorkflowByKeys(t, m, false)
	if _, err := os.Stat(filepath.Join(local.dir, "elsewhere")); err != nil {
		t.Fatalf("elsewhere pull did not update worktree: %v", err)
	}

	// Fast-forward merge.
	base := local.git("rev-parse", "HEAD")
	local.git("switch", "-c", "ff-topic")
	local.write("ff", "ff\n")
	local.git("add", "ff")
	local.git("commit", "-m", "ff")
	ffHead := local.git("rev-parse", "HEAD")
	local.git("switch", "main")
	local.git("reset", "--hard", base)
	sendE2EKey(t, m, keyMsg("g"))
	openMergeTarget(t, m, "refs/heads/ff-topic", false)
	if got := local.git("rev-parse", "HEAD"); got != ffHead {
		t.Fatalf("fast-forward merge HEAD = %s, want %s", got, ffHead)
	}

	// Reviewed --no-ff merge and cancellation both stay key-driven.
	local.git("switch", "-c", "noff-topic")
	local.write("topic", "topic\n")
	local.git("add", "topic")
	local.git("commit", "-m", "topic")
	local.git("switch", "main")
	local.write("main", "main\n")
	local.git("add", "main")
	local.git("commit", "-m", "main")
	sendE2EKey(t, m, keyMsg("g"))
	sendE2EKey(t, m, keyMsg("m"))
	sendE2EKey(t, m, keyMsg("-n"))
	sendE2EKey(t, m, keyMsg("m"))
	selectWorkflowValue(t, m, "target", "refs/heads/noff-topic")
	submitWorkflowByKeys(t, m, true)
	if parents := local.git("rev-list", "--parents", "-n", "1", "HEAD"); len(strings.Fields(parents)) != 3 {
		t.Fatalf("no-ff merge was not a two-parent commit: %s", parents)
	}
	sendE2EKey(t, m, keyMsg("m"))
	sendE2EKey(t, m, keyMsg("m"))
	beforeCancel := local.git("rev-parse", "HEAD")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if got := local.git("rev-parse", "HEAD"); got != beforeCancel {
		t.Fatalf("cancel changed HEAD from %s to %s", beforeCancel, got)
	}

	// Conflict, resolution/continue, then another conflict and reviewed abort.
	makeConflictBranches(t, local)
	sendE2EKey(t, m, keyMsg("g"))
	openMergeTarget(t, m, "refs/heads/conflict-topic", true)
	local.write("conflict", "resolved\n")
	sendE2EKey(t, m, keyMsg("g"))
	selectE2EPath(t, m, "conflict", rowUnstaged)
	sendE2EKey(t, m, keyMsg("s"))
	sendE2EKey(t, m, keyMsg("m"))
	sendE2EKey(t, m, keyMsg("m"))
	submitWorkflowByKeys(t, m, false)
	if status := local.git("status", "--porcelain"); status != "" {
		t.Fatalf("continue left worktree changes: %s", status)
	}

	local.git("reset", "--hard", "conflict-main")
	if err := os.Remove(filepath.Join(local.dir, "conflict")); err != nil {
		t.Fatal(err)
	}
	local.git("reset", "--hard", "conflict-main")
	sendE2EKey(t, m, keyMsg("g"))
	openMergeTarget(t, m, "refs/heads/conflict-topic", true)
	sendE2EKey(t, m, keyMsg("m"))
	sendE2EKey(t, m, keyMsg("a"))
	submitWorkflowByKeys(t, m, true)
	if local.git("status", "--porcelain") != "" {
		t.Fatalf("abort left worktree changes: %s", local.git("status", "--porcelain"))
	}
}

func openMergeTarget(t *testing.T, m *Model, target string, expectFailure bool) {
	t.Helper()
	sendE2EKey(t, m, keyMsg("m"))
	sendE2EKey(t, m, keyMsg("m"))
	selectWorkflowValue(t, m, "target", target)
	submitWorkflowByKeys(t, m, true)
	if expectFailure && !m.isError {
		t.Fatal("conflicting merge did not report an operation error")
	}
}

func selectWorkflowValue(t *testing.T, m *Model, field, value string) {
	t.Helper()
	if m.workflow == nil {
		t.Fatal("workflow is not open")
	}
	for i := 0; i < len(m.workflow.dialog.Fields); i++ {
		if m.workflow.dialog.Fields[i].Name != field {
			continue
		}
		for attempts := 0; m.workflow.dialog.Fields[i].Value != value && attempts <= len(m.workflow.dialog.Fields[i].Choices); attempts++ {
			sendE2EKey(t, m, keyMsg("right"))
		}
		if m.workflow.dialog.Fields[i].Value != value {
			t.Fatalf("workflow choice %s=%q is unavailable", field, value)
		}
		return
	}
	t.Fatalf("workflow field %q is unavailable", field)
}

func submitWorkflowByKeys(t *testing.T, m *Model, reviewed bool) {
	t.Helper()
	for m.workflow != nil && m.workflow.field < len(m.workflow.dialog.Fields) {
		sendE2EKey(t, m, keyMsg("tab"))
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if reviewed && m.workflow != nil {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}
}

func makeConflictBranches(t *testing.T, local *uiE2ERepo) {
	t.Helper()
	local.write("conflict", "base\n")
	local.git("add", "conflict")
	local.git("commit", "-m", "conflict base")
	local.git("branch", "conflict-base")
	local.git("switch", "-c", "conflict-topic")
	local.write("conflict", "topic\n")
	local.git("add", "conflict")
	local.git("commit", "-m", "conflict topic")
	local.git("switch", "main")
	local.write("conflict", "main\n")
	local.git("add", "conflict")
	local.git("commit", "-m", "conflict main")
	local.git("branch", "conflict-main")
}
