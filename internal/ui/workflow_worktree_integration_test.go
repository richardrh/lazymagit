package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func worktreeE2EText(t *testing.T, m *Model, value string) {
	t.Helper()
	for _, r := range value {
		sendE2EKey(t, m, keyMsg(string(r)))
	}
}

func worktreeE2ETab(t *testing.T, m *Model) {
	t.Helper()
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
}

func worktreeE2EEnter(t *testing.T, m *Model) {
	t.Helper()
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
}

func TestWorktreeBrowserShowsAndFiltersLinkedWorktrees(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	r.git("branch", "listed-topic")
	linked := filepath.Join(t.TempDir(), "listed")
	r.git("worktree", "add", linked, "listed-topic")
	m := newE2EModel(t, r)
	m.scheme = schemeMagit

	sendE2EKey(t, m, keyMsg("Z"))
	sendE2EKey(t, m, keyMsg("g"))
	plain := ansi.Strip(m.renderWorkflowOverlay(120, 24))
	for _, want := range []string{"primary", "listed-topic", "Close"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("worktree browser omitted %q:\n%s", want, plain)
		}
	}
	values := map[string]bool{}
	for _, choice := range m.workflow.dialog.Fields[0].Choices {
		values[choice.Value] = true
	}
	primaryValue, err := filepath.EvalSymlinks(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	linkedValue, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	if !values[primaryValue] || !values[linkedValue] {
		t.Fatalf("worktree browser values = %v", values)
	}
	sendE2EKey(t, m, keyMsg("listed-topic"))
	if got := m.workflow.dialog.Fields[0].Value; got != linkedValue {
		t.Fatalf("worktree filter selected %q, want %q", got, linkedValue)
	}
}

func TestWorktreeKeysListAddBranchDetachedMoveAndCancel(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	m := newE2EModel(t, r)
	root := t.TempDir()

	// Both effective Magit aliases enter the same top-level transient.
	for _, prefix := range []string{"Z", "%"} {
		sendE2EKey(t, m, keyMsg(prefix))
		if m.resolver.ActiveTransient() != "magit-worktree" {
			t.Fatalf("%s did not enter worktree transient: %q", prefix, m.resolver.ActiveTransient())
		}
		sendE2EKey(t, m, keyMsg("g"))
		if m.mode != modeWorkflow || m.workflow == nil || m.workflow.dialog.Title != "Worktrees" {
			t.Fatalf("%s g did not list worktrees", prefix)
		}
		if len(m.workflow.dialog.Fields) != 1 || m.workflow.dialog.Fields[0].Kind != WorkflowSearch {
			t.Fatalf("%s g did not open searchable worktrees: %+v", prefix, m.workflow.dialog.Fields)
		}
		plain := ansi.Strip(m.renderWorkflowOverlay(120, 24))
		for _, want := range []string{"primary", "Close"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%s g omitted %q:\n%s", prefix, want, plain)
			}
		}
		primary, err := filepath.EvalSymlinks(r.dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.workflow.dialog.Fields[0].Choices[0].Value; got != primary {
			t.Fatalf("%s g primary value = %q, want %q", prefix, got, primary)
		}
		sendE2EKey(t, m, keyMsg("esc"))
	}

	// Cancellation is non-mutating.
	sendE2EKey(t, m, keyMsg("Z"))
	sendE2EKey(t, m, keyMsg("b"))
	sendE2EKey(t, m, keyMsg("esc"))
	if got := len(strings.Fields(r.git("worktree", "list", "--porcelain"))); got == 0 {
		t.Fatal("cancel unexpectedly removed the primary worktree")
	}

	detached := filepath.Join(root, "detached")
	sendE2EKey(t, m, keyMsg("Z"))
	sendE2EKey(t, m, keyMsg("b"))
	worktreeE2EText(t, m, detached)
	worktreeE2ETab(t, m) // revision: HEAD
	worktreeE2ETab(t, m) // detached
	sendE2EKey(t, m, keyMsg("space"))
	worktreeE2ETab(t, m) // force
	worktreeE2ETab(t, m) // submit
	worktreeE2EEnter(t, m)
	worktreeE2EEnter(t, m)
	if got := r.git("-C", detached, "branch", "--show-current"); got != "" {
		t.Fatalf("detached worktree branch = %q", got)
	}

	branched := filepath.Join(root, "branched")
	sendE2EKey(t, m, keyMsg("%"))
	sendE2EKey(t, m, keyMsg("c"))
	worktreeE2EText(t, m, branched)
	worktreeE2ETab(t, m)
	worktreeE2EText(t, m, "worktree-topic")
	worktreeE2ETab(t, m) // start: HEAD
	worktreeE2ETab(t, m) // force
	worktreeE2ETab(t, m) // submit
	worktreeE2EEnter(t, m)
	worktreeE2EEnter(t, m)
	if got := r.git("-C", branched, "branch", "--show-current"); got != "worktree-topic" {
		t.Fatalf("new worktree branch = %q", got)
	}

	moved := filepath.Join(root, "moved")
	sendE2EKey(t, m, keyMsg("Z"))
	sendE2EKey(t, m, keyMsg("m"))
	worktreeE2ETab(t, m)
	worktreeE2EText(t, m, moved)
	worktreeE2ETab(t, m)
	worktreeE2ETab(t, m)
	worktreeE2EEnter(t, m)
	worktreeE2EEnter(t, m)
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("key-driven move: %v", err)
	}
	movedChoice, err := filepath.EvalSymlinks(moved)
	if err != nil {
		t.Fatal(err)
	}

	sendE2EKey(t, m, keyMsg("Z"))
	sendE2EKey(t, m, keyMsg("k"))
	for attempts := 0; m.workflow.dialog.Fields[0].Value != movedChoice && attempts < len(m.workflow.dialog.Fields[0].Choices); attempts++ {
		sendE2EKey(t, m, keyMsg("right"))
	}
	if m.workflow.dialog.Fields[0].Value != movedChoice {
		t.Fatalf("moved worktree is not removable: %+v", m.workflow.dialog.Fields[0].Choices)
	}
	worktreeE2ETab(t, m) // force
	worktreeE2ETab(t, m) // submit
	worktreeE2EEnter(t, m)
	worktreeE2EEnter(t, m)
	if _, err := os.Stat(moved); !os.IsNotExist(err) {
		t.Fatalf("key-driven remove left destination: %v", err)
	}
}

func TestWorktreeKeyRemovalRejectsStaleReviewedState(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	linked := filepath.Join(t.TempDir(), "linked")
	r.git("worktree", "add", "--detach", linked, "HEAD")
	m := newE2EModel(t, r)

	sendE2EKey(t, m, keyMsg("Z"))
	sendE2EKey(t, m, keyMsg("k"))
	worktreeE2ETab(t, m) // force
	sendE2EKey(t, m, keyMsg("space"))
	worktreeE2ETab(t, m) // submit
	worktreeE2EEnter(t, m)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatal("remove did not produce an exact reviewed state")
	}
	if err := os.WriteFile(filepath.Join(linked, "stale.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktreeE2EEnter(t, m)
	if !m.isError || !strings.Contains(m.message, "stale") {
		t.Fatalf("stale removal message = %q", m.message)
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatalf("stale removal deleted worktree: %v", err)
	}
}
