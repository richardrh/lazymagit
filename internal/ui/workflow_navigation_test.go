package ui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
	sectionmodel "github.com/richardrh/lazymagit/internal/model"
)

func navigationUIModel() *Model {
	m := New(nil)
	m.loading = false

	m.width, m.height = 100, 18
	m.install(snapshot{
		summary: gitbackend.Summary{Branch: "main", Head: "0123456789abcdef", Upstream: "origin/main", Ahead: 2, Behind: 1},
		status: gitbackend.Status{Files: []gitbackend.FileStatus{
			{Path: "one.txt", Unstaged: gitbackend.ChangeModified},
			{Path: "two.txt", Staged: gitbackend.ChangeModified},
		}},
		stashes: []gitbackend.Stash{{Ref: "stash@{0}", ID: "stash-one", ShortID: "stash01", Subject: "On main: saved"}},
		recent:  []gitbackend.Commit{{ID: "abcdef0123456789", ShortID: "abcdef0", Subject: "subject"}},
	})
	return m
}

func TestDoomSectionNavigationPreservesJKMovement(t *testing.T) {
	m := navigationUIModel()
	unstaged := sectionmodel.SectionID("status/unstaged")
	m.tree.SetCursor("status/unstaged/file/one.txt")
	_, _ = m.Update(keyMsg("g"))
	_, _ = m.Update(keyMsg("h"))
	if m.tree.Cursor() != unstaged {
		t.Fatalf("gh cursor = %q", m.tree.Cursor())
	}
	_, _ = m.Update(keyMsg("z"))
	_, _ = m.Update(keyMsg("c"))
	if !m.tree.IsFolded(unstaged) {
		t.Fatal("zc did not close the section")
	}
	_, _ = m.Update(keyMsg("z"))
	_, _ = m.Update(keyMsg("c"))
	if !m.tree.IsFolded(unstaged) {
		t.Fatal("repeating zc must not reopen the section")
	}
	_, _ = m.Update(keyMsg("z"))
	_, _ = m.Update(keyMsg("o"))
	if m.tree.IsFolded(unstaged) {
		t.Fatal("zo did not open the section")
	}
	_, _ = m.Update(keyMsg("z"))
	_, _ = m.Update(keyMsg("o"))
	if m.tree.IsFolded(unstaged) {
		t.Fatal("repeating zo must not close the section")
	}
	_, _ = m.Update(keyMsg("]"))
	if m.tree.Cursor() != "status/staged" {
		t.Fatalf("] cursor = %q", m.tree.Cursor())
	}
	before := m.tree.Cursor()
	_, _ = m.Update(keyMsg("j"))
	if m.tree.Cursor() == before || m.resolver.ActiveTransient() != "" {
		t.Fatal("j must move down, not open a transient")
	}
	_, _ = m.Update(keyMsg("k"))
	if m.tree.Cursor() != before || m.mode == modeConfirm {
		t.Fatal("k must move up, not discard")
	}
}

func TestDepthGlobalCycleVisitAndInformationRender(t *testing.T) {
	m := navigationUIModel()
	m.tree.SetCursor("status/unstaged")
	_, _ = m.Update(keyMsg("z"))
	_, _ = m.Update(keyMsg("1"))
	if !m.tree.IsFolded("status/unstaged") {
		t.Fatal("z1 did not collapse sections")
	}
	_, _ = m.Update(keyMsg("z"))
	_, _ = m.Update(keyMsg("2"))
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
	if _, handled := m.handleNavigationKey(keyMsg("=")); !handled || strings.Contains(m.detail, " before") || strings.Contains(m.detail, " after") {
		t.Fatalf("less-context output = %q", m.detail)
	}

	m.tree.SetCursor("status/unstaged/file/one.txt")
	_, _ = m.Update(keyMsg("y"))
	_, cmd := m.Update(keyMsg("s"))
	if cmd == nil || clipboardPayload(cmd) != "one.txt" || !strings.Contains(m.message, "OSC52") {
		t.Fatalf("section copy payload=%q message=%q", clipboardPayload(cmd), m.message)
	}
	_, _ = m.Update(keyMsg("y"))
	_, cmd = m.Update(keyMsg("b"))
	if cmd == nil || clipboardPayload(cmd) != "0123456789abcdef" {
		t.Fatalf("revision copy payload=%q", clipboardPayload(cmd))
	}
}

