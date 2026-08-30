package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func routingKeyMsg(key string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: rune(key[0]), Text: key})
}

func TestHandleWindowSizeMsgClampsViewState(t *testing.T) {
	m := New(nil)
	m.detail = "one\ntwo\nthree\nfour\nfive\nsix"
	m.detailOffset = 99
	m.transientOffset = 99
	m.processOffset = 99

	if cmd := m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 12}); cmd != nil {
		t.Fatal("window size helper returned a command")
	}
	if m.width != 80 || m.height != 12 {
		t.Fatalf("size = %dx%d", m.width, m.height)
	}
	if m.detailOffset > m.detailMaximumOffset() {
		t.Fatalf("detail offset was not clamped: %d", m.detailOffset)
	}
}

func TestHandleDiffMsgRoutesCurrentAndRejectsStaleResults(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.detailRequest = 4
	id := m.tree.Cursor()

	m.handleDiffMsg(diffMsg{request: 3, id: id, text: "stale"})
	if m.detail == "stale" {
		t.Fatal("stale diff was installed")
	}
	m.handleDiffMsg(diffMsg{request: 4, id: id, text: "current"})
	if m.detail != "current" || m.detailID != id {
		t.Fatalf("current diff detail=%q id=%q", m.detail, m.detailID)
	}
	m.handleDiffMsg(diffMsg{request: 4, id: id, err: errors.New("boom\nunsafe")})
	want := "Unable to load diff:\n" + sanitizeSingleLine("boom\nunsafe")
	if m.detail != want {
		t.Fatalf("sanitized error detail = %q, want %q", m.detail, want)
	}
}

func TestHandleModeKeyRoutesAndPassesThrough(t *testing.T) {
	m := New(nil)
	m.mode = modeCommit
	m.input = "message"
	if _, handled := m.handleModeKey(routingKeyMsg("esc"), "esc"); !handled {
		t.Fatal("commit mode key was not routed")
	}
	if m.mode != modeStatus || m.input != "" || m.message != "Commit cancelled" {
		t.Fatalf("commit cancellation mode=%d input=%q message=%q", m.mode, m.input, m.message)
	}
	if _, handled := m.handleModeKey(routingKeyMsg("x"), "x"); handled {
		t.Fatal("status mode key should pass through")
	}
}

func TestCommitEditingHelpersHandleUnicodeAndEmptyInput(t *testing.T) {
	m := New(nil)
	m.appendCommitText("hé")
	m.appendCommitText("")
	m.deleteCommitRune()
	if m.input != "h" {
		t.Fatalf("input after unicode delete = %q", m.input)
	}
	m.deleteCommitRune()
	m.deleteCommitRune()
	if m.input != "" {
		t.Fatalf("empty delete changed input to %q", m.input)
	}
}

func TestSubmitCommitHelperValidatesAndStartsOperation(t *testing.T) {
	m := New(nil)
	m.loading = false
	if cmd := m.submitCommit(); cmd != nil || !m.isError {
		t.Fatalf("empty commit cmd=%v error=%v", cmd != nil, m.isError)
	}

	m.isError = false
	m.input = "  subject  "
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
	if cmd := m.submitCommit(); cmd == nil || m.mode != modeStatus || m.input != "" || !m.busy {
		t.Fatalf("submit cmd=%v mode=%d input=%q busy=%v", cmd != nil, m.mode, m.input, m.busy)
	}
}

func TestMoveBranchCursorClampsAtBothEnds(t *testing.T) {
	m := New(nil)
	m.branches = []gitbackend.Branch{{Name: "one"}, {Name: "two"}}
	m.moveBranchCursor(-1)
	if m.branchCursor != 0 {
		t.Fatalf("cursor below start = %d", m.branchCursor)
	}
	m.moveBranchCursor(1)
	m.moveBranchCursor(1)
	if m.branchCursor != 1 {
		t.Fatalf("cursor past end = %d", m.branchCursor)
	}
	m.branches = nil
	m.moveBranchCursor(1)
	if m.branchCursor != 0 {
		t.Fatalf("empty branch cursor = %d", m.branchCursor)
	}
}
