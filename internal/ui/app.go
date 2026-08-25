// Package ui implements the standalone terminal status buffer.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

type mode uint8

const (
	modeStatus mode = iota
	modeCommit
	modeBranches
	modeConfirm
	modeHelp
)

type keyScheme uint8

const (
	schemeVim keyScheme = iota
	schemeMagit
)

type snapshotMsg struct {
	request  uint64
	snapshot snapshot
	err      error
}

type operationMsg struct {
	request  uint64
	name     string
	opErr    error
	snapshot snapshot
	loadErr  error
}

type vimGTimeoutMsg struct{ token uint64 }

const vimGTimeout = 350 * time.Millisecond

type diffMsg struct {
	id      sectionmodel.SectionID
	request uint64
	text    string
	err     error
}

type branchesMsg struct {
	request  uint64
	state    uint64
	branches []gitbackend.Branch
	err      error
}

// Model is the Bubble Tea application model.
type Model struct {
	repo     *gitbackend.Repository
	tree     *sectionmodel.Model
	rows     map[sectionmodel.SectionID]row
	snapshot snapshot
	resolver *keymap.Resolver

	width, height    int
	loading          bool
	busy             bool
	message          string
	isError          bool
	detail           string
	detailID         sectionmodel.SectionID
	mode             mode
	scheme           keyScheme
	input            string
	branches         []gitbackend.Branch
	branchCursor     int
	confirmPath      string
	detailRequest    uint64
	branchRequest    uint64
	stateGeneration  uint64
	detailOffset     int
	vimGToken        uint64
	snapshotRequest  uint64
	operationRequest uint64

	appCtx       context.Context
	appCancel    context.CancelFunc
	detailCtx    context.Context
	detailCancel context.CancelFunc
}

// New creates an application for a discovered, non-bare repository.
func New(repo *gitbackend.Repository) *Model {
	roots, rows := project(snapshot{})
	appCtx, appCancel := context.WithCancel(context.Background())
	return &Model{
		repo: repo, tree: sectionmodel.New(roots), rows: rows,
		resolver: keymap.NewResolver(), scheme: schemeVim, loading: true,
		message: "Loading repository…",
		appCtx:  appCtx, appCancel: appCancel,
	}
}

