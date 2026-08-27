package ui

import (
	"path/filepath"
	"strings"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestBranchWorkflowIntegrationCreateSwitchRenameResetDelete(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	m := newE2EModel(t, r)

	openBranchWorkflow(t, m, "magit-branch-create")
	setBranchWorkflowValue(t, m, "branch", "topic")
	setBranchWorkflowValue(t, m, "start", "HEAD")
	executeBranchWorkflow(t, m)
	if got := r.git("branch", "--show-current"); got != "main" {
		t.Fatalf("create-only changed HEAD to %q", got)
	}

	openBranchWorkflow(t, m, "magit-checkout")
	setBranchWorkflowValue(t, m, "revision", "topic")
	executeBranchWorkflow(t, m)
	if got := r.git("branch", "--show-current"); got != "topic" {
		t.Fatalf("checkout selected %q", got)
	}
	r.write("topic.txt", "topic\n")
	r.git("add", ".")
	r.git("commit", "-m", "topic")

	openBranchWorkflow(t, m, "magit-branch-rename")
	setBranchWorkflowValue(t, m, "branch", "topic")
	setBranchWorkflowValue(t, m, "new", "renamed")
	executeBranchWorkflow(t, m)
	if got := r.git("branch", "--show-current"); got != "renamed" {
		t.Fatalf("renamed current branch = %q", got)
	}

	openBranchWorkflow(t, m, "magit-branch-checkout")
	setBranchWorkflowValue(t, m, "branch", "main")
	executeBranchWorkflow(t, m)

	openBranchWorkflow(t, m, "magit-branch-reset")
	setBranchWorkflowValue(t, m, "branch", "renamed")
	setBranchWorkflowValue(t, m, "revision", base)
	executeBranchWorkflow(t, m)
	if got := r.git("rev-parse", "renamed"); got != base {
		t.Fatalf("reset OID = %q, want %q", got, base)
	}

	openBranchWorkflow(t, m, "magit-branch-delete")
	setBranchWorkflowValue(t, m, "branch", "renamed")
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, reviewCmd := m.handleWorkflowKey(keyMsg("enter"))
	if reviewCmd == nil {
		t.Fatal("delete did not start reviewed preflight")
	}
	_, _ = m.Update(reviewCmd())
	if m.workflow.review == nil || len(m.workflow.review.Plan) < 3 || !strings.Contains(strings.Join(m.workflow.review.Plan, "\n"), base) {
		t.Fatalf("delete review did not display exact OID: %+v", m.workflow.review)
	}
	_, deleteCmd := m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, deleteCmd)
	if m.isError || r.git("branch", "--list", "renamed") != "" {
		t.Fatalf("reviewed delete failed: %q", m.message)
	}
}

func TestBranchRemoteCheckoutAndDefaultBranchAliases(t *testing.T) {
	remote := newUIE2ERepo(t)
	remote.write("base.txt", "base\n")
	remote.git("add", ".")
	remote.git("commit", "-m", "base")
	remote.git("switch", "-c", "topic")
	remote.write("topic.txt", "topic\n")
	remote.git("add", ".")
	remote.git("commit", "-m", "topic")
	topic := remote.git("rev-parse", "HEAD")
	remote.git("switch", "main")

	local := newUIE2ERepo(t)
	local.git("remote", "add", "origin", remote.dir)
	local.git("fetch", "origin")
	m := newE2EModel(t, local)

	openBranchWorkflow(t, m, "magit-checkout-remote-ref")
	setBranchWorkflowValue(t, m, "revision", "origin/topic")
	executeBranchWorkflow(t, m)
	if got := local.git("rev-parse", "HEAD"); got != topic {
		t.Fatalf("remote checkout HEAD = %q, want %q", got, topic)
	}
	if got := local.git("branch", "--show-current"); got != "" {
		t.Fatalf("remote checkout created/selected local branch %q", got)
	}

	local.git("switch", "-C", "main", "origin/main")
	remote.git("symbolic-ref", "HEAD", "refs/heads/main")
	sendE2EKey(t, m, keyMsg("g"))
	sendE2EKey(t, m, keyMsg("b"))
	sendE2EKey(t, m, keyMsg("B"))
	if m.workflow == nil {
		t.Fatalf("branch default-branch alias did not open: %q", m.message)
	}
}

