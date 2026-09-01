package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
	sectionmodel "github.com/richardrh/lazymagit/internal/model"
)

func TestHistoryTransientHandlerRoutesEveryFamily(t *testing.T) {
	tests := []struct{ transient, upstream string }{
		{"magit-cherry-pick", "magit-cherry-copy"}, {"magit-revert", "magit-revert-no-commit"},
		{"magit-rebase", "magit-rebase-interactive"}, {"magit-reset", "magit-reset-hard"},
		{"magit-bisect", "magit-bisect-good"},
	}
	for _, tt := range tests {
		if handler := historyTransientHandler(tt.transient, tt.upstream); handler == nil {
			t.Errorf("%s/%s has no handler", tt.transient, tt.upstream)
		}
	}
	if handler := historyTransientHandler("unknown", "unknown"); handler != nil {
		t.Fatal("unknown history route unexpectedly has a handler")
	}
}

func TestCompactDetailRendererStylesAllLineKinds(t *testing.T) {
	m := New(nil)
	m.detail = strings.Join([]string{"diff --git a/a b/a", "--- a/a", "+++ b/a", "@@ -1 +1 @@", "-old", "+new", "plain"}, "\n")
	m.detailHunk, m.detailRangeStart, m.detailRangeEnd, m.detailLine = 3, 4, 5, 5
	got := m.renderCompactDetail(40, 7)
	if plain := ansi.Strip(got); !strings.Contains(plain, "diff --git") || !strings.Contains(plain, "+new") {
		t.Fatalf("compact detail omitted content: %q", plain)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("compact detail omitted styling")
	}
	m.graphActive, m.graphCursor = true, 6
	_ = m.renderCompactDetail(40, 7)
}