func TestVisitTerminalRowsAndCycleSelectedDetail(t *testing.T) {
	m := navigationUIModel()
	m.tree.SetCursor("status/unstaged/file/one.txt")
	m.detail = "diff --git a/one.txt b/one.txt\n@@ -1 +1 @@\n-old\n+new"

	if _, handled := m.handleNavigationKey(keyMsg("enter")); !handled || m.detailHidden || !strings.Contains(m.message, "Opened") {
		t.Fatalf("terminal visit handled=%v hidden=%v message=%q", handled, m.detailHidden, m.message)
	}
	if _, handled := m.handleNavigationKey(keyMsg("alt+tab")); !handled || !m.detailHidden || !strings.Contains(m.renderDetailPanel(60, 10), "Diff hidden") {
		t.Fatalf("detail cycle did not hide selected terminal diff: hidden=%v", m.detailHidden)
	}
	if _, handled := m.handleNavigationKey(keyMsg("alt+tab")); !handled || m.detailHidden {
		t.Fatalf("detail cycle did not restore selected terminal diff: hidden=%v", m.detailHidden)
	}

	m.tree.SetCursor("status/unstaged")
	if _, handled := m.handleNavigationKey(keyMsg("alt+tab")); !handled || !strings.Contains(m.message, "No terminal diff") {
		t.Fatalf("heading detail cycle was unsafe: %q", m.message)
	}
}

func TestEditBrowseAndNextReferenceStayInsideTerminalStatus(t *testing.T) {
	m := New(nil)
	m.loading = false

	m.showCommit = func(_ context.Context, id string) (string, error) {
		return "commit " + id + "\n\nterminal detail", nil
	}
	m.install(snapshot{recent: []gitbackend.Commit{
		{ID: "plain", Subject: "plain"},
		{ID: "tagged", Refs: "tag: v1", Subject: "tagged"},
		{ID: "tracked", Refs: "origin/main", Subject: "tracked"},
	}})
	m.tree.ToggleFold("status/recent")
	m.tree.SetCursor("status/recent/commit/plain")

	pressControlSequence := func(key rune) tea.Cmd {
		t.Helper()
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		if cmd != nil || m.resolver.PendingPrefix() != "ctrl+c" {
			t.Fatalf("C-c prefix cmd=%v pending=%q", cmd != nil, m.resolver.PendingPrefix())
		}
		_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: key, Mod: tea.ModCtrl}))
		return cmd
	}

	for key, action := range map[rune]string{'e': "Edit", 'o': "Browse"} {
		m.tree.SetCursor("status/recent/commit/plain")
		cmd := pressControlSequence(key)
		if cmd == nil || !strings.Contains(m.message, action+" opened selected item in terminal detail (read-only)") || m.busy || m.mode != modeStatus {
			t.Fatalf("C-c C-%c cmd=%v message=%q busy=%v mode=%d", key, cmd != nil, m.message, m.busy, m.mode)
		}
		_, _ = m.Update(cmd())
		if !strings.Contains(m.detail, "commit plain") {
			t.Fatalf("C-c C-%c did not load internal detail: %q", key, m.detail)
		}
	}

	m.tree.SetCursor("status/recent/commit/plain")
	if cmd := pressControlSequence('r'); cmd == nil || m.tree.Cursor() != "status/recent/commit/tagged" || !strings.Contains(m.message, "tag: v1") {
		t.Fatalf("first C-c C-r cmd=%v cursor=%q message=%q", cmd != nil, m.tree.Cursor(), m.message)
	}
	if cmd := pressControlSequence('r'); cmd == nil || m.tree.Cursor() != "status/recent/commit/tracked" || !strings.Contains(m.message, "origin/main") {
		t.Fatalf("second C-c C-r cmd=%v cursor=%q message=%q", cmd != nil, m.tree.Cursor(), m.message)
	}
	if cmd := pressControlSequence('r'); cmd != nil || m.tree.Cursor() != "status/recent/commit/tracked" || m.message != "No more visible references" {
		t.Fatalf("terminal reference end cmd=%v cursor=%q message=%q", cmd != nil, m.tree.Cursor(), m.message)
	}

	m.tree.SetCursor("status/recent")
	m.tree.ToggleFold("status/recent")
	if cmd, handled := m.performNavigationCommand(keymap.CommandNextReference); !handled || cmd != nil || m.message != "No more visible references" {
		t.Fatalf("folded references handled=%v cmd=%v message=%q", handled, cmd != nil, m.message)
	}
}