func TestBranchWorkflowIntegrationDeleteRejectsCurrentAndStaleOIDToken(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	r.git("branch", "doomed")
	r.write("next.txt", "next\n")
	r.git("add", ".")
	r.git("commit", "-m", "next")
	next := r.git("rev-parse", "HEAD")
	m := newE2EModel(t, r)

	openBranchWorkflow(t, m, "magit-branch-delete")
	setBranchWorkflowValue(t, m, "branch", "main")
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, currentCmd := m.handleWorkflowKey(keyMsg("enter"))
	_, _ = m.Update(currentCmd())
	if m.workflow.review != nil || !strings.Contains(m.workflow.error, "current branch") {
		t.Fatalf("current branch delete was not rejected clearly: %q", m.workflow.error)
	}

	setBranchWorkflowValue(t, m, "branch", "doomed")
	m.workflow.error = ""
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, reviewCmd := m.handleWorkflowKey(keyMsg("enter"))
	_, _ = m.Update(reviewCmd())
	if m.workflow.review == nil {
		t.Fatal("delete review missing")
	}
	r.git("branch", "-f", "doomed", next)
	_, deleteCmd := m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, deleteCmd)
	got := r.git("rev-parse", "doomed")
	if !m.isError || !strings.Contains(m.message, "stale") || got != next {
		t.Fatalf("stale delete result: message=%q branch=%q base=%q", m.message, got, base)
	}
}

func TestBranchWorkflowIntegrationWorktreesAndConfiguration(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	r.git("branch", "existing")
	r.git("remote", "add", "origin", filepath.Join(t.TempDir(), "remote.git"))
	m := newE2EModel(t, r)
	root := t.TempDir()

	existingPath := filepath.Join(root, "existing-worktree")
	openBranchWorkflow(t, m, "magit-worktree-checkout")
	setBranchWorkflowValue(t, m, "path", existingPath)
	setBranchWorkflowValue(t, m, "revision", "existing")
	executeBranchWorkflow(t, m)
	if got := r.git("-C", existingPath, "branch", "--show-current"); got != "existing" {
		t.Fatalf("existing worktree branch = %q", got)
	}

	newPath := filepath.Join(root, "new-worktree")
	openBranchWorkflow(t, m, "magit-worktree-branch")
	setBranchWorkflowValue(t, m, "path", newPath)
	setBranchWorkflowValue(t, m, "branch", "worktree-topic")
	setBranchWorkflowValue(t, m, "start", "HEAD")
	executeBranchWorkflow(t, m)
	if got := r.git("-C", newPath, "branch", "--show-current"); got != "worktree-topic" {
		t.Fatalf("new worktree branch = %q", got)
	}

	openBranchWorkflow(t, m, "magit-branch-configure")
	fields := map[string]WorkflowField{}
	for _, field := range m.workflow.dialog.Fields {
		fields[field.Name] = field
	}
	for _, name := range []string{"branch", "description_action", "description", "upstream", "rebase", "push_remote", "pull_rebase", "push_default"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("configuration field %s missing", name)
		}
	}
	setBranchWorkflowValue(t, m, "branch", "main")
	setBranchWorkflowValue(t, m, "description_action", "set")
	setBranchWorkflowValue(t, m, "description", "primary line")
	setBranchWorkflowValue(t, m, "upstream", "existing")
	setBranchWorkflowValue(t, m, "rebase", "merges")
	setBranchWorkflowValue(t, m, "push_remote", "origin")
	setBranchWorkflowValue(t, m, "pull_rebase", "true")
	setBranchWorkflowValue(t, m, "push_default", "origin")
	executeBranchWorkflow(t, m)
	for key, want := range map[string]string{
		"branch.main.description": "primary line", "branch.main.rebase": "merges",
		"branch.main.pushremote": "origin", "pull.rebase": "true", "remote.pushdefault": "origin",
	} {
		if got := r.git("config", "--get", key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := r.git("rev-parse", "--abbrev-ref", "main@{upstream}"); got != "existing" {
		t.Errorf("upstream = %q", got)
	}
}

func TestBranchWorktreeIntegrationCurrentBranchAndUnsafeDestinationAreRejected(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	m := newE2EModel(t, r)

	openBranchWorkflow(t, m, "magit-worktree-checkout")
	setBranchWorkflowValue(t, m, "path", filepath.Join(t.TempDir(), "linked"))
	setBranchWorkflowValue(t, m, "revision", "main")
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd := m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, cmd)
	if !m.isError {
		t.Fatal("checked-out branch was implicitly forced into another worktree")
	}

	// The backend contract, not UI path guessing, rejects the filesystem root.
	m = newE2EModel(t, r)
	openBranchWorkflow(t, m, "magit-worktree-branch")
	setBranchWorkflowValue(t, m, "path", string(filepath.Separator))
	setBranchWorkflowValue(t, m, "branch", "unsafe")
	setBranchWorkflowValue(t, m, "start", "HEAD")
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd = m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, cmd)
	if !m.isError || !strings.Contains(m.message, gitbackend.ErrUnsafeDestination.Error()) {
		t.Fatalf("unsafe destination error = %q", m.message)
	}
}
