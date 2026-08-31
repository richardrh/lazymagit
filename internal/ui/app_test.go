package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

func TestDiscardConfirmationHelpers(t *testing.T) {
	for key, want := range map[string]discardConfirmationAction{
		"y": discardConfirmationAccepted, "Y": discardConfirmationAccepted,
		"n": discardConfirmationCancelled, "N": discardConfirmationCancelled,
		"q": discardConfirmationCancelled, "esc": discardConfirmationCancelled,
		"other": discardConfirmationIgnored,
	} {
		if got := discardConfirmationActionForKey(key); got != want {
			t.Errorf("discardConfirmationActionForKey(%q) = %v, want %v", key, got, want)
		}
	}

	original := []string{"one", "two"}
	confirmed := confirmedDiscardPaths(original, "fallback")
	original[0] = "changed"
	if !reflect.DeepEqual(confirmed, []string{"one", "two"}) {
		t.Fatalf("confirmed paths were not copied: %v", confirmed)
	}
	if got := confirmedDiscardPaths(nil, "fallback"); !reflect.DeepEqual(got, []string{"fallback"}) {
		t.Fatalf("fallback confirmed paths = %v", got)
	}
	if got := confirmedDiscardPaths(nil, ""); got != nil {
		t.Fatalf("empty confirmed paths = %#v", got)
	}
}

func TestHandleConfirmKeyRoutesEveryAction(t *testing.T) {
	m := New(nil)
	m.setMode(modeConfirm)
	m.confirmPath = "one"
	if cmd := m.handleConfirmKey("other"); cmd != nil || m.mode != modeConfirm || m.confirmPath != "one" {
		t.Fatalf("ignored confirmation changed state: mode=%v path=%q cmd=%v", m.mode, m.confirmPath, cmd)
	}
	if cmd := m.handleConfirmKey("n"); cmd != nil || m.mode != modeStatus || m.confirmPath != "" || m.message != "Discard cancelled" {
		t.Fatalf("cancel confirmation state: mode=%v path=%q message=%q cmd=%v", m.mode, m.confirmPath, m.message, cmd)
	}

	m.setMode(modeConfirm)
	m.confirmPaths = []string{"one", "two"}
	if cmd := m.handleConfirmKey("y"); cmd == nil || m.mode != modeStatus || len(m.confirmPaths) != 0 || !m.busy {
		t.Fatalf("accepted confirmation state: mode=%v paths=%v busy=%v cmd=%v", m.mode, m.confirmPaths, m.busy, cmd)
	}
}

func TestVimGTimeoutRefreshesAndGGStillNavigatesFirst(t *testing.T) {
	m := New(nil)
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "a", Unstaged: gitbackend.ChangeModified}, {Path: "b", Staged: gitbackend.ChangeModified}}}})
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

func TestStageAllAndUnstageAllDirectActionsUseInjectedCallbacks(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "tracked", Unstaged: gitbackend.ChangeModified}}}})
	var staged, unstaged int
	m.stageAll = func(context.Context) error { staged++; return nil }
	m.unstageAll = func(context.Context) error { unstaged++; return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
	for key, wantName := range map[string]string{"S": "stage all tracked changes", "U": "unstage all"} {
		m.busy = false
		_, cmd := m.Update(keyMsg(key))
		if cmd == nil || !m.busy {
			t.Fatalf("%s did not start aggregate operation", key)
		}
		msg := cmd().(operationMsg)
		if msg.name != wantName || msg.opErr != nil {
			t.Fatalf("%s operation = %+v", key, msg)
		}
	}
	if staged != 1 || unstaged != 1 {
		t.Fatalf("aggregate callback calls stage=%d unstage=%d", staged, unstaged)
	}
}

