package model

import (
	"reflect"
	"testing"
)

const (
	unstagedID = SectionID("status/unstaged")
	stagedID   = SectionID("status/staged")
	fileAID    = SectionID("status/unstaged/file/a.txt")
	fileBID    = SectionID("status/unstaged/file/b.txt")
	hunkAID    = SectionID("status/unstaged/file/a.txt/hunk/1")
)

func statusSections(fileATitle string) []*Section {
	return []*Section{
		NewSection(unstagedID, "Unstaged changes",
			NewSection(fileAID, fileATitle,
				NewSection(hunkAID, "@@ -1 +1 @@")),
			NewSection(fileBID, "modified b.txt")),
		NewSection(stagedID, "Staged changes"),
	}
}

func reorderedStatusSections() []*Section {
	return []*Section{
		NewSection(unstagedID, "Unstaged changes",
			NewSection(fileBID, "modified b.txt"),
			NewSection(fileAID, "modified a.txt",
				NewSection(hunkAID, "@@ -1 +1 @@"))),
		NewSection(stagedID, "Staged changes"),
	}
}

func TestSectionIdentityIsStableAndIndependentOfDisplayText(t *testing.T) {
	before := New(statusSections("modified a.txt"))
	after := New(statusSections("renamed: a.txt -> c.txt"))

	if before.Section(fileAID) == after.Section(fileAID) {
		t.Fatal("separately built trees unexpectedly share section pointers")
	}
	if got := after.Section(fileAID).ID(); got != fileAID {
		t.Fatalf("section ID changed with its display text: got %q, want %q", got, fileAID)
	}
	if got := after.Section(fileBID).ID(); got == fileAID {
		t.Fatalf("different semantic sections have the same ID %q", got)
	}
}

func TestFoldingBelongsToSectionIdentityAcrossRefresh(t *testing.T) {
	m := New(statusSections("modified a.txt"))
	m.ToggleFold(fileAID)

	assertVisible(t, m, unstagedID, fileAID, fileBID, stagedID)
	if !m.IsFolded(fileAID) {
		t.Fatal("file section should be folded")
	}

	// Replacing the rendered tree models a status refresh. Fold state follows the
	// stable identity, not the old Section pointer or heading.
	m.ReplaceSections(statusSections("renamed: a.txt -> c.txt"))

	if !m.IsFolded(fileAID) {
		t.Fatal("refresh lost fold state for an unchanged section identity")
	}
	assertVisible(t, m, unstagedID, fileAID, fileBID, stagedID)

	m.ToggleFold(fileAID)
	assertVisible(t, m, unstagedID, fileAID, hunkAID, fileBID, stagedID)
}

func TestGlobalDepthControlsTheWholeTree(t *testing.T) {
	m := New(statusSections("modified a.txt"))

	m.SetGlobalDepth(1)
	assertVisible(t, m, unstagedID, stagedID)

	m.SetGlobalDepth(2)
	assertVisible(t, m, unstagedID, fileAID, fileBID, stagedID)

	m.SetGlobalDepth(3)
	assertVisible(t, m, unstagedID, fileAID, hunkAID, fileBID, stagedID)
}

func TestCursorRetainsSectionIdentityWhenARefreshReordersSections(t *testing.T) {
	m := New(statusSections("modified a.txt"))
	m.SetCursor(fileBID)

	m.ReplaceSections(reorderedStatusSections())

	if got := m.Cursor(); got != fileBID {
		t.Fatalf("cursor followed row position instead of identity: got %q, want %q", got, fileBID)
	}
	assertVisible(t, m, unstagedID, fileBID, fileAID, hunkAID, stagedID)
}

func TestNearestVisibleCursor(t *testing.T) {
	order := []SectionID{"a", "b", "c", "d"}
	tests := []struct {
		name    string
		visible []SectionID
		cursor  SectionID
		want    SectionID
	}{
		{"empty", nil, "b", ""},
		{"next surviving", []SectionID{"a", "c"}, "b", "c"},
		{"previous surviving", []SectionID{"a"}, "c", "a"},
		{"fallback first", []SectionID{"d", "a"}, "missing", "d"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nearestVisibleCursor(order, test.visible, test.cursor); got != test.want {
				t.Fatalf("nearestVisibleCursor() = %q, want %q", got, test.want)
			}
		})
	}
}

func assertVisible(t *testing.T, m *Model, want ...SectionID) {
	t.Helper()
	if got := m.VisibleSectionIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible sections = %v, want %v", got, want)
	}
}
