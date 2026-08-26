package keymap

import (
	"reflect"
	"strings"
	"testing"
)

func FuzzResolver(f *testing.F) {
	for _, binding := range Registry() {
		f.Add(string(binding.Scheme), strings.Join(binding.Sequence, "\x00"))
	}
	f.Add("vim", "g\x00g")
	f.Add("magit", "esc")
	f.Fuzz(func(t *testing.T, schemeText, encoded string) {
		if len(schemeText) > 16 || len(encoded) > 4096 {
			t.Skip()
		}
		tokens := strings.Split(encoded, "\x00")
		if encoded == "" {
			tokens = nil
		}
		if len(tokens) > 64 {
			t.Skip()
		}
		for _, token := range tokens {
			if len(token) > 64 {
				t.Skip()
			}
		}
		scheme := Scheme(schemeText)
		ctx := Context{View: ViewStatus, Section: SectionUnstaged, Scheme: scheme}
		first := resolveTokens(ctx, tokens)
		second := resolveTokens(ctx, tokens)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("resolver is not deterministic: %#v != %#v", first, second)
		}
	})
}

func resolveTokens(ctx Context, tokens []string) []Result {
	r := NewResolver()
	results := make([]Result, 0, len(tokens)+1)
	for _, token := range tokens {
		result := r.Feed(ctx, token)
		if result.Pending && result.Prefix == "" {
			panic("pending result has no prefix")
		}
		if result.Pending && r.PendingPrefix() != "" && result.Prefix != r.PendingPrefix() {
			panic("pending result does not describe resolver prefix")
		}
		if !result.Pending && r.PendingPrefix() != "" {
			panic("completed result retained a prefix")
		}
		results = append(results, result)
	}
	results = append(results, r.Flush(ctx))
	if r.PendingPrefix() != "" {
		panic("flush retained a prefix")
	}
	return results
}