func TestRealShiftSStagesTrackedChangesAndReportsNoOp(t *testing.T) {
	shiftS := tea.KeyPressMsg(tea.Key{Code: 's', Text: "S", Mod: tea.ModShift})
	m := New(nil)
	m.loading = false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{
		{Path: "tracked", Unstaged: gitbackend.ChangeModified},
		{Path: "new", Unstaged: gitbackend.ChangeUntracked},
	}}})
	called := 0
	m.stageAll = func(context.Context) error { called++; return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }

	_, cmd := m.Update(shiftS)
	if cmd == nil || !m.busy {
		t.Fatal("physical Shift-S did not start stage-modified operation")
	}
	msg := cmd().(operationMsg)
	if msg.name != "stage all tracked changes" || msg.opErr != nil || called != 1 {
		t.Fatalf("Shift-S operation = %+v, calls=%d", msg, called)
	}

	noOp := New(nil)
	noOp.loading = false
	noOp.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "new", Unstaged: gitbackend.ChangeUntracked}}}})
	noOp.stageAll = func(context.Context) error { return errNoTrackedUnstagedChanges }
	noOp.snapshotLoader = func(context.Context) (snapshot, error) { return noOp.snapshot, nil }
	_, cmd = noOp.Update(shiftS)
	if cmd == nil {
		t.Fatal("stage-modified no-op did not refresh repository state")
	}
	_, _ = noOp.Update(cmd())
	if noOp.isError || noOp.message != "No tracked unstaged changes to stage" {
		t.Fatalf("stage-modified no-op error=%v message=%q", noOp.isError, noOp.message)
	}
}

func TestExecutableNilCallbackReportsInternalError(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "tracked", Unstaged: gitbackend.ChangeModified}}}})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "S", Mod: tea.ModShift}))
	if cmd != nil || m.busy || !m.isError || !strings.Contains(m.message, "internal error") || !strings.Contains(m.message, "stage all tracked changes") {
		t.Fatalf("nil callback cmd=%v busy=%v error=%v message=%q", cmd != nil, m.busy, m.isError, m.message)
	}
}

func TestUnhandledProcessKeyClosesPaneAndPassesThroughShiftS(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.mode = modeProcess
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "tracked", Unstaged: gitbackend.ChangeModified}}}})
	m.stageAll = func(context.Context) error { return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "S", Mod: tea.ModShift}))
	if cmd == nil || m.mode != modeStatus || !m.busy {
		t.Fatalf("process pass-through cmd=%v mode=%d busy=%v", cmd != nil, m.mode, m.busy)
	}
}

func TestCommandPrefixWaitsAndUnknownSuffixStaysInTransient(t *testing.T) {
	m := New(nil)
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "a", Unstaged: gitbackend.ChangeModified}, {Path: "b", Staged: gitbackend.ChangeModified}}}})
	_, cmd := m.Update(keyMsg("c"))
	if cmd != nil || m.resolver.PendingPrefix() != "c" {
		t.Fatal("commit prefix should wait for its suffix without a timer")
	}
	before := m.tree.Cursor()
	_, _ = m.Update(keyMsg("j"))
	if m.resolver.PendingPrefix() != "c" {
		t.Fatal("unknown suffix closed command transient")
	}
	if m.tree.Cursor() != before || !strings.Contains(m.message, "not implemented") {
		t.Fatal("unknown suffix was replayed as navigation")
	}
}

