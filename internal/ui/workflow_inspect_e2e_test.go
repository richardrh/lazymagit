package ui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newInspectE2EModel(t *testing.T) *Model {
	t.Helper()
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "inspection first")
	r.write("story.txt", "one\ntwo\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "inspection second")
	r.write("story.txt", "one\ntwo\nworking\n")
	m := newE2EModel(t, r)

	return m
}

func sendInspectSequence(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, key := range keys {
		sendE2EKey(t, m, keyMsg(key))
	}
}

func assertInspectDetail(t *testing.T, m *Model, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(m.detail, value) {
			t.Fatalf("inspection detail omitted %q:\n%s", value, m.detail)
		}
	}
	if strings.ContainsAny(m.detail, "\x1b\x00") {
		t.Fatalf("inspection detail retained terminal controls: %q", m.detail)
	}
}

// assertInspectView checks the rendered screen rather than the raw detail
// buffer. The Log tab owns its own list, so its assertions belong on the view.
func assertInspectView(t *testing.T, m *Model, values ...string) {
	t.Helper()
	plain := ansi.Strip(m.render())
	for _, value := range values {
		if !strings.Contains(plain, value) {
			t.Fatalf("rendered view omitted %q:\n%s", value, plain)
		}
	}
}

func TestInspectTopLevelFamiliesThroughModelUpdate(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []string
	}{
		{"d", []string{"d", "d"}, []string{"Unstaged diff", "+working"}},
		{"D", []string{"D", "g"}, []string{"Unstaged diff", "+working"}},
		{"yr", []string{"y", "r", "y"}, []string{"References", "Local branches", "main"}},
		{"Y", []string{"Y"}, []string{"Cherries", "inspection second"}},
		{"H", []string{"H"}, []string{"Section information", "Section:", "Repository operation: none"}},
		{"e", []string{"e"}, []string{"Terminal comparison (unified)", "+working"}},
		{"E", []string{"E", "u"}, []string{"Terminal comparison (unified): unstaged", "+working"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newInspectE2EModel(t)
			sendInspectSequence(t, m, test.keys...)
			assertInspectDetail(t, m, test.want...)
		})
	}
}

func TestInspectBlameThroughModelUpdate(t *testing.T) {
	m := newInspectE2EModel(t)
	// The initial cursor is the Unstaged heading; select its tracked file.
	sendInspectSequence(t, m, "j", "alt+b")
	assertInspectDetail(t, m, "Blame story.txt", "inspection second", "| one", "| two", "| working")
	if !m.blameActive || m.blameCursor < 0 {
		t.Fatalf("blame was not selectable: active=%t cursor=%d", m.blameActive, m.blameCursor)
	}
	first := m.blameCursor
	sendInspectSequence(t, m, "j")
	if m.blameCursor == first {
		t.Fatalf("blame next did not advance from line %d", first)
	}
	selected := m.blameCursor
	sendInspectSequence(t, m, "enter")
	if m.blameActive || !m.revisionActive {
		t.Fatalf("opening blamed commit did not enter revision inspection: blame=%t revision=%t", m.blameActive, m.revisionActive)
	}
	assertInspectDetail(t, m, "Commit", "inspection")
	sendInspectSequence(t, m, "esc")
	if !m.blameActive || m.blameCursor != selected {
		t.Fatalf("Esc did not restore blame selection: active=%t cursor=%d want=%d", m.blameActive, m.blameCursor, selected)
	}
}

func TestInspectLogTabRoutesMagitLogSuffixes(t *testing.T) {
	m := newInspectE2EModel(t)
	sendInspectSequence(t, m, "l", "l")
	if !m.logTab || len(m.logEntries) != 2 {
		t.Fatalf("l l did not open the log tab: tab=%t entries=%d", m.logTab, len(m.logEntries))
	}
	assertInspectView(t, m, "inspection second", "inspection first")
	if m.logCursor != 0 {
		t.Fatalf("log tab opened on row %d, want the newest commit", m.logCursor)
	}
	// Selecting a row loads that commit into the detail pane.
	if !strings.Contains(m.detail, "inspection second") {
		t.Fatalf("log detail pane omitted the selected commit:\n%s", m.detail)
	}
	sendInspectSequence(t, m, "esc")
	if m.logTab {
		t.Fatal("Esc did not leave the Log tab")
	}
}

