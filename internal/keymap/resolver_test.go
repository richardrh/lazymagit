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

func TestBindingQueries(t *testing.T) {
	status := BindingsFor(SchemeVim, ContextStatus)
	if len(status) == 0 {
		t.Fatal("BindingsFor returned no Vim status bindings")
	}
	for _, b := range status {
		if b.Scheme != SchemeVim || b.Context != ContextStatus {
			t.Fatalf("BindingsFor returned mismatched binding: %+v", b)
		}
	}
	if got := BindingsFor(Scheme("missing"), ContextStatus); len(got) != 0 {
		t.Fatalf("unknown scheme returned %d bindings", len(got))
	}
	transient := BindingsForTransient(SchemeMagit, "magit-commit")
	if len(transient) == 0 {
		t.Fatal("BindingsForTransient returned no commit bindings")
	}
	for _, b := range transient {
		if b.Scheme != SchemeMagit || b.Transient != "magit-commit" {
			t.Fatalf("BindingsForTransient returned mismatched binding: %+v", b)
		}
	}
	for _, scheme := range []Scheme{SchemeVim, SchemeMagit} {
		got := PrimaryBindings(scheme)
		if len(got) != 4 {
			t.Fatalf("PrimaryBindings(%s) returned %d bindings", scheme, len(got))
		}
		for _, b := range got {
			if b.Scheme != scheme || b.Context != ContextStatus || b.Handler != HandlerPrefix {
				t.Fatalf("PrimaryBindings(%s) returned %+v", scheme, b)
			}
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

func TestRegistryValidationHelpers(t *testing.T) {
	base := Binding{Sequence: []string{"x"}, Display: "x", Command: "one", Scheme: SchemeVim, Context: ContextStatus, Handler: HandlerExecute, Availability: AvailabilityAlways}
	if err := validateBinding(base); err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.Availability = AvailabilityNever
	if err := validateBinding(bad); err == nil {
		t.Fatal("validateBinding accepted untyped unavailability")
	}
	bad = base
	bad.UpstreamCommand = "magit-one"
	if err := validateBinding(bad); err == nil {
		t.Fatal("validateBinding accepted missing source")
	}
	if err := validateTokens([]string{"x", "ctrl+TAB"}); err == nil {
		t.Fatal("validateTokens accepted noncanonical token")
	}
	if err := validateTokens([]string{"x"}); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryIdentityHelpers(t *testing.T) {
	transient := Binding{Sequence: []string{"x"}, Display: "x", Command: "one", Scheme: SchemeVim, Context: ContextTransient + ".test", Handler: HandlerExecute, Occurrence: "one"}
	occurrences := map[string]bool{}
	if err := recordOccurrence(transient, occurrences); err != nil {
		t.Fatal(err)
	}
	if err := recordOccurrence(transient, occurrences); err == nil {
		t.Fatal("recordOccurrence accepted duplicate")
	}
	transient.Occurrence = ""
	if err := recordOccurrence(transient, map[string]bool{}); err == nil {
		t.Fatal("recordOccurrence accepted missing identity")
	}
	if err := recordOccurrence(Binding{Context: ContextStatus}, map[string]bool{}); err != nil {
		t.Fatal(err)
	}

	seen := map[string]CommandID{}
	status := transient
	status.Context, status.Occurrence = ContextStatus, ""
	if err := recordSequence(status, seen); err != nil {
		t.Fatal(err)
	}
	duplicate := status
	duplicate.Command = "two"
	if err := recordSequence(duplicate, seen); err == nil {
		t.Fatal("recordSequence accepted duplicate")
	}
	infix := duplicate
	infix.Handler = HandlerInfix
	if err := recordSequence(infix, seen); err != nil {
		t.Fatal(err)
	}
}
