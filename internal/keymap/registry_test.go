package keymap

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPinnedManifestAndRegistryCounts(t *testing.T) {
	var m struct {
		SchemaVersion string `json:"schema_version"`
		Upstream      struct {
			Version, Commit, Tag string
			CheckoutClean        bool `json:"checkout_clean"`
		}
		Scope struct {
			Mode     string
			MapChain []string `json:"map_chain"`
		}
		Top        []struct{ Effective bool }            `json:"top_level_bindings"`
		Transients []struct{ Entries []json.RawMessage } `json:"transients"`
	}
	if err := json.Unmarshal(upstreamManifest, &m); err != nil {
		t.Fatal(err)
	}
	effective, occurrences := 0, 0
	for _, top := range m.Top {
		if top.Effective {
			effective++
		}
	}
	for _, transient := range m.Transients {
		occurrences += len(transient.Entries)
	}
	wantChain := []string{"magit-status-mode-map", "magit-mode-map", "magit-section-mode-map"}
	if m.SchemaVersion != "1.0.0" || m.Upstream.Version != "4.7.0" || m.Upstream.Tag != "v4.7.0" || m.Upstream.Commit != "67f203853e74e926e2c99f60ed508840714f7ced" || !m.Upstream.CheckoutClean || m.Scope.Mode != "magit-status-mode" || !reflect.DeepEqual(m.Scope.MapChain, wantChain) || effective != 98 || len(m.Transients) != 44 || occurrences != 554 {
		t.Fatalf("manifest commit/counts = %s/%d/%d/%d", m.Upstream.Commit, effective, len(m.Transients), occurrences)
	}
	registered := 0
	for _, binding := range Registry() {
		if binding.EffectiveTop {
			registered++
		}
	}
	if registered != 98 {
		t.Fatalf("effective registry entries = %d", registered)
	}
}

func TestKeybindingDocumentationGolden(t *testing.T) {
	want, err := RenderLedger()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/keybindings.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("docs/keybindings.md drifted; run go run ./internal/keymap/cmd/keymapdoc")
	}
}

func TestEffectiveTopLevelProvenanceIdentity(t *testing.T) {
	var m manifest
	if err := json.Unmarshal(upstreamManifest, &m); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Binding{}
	for _, b := range Registry() {
		if b.EffectiveTop {
			byKey[b.UpstreamKey] = b
		}
	}
	for _, row := range m.Top {
		if !row.Effective {
			continue
		}
		b, ok := byKey[row.Key]
		if !ok || b.UpstreamCommand != row.Command || b.Kind != EntryKind(row.Kind) || b.Domain != row.Domain || b.Layer != row.Layer || b.Source != row.Source {
			t.Errorf("top-level provenance drift for %q: %+v", row.Key, b)
		}
	}
}

func TestTransientCatalogIsExactCompleteOccurrenceMultiset(t *testing.T) {
	var m manifest
	if err := json.Unmarshal(upstreamManifest, &m); err != nil {
		t.Fatal(err)
	}
	type identity struct {
		transient, key, command string
		kind                    EntryKind
	}
	var want []identity
	for _, tr := range m.Transients {
		for _, row := range tr.Entries {
			want = append(want, identity{tr.Name, row.Key, row.Command, EntryKind(row.Kind)})
		}
	}
	var got []identity
	for _, b := range Registry() {
		if b.Scheme != SchemeMagit || !strings.HasPrefix(b.Context, ContextTransient) {
			continue
		}
		got = append(got, identity{b.Transient, b.UpstreamKey, b.UpstreamCommand, b.Kind})
	}
	if len(got) != 554 || !reflect.DeepEqual(got, want) {
		t.Fatalf("displayed transient identities drifted\ngot: %#v\nwant: %#v", got, want)
	}
	for _, b := range Registry() {
		if strings.HasPrefix(b.Context, ContextTransient) && b.Handler == HandlerInfix && (b.Kind != KindInfix || b.Availability != AvailabilityNever || b.UnavailableCategory != UnavailableInfix) {
			t.Errorf("infix is presented as executable: %+v", b)
		}
	}
}

