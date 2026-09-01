package git

import (
	"reflect"
	"testing"
)

func TestNotesPruneHelpers(t *testing.T) {
	refs := map[string]string{
		"":                  "refs/notes/commits",
		"review":            "refs/notes/review",
		"refs/notes/review": "refs/notes/review",
	}
	for input, want := range refs {
		if got := canonicalNotesRef(input); got != want {
			t.Errorf("canonicalNotesRef(%q) = %q, want %q", input, got, want)
		}
	}
	if got, want := parseNotesPruneObjects([]byte("deadbeef\n\ncafebabe\n")), []string{"cafebabe", "deadbeef"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNotesPruneObjects() = %#v, want %#v", got, want)
	}
}