func (m *Model) Init() tea.Cmd { return m.loadSnapshotCmd() }

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampDetailOffset()
		return m, nil
	case snapshotMsg:
		if msg.request != m.snapshotRequest || !m.appActive() {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.install(msg.snapshot)
		m.setMessage("Repository refreshed")
		return m, m.loadDetailCmd()
	case operationMsg:
		if msg.request != m.operationRequest || !m.appActive() {
			return m, nil
		}
		m.busy = false
		if msg.loadErr == nil {
			m.install(msg.snapshot)
		}
		if msg.opErr != nil {
			m.setError(fmt.Errorf("%s: %w", msg.name, msg.opErr))
		} else if msg.loadErr != nil {
			m.setError(fmt.Errorf("%s succeeded; refresh failed: %w", msg.name, msg.loadErr))
		} else {
			m.setMessage(msg.name + " complete")
		}
		return m, m.loadDetailCmd()
	case diffMsg:
		if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || m.mode != modeStatus || !m.appActive() {
			return m, nil
		}
		m.cancelDetail()
		m.detailID = msg.id
		m.detailOffset = 0
		if msg.err != nil {
			m.detail = "Unable to load diff:\n" + sanitizeSingleLine(msg.err.Error())
		} else if msg.text == "" {
			m.detail = "No diff output."
		} else {
			m.detail = sanitizeDiff(msg.text)
		}
		return m, nil
	case branchesMsg:
		if msg.request != m.branchRequest || !m.appActive() {
			return m, nil
		}
		m.busy = false
		if msg.state != m.stateGeneration {
			return m, nil
		}
		if msg.err != nil {
			m.setMode(modeStatus)
			m.setError(fmt.Errorf("list branches: %w", msg.err))
			return m, nil
		}
		m.branches = msg.branches
		m.branchCursor = 0
		for i, b := range m.branches {
			if b.Current {
				m.branchCursor = i
				break
			}
		}
		m.setMode(modeBranches)
		return m, nil
	case vimGTimeoutMsg:
		if msg.token != m.vimGToken || m.scheme != schemeVim || m.mode != modeStatus || m.resolver.PendingPrefix() != "g" || !m.appActive() {
			return m, nil
		}
		m.resolver.Reset()
		m.vimGToken++
		return m, m.perform(keymap.ActionRefresh)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.shutdown()
		return m, tea.Quit
	}
	if key == "f2" {
		m.cancelPrefix()
		if m.scheme == schemeVim {
			m.scheme = schemeMagit
			m.setMessage("Magit key scheme active")
		} else {
			m.scheme = schemeVim
			m.setMessage("Vim key scheme active")
		}
		return m, nil
	}
	switch m.mode {
	case modeCommit:
		return m.handleCommitKey(msg)
	case modeBranches:
		return m.handleBranchKey(key)
	case modeConfirm:
		if key == "y" || key == "Y" {
			path := m.confirmPath
			m.setMode(modeStatus)
			m.confirmPath = ""
			return m, m.startOperation("discard", func(ctx context.Context) error { return m.repo.Discard(ctx, []string{path}) })
		}
		if key == "n" || key == "N" || key == "q" || key == "esc" {
			m.setMode(modeStatus)
			m.confirmPath = ""
			m.setMessage("Discard cancelled")
			return m, m.loadDetailCmd()
		}
		return m, nil
	case modeHelp:
		if key == "q" || key == "?" || key == "esc" {
			m.setMode(modeStatus)
			return m, m.loadDetailCmd()
		}
		return m, nil
	}

	if key == "q" {
		m.shutdown()
		return m, tea.Quit
	}
	if key == "?" {
		m.resolver.Reset()
		m.setMode(modeHelp)
		return m, nil
	}
	if key == "tab" {
		m.cancelPrefix()
		m.tree.ToggleFold(m.tree.Cursor())
		return m, m.loadDetailCmd()
	}
	if key == "1" || key == "2" || key == "3" {
		m.cancelPrefix()
		m.tree.SetGlobalDepth(int(key[0] - '0'))
		return m, m.loadDetailCmd()
	}
	if cmd, handled := m.handleDetailScroll(key); handled {
		m.cancelPrefix()
		return m, cmd
	}
	if m.scheme == schemeMagit {
		switch key {
		case "n":
			m.cancelPrefix()
			return m, m.move(1)
		case "p":
			m.cancelPrefix()
			return m, m.move(-1)
		case "g", "G":
			m.cancelPrefix()
			return m, m.perform(keymap.ActionRefresh)
		case "k":
			m.cancelPrefix()
			m.beginDiscard(rowUntracked, rowUnstaged, rowStaged)
			return m, nil
		case "x":
			m.cancelPrefix()
			m.setMessage("x is reserved for reset and unsupported")
			return m, nil
		case "j":
			m.cancelPrefix()
			return m, nil
		}
	}

	prefix := m.resolver.PendingPrefix()
	hadPrefix := prefix != ""
	result := m.resolver.Feed(m.keyContext(), key)
	// The resolver deliberately does not claim an unmatched suffix. Interpret
	// it once more as an ordinary key after cancelling the old prefix.
	if hadPrefix && !result.Handled {
		result = m.resolver.Feed(m.keyContext(), key)
	}
	if result.Pending {
		if m.scheme == schemeVim && key == "g" && m.resolver.PendingPrefix() == "g" {
			m.vimGToken++
			token := m.vimGToken
			return m, tea.Tick(vimGTimeout, func(time.Time) tea.Msg { return vimGTimeoutMsg{token: token} })
		}
		return m, nil
	}
	if prefix == "g" {
		m.vimGToken++
	}
	if result.Handled || result.Action != keymap.ActionNone {
		return m, m.perform(result.Action)
	}
	return m, nil
}