func TestPerformAppliesDirectDisplayCommands(t *testing.T) {
	m := New(nil)
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "file", Unstaged: gitbackend.ChangeModified}}}})
	m.loading = false
	m.perform(keymap.CommandOpenDispatcher)
	if m.mode != modeHelp {
		t.Fatalf("dispatcher mode = %d", m.mode)
	}
	m.setMode(modeStatus)
	m.perform(keymap.CommandDepth1)
	if m.tree.SetCursor("status/unstaged/file/file") {
		t.Fatal("depth one retained a file row")
	}
	m.perform(keymap.CommandToggleSection)
	if !m.tree.IsFolded(m.tree.Cursor()) {
		t.Fatal("toggle section did not fold current section")
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

func TestCommitDetailIsAsyncAndStaleRevisionCannotWin(t *testing.T) {
	m := New(nil)
	m.showCommit = func(_ context.Context, id string) (string, error) {
		return "commit " + id + "\nAuthor: Test\n file.txt | 1 +\n\ndiff --git a/file.txt b/file.txt\n+line", nil
	}
	m.install(snapshot{recent: []gitbackend.Commit{{ID: "one", Subject: "first"}, {ID: "two", Subject: "second"}}})
	// Initial recent folding is intentional; open it before selecting a commit.
	m.tree.ToggleFold("status/recent")
	m.tree.SetCursor("status/recent/commit/one")
	cmd := m.loadDetailCmd()
	if cmd == nil || m.detail != "Loading commit/revision…" {
		t.Fatalf("commit detail was not asynchronous: cmd=%v detail=%q", cmd != nil, m.detail)
	}
	_, _ = m.Update(cmd())
	for _, want := range []string{"commit one", "Author: Test", "file.txt | 1 +", "diff --git", "+line"} {
		if !strings.Contains(m.detail, want) {
			t.Errorf("revision detail omitted %q: %q", want, m.detail)
		}
	}
	oldRequest := m.detailRequest
	m.tree.SetCursor("status/recent/commit/two")
	_ = m.loadDetailCmd()
	_, _ = m.Update(diffMsg{id: "status/recent/commit/one", request: oldRequest, text: "stale revision", revision: true})
	if strings.Contains(m.detail, "stale revision") {
		t.Fatal("stale revision result replaced current detail")
	}
	_, _ = m.Update(diffMsg{id: m.tree.Cursor(), request: m.detailRequest, err: errors.New("bad"), revision: true})
	if !strings.Contains(m.detail, "commit/revision") || strings.Contains(m.detail, "diff") {
		t.Fatalf("revision error wording = %q", m.detail)
	}
}

func TestStashDetailIsAsyncUsesOIDAndStaleResultCannotWin(t *testing.T) {
	m := New(nil)
	var requested string
	m.showStash = func(_ context.Context, id string) (gitbackend.StashDetails, error) {
		requested = id
		return gitbackend.StashDetails{Stash: gitbackend.Stash{ID: id, ShortID: "1234567", Subject: "subject", Author: "Test"}, Patch: "diff --git a/file b/file\n+stashed"}, nil
	}
	m.install(snapshot{stashes: []gitbackend.Stash{{Ref: "stash@{0}", ID: "one", ShortID: "1234567", Subject: "subject"}, {Ref: "stash@{1}", ID: "two", ShortID: "7654321", Subject: "older"}}})
	m.tree.ToggleFold("status/stashes")
	m.tree.SetCursor("status/stashes/stash/one")
	cmd := m.loadDetailCmd()
	if cmd == nil || m.detail != "Loading stash…" {
		t.Fatalf("stash detail was not asynchronous: cmd=%v detail=%q", cmd != nil, m.detail)
	}
	_, _ = m.Update(cmd())
	if requested != "one" || !strings.Contains(m.detail, "1234567  one") || !strings.Contains(m.detail, "+stashed") {
		t.Fatalf("stash detail requested=%q detail=%q", requested, m.detail)
	}
	oldRequest := m.detailRequest
	m.tree.SetCursor("status/stashes/stash/two")
	_ = m.loadDetailCmd()
	_, _ = m.Update(diffMsg{id: "status/stashes/stash/one", request: oldRequest, text: "stale stash", stash: true})
	if strings.Contains(m.detail, "stale stash") {
		t.Fatal("stale stash result replaced current detail")
	}
	_, _ = m.Update(diffMsg{id: m.tree.Cursor(), request: m.detailRequest, err: errors.New("bad"), stash: true})
	if !strings.Contains(m.detail, "Unable to load stash") {
		t.Fatalf("stash error wording = %q", m.detail)
	}
}

func TestInitialMagitFoldsAndLaterUserFoldIsPreserved(t *testing.T) {
	m := New(nil)
	s := snapshot{
		status:   gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "u", Unstaged: gitbackend.ChangeUntracked}, {Path: "m", Unstaged: gitbackend.ChangeModified}, {Path: "s", Staged: gitbackend.ChangeModified}}},
		stashes:  []gitbackend.Stash{{ID: "stash", ShortID: "stash01"}},
		recent:   []gitbackend.Commit{{ID: "recent"}},
		upstream: gitbackend.UpstreamRanges{Ahead: []gitbackend.Commit{{ID: "ahead"}}, Behind: []gitbackend.Commit{{ID: "behind"}}},
		summary:  gitbackend.Summary{Upstream: "origin/main", Ahead: 1, Behind: 1},
	}
	m.install(s)
	for _, id := range []sectionmodel.SectionID{"status/untracked", "status/stashes", "status/unpulled"} {
		if !m.tree.IsFolded(id) {
			t.Errorf("%s should initially be folded", id)
		}
	}
	for _, id := range []sectionmodel.SectionID{"status/unstaged", "status/staged", "status/unpushed"} {
		if m.tree.IsFolded(id) {
			t.Errorf("%s should initially be expanded", id)
		}
	}
	for _, id := range []sectionmodel.SectionID{"status/unstaged", "status/untracked"} {
		m.tree.SetCursor(id)
		_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	// Preferences survive more than one disappearance/reappearance cycle.
	m.install(snapshot{})
	m.install(s)
	m.install(snapshot{})
	m.install(s)
	if !m.tree.IsFolded("status/unstaged") {
		t.Fatal("refresh lost user fold")
	}
	if m.tree.IsFolded("status/untracked") {
		t.Fatal("refresh reapplied the default after the user expanded untracked")
	}
	fallback := New(nil)
	fallback.install(snapshot{})
	fallback.install(snapshot{recent: []gitbackend.Commit{{ID: "recent"}}})
	if !fallback.tree.IsFolded("status/recent") {
		t.Fatal("recent fallback should be folded when it first appears")
	}
}

