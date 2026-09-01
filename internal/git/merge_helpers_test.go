package git

import (
	"strings"
	"testing"
)

func TestMergeHelpers(t *testing.T) {
	in := MergeArgs{Mode: MergeNoFF, NoCommit: true, Squash: true, Strategy: "ort", StrategyOptions: []string{"ours"}, Signoff: true}
	if err := validateMergeArgs(in); err != nil {
		t.Fatal(err)
	}
	args, err := mergeArgs(in, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{"--no-ff", "--no-commit", "--squash", "--strategy=ort", "--strategy-option=ours", "--signoff", "-- abc123"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("merge args %q omit %q", joined, expected)
		}
	}
	if _, err := mergeArgs(MergeArgs{Mode: MergeMode(255)}, "abc"); err == nil {
		t.Fatal("invalid merge mode accepted")
	}
	for _, preflight := range []MergePreflight{
		{State: MergeState{InProgress: true}},
		{State: MergeState{Conflicts: []string{"file"}}},
		{RequiresDirtyConfirmation: true},
	} {
		if err := validateMergePreflight(preflight, false); err == nil {
			t.Errorf("unsafe preflight accepted: %#v", preflight)
		}
	}
}
