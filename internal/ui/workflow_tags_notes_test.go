package ui

import (
	"strings"
	"testing"

	"github.com/richard/lazymagit/internal/keymap"
)

func TestTagAndNotesHandlersAreRegistered(t *testing.T) {
	m := New(nil)
	for _, id := range []keymap.CommandID{tagCreateID, tagReleaseID, tagDeleteID, tagPruneID, notesEditID, notesRemoveID, notesMergeID, notesPruneID, notesMergeContinueID, notesMergeAbortID} {
		if m.workflowHandlers[id] == nil {
			t.Errorf("handler %s is not registered", id)
		}
	}
}

func TestSignedTagRequiresExplicitConsent(t *testing.T) {
	m := New(nil)
	tagCreateWorkflow(false)(m, WorkflowCommand{Options: map[keymap.CommandID]OptionValue{tagSignID: {Enabled: true}}})
	if m.workflow == nil {
		t.Fatal("tag workflow did not open")
	}
	values := WorkflowValues{"name": "v1", "target": "HEAD", "kind": "signed", "message": "release", "sign-consent": "false"}
	if err := m.workflow.dialog.Validate(values); err == nil || !strings.Contains(err.Error(), "explicit interactive consent") {
		t.Fatalf("signing refusal = %v", err)
	}
	values["sign-consent"] = "true"
	if err := m.workflow.dialog.Validate(values); err != nil {
		t.Fatalf("explicit signing consent rejected: %v", err)
	}
}
