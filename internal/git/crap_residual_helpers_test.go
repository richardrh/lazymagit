package git

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSummary(t *testing.T) {
	out := []byte(strings.Join([]string{
		"# branch.oid abc123",
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +3 -2",
	}, "\x00") + "\x00")
	got := parseSummary(out)
	want := Summary{Head: "abc123", Branch: "main", Upstream: "origin/main", Ahead: 3, Behind: 2}
	if got != want {
		t.Fatalf("parseSummary() = %#v, want %#v", got, want)
	}

	got = parseSummary([]byte("# branch.oid (initial)\x00# branch.head (detached)\x00"))
	if !got.Unborn || !got.Detached {
		t.Fatalf("special summary flags = %#v", got)
	}
}

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

func TestParseRebaseNumber(t *testing.T) {
	if got, err := parseRebaseNumber(" 12\n"); err != nil || got != 12 {
		t.Fatalf("parseRebaseNumber(valid) = %d, %v", got, err)
	}
	for _, input := range []string{"", "nope", "-1"} {
		if _, err := parseRebaseNumber(input); err == nil {
			t.Errorf("parseRebaseNumber(%q) succeeded", input)
		}
	}
}

func TestValidateBranchConfigUpdate(t *testing.T) {
	valid := BranchConfigUpdate{
		Description: ConfigUpdate{Action: ConfigKeep}, Upstream: ConfigUpdate{Action: ConfigKeep},
		Rebase: ConfigUpdate{Action: ConfigSet, Value: "merges"}, PushRemote: ConfigUpdate{Action: ConfigUnset},
		PullRebase: ConfigUpdate{Action: ConfigSet, Value: "interactive"}, RemotePushDefault: ConfigUpdate{Action: ConfigKeep},
		AutoSetupMerge: ConfigUpdate{Action: ConfigSet, Value: "simple"}, AutoSetupRebase: ConfigUpdate{Action: ConfigSet, Value: "remote"},
	}
	if err := validateBranchConfigUpdate(valid); err != nil {
		t.Fatalf("valid update: %v", err)
	}
	invalid := valid
	invalid.Description.Action = "invalid"
	if err := validateBranchConfigUpdate(invalid); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("invalid action error = %v", err)
	}
	invalid = valid
	invalid.AutoSetupMerge.Value = "invalid"
	if err := validateBranchConfigUpdate(invalid); err == nil || !strings.Contains(err.Error(), "autoSetupMerge") {
		t.Fatalf("invalid mode error = %v", err)
	}
}
