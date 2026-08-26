package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

func navigationUIModel() *Model {
	m := New(nil)
	m.loading = false
	m.scheme = schemeMagit
	m.width, m.height = 100, 18
	m.install(snapshot{
		summary: gitbackend.Summary{Branch: "main", Head: "0123456789abcdef", Upstream: "origin/main", Ahead: 2, Behind: 1},
		status: gitbackend.Status{Files: []gitbackend.FileStatus{
			{Path: "one.txt", Unstaged: gitbackend.ChangeModified},
			{Path: "two.txt", Staged: gitbackend.ChangeModified},
		}},
		recent: []gitbackend.Commit{{ID: "abcdef0123456789", ShortID: "abcdef0", Subject: "subject"}},
	})
	return m
}

func TestNavigationKeysDriveTreeAndPreserveVimJ(t *testing.T) {
	m := navigationUIModel()
	unstaged := sectionmodel.SectionID("status/unstaged")
	file := sectionmodel.SectionID("status/unstaged/file/one.txt")
	m.tree.SetCursor(file)

	if _, handled := m.handleNavigationKey(keyMsg("^")); !handled || m.tree.Cursor() != unstaged {
		t.Fatalf("parent key handled=%v cursor=%q", handled, m.tree.Cursor())
	}
	if _, handled := m.handleNavigationKey(keyMsg("ctrl+tab")); !handled || !m.tree.IsFolded(unstaged) {
		t.Fatalf("local cycle handled=%v folded=%v", handled, m.tree.IsFolded(unstaged))
	}
	if _, handled := m.handleNavigationKey(keyMsg("ctrl+tab")); !handled || m.tree.IsFolded(unstaged) {
		t.Fatalf("second local cycle did not reveal children")
	}
	if _, handled := m.handleNavigationKey(keyMsg("alt+n")); !handled || m.tree.Cursor() != "status/staged" {
		t.Fatalf("next sibling handled=%v cursor=%q", handled, m.tree.Cursor())
	}

	m.scheme = schemeVim
	before := m.tree.Cursor()
	if cmd, handled := m.handleNavigationKey(keyMsg("j")); handled || cmd != nil || m.tree.Cursor() != before {
		t.Fatal("navigation domain stole Vim j collision")
	}
	m.scheme = schemeMagit
	if _, handled := m.handleNavigationKey(keyMsg("j")); !handled || m.rows[m.tree.Cursor()].kind != rowHeading {
		t.Fatal("Magit j did not jump to a top-level status section")
	}
}

func TestDepthGlobalCycleVisitAndInformationRender(t *testing.T) {
	m := navigationUIModel()
	m.tree.SetCursor("status/unstaged")
	if _, handled := m.handleNavigationKey(keyMsg("1")); !handled || !m.tree.IsFolded("status/unstaged") {
		t.Fatal("local level one did not collapse selected tree")
	}
	if _, handled := m.handleNavigationKey(keyMsg("alt+2")); !handled {
		t.Fatal("global level two was not handled")
	}
	if got := m.tree.VisibleSectionIDs(); len(got) < 4 {
		t.Fatalf("global level two did not reveal children: %v", got)
	}
	if _, handled := m.handleNavigationKey(keyMsg("enter")); !handled || !m.tree.IsFolded("status/unstaged") {
		t.Fatal("terminal visit did not toggle heading")
	}
	if _, handled := m.handleNavigationKey(keyMsg("H")); !handled {
		t.Fatal("section information key was not handled")
	}
	plain := m.renderDetailPanel(60, 10)
	for _, want := range []string{"Section information", "status/unstaged", "Children: 1"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("section information omitted %q: %q", want, plain)
		}
	}
	if _, handled := m.handleNavigationKey(keyMsg("J")); !handled || !strings.Contains(m.detail, "Branch: main") {
		t.Fatalf("repository status not displayed: %q", m.detail)
	}
}

func TestDetailContextScrollAndOSC52ClipboardPayloads(t *testing.T) {
	m := navigationUIModel()
	m.detail = "diff --git a/a b/a\n@@ -1,4 +1,4 @@\n before\n-old\n+new\n after"
	m.detailOffset = 3
	if _, handled := m.handleNavigationKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})); !handled || m.detailOffset != 0 {
		t.Fatalf("backspace scroll handled=%v offset=%d", handled, m.detailOffset)
	}
	if _, handled := m.handleNavigationKey(keyMsg("-")); !handled || strings.Contains(m.detail, " before") || strings.Contains(m.detail, " after") {
		t.Fatalf("less-context output = %q", m.detail)
	}

	m.tree.SetCursor("status/unstaged/file/one.txt")
	cmd, handled := m.handleNavigationKey(keyMsg("ctrl+w"))
	if !handled || cmd == nil || clipboardPayload(cmd) != "one.txt" || !strings.Contains(m.message, "OSC52") {
		t.Fatalf("section copy handled=%v payload=%q message=%q", handled, clipboardPayload(cmd), m.message)
	}
	cmd, handled = m.handleNavigationKey(keyMsg("alt+w"))
	if !handled || cmd == nil || clipboardPayload(cmd) != "0123456789abcdef" {
		t.Fatalf("revision copy handled=%v payload=%q", handled, clipboardPayload(cmd))
	}
}

func TestMultiKeyNavigationCommandsHaveDispatcherTargets(t *testing.T) {
	m := navigationUIModel()
	m.tree.SetCursor("status/unstaged")
	if _, handled := m.performNavigationCommand(commandSectionCycle); !handled || !m.tree.IsFolded("status/unstaged") {
		t.Fatal("C-c TAB command target did not cycle section")
	}
	m.tree.ToggleFold("status/unstaged")
	m.tree.SetCursor("status/unstaged/file/one.txt")
	cmd, handled := m.performNavigationCommand(commandCopyThing)
	if !handled || clipboardPayload(cmd) != "one.txt" {
		t.Fatal("C-c C-w command target did not copy selected thing")
	}
}

func clipboardPayload(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	value := reflect.ValueOf(cmd())
	if value.IsValid() && value.Kind() == reflect.String {
		return value.String()
	}
	return ""
}
