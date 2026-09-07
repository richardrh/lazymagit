package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCommitMessageDraftSurvivesRestartAndHookFailure(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked", "before\n")
	r.git("add", ".")
	r.git("commit", "-m", "initial")
	r.write("tracked", "after\n")
	r.git("add", ".")
	m := newE2EModel(t, r)
	open := func(model *Model) {
		sendE2EKey(t, model, keyMsg("c"))
		sendE2EKey(t, model, keyMsg("c"))
		if model.workflow == nil {
			t.Fatalf("composer did not open: %s", model.message)
		}
	}
	ctrl := func(model *Model, r rune) { sendE2EKey(t, model, tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl})) }
	message := "Preserve message α\n\nDetailed rationale and verification."
	open(m)
	_, _ = m.Update(tea.PasteMsg{Content: message})
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.mode != modeWorkflow || m.workflow.message.buffer.Mode() != "NORMAL" {
		t.Fatal("Escape discarded the draft instead of entering normal mode")
	}
	ctrl(m, 'g')
	m.shutdown()
	m = newE2EModel(t, r)
	defer m.shutdown()
	open(m)
	if got := m.workflowValues()[commitMessageField]; got != message {
		t.Fatalf("restored draft = %q", got)
	}
	hook := filepath.Join(r.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf 'deliberate hook rejection\\n' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	ctrl(m, 'c')
	ctrl(m, 'c')
	if m.workflow == nil || m.workflowValues()[commitMessageField] != message || !strings.Contains(m.workflow.error, "deliberate hook rejection") {
		t.Fatalf("failed commit lost draft or useful failure: %s", m.message)
	}
	if got := r.git("log", "-1", "--format=%s"); got != "initial" {
		t.Fatalf("rejected commit changed HEAD: %s", got)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	ctrl(m, 'c')
	ctrl(m, 'c')
	if got := r.git("log", "-1", "--format=%B"); got != message {
		t.Fatalf("retry committed %q", got)
	}
	open(m)
	if got := m.workflowValues()[commitMessageField]; got != "" {
		t.Fatalf("successful draft was restored again: %q", got)
	}
}

func TestCommitAmendAndRewordPreloadExistingMessage(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked", "text\n")
	r.git("add", ".")
	message := "Existing subject\n\nExisting body."
	r.git("commit", "-m", message)
	m := newE2EModel(t, r)
	defer m.shutdown()
	for _, suffix := range []string{"a", "w"} {
		sendE2EKey(t, m, keyMsg("c"))
		sendE2EKey(t, m, keyMsg(suffix))
		if m.workflow == nil || strings.TrimSpace(m.workflowValues()[commitMessageField]) != message {
			t.Fatalf("c %s did not preload HEAD message: %s", suffix, m.message)
		}
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: 'g', Mod: tea.ModCtrl}))
	}
}
