package keymap

import "testing"

func TestNativeWorktreeBindingsAreExecutableInBothSchemes(t *testing.T) {
	want := map[string]CommandID{
		"L": "worktree.worktree-lock",
		"U": "worktree.worktree-unlock",
		"p": "worktree.worktree-prune",
	}
	for _, scheme := range []Scheme{SchemeMagit, SchemeVim} {
		seen := map[string]bool{}
		for _, binding := range BindingsForTransient(scheme, "magit-worktree") {
			key := displaySequence(binding.LocalSequence)
			command, ok := want[key]
			if !ok {
				continue
			}
			if binding.Command != command || binding.Handler != HandlerExecute || binding.Availability != AvailabilityAlways {
				t.Fatalf("%s %s binding = %+v", scheme, key, binding)
			}
			seen[key] = true
		}
		for key := range want {
			if !seen[key] {
				t.Errorf("%s worktree transient lacks native %s binding", scheme, key)
			}
		}
	}
}
