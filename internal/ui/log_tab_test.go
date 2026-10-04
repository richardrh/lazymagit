package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	gitbackend "github.com/richardrh/lazymagit/internal/git"

	"github.com/charmbracelet/x/ansi"
)

func TestLogRowDropsColumnsBeforeTruncatingSubject(t *testing.T) {
	m := New(nil)
	when := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	m.logEntries = []gitbackend.LogEntry{
		{ShortID: "abc1234", AuthorName: "A Very Long Author Name", AuthorDate: when, Subject: "a representative subject line"},
		{ShortID: "def5678", AuthorName: "A Very Long Author Name", AuthorDate: when, Subject: "short"},
	}

	narrow := m.logLayout(41)
	if narrow.author != 0 || narrow.date {
		t.Fatalf("narrow layout kept columns it cannot afford: %+v", narrow)
	}
	row := m.logRowText(m.logEntries[0], 41, narrow, false)
	if !strings.Contains(row, "representative subject") {
		t.Fatalf("narrow row dropped the subject: %q", row)
	}
	if strings.Contains(row, "Very Long Author") {
		t.Fatalf("narrow row kept the author column: %q", row)
	}

	wide := m.logLayout(120)
	if !wide.date || wide.author == 0 {
		t.Fatalf("wide layout dropped the date or author column: %+v", wide)
	}
	row = m.logRowText(m.logEntries[0], 120, wide, false)
	if !strings.Contains(row, "A Very Long Auth") || !strings.Contains(row, "representative subject") {
		t.Fatalf("wide row lost a column: %q", row)
	}
}

func TestLogRowsAlignOnSharedColumns(t *testing.T) {
	m := New(nil)
	m.logEntries = []gitbackend.LogEntry{
		{ShortID: "aaaaaaa", AuthorName: "Short Name", Subject: "a much longer subject that would drop the date for this row alone"},
		{ShortID: "bbbbbbb", AuthorName: "A Much Longer Name", Subject: "tiny"},
	}
	layout := m.logLayout(200)
	first := m.logRowText(m.logEntries[0], 200, layout, false)
	second := m.logRowText(m.logEntries[1], 200, layout, false)
	if strings.Index(first, "Short Name") != strings.Index(second, "A Much Longer Name") {
		t.Fatalf("author columns are not aligned:\n%q\n%q", first, second)
	}
	if strings.Index(first, "aaaaaaa") != strings.Index(second, "bbbbbbb") {
		t.Fatalf("short id columns are not aligned:\n%q\n%q", first, second)
	}
}

func TestLogRowMarksOnlyMarkedRows(t *testing.T) {
	m := New(nil)
	m.logEntries = []gitbackend.LogEntry{{ShortID: "abc1234", Subject: "subject"}}
	layout := m.logLayout(80)
	if marked := m.logRowText(m.logEntries[0], 80, layout, true); !strings.HasPrefix(marked, logMarkGlyph) {
		t.Fatalf("marked row did not open its gutter: %q", marked)
	}
	if unmarked := m.logRowText(m.logEntries[0], 80, layout, false); strings.HasPrefix(unmarked, logMarkGlyph) {
		t.Fatalf("unmarked row showed the mark glyph: %q", unmarked)
	}
}

func TestLogRowBoundsWideGraphLanes(t *testing.T) {
	m := New(nil)
	m.logEntries = []gitbackend.LogEntry{
		{ShortID: "abc1234", Graph: "* | | |\\|_", Subject: "merge commit"},
		{ShortID: "def5678", Graph: "|", Subject: "side commit"},
	}
	layout := m.logLayout(41)
	if layout.graph > logGraphWidth {
		t.Fatalf("graph column exceeded its bound: %+v", layout)
	}
	row := m.logRowText(m.logEntries[0], 41, layout, false)
	if ansi.StringWidth(row) > 41 {
		t.Fatalf("row exceeded the pane width: %q", row)
	}
	if !strings.Contains(row, "merge commit") {
		t.Fatalf("graph lanes crowded out the subject: %q", row)
	}
}

func TestLogTabMarkToggleRejectsAThirdRevision(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.logTab = true
	m.logEntries = []gitbackend.LogEntry{{ID: "one", ShortID: "one"}, {ID: "two", ShortID: "two"}, {ID: "three", ShortID: "three"}}

	m.logCursor = 0
	m.toggleLogCommitMark()
	m.logCursor = 1
	m.toggleLogCommitMark()
	if len(m.markedCommits) != 2 {
		t.Fatalf("two marks = %v", m.markedCommits)
	}

	m.logCursor = 2
	m.toggleLogCommitMark()
	if len(m.markedCommits) != 2 || !strings.Contains(m.message, "already marked") {
		t.Fatalf("third mark accepted: marks=%v message=%q", m.markedCommits, m.message)
	}

	m.logCursor = 0
	m.toggleLogCommitMark()
	if len(m.markedCommits) != 1 || m.markedCommits[0] != "two" {
		t.Fatalf("unmark left the wrong set: %v", m.markedCommits)
	}
}

