package ui

import (
	"context"
	"errors"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func commitInfixID(t *testing.T, upstream string) keymap.CommandID {
	t.Helper()
	id, ok := commitCommandID(upstream)
	if !ok {
		t.Fatalf("commit infix %q is missing", upstream)
	}
	return id
}

func TestCommitWorkflowRegistersNineExactSuffixes(t *testing.T) {
	m := New(&gitbackend.Repository{})
	for _, spec := range commitWorkflowSpecs {
		id, ok := commitCommandID(spec.upstream)
		if !ok {
			t.Errorf("%s has no registry ID", spec.upstream)
			continue
		}
		if m.workflowHandlers[id] == nil {
			t.Errorf("%s (%s) has no handler", spec.upstream, id)
		}
	}
	if len(commitWorkflowSpecs) != 9 {
		t.Fatalf("registered commit suffix count = %d", len(commitWorkflowSpecs))
	}
	if id, _ := commitCommandID("magit-commit-create"); id != keymap.CommandCommit {
		t.Fatalf("create ID = %q; built-in collision was not safely replaced", id)
	}
}

func TestCommitWorkflowOptionConversion(t *testing.T) {
	values := map[keymap.CommandID]OptionValue{
		commitInfixID(t, "transient:magit-commit:--all"):          {Enabled: true},
		commitInfixID(t, "transient:magit-commit:--allow-empty"):  {Enabled: true},
		commitInfixID(t, "transient:magit-commit:--no-verify"):    {Enabled: true},
		commitInfixID(t, "transient:magit-commit:--reset-author"): {Enabled: true},
		commitInfixID(t, "magit:--author"):                        {Value: "A U Thor <a@example.invalid>"},
		commitInfixID(t, "magit-commit:--date"):                   {Value: "yesterday"},
		commitInfixID(t, "magit:--signoff"):                       {Enabled: true},
		commitInfixID(t, "magit-commit:--reuse-message"):          {Value: "HEAD~1"},
	}
	got, err := commitOptionsFromWorkflow(values)
	if err != nil {
		t.Fatal(err)
	}
	if !got.All || !got.AllowEmpty || !got.NoVerify || !got.ResetAuthor || !got.Signoff || got.Author != "A U Thor <a@example.invalid>" || got.Date != "yesterday" || got.ReuseMessage != "HEAD~1" {
		t.Fatalf("converted options = %+v", got)
	}

	reedit, err := commitOptionsFromWorkflow(map[keymap.CommandID]OptionValue{
		commitInfixID(t, "magit-commit:--reedit-message"): {Value: "HEAD"},
	})
	if err != nil || reedit.ReeditMessage != "HEAD" {
		t.Fatalf("reedit-message conversion = %+v, %v", reedit, err)
	}
}

func TestCommitSigningRequiresDialogConsent(t *testing.T) {
	command := WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		commitInfixID(t, "magit:--gpg-sign"): {Value: "test-key"},
	}}
	options, err := commitOptionsFromWorkflow(command.Options)
	if err != nil || options.SigningKey != "test-key" {
		t.Fatalf("sign conversion: options=%+v err=%v", options, err)
	}
	dialog := WorkflowDialog{
		Fields: []WorkflowField{{Name: commitConsentField, Label: "Consent", Kind: WorkflowBool}},
		Validate: func(values WorkflowValues) error {
			if values[commitConsentField] != "true" {
				return gitbackend.ErrCommitSigningConsentRequired
			}
			return nil
		},
		Submit: func(context.Context, WorkflowValues) error { return nil },
	}
	if err := validateWorkflow(dialog, WorkflowValues{commitConsentField: "false"}); !errors.Is(err, gitbackend.ErrCommitSigningConsentRequired) {
		t.Fatalf("signing refusal = %v", err)
	}
}

func TestCommitWorkflowCancelAndUnsupportedOptionError(t *testing.T) {
	m := New(&gitbackend.Repository{})
	id, _ := commitCommandID("magit-commit-create")
	load, handled := m.performWorkflow(WorkflowCommand{ID: id})
	if !handled || load == nil {
		t.Fatal("create workflow did not start loading")
	}
	_, _ = m.Update(load())
	if m.workflow == nil {
		t.Fatal("create dialog did not open")
	}
	_, _ = m.handleWorkflowKey(keyMsg("esc"))
	if m.workflow != nil || m.mode != modeStatus || m.message != "Workflow cancelled" {
		t.Fatalf("cancel retained state: workflow=%v mode=%v message=%q", m.workflow != nil, m.mode, m.message)
	}

	m.isError = false
	m.message = ""
	cmd, handled := m.performWorkflow(WorkflowCommand{ID: id, Options: map[keymap.CommandID]OptionValue{
		commitInfixID(t, "transient:magit-commit:--verbose"): {Enabled: true},
	}})
	if !handled || cmd != nil || !m.isError || m.workflow != nil {
		t.Fatalf("unsupported preview did not fail safely: handled=%v cmd=%v error=%v", handled, cmd != nil, m.isError)
	}
}
