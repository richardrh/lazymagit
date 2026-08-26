package keymap

import "testing"

var statusContext = Context{View: ViewStatus, Scheme: SchemeVim}

func TestResolverUsesGenericSequences(t *testing.T) {
	tests := []struct {
		keys []string
		want CommandID
	}{
		{[]string{"j"}, CommandMoveDown}, {[]string{"k"}, CommandMoveUp}, {[]string{"g", "g"}, CommandFirst}, {[]string{"G"}, CommandLast},
		{[]string{"c", "c"}, CommandCommit}, {[]string{"b", "b"}, CommandSwitchBranch}, {[]string{"f", "u"}, CommandFetchUpstream},
		{[]string{"f", "p"}, CommandFetchPush}, {[]string{"f", "e"}, CommandFetchElsewhere}, {[]string{"f", "a"}, CommandFetchAll},
		{[]string{"P", "p"}, CommandPush}, {[]string{"M", "a"}, CommandAddRemote},
	}
	for _, tt := range tests {
		r := NewResolver()
		var got Result
		for i, key := range tt.keys {
			got = r.Feed(statusContext, key)
			if i < len(tt.keys)-1 && !got.Pending {
				t.Fatalf("%v did not pend: %+v", tt.keys, got)
			}
		}
		if got.Command != tt.want || !got.Handled || got.Pending {
			t.Errorf("%v = %+v, want %s", tt.keys, got, tt.want)
		}
	}
}

func TestAmbiguousVimGFlushesToRefresh(t *testing.T) {
	r := NewResolver()
	if !r.Feed(statusContext, "g").Pending {
		t.Fatal("g not pending")
	}
	if got := r.Flush(statusContext); got.Command != CommandRefresh {
		t.Fatalf("flush = %+v", got)
	}
}

func TestContextualAvailabilityIsHandledWithReason(t *testing.T) {
	for _, tc := range []struct {
		key     string
		section Section
		want    CommandID
	}{{"s", SectionUnstaged, CommandStage}, {"u", SectionStaged, CommandUnstage}, {"x", SectionUnstaged, CommandDiscard}} {
		ctx := statusContext
		ctx.Section = tc.section
		if got := NewResolver().Feed(ctx, tc.key); got.Command != tc.want {
			t.Errorf("%s = %+v", tc.key, got)
		}
	}
	ctx := statusContext
	ctx.Section = SectionStaged
	got := NewResolver().Feed(ctx, "s")
	if !got.Handled || got.Command != CommandNone || got.Reason == "" {
		t.Fatalf("unavailable stage = %+v", got)
	}
}

func TestDomainConnectedSuffixAndDirectTopAreRecognized(t *testing.T) {
	r := NewResolver()
	r.Feed(statusContext, "b")
	got := r.Feed(statusContext, "c")
	if !got.Handled || got.Command != "branch.branch-and-checkout" || got.Binding == nil || got.Binding.UpstreamCommand != "magit-branch-and-checkout" {
		t.Fatalf("b c = %+v", got)
	}
	ctx := statusContext
	ctx.Scheme = SchemeMagit
	got = NewResolver().Feed(ctx, "x")
	if !got.Handled || got.Binding == nil || got.Binding.UpstreamCommand != "magit-reset-quickly" {
		t.Fatalf("Magit x = %+v", got)
	}
}

func TestArbitraryLengthTrie(t *testing.T) {
	original := registry
	registry = append(Registry(), Binding{Sequence: []string{"ctrl+x", "m", "x"}, Display: "C-x m x", Command: "test.nested", Scheme: SchemeVim, Context: ContextStatus, Handler: HandlerExecute, Availability: AvailabilityAlways})
	defer func() { registry = original }()
	if err := ValidateRegistry(registry); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	if !r.Feed(statusContext, "ctrl+x").Pending || !r.Feed(statusContext, "m").Pending {
		t.Fatal("nested sequence did not remain pending")
	}
	if got := r.Feed(statusContext, "x"); got.Command != "test.nested" {
		t.Fatalf("nested resolution = %+v", got)
	}
}

func TestRegistryValidation(t *testing.T) {
	if err := ValidateRegistry(Registry()); err != nil {
		t.Fatal(err)
	}
	base := Binding{Sequence: []string{"x"}, Display: "x", Command: "one", Scheme: SchemeVim, Context: ContextStatus, Handler: HandlerExecute, Availability: AvailabilityAlways}
	for name, bindings := range map[string][]Binding{
		"duplicate": {base, func() Binding { b := base; b.Command = "two"; return b }()},
		"handler":   {func() Binding { b := base; b.Handler = ""; return b }()},
		"canonical": {func() Binding { b := base; b.Sequence = []string{"Tab"}; return b }()},
	} {
		if ValidateRegistry(bindings) == nil {
			t.Errorf("%s validation passed", name)
		}
	}
}