func TestAddRemoteModalEditingAndFetchTrueSubmission(t *testing.T) {
	m := New(nil)
	m.loading = false
	var gotName, gotURL string
	var gotFetch bool
	m.addRemote = func(_ context.Context, name, url string, fetch bool) error {
		gotName, gotURL, gotFetch = name, url, fetch
		return nil
	}
	_, _ = m.Update(keyMsg("M"))
	_, _ = m.Update(keyMsg("a"))
	if m.mode != modeAddRemote || m.remoteFetch || m.remoteField != 0 {
		t.Fatalf("M a state = mode %d fetch %v field %d", m.mode, m.remoteFetch, m.remoteField)
	}
	_, _ = m.Update(keyMsg("é"))
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.remoteInput[0] != "" {
		t.Fatalf("UTF-8 backspace left %q", m.remoteInput[0])
	}
	for _, r := range "origin" {
		_, _ = m.Update(keyMsg(string(r)))
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	for _, r := range "https://example.test/repo" {
		_, _ = m.Update(keyMsg(string(r)))
	}
	_, _ = m.Update(keyMsg("ctrl+f"))
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil || m.mode != modeStatus {
		t.Fatal("remote submission did not close modal/start operation")
	}
	msg := cmd()
	if operation, ok := msg.(operationMsg); !ok || operation.opErr != nil {
		t.Fatalf("remote operation result = %#v", msg)
	}
	if gotName != "origin" || gotURL != "https://example.test/repo" || !gotFetch {
		t.Fatalf("AddRemote called with %q %q fetch=%v", gotName, gotURL, gotFetch)
	}
}

func TestExactFetchActionsAndFFDoesNothing(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.snapshot.pushRemote = "publish"
	var upstreamCalls, pushCalls int
	m.fetchUpstream = func(context.Context) error { upstreamCalls++; return nil }
	m.fetchPush = func(context.Context) error { pushCalls++; return nil }
	m.fetchAll = func(context.Context) error { return nil }
	for _, suffix := range []string{"u", "p", "e", "a"} {
		m.busy = false
		_, _ = m.Update(keyMsg("f"))
		_, cmd := m.Update(keyMsg(suffix))
		if suffix == "e" { // no remotes: chooser reports a safe error instead of starting Git
			if cmd != nil || !m.isError {
				t.Fatalf("f e without remotes was not handled safely")
			}
			continue
		}
		if cmd == nil || !m.busy {
			t.Fatalf("f %s did not start an operation", suffix)
		}
		if suffix == "u" || suffix == "p" {
			_ = cmd()
		}
	}
	if upstreamCalls != 1 || pushCalls != 1 {
		t.Fatalf("dedicated fetch resolvers called upstream=%d push=%d", upstreamCalls, pushCalls)
	}
	m.busy = false
	_, _ = m.Update(keyMsg("f"))
	_, cmd := m.Update(keyMsg("f"))
	if cmd != nil || m.busy || m.resolver.PendingPrefix() != "" {
		t.Fatal("f f must be unbound")
	}

	chooser := New(nil)
	chooser.loading = false
	chooser.install(snapshot{remotes: []gitbackend.Remote{{Name: "origin"}, {Name: "backup"}}})
	var fetched string
	chooser.fetch = func(_ context.Context, remote ...string) error { fetched = remote[0]; return nil }
	_, _ = chooser.Update(keyMsg("f"))
	_, cmd = chooser.Update(keyMsg("e"))
	if cmd != nil || chooser.mode != modeRemotes {
		t.Fatal("f e did not open the remote chooser")
	}
	_, _ = chooser.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, cmd = chooser.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("remote chooser did not submit")
	}
	_ = cmd()
	if fetched != "backup" {
		t.Fatalf("chooser fetched %q", fetched)
	}
}