func TestDiffContextKeysReloadSelectedFileDiff(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("notes.txt", "zero\none\ntwo\nthree\nfour\nfive\nsix\n")
	r.git("add", "notes.txt")
	r.git("commit", "-m", "base")
	r.write("notes.txt", "zero\none\ntwo\nCHANGED\nfour\nfive\nsix\n")
	m := newE2EModel(t, r)
	selectE2EPath(t, m, "notes.txt", rowUnstaged)

	sendE2EKey(t, m, keyMsg("+"))
	if m.diffContext != 6 || !strings.Contains(m.detail, " four") || !strings.Contains(m.message, "6 lines") {
		t.Fatalf("more context=%d detail=%q message=%q", m.diffContext, m.detail, m.message)
	}
	sendE2EKey(t, m, keyMsg("="))
	if m.diffContext != defaultDiffContext || !strings.Contains(m.message, "3 lines") {
		t.Fatalf("less context=%d message=%q", m.diffContext, m.message)
	}
	sendE2EKey(t, m, keyMsg("+"))
	sendE2EKey(t, m, keyMsg("g"))
	sendE2EKey(t, m, keyMsg("="))
	if m.diffContext != defaultDiffContext || !strings.Contains(m.message, "Default diff context") {
		t.Fatalf("default context=%d message=%q", m.diffContext, m.message)
	}
}

func TestStatusJumpTransientRoutesExactProjectedSections(t *testing.T) {
	for key, section := range map[string]string{"n": "status/untracked", "u": "status/unstaged", "s": "status/staged", "fu": "status/unpulled", "pu": "status/unpushed"} {
		m := navigationUIModel()
		m.repo = &gitbackend.Repository{}

		_, _ = m.Update(keyMsg("g"))
		for _, token := range strings.Split(key, "") {
			_, _ = m.Update(keyMsg(token))
		}
		sectionID := sectionmodel.SectionID(section)
		if m.tree.Section(sectionID) == nil {
			if !strings.Contains(m.message, "not present") {
				t.Errorf("%s missing section message = %q", key, m.message)
			}
			continue
		}
		if got := string(m.tree.Cursor()); got != section {
			t.Errorf("%s cursor = %q, want %q", key, got, section)
		}
	}
}

func TestMultiKeyNavigationCommandsHaveDispatcherTargets(t *testing.T) {
	m := navigationUIModel()
	m.tree.SetCursor("status/unstaged")
	if _, handled := m.performNavigationCommand(keymap.CommandSectionCycle); !handled || !m.tree.IsFolded("status/unstaged") {
		t.Fatal("C-c TAB command target did not cycle section")
	}
	m.tree.ToggleFold("status/unstaged")
	m.tree.SetCursor("status/unstaged/file/one.txt")
	cmd, handled := m.performNavigationCommand(keymap.CommandCopyThing)
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
