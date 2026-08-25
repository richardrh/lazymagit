package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestVimGTimeoutRefreshesAndGGStillNavigatesFirst(t *testing.T) {
	m := New(nil)
	m.loading = false
	last := len(m.tree.VisibleSectionIDs()) - 1
	_, _ = m.Update(keyMsg("G"))
	if m.tree.Cursor() != m.tree.VisibleSectionIDs()[last] || m.busy {
		t.Fatal("Vim G should navigate to the last row without refreshing")
	}
	_, cmd := m.Update(keyMsg("g"))
	if cmd == nil || m.busy || m.resolver.PendingPrefix() != "g" {
		t.Fatal("Vim g should briefly wait for a navigation suffix")
	}
	firstTimeout := vimGTimeoutMsg{token: m.vimGToken}
	_, cmd = m.Update(keyMsg("g"))
	if m.tree.Cursor() != m.tree.VisibleSectionIDs()[0] || m.busy {
		t.Fatal("Vim gg should navigate and load detail, not refresh")
	}
	_, cmd = m.Update(firstTimeout)
	if cmd != nil || m.busy {
		t.Fatal("stale g timeout refreshed after gg had consumed the prefix")
	}

	_, _ = m.Update(keyMsg("g"))
	current := vimGTimeoutMsg{token: m.vimGToken}
	_, cmd = m.Update(current)
	if cmd == nil || !m.busy || m.resolver.PendingPrefix() != "" {
		t.Fatal("current single-g timeout did not refresh")
	}
}

func TestKeySchemeToggleAndCollisions(t *testing.T) {
	m := New(nil)
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{
		{Path: "unstaged", Unstaged: gitbackend.ChangeModified},
		{Path: "staged", Staged: gitbackend.ChangeModified},
	}}})
	m.loading = false
	selectPath := func(path string) {
		for id, r := range m.rows {
			if r.path == path {
				m.tree.SetCursor(id)
				return
			}
		}
		t.Fatalf("missing row for %q", path)
	}

	selectPath("unstaged")
	_, _ = m.Update(keyMsg("k"))
	if m.mode == modeConfirm {
		t.Fatal("Vim k discarded instead of navigating")
	}
	selectPath("unstaged")
	_, _ = m.Update(keyMsg("x"))
	if m.mode != modeConfirm || m.confirmPath != "unstaged" {
		t.Fatal("Vim x did not retain convenience discard")
	}
	m.setMode(modeStatus)
	m.confirmPath = ""

	_, _ = m.Update(keyMsg("f2"))
	if m.scheme != schemeMagit {
		t.Fatal("F2 did not enable Magit scheme")
	}
	selectPath("staged")
	_, _ = m.Update(keyMsg("k"))
	if m.mode != modeConfirm || m.confirmPath != "staged" {
		t.Fatal("Magit k did not make staged-only whole-file discard reachable")
	}
	m.setMode(modeStatus)
	m.confirmPath = ""
	selectPath("unstaged")
	_, cmd := m.Update(keyMsg("x"))
	if cmd != nil || m.mode == modeConfirm || m.confirmPath != "" {
		t.Fatal("Magit x must never discard")
	}

	for _, key := range []string{"g", "G"} {
		m.busy = false
		_, cmd = m.Update(keyMsg(key))
		if cmd == nil || !m.busy || m.resolver.PendingPrefix() != "" {
			t.Fatalf("Magit %s did not refresh immediately", key)
		}
	}

	_, _ = m.Update(keyMsg("f2"))
	if m.scheme != schemeVim {
		t.Fatal("second F2 did not restore Vim scheme")
	}
}

func TestCommandPrefixWaitsAndUnmatchedSuffixIsReplayed(t *testing.T) {
	m := New(nil)
	_, cmd := m.Update(keyMsg("c"))
	if cmd != nil || m.resolver.PendingPrefix() != "c" {
		t.Fatal("commit prefix should wait for its suffix without a timer")
	}
	before := m.tree.Cursor()
	_, _ = m.Update(keyMsg("j"))
	if m.resolver.PendingPrefix() != "" {
		t.Fatal("unmatched suffix did not cancel command prefix")
	}
	if m.tree.Cursor() == before {
		t.Fatal("unmatched j suffix was not replayed as navigation")
	}
}

func TestStaleDiffRequestCannotReplaceNewerSnapshotDetail(t *testing.T) {
	m := New(nil)
	s := snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "file", Unstaged: gitbackend.ChangeModified}}}}
	m.install(s)
	_ = m.moveTo(1)
	oldRequest := m.detailRequest
	id := m.tree.Cursor()

	m.install(s)
	_ = m.loadDetailCmd()
	want := m.detail
	_, _ = m.Update(diffMsg{id: id, request: oldRequest, text: "stale"})
	if m.detail != want {
		t.Fatalf("stale diff replaced newer snapshot detail: %q", m.detail)
	}
}

func TestStaleBranchResultCannotOverrideNewerState(t *testing.T) {
	m := New(nil)
	m.busy = true
	m.branchRequest = 7
	requestState := m.stateGeneration
	m.bumpState()

	_, _ = m.Update(branchesMsg{
		request:  7,
		state:    requestState,
		branches: []gitbackend.Branch{{Name: "stale"}},
	})
	if m.mode == modeBranches || len(m.branches) != 0 {
		t.Fatal("stale branch result opened or replaced the branch picker")
	}
	if m.busy {
		t.Fatal("completed current branch request left the model busy")
	}
}

func TestSupersededDetailLoadCancelsItsContext(t *testing.T) {
	m := New(nil)
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel

	_ = m.loadDetailCmd()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("replacing a detail load did not cancel the previous context")
	}
}

func TestQuitCancelsApplicationLifecycle(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		keyMsg("q"),
		tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}),
	} {
		m := New(nil)
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatal("quit key did not return a command")
		}
		select {
		case <-m.appCtx.Done():
		default:
			t.Fatalf("%s did not cancel application context", key.String())
		}
	}
}

func keyMsg(text string) tea.KeyPressMsg {
	runes := []rune(text)
	return tea.KeyPressMsg(tea.Key{Text: text, Code: runes[0]})
}