func TestSnapshotLoadUsesMagitLimitsPushRemoteAndTruncationProbe(t *testing.T) {
	var recentLimit, upstreamLimit, stashCalls int
	commits := make([]gitbackend.Commit, 257)
	for i := range commits {
		commits[i].ID = fmt.Sprintf("%040d", i)
	}
	s, err := loadSnapshotWith(context.Background(), snapshotQueries{
		summary: func(context.Context) (gitbackend.Summary, error) {
			return gitbackend.Summary{Upstream: "origin/main", Ahead: 300}, nil
		},
		status: func(context.Context) (gitbackend.Status, error) { return gitbackend.Status{}, nil },
		stashes: func(context.Context) ([]gitbackend.Stash, error) {
			stashCalls++
			return []gitbackend.Stash{{ID: "stash"}}, nil
		},
		remotes: func(context.Context) ([]gitbackend.Remote, error) { return nil, nil },
		recentLog: func(_ context.Context, limit int) ([]gitbackend.Commit, error) {
			recentLimit = limit
			return nil, nil
		},
		upstreamLogLimit: func(_ context.Context, limit int) (gitbackend.UpstreamRanges, error) {
			upstreamLimit = limit
			return gitbackend.UpstreamRanges{Ahead: commits}, nil
		},
		pushRemote: func(context.Context) (string, error) { return "publish", nil },
	})
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if recentLimit != 10 || upstreamLimit != 257 || stashCalls != 1 || len(s.stashes) != 1 || s.pushRemote != "publish" {
		t.Fatalf("load calls recent=%d upstream=%d stashes=%d retained=%d push=%q", recentLimit, upstreamLimit, stashCalls, len(s.stashes), s.pushRemote)
	}
	if len(s.upstream.Ahead) != 256 || !s.aheadTruncated {
		t.Fatalf("probe normalization retained=%d truncated=%v", len(s.upstream.Ahead), s.aheadTruncated)
	}
}

func TestSnapshotLoadWrapsStashErrors(t *testing.T) {
	_, err := loadSnapshotWith(context.Background(), snapshotQueries{
		summary:   func(context.Context) (gitbackend.Summary, error) { return gitbackend.Summary{}, nil },
		status:    func(context.Context) (gitbackend.Status, error) { return gitbackend.Status{}, nil },
		stashes:   func(context.Context) ([]gitbackend.Stash, error) { return nil, errors.New("bad stash list") },
		remotes:   func(context.Context) ([]gitbackend.Remote, error) { return nil, nil },
		recentLog: func(context.Context, int) ([]gitbackend.Commit, error) { return nil, nil },
		pushRemote: func(context.Context) (string, error) {
			return "", gitbackend.ErrNoFetchRemote
		},
	})
	if err == nil || !strings.Contains(err.Error(), "stashes: bad stash list") {
		t.Fatalf("stash load error = %v", err)
	}
}

func TestAddRemotePreservesURLWhitespaceAndRejectsOnlyEmptyURL(t *testing.T) {
	m := New(nil)
	m.loading = false
	var gotName, gotURL string
	m.addRemote = func(_ context.Context, name, url string, _ bool) error { gotName, gotURL = name, url; return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
	m.mode, m.remoteField = modeAddRemote, 1
	m.remoteInput = [2]string{"  origin  ", "  ssh://example/repo  "}
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("non-empty whitespace-bearing URL was rejected")
	}
	_ = cmd()
	if gotName != "origin" || gotURL != "  ssh://example/repo  " {
		t.Fatalf("add remote received name=%q URL=%q", gotName, gotURL)
	}

	empty := New(nil)
	empty.loading, empty.mode, empty.remoteField = false, modeAddRemote, 1
	empty.remoteInput = [2]string{"origin", ""}
	if _, cmd := empty.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd != nil || !empty.isError {
		t.Fatal("truly empty URL was not rejected")
	}
}

