package ui

import (
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

func TestHandleStatusSearchKeyDirectRoutes(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{
		{Path: "alpha.txt", Unstaged: gitbackend.ChangeModified},
		{Path: "alphabet.txt", Unstaged: gitbackend.ChangeUntracked},
	}}})
	m.tree.RevealGlobalDepth(4)

	if m.handleStatusSearchKey("x", "") {
		t.Fatal("inactive empty search handled unrelated key")
	}
	if !m.handleStatusSearchKey("/", "") || !m.searching {
		t.Fatal("slash did not start status search")
	}
	if !m.handleStatusSearchKey("a", "a") {
		t.Fatal("text input was not handled")
	}
	if m.searchQuery != "a" || len(m.searchMatches) < 2 {
		t.Fatalf("text input search state query=%q matches=%v", m.searchQuery, m.searchMatches)
	}
	if !m.handleStatusSearchKey("ctrl+h", "") || m.searchQuery != "" {
		t.Fatalf("backspace did not remove query: %q", m.searchQuery)
	}
	if !m.handleStatusSearchKey("enter", "") || m.searching || !strings.Contains(m.message, "No status matches") {
		t.Fatalf("empty search completion state searching=%v message=%q", m.searching, m.message)
	}

	m.searching, m.searchQuery = true, "alpha"
	m.refreshStatusSearch()
	if !m.handleStatusSearchKey("enter", "") || m.searching || len(m.searchMatches) != 2 {
		t.Fatal("matching search did not complete")
	}
	first := m.tree.Cursor()
	if !m.handleStatusSearchKey("n", "") || m.tree.Cursor() == first {
		t.Fatal("inactive n did not move to the next match")
	}
	if !m.handleStatusSearchKey("N", "") || m.tree.Cursor() != first {
		t.Fatal("inactive N did not move to the previous match")
	}
	if m.handleStatusSearchKey("x", "") {
		t.Fatal("inactive search handled unrelated key")
	}
	if !m.handleStatusSearchKey("esc", "") || m.searchQuery != "" {
		t.Fatal("inactive escape did not clear search")
	}

	m.searching, m.searchQuery = true, "alpha"
	m.refreshStatusSearch()
	if !m.handleStatusSearchKey("esc", "") || m.searching || m.searchQuery != "" {
		t.Fatal("active escape did not clear search")
	}
	m.detailRangeStart, m.detailRangeEnd = 1, 2
	m.searchQuery = "alpha"
	if m.handleStatusSearchKey("n", "") {
		t.Fatal("search stole patch range navigation")
	}
}

