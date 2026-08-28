package ui

import (
	"testing"

	"github.com/richard/lazymagit/internal/keymap"
)

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
