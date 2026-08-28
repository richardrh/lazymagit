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

func TestInspectShortlogOptionsMapToTypedQuery(t *testing.T) {
	command := WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		inspectOptionID(t, "transient:magit-shortlog:--summary"): {Enabled: true},
		inspectOptionID(t, "transient:magit-shortlog:-w"):        {Value: "80,4,8"},
	}}
	query, err := shortlogQueryFromCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	if !query.Summary || query.WrapWidth != 80 || query.WrapIndent1 != 4 || query.WrapIndent2 != 8 || !query.WrapIndent1Set || !query.WrapIndent2Set {
		t.Fatalf("mapped shortlog query = %+v", query)
	}
	command.Options[inspectOptionID(t, "transient:magit-shortlog:-w")] = OptionValue{Value: "80,4,8,12"}
	if _, err := shortlogQueryFromCommand(command); err == nil {
		t.Fatal("shortlog accepted more than two indents")
	}
}

func TestInspectRefOptionsMapToClosedTypedQuery(t *testing.T) {
	command := WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		inspectOptionID(t, "magit-for-each-ref:--contains"): {Value: "HEAD"},
		inspectOptionID(t, "magit-for-each-ref:--sort"):     {Value: "--sort=-subject"},
	}}
	query, err := refQueryFromCommand(command, "HEAD")
	if err != nil || query.Contains != "HEAD" || query.Sort != gitbackend.RefSortSubjectReverse {
		t.Fatalf("mapped refs query = %+v, %v", query, err)
	}
	command.Options[inspectOptionID(t, "magit-for-each-ref:--sort")] = OptionValue{Value: "%(refname)"}
	if _, err := refQueryFromCommand(command, "HEAD"); err == nil {
		t.Fatal("refs accepted an arbitrary Git sort atom")
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

func TestInspectionMergetoolIsOnlyConnectedInsideItsTerminalSafeTransient(t *testing.T) {
	m := New(nil)
	for _, binding := range keymap.Registry() {
		if binding.UpstreamCommand != "magit-git-mergetool" || binding.Kind != keymap.KindSuffix {
			continue
		}
		_, installed := m.workflowHandlers[binding.Command]
		if binding.Transient == "magit-git-mergetool" && !installed {
			t.Errorf("nested mergetool action is not connected")
		}
		if binding.Transient != "magit-git-mergetool" && installed {
			t.Errorf("external mergetool route %s was connected", binding.Transient)
		}
	}
}

func TestInspectionUnsupportedEmacsBufferAndMutationOperations(t *testing.T) {
	for _, upstream := range []string{
		"transient-save-and-exit", "magit-toggle-buffer-lock", "magit-toggle-margin",
		"magit-diff-toggle-refine-hunk", "magit-ediff-stage", "magit-ediff-resolve-all",
		"magit-wip-log-index", "magit-wip-log-worktree", "magit-log-merged",
	} {
		if inspectSuffixes[upstream] != nil {
			t.Errorf("unsupported operation %s was connected", upstream)
		}
	}
}
