package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func newUIBareRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "remote.git")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "init", "--bare")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare remote: %v\n%s", err, out)
	}
	return dir
}

func setRemoteWorkflowValue(t *testing.T, m *Model, name, value string) {
	t.Helper()
	for i := range m.workflow.dialog.Fields {
		if m.workflow.dialog.Fields[i].Name == name {
			m.workflow.dialog.Fields[i].Value = value
			return
		}
	}
	t.Fatalf("workflow field %q not found", name)
}

func loadRemoteWorkflow(t *testing.T, m *Model, command keymap.CommandID) {
	t.Helper()
	cmd, handled := m.performWorkflow(WorkflowCommand{ID: command})
	if !handled || cmd == nil {
		t.Fatalf("%s did not start async choices", command)
	}
	runE2ECmd(t, m, cmd)
	if m.mode != modeWorkflow {
		t.Fatalf("%s mode = %v, message %q", command, m.mode, m.message)
	}
}

func reviewAndSubmitRemoteWorkflow(t *testing.T, m *Model) {
	t.Helper()
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd := m.handleWorkflowKey(keyMsg("enter"))
	if cmd == nil {
		t.Fatalf("review did not start: %s", m.workflow.error)
	}
	runE2ECmd(t, m, cmd)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("review not displayed: mode=%v message=%q", m.mode, m.message)
	}
	_, cmd = m.handleWorkflowKey(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("reviewed submit did not start")
	}
	runE2ECmd(t, m, cmd)
	if m.isError {
		t.Fatalf("operation failed: %s", m.message)
	}
}

func TestRemoteWorkflowIntegrationAddConfigureRenamePruneRemoveAndCancel(t *testing.T) {
	local := newUIE2ERepo(t)
	local.write("base", "base\n")
	local.git("add", "base")
	local.git("commit", "-m", "base")
	bare := newUIBareRemote(t)
	m := newE2EModel(t, local)

	remoteAddWorkflow(m, WorkflowCommand{ID: keymap.CommandAddRemote, Options: map[keymap.CommandID]OptionValue{commandRemoteFetchAfterAdd: {Enabled: true}}})
	setRemoteWorkflowValue(t, m, "name", "origin")
	setRemoteWorkflowValue(t, m, "url", bare)
	m.workflow.dialog.Fields[2].Bool = true
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd := m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, cmd)
	if got := local.git("remote"); got != "origin" {
		t.Fatalf("add remote = %q", got)
	}

	loadRemoteWorkflow(t, m, commandRemoteConfigure)
	setRemoteWorkflowValue(t, m, "U-mode", "replace")
	setRemoteWorkflowValue(t, m, "U", `["+refs/heads/*:refs/remotes/origin/*"]`)
	setRemoteWorkflowValue(t, m, "S-mode", "clear")
	setRemoteWorkflowValue(t, m, "O", "none")
	reviewAndSubmitRemoteWorkflow(t, m)
	if got := local.git("config", "--get", "remote.origin.tagopt"); got != "--no-tags" {
		t.Fatalf("tagopt = %q", got)
	}

	loadRemoteWorkflow(t, m, commandRemoteRename)
	setRemoteWorkflowValue(t, m, "new", "upstream")
	// Cancelling a reviewed destructive workflow must not mutate anything.
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd = m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, cmd)
	_, _ = m.handleWorkflowKey(keyMsg("esc"))
	if got := local.git("remote"); got != "origin" {
		t.Fatalf("cancelled rename = %q", got)
	}
	loadRemoteWorkflow(t, m, commandRemoteRename)
	setRemoteWorkflowValue(t, m, "new", "upstream")
	reviewAndSubmitRemoteWorkflow(t, m)
	if got := local.git("remote"); got != "upstream" {
		t.Fatalf("rename = %q", got)
	}

	local.git("push", "upstream", "main:stale")
	local.git("fetch", "upstream")
	gitCmd := exec.Command("git", "-C", bare, "update-ref", "-d", "refs/heads/stale")
	gitCmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C")
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("delete remote ref: %v\n%s", err, out)
	}
	loadRemoteWorkflow(t, m, commandRemotePrune)
	reviewAndSubmitRemoteWorkflow(t, m)
	if got := local.git("for-each-ref", "--format=%(refname)", "refs/remotes/upstream/stale"); got != "" {
		t.Fatalf("prune left %q", got)
	}

	loadRemoteWorkflow(t, m, commandRemoteRemove)
	reviewAndSubmitRemoteWorkflow(t, m)
	if got := local.git("remote"); got != "" {
		t.Fatalf("remove left %q", got)
	}
}

func TestRemoteWorkflowIntegrationUnshallow(t *testing.T) {
	seed := newUIE2ERepo(t)
	seed.write("base", "base\n")
	seed.git("add", "base")
	seed.git("commit", "-m", "one")
	seed.write("base", "two\n")
	seed.git("commit", "-am", "two")
	bare := newUIBareRemote(t)
	seed.git("remote", "add", "origin", bare)
	seed.git("push", "origin", "main")
	cmd := exec.Command("git", "-C", bare, "symbolic-ref", "HEAD", "refs/heads/main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set bare HEAD: %v\n%s", err, out)
	}

	cloneDir := filepath.Join(t.TempDir(), "clone")
	cmd = exec.Command("git", "clone", "--depth", "1", "file://"+bare, cloneDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shallow clone: %v\n%s", err, out)
	}
	repo, err := gitbackend.Discover(cloneDir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(repo)
	m.width, m.height, m.loading = 100, 30, false
	m.snapshotLoader = nil
	loadRemoteWorkflow(t, m, commandRemoteUnshallow)
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, operation := m.handleWorkflowKey(keyMsg("enter"))
	runE2ECmd(t, m, operation)
	if _, err := os.Stat(filepath.Join(repo.GitDir(), "shallow")); !os.IsNotExist(err) {
		t.Fatalf("repository remains shallow: %v", err)
	}
}
