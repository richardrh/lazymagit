package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func fetchE2ERemote(t *testing.T, local *uiE2ERepo) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", remote, "init", "--bare")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare remote: %v\n%s", err, out)
	}
	local.git("remote", "add", "origin", remote)
	local.git("push", "-u", "origin", "main")
	local.git("config", "remote.pushDefault", "origin")
	return remote
}

func fetchE2ECommand() WorkflowCommand {
	return WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		optionFetchPrune: {Enabled: true},
		optionFetchTags:  {Enabled: true},
		optionFetchForce: {Enabled: true},
	}}
}

func runFetchOperation(t *testing.T, m *Model, cmd tea.Cmd) operationMsg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("fetch path produced no asynchronous command: mode=%v message=%q", m.mode, m.message)
	}
	raw := cmd()
	msg, ok := raw.(operationMsg)
	if !ok {
		t.Fatalf("fetch command returned %T", raw)
	}
	if msg.opErr != nil {
		t.Fatalf("fetch operation: %v", msg.opErr)
	}
	return msg
}

func TestFetchWorkflowIntegrationSuffixesAgainstLocalBareRemote(t *testing.T) {
	local := newUIE2ERepo(t)
	local.write("base", "base\n")
	local.git("add", "--", "base")
	local.git("commit", "-m", "base")
	fetchE2ERemote(t, local)
	repo, err := gitbackend.Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}

	direct := []struct {
		name string
		id   keymap.CommandID
		want []string
	}{
		{"p", keymap.CommandFetchPush, []string{"fetch", "--prune", "--tags", "--force", "--", "origin"}},
		{"u", keymap.CommandFetchUpstream, []string{"fetch", "--prune", "--tags", "--force", "--", "origin"}},
		{"a", keymap.CommandFetchAll, []string{"fetch", "--prune", "--tags", "--force", "--all"}},
		{"m", commandFetchModules, []string{"fetch", "--prune", "--tags", "--force", "--recurse-submodules=yes"}},
	}
	for _, test := range direct {
		t.Run(test.name, func(t *testing.T) {
			m := New(repo)
			m.snapshot.pushRemote = "origin"
			command := fetchE2ECommand()
			command.ID = test.id
			cmd, handled := m.performWorkflow(command)
			if !handled {
				t.Fatal("suffix was not routed")
			}
			msg := runFetchOperation(t, m, cmd)
			if len(msg.records) != 1 || !reflect.DeepEqual(msg.records[0].Args, test.want) {
				t.Fatalf("process records = %#v; want %#v", msg.records, test.want)
			}
		})
	}

	dialogs := []struct {
		name string
		id   keymap.CommandID
		want []string
		set  func(*Model)
	}{
		{"e", keymap.CommandFetchElsewhere, []string{"fetch", "--prune", "--tags", "--force", "--", "origin"}, nil},
		{"o", commandFetchBranch, []string{"fetch", "--prune", "--tags", "--force", "--", "origin", "main"}, nil},
		{"r", commandFetchRefspec, []string{"fetch", "--prune", "--tags", "--force", "--", "origin", "+refs/heads/main:refs/remotes/origin/copied"}, func(m *Model) {
			m.workflow.dialog.Fields[1].Value = "+refs/heads/main:refs/remotes/origin/copied"
		}},
	}
	for _, test := range dialogs {
		t.Run(test.name, func(t *testing.T) {
			m := New(repo)
			command := fetchE2ECommand()
			command.ID = test.id
			load, handled := m.performWorkflow(command)
			if !handled || load == nil {
				t.Fatal("dialog suffix was not asynchronously routed")
			}
			_, _ = m.Update(load())
			if m.workflow == nil {
				t.Fatalf("dialog did not load: %s", m.message)
			}
			if test.set != nil {
				test.set(m)
			}
			values := m.workflowValues()
			if err := validateWorkflow(m.workflow.dialog, values); err != nil {
				t.Fatal(err)
			}
			if m.workflow.dialog.Preflight != nil {
				if err := m.workflow.dialog.Preflight(context.Background(), values); err != nil {
					t.Fatal(err)
				}
			}
			operation := m.submitWorkflow(values)
			msg := runFetchOperation(t, m, operation)
			if len(msg.records) != 1 || !reflect.DeepEqual(msg.records[0].Args, test.want) {
				t.Fatalf("process records = %#v; want %#v", msg.records, test.want)
			}
		})
	}

	t.Run("C", func(t *testing.T) {
		m := New(repo)
		load, handled := m.performWorkflow(WorkflowCommand{ID: commandFetchConfigure})
		if !handled || load == nil {
			t.Fatal("configuration suffix was not routed")
		}
		_, _ = m.Update(load())
		setBranchWorkflowValue(t, m, "upstream", "origin/main")
		values := m.workflowValues()
		if err := validateWorkflow(m.workflow.dialog, values); err != nil {
			t.Fatal(err)
		}
		review, err := m.workflow.dialog.ReviewPreflight(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		msg := runFetchOperation(t, m, m.submitReviewedWorkflow(values, review))
		if len(msg.records) != 1 || msg.records[0].Args[0] != "branch" {
			t.Fatalf("configuration process records = %#v", msg.records)
		}
		if got := local.git("config", "--get", "branch.main.remote"); got != "origin" {
			t.Fatalf("configured branch remote = %q", got)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		m := New(repo)
		load, _ := m.performWorkflow(WorkflowCommand{ID: keymap.CommandFetchElsewhere, Options: fetchE2ECommand().Options})
		_, _ = m.Update(load())
		_, operation := m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		if m.workflow != nil || m.mode != modeStatus || m.busy {
			t.Fatalf("cancel left workflow state: mode=%v busy=%v", m.mode, m.busy)
		}
		// Cancellation may only schedule a detail refresh; it must not produce a
		// mutating operation.
		if operation != nil {
			if _, ok := operation().(operationMsg); ok {
				t.Fatal("cancel launched a fetch operation")
			}
		}
	})
}
