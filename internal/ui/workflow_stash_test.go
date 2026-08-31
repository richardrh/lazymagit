package ui

import (
	"strings"
	"testing"

	"github.com/richardrh/lazymagit/internal/keymap"
)

func TestStashDomainRegistrationAndRecursivePushOccurrence(t *testing.T) {
	handlers := workflowHandlersFor(&Model{})
	capabilities := workflowCapabilitiesFor()
	for _, id := range []keymap.CommandID{stashBothID, stashKeepIndexID, stashPushID, stashApplyID, stashListID, stashBranchID, stashFormatPatchID} {
		if handlers[id] == nil {
			t.Errorf("stash handler %s is not registered", id)
		}
	}

	for _, binding := range keymap.Registry() {
		if binding.Transient == "magit-stash" && binding.UpstreamCommand == "magit-stash-push" {
			if binding.Handler != keymap.HandlerPrefix || handlers[binding.Command] != nil {
				t.Errorf("parent P must only enter its child transient: %+v", binding)
			}
		}
		if binding.Transient == "magit-stash-push" && binding.UpstreamCommand == "magit-stash-push" {
			if binding.Handler != keymap.HandlerExecute || binding.Command != stashPushID || handlers[binding.Command] == nil {
				t.Errorf("terminal child P is not executable: %+v", binding)
			}
		}
		if binding.UpstreamCommand == "magit-stash-show" {
			capability := capabilities[binding.Command]
			if binding.Handler != keymap.HandlerExecute || handlers[binding.Command] == nil || capability.Transient != binding.Transient || capability.UpstreamCommand != binding.UpstreamCommand {
				t.Errorf("existing stash-show handler is not statically classified: %+v", binding)
			}
		}
		if binding.Transient == "magit-stash" && (binding.UpstreamCommand == "magit-stash-pop" || binding.UpstreamCommand == "magit-stash-drop") {
			if binding.Handler != keymap.HandlerUnsupported || binding.Availability != keymap.AvailabilityNever || handlers[binding.Command] != nil {
				t.Errorf("unsafe stash removal suffix is available: %+v", binding)
			}
		}
		if binding.Transient == "magit-stash" && (binding.UpstreamCommand == "magit-stash-index" || binding.UpstreamCommand == "magit-stash-worktree" || binding.UpstreamCommand == "magit-stash-branch-here" || strings.HasPrefix(binding.UpstreamCommand, "magit-snapshot") || binding.UpstreamCommand == "magit-wip-commit") {
			if handlers[binding.Command] != nil || binding.Handler == keymap.HandlerExecute {
				t.Errorf("unsupported stash suffix became executable: %+v", binding)
			}
		}
	}
}

func TestStashSuffixesAndInfixesAreAvailableAtRuntime(t *testing.T) {
	m := New(nil)
	for _, name := range []string{"magit-stash", "magit-stash-push"} {
		catalog, ok := m.transientCatalog(name)
		if !ok {
			t.Fatalf("transient %s is missing", name)
		}
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				supported := name == "magit-stash-push" || map[string]bool{
					"magit-stash-both": true, "magit-stash-keep-index": true, "magit-stash-apply": true,
					"magit-stash-list": true,
					"magit-stash-show": true, "magit-stash-branch": true, "magit-stash-format-patch": true,
				}[entry.UpstreamCommand]
				if supported && !entry.Available {
					t.Errorf("%s %s unavailable: %s", name, entry.UpstreamCommand, entry.Reason)
				}
			}
		}
	}
}

func TestStashInfixMappingAndLiteralPathValidation(t *testing.T) {
	option := func(transient, upstream string, value OptionValue) (keymap.CommandID, OptionValue) {
		t.Helper()
		for _, binding := range keymap.Registry() {
			if binding.Scheme == keymap.SchemeMagit && binding.Transient == transient && binding.UpstreamCommand == upstream {
				return binding.Command, value
			}
		}
		t.Fatalf("option %s in %s not found", upstream, transient)
		return "", OptionValue{}
	}
	values := map[keymap.CommandID]OptionValue{}
	for _, item := range []struct {
		upstream string
		value    OptionValue
	}{
		{"transient:magit-stash-push:--include-untracked", OptionValue{Enabled: true}},
		{"transient:magit-stash-push:--all", OptionValue{Enabled: true}},
		{"transient:magit-stash-push:--keep-index", OptionValue{Enabled: true}},
		{"magit:--", OptionValue{Value: "dir/file one\n-leading"}},
	} {
		id, value := option("magit-stash-push", item.upstream, item.value)
		values[id] = value
	}
	got, err := stashPushOptions(WorkflowCommand{ID: stashPushID, Options: values}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IncludeUntracked || !got.All || !got.KeepIndex || len(got.Paths) != 2 || got.Paths[0] != "dir/file one" || got.Paths[1] != "-leading" {
		t.Fatalf("mapped stash options = %#v", got)
	}

	noKeepID, noKeep := option("magit-stash-push", "transient:magit-stash-push:--no-keep-index", OptionValue{Enabled: true})
	values[noKeepID] = noKeep
	if _, err := stashPushOptions(WorkflowCommand{ID: stashPushID, Options: values}, false); err == nil {
		t.Fatal("conflicting keep-index switches were accepted")
	}
	if _, err := stashPaths("bad\x00path"); err == nil {
		t.Fatal("NUL stash path was accepted")
	}
	if _, err := stashPaths(strings.Repeat("x\n", stashPathCount+1)); err == nil {
		t.Fatal("oversized stash path list was accepted")
	}
}

func TestStashChildPushIgnoresParentOptions(t *testing.T) {
	optionID := func(transient, upstream string) keymap.CommandID {
		t.Helper()
		for _, binding := range keymap.Registry() {
			if binding.Scheme == keymap.SchemeMagit && binding.Transient == transient && binding.UpstreamCommand == upstream {
				return binding.Command
			}
		}
		t.Fatalf("option %s in %s not found", upstream, transient)
		return ""
	}
	parent := optionID("magit-stash", "transient:magit-stash:--include-untracked")
	child := optionID("magit-stash-push", "transient:magit-stash-push:--include-untracked")
	options := map[keymap.CommandID]OptionValue{parent: {Enabled: true}}
	got, err := stashPushOptions(WorkflowCommand{ID: stashPushID, Options: options}, false)
	if err != nil || got.IncludeUntracked {
		t.Fatalf("child inherited parent include-untracked: options=%#v err=%v", got, err)
	}
	options[child] = OptionValue{Enabled: true}
	got, err = stashPushOptions(WorkflowCommand{ID: stashPushID, Options: options}, false)
	if err != nil || !got.IncludeUntracked {
		t.Fatalf("child include-untracked was not consumed: options=%#v err=%v", got, err)
	}
}