func TestInspectAllRefsLogTabThroughModelUpdate(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", "--", "base.txt")
	r.git("commit", "-m", "graph base")
	r.git("switch", "-c", "topic")
	r.write("topic.txt", "topic\n")
	r.git("add", "--", "topic.txt")
	r.git("commit", "-m", "graph topic")
	r.git("switch", "main")
	r.write("main.txt", "main\n")
	r.git("add", "--", "main.txt")
	r.git("commit", "-m", "graph main")

	m := newE2EModel(t, r)

	sendInspectSequence(t, m, "alt+g")
	if !m.logTab || len(m.logEntries) != 3 || m.logCursor != 0 {
		t.Fatalf("all-refs log tab not loaded: tab=%t entries=%d cursor=%d", m.logTab, len(m.logEntries), m.logCursor)
	}
	assertInspectView(t, m, "graph topic", "graph main", "graph base")

	first := m.logCursor
	sendInspectSequence(t, m, "j")
	if m.logCursor == first {
		t.Fatalf("log tab next did not advance from row %d", first)
	}
	// Mark the first row, then the second. A third mark is refused so the
	// compared range can never widen past what the user reviewed.
	sendInspectSequence(t, m, "g", "g", "alt+m")
	if len(m.markedCommits) != 1 || m.markedCommits[0] != m.logEntries[0].ID {
		t.Fatalf("mark did not record the selected commit: %v", m.markedCommits)
	}
	// The marked row must be visibly distinguishable in the gutter.
	assertInspectView(t, m, logMarkGlyph)
	sendInspectSequence(t, m, "j", "alt+m")
	if len(m.markedCommits) != 2 {
		t.Fatalf("second mark rejected: %v", m.markedCommits)
	}
	sendInspectSequence(t, m, "j", "alt+m")
	if len(m.markedCommits) != 2 || !strings.Contains(m.message, "already marked") {
		t.Fatalf("third mark was not refused: marks=%v message=%q", m.markedCommits, m.message)
	}
	sendInspectSequence(t, m, "esc")
	if m.logTab {
		t.Fatal("Esc did not return to the Status tab")
	}
}

func TestInspectPromptedLogAndRefsThroughModelUpdate(t *testing.T) {
	m := newInspectE2EModel(t)

	sendInspectSequence(t, m, "l", "o")
	if m.workflow == nil {
		t.Fatalf("log-other prompt did not open: %q", m.message)
	}
	historyE2EReplaceField(t, m, "HEAD~1")
	historyE2ESubmit(t, m)
	assertInspectView(t, m, "Log HEAD~1", "inspection first")

	sendInspectSequence(t, m, "l", "B")
	if m.workflow == nil {
		t.Fatalf("matching-branches prompt did not open: %q", m.message)
	}
	historyE2EReplaceField(t, m, "main")
	historyE2ESubmit(t, m)
	assertInspectView(t, m, "Log matching branches", "inspection second")

	sendInspectSequence(t, m, "y", "r", "o")
	if m.workflow == nil {
		t.Fatalf("refs-other prompt did not open: %q", m.message)
	}
	historyE2EReplaceField(t, m, "HEAD~1")
	historyE2ESubmit(t, m)
	assertInspectDetail(t, m, "References for HEAD~1", "Local branches")
}

func TestInspectReflogShortlogAndMergedRefsThroughModelUpdate(t *testing.T) {
	m := newInspectE2EModel(t)

	sendInspectSequence(t, m, "l", "r")
	assertInspectDetail(t, m, "Reflog main", "commit: inspection second")

	sendInspectSequence(t, m, "l", "O")
	if m.workflow == nil {
		t.Fatalf("reflog-other prompt did not open: %q", m.message)
	}
	historyE2EReplaceField(t, m, "HEAD")
	historyE2ESubmit(t, m)
	assertInspectDetail(t, m, "Reflog HEAD", "commit: inspection second")

	sendInspectSequence(t, m, "l", "s", "-", "s", "s")
	if m.workflow == nil {
		t.Fatalf("shortlog prompt did not open: %q", m.message)
	}
	historyE2EReplaceField(t, m, "HEAD~1")
	historyE2ESubmit(t, m)
	assertInspectDetail(t, m, "Shortlog HEAD~1", "UI E2E Test")

	sendInspectSequence(t, m, "y", "r", "-", "m", "y")
	assertInspectDetail(t, m, "References", "Local branches", "main")
}

func TestInspectRefsContainsAndSortOptionsThroughModelUpdate(t *testing.T) {
	m := newInspectE2EModel(t)

	// --contains and --sort are entered through the manifest transient, then
	// executed through the same model/update path as the other inspection views.
	sendInspectSequence(t, m, "y", "r", "-", "c", "H", "E", "A", "D")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	sendInspectSequence(t, m, "-", "s", "-", "s", "u", "b", "j", "e", "c", "t")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	sendInspectSequence(t, m, "y")
	assertInspectDetail(t, m, "References", "Local branches", "main")
}

func TestInspectLogRefreshPropagatesLimitOptionThroughModelUpdate(t *testing.T) {
	m := newInspectE2EModel(t)
	sendInspectSequence(t, m, "L", "-", "n", "1")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	sendInspectSequence(t, m, "g")
	if len(m.logEntries) != 1 || m.logEntries[0].Subject != "inspection second" {
		t.Fatalf("-n 1 was not propagated: %d entries %v", len(m.logEntries), m.logEntries)
	}
	assertInspectView(t, m, "Log", "inspection second")
}

func TestInspectionResultViewPagesWithoutLaunchingWork(t *testing.T) {
	m := newInspectE2EModel(t)
	sendInspectSequence(t, m, "l", "l")
	// Preserve the real query result and extend it beyond the viewport to exercise
	// the generic detail pane's paging boundary deterministically.
	m.detail += strings.Repeat("\ncontinued result", 100)
	before := m.detailOffset
	sendE2EKey(t, m, keyMsg("pgdown"))
	if m.busy || m.workflowLoading || m.detailOffset <= before {
		t.Fatalf("page down launched work or moved backwards: busy=%v loading=%v offset=%d", m.busy, m.workflowLoading, m.detailOffset)
	}
}
