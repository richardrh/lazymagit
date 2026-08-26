package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func openSubprojectDialog(t *testing.T, m *Model, handler WorkflowHandler, command WorkflowCommand) {
	t.Helper()
	runE2ECmd(t, m, handler(m, command))
	if m.workflow == nil {
		t.Fatalf("workflow did not open: %q", m.message)
	}
}

func typeWorkflowField(t *testing.T, m *Model, name, value string) {
	t.Helper()
	index := -1
	for i := range m.workflow.dialog.Fields {
		if m.workflow.dialog.Fields[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("workflow field %q is absent", name)
	}
	m.workflow.field = index
	for m.workflow.dialog.Fields[index].Value != "" {
		sendE2EKey(t, m, keyMsg("backspace"))
	}
	for _, r := range value {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func executeSubprojectDialog(t *testing.T, m *Model, reviewed bool) {
	t.Helper()
	m.workflow.field = len(m.workflow.dialog.Fields)
	sendE2EKey(t, m, keyMsg("enter"))
	if reviewed {
		if m.workflow == nil || m.workflow.review == nil {
			t.Fatalf("review was not displayed: %q", m.message)
		}
		sendE2EKey(t, m, keyMsg("enter"))
	}
	if m.isError {
		t.Fatalf("workflow failed: %q", m.message)
	}
}

func TestSubmoduleKeyDrivenLifecycle(t *testing.T) {
	t.Setenv("GIT_ALLOW_PROTOCOL", "file") // Local file transport is test-scoped.
	source := newUIE2ERepo(t)
	source.write("module.txt", "one\n")
	source.git("add", ".")
	source.git("commit", "-m", "module one")

	host := newUIE2ERepo(t)
	host.write("host.txt", "host\n")
	host.git("add", ".")
	host.git("commit", "-m", "host")
	m := newE2EModel(t, host)

	openSubprojectDialog(t, m, submoduleAddWorkflow, WorkflowCommand{Prefix: submodulePrefix})
	typeWorkflowField(t, m, "url", source.dir)
	typeWorkflowField(t, m, "path", "modules/demo")
	executeSubprojectDialog(t, m, false)
	if got := host.git("submodule", "status", "--", "modules/demo"); !strings.Contains(got, "modules/demo") {
		t.Fatalf("added submodule is absent: %q", got)
	}
	// Deinit without --force is intentionally safe and rejects staged gitlink
	// changes. Commit the completed add so this lifecycle exercises clean deinit;
	// force remains available only through its separately reviewed transient.
	host.git("commit", "-m", "add demo submodule")

	openSubprojectDialog(t, m, submoduleSyncWorkflow, WorkflowCommand{Prefix: submodulePrefix})
	executeSubprojectDialog(t, m, false)
	openSubprojectDialog(t, m, submoduleDeinitWorkflow, WorkflowCommand{Prefix: submodulePrefix})
	executeSubprojectDialog(t, m, true)
	if got := host.git("submodule", "status", "--", "modules/demo"); !strings.HasPrefix(got, "-") {
		t.Fatalf("deinitialized status = %q", got)
	}

	openSubprojectDialog(t, m, submodulePopulateWorkflow, WorkflowCommand{Prefix: submodulePrefix})
	executeSubprojectDialog(t, m, false)
	if got := host.git("submodule", "status", "--", "modules/demo"); strings.HasPrefix(got, "-") {
		t.Fatalf("update --init left module deinitialized: %q", got)
	}

	openSubprojectDialog(t, m, submoduleUpdateWorkflow, WorkflowCommand{Prefix: submodulePrefix})
	executeSubprojectDialog(t, m, false)
	openSubprojectDialog(t, m, submoduleRemoveWorkflow, WorkflowCommand{Prefix: submodulePrefix})
	executeSubprojectDialog(t, m, true)
	if got := host.git("submodule", "status"); got != "" {
		t.Fatalf("removed submodule remains configured: %q", got)
	}
}

func TestSparseCheckoutKeyDrivenInitSetAddDisable(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("one/file.txt", "one\n")
	r.write("two/file.txt", "two\n")
	r.git("add", ".")
	r.git("commit", "-m", "tree")
	m := newE2EModel(t, r)

	openSubprojectDialog(t, m, sparseEnableWorkflow, WorkflowCommand{Prefix: sparsePrefix})
	executeSubprojectDialog(t, m, true)
	openSubprojectDialog(t, m, sparseSetWorkflow, WorkflowCommand{Prefix: sparsePrefix})
	typeWorkflowField(t, m, "patterns", "one")
	executeSubprojectDialog(t, m, true)
	if _, err := os.Stat(filepath.Join(r.dir, "two", "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("sparse set did not remove two/: %v", err)
	}

	openSubprojectDialog(t, m, sparseAddWorkflow, WorkflowCommand{Prefix: sparsePrefix})
	typeWorkflowField(t, m, "patterns", "two")
	executeSubprojectDialog(t, m, true)
	if _, err := os.Stat(filepath.Join(r.dir, "two", "file.txt")); err != nil {
		t.Fatalf("sparse add did not restore two/: %v", err)
	}

	openSubprojectDialog(t, m, sparseDisableWorkflow, WorkflowCommand{Prefix: sparsePrefix})
	executeSubprojectDialog(t, m, true)
	if got := r.git("config", "--bool", "core.sparseCheckout"); got != "false" {
		t.Fatalf("sparse checkout remains enabled: %q", got)
	}
}

func TestSubtreeKeyDrivenAddPullPushAndInjectionRejection(t *testing.T) {
	if err := exec.Command("git", "subtree").Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 129 {
			t.Skip("git subtree is unavailable")
		}
	}
	t.Setenv("GIT_ALLOW_PROTOCOL", "file") // Local file transport is test-scoped.
	source := newUIE2ERepo(t)
	source.write("upstream.txt", "one\n")
	source.git("add", ".")
	source.git("commit", "-m", "upstream one")
	host := newUIE2ERepo(t)
	host.write("host.txt", "host\n")
	host.git("add", ".")
	host.git("commit", "-m", "host")
	m := newE2EModel(t, host)

	openSubprojectDialog(t, m, subtreeWorkflow("add"), WorkflowCommand{Prefix: subtreePrefix})
	typeWorkflowField(t, m, "prefix", "vendor/demo")
	typeWorkflowField(t, m, "repository", source.dir)
	typeWorkflowField(t, m, "ref", "main")
	executeSubprojectDialog(t, m, false)

	source.write("second.txt", "two\n")
	source.git("add", ".")
	source.git("commit", "-m", "upstream two")
	openSubprojectDialog(t, m, subtreeWorkflow("pull"), WorkflowCommand{Prefix: subtreePrefix})
	typeWorkflowField(t, m, "prefix", "vendor/demo")
	typeWorkflowField(t, m, "repository", source.dir)
	typeWorkflowField(t, m, "ref", "main")
	executeSubprojectDialog(t, m, false)
	if _, err := os.Stat(filepath.Join(host.dir, "vendor", "demo", "second.txt")); err != nil {
		t.Fatalf("subtree pull result: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "destination.git")
	cmd := exec.Command("git", "init", "--bare", destination)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init subtree destination: %v\n%s", err, out)
	}
	openSubprojectDialog(t, m, subtreeWorkflow("push"), WorkflowCommand{Prefix: subtreePrefix})
	typeWorkflowField(t, m, "prefix", "vendor/demo")
	typeWorkflowField(t, m, "repository", destination)
	typeWorkflowField(t, m, "ref", "main")
	executeSubprojectDialog(t, m, false)

	openSubprojectDialog(t, m, subtreeWorkflow("add"), WorkflowCommand{Prefix: subtreePrefix})
	typeWorkflowField(t, m, "prefix", "vendor/bad")
	typeWorkflowField(t, m, "repository", "--upload-pack=bad")
	typeWorkflowField(t, m, "ref", "main")
	m.workflow.field = len(m.workflow.dialog.Fields)
	sendE2EKey(t, m, keyMsg("enter"))
	if m.workflow == nil || !strings.Contains(m.workflow.error, "option-like") {
		t.Fatalf("option injection was not rejected in-dialog: %q", m.message)
	}
}