func TestFetchPushConfiguresMissingRemoteThenFetches(t *testing.T) {
	m := New(nil)
	m.width, m.loading = 80, false
	m.install(snapshot{remotes: []gitbackend.Remote{{Name: "origin"}, {Name: "publish"}}})
	var configured string
	var order []string
	m.setPushRemote = func(_ context.Context, remote string) error {
		configured = remote
		order = append(order, "set")
		return nil
	}
	m.fetchPush = func(context.Context) error { order = append(order, "fetch"); return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }

	_, _ = m.Update(keyMsg("f"))
	_, cmd := m.Update(keyMsg("p"))
	if cmd != nil || m.mode != modeRemotes || m.remotePurpose != remoteConfigurePush {
		t.Fatal("f p missing push remote did not open configure chooser")
	}
	if overlay := m.renderOverlay(12); !strings.Contains(overlay, "Configure push remote") {
		t.Fatalf("push chooser title is not distinct: %q", overlay)
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil || !m.busy {
		t.Fatal("choosing push remote did not start operation")
	}
	msg := cmd().(operationMsg)
	if msg.opErr != nil || configured != "publish" || !reflect.DeepEqual(order, []string{"set", "fetch"}) {
		t.Fatalf("configure/fetch result err=%v configured=%q order=%v", msg.opErr, configured, order)
	}
}

func TestPushSelectsSetupChooserConfiguredRemoteOrPlainPush(t *testing.T) {
	t.Run("chooser configures and pushes without fetching", func(t *testing.T) {
		m := New(nil)
		m.width, m.loading = 80, false
		m.install(snapshot{summary: gitbackend.Summary{Branch: "main"}, remotes: []gitbackend.Remote{{Name: "origin"}, {Name: "publish"}}})
		var order []string
		m.setPushRemote = func(context.Context, string) error {
			t.Fatal("destination chooser persisted push remote before push")
			return nil
		}
		m.pushSetUpstream = func(_ context.Context, remote string) error {
			order = append(order, "push:"+remote)
			return nil
		}
		m.fetchPush = func(context.Context) error {
			order = append(order, "fetch")
			return nil
		}
		m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }

		_, _ = m.Update(keyMsg("P"))
		_, cmd := m.Update(keyMsg("p"))
		if cmd != nil || m.mode != modeRemotes || m.remotePurpose != remoteConfigureAndPush {
			t.Fatalf("P p state = mode %d purpose %d", m.mode, m.remotePurpose)
		}
		if chooser := m.renderOverlay(12); !strings.Contains(chooser, "Push and set upstream") || !strings.Contains(chooser, "Enter choose destination") {
			t.Fatalf("push destination chooser is unclear: %q", chooser)
		}
		_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if cmd == nil {
			t.Fatal("chooser selection did not start operation")
		}
		msg := cmd().(operationMsg)
		if msg.opErr != nil || !reflect.DeepEqual(order, []string{"push:publish"}) {
			t.Fatalf("setup-push result err=%v order=%v", msg.opErr, order)
		}
	})

	t.Run("configured push remote sets upstream", func(t *testing.T) {
		m := New(nil)
		m.loading = false
		m.snapshot = snapshot{summary: gitbackend.Summary{Branch: "main"}, pushRemote: "publish"}
		var setup string
		m.pushSetUpstream = func(_ context.Context, remote string) error { setup = remote; return nil }
		m.push = func(context.Context) error { t.Fatal("plain push called without upstream"); return nil }
		m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
		_, _ = m.Update(keyMsg("P"))
		_, cmd := m.Update(keyMsg("p"))
		if cmd == nil {
			t.Fatal("configured push remote did not start setup push")
		}
		_ = cmd()
		if setup != "publish" {
			t.Fatalf("setup push remote = %q", setup)
		}
	})

	t.Run("existing upstream uses plain push", func(t *testing.T) {
		m := New(nil)
		m.loading = false
		m.snapshot = snapshot{summary: gitbackend.Summary{Branch: "main", Upstream: "origin/main"}, pushRemote: "publish"}
		plainCalls := 0
		m.push = func(context.Context) error { plainCalls++; return nil }
		m.pushSetUpstream = func(context.Context, string) error { t.Fatal("setup push called with upstream"); return nil }
		m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
		_, _ = m.Update(keyMsg("P"))
		_, cmd := m.Update(keyMsg("p"))
		if cmd == nil {
			t.Fatal("plain push did not start")
		}
		_ = cmd()
		if plainCalls != 1 {
			t.Fatalf("plain push calls = %d", plainCalls)
		}
	})

	t.Run("no remotes is a clear error", func(t *testing.T) {
		m := New(nil)
		m.loading = false
		_, _ = m.Update(keyMsg("P"))
		_, cmd := m.Update(keyMsg("p"))
		if cmd != nil || !m.isError || !strings.Contains(m.message, "requires a repository") {
			t.Fatalf("no-remotes result cmd=%v error=%v message=%q", cmd != nil, m.isError, m.message)
		}
	})
}