func TestHandleGlobalKeyDirectRoutes(t *testing.T) {
	quit := New(nil)
	if cmd, handled := quit.handleGlobalKey("ctrl+c"); !handled || cmd == nil {
		t.Fatal("Vim ctrl+c did not quit")
	}

	graph := New(nil)
	graph.inspectionActive, graph.revisionActive = true, true
	graph.graphReturn = &graphInspection{detail: "graph", entries: map[int]gitbackend.LogEntry{}, cursor: -1}
	if cmd, handled := graph.handleGlobalKey("esc"); !handled || cmd != nil || !graph.graphActive || graph.message != "Returned to graph" {
		t.Fatalf("graph escape handled=%v cmd=%v active=%v message=%q", handled, cmd, graph.graphActive, graph.message)
	}

	blame := New(nil)
	blame.inspectionActive, blame.revisionActive = true, true
	blame.blameReturn = &blameInspection{detail: "blame", entries: map[int]gitbackend.BlameLine{}, cursor: -1}
	if cmd, handled := blame.handleGlobalKey("esc"); !handled || cmd != nil || !blame.blameActive || blame.message != "Returned to blame" {
		t.Fatalf("blame escape handled=%v cmd=%v active=%v message=%q", handled, cmd, blame.blameActive, blame.message)
	}

	inspection := New(nil)
	inspection.inspectionActive, inspection.graphActive, inspection.blameActive, inspection.revisionActive = true, true, true, true
	if _, handled := inspection.handleGlobalKey("esc"); !handled || inspection.inspectionActive || inspection.message != "Inspection closed" {
		t.Fatalf("inspection escape handled=%v active=%v message=%q", handled, inspection.inspectionActive, inspection.message)
	}

	loading := New(nil)
	loading.workflowLoading = true
	if cmd, handled := loading.handleGlobalKey("esc"); !handled || cmd != nil || loading.workflowLoading || loading.message != "Workflow loading cancelled" {
		t.Fatalf("workflow escape handled=%v loading=%v message=%q", handled, loading.workflowLoading, loading.message)
	}

	toggle := New(nil)
	toggle.mode = modeHelp
	if cmd, handled := toggle.handleGlobalKey("f2"); !handled || cmd != nil || toggle.scheme != schemeMagit || toggle.mode != modeStatus {
		t.Fatalf("first scheme toggle handled=%v scheme=%v mode=%v", handled, toggle.scheme, toggle.mode)
	}
	if _, handled := toggle.handleGlobalKey("f2"); !handled || toggle.scheme != schemeVim {
		t.Fatalf("second scheme toggle handled=%v scheme=%v", handled, toggle.scheme)
	}
	if cmd, handled := toggle.handleGlobalKey("unrelated"); handled || cmd != nil {
		t.Fatal("global handler accepted unrelated key")
	}
}

func TestHandleHelpKeyDirectRoutes(t *testing.T) {
	closed := New(nil)
	closed.mode = modeHelp
	if cmd := closed.handleHelpKey("q"); cmd != nil || closed.mode != modeStatus {
		t.Fatalf("help close cmd=%v mode=%v", cmd, closed.mode)
	}

	scrolled := New(nil)
	scrolled.mode, scrolled.width, scrolled.height = modeHelp, 40, 8
	if cmd := scrolled.handleHelpKey("down"); cmd != nil {
		t.Fatalf("help scroll returned command %v", cmd)
	}

	prefix := New(nil)
	prefix.mode = modeHelp
	if cmd := prefix.handleHelpKey("c"); cmd != nil || prefix.mode != modeStatus || prefix.resolver.PendingPrefix() != "c" {
		t.Fatalf("help prefix cmd=%v mode=%v pending=%q", cmd, prefix.mode, prefix.resolver.PendingPrefix())
	}

	dispatch := New(nil)
	dispatch.mode = modeHelp
	_ = dispatch.handleHelpKey("tab")
	if dispatch.mode != modeStatus {
		t.Fatalf("available dispatcher entry left mode=%v", dispatch.mode)
	}

	unavailable := New(nil)
	unavailable.mode = modeHelp
	unavailable.loading = false
	unavailableKey := ""
	for _, section := range unavailable.dispatcherCatalog() {
		for _, column := range section.Columns {
			for _, entry := range column {
				if _, prefix := prefixCatalogs[entry.Key]; !entry.Available && !prefix {
					unavailableKey = entry.Key
					break
				}
			}
		}
	}
	if unavailableKey == "" {
		t.Fatal("dispatcher fixture has no unavailable non-prefix entry")
	}
	if cmd := unavailable.handleHelpKey(unavailableKey); cmd != nil || !strings.Contains(unavailable.message, "unavailable") {
		t.Fatalf("unavailable help entry cmd=%v message=%q", cmd, unavailable.message)
	}
	if cmd := unavailable.handleHelpKey("~"); cmd != nil {
		t.Fatalf("unknown help key returned command %v", cmd)
	}
}

