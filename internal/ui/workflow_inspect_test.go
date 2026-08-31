package ui

import (
	"context"
	"strings"
	"testing"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
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

func TestShortlogExtractedOptionHelpers(t *testing.T) {
	query := gitbackend.ShortlogQuery{}
	values := []struct {
		upstream string
		value    OptionValue
	}{
		{"transient:magit-shortlog:--numbered", OptionValue{Enabled: true}},
		{"transient:magit-shortlog:--summary", OptionValue{Enabled: true}},
		{"transient:magit-shortlog:--email", OptionValue{Enabled: true}},
		{"transient:magit-shortlog:--group=", OptionValue{Value: "author"}},
		{"transient:magit-shortlog:--format=", OptionValue{Value: "%s"}},
		{"transient:magit-shortlog:-w", OptionValue{Value: "72,,4"}},
	}
	for _, tt := range values {
		if err := applyShortlogOption(&query, tt.upstream, tt.value); err != nil {
			t.Fatalf("%s: %v", tt.upstream, err)
		}
	}
	if !query.Numbered || !query.Summary || !query.Email || query.Group != "author" || query.Format != "%s" || query.WrapWidth != 72 || query.WrapIndent1Set || !query.WrapIndent2Set || query.WrapIndent2 != 4 {
		t.Fatalf("shortlog helper query = %+v", query)
	}
	if err := applyShortlogOption(&query, "unsupported", OptionValue{}); err == nil {
		t.Fatal("unsupported shortlog option accepted")
	}
	for _, value := range []string{"1,2,3,4", "bad", "80,-1", "80,1,-1"} {
		if err := applyShortlogWrap(&query, value); err == nil {
			t.Fatalf("invalid wrap %q accepted", value)
		}
	}
	if err := applyShortlogWrap(&query, ""); err != nil {
		t.Fatal(err)
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

func TestInspectionRegistersPortableBlameAndGraphInBothSchemes(t *testing.T) {
	m := New(nil)
	for _, scheme := range []keymap.Scheme{keymap.SchemeVim, keymap.SchemeMagit} {
		for key, command := range map[string]keymap.CommandID{"ctrl+b": keymap.CommandBlame, "ctrl+g": keymap.CommandGraph} {
			binding, ok := keymap.Find(scheme, keymap.ContextStatus, key)
			if !ok || binding.Command != command || binding.Handler != keymap.HandlerExecute {
				t.Fatalf("%s %s binding = %#v, found=%t", scheme, key, binding, ok)
			}
			if m.workflowHandlers[command] == nil {
				t.Fatalf("portable %s binding has no workflow handler", key)
			}
		}
	}
}

func TestBlameSelectionHelpers(t *testing.T) {
	entries := map[int]gitbackend.BlameLine{
		4: {Line: 2, CommitID: "2222222222222222222222222222222222222222"},
		3: {Line: 1, CommitID: "1111111111111111111111111111111111111111"},
	}
	if got := firstBlameLine(entries); got != 3 {
		t.Fatalf("first blame line = %d", got)
	}
	m := &Model{blameActive: true, blameCursor: 3, blameEntries: entries, appCtx: context.Background()}
	if _, handled := m.handleBlameKey("down"); !handled || m.blameCursor != 4 {
		t.Fatalf("blame down handled=%t cursor=%d", handled, m.blameCursor)
	}
	if got := activeInspectionRevision(m); got != entries[4].CommitID {
		t.Fatalf("active blame revision = %q", got)
	}
}

func TestUncommittedBlameLineDoesNotOpenRevision(t *testing.T) {
	m := &Model{blameActive: true, blameCursor: 2, blameEntries: map[int]gitbackend.BlameLine{2: {Line: 1, CommitID: strings.Repeat("0", 40)}}}
	if cmd := m.openSelectedBlameCommit(); cmd != nil {
		t.Fatal("uncommitted blame line returned a revision command")
	}
	if !strings.Contains(m.message, "not been committed") || !m.blameActive {
		t.Fatalf("uncommitted selection message=%q active=%t", m.message, m.blameActive)
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
