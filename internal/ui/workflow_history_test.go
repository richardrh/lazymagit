package ui

import (
	"testing"

	"github.com/richardrh/lazymagit/internal/keymap"
)

func TestHistoryPickOptionsMapSupportedInfixesAndRejectUnsafeEdit(t *testing.T) {
	option := func(upstream string) keymap.CommandID {
		t.Helper()
		for _, binding := range keymap.Registry() {
			if binding.UpstreamCommand == upstream {
				return binding.Command
			}
		}
		t.Fatalf("missing history option %q", upstream)
		return ""
	}
	opts, err := historyPickOptions(WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		option("magit-cherry-pick:--mainline"):     {Value: "2"},
		option("magit-merge:--strategy"):           {Value: "recursive"},
		option("magit:--signoff"):                  {Enabled: true},
		option("transient:magit-cherry-pick:--ff"): {Enabled: true},
		option("transient:magit-cherry-pick:-x"):   {Enabled: true},
		option("transient:magit-revert:--no-edit"): {Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Mainline != 2 || opts.Strategy != "recursive" || !opts.Signoff || !opts.FastForward || !opts.RecordOrigin || !opts.NoEdit {
		t.Fatalf("pick options = %#v", opts)
	}
	if _, err := historyPickOptions(WorkflowCommand{Options: map[keymap.CommandID]OptionValue{option("magit-cherry-pick:--mainline"): {Value: "0"}}}); err == nil {
		t.Fatal("non-positive mainline was accepted")
	}
	if _, err := historyPickOptions(WorkflowCommand{Options: map[keymap.CommandID]OptionValue{option("transient:magit-cherry-pick:--edit"): {Enabled: true}}}); err == nil {
		t.Fatal("editor-backed cherry-pick was accepted")
	}
	if _, err := historyPickOptions(WorkflowCommand{Options: map[keymap.CommandID]OptionValue{"unknown": {Enabled: true}}}); err == nil {
		t.Fatal("unknown history option was accepted")
	}
}

func TestRebaseOptionsMapEverySupportedInfix(t *testing.T) {
	option := func(upstream string) keymap.CommandID {
		t.Helper()
		for _, binding := range keymap.Registry() {
			if binding.UpstreamCommand == upstream {
				return binding.Command
			}
		}
		t.Fatalf("missing rebase option %q", upstream)
		return ""
	}
	options, err := rebaseOptions(WorkflowCommand{Options: map[keymap.CommandID]OptionValue{
		option("transient:magit-rebase:--keep-empty"):     {Enabled: true},
		option("transient:magit-rebase:--rebase-merges="): {Enabled: true},
		option("transient:magit-rebase:--update-refs"):    {Enabled: true},
		option("transient:magit-rebase:--autostash"):      {Enabled: true},
		option("transient:magit-rebase:--force-rebase"):   {Enabled: true},
		option("magit-merge:--strategy"):                  {Value: "recursive"},
		option("magit:--signoff"):                         {Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !options.KeepEmpty || !options.RebaseMerges || !options.UpdateRefs || !options.Autostash || !options.ForceRebase || options.Strategy != "recursive" || !options.Signoff {
		t.Fatalf("rebase options = %#v", options)
	}
	if _, err := rebaseOptions(WorkflowCommand{Options: map[keymap.CommandID]OptionValue{"unknown": {Enabled: true}}}); err == nil {
		t.Fatal("unsupported rebase option was accepted")
	}
}
