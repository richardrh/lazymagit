package keymap

import (
	"reflect"
	"testing"
)

func TestTransientEntryBehaviorHelpers(t *testing.T) {
	tr := manifestTransient{Name: "magit-test"}
	entry := manifestEntry{Command: "--flag", Kind: string(KindInfix)}
	want := transientBehavior{command: domainCommandID(tr.Name, entry.Command), handler: HandlerInfix, availability: AvailabilityNever, parity: ParityMissing, category: UnavailableInfix, reason: "infix: argument editing is not implemented"}
	if got := transientEntryBehavior(tr, entry, nil, nil, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("transientEntryBehavior() = %+v, want %+v", got, want)
	}

	entry = manifestEntry{Command: "magit-missing", Kind: string(KindSuffix)}
	want = transientSuffixBehavior(tr.Name, entry.Command, domainCommandID(tr.Name, entry.Command), nil, nil, nil)
	if got := transientEntryBehavior(tr, entry, nil, nil, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("transientEntryBehavior() suffix = %+v, want %+v", got, want)
	}
}

func TestTransientSuffixBehaviorHelper(t *testing.T) {
	missingID := CommandID("test.missing")
	tests := []struct {
		name           string
		owner, command string
		id             CommandID
		names, orphans map[string]bool
		implemented    map[string]CommandID
		want           transientBehavior
	}{
		{name: "child", owner: "magit-parent", command: "magit-child", id: missingID, names: map[string]bool{"magit-child": true}, want: transientBehavior{command: transientCommandID("magit-child"), child: "magit-child", handler: HandlerPrefix, availability: AvailabilityAlways, parity: ParityPartial}},
		{name: "implemented", owner: "magit-test", command: "magit-run", id: missingID, implemented: map[string]CommandID{"magit-test\x00magit-run": CommandCommit}, want: transientBehavior{command: CommandCommit, handler: HandlerExecute, availability: AvailabilityAlways, parity: ParityPartial}},
		{name: "capable", owner: "magit-commit", command: "magit-commit-amend", id: missingID, want: capableTransientBehavior("magit-commit", "magit-commit-amend", missingID)},
		{name: "unsupported spinoff", owner: "magit-branch", command: "magit-branch-spinoff", id: missingID, want: transientBehavior{command: missingID, handler: HandlerUnsupported, availability: AvailabilityNever, parity: ParityMissing, category: UnavailableUnsupported, reason: "backend explicitly reports this branch workflow as unsupported"}},
		{name: "unsupported spinout", owner: "magit-branch", command: "magit-branch-spinout", id: missingID, want: transientBehavior{command: missingID, handler: HandlerUnsupported, availability: AvailabilityNever, parity: ParityMissing, category: UnavailableUnsupported, reason: "backend explicitly reports this branch workflow as unsupported"}},
		{name: "missing", owner: "magit-test", command: "magit-missing", id: missingID, want: transientBehavior{command: missingID, handler: HandlerUnsupported, availability: AvailabilityNever, parity: ParityMissing, category: UnavailableMissing, reason: "suffix: not implemented"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := transientSuffixBehavior(test.owner, test.command, test.id, test.names, test.orphans, test.implemented)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("transientSuffixBehavior() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestCapableTransientBehaviorHelper(t *testing.T) {
	for _, test := range []struct {
		owner, command string
		want           Parity
	}{
		{owner: "magit-commit", command: "magit-commit-amend", want: ParityPartial},
		{owner: "magit-git-mergetool", command: "magit-git-mergetool", want: ParityAdapted},
	} {
		got := capableTransientBehavior(test.owner, test.command, CommandCommit)
		if got.command != CommandCommit || got.handler != HandlerExecute || got.availability != AvailabilityAlways || got.parity != test.want {
			t.Errorf("capableTransientBehavior(%q, %q) = %+v", test.owner, test.command, got)
		}
	}
}

func TestTransientConditionsHelper(t *testing.T) {
	raw := []manifestCondition{{Type: "if", Expression: "ready"}, {Type: "if-not", Expression: "direct-configure disabled"}}
	conditions, directConfigure := transientConditions(raw)
	want := []string{"if: ready", "if-not: direct-configure disabled"}
	if !reflect.DeepEqual(conditions, want) || !directConfigure {
		t.Fatalf("transientConditions() = %v, %t, want %v, true", conditions, directConfigure, want)
	}
	if conditions, directConfigure := transientConditions(nil); len(conditions) != 0 || directConfigure {
		t.Fatalf("transientConditions(nil) = %v, %t", conditions, directConfigure)
	}
}

func TestConfigureTransientInfixHelper(t *testing.T) {
	base := transientBehavior{parity: ParityMissing, reason: "infix: argument editing is not implemented"}
	tests := []struct {
		name            string
		kind            EntryKind
		prefix, command string
		directConfigure bool
		wantParity      Parity
		wantReason      string
	}{
		{name: "suffix unchanged", kind: KindSuffix, directConfigure: true, wantParity: ParityMissing, wantReason: base.reason},
		{name: "unconnected infix", kind: KindInfix, prefix: "?", command: "--flag", wantParity: ParityMissing, wantReason: base.reason},
		{name: "consumer", kind: KindInfix, prefix: "c", command: "--gpg-sign", wantParity: ParityPartial, wantReason: "availability is resolved from installed TUI consumers"},
		{name: "direct configure", kind: KindInfix, prefix: "?", command: "--flag", directConfigure: true, wantParity: ParityPartial, wantReason: "available in the corresponding Configure dialog"},
		{name: "direct configure wins", kind: KindInfix, prefix: "c", command: "--gpg-sign", directConfigure: true, wantParity: ParityPartial, wantReason: "available in the corresponding Configure dialog"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := configureTransientInfix(base, test.kind, test.prefix, test.command, test.directConfigure)
			if got.parity != test.wantParity || got.reason != test.wantReason {
				t.Fatalf("configureTransientInfix() = %+v", got)
			}
		})
	}
}

func TestTransientScalarHelpers(t *testing.T) {
	if got := transientGroup([]string{"first", "second"}); got != "first" {
		t.Errorf("transientGroup() = %q", got)
	}
	if got := transientGroup(nil); got != "" {
		t.Errorf("transientGroup(nil) = %q", got)
	}
	argument := "value"
	if got := transientArgument(&argument); got != argument {
		t.Errorf("transientArgument() = %q", got)
	}
	if got := transientArgument(nil); got != "" {
		t.Errorf("transientArgument(nil) = %q", got)
	}
}