func (m *Model) cancelPrefix() {
	if m.resolver.PendingPrefix() != "" {
		if m.resolver.PendingPrefix() == "g" {
			m.vimGToken++
		}
		m.resolver.Reset()
	}
}

func (m *Model) handleCommitKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.setMode(modeStatus)
		m.input = ""
		m.setMessage("Commit cancelled")
		return m, m.loadDetailCmd()
	case "enter":
		message := strings.TrimSpace(m.input)
		if message == "" {
			m.setError(errors.New("commit message cannot be empty"))
			return m, nil
		}
		m.setMode(modeStatus)
		m.input = ""
		return m, m.startOperation("commit", func(ctx context.Context) error {
			_, err := m.repo.Commit(ctx, message)
			return err
		})
	case "backspace", "ctrl+h":
		if len(m.input) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.input)
			m.input = m.input[:len(m.input)-size]
		}
	default:
		if text := msg.Key().Text; text != "" {
			m.input += text
		}
	}
	return m, nil
}

func (m *Model) handleBranchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		m.setMode(modeStatus)
		return m, m.loadDetailCmd()
	case "j", "n", "down":
		if m.branchCursor+1 < len(m.branches) {
			m.branchCursor++
		}
	case "k", "p", "up":
		if m.branchCursor > 0 {
			m.branchCursor--
		}
	case "enter":
		if len(m.branches) == 0 {
			return m, nil
		}
		branch := m.branches[m.branchCursor]
		m.setMode(modeStatus)
		return m, m.startOperation("switch branch", func(ctx context.Context) error { return m.repo.SwitchBranch(ctx, branch.Name) })
	}
	return m, nil
}

func (m *Model) handleDetailScroll(key string) (tea.Cmd, bool) {
	viewport := m.detailViewportHeight()
	if viewport <= 0 {
		return nil, false
	}
	page := viewport
	halfPage := max(1, viewport/2)
	switch key {
	case "space", "pgdown":
		m.scrollDetail(page)
	case "ctrl+d":
		m.scrollDetail(halfPage)
	case "shift+space", "pgup":
		m.scrollDetail(-page)
	case "ctrl+u":
		m.scrollDetail(-halfPage)
	default:
		return nil, false
	}
	return nil, true
}

func (m *Model) scrollDetail(delta int) {
	m.detailOffset += delta
	m.clampDetailOffset()
}

func (m *Model) clampDetailOffset() {
	viewport := m.detailViewportHeight()
	lineCount := len(strings.Split(strings.TrimSuffix(sanitizeDiff(m.detail), "\n"), "\n"))
	maximum := max(0, lineCount-viewport)
	m.detailOffset = min(max(0, m.detailOffset), maximum)
}

func (m *Model) detailViewportHeight() int {
	bodyHeight := m.height - 4
	if bodyHeight < 3 {
		return 0
	}
	if m.width >= 96 {
		return max(0, bodyHeight-2)
	}
	if bodyHeight < 7 {
		return 0
	}
	panelHeight := bodyHeight - 1
	statusHeight := max(3, panelHeight*55/100)
	statusHeight = min(statusHeight, panelHeight-3)
	detailHeight := panelHeight - statusHeight
	return max(0, detailHeight-2)
}