func TestRenderStatusPanelDirectRoutes(t *testing.T) {
	m := navigationUIModel()
	if got := m.renderStatusPanel(2, 2); !strings.Contains(ansi.Strip(got), "St") {
		t.Fatalf("small status panel omitted truncated status content: %q", got)
	}

	m.tree.RevealGlobalDepth(4)
	fileID := sectionmodel.SectionID("status/unstaged/file/one.txt")
	m.tree.SetCursor(fileID)
	m.markedFiles[fileMark{kind: rowUnstaged, path: "one.txt"}] = true
	m.searchMatches = []sectionmodel.SectionID{fileID}
	expanded := m.renderStatusPanel(48, 12)
	plain := ansi.Strip(expanded)
	if lipgloss.Width(expanded) != 48 || lipgloss.Height(expanded) != 12 || !strings.Contains(plain, "●") || !strings.Contains(plain, "one.txt") || !strings.Contains(expanded, "\x1b[") {
		t.Fatalf("expanded status panel dimensions/content failed:\n%s", plain)
	}

	m.tree.ToggleFold("status/unstaged")
	m.tree.SetCursor("status/unstaged")
	if folded := ansi.Strip(m.renderStatusPanel(48, 5)); !strings.Contains(folded, "▸ Unstaged changes") {
		t.Fatalf("folded status heading missing: %q", folded)
	}

	empty := New(nil)
	if got := empty.renderStatusPanel(20, 6); lipgloss.Width(got) != 20 || lipgloss.Height(got) != 6 {
		t.Fatalf("empty status panel dimensions = %dx%d", lipgloss.Width(got), lipgloss.Height(got))
	}
}

func changeTargetModel() *Model {
	m := New(&gitbackend.Repository{})
	m.loading = false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{
		{Path: "unstaged.txt", Unstaged: gitbackend.ChangeModified},
		{Path: "staged.txt", Staged: gitbackend.ChangeModified},
	}}})
	m.stageAll = func(context.Context) error { return nil }
	m.unstageAll = func(context.Context) error { return nil }
	return m
}

func selectTargetRow(m *Model, kind rowKind) {
	for id, candidate := range m.rows {
		if candidate.kind == kind {
			m.tree.SetCursor(id)
			return
		}
	}
}

func TestPerformChangeCommandDirectRoutes(t *testing.T) {
	stage := changeTargetModel()
	selectTargetRow(stage, rowUnstaged)
	if cmd, handled := stage.performChangeCommand(keymap.CommandStage); !handled || cmd == nil || !stage.busy {
		t.Fatalf("stage handled=%v cmd=%v busy=%v", handled, cmd, stage.busy)
	}

	unstage := changeTargetModel()
	selectTargetRow(unstage, rowStaged)
	if cmd, handled := unstage.performChangeCommand(keymap.CommandUnstage); !handled || cmd == nil || !unstage.busy {
		t.Fatalf("unstage handled=%v cmd=%v busy=%v", handled, cmd, unstage.busy)
	}

	for _, command := range []keymap.CommandID{keymap.CommandStageAll, keymap.CommandUnstageAll} {
		m := changeTargetModel()
		if cmd, handled := m.performChangeCommand(command); !handled || cmd == nil || !m.busy {
			t.Errorf("%s handled=%v cmd=%v busy=%v", command, handled, cmd, m.busy)
		}
	}

	vimDiscard := changeTargetModel()
	selectTargetRow(vimDiscard, rowUnstaged)
	if cmd, handled := vimDiscard.performChangeCommand(keymap.CommandDiscard); !handled || cmd != nil || vimDiscard.mode != modeConfirm {
		t.Fatalf("Vim discard handled=%v cmd=%v mode=%v", handled, cmd, vimDiscard.mode)
	}
	magitDiscard := changeTargetModel()
	magitDiscard.scheme = schemeMagit
	selectTargetRow(magitDiscard, rowStaged)
	if _, handled := magitDiscard.performChangeCommand(keymap.CommandDiscard); !handled || magitDiscard.mode != modeConfirm {
		t.Fatalf("Magit staged discard handled=%v mode=%v", handled, magitDiscard.mode)
	}

	commit := changeTargetModel()
	commit.input = "old"
	if cmd, handled := commit.performChangeCommand(keymap.CommandCommit); !handled || cmd != nil || commit.mode != modeCommit || commit.input != "" {
		t.Fatalf("commit handled=%v cmd=%v mode=%v input=%q", handled, cmd, commit.mode, commit.input)
	}
	if cmd, handled := commit.performChangeCommand(keymap.CommandNone); handled || cmd != nil {
		t.Fatal("unknown change command was handled")
	}
}

