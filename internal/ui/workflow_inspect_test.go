package ui

import (
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func inspectOptionID(t *testing.T, upstream string) keymap.CommandID {
	t.Helper()
	for _, binding := range keymap.Registry() {
		if binding.UpstreamCommand == upstream {
			return binding.Command
		}
	}
	t.Fatalf("option %q is not exposed by the registry", upstream)
	return keymap.CommandNone
}

func TestInspectLogOptionsMapToTypedBoundedQuery(t *testing.T) {
	command := WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		inspectOptionID(t, "magit-log:-n"):                           {Value: "9000"},
		inspectOptionID(t, "transient:magit-log-refresh:--graph"):    {Enabled: false},
		inspectOptionID(t, "transient:magit-log-refresh:--decorate"): {Enabled: false},
		inspectOptionID(t, "magit-log:--*-order"):                    {Value: "--author-date-order"},
	}}
	query := logQueryFromCommand(command)
	if query.Limit != inspectItemLimit || query.Graph || query.Decorations || query.Order != gitbackend.LogOrderAuthorDate {
		t.Fatalf("mapped log query = %+v", query)
	}
	if query.OutputLimit != inspectOutputLimit {
		t.Fatalf("output limit = %d", query.OutputLimit)
	}
}

func TestInspectionRegistrationCoversSafeExpandedOccurrences(t *testing.T) {
	m := New(nil)
	for _, binding := range keymap.Registry() {
		if inspectTopLevel[binding.UpstreamCommand] == nil && inspectSuffixes[binding.UpstreamCommand] == nil {
			continue
		}
		if _, ok := m.workflowHandlers[binding.Command]; !ok {
			t.Errorf("%s (%s) is not registered", binding.Command, binding.UpstreamCommand)
		}
	}
}

func TestInspectionUnsupportedEmacsBufferAndMutationOperations(t *testing.T) {
	for _, upstream := range []string{
		"transient-save-and-exit", "magit-toggle-buffer-lock", "magit-toggle-margin",
		"magit-diff-toggle-refine-hunk", "magit-ediff-stage", "magit-ediff-resolve-all",
		"magit-git-mergetool", "magit-reflog-current", "magit-shortlog",
	} {
		if inspectSuffixes[upstream] != nil {
			t.Errorf("unsupported operation %s was connected", upstream)
		}
	}
}