func (m *Model) perform(action keymap.Action) tea.Cmd {
	switch action {
	case keymap.ActionMoveDown:
		return m.move(1)
	case keymap.ActionMoveUp:
		return m.move(-1)
	case keymap.ActionFirst:
		return m.moveTo(0)
	case keymap.ActionLast:
		return m.moveTo(len(m.tree.VisibleSectionIDs()) - 1)
	case keymap.ActionRefresh:
		if !m.canOperate() {
			return nil
		}
		m.busy = true
		m.setMessage("Refreshing…")
		return m.refreshOperationCmd("refresh", nil)
	case keymap.ActionStage:
		if r, ok := m.selectedFile(rowUnstaged, rowUntracked); ok && m.canOperate() {
			return m.startOperation("stage", func(ctx context.Context) error { return m.repo.Stage(ctx, []string{r.path}) })
		}
	case keymap.ActionUnstage:
		if r, ok := m.selectedFile(rowStaged); ok && m.canOperate() {
			return m.startOperation("unstage", func(ctx context.Context) error { return m.repo.Unstage(ctx, []string{r.path}) })
		}
	case keymap.ActionDiscard:
		m.beginDiscard(rowUnstaged, rowUntracked)
	case keymap.ActionCommit:
		if m.canOperate() {
			m.setMode(modeCommit)
			m.input = ""
		}
	case keymap.ActionSwitchBranch:
		if m.canOperate() {
			m.busy = true
			m.setMessage("Loading branches…")
			return m.loadBranchesCmd()
		}
	case keymap.ActionFetch:
		if m.canOperate() {
			return m.startOperation("fetch", func(ctx context.Context) error { return m.repo.Fetch(ctx) })
		}
	case keymap.ActionPush:
		if m.canOperate() {
			return m.startOperation("push", func(ctx context.Context) error { return m.repo.Push(ctx) })
		}
	}
	return nil
}

func (m *Model) beginDiscard(kinds ...rowKind) {
	if r, ok := m.selectedFile(kinds...); ok && m.canOperate() {
		m.setMode(modeConfirm)
		m.confirmPath = r.path
	}
}

func (m *Model) canOperate() bool {
	if m.loading || m.busy {
		m.setMessage("An operation is already in progress")
		return false
	}
	return true
}

func (m *Model) selectedFile(kinds ...rowKind) (row, bool) {
	r, ok := m.rows[m.tree.Cursor()]
	if !ok || r.path == "" {
		return row{}, false
	}
	for _, kind := range kinds {
		if r.kind == kind {
			return r, true
		}
	}
	return row{}, false
}

