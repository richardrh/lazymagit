package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func newMessageWorkflowRenderModel(t *testing.T) *Model {
	t.Helper()
	m := New(nil)
	m.width, m.height, m.loading, m.mode = 120, 30, false, modeWorkflow
	w := &workflowState{dialog: WorkflowDialog{
		Title: "Commit changes",
		Fields: []WorkflowField{
			{Name: commitMessageField, Label: "Message", Kind: WorkflowMultiline, Value: "Subject\n\nBody"},
			{Name: "signoff", Label: "Sign off", Kind: WorkflowBool},
		},
		Message: &MessageWorkflow{Context: "main", Preview: "diff --git a/file b/file\n-old\n+new"},
	}}
	if err := prepareMessageWorkflow(w); err != nil {
		t.Fatal(err)
	}
	m.workflow = w
	return m
}

func TestMessageWorkflowRenderStates(t *testing.T) {
	m := newMessageWorkflowRenderModel(t)
	if got := ansi.Strip(m.renderMessageWorkflow(31, 11)); !strings.Contains(got, "Enlarge terminal") {
		t.Fatalf("small message workflow = %q", got)
	}
	normal := ansi.Strip(m.renderMessageWorkflow(100, 24))
	for _, want := range []string{"Commit changes", "main", "INSERT", "Subject"} {
		if !strings.Contains(normal, want) {
			t.Fatalf("normal message workflow omitted %q:\n%s", want, normal)
		}
	}

	m.workflow.message.showPreview = true
	wide := ansi.Strip(m.renderMessageWorkflow(120, 24))
	if !strings.Contains(wide, "Changes") || !strings.Contains(wide, "diff --git") {
		t.Fatalf("preview message workflow omitted changes:\n%s", wide)
	}

	m.workflow.dialog.Fields[0].Value = strings.Repeat("long subject ", 8)
	m.workflow.message.buffer.SetText(m.workflow.dialog.Fields[0].Value)
	long := ansi.Strip(m.renderMessageWorkflow(120, 24))
	if !strings.Contains(long, "Long subject") {
		t.Fatalf("long subject warning omitted:\n%s", long)
	}

	m.workflow.review = &WorkflowReview{Plan: []string{"Review title", "Review body"}}
	review := ansi.Strip(m.renderMessageWorkflow(120, 24))
	if !strings.Contains(review, "REVIEW") || !strings.Contains(review, "Review") {
		t.Fatalf("review message workflow omitted review state:\n%s", review)
	}
}

func TestMessageWorkflowKeyRoutingStates(t *testing.T) {
	m := newMessageWorkflowRenderModel(t)
	w := m.workflow
	w.busy = true
	if cmd := m.handleMessageWorkflowKey(messageKey("x")); cmd != nil {
		t.Fatalf("busy message key returned command %v", cmd)
	}
	w.busy = false
	if cmd := m.handleMessageWorkflowKey(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})); cmd != nil || !w.message.prefix {
		t.Fatalf("C-c prefix state: prefix=%v cmd=%v", w.message.prefix, cmd)
	}
	if cmd := m.handleMessageWorkflowKey(messageKey("x")); cmd != nil || w.error == "" || w.message.prefix {
		t.Fatalf("invalid prefix state: error=%q prefix=%v cmd=%v", w.error, w.message.prefix, cmd)
	}
	if cmd := m.handleMessageWorkflowKey(messageKey("ctrl+s")); cmd != nil || m.message != "Message draft saved" {
		t.Fatalf("save draft state: message=%q cmd=%v", m.message, cmd)
	}
	if cmd := m.handleMessageWorkflowKey(messageKey("alt+d")); cmd != nil || !w.message.showPreview {
		t.Fatalf("preview toggle state: preview=%v cmd=%v", w.message.showPreview, cmd)
	}
	w.field = 1
	if cmd := m.handleMessageWorkflowKey(messageKey("enter")); cmd == nil {
		t.Fatal("non-message field edit did not return a draft command")
	}
	if !w.dialog.Fields[1].Bool {
		t.Fatal("boolean field was not edited")
	}
	if cmd := m.handleMessageWorkflowKey(messageKey("esc")); cmd != nil || w.field != w.message.index {
		t.Fatalf("escape did not return to message field: field=%d cmd=%v", w.field, cmd)
	}
	w.field = w.message.index
	if cmd := m.handleMessageWorkflowKey(messageKey("i")); cmd != nil {
		t.Fatalf("message insert returned unexpected command before text: %v", cmd)
	}
	if cmd := m.handleMessageWorkflowKey(messageKey("X")); cmd == nil || !strings.Contains(w.message.buffer.Text(), "X") {
		t.Fatalf("message text edit: text=%q cmd=%v", w.message.buffer.Text(), cmd)
	}
	w.review = &WorkflowReview{Plan: []string{"one", "two"}}
	if cmd := m.handleMessageWorkflowKey(messageKey("alt+k")); cmd != nil || w.message.previewOffset != 0 {
		t.Fatalf("review preview scroll clamped incorrectly: offset=%d cmd=%v", w.message.previewOffset, cmd)
	}
	if cmd := m.handleMessageWorkflowKey(messageKey("esc")); cmd != nil || w.review != nil || w.field != w.message.index {
		t.Fatalf("review escape state: review=%v field=%d cmd=%v", w.review != nil, w.field, cmd)
	}
}