func TestSnapshotLoaderHelpersWrapEachStage(t *testing.T) {
	base := snapshotQueries{
		summary: func(context.Context) (gitbackend.Summary, error) {
			return gitbackend.Summary{Upstream: "origin/main"}, nil
		},
		status:     func(context.Context) (gitbackend.Status, error) { return gitbackend.Status{}, nil },
		stashes:    func(context.Context) ([]gitbackend.Stash, error) { return nil, nil },
		remotes:    func(context.Context) ([]gitbackend.Remote, error) { return nil, nil },
		pushRemote: func(context.Context) (string, error) { return "", gitbackend.ErrNoFetchRemote },
		operations: func(context.Context) (gitbackend.OperationState, error) { return gitbackend.OperationState{}, nil },
		sparse: func(context.Context) (gitbackend.SparseCheckoutState, error) {
			return gitbackend.SparseCheckoutState{}, nil
		},
		recentLog: func(context.Context, int) ([]gitbackend.Commit, error) { return nil, nil },
		upstreamLogLimit: func(context.Context, int) (gitbackend.UpstreamRanges, error) {
			return gitbackend.UpstreamRanges{}, gitbackend.ErrNoUpstream
		},
	}
	if _, err := loadSnapshotWith(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	base.operations = func(context.Context) (gitbackend.OperationState, error) {
		return gitbackend.OperationState{}, errors.New("boom")
	}
	if _, err := loadSnapshotWith(context.Background(), base); err == nil || !strings.Contains(err.Error(), "operation state") {
		t.Fatalf("error = %v", err)
	}
}

func TestTransientAvailabilityAndConditionHelpers(t *testing.T) {
	m := New(nil)
	binding := keymap.Binding{Kind: keymap.KindInfix, Conditions: []string{"direct-configure"}}
	if active, reason := m.bindingCondition(binding); active || !strings.Contains(reason, "Configure") {
		t.Fatalf("condition = %v, %q", active, reason)
	}
	available, reason, category := m.transientAvailability("missing", binding, m.keyContext(), true, "")
	if available || reason == "" || category != menuEntryInfix {
		t.Fatalf("availability = %v %q %q", available, reason, category)
	}
	catalog, ok := m.transientCatalog("magit-branch")
	if !ok || len(catalog.Groups) == 0 {
		t.Fatal("branch catalog was not built")
	}
}

func TestNavigationHelperDispatch(t *testing.T) {
	m := navigationUIModel()
	for _, command := range []keymap.CommandID{keymap.CommandSectionCycle, keymap.CommandSectionCycleGlobal, keymap.CommandLocalDepth1, keymap.CommandGlobalDepth2, keymap.CommandDescribeSection} {
		if _, handled := m.performNavigationCommand(command); !handled {
			t.Errorf("%s was not handled", command)
		}
	}
	if _, handled := m.performNavigationCommand(keymap.CommandNone); handled {
		t.Fatal("unknown navigation command handled")
	}
}

func TestStageTwoSimpleUIResiduals(t *testing.T) {
	options := map[keymap.CommandID]OptionValue{
		"b": {Value: "value"}, "a": {Enabled: true}, "ignored": {},
	}
	if got := optionSummary(options); got != "a, b" {
		t.Fatalf("optionSummary = %q", got)
	}

	lines := renderWorkflowMultilineField(WorkflowField{Label: "Message", Value: "one\ntwo"}, "▶ ", true)
	if !reflect.DeepEqual(lines, []string{"▶ Message:", "    one", "    two█"}) {
		t.Fatalf("multiline lines = %#v", lines)
	}
	if got := renderWorkflowMultilineField(WorkflowField{Label: "Empty"}, "  ", false); !reflect.DeepEqual(got, []string{"  Empty:", "    "}) {
		t.Fatalf("empty multiline lines = %#v", got)
	}
	choices := tagsToChoices([]gitbackend.TagInfo{{Name: "v1", ObjectID: "abc"}, {Name: "v2", ObjectID: "def"}})
	if !reflect.DeepEqual(choices, []WorkflowChoice{{Value: "v1", Label: "v1  abc"}, {Value: "v2", Label: "v2  def"}}) {
		t.Fatalf("tag choices = %#v", choices)
	}

	m := New(nil)
	m.branches = []gitbackend.Branch{{Name: "one"}, {Name: "two"}}
	m.branchCursor = 0
	_, _ = m.handleBranchKey("down")
	_, _ = m.handleBranchKey("k")
	_, _ = m.handleBranchKey("unknown")
	if m.branchCursor != 0 {
		t.Fatalf("branch cursor = %d", m.branchCursor)
	}
	_, _ = m.handleBranchKey("esc")
	if m.mode != modeStatus {
		t.Fatalf("branch close mode = %v", m.mode)
	}

	navigation := navigationUIModel()
	navigation.tree.SetCursor(sectionmodel.SectionID("status/unstaged/file/one.txt"))
	if cmd := navigation.jumpToNextStatusSection(); cmd != nil || navigation.tree.Cursor() != "status/staged" {
		t.Fatalf("next section cursor=%q cmd=%v", navigation.tree.Cursor(), cmd)
	}
	empty := New(nil)
	if cmd := empty.jumpToNextStatusSection(); cmd != nil {
		t.Fatal("empty status navigation returned a command")
	}
}

func TestStageTwoBranchConfigurationHelpers(t *testing.T) {
	var setMode gitbackend.RebaseMode
	unset := false
	set := func(mode gitbackend.RebaseMode) error { setMode = mode; return nil }
	clear := func() error { unset = true; return nil }
	for _, action := range []string{"false", "true", "merges", "interactive"} {
		if err := applyRebase(context.Background(), action, set, clear); err != nil || setMode != gitbackend.RebaseMode(action) {
			t.Fatalf("applyRebase(%q) mode=%q err=%v", action, setMode, err)
		}
	}
	if err := applyRebase(context.Background(), "unset", set, clear); err != nil || !unset {
		t.Fatalf("unset = %t, %v", unset, err)
	}
	if err := applyRebase(context.Background(), "keep", set, clear); err != nil {
		t.Fatal(err)
	}
	if err := applyRebase(context.Background(), "bad", set, clear); err == nil {
		t.Fatal("invalid rebase action accepted")
	}
	values := WorkflowValues{"description_action": "keep", "upstream": "keep", "rebase": "keep", "push_remote": "keep", "pull_rebase": "keep", "push_default": "keep"}
	if err := applyBranchConfiguration(context.Background(), nil, values); err != nil {
		t.Fatalf("keep-only branch configuration: %v", err)
	}
}

func TestStageTwoCheckoutAndValidationHelpers(t *testing.T) {
	if optionValueActive(OptionValue{}) || !optionValueActive(OptionValue{Enabled: true}) || !optionValueActive(OptionValue{Value: "true"}) {
		t.Fatal("optionValueActive did not distinguish inactive and active values")
	}
	if checkoutRecurseSubmodulesOption(keymap.CommandNone) {
		t.Fatal("unknown checkout option matched recurse-submodules")
	}
	if err := validateUIBinding(keymap.Binding{Command: keymap.CommandMoveDown, Handler: keymap.HandlerExecute}, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateUIBinding(keymap.Binding{Command: "missing", Handler: keymap.HandlerExecute}, nil); err == nil {
		t.Fatal("missing execute handler was accepted")
	}
	if err := validateUIBinding(keymap.Binding{Command: "bad-infix", Handler: keymap.HandlerInfix}, nil); err == nil {
		t.Fatal("invalid infix handler was accepted")
	}
	if err := validateUIBinding(keymap.Binding{Command: "untyped", Availability: keymap.AvailabilityNever}, nil); err == nil {
		t.Fatal("untyped unavailability was accepted")
	}
	if capabilityMatchesManifest(WorkflowCapability{UpstreamCommand: "not-in-manifest"}) {
		t.Fatal("unknown capability matched manifest")
	}
}

func TestStageTwoRemoteConfigurationHelpers(t *testing.T) {
	v := WorkflowValues{"mode": "replace", "value": "url"}
	if got := replacementValue(v, "mode", "value"); got == nil || *got != "url" {
		t.Fatalf("replacementValue = %v", got)
	}
	for _, test := range []struct {
		mode, value string
		wantErr     bool
	}{{"unchanged", "", false}, {"clear", "", false}, {"replace", `["a"]`, false}, {"replace", "null", true}, {"replace", "bad", true}, {"bad", "", true}} {
		_, err := remoteRefspecs(WorkflowValues{"mode": test.mode, "value": test.value}, "mode", "value")
		if (err != nil) != test.wantErr {
			t.Errorf("remoteRefspecs(%q, %q) err=%v", test.mode, test.value, err)
		}
	}
	for _, value := range []string{"unchanged", "default", "all", "none"} {
		if _, err := remoteTagOption(value); err != nil {
			t.Errorf("remoteTagOption(%q): %v", value, err)
		}
	}
	if _, err := remoteTagOption("bad"); err == nil {
		t.Fatal("invalid tag option accepted")
	}
	for _, value := range []string{"", "unchanged", "default", "never", "create", "warn", "always"} {
		if _, err := remoteFollowMode(value); err != nil {
			t.Errorf("remoteFollowMode(%q): %v", value, err)
		}
	}
	if _, err := remoteFollowMode("bad"); err == nil {
		t.Fatal("invalid follow mode accepted")
	}
}

func TestStageTwoPushAndRenderHelpers(t *testing.T) {
	for _, kind := range []pushWorkflowKind{pushCurrentRemote, pushCurrentUpstream, pushCurrentElsewhere, pushAnotherBranch, pushExplicitRefspecs, pushMatchingBranches, pushOneTag, pushAllTags, pushOneNotesRef} {
		if kind == pushCurrentRemote {
			continue
		}
		need, configure, err := pushRemoteRequirement(context.Background(), nil, kind)
		if err != nil || configure || need != (kind != pushCurrentUpstream) {
			t.Errorf("pushRemoteRequirement(%d) = %v %v %v", kind, need, configure, err)
		}
	}
	fields, err := pushKindFields(context.Background(), nil, pushExplicitRefspecs)
	if err != nil || len(fields) != 1 || fields[0].Name != "refspecs" {
		t.Fatalf("refspec fields = %#v, %v", fields, err)
	}
	if fields, err := pushKindFields(context.Background(), nil, pushMatchingBranches); err != nil || fields != nil {
		t.Fatalf("matching fields = %#v, %v", fields, err)
	}

	m := New(nil)
	m.width, m.mode = 120, modeStatus
	if got := ansi.Strip(m.footerLeft("Vim")); !strings.Contains(got, "Commit") {
		t.Fatalf("status footer = %q", got)
	}
	for _, current := range []mode{modeCommit, modeWorkflow, modeHelp, modeProcess, modeBranches} {
		if got := ansi.Strip(modeFooter(current)); got == "" {
			t.Errorf("modeFooter(%v) is empty", current)
		}
	}
	m.mode, m.branches, m.branchCursor = modeBranches, nil, 0
	if _, content := m.basicOverlayContent(40, 10); !strings.Contains(content, "No local branches") {
		t.Fatalf("branch overlay = %q", content)
	}
	m.mode, m.remoteField, m.remoteFetch = modeAddRemote, 1, false
	if _, content := m.basicOverlayContent(40, 10); !strings.Contains(content, "Fetch after add: no") || !strings.Contains(content, "URL: █") {
		t.Fatalf("remote overlay = %q", content)
	}
}