func (m *Model) move(delta int) tea.Cmd {
	ids := m.tree.VisibleSectionIDs()
	index := 0
	for i, id := range ids {
		if id == m.tree.Cursor() {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(ids) {
		index = len(ids) - 1
	}
	return m.moveTo(index)
}

func (m *Model) moveTo(index int) tea.Cmd {
	ids := m.tree.VisibleSectionIDs()
	if index >= 0 && index < len(ids) {
		if m.tree.Cursor() != ids[index] {
			m.tree.SetCursor(ids[index])
			m.bumpState()
		}
	}
	return m.loadDetailCmd()
}

func (m *Model) keyContext() keymap.Context {
	ctx := keymap.Context{View: keymap.ViewStatus}
	switch m.rows[m.tree.Cursor()].kind {
	case rowUntracked, rowUnstaged:
		ctx.Section = keymap.SectionUnstaged
	case rowStaged:
		ctx.Section = keymap.SectionStaged
	}
	return ctx
}

func (m *Model) install(s snapshot) {
	m.bumpState()
	m.snapshot = s
	roots, rows := project(s)
	m.rows = rows
	m.tree.ReplaceSections(roots)
}

func (m *Model) appActive() bool {
	select {
	case <-m.appCtx.Done():
		return false
	default:
		return true
	}
}

func (m *Model) shutdown() {
	m.cancelPrefix()
	m.cancelDetail()
	m.appCancel()
}

func (m *Model) cancelDetail() {
	if m.detailCancel != nil {
		m.detailCancel()
		m.detailCancel = nil
		m.detailCtx = nil
	}
}

func (m *Model) setMessage(message string) { m.message, m.isError = sanitizeSingleLine(message), false }
func (m *Model) setError(err error)        { m.message, m.isError = sanitizeSingleLine(err.Error()), true }

func (m *Model) bumpState() {
	m.cancelDetail()
	m.detailOffset = 0
	m.stateGeneration++
	m.detailRequest++
}

func (m *Model) setMode(next mode) {
	if m.mode != next {
		m.mode = next
		m.bumpState()
	}
}

func (m *Model) loadSnapshotCmd() tea.Cmd {
	m.snapshotRequest++
	request := m.snapshotRequest
	ctx := m.appCtx
	return func() tea.Msg {
		s, err := loadSnapshot(ctx, m.repo)
		return snapshotMsg{request: request, snapshot: s, err: err}
	}
}

func loadSnapshot(ctx context.Context, repo *gitbackend.Repository) (snapshot, error) {
	var s snapshot
	var err error
	if s.summary, err = repo.Summary(ctx); err != nil {
		return s, fmt.Errorf("summary: %w", err)
	}
	if s.status, err = repo.Status(ctx); err != nil {
		return s, fmt.Errorf("status: %w", err)
	}
	if s.recent, err = repo.RecentLog(ctx, 20); err != nil {
		return s, fmt.Errorf("recent log: %w", err)
	}
	if s.summary.Upstream != "" {
		if s.upstream, err = repo.UpstreamLogLimit(ctx, 100); err != nil && !errors.Is(err, gitbackend.ErrNoUpstream) {
			return s, fmt.Errorf("upstream log: %w", err)
		}
	}
	return s, nil
}

func (m *Model) startOperation(name string, operation func(context.Context) error) tea.Cmd {
	m.busy = true
	m.setMessage(strings.ToUpper(name[:1]) + name[1:] + " in progress…")
	return m.refreshOperationCmd(name, operation)
}

func (m *Model) refreshOperationCmd(name string, operation func(context.Context) error) tea.Cmd {
	m.operationRequest++
	request := m.operationRequest
	ctx := m.appCtx
	return func() tea.Msg {
		var opErr error
		if operation != nil {
			opErr = operation(ctx)
		}
		s, loadErr := loadSnapshot(ctx, m.repo)
		return operationMsg{request: request, name: name, opErr: opErr, snapshot: s, loadErr: loadErr}
	}
}

func (m *Model) loadDetailCmd() tea.Cmd {
	m.cancelDetail()
	m.detailOffset = 0
	m.detailRequest++
	request := m.detailRequest
	r, ok := m.rows[m.tree.Cursor()]
	if !ok {
		m.detail = "Select a file to inspect its diff."
		return nil
	}
	if r.kind == rowCommit {
		m.detailID = r.id
		m.detail = fmt.Sprintf("commit %s\nAuthor: %s\nDate:   %s\n\n    %s", sanitizeSingleLine(r.commit.ID), sanitizeSingleLine(r.commit.Author), r.commit.Date.Format(time.RFC1123), sanitizeSingleLine(r.commit.Subject))
		return nil
	}
	if r.kind == rowUntracked {
		m.detailID = r.id
		m.detail = "Untracked file\n\nStage it to inspect the cached diff."
		return nil
	}
	if r.kind != rowUnstaged && r.kind != rowStaged {
		m.detailID = r.id
		m.detail = "Select a file or commit to inspect details."
		return nil
	}
	m.detailID, m.detail = r.id, "Loading diff…"
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		var text string
		var err error
		if r.kind == rowStaged {
			text, err = m.repo.DiffStaged(ctx, r.path)
		} else {
			text, err = m.repo.Diff(ctx, r.path)
		}
		return diffMsg{id: r.id, request: request, text: text, err: err}
	}
}

func (m *Model) loadBranchesCmd() tea.Cmd {
	m.branchRequest++
	request := m.branchRequest
	state := m.stateGeneration
	ctx := m.appCtx
	return func() tea.Msg {
		all, err := m.repo.Branches(ctx)
		if err != nil {
			return branchesMsg{request: request, state: state, err: err}
		}
		local := all[:0]
		for _, branch := range all {
			if !branch.Remote {
				local = append(local, branch)
			}
		}
		return branchesMsg{request: request, state: state, branches: local}
	}
}
