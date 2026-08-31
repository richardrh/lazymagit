package keymap

import (
	"reflect"
	"testing"
)

func TestClassifyTopDirectCases(t *testing.T) {
	tests := []struct {
		name       string
		binding    Binding
		transients map[string]bool
		check      func(t *testing.T, got Binding)
	}{
		{
			name:    "adapted navigation",
			binding: Binding{UpstreamCommand: "magit-section-cycle-diffs", Parity: ParityMissing},
			check: func(t *testing.T, got Binding) {
				if got.Command != CommandCycleDiffs || got.Handler != HandlerExecute || got.Availability != AvailabilityAlways || got.Parity != ParityAdapted {
					t.Fatalf("classifyTop navigation = %+v", got)
				}
			},
		},
		{
			name:       "manifest transient",
			binding:    Binding{UpstreamCommand: "magit-test", Parity: ParityMissing},
			transients: map[string]bool{"magit-test": true},
			check: func(t *testing.T, got Binding) {
				if got.Command != "transient.test" || got.Handler != HandlerPrefix || !got.Primary || got.Parity != ParityPartial {
					t.Fatalf("classifyTop transient = %+v", got)
				}
			},
		},
		{
			name:    "staged action",
			binding: Binding{Sequence: []string{"s"}, Parity: ParityMissing},
			check: func(t *testing.T, got Binding) {
				if got.Command != CommandStage || got.Availability != AvailabilityUnstaged || got.UnavailableCategory != UnavailableContext {
					t.Fatalf("classifyTop stage = %+v", got)
				}
			},
		},
		{
			name:    "key prefix",
			binding: Binding{Sequence: []string{"b"}, Parity: ParityMissing},
			check: func(t *testing.T, got Binding) {
				if got.Command != "transient.branch" || got.Handler != HandlerPrefix || !got.Primary {
					t.Fatalf("classifyTop key prefix = %+v", got)
				}
			},
		},
		{
			name:    "ediff integration",
			binding: Binding{UpstreamCommand: "magit-ediff-dwim", Parity: ParityMissing},
			check: func(t *testing.T, got Binding) {
				if got.Command != "inspection.ediff-dwim" || got.Handler != HandlerExecute || got.Parity != ParityAdapted {
					t.Fatalf("classifyTop ediff = %+v", got)
				}
			},
		},
		{
			name:    "Emacs-only remap",
			binding: Binding{UpstreamCommand: "missing", UpstreamKey: "<remap> foo", Parity: ParityMissing},
			check: func(t *testing.T, got Binding) {
				if got.Parity != ParityNotApplicable || got.UnavailableCategory != UnavailableNotApplicable {
					t.Fatalf("classifyTop remap = %+v", got)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.binding
			classifyTop(&got, test.transients)
			test.check(t, got)
		})
	}
}

func TestTransientRoutesDirectShortestRecursiveRoute(t *testing.T) {
	m := manifest{
		Top: []manifestTop{
			{Key: "p", Command: "magit-parent", Effective: true},
			{Key: "ctrl+x ctrl+c ctrl+d", Command: "magit-child", Effective: true},
			{Key: "ignored", Command: "magit-grandchild", Effective: false},
		},
		Transients: []manifestTransient{
			{Name: "magit-parent", Entries: []manifestEntry{{Key: "c", Command: "magit-child"}, {Key: "s", Command: "magit-parent"}}},
			{Name: "magit-child", Entries: []manifestEntry{{Key: "g", Command: "magit-grandchild"}}},
			{Name: "magit-grandchild"},
		},
	}

	got := transientRoutes(m)
	want := map[string][]string{
		"magit-parent":     {"p"},
		"magit-child":      {"p", "c"},
		"magit-grandchild": {"p", "c", "g"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("transientRoutes() = %#v, want %#v", got, want)
	}
}

func TestTransientRoutesDirectRejectsUnreachableTransient(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("transientRoutes accepted an unreachable transient")
		}
	}()
	transientRoutes(manifest{Transients: []manifestTransient{{Name: "magit-orphan"}}})
}