func TestDiffResultInstallsWhileProcessPaneIsOpen(t *testing.T) {
	m := New(nil)
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "file", Unstaged: gitbackend.ChangeModified}}}})
	for id, row := range m.rows {
		if row.path == "file" {
			m.tree.SetCursor(id)
		}
	}
	m.mode = modeProcess
	m.detail = "Loading diff…"
	m.detailRequest = 9
	_, _ = m.Update(diffMsg{id: m.tree.Cursor(), request: 9, text: "+loaded"})
	if m.detail != "+loaded" {
		t.Fatalf("process mode discarded visible detail result: %q", m.detail)
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

func TestBranchResponseCancelsPrefixBeforeOpeningChooser(t *testing.T) {
	m := New(nil)
	m.loading = false
	_, _ = m.Update(keyMsg("b"))
	_, _ = m.Update(keyMsg("b"))
	request, state := m.branchRequest, m.stateGeneration
	_, _ = m.Update(keyMsg("c"))
	if m.resolver.PendingPrefix() != "c" {
		t.Fatal("commit transient did not open while branches loaded")
	}
	_, _ = m.Update(branchesMsg{request: request, state: state, branches: []gitbackend.Branch{{Name: "main"}}})
	if m.mode != modeBranches || m.resolver.PendingPrefix() != "" {
		t.Fatalf("branch response left conflicting transient: mode=%d prefix=%q", m.mode, m.resolver.PendingPrefix())
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

func TestGraphMessagesValidateRequestsAndRenderErrors(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.detailRequest = 4
	id := m.tree.Cursor()

	_, _ = m.Update(graphMsg{id: id, request: 3, text: "stale"})
	if m.detail == "stale" || m.graphActive {
		t.Fatalf("stale graph result changed detail=%q active=%t", m.detail, m.graphActive)
	}

	_, _ = m.Update(graphMsg{id: id, request: 4, err: errors.New("query failed")})
	if !strings.Contains(m.detail, "Unable to load graph") || !strings.Contains(m.detail, "query failed") || m.graphEntries != nil || m.graphActive {
		t.Fatalf("graph error detail=%q entries=%v active=%t", m.detail, m.graphEntries, m.graphActive)
	}

	entries := map[int]gitbackend.LogEntry{8: {ID: "later"}, 3: {ID: "first"}}
	_, _ = m.Update(graphMsg{id: id, request: 4, text: "* first\n* later", entries: entries})
	if !m.graphActive || m.graphCursor != 3 || len(m.graphEntries) != 2 {
		t.Fatalf("graph success active=%t cursor=%d entries=%v", m.graphActive, m.graphCursor, m.graphEntries)
	}
}

func TestEscapeClosesInspectionAndCancelsWorkflowLoad(t *testing.T) {
	inspection := New(nil)
	inspection.loading = false
	inspection.inspectionActive = true
	inspection.graphActive = true
	inspection.graphEntries = map[int]gitbackend.LogEntry{1: {ID: "one"}}
	inspection.graphCursor = 1
	_, cmd := inspection.Update(keyMsg("esc"))
	if inspection.inspectionActive || inspection.graphActive || inspection.graphEntries != nil || inspection.graphCursor != -1 || inspection.message != "Inspection closed" {
		t.Fatalf("inspection escape cmd=%v active=%t graph=%t entries=%v cursor=%d message=%q", cmd != nil, inspection.inspectionActive, inspection.graphActive, inspection.graphEntries, inspection.graphCursor, inspection.message)
	}

	workflow := New(nil)
	ctx, cancel := context.WithCancel(workflow.appCtx)
	workflow.workflowLoading, workflow.workflowLoadCancel = true, cancel
	_, cmd = workflow.Update(keyMsg("esc"))
	if cmd != nil || workflow.workflowLoading || workflow.workflowLoadCancel != nil || workflow.message != "Workflow loading cancelled" {
		t.Fatalf("workflow escape cmd=%v loading=%t cancel=%v message=%q", cmd != nil, workflow.workflowLoading, workflow.workflowLoadCancel != nil, workflow.message)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("workflow escape did not cancel the workflow load context")
	}
}

func TestPerformRepositoryAndTransferCommands(t *testing.T) {
	newModel := func() *Model {
		m := New(nil)
		m.loading = false
		return m
	}

	t.Run("repository commands", func(t *testing.T) {
		branches := newModel()
		if cmd, handled := branches.performRepositoryCommand(keymap.CommandSwitchBranch); !handled || cmd == nil || !branches.busy {
			t.Fatalf("switch branch handled=%t cmd=%v busy=%t", handled, cmd != nil, branches.busy)
		}

		remote := newModel()
		if cmd, handled := remote.performRepositoryCommand(keymap.CommandAddRemote); !handled || cmd != nil || remote.mode != modeAddRemote {
			t.Fatalf("add remote handled=%t cmd=%v mode=%d", handled, cmd != nil, remote.mode)
		}

		quit := newModel()
		if cmd, handled := quit.performRepositoryCommand(keymap.CommandQuit); !handled || cmd == nil {
			t.Fatalf("quit handled=%t cmd=%v", handled, cmd != nil)
		}
		select {
		case <-quit.appCtx.Done():
		default:
			t.Fatal("quit command did not cancel the application context")
		}
	})

	t.Run("transfer commands", func(t *testing.T) {
		upstream := newModel()
		upstream.fetchUpstream = func(context.Context) error { return nil }
		if cmd, handled := upstream.performTransferCommand(keymap.CommandFetchUpstream); !handled || cmd == nil || !upstream.busy {
			t.Fatalf("fetch upstream handled=%t cmd=%v busy=%t", handled, cmd != nil, upstream.busy)
		}

		pushRemote := newModel()
		pushRemote.snapshot.pushRemote = "origin"
		pushRemote.fetchPush = func(context.Context) error { return nil }
		if cmd, handled := pushRemote.performTransferCommand(keymap.CommandFetchPush); !handled || cmd == nil || !pushRemote.busy {
			t.Fatalf("fetch push handled=%t cmd=%v busy=%t", handled, cmd != nil, pushRemote.busy)
		}

		configurePush := newModel()
		configurePush.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
		if cmd, handled := configurePush.performTransferCommand(keymap.CommandFetchPush); !handled || cmd != nil || configurePush.mode != modeRemotes || configurePush.remotePurpose != remoteConfigurePush {
			t.Fatalf("configure push handled=%t cmd=%v mode=%d purpose=%d", handled, cmd != nil, configurePush.mode, configurePush.remotePurpose)
		}

		elsewhere := newModel()
		if cmd, handled := elsewhere.performTransferCommand(keymap.CommandFetchElsewhere); !handled || cmd != nil || !elsewhere.isError {
			t.Fatalf("fetch elsewhere without remotes handled=%t cmd=%v error=%t", handled, cmd != nil, elsewhere.isError)
		}
		elsewhere = newModel()
		elsewhere.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
		if cmd, handled := elsewhere.performTransferCommand(keymap.CommandFetchElsewhere); !handled || cmd != nil || elsewhere.mode != modeRemotes || elsewhere.remotePurpose != remoteFetchElsewhere {
			t.Fatalf("fetch elsewhere handled=%t cmd=%v mode=%d purpose=%d", handled, cmd != nil, elsewhere.mode, elsewhere.remotePurpose)
		}

		all := newModel()
		all.fetchAll = func(context.Context) error { return nil }
		if cmd, handled := all.performTransferCommand(keymap.CommandFetchAll); !handled || cmd == nil || !all.busy {
			t.Fatalf("fetch all handled=%t cmd=%v busy=%t", handled, cmd != nil, all.busy)
		}

		push := newModel()
		push.snapshot.summary.Upstream = "origin/main"
		push.push = func(context.Context) error { return nil }
		if cmd, handled := push.performTransferCommand(keymap.CommandPush); !handled || cmd == nil || !push.busy {
			t.Fatalf("push upstream handled=%t cmd=%v busy=%t", handled, cmd != nil, push.busy)
		}

		setupPush := newModel()
		setupPush.snapshot.pushRemote = "origin"
		setupPush.pushSetUpstream = func(context.Context, string) error { return nil }
		if cmd, handled := setupPush.performTransferCommand(keymap.CommandPush); !handled || cmd == nil || !setupPush.busy {
			t.Fatalf("setup push handled=%t cmd=%v busy=%t", handled, cmd != nil, setupPush.busy)
		}

		noPushRemote := newModel()
		if cmd, handled := noPushRemote.performTransferCommand(keymap.CommandPush); !handled || cmd != nil || !noPushRemote.isError {
			t.Fatalf("push without remotes handled=%t cmd=%v error=%t", handled, cmd != nil, noPushRemote.isError)
		}
	})
}

func keyMsg(text string) tea.KeyPressMsg {
	runes := []rune(text)
	return tea.KeyPressMsg(tea.Key{Text: text, Code: runes[0]})
}
