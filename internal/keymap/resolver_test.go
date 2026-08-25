package keymap

import "testing"

var statusContext = Context{View: ViewStatus}

func TestVimNavigationSequences(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want Action
	}{
		{name: "down", keys: []string{"j"}, want: ActionMoveDown},
		{name: "up", keys: []string{"k"}, want: ActionMoveUp},
		{name: "first", keys: []string{"g", "g"}, want: ActionFirst},
		{name: "last", keys: []string{"G"}, want: ActionLast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResolver()
			for i, key := range tt.keys {
				got := r.Feed(statusContext, key)
				if i < len(tt.keys)-1 {
					assertPending(t, got)
					continue
				}
				assertAction(t, got, tt.want)
			}
		})
	}
}

func TestSingleGRefreshesWhenTheAmbiguousPrefixIsFlushed(t *testing.T) {
	r := NewResolver()
	assertPending(t, r.Feed(statusContext, "g"))
	assertAction(t, r.Flush(statusContext), ActionRefresh)
}

func TestStatusSectionBindingsAreContextAware(t *testing.T) {
	tests := []struct {
		name    string
		context Context
		key     string
		want    Action
	}{
		{name: "stage unstaged", context: Context{View: ViewStatus, Section: SectionUnstaged}, key: "s", want: ActionStage},
		{name: "unstage staged", context: Context{View: ViewStatus, Section: SectionStaged}, key: "u", want: ActionUnstage},
		{name: "discard unstaged", context: Context{View: ViewStatus, Section: SectionUnstaged}, key: "x", want: ActionDiscard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertAction(t, NewResolver().Feed(tt.context, tt.key), tt.want)
		})
	}

	wrongSections := []struct {
		context Context
		key     string
	}{
		{context: Context{View: ViewStatus, Section: SectionStaged}, key: "s"},
		{context: Context{View: ViewStatus, Section: SectionUnstaged}, key: "u"},
		{context: Context{View: ViewStatus, Section: SectionStaged}, key: "x"},
	}
	for _, tt := range wrongSections {
		if got := NewResolver().Feed(tt.context, tt.key); got.Handled || got.Action != ActionNone || got.Pending {
			t.Errorf("key %q unexpectedly resolved in section %v: %+v", tt.key, tt.context.Section, got)
		}
	}
}

func TestMagitCommandPrefixes(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want Action
	}{
		{name: "commit", keys: []string{"c", "c"}, want: ActionCommit},
		{name: "switch branch", keys: []string{"b", "b"}, want: ActionSwitchBranch},
		{name: "fetch", keys: []string{"f", "f"}, want: ActionFetch},
		{name: "push", keys: []string{"P", "p"}, want: ActionPush},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResolver()
			assertPending(t, r.Feed(statusContext, tt.keys[0]))
			assertAction(t, r.Feed(statusContext, tt.keys[1]), tt.want)
		})
	}
}

func TestEscapeCancelsAPrefixWithoutExecutingIt(t *testing.T) {
	r := NewResolver()
	assertPending(t, r.Feed(statusContext, "c"))

	got := r.Feed(statusContext, "esc")
	if !got.Handled || got.Pending || got.Action != ActionNone {
		t.Fatalf("escape should cancel with no action, got %+v", got)
	}

	// A following c starts a new prefix; it must not complete the cancelled one.
	assertPending(t, r.Feed(statusContext, "c"))
	assertAction(t, r.Feed(statusContext, "c"), ActionCommit)
}

func assertPending(t *testing.T, got Result) {
	t.Helper()
	if !got.Handled || !got.Pending || got.Action != ActionNone {
		t.Fatalf("result = %+v, want handled pending prefix with no action", got)
	}
}

func assertAction(t *testing.T, got Result, want Action) {
	t.Helper()
	if !got.Handled || got.Pending || got.Action != want {
		t.Fatalf("result = %+v, want completed action %v", got, want)
	}
}