func TestBindingConditionTargetsDirectRoutes(t *testing.T) {
	m := New(nil)
	staticCases := []struct {
		name      string
		kind      keymap.EntryKind
		condition string
		active    bool
		reason    string
	}{
		{"direct config", keymap.KindInfix, "direct-configure", false, "Configure dialog"},
		{"remote", keymap.KindBinding, "magit-get-some-remote", false, "configured remote"},
		{"branch", keymap.KindBinding, "inapt-if-not magit-get-current-branch", false, "local branch"},
		{"upstream", keymap.KindBinding, "inapt-if-not magit-get-current-remote", false, "configured upstream"},
		{"unmatched", keymap.KindBinding, "other", true, ""},
	}
	for _, test := range staticCases {
		t.Run(test.name, func(t *testing.T) {
			active, reason := m.staticBindingCondition(test.kind, test.condition)
			if active != test.active || !strings.Contains(reason, test.reason) {
				t.Fatalf("static condition = %v, %q", active, reason)
			}
		})
	}
	m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
	m.snapshot.summary.Branch, m.snapshot.summary.Upstream = "main", "origin/main"
	for _, condition := range []string{"magit-get-some-remote", "inapt-if-not magit-get-current-branch", "inapt-if-not magit-get-current-remote"} {
		if active, reason := m.staticBindingCondition(keymap.KindBinding, condition); !active || reason != "" {
			t.Errorf("satisfied static condition %q = %v, %q", condition, active, reason)
		}
	}

	if matched, active, reason := m.statusBindingCondition("unrelated"); matched || active || reason != "" {
		t.Fatalf("unrelated status condition = %v, %v, %q", matched, active, reason)
	}
	for _, kind := range []string{"unmerged", "unstaged", "staged", "modified"} {
		empty := New(nil)
		matched, active, reason := empty.statusBindingCondition("magit-anything-" + kind + "-p")
		if !matched || active || !strings.HasPrefix(reason, "requires ") {
			t.Errorf("empty %s condition = %v, %v, %q", kind, matched, active, reason)
		}
	}
	activeCases := []struct {
		kind string
		file gitbackend.FileStatus
	}{
		{"unmerged", gitbackend.FileStatus{Staged: gitbackend.ChangeUnmerged}},
		{"unstaged", gitbackend.FileStatus{Unstaged: gitbackend.ChangeModified}},
		{"staged", gitbackend.FileStatus{Staged: gitbackend.ChangeAdded}},
		{"modified", gitbackend.FileStatus{Unstaged: gitbackend.ChangeModified}},
	}
	for _, test := range activeCases {
		active := New(nil)
		active.snapshot.status.Files = []gitbackend.FileStatus{test.file}
		matched, available, reason := active.statusBindingCondition("magit-anything-" + test.kind + "-p")
		if !matched || !available || reason != "" {
			t.Errorf("active %s condition = %v, %v, %q", test.kind, matched, available, reason)
		}
	}
}

func TestPerformNavigationCommandDirectRoutes(t *testing.T) {
	m := navigationUIModel()
	m.tree.SetCursor("status/unstaged")
	commands := []keymap.CommandID{
		keymap.CommandSectionCycle,
		keymap.CommandLocalDepth1,
		keymap.CommandCopyThing,
		keymap.CommandDescribeSection,
	}
	for _, command := range commands {
		if _, handled := m.performNavigationCommand(command); !handled {
			t.Errorf("%s was not handled", command)
		}
	}
	if cmd, handled := m.performNavigationCommand(keymap.CommandNone); handled || cmd != nil {
		t.Fatal("unknown navigation command was handled")
	}
}
