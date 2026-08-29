package ui

import (
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func TestPullAndMergeOptionMapping(t *testing.T) {
	pull, err := pullArgs(map[keymap.CommandID]OptionValue{
		"pull.--rebase":    {Enabled: true},
		"pull.--autostash": {Enabled: true},
		"pull.--force":     {Enabled: true},
	})
	if err != nil || pull.Mode != gitbackend.PullRebase || !pull.Autostash || !pull.Force {
		t.Fatalf("pull args = %#v, %v", pull, err)
	}
	if _, err := pullArgs(map[keymap.CommandID]OptionValue{"pull.--rebase": {Enabled: true}, "pull.--ff-only": {Enabled: true}}); err == nil {
		t.Fatal("pull accepted mutually exclusive modes")
	}
	merge, err := mergeArgs(map[keymap.CommandID]OptionValue{"merge.--no-ff": {Enabled: true}})
	if err != nil || merge.Mode != gitbackend.MergeNoFF {
		t.Fatalf("merge args = %#v, %v", merge, err)
	}
	merge, err = mergeArgs(map[keymap.CommandID]OptionValue{"merge.--strategy": {Value: "ours"}, "merge.--strategy-option": {Value: "ignore-space-change"}, "merge.--signoff": {Enabled: true}})
	if err != nil || merge.Strategy != "ours" || len(merge.StrategyOptions) != 1 || !merge.Signoff {
		t.Fatalf("merge advanced args = %#v, %v", merge, err)
	}
}
