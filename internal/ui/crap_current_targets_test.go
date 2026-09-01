package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

func TestCurrentCRAPRenderWorkflowField(t *testing.T) {
	tests := []struct {
		name     string
		field    WorkflowField
		selected bool
		want     []string
	}{
		{"text", WorkflowField{Label: "Name", Kind: WorkflowText, Value: "value"}, true, []string{"▶ Name: value█"}},
		{"confirm", WorkflowField{Label: "Confirm", Kind: WorkflowConfirm, Value: "yes"}, true, []string{"▶ Confirm: yes█"}},
		{"bool", WorkflowField{Label: "Flag", Kind: WorkflowBool}, false, []string{"  Flag: no"}},
		{"enum", WorkflowField{Label: "Mode", Kind: WorkflowEnum, Value: "v", Choices: []WorkflowChoice{{Value: "v", Label: "Visible"}}}, true, []string{"▶ Mode: Visible"}},
		{"multiline", WorkflowField{Label: "Body", Kind: WorkflowMultiline, Value: "one\ntwo"}, true, []string{"▶ Body:", "    one", "    two█"}},
		{"search", WorkflowField{Label: "Target", Kind: WorkflowSearch, Search: "topic", AllowCustom: true}, true, []string{"▶ Target: topic█", "    Use revision: topic"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := renderWorkflowField(test.field, test.selected); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("renderWorkflowField() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func validFormatPatchWorkflowValues() WorkflowValues {
	return WorkflowValues{
		"directory": "out", "numbered": "true", "cover": "false", "signoff": "true", "thread": "false",
		"subject": "PATCH", "cover-message": "Summary", "from": "Author <author@example.test>",
		"in-reply-to": "<root@example.test>", "base": "HEAD~1", "reroll": "2", "start": "3",
		"to": "dev@example.test", "cc": "Reviewer <review@example.test>",
	}
}

func TestCurrentCRAPFormatPatchOptionsFromValues(t *testing.T) {
	defaults := gitbackend.FormatPatchOptions{ThreadStyle: "deep", RFC: true}
	got, err := formatPatchOptionsFromValues(defaults, validFormatPatchWorkflowValues())
	if err != nil {
		t.Fatal(err)
	}
	if got.Thread || got.ThreadStyle != "" || !got.CoverLetter || !got.Numbered || !got.Signoff || !got.RFC || got.RerollCount != 2 || got.StartNumber != 3 || len(got.To) != 1 || len(got.Cc) != 1 {
		t.Fatalf("format patch options = %#v", got)
	}

	invalid := map[string]struct{ field, value string }{
		"reroll": {"reroll", "-1"}, "start": {"start", "bad"}, "to": {"to", "bad"}, "cc": {"cc", "bad"},
		"subject": {"subject", "bad\nsubject"}, "cover message": {"cover-message", "bad\x00message"},
		"from": {"from", "bad"}, "message id": {"in-reply-to", "bad"}, "base": {"base", "bad\x00base"},
	}
	for name, test := range invalid {
		t.Run(name, func(t *testing.T) {
			values := validFormatPatchWorkflowValues()
			values[test.field] = test.value
			if _, err := formatPatchOptionsFromValues(defaults, values); err == nil {
				t.Fatalf("invalid %s was accepted", test.field)
			}
		})
	}
}

func TestCurrentCRAPCommitOptionsFromWorkflow(t *testing.T) {
	options := map[keymap.CommandID]OptionValue{
		commitInfixID(t, "transient:magit-commit:--all"):          {Enabled: true},
		commitInfixID(t, "transient:magit-commit:--allow-empty"):  {Enabled: true},
		commitInfixID(t, "transient:magit-commit:--no-verify"):    {Enabled: true},
		commitInfixID(t, "transient:magit-commit:--reset-author"): {Enabled: true},
		commitInfixID(t, "magit:--author"):                        {Value: "A U Thor"},
		commitInfixID(t, "magit-commit:--date"):                   {Value: "yesterday"},
		commitInfixID(t, "magit:--signoff"):                       {Enabled: true},
		commitInfixID(t, "magit-commit:--reuse-message"):          {Value: "HEAD~1"},
		commitInfixID(t, "magit:--gpg-sign"):                      {Enabled: true},
		commitInfixID(t, "magit-commit:--reedit-message"):         {Value: "HEAD"},
		"unknown": {Enabled: true},
	}
	got, err := commitOptionsFromWorkflow(options)
	if err != nil {
		t.Fatal(err)
	}
	if !got.All || !got.AllowEmpty || !got.NoVerify || !got.ResetAuthor || !got.Signoff || !got.Sign || got.SigningKey != "" || got.Author != "A U Thor" || got.Date != "yesterday" || got.ReuseMessage != "HEAD~1" || got.ReeditMessage != "HEAD" {
		t.Fatalf("commit options = %#v", got)
	}
	keyed, err := commitOptionsFromWorkflow(map[keymap.CommandID]OptionValue{commitInfixID(t, "magit:--gpg-sign"): {Value: "key-id"}})
	if err != nil || keyed.Sign || keyed.SigningKey != "key-id" {
		t.Fatalf("keyed signing options = %#v, %v", keyed, err)
	}
	verbose := commitInfixID(t, "transient:magit-commit:--verbose")
	if _, err := commitOptionsFromWorkflow(map[keymap.CommandID]OptionValue{verbose: {Enabled: true}}); err == nil {
		t.Fatal("verbose preview option was accepted")
	}
	if _, err := commitOptionsFromWorkflow(map[keymap.CommandID]OptionValue{verbose: {}}); err != nil {
		t.Fatalf("inactive verbose option was not ignored: %v", err)
	}
}

func stashInfixID(t *testing.T, transient, upstream string) keymap.CommandID {
	t.Helper()
	for _, binding := range keymap.Registry() {
		if binding.Transient == transient && binding.UpstreamCommand == upstream && binding.Kind == keymap.KindInfix {
			return binding.Command
		}
	}
	t.Fatalf("missing stash option %s/%s", transient, upstream)
	return ""
}

func TestCurrentCRAPStashPushOptions(t *testing.T) {
	parentInclude := stashInfixID(t, "magit-stash", "transient:magit-stash:--include-untracked")
	childInclude := stashInfixID(t, "magit-stash-push", "transient:magit-stash-push:--include-untracked")
	noKeep := stashInfixID(t, "magit-stash-push", "transient:magit-stash-push:--no-keep-index")
	paths := stashInfixID(t, "magit-stash-push", "magit:--")

	got, err := stashPushOptions(WorkflowCommand{ID: stashBothID, Options: map[keymap.CommandID]OptionValue{
		parentInclude: {Enabled: true}, childInclude: {Enabled: true},
	}}, false)
	if err != nil || !got.IncludeUntracked {
		t.Fatalf("parent stash options = %#v, %v", got, err)
	}
	got, err = stashPushOptions(WorkflowCommand{ID: stashPushID, Options: map[keymap.CommandID]OptionValue{
		parentInclude: {Enabled: true}, noKeep: {Enabled: true},
	}}, false)
	if err != nil || got.IncludeUntracked || got.KeepIndex {
		t.Fatalf("child stash options = %#v, %v", got, err)
	}
	if _, err := stashPushOptions(WorkflowCommand{ID: stashPushID, Options: map[keymap.CommandID]OptionValue{noKeep: {Enabled: true}}}, true); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("forced keep-index conflict = %v", err)
	}
	if _, err := stashPushOptions(WorkflowCommand{ID: stashPushID, Options: map[keymap.CommandID]OptionValue{paths: {Value: "bad\x00path"}}}, false); err == nil {
		t.Fatal("invalid stash path was accepted")
	}
}

func TestCurrentCRAPValidateWorkflow(t *testing.T) {
	submit := func(context.Context, WorkflowValues) error { return nil }
	review := func(context.Context, WorkflowValues) (WorkflowReview, error) { return WorkflowReview{}, nil }
	submitReview := func(context.Context, WorkflowValues, WorkflowReview) error { return nil }

	if err := validateWorkflow(WorkflowDialog{Fields: []WorkflowField{{Name: "name", Label: "Name", Required: true}}, Submit: submit}, WorkflowValues{"name": "  "}); err == nil || err.Error() != "Name is required" {
		t.Fatalf("required field validation = %v", err)
	}
	runCallback := func(WorkflowValues) tea.Cmd { return nil }
	cases := map[string]struct {
		dialog  WorkflowDialog
		wantErr bool
	}{
		"run":               {WorkflowDialog{Run: runCallback}, false},
		"run conflict":      {WorkflowDialog{Run: runCallback, Submit: submit}, true},
		"review preflight":  {WorkflowDialog{ReviewPreflight: review}, true},
		"review submit":     {WorkflowDialog{SubmitReview: submitReview}, true},
		"review pair":       {WorkflowDialog{ReviewPreflight: review, SubmitReview: submitReview}, false},
		"missing submit":    {WorkflowDialog{}, true},
		"ordinary submit":   {WorkflowDialog{Submit: submit}, false},
		"custom validation": {WorkflowDialog{Submit: submit, Validate: func(WorkflowValues) error { return errors.New("custom") }}, true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateWorkflow(test.dialog, WorkflowValues{})
			if (err != nil) != test.wantErr {
				t.Fatalf("validateWorkflow() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestCurrentCRAPMergePlan(t *testing.T) {
	preflight := gitbackend.MergePreflight{Target: "topic", TargetOID: "abc", AlreadyUpToDate: true, FastForwardPossible: true, State: gitbackend.MergeState{Dirty: true}}
	args := gitbackend.MergeArgs{Mode: gitbackend.MergeNoFF, NoCommit: true, Squash: true, Strategy: "ort", StrategyOptions: []string{"ours", "renormalize"}, Signoff: true}
	want := []string{
		"target: topic", "resolved target: abc", "Already up to date", "Fast-forward is possible",
		"Create a merge commit even when fast-forward is possible", "Stop before creating the merge commit",
		"Squash changes without creating a merge commit", "Strategy: ort", "Strategy option: ours",
		"Strategy option: renormalize", "Add Signed-off-by trailer", "WARNING: merge into a dirty worktree",
	}
	if got := mergePlan(preflight, args); !reflect.DeepEqual(got, want) {
		t.Fatalf("mergePlan() = %#v, want %#v", got, want)
	}
	if got := mergePlan(gitbackend.MergePreflight{}, gitbackend.MergeArgs{Mode: gitbackend.MergeFFOnly}); !reflect.DeepEqual(got, []string{"target: ", "resolved target: ", "Refuse unless fast-forward is possible"}) {
		t.Fatalf("ff-only merge plan = %#v", got)
	}
}

func TestCurrentCRAPSelectedFileContextVerify(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo, err := gitbackend.Init(ctx, root)
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := selectedFileContext{path: "untracked.txt", kind: rowUntracked}
	if err := untracked.verify(ctx, repo, false); err != nil {
		t.Fatalf("verify untracked file: %v", err)
	}
	if err := untracked.verify(ctx, repo, true); err == nil || !strings.Contains(err.Error(), "tracked") {
		t.Fatalf("tracked-only untracked verification = %v", err)
	}

	trackedPath := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(trackedPath, []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Stage(ctx, []string{"tracked.txt"}); err != nil {
		t.Fatalf("stage tracked file: %v", err)
	}
	if err := os.WriteFile(trackedPath, []byte("unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []rowKind{rowStaged, rowUnstaged} {
		selected := selectedFileContext{path: "tracked.txt", kind: kind}
		if err := selected.verify(ctx, repo, true); err != nil {
			t.Errorf("verify kind %v: %v", kind, err)
		}
	}
	for _, selected := range []selectedFileContext{{path: "missing.txt", kind: rowStaged}, {path: "tracked.txt", kind: rowUntracked}, {path: "tracked.txt", kind: rowKind(255)}} {
		if err := selected.verify(ctx, repo, false); err == nil || !strings.Contains(err.Error(), "stale") {
			t.Errorf("stale verification for %+v = %v", selected, err)
		}
	}

	removedRoot := t.TempDir()
	removedRepo, err := gitbackend.Init(ctx, removedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(removedRoot); err != nil {
		t.Fatal(err)
	}
	removed := selectedFileContext{path: "anything", kind: rowUntracked}
	if err := removed.verify(ctx, removedRepo, false); err == nil {
		t.Fatal("repository status error was not returned")
	}
}

func TestCurrentCRAPFocusedPatchCoordinates(t *testing.T) {
	m := New(nil)
	m.detail = strings.Join([]string{
		"diff --git a/a b/a", "@@ -1,2 +1,2 @@", " one", "-old", "+new", "\\ No newline at end of file",
		"@@ -4 +4 @@", "-two", "+three",
	}, "\n")
	m.detailHunk, m.detailRangeStart, m.detailRangeEnd = 6, -1, -1
	if hunk, start, end, ok := m.focusedPatchCoordinates(); !ok || hunk != 1 || start != 0 || end != 0 {
		t.Fatalf("whole hunk coordinates = %d %d %d %v", hunk, start, end, ok)
	}
	m.detailRangeStart, m.detailRangeEnd = 8, 7
	if hunk, start, end, ok := m.focusedPatchCoordinates(); !ok || hunk != 1 || start != 0 || end != 2 {
		t.Fatalf("range coordinates = %d %d %d %v", hunk, start, end, ok)
	}
	m.detailRangeStart, m.detailRangeEnd = 0, 0
	if _, _, _, ok := m.focusedPatchCoordinates(); ok {
		t.Fatal("non-patch range unexpectedly produced coordinates")
	}
	m.detailHunk, m.detailRangeStart, m.detailRangeEnd = 0, -1, -1
	if _, _, _, ok := m.focusedPatchCoordinates(); ok {
		t.Fatal("focus before the first hunk was accepted")
	}
}