func TestLogTabCursorMovementClampsToTheLoadedRange(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.logTab = true
	m.logEntries = []gitbackend.LogEntry{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	m.height, m.width = 30, 100

	m.moveLogCursor(-5)
	if m.logCursor != 0 {
		t.Fatalf("cursor above the first row: %d", m.logCursor)
	}
	m.moveLogCursor(99)
	if m.logCursor != 2 {
		t.Fatalf("cursor past the last row: %d", m.logCursor)
	}
	if m.logOffset < 0 || m.logOffset > 2 {
		t.Fatalf("scroll offset out of range: %d", m.logOffset)
	}
}

func TestLogTabIsNotRenderedOnShortTerminals(t *testing.T) {
	for _, height := range []int{5, 7} {
		m := New(&gitbackend.Repository{})
		m.width, m.height, m.loading = 100, height, false
		m.logTab = true
		m.logEntries = []gitbackend.LogEntry{{ID: "a", ShortID: "a", Subject: "only commit"}}
		rendered := m.render()
		if strings.Contains(rendered, "Status") && strings.Contains(rendered, "|  Log") {
			t.Fatalf("height %d drew the tab strip and stole body rows: %q", height, rendered)
		}
	}
}

func TestLogCompareUsesMarkedRevisionsAsAMergeBaseRange(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "compare base")
	base := r.git("rev-parse", "HEAD")
	r.write("story.txt", "one\ntwo\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "compare middle")
	r.write("story.txt", "one\ntwo\nthree\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "compare tip")

	m := newE2EModel(t, r)
	sendInspectSequence(t, m, "alt+l")
	if !m.logTab || len(m.logEntries) != 3 {
		t.Fatalf("log tab not loaded: tab=%t entries=%d", m.logTab, len(m.logEntries))
	}
	tip := m.logEntries[0].ID

	// Mark the newest row first and the oldest second. The comparison must not
	// depend on that order: git's one-second timestamps cannot order these two
	// commits, so the range is resolved by ancestry instead.
	m.logCursor = 0
	m.toggleLogCommitMark()
	m.logCursor = 2
	m.toggleLogCommitMark()
	if m.markedCommits[0] != tip || m.markedCommits[1] != base {
		t.Fatalf("marks = %v, want tip then base", m.markedCommits)
	}

	comparison, err := logCompareRevisions(m)
	if err != nil {
		t.Fatalf("logCompareRevisions: %v", err)
	}
	if comparison.Base != base || comparison.Target != tip {
		t.Fatalf("compare range = %s...%s, want %s...%s", shortID(comparison.Base), shortID(comparison.Target), shortID(base), shortID(tip))
	}
	if !comparison.TripleDot {
		t.Fatal("two-mark comparison did not request merge-base semantics")
	}
	if want := shortID(base) + "..." + shortID(tip); comparison.Label != want {
		t.Fatalf("compare label = %q, want %q", comparison.Label, want)
	}

	runE2ECmd(t, m, compareMarkedCommits(m))
	if !m.inspectionActive {
		t.Fatal("compare did not open an inspection over the Log tab")
	}
	// Merge-base semantics means the diff is the work between the two commits,
	// whichever order they were marked in.
	for _, want := range []string{"+two", "+three"} {
		if !strings.Contains(m.detail, want) {
			t.Fatalf("comparison omitted %q:\n%s", want, m.detail)
		}
	}
	if strings.Contains(m.detail, "+one") {
		t.Fatalf("comparison included the shared ancestor's change:\n%s", m.detail)
	}

	// Esc closes the comparison but must leave the Log tab standing.
	sendInspectSequence(t, m, "esc")
	if !m.logTab || m.inspectionActive {
		t.Fatalf("Esc discarded the Log tab: tab=%t inspection=%v", m.logTab, m.inspectionActive)
	}
}

func TestLogCompareWithOneMarkUsesTheFirstParent(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "parent commit")
	parent := r.git("rev-parse", "HEAD")
	r.write("story.txt", "one\ntwo\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "child commit")

	m := newE2EModel(t, r)
	sendInspectSequence(t, m, "alt+l")
	m.logCursor = 0
	m.toggleLogCommitMark()

	comparison, err := logCompareRevisions(m)
	if err != nil {
		t.Fatalf("logCompareRevisions: %v", err)
	}
	if comparison.Base != parent {
		t.Fatalf("single-mark base = %s, want the first parent %s", shortID(comparison.Base), shortID(parent))
	}
	if comparison.TripleDot {
		t.Fatal("single-mark comparison must stay two-dot against the first parent")
	}
	if comparison.Label != shortID(comparison.Target) {
		t.Fatalf("single-mark label = %q, want %q", comparison.Label, shortID(comparison.Target))
	}

	runE2ECmd(t, m, compareMarkedCommits(m))
	if !strings.Contains(m.detail, "+two") {
		t.Fatalf("single-mark comparison omitted the commit's change:\n%s", m.detail)
	}
}

func TestLogCompareWithoutMarksIsRefused(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "only commit")

	m := newE2EModel(t, r)
	sendInspectSequence(t, m, "alt+l")
	if cmd := compareMarkedCommits(m); cmd != nil {
		t.Fatal("compare ran without a marked commit")
	}
	if !m.isError || !strings.Contains(m.message, "mark one or two") {
		t.Fatalf("unmarked compare message = %q error=%v", m.message, m.isError)
	}
}

func TestLogCompareOnARootCommitIsRefused(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("root.txt", "root\n")
	r.git("add", "--", "root.txt")
	r.git("commit", "-m", "root commit")

	m := newE2EModel(t, r)
	sendInspectSequence(t, m, "alt+l")
	m.logCursor = 0
	m.toggleLogCommitMark()
	if cmd := compareMarkedCommits(m); cmd != nil {
		t.Fatal("compare ran against a parentless root commit")
	}
	if !strings.Contains(m.message, "no parent") {
		t.Fatalf("root compare message = %q", m.message)
	}
}

func TestLogTabKeyHandlesEveryMotionBinding(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.logTab = true
	m.height, m.width = 40, 100
	m.logEntries = make([]gitbackend.LogEntry, 60)
	for index := range m.logEntries {
		m.logEntries[index] = gitbackend.LogEntry{ID: fmt.Sprintf("commit-%02d", index), ShortID: fmt.Sprintf("c%02d", index)}
	}

	page := max(1, m.detailViewportHeight())
	half := max(1, page/2)
	cursor := 0
	for _, test := range []struct {
		key  string
		want int
	}{
		{"down", 1},
		{"j", 2},
		{"k", 1},
		{"up", 0},
		{"ctrl+d", half},
		{"ctrl+u", 0},
		{"ctrl+f", page},
		{"ctrl+b", 0},
		{"pagedown", page},
		{"pageup", 0},
		{"home", 0},
		{"end", 59},
	} {
		m.logCursor = cursor
		if _, handled := m.handleLogTabKey(test.key); !handled {
			t.Fatalf("%s was not handled by the Log tab", test.key)
		}
		if m.logCursor != test.want {
			t.Errorf("%s moved to row %d, want %d", test.key, m.logCursor, test.want)
		}
		cursor = test.want
	}

	if _, handled := m.handleLogTabKey("x"); handled {
		t.Error("the Log tab swallowed an unrelated key")
	}
}

func TestLogTabKeyIgnoresMotionWithoutCommits(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.logTab = true
	m.logEntries = nil
	for _, key := range []string{"j", "k", "down", "up", "home", "end", "ctrl+d"} {
		if _, handled := m.handleLogTabKey(key); handled {
			t.Errorf("%s was handled with an empty commit list", key)
		}
	}
}

func TestLogCompareRefusesMoreMarksThanItCanExpress(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "only commit")

	m := newE2EModel(t, r)
	sendInspectSequence(t, m, "alt+l")
	// The toggle caps the set, so an oversized set can only arrive from a future
	// caller. It must still resolve to an error rather than a silent truncation.
	m.markedCommits = []string{"a", "b", "c"}
	if _, err := logCompareRevisions(m); err == nil {
		t.Fatal("an oversized mark set resolved to a comparison")
	}
}

func TestLogTabRefreshKeepsSelectionAndOwnQuery(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "first")
	r.write("story.txt", "one\ntwo\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "second")

	m := newE2EModel(t, r)
	sendInspectSequence(t, m, "alt+g")
	if !m.logTab || len(m.logEntries) != 2 {
		t.Fatalf("log tab not loaded: tab=%t entries=%d", m.logTab, len(m.logEntries))
	}
	m.logCursor = 1

	// A commit on another branch must appear after a refresh, and the selection
	// must stay on the commit the user was looking at.
	r.git("switch", "-c", "topic")
	r.write("topic.txt", "topic\n")
	r.git("add", "--", "topic.txt")
	r.git("commit", "-m", "topic commit")
	r.git("switch", "main")

	sendInspectSequence(t, m, "g", "r")
	if len(m.logEntries) != 3 {
		t.Fatalf("refresh did not reload the tab's own query: %d entries", len(m.logEntries))
	}
	if got := m.logEntries[m.logCursor].Subject; got != "second" {
		t.Fatalf("refresh lost the selection: row %d = %q", m.logCursor, got)
	}
}
