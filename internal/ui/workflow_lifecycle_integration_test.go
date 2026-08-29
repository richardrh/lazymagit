package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeLifecycleValue(t *testing.T, m *Model, value string) {
	t.Helper()
	for _, r := range value {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func tabLifecycle(t *testing.T, m *Model, count int) {
	t.Helper()
	for range count {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
}

func TestLifecycleCloneAndInitDialogsAreKeyDrivenAndKeepCurrentRepository(t *testing.T) {
	source := newUIE2ERepo(t)
	source.write("tracked.txt", "content\n")
	source.git("add", "--", "tracked.txt")
	source.git("commit", "-m", "source")
	m := newE2EModel(t, source)
	originalWorktree := m.repo.WorkTree()

	cloneDestination := filepath.Join(t.TempDir(), "clone target")
	// Top-level C enters the exact Magit clone transient; C chooses its regular
	// suffix. All remaining interaction is through dialog key messages.
	sendE2EKey(t, m, keyMsg("C"))
	sendE2EKey(t, m, keyMsg("C"))
	if m.mode != modeWorkflow {
		t.Fatalf("C C did not open clone dialog: mode=%v message=%q", m.mode, m.message)
	}
	typeLifecycleValue(t, m, source.dir)
	tabLifecycle(t, m, 1)
	typeLifecycleValue(t, m, cloneDestination)
	tabLifecycle(t, m, 9)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if _, err := os.Stat(filepath.Join(cloneDestination, ".git")); err != nil {
		t.Fatalf("key-driven clone did not create repository: %v", err)
	}
	if m.repo.WorkTree() != originalWorktree {
		t.Fatalf("clone switched repository model to %q", m.repo.WorkTree())
	}
	if !strings.Contains(m.message, cloneDestination) || !strings.Contains(m.message, "restart lazymagit") {
		t.Fatalf("clone completion did not identify restart path: %q", m.message)
	}

	initDestination := t.TempDir()
	initRepositoryWorkflow(m, WorkflowCommand{})
	typeLifecycleValue(t, m, initDestination)
	tabLifecycle(t, m, 1)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if _, err := os.Stat(filepath.Join(initDestination, ".git")); err != nil {
		t.Fatalf("key-driven init did not create repository: %v", err)
	}
	if m.repo.WorkTree() != originalWorktree {
		t.Fatalf("init switched repository model to %q", m.repo.WorkTree())
	}
	if !strings.Contains(m.message, initDestination) || !strings.Contains(m.message, "restart lazymagit") {
		t.Fatalf("init completion did not identify restart path: %q", m.message)
	}
}

func TestCloneTransientOptionsReachTypedCloneBackend(t *testing.T) {
	source := newUIE2ERepo(t)
	source.write("tracked.txt", "content\n")
	source.git("add", "--", "tracked.txt")
	source.git("commit", "-m", "source")
	source.git("tag", "v1")
	m := newE2EModel(t, source)
	destination := filepath.Join(t.TempDir(), "option clone")

	// C's high-value fetch/setup infixes remain portable terminal sequences.
	sendE2EKey(t, m, keyMsg("C"))
	for _, key := range []string{"-", "B", "-", "n", "-", "S"} {
		sendE2EKey(t, m, keyMsg(key))
	}
	sendE2EKey(t, m, keyMsg("C"))
	if m.workflow == nil {
		t.Fatalf("clone workflow did not open: %q", m.message)
	}
	for _, name := range []string{"recurse", "single", "no-tags"} {
		found := false
		for _, field := range m.workflow.dialog.Fields {
			if field.Name == name {
				found = field.Bool
			}
		}
		if !found {
			t.Fatalf("clone option %s was not carried into the typed dialog", name)
		}
	}
	typeLifecycleValue(t, m, source.dir)
	tabLifecycle(t, m, 1)
	typeLifecycleValue(t, m, destination)
	tabLifecycle(t, m, 9)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	out, err := exec.Command("git", "-C", destination, "config", "--get", "remote.origin.tagOpt").Output()
	if err != nil || strings.TrimSpace(string(out)) != "--no-tags" {
		t.Fatalf("clone did not consume --no-tags: %q (%v)", out, err)
	}
}

func TestLifecycleDialogCancellationAndValidationAreNonMutating(t *testing.T) {
	source := newUIE2ERepo(t)
	m := newE2EModel(t, source)
	destination := filepath.Join(t.TempDir(), "cancelled target")

	sendE2EKey(t, m, keyMsg("C"))
	sendE2EKey(t, m, keyMsg("C"))
	typeLifecycleValue(t, m, source.dir)
	tabLifecycle(t, m, 1)
	typeLifecycleValue(t, m, destination)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("cancelled dialog created destination: %v", err)
	}

	sendE2EKey(t, m, keyMsg("C"))
	sendE2EKey(t, m, keyMsg("C"))
	typeLifecycleValue(t, m, "https://user:secret@example.invalid/repo.git")
	tabLifecycle(t, m, 1)
	typeLifecycleValue(t, m, destination)
	tabLifecycle(t, m, 9)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.workflow == nil || !strings.Contains(m.workflow.error, "credential helper") {
		t.Fatalf("credential-bearing URL validation = workflow %#v", m.workflow)
	}
	if strings.Contains(m.message, "secret") {
		t.Fatalf("credential leaked to status message: %q", m.message)
	}
}
