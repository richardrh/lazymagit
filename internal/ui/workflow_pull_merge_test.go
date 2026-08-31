package ui

import (
	"reflect"
	"strings"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func TestMergeArgumentHelpers(t *testing.T) {
	selected := map[keymap.CommandID]OptionValue{
		"merge.--ff-only": {Enabled: true},
		"merge.--no-ff":   {Value: "configured"},
		"merge.inactive":  {},
	}
	if !mergeOptionSelected(selected, "--ff-only") || !mergeOptionSelected(selected, "--no-ff") || mergeOptionSelected(selected, "inactive") {
		t.Fatalf("merge option selection was incorrect: %v", selected)
	}

	for name, test := range map[string]struct {
		options map[keymap.CommandID]OptionValue
		mode    gitbackend.MergeMode
		wantErr bool
	}{
		"plain": {},
		"ff-only": {
			options: map[keymap.CommandID]OptionValue{"merge.--ff-only": {Enabled: true}},
			mode:    gitbackend.MergeFFOnly,
		},
		"no-ff": {
			options: map[keymap.CommandID]OptionValue{"merge.--no-ff": {Enabled: true}},
			mode:    gitbackend.MergeNoFF,
		},
		"conflict": {
			options: map[keymap.CommandID]OptionValue{"merge.--ff-only": {Enabled: true}, "merge.--no-ff": {Enabled: true}},
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			mode, err := mergeMode(test.options)
			if (err != nil) != test.wantErr || mode != test.mode {
				t.Fatalf("mergeMode() = %q, %v; want %q, error=%v", mode, err, test.mode, test.wantErr)
			}
		})
	}

	args := gitbackend.MergeArgs{}
	for _, option := range []struct {
		id    keymap.CommandID
		value OptionValue
	}{
		{"merge.--strategy", OptionValue{Value: "ours"}},
		{"merge.--strategy-option", OptionValue{Value: "patience"}},
		{"merge.-Xignore-space-change", OptionValue{Enabled: true}},
		{"merge.-Xignore-all-space", OptionValue{Enabled: true}},
		{"merge.--signoff", OptionValue{Enabled: true}},
		{"merge.ignored", OptionValue{Enabled: true}},
	} {
		if err := applyMergeOption(&args, option.id, option.value); err != nil {
			t.Fatalf("applyMergeOption(%q): %v", option.id, err)
		}
	}
	if args.Strategy != "ours" || !args.Signoff || !reflect.DeepEqual(args.StrategyOptions, []string{"patience", "ignore-space-change", "ignore-all-space"}) {
		t.Fatalf("applied merge args = %#v", args)
	}
	for _, unsupported := range []keymap.CommandID{"merge.diff-algorithm", "merge.gpg-sign"} {
		if err := applyMergeOption(&args, unsupported, OptionValue{Enabled: true}); err == nil || !strings.Contains(err.Error(), string(unsupported)) {
			t.Fatalf("unsupported option %q error = %v", unsupported, err)
		}
	}
}

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
	if _, err := mergeArgs(map[keymap.CommandID]OptionValue{"merge.gpg-sign": {Enabled: true}}); err == nil {
		t.Fatal("merge accepted an unsupported typed-backend option")
	}
}
