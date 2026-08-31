package ui

import (
	"reflect"
	"strings"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

func markedFileModel(t *testing.T) *Model {
	t.Helper()
	roots, rows := project(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{
		{Path: "a.txt", Unstaged: gitbackend.ChangeModified},
		{Path: "b.txt", Unstaged: gitbackend.ChangeUntracked},
		{Path: "c.txt", Staged: gitbackend.ChangeModified},
	}}})
	tree := sectionmodel.New(roots)
	return &Model{tree: tree, rows: rows, markedFiles: make(map[fileMark]bool)}
}

func moveCursorToPath(t *testing.T, m *Model, path string) {
	t.Helper()
	ids := m.tree.VisibleSectionIDs()
	for _, id := range ids {
		if m.rows[id].path == path {
			m.tree.SetCursor(id)
			return
		}
	}
	t.Fatalf("path %q not visible", path)
}

func TestFileMarksDriveCompatibleBatchOperations(t *testing.T) {
	m := markedFileModel(t)
	moveCursorToPath(t, m, "a.txt")
	if !m.toggleFileMark() {
		t.Fatal("unstaged file was not markable")
	}
	moveCursorToPath(t, m, "b.txt")
	m.toggleFileMark()
	moveCursorToPath(t, m, "c.txt")
	m.toggleFileMark()

	if got, want := m.fileOperationPaths(rowUnstaged, rowUntracked), []string{"a.txt", "b.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stage paths = %v, want %v", got, want)
	}
	if got, want := m.fileOperationPaths(rowStaged), []string{"c.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unstage paths = %v, want %v", got, want)
	}
}

func TestFileMarksToggleAndPrune(t *testing.T) {
	m := markedFileModel(t)
	moveCursorToPath(t, m, "a.txt")
	m.toggleFileMark()
	if !m.rowMarked(m.rows[m.tree.Cursor()]) {
		t.Fatal("marked row not reported as marked")
	}
	m.toggleFileMark()
	if len(m.markedFiles) != 0 {
		t.Fatalf("toggle left marks: %v", m.markedFiles)
	}
	m.markedFiles[fileMark{kind: rowUnstaged, path: "missing.txt"}] = true
	m.pruneFileMarks()
	if len(m.markedFiles) != 0 {
		t.Fatalf("prune left stale marks: %v", m.markedFiles)
	}
}

func TestActiveInspectionRevisionFeedsContextualActions(t *testing.T) {
	m := &Model{graphActive: true, graphCursor: 4, graphEntries: map[int]gitbackend.LogEntry{4: {ID: "graph-id"}}}
	if got := selectedHistoryRevision(m); got != "graph-id" {
		t.Fatalf("history revision = %q", got)
	}
	if got := selectedInspectRevision(m); got != "graph-id" {
		t.Fatalf("inspect revision = %q", got)
	}
	m.graphActive, m.revisionActive, m.revisionID = false, true, "revision-id"
	if got := selectedHistoryRevision(m); got != "revision-id" {
		t.Fatalf("revision detail selection = %q", got)
	}
}

func TestDiscardConfirmationListsEveryMarkedPath(t *testing.T) {
	text := discardConfirmationText("", []string{"a.txt", "b.txt"})
	for _, want := range []string{"• a.txt", "• b.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("confirmation omitted %q: %q", want, text)
		}
	}
}