func TestOccurrencesHaveStableUniqueDomainIdentity(t *testing.T) {
	seen := map[string]bool{}
	count := 0
	for _, b := range Registry() {
		if b.Scheme != SchemeMagit || !strings.HasPrefix(b.Context, ContextTransient) {
			continue
		}
		count++
		if b.Occurrence == "" || seen[b.Occurrence] {
			t.Fatalf("invalid occurrence identity %q", b.Occurrence)
		}
		seen[b.Occurrence] = true
		if b.Kind == KindSuffix && strings.HasPrefix(string(b.Command), "missing/") {
			t.Fatalf("suffix %s has placeholder behavior identity %s", b.Occurrence, b.Command)
		}
		if (b.UpstreamCommand == "magit-branch-spinoff" || b.UpstreamCommand == "magit-branch-spinout") && b.UnavailableCategory != UnavailableUnsupported {
			t.Fatalf("%s is not typed backend-unsupported: %+v", b.UpstreamCommand, b)
		}
	}
	if count != 554 {
		t.Fatalf("checked %d transient occurrences", count)
	}
}

func TestSelfNamedSuffixesAreTerminalOrUnavailable(t *testing.T) {
	for _, binding := range Registry() {
		if binding.Scheme != SchemeMagit || binding.Kind != KindSuffix || binding.Transient != binding.UpstreamCommand {
			continue
		}
		if binding.Handler == HandlerPrefix || binding.ChildTransient != "" {
			t.Errorf("self-named suffix forms a recursive edge: %+v", binding)
		}
		if !transientCapability[binding.Transient][binding.UpstreamCommand] && (binding.Handler != HandlerUnsupported || binding.Availability != AvailabilityNever) {
			t.Errorf("unsupported self-named suffix is actionable: %+v", binding)
		}
	}
}

func TestEveryAvailableCatalogSequenceResolvesToItsRegisteredCommand(t *testing.T) {
	for _, scheme := range []Scheme{SchemeVim, SchemeMagit} {
		ctx := Context{View: ViewStatus, Section: SectionUnstaged, Scheme: scheme}
		for _, binding := range Registry() {
			if binding.Scheme != scheme || binding.Handler != HandlerExecute {
				continue
			}
			available, _ := binding.Available(ctx)
			if !available && binding.Availability == AvailabilityStaged {
				ctx.Section = SectionStaged
				available, _ = binding.Available(ctx)
			}
			if !available {
				continue
			}
			r := NewResolver()
			var got Result
			for _, token := range binding.Sequence {
				got = r.Feed(ctx, token)
			}
			if got.Pending && got.Binding != nil && got.Binding.Command == binding.Command {
				got = r.Flush(ctx)
			}
			if got.Command != binding.Command {
				t.Errorf("%s/%s %v resolves %s, want %s", scheme, binding.Context, binding.Sequence, got.Command, binding.Command)
			}
			ctx.Section = SectionUnstaged
		}
	}
}

func TestEveryEffectiveMagitTopLevelSequenceIsRecognized(t *testing.T) {
	ctx := Context{View: ViewStatus, Scheme: SchemeMagit}
	count := 0
	for _, binding := range Registry() {
		if !binding.EffectiveTop {
			continue
		}
		count++
		r := NewResolver()
		var got Result
		for _, token := range binding.Sequence {
			got = r.Feed(ctx, token)
		}
		if !got.Handled || got.Binding == nil || got.Binding.UpstreamKey != binding.UpstreamKey {
			t.Errorf("%q (%v) disappeared or resolved elsewhere: %+v", binding.UpstreamKey, binding.Sequence, got)
		}
	}
	if count != 98 {
		t.Fatalf("tested %d effective keys", count)
	}
}
