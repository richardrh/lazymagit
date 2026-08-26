package model

import (
	"reflect"
	"testing"
)

const (
	rootA  = SectionID("a")
	childA = SectionID("a/1")
	grandA = SectionID("a/1/i")
	deepA  = SectionID("a/1/i/x")
	peerA  = SectionID("a/2")
	rootB  = SectionID("b")
	childB = SectionID("b/1")
)

func navigationSections() []*Section {
	return []*Section{
		NewSection(rootA, "A",
			NewSection(childA, "A1",
				NewSection(grandA, "A1i",
					NewSection(deepA, "A1ix"))),
			NewSection(peerA, "A2")),
		NewSection(rootB, "B", NewSection(childB, "B1")),
	}
}

func TestParentAndSiblingNavigationFallbacks(t *testing.T) {
	m := New(navigationSections())

	m.SetCursor(childA)
	if !m.MoveToNextSibling() || m.Cursor() != peerA {
		t.Fatalf("next sibling cursor = %q, want %q", m.Cursor(), peerA)
	}
	if !m.MoveToNextSibling() || m.Cursor() != rootB {
		t.Fatalf("next sibling fallback cursor = %q, want %q", m.Cursor(), rootB)
	}
	if !m.MoveToPreviousSibling() || m.Cursor() != rootA {
		t.Fatalf("previous root sibling cursor = %q, want %q", m.Cursor(), rootA)
	}

	m.SetCursor(childA)
	if !m.MoveToPreviousSibling() || m.Cursor() != rootA {
		t.Fatalf("previous sibling fallback cursor = %q, want parent %q", m.Cursor(), rootA)
	}
	if m.MoveToParent() {
		t.Fatal("top-level section unexpectedly had a parent")
	}
	m.SetCursor(grandA)
	if !m.MoveToParent() || m.Cursor() != childA {
		t.Fatalf("parent cursor = %q, want %q", m.Cursor(), childA)
	}

	m.SetCursor(rootB)
	if !m.MoveToNextSibling() || m.Cursor() != childB {
		t.Fatalf("expanded last sibling did not fall forward to child: got %q", m.Cursor())
	}
	if m.MoveToNextSibling() || m.Cursor() != childB {
		t.Fatalf("movement past end changed cursor to %q", m.Cursor())
	}
}

func TestSiblingFallbackUsesVisibleTraversal(t *testing.T) {
	m := New(navigationSections())
	m.SetCursor(grandA)
	if !m.MoveToNextSibling() || m.Cursor() != deepA {
		t.Fatalf("expanded last sibling should fall forward to child: got %q", m.Cursor())
	}

	m.ToggleFold(grandA)
	m.SetCursor(grandA)
	if !m.MoveToNextSibling() || m.Cursor() != peerA {
		t.Fatalf("folded last sibling should fall forward out of ancestors: got %q", m.Cursor())
	}
}

func TestRevealSelectedDepthOnlyChangesSelectedTree(t *testing.T) {
	m := New(navigationSections())
	m.SetCursor(childA)
	m.ToggleFold(rootB)

	if !m.RevealSelectedDepth(2) {
		t.Fatal("selected reveal failed")
	}
	assertNavigationVisible(t, m, rootA, childA, grandA, peerA, rootB)
	if m.Cursor() != childA || !m.IsFolded(grandA) || !m.IsFolded(rootB) {
		t.Fatalf("selected reveal disturbed cursor or another tree: cursor=%q grand-fold=%v b-fold=%v",
			m.Cursor(), m.IsFolded(grandA), m.IsFolded(rootB))
	}

	before := append([]SectionID(nil), m.VisibleSectionIDs()...)
	if m.RevealSelectedDepth(0) || m.RevealSelectedDepth(5) {
		t.Fatal("out-of-range selected depth was accepted")
	}
	if !reflect.DeepEqual(m.VisibleSectionIDs(), before) {
		t.Fatal("invalid depth changed visibility")
	}
}

func TestRevealGlobalDepthOneThroughFour(t *testing.T) {
	m := New(navigationSections())
	cases := []struct {
		depth int
		want  []SectionID
	}{
		{1, []SectionID{rootA, rootB}},
		{2, []SectionID{rootA, childA, peerA, rootB, childB}},
		{3, []SectionID{rootA, childA, grandA, peerA, rootB, childB}},
		{4, []SectionID{rootA, childA, grandA, deepA, peerA, rootB, childB}},
	}
	for _, tc := range cases {
		if !m.RevealGlobalDepth(tc.depth) {
			t.Fatalf("global depth %d failed", tc.depth)
		}
		if got := m.VisibleSectionIDs(); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("global depth %d visible = %v, want %v", tc.depth, got, tc.want)
		}
	}
}

func TestRevealGlobalDepthRetainsNearestVisibleCursor(t *testing.T) {
	m := New(navigationSections())
	m.SetCursor(deepA)
	m.RevealGlobalDepth(2)
	if m.Cursor() != childA {
		t.Fatalf("cursor after depth reduction = %q, want %q", m.Cursor(), childA)
	}
}

func TestCycleLocalCollapsedChildrenAll(t *testing.T) {
	m := New(navigationSections())
	m.SetCursor(rootA)

	if !m.CycleLocal() { // all -> collapsed
		t.Fatal("local cycle failed")
	}
	assertNavigationVisible(t, m, rootA, rootB, childB)
	if m.Cursor() != rootA {
		t.Fatalf("local collapse moved cursor to %q", m.Cursor())
	}

	m.CycleLocal() // collapsed -> children
	assertNavigationVisible(t, m, rootA, childA, peerA, rootB, childB)
	m.CycleLocal() // children -> all
	assertNavigationVisible(t, m, rootA, childA, grandA, deepA, peerA, rootB, childB)
}

func TestCycleGlobalCollapsedChildrenAll(t *testing.T) {
	m := New(navigationSections())
	m.SetCursor(deepA)

	m.CycleGlobal() // all -> collapsed
	assertNavigationVisible(t, m, rootA, rootB)
	if m.Cursor() != rootA {
		t.Fatalf("global collapse cursor = %q, want %q", m.Cursor(), rootA)
	}
	m.CycleGlobal() // collapsed -> children
	assertNavigationVisible(t, m, rootA, childA, peerA, rootB, childB)
	m.CycleGlobal() // children -> all
	assertNavigationVisible(t, m, rootA, childA, grandA, deepA, peerA, rootB, childB)
}

func TestNavigationFoldAndCursorStateSurvivesRefresh(t *testing.T) {
	m := New(navigationSections())
	m.SetCursor(childA)
	m.RevealSelectedDepth(2)
	m.ReplaceSections(navigationSections())

	if m.Cursor() != childA {
		t.Fatalf("refresh cursor = %q, want %q", m.Cursor(), childA)
	}
	if !m.IsFolded(grandA) {
		t.Fatal("refresh lost fold established by selected reveal")
	}
	assertNavigationVisible(t, m, rootA, childA, grandA, peerA, rootB, childB)
}

func assertNavigationVisible(t *testing.T, m *Model, want ...SectionID) {
	t.Helper()
	if got := m.VisibleSectionIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible sections = %v, want %v", got, want)
	}
}
