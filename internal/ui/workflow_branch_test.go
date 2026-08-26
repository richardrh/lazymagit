package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

var connectedBranchCommands = []string{
	"magit-checkout",
	"magit-branch-checkout",
	"magit-branch-orphan",
	"magit-branch-and-checkout",
	"magit-worktree-checkout",
	"magit-branch-create",
	"magit-worktree-branch",
	"magit-branch-configure",
	"magit-branch-rename",
	"magit-branch-reset",
	"magit-branch-delete",
}

func branchBinding(t *testing.T, upstream string) keymap.Binding {
	t.Helper()
	for _, binding := range keymap.Registry() {
		if binding.Scheme == keymap.SchemeMagit && binding.Context == keymap.ContextTransient+branchPrefix && binding.UpstreamCommand == upstream {
			return binding
		}
	}
	t.Fatalf("branch registry occurrence for %s not found", upstream)
	return keymap.Binding{}
}

func TestBranchWorkflowExactHandlerRegistrationAndAvailability(t *testing.T) {
	m := New(&gitbackend.Repository{})
	catalog, ok := m.transientCatalog(branchPrefix)
	if !ok {
		t.Fatal("branch transient is missing")
	}
	for _, upstream := range connectedBranchCommands {
		binding := branchBinding(t, upstream)
		if _, ok := m.workflowHandlers[binding.Command]; !ok {
			t.Errorf("%s (%s) has no handler", binding.Occurrence, upstream)
		}
		entry, ok := catalog.occurrence(binding.Occurrence)
		if !ok || !entry.Available {
			t.Errorf("%s (%s) unavailable: %+v", binding.Occurrence, upstream, entry)
		}
	}

	for _, upstream := range []string{
		"magit-update-default-branch",
		"magit-checkout-remote-ref", "magit-branch-spinoff", "magit-branch-spinout",
		"magit-branch-shelve", "magit-branch-unshelve",
	} {
		binding := branchBinding(t, upstream)
		if _, ok := m.workflowHandlers[binding.Command]; ok {
			t.Errorf("unsupported %s was registered", upstream)
		}
		entry, _ := catalog.occurrence(binding.Occurrence)
		if entry.Available {
			t.Errorf("unsupported %s is available", upstream)
		}
	}
	// The generic transient editor presents registry infixes. The branch domain
	// never registers -r and every connected suffix rejects it, so toggling it
	// cannot silently approximate recursive checkout semantics.
	recurse := branchBinding(t, "transient:magit-branch:--recurse-submodules")
	if _, ok := m.workflowHandlers[recurse.Command]; ok {
		t.Fatal("recursive checkout infix acquired an execution handler")
	}
	checkout := branchBinding(t, "magit-checkout")
	cmd, handled := m.performWorkflow(WorkflowCommand{ID: checkout.Command, Options: map[keymap.CommandID]OptionValue{recurse.Command: {Enabled: true}}})
	if !handled || cmd != nil || !m.isError || !strings.Contains(m.message, "recursive-submodule") {
		t.Fatalf("recursive option was not rejected: handled=%v cmd=%v message=%q", handled, cmd != nil, m.message)
	}
}

func TestBranchConfigurationInfixesRemainConditionalAndAreCoveredByC(t *testing.T) {
	m := New(&gitbackend.Repository{})
	catalog, _ := m.transientCatalog(branchPrefix)
	for _, upstream := range []string{
		"magit-branch.<branch>.description", "magit-branch.<branch>.merge/remote",
		"magit-branch.<branch>.rebase", "magit-branch.<branch>.pushRemote",
		"magit-pull.rebase", "magit-remote.pushDefault",
	} {
		binding := branchBinding(t, upstream)
		entry, ok := catalog.occurrence(binding.Occurrence)
		if !ok || entry.Available || entry.Kind != keymap.KindInfix || !strings.Contains(entry.Reason, "Configure dialog") {
			t.Errorf("conditional configuration occurrence %s = %+v", upstream, entry)
		}
	}
	configure := branchBinding(t, "magit-branch-configure")
	if _, ok := m.workflowHandlers[configure.Command]; !ok {
		t.Fatal("C configuration workflow is not registered")
	}
}

func TestBranchHandlerWithoutRepositoryFailsWithoutBlocking(t *testing.T) {
	m := New(nil)
	binding := branchBinding(t, "magit-branch-create")
	cmd, handled := m.performWorkflow(WorkflowCommand{ID: binding.Command, Occurrence: binding.Occurrence, Prefix: branchPrefix})
	if !handled || cmd != nil || !m.isError || !strings.Contains(m.message, "requires a repository") {
		t.Fatalf("nil repository route: handled=%v cmd=%v error=%v message=%q", handled, cmd != nil, m.isError, m.message)
	}
}

func TestBranchWorkflowCancelDropsLoadedDialog(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	m := newE2EModel(t, r)
	openBranchWorkflow(t, m, "magit-branch-create")
	if m.mode != modeWorkflow {
		t.Fatal("branch workflow did not open")
	}
	_, cmd := m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.workflow != nil || m.mode != modeStatus || !strings.Contains(m.message, "cancelled") {
		t.Fatalf("cancel retained workflow: mode=%v message=%q", m.mode, m.message)
	}
	_ = cmd // Detail refresh is optional when no detail row is selected.
}

func openBranchWorkflow(t *testing.T, m *Model, upstream string) {
	t.Helper()
	binding := branchBinding(t, upstream)
	cmd, handled := m.performWorkflow(WorkflowCommand{ID: binding.Command, Occurrence: binding.Occurrence, Prefix: branchPrefix})
	if !handled || cmd == nil {
		t.Fatalf("open %s: handled=%v cmd=%v message=%q", upstream, handled, cmd != nil, m.message)
	}
	_, next := m.Update(cmd())
	if next != nil || m.workflow == nil || m.mode != modeWorkflow {
		t.Fatalf("load %s did not open dialog: mode=%v message=%q", upstream, m.mode, m.message)
	}
}

func setBranchWorkflowValue(t *testing.T, m *Model, name, value string) {
	t.Helper()
	for i := range m.workflow.dialog.Fields {
		if m.workflow.dialog.Fields[i].Name == name {
			m.workflow.dialog.Fields[i].Value = value
			return
		}
	}
	t.Fatalf("workflow field %q not found", name)
}

func executeBranchWorkflow(t *testing.T, m *Model) {
	t.Helper()
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd := m.handleWorkflowKey(keyMsg("enter"))
	if cmd == nil {
		t.Fatalf("workflow submit returned no command: %+v", m.workflow)
	}
	runE2ECmd(t, m, cmd)
	if m.workflow != nil && m.workflow.review != nil {
		_, cmd = m.handleWorkflowKey(keyMsg("enter"))
		if cmd == nil {
			t.Fatal("reviewed workflow produced no execute command")
		}
		runE2ECmd(t, m, cmd)
	}
	if m.isError {
		t.Fatalf("workflow failed: %s", m.message)
	}
}
