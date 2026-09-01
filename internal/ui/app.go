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
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
	sectionmodel "github.com/richardrh/lazymagit/internal/model"
)

type mode uint8

const (
	modeStatus mode = iota
	modeCommit
	modeBranches
	modeConfirm
	modeHelp
	modeAddRemote
	modeRemotes
	modeProcess
	modeWorkflow
)

type keyScheme uint8

const (
	schemeVim keyScheme = iota
	schemeMagit
)

type remotePurpose uint8

const (
	remoteFetchElsewhere remotePurpose = iota
	remoteConfigurePush
	remoteConfigureAndPush
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
	records  []gitbackend.ProcessRecord
}

type vimGTimeoutMsg struct{ token uint64 }

const vimGTimeout = 350 * time.Millisecond

var errNoTrackedUnstagedChanges = errors.New("no tracked unstaged changes to stage")

type diffMsg struct {
	id       sectionmodel.SectionID
	request  uint64
	text     string
	err      error
	revision bool
	stash    bool
}

func (m diffMsg) detailKind() string {
	if m.stash {
		return "stash"
	}
	if m.revision {
		return "commit/revision"
	}
	return "diff"
}

func (m diffMsg) emptyDetail() string { return "No " + m.detailKind() + " output." }

type branchesMsg struct {
	request  uint64
	state    uint64
	branches []gitbackend.Branch
	err      error
}

// graphMsg carries the immutable all-refs graph result back to the UI thread.
// It is distinct from diffMsg because graph rows remain selectable after load.
type graphMsg struct {
	id      sectionmodel.SectionID
	request uint64
	title   string
	text    string
	entries map[int]gitbackend.LogEntry
	err     error
}

// revisionMsg retains resolved parent IDs so revision inspection can navigate
// commit ancestry without parsing display text.
type revisionMsg struct {
	id       sectionmodel.SectionID
	request  uint64
	text     string
	revision gitbackend.Revision
	err      error
}

type blameMsg struct {
	id      sectionmodel.SectionID
	request uint64
	title   string
	text    string
	entries map[int]gitbackend.BlameLine
	err     error
}

type graphInspection struct {
	detail  string
	id      sectionmodel.SectionID
	entries map[int]gitbackend.LogEntry
	cursor  int
	offset  int
}

type blameInspection struct {
	detail  string
	id      sectionmodel.SectionID
	entries map[int]gitbackend.BlameLine
	cursor  int
	offset  int
}

type fileMark struct {
	kind rowKind
	path string
}

// Model is the Bubble Tea application model.
type Model struct {
	repo     *gitbackend.Repository
	tree     *sectionmodel.Model
	rows     map[sectionmodel.SectionID]row
	snapshot snapshot
	resolver *keymap.Resolver

	width, height         int
	loading               bool
	busy                  bool
	compact               bool
	searching             bool
	searchQuery           string
	searchMatches         []sectionmodel.SectionID
	searchIndex           int
	message               string
	isError               bool
	detail                string
	detailID              sectionmodel.SectionID
	mode                  mode
	scheme                keyScheme
	input                 string
	branches              []gitbackend.Branch
	branchCursor          int
	confirmPath           string
	confirmPaths          []string
	detailRequest         uint64
	branchRequest         uint64
	stateGeneration       uint64
	detailOffset          int
	detailHidden          bool
	diffContext           int
	detailHunk            int
	detailLine            int
	detailRangeStart      int
	detailRangeEnd        int
	detailSelectedHunks   map[int]bool
	detailSelections      []gitbackend.InteractiveChangeSelection
	markedFiles           map[fileMark]bool
	inspectionActive      bool
	graphActive           bool
	graphCursor           int
	graphEntries          map[int]gitbackend.LogEntry
	revisionActive        bool
	revisionID            string
	revisionParents       []string
	graphReturn           *graphInspection
	blameActive           bool
	blameCursor           int
	blameEntries          map[int]gitbackend.BlameLine
	blameReturn           *blameInspection
	conflictInspectPath   string
	conflictResolution    string
	transientOffset       int
	vimGToken             uint64
	snapshotRequest       uint64
	operationRequest      uint64
	workflowRequest       uint64
	workflowLoading       bool
	foldPreferences       map[sectionmodel.SectionID]bool
	remoteInput           [2]string
	remoteField           int
	remoteFetch           bool
	remoteCursor          int
	remotePurpose         remotePurpose
	processBatches        []processBatch
	processOffset         int
	workflow              *workflowState
	workflowHandlers      map[keymap.CommandID]WorkflowHandler
	workflowCapabilities  map[keymap.CommandID]WorkflowCapability
	transientOptions      map[keymap.CommandID]OptionValue
	transientEdit         *menuEntry
	transientEditOriginal OptionValue

	showCommit      func(context.Context, string) (string, error)
	showStash       func(context.Context, string) (gitbackend.StashDetails, error)
	addRemote       func(context.Context, string, string, bool) error
	fetch           func(context.Context, ...string) error
	fetchUpstream   func(context.Context) error
	fetchPush       func(context.Context) error
	pushRemote      func(context.Context) (string, error)
	push            func(context.Context) error
	pushSetUpstream func(context.Context, string) error
	setPushRemote   func(context.Context, string) error
	fetchAll        func(context.Context) error
	stageAll        func(context.Context) error
	unstageAll      func(context.Context) error
	snapshotLoader  func(context.Context) (snapshot, error)

	appCtx             context.Context
	appCancel          context.CancelFunc
	detailCtx          context.Context
	detailCancel       context.CancelFunc
	workflowLoadCancel context.CancelFunc
}

type Options struct {
	Compact bool
}

// New creates an application for a discovered, non-bare repository.
func New(repo *gitbackend.Repository) *Model { return NewWithOptions(repo, Options{}) }

func NewWithOptions(repo *gitbackend.Repository, options Options) *Model {
	roots, rows := project(snapshot{})
	appCtx, appCancel := context.WithCancel(context.Background())
	m := &Model{
		repo: repo, tree: sectionmodel.New(roots), rows: rows,
		resolver: keymap.NewResolver(), scheme: schemeVim, loading: true, compact: options.Compact,
		message: "Loading repository…", diffContext: defaultDiffContext, detailHunk: -1, detailLine: -1, detailRangeStart: -1, detailRangeEnd: -1,
		appCtx: appCtx, appCancel: appCancel,
		foldPreferences: map[sectionmodel.SectionID]bool{
			"status/untracked": true, "status/stashes": true, "status/unpulled": true, "status/recent": true,
		},
		transientOptions: make(map[keymap.CommandID]OptionValue),
		markedFiles:      make(map[fileMark]bool),
	}
	if repo != nil {
		m.showCommit = repo.ShowCommit
		m.showStash = repo.ShowStash
		m.addRemote = repo.AddRemote
		m.fetch = repo.Fetch
		m.fetchUpstream = repo.FetchUpstream
		m.fetchPush = repo.FetchPush
		m.pushRemote = repo.PushRemote
		m.push = func(ctx context.Context) error { return repo.Push(ctx) }
		m.pushSetUpstream = repo.PushSetUpstream
		m.setPushRemote = repo.SetPushRemote
		m.fetchAll = repo.FetchAll
		m.stageAll = func(ctx context.Context) error {
			changed, err := repo.StageAll(ctx)
			if err != nil {
				return err
			}
			if !changed {
				return errNoTrackedUnstagedChanges
			}
			return nil
		}
		m.unstageAll = repo.UnstageAll
		m.snapshotLoader = func(ctx context.Context) (snapshot, error) {
			return loadSnapshotWith(ctx, snapshotQueries{
				summary: repo.Summary, status: repo.Status, stashes: repo.Stashes, remotes: repo.Remotes,
				recentLog: repo.RecentLog, upstreamLogLimit: repo.UpstreamLogLimit,
				pushRemote: m.pushRemote, operations: repo.QueryOperationState,
				sparse: repo.SparseCheckoutState,
			})
		}
	}
	m.workflowHandlers = workflowHandlersFor(m)
	m.workflowCapabilities = workflowCapabilitiesFor()
	if err := m.ValidateUIHandlers(); err != nil {
		panic("ui handler registry: " + err.Error())
	}
	return m
}

func (m *Model) Init() tea.Cmd { return m.loadSnapshotCmd() }

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if cmd, handled := m.handleAppMessage(message); handled {
		return m, cmd
	}
	if cmd, handled := m.handleDetailMessage(message); handled {
		return m, cmd
	}
	if msg, ok := message.(tea.KeyPressMsg); ok {
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleAppMessage(message tea.Msg) (tea.Cmd, bool) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg), true
	case snapshotMsg:
		return m.handleSnapshotMsg(msg), true
	case operationMsg:
		return m.handleOperationMsg(msg), true
	case workflowPreflightMsg:
		return m.handleWorkflowPreflightMsg(msg), true
	case workflowReviewMsg:
		return m.handleWorkflowReviewMsg(msg), true
	case workflowLoadMsg:
		return m.handleWorkflowLoadMsg(msg), true
	case branchesMsg:
		return m.handleBranchesMsg(msg), true
	case vimGTimeoutMsg:
		return m.handleVimGTimeoutMsg(msg), true
	default:
		return nil, false
	}
}

func (m *Model) handleDetailMessage(message tea.Msg) (tea.Cmd, bool) {
	switch msg := message.(type) {
	case graphMsg:
		return m.handleGraphMsg(msg), true
	case revisionMsg:
		return m.handleRevisionMsg(msg), true
	case blameMsg:
		return m.handleBlameMsg(msg), true
	case diffMsg:
		return m.handleDiffMsg(msg), true
	default:
		return nil, false
	}
}

func (m *Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) tea.Cmd {
	m.width, m.height = msg.Width, msg.Height
	m.clampDetailOffset()
	m.clampTransientOffset()
	m.clampProcessOffset()
	return nil
}

func (m *Model) handleSnapshotMsg(msg snapshotMsg) tea.Cmd {
	if msg.request != m.snapshotRequest || !m.appActive() {
		return nil
	}
	m.loading = false
	if msg.err != nil {
		m.setError(msg.err)
		return nil
	}
	m.install(msg.snapshot)
	m.setMessage("Repository refreshed")
	return m.loadDetailCmd()
}

func (m *Model) handleOperationMsg(msg operationMsg) tea.Cmd {
	if msg.request != m.operationRequest || !m.appActive() {
		return nil
	}
	m.busy = false
	stageNoOp := errors.Is(msg.opErr, errNoTrackedUnstagedChanges)
	if stageNoOp {
		msg.opErr = nil
	}
	if len(msg.records) > 0 || msg.opErr != nil {
		m.appendProcessBatch(msg.name, msg.records, msg.opErr)
	}
	if msg.loadErr != nil {
		m.appendProcessBatch("refresh after "+msg.name, nil, msg.loadErr)
	}
	if msg.opErr != nil || msg.loadErr != nil {
		m.openProcesses()
		m.scrollProcessesToEnd()
	}
	if msg.loadErr == nil {
		m.install(msg.snapshot)
	}
	m.finishOperationMsg(msg, stageNoOp)
	return m.loadDetailCmd()
}

func (m *Model) finishOperationMsg(msg operationMsg, stageNoOp bool) {
	switch {
	case msg.opErr != nil:
		m.setError(fmt.Errorf("%s: %w", msg.name, msg.opErr))
	case msg.loadErr != nil:
		m.setError(fmt.Errorf("%s succeeded; refresh failed: %w", msg.name, msg.loadErr))
	case stageNoOp:
		m.setMessage("No tracked unstaged changes to stage")
	default:
		m.setMessage(msg.name + " complete")
	}
}

func (m *Model) handleWorkflowPreflightMsg(msg workflowPreflightMsg) tea.Cmd {
	if m.workflow == nil || msg.request != m.workflow.request || !m.appActive() {
		return nil
	}
	m.finishWorkflowRequest(msg.err)
	if msg.err != nil {
		return nil
	}
	return m.submitWorkflow(msg.values)
}

func (m *Model) handleWorkflowReviewMsg(msg workflowReviewMsg) tea.Cmd {
	if m.workflow == nil || msg.request != m.workflow.request || !m.appActive() {
		return nil
	}
	m.finishWorkflowRequest(msg.err)
	if msg.err != nil {
		return nil
	}
	review := cloneWorkflowReview(msg.review)
	m.workflow.review = &review
	m.workflow.error = ""
	return nil
}

func (m *Model) finishWorkflowRequest(err error) {
	m.workflow.busy = false
	if m.workflow.cancel != nil {
		m.workflow.cancel()
		m.workflow.cancel = nil
	}
	if err != nil {
		m.workflow.error = sanitizeSingleLine(err.Error())
	}
}

func (m *Model) handleWorkflowLoadMsg(msg workflowLoadMsg) tea.Cmd {
	if msg.request != m.workflowRequest || msg.state != m.stateGeneration || !m.appActive() {
		return nil
	}
	m.workflowLoading = false
	if m.workflowLoadCancel != nil {
		m.workflowLoadCancel()
		m.workflowLoadCancel = nil
	}
	if msg.err != nil {
		m.setError(fmt.Errorf("load workflow: %w", msg.err))
		return nil
	}
	return m.OpenWorkflow(msg.dialog)
}

func (m *Model) handleDiffMsg(msg diffMsg) tea.Cmd {
	if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || (m.mode != modeStatus && m.mode != modeProcess) || !m.appActive() {
		return nil
	}
	m.cancelDetail()
	m.detailID, m.detailOffset = msg.id, 0
	m.resetDetailSelection()
	switch {
	case msg.err != nil:
		m.detail = "Unable to load " + msg.detailKind() + ":\n" + sanitizeSingleLine(msg.err.Error())
	case msg.text == "":
		m.detail = msg.emptyDetail()
	default:
		m.detail = sanitizeDiff(msg.text)
	}
	return nil
}

func (m *Model) handleVimGTimeoutMsg(msg vimGTimeoutMsg) tea.Cmd {
	if msg.token != m.vimGToken || m.scheme != schemeVim || m.mode != modeStatus || m.resolver.PendingPrefix() != "g" || !m.appActive() {
		return nil
	}
	m.resolver.Reset()
	m.vimGToken++
	return m.perform(keymap.CommandRefresh)
}

func (m *Model) handleBranchesMsg(msg branchesMsg) tea.Cmd {
	if msg.request != m.branchRequest || !m.appActive() {
		return nil
	}
	m.busy = false
	if msg.state != m.stateGeneration {
		return nil
	}
	m.cancelPrefix()
	if msg.err != nil {
		m.setMode(modeStatus)
		m.setError(fmt.Errorf("list branches: %w", msg.err))
		return nil
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
	return nil
}

func (m *Model) handleRevisionMsg(msg revisionMsg) tea.Cmd {
	if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || (m.mode != modeStatus && m.mode != modeProcess) || !m.appActive() {
		return nil
	}
	m.cancelDetail()
	m.detailID, m.detailOffset = msg.id, 0
	m.resetDetailSelection()
	if msg.err != nil {
		m.revisionActive, m.revisionID, m.revisionParents = false, "", nil
		m.detail = "Unable to load commit/revision:\n" + sanitizeSingleLine(msg.err.Error())
		return nil
	}
	m.revisionActive, m.revisionID = true, msg.revision.ID
	m.revisionParents = append(m.revisionParents[:0], msg.revision.ParentIDs...)
	m.detail = sanitizeDiff(msg.text)
	return nil
}

func (m *Model) handleGraphMsg(msg graphMsg) tea.Cmd {
	if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || m.mode != modeStatus || !m.appActive() {
		return nil
	}
	m.cancelDetail()
	title := msg.title
	if title == "" {
		title = "History"
	}
	m.detailID, m.detail = msg.id, sanitizeDiff(title+"\n\n"+msg.text)
	m.detailOffset, m.graphEntries, m.graphActive = 0, msg.entries, msg.err == nil && len(msg.entries) > 0
	m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn = false, "", nil, nil
	m.graphCursor = -1
	if msg.err != nil {
		m.installGraphFailure(msg)
	} else {
		m.installGraphCursor(msg.entries)
	}
	m.resetDetailSelection()
	return nil
}

func (m *Model) installGraphFailure(msg graphMsg) {
	failure := "Unable to load history:\n"
	if msg.title == "" || msg.title == "All refs graph" {
		failure = "Unable to load graph:\n"
	}
	m.detail = failure + sanitizeSingleLine(msg.err.Error())
	m.graphEntries = nil
}

func (m *Model) installGraphCursor(entries map[int]gitbackend.LogEntry) {
	for line := range entries {
		if m.graphCursor < 0 || line < m.graphCursor {
			m.graphCursor = line
		}
	}
	if m.graphCursor >= 0 {
		m.detailOffset = min(m.graphCursor, m.detailMaximumOffset())
	}
}

func (m *Model) handleBlameMsg(msg blameMsg) tea.Cmd {
	if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || m.mode != modeStatus || !m.appActive() {
		return nil
	}
	m.cancelDetail()
	m.detailID, m.detail = msg.id, sanitizeDiff(msg.title+"\n\n"+msg.text)
	m.detailOffset, m.blameEntries, m.blameActive = 0, msg.entries, msg.err == nil && len(msg.entries) > 0
	m.graphActive, m.graphEntries, m.graphCursor = false, nil, -1
	m.revisionActive, m.revisionID, m.revisionParents, m.blameReturn = false, "", nil, nil
	m.blameCursor = firstBlameLine(msg.entries)
	if msg.err != nil {
		m.detail = "Unable to load blame:\n" + sanitizeSingleLine(msg.err.Error())
		m.blameEntries = nil
	}
	if m.blameCursor >= 0 {
		m.detailOffset = min(m.blameCursor, m.detailMaximumOffset())
	}
	m.resetDetailSelection()
	return nil
}

func (m *Model) handleGlobalKey(key string) (tea.Cmd, bool) {
	if key == "ctrl+c" && m.scheme == schemeVim {
		m.shutdown()
		return tea.Quit, true
	}
	if key == "esc" && m.resolver.PendingPrefix() == "" {
		if cmd, handled := m.handleInspectionEscape(); handled {
			return cmd, true
		}
	}
	if key == "esc" && m.workflowLoading {
		m.cancelWorkflowLoad()
		m.setMessage("Workflow loading cancelled")
		return nil, true
	}
	if key != "f2" {
		return nil, false
	}
	m.cancelPrefix()
	if m.mode == modeHelp {
		m.setMode(modeStatus)
	}
	if m.scheme == schemeVim {
		m.scheme = schemeMagit
		m.setMessage("Magit key scheme active")
	} else {
		m.scheme = schemeVim
		m.setMessage("Vim key scheme active")
	}
	return nil, true
}

func (m *Model) handleInspectionEscape() (tea.Cmd, bool) {
	if m.revisionActive && m.graphReturn != nil {
		m.restoreGraphInspection()
		m.setMessage("Returned to graph")
		return nil, true
	}
	if m.revisionActive && m.blameReturn != nil {
		m.restoreBlameInspection()
		m.setMessage("Returned to blame")
		return nil, true
	}
	if !m.inspectionActive {
		return nil, false
	}
	m.closeInspection()
	return m.loadDetailCmd(), true
}

func (m *Model) closeInspection() {
	m.inspectionActive, m.graphActive, m.graphEntries, m.graphCursor = false, false, nil, -1
	m.blameActive, m.blameEntries, m.blameCursor, m.blameReturn = false, nil, -1, nil
	m.conflictInspectPath, m.conflictResolution = "", ""
	m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn = false, "", nil, nil
	m.setMessage("Inspection closed")
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if cmd, handled := m.handleGlobalKey(key); handled {
		return m, cmd
	}
	if cmd, handled := m.handleModeKey(msg, key); handled {
		return m, cmd
	}
	if cmd, handled := m.handlePendingTransientKey(msg, key); handled {
		return m, cmd
	}
	return m.handleStatusKey(msg, key)
}

func (m *Model) handleModeKey(msg tea.KeyPressMsg, key string) (tea.Cmd, bool) {
	switch m.mode {
	case modeCommit:
		_, cmd := m.handleCommitKey(msg)
		return cmd, true
	case modeBranches:
		_, cmd := m.handleBranchKey(key)
		return cmd, true
	case modeAddRemote:
		_, cmd := m.handleAddRemoteKey(msg)
		return cmd, true
	case modeRemotes:
		_, cmd := m.handleRemoteKey(key)
		return cmd, true
	case modeProcess:
		return m.routeProcessKey(msg, key), true
	case modeWorkflow:
		_, cmd := m.handleWorkflowKey(msg)
		return cmd, true
	case modeConfirm:
		return m.handleConfirmKey(key), true
	case modeHelp:
		return m.handleHelpKey(key), true
	}
	return nil, false
}

func (m *Model) routeProcessKey(msg tea.KeyPressMsg, key string) tea.Cmd {
	switch key {
	case "q", "esc", "$", "up", "down", "pgup", "pgdown", "y":
		_, cmd := m.handleProcessKey(key)
		return cmd
	default:
		m.closeProcesses()
		_, cmd := m.handleKey(msg)
		return cmd
	}
}

type discardConfirmationAction uint8

const (
	discardConfirmationIgnored discardConfirmationAction = iota
	discardConfirmationAccepted
	discardConfirmationCancelled
)

func discardConfirmationActionForKey(key string) discardConfirmationAction {
	switch key {
	case "y", "Y":
		return discardConfirmationAccepted
	case "n", "N", "q", "esc":
		return discardConfirmationCancelled
	default:
		return discardConfirmationIgnored
	}
}

func confirmedDiscardPaths(paths []string, path string) []string {
	confirmed := append([]string(nil), paths...)
	if len(confirmed) == 0 && path != "" {
		return []string{path}
	}
	return confirmed
}

func (m *Model) handleConfirmKey(key string) tea.Cmd {
	action := discardConfirmationActionForKey(key)
	if action == discardConfirmationIgnored {
		return nil
	}
	paths := confirmedDiscardPaths(m.confirmPaths, m.confirmPath)
	m.setMode(modeStatus)
	m.confirmPath = ""
	m.confirmPaths = nil
	if action == discardConfirmationAccepted {
		return m.startOperation(fileOperationName("discard", paths), func(ctx context.Context) error { return m.repo.Discard(ctx, paths) })
	}
	m.setMessage("Discard cancelled")
	return m.loadDetailCmd()
}

func (m *Model) handleHelpKey(key string) tea.Cmd {
	if key == "q" || key == "?" || key == "esc" {
		m.setMode(modeStatus)
		return m.loadDetailCmd()
	}
	if m.scrollTransient(key) {
		return nil
	}
	if _, ok := prefixCatalogs[key]; ok {
		m.setMode(modeStatus)
		m.transientOffset = 0
		_ = m.resolver.Feed(m.keyContext(), key)
		return nil
	}
	entry, found := dispatcherEntry(m.dispatcherCatalog(), key)
	if !found {
		return nil
	}
	if entry.Available && entry.Command != keymap.CommandNone {
		m.setMode(modeStatus)
		m.transientOffset = 0
		return m.perform(entry.Command)
	}
	if !entry.Available {
		m.setMessage(entry.Label + " unavailable: " + entry.Reason)
	}
	return nil
}

func (m *Model) handlePendingTransientKey(msg tea.KeyPressMsg, key string) (tea.Cmd, bool) {
	pending := m.resolver.PendingPrefix()
	if pending == "" || pending == "g" || m.resolver.ActiveTransient() == "" {
		return nil, false
	}
	return m.handleActiveTransientKey(msg, key, transientInvocationRoot(pending)), true
}

func (m *Model) handleActiveTransientKey(msg tea.KeyPressMsg, key, prefix string) tea.Cmd {
	if cmd, handled := m.handleTransientEditKey(msg, key); handled {
		return cmd
	}
	if key == "q" || key == "esc" {
		m.cancelPrefix()
		m.setMessage("Transient cancelled")
		return nil
	}
	if m.scrollTransient(key) {
		return nil
	}
	catalog, _ := m.transientCatalog(prefix)
	candidate := strings.TrimSpace(m.resolver.PendingSuffix() + " " + key)
	entry, found := catalog.entry(candidate)
	if catalog.hasDescendant(candidate) && (!found || entry.Kind == keymap.KindInfix || !entry.Available) && m.continueTransientKey(key) {
		return nil
	}
	if found && entry.Available {
		return m.dispatchTransientEntry(prefix, key, entry)
	}
	return m.handleUnmatchedTransientKey(prefix, key, entry, found)
}

func (m *Model) continueTransientKey(key string) bool {
	if !m.resolver.Feed(m.keyContext(), key).Pending {
		return false
	}
	m.transientOffset = 0
	return true
}

func (m *Model) dispatchTransientEntry(prefix, key string, entry menuEntry) tea.Cmd {
	if entry.Prefix && m.workflowHandlers[entry.Command] == nil {
		if m.continueTransientKey(key) {
			return nil
		}
	}
	if entry.Kind == keymap.KindInfix {
		m.resolver.ContinueTransient()
		m.editTransientOption(entry)
		return nil
	}
	if err := m.validateTransientOptions(prefix, entry.Command); err != nil {
		m.setError(err)
		return nil
	}
	m.cancelPrefix()
	return m.performEntry(entry, prefix)
}

func (m *Model) handleUnmatchedTransientKey(prefix, key string, entry menuEntry, found bool) tea.Cmd {
	if prefix == "f" && key == "f" {
		m.cancelPrefix()
		return nil
	}
	if key == "?" {
		m.cancelPrefix()
		m.transientOffset = 0
		m.setMode(modeHelp)
		return nil
	}
	if !found {
		if _, switches := prefixCatalogs[key]; switches {
			m.cancelPrefix()
			m.transientOffset = 0
			_ = m.resolver.Feed(m.keyContext(), key)
			return nil
		}
	}
	label := key
	if found {
		label = entry.Label
	}
	if found && entry.Reason != "" {
		m.setMessage(label + " unavailable: " + entry.Reason)
	} else {
		m.setMessage("not implemented: " + label)
	}
	return nil
}

func (m *Model) handleTransientEditKey(msg tea.KeyPressMsg, key string) (tea.Cmd, bool) {
	if m.transientEdit == nil {
		return nil, false
	}
	entry := *m.transientEdit
	value := m.transientOptions[entry.Command]
	switch key {
	case "enter":
		value.Enabled = value.Value != ""
		m.transientOptions[entry.Command] = value
		m.transientEdit = nil
		m.setMessage(entry.Label + " updated")
	case "esc":
		m.transientOptions[entry.Command] = m.transientEditOriginal
		m.transientEdit = nil
		m.setMessage("Option edit cancelled")
	case "backspace", "ctrl+h":
		if value.Value != "" {
			_, n := utf8.DecodeLastRuneInString(value.Value)
			value.Value = value.Value[:len(value.Value)-n]
			m.transientOptions[entry.Command] = value
		}
	default:
		if text := msg.Key().Text; text != "" {
			value.Value += text
			m.transientOptions[entry.Command] = value
		}
	}
	return nil, true
}

func (m *Model) handleStatusKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	if cmd, handled := m.handleStatusPreRouting(msg, key); handled {
		return m, cmd
	}
	prefix := m.resolver.PendingPrefix()
	hadPrefix := prefix != ""
	result := m.resolver.Feed(m.keyContext(), key)
	// Magit 4.7 deliberately has no f f action. Do not replay the unmatched
	// suffix as a fresh prefix, which would leave the UI misleadingly pending.
	if hadPrefix && prefix == "f" && key == "f" && !result.Handled {
		return m, nil
	}
	// The resolver deliberately does not claim an unmatched suffix. Interpret
	// it once more as an ordinary key after cancelling the old prefix.
	if hadPrefix && !result.Handled {
		result = m.resolver.Feed(m.keyContext(), key)
	}
	if result.Pending {
		return m, m.handlePendingStatusResult(key, hadPrefix)
	}
	if prefix == "g" {
		m.vimGToken++
	}
	return m, m.handleResolvedStatusResult(result)
}

func (m *Model) handlePendingStatusResult(key string, hadPrefix bool) tea.Cmd {
	if m.scheme == schemeVim && key == "g" && m.resolver.PendingPrefix() == "g" {
		m.vimGToken++
		token := m.vimGToken
		return tea.Tick(vimGTimeout, func(time.Time) tea.Msg { return vimGTimeoutMsg{token: token} })
	}
	if !hadPrefix {
		if _, ok := m.transientCatalog(m.resolver.PendingPrefix()); ok {
			m.transientOptions = make(map[keymap.CommandID]OptionValue)
		}
	}
	m.transientOffset = 0
	return nil
}

func (m *Model) handleResolvedStatusResult(result keymap.Result) tea.Cmd {
	if result.Command != keymap.CommandNone {
		return m.perform(result.Command)
	}
	if !result.Handled || result.Binding == nil {
		return nil
	}
	if m.workflowHandlers[result.Binding.Command] != nil {
		return m.perform(result.Binding.Command)
	}
	if id, ok := m.directWorkflowCommand(result.Binding.UpstreamCommand); ok {
		return m.perform(id)
	}
	if result.Binding.Handler == keymap.HandlerUnsupported {
		m.setMessage(fmt.Sprintf("%s unavailable: %s (%s)", result.Binding.Display, result.Binding.UpstreamCommand, result.Binding.Parity))
	} else if result.Reason != "" {
		m.setMessage(result.Binding.Label + " unavailable: " + result.Reason)
	}
	return nil
}

func (m *Model) directWorkflowCommand(upstream string) (keymap.CommandID, bool) {
	for id, capability := range m.workflowCapabilities {
		if capability.Transient == "" && capability.UpstreamCommand == upstream {
			return id, true
		}
	}
	return keymap.CommandNone, false
}

func (m *Model) handleStatusPreRouting(msg tea.KeyPressMsg, key string) (tea.Cmd, bool) {
	if cmd, handled := m.handleStatusModeKey(msg, key); handled {
		return cmd, true
	}
	return m.handleStatusNavigation(msg, key)
}

func (m *Model) handleStatusModeKey(msg tea.KeyPressMsg, key string) (tea.Cmd, bool) {
	if m.handleStatusSearchKey(key, msg.Key().Text) {
		m.cancelPrefix()
		return m.loadDetailCmd(), true
	}
	if key == "q" {
		m.shutdown()
		return tea.Quit, true
	}
	if key == "?" {
		m.resolver.Reset()
		m.transientOffset = 0
		m.setMode(modeHelp)
		return nil, true
	}
	if m.handlePatchHunkSelectionKey(key) || m.handlePatchRangeKey(key) {
		m.cancelPrefix()
		return nil, true
	}
	if key == "alt+m" && m.toggleFileMark() {
		m.cancelPrefix()
		return nil, true
	}
	if cmd, handled := m.handleInspectionNavigationKey(key); handled {
		return cmd, true
	}
	if cmd, handled := m.handleDetailScroll(key); handled {
		m.cancelPrefix()
		return cmd, true
	}
	if request, ok := m.focusedHunkRequest(key); ok {
		m.cancelPrefix()
		return m.openInteractiveChange(request), true
	}
	return nil, false
}

func (m *Model) handleStatusNavigation(msg tea.KeyPressMsg, key string) (tea.Cmd, bool) {
	if binding, ok := keymap.Find(schemeID(m.scheme), keymap.ContextStatus, key); !ok || m.workflowHandlers[binding.Command] == nil {
		if cmd, handled := m.handleNavigationKey(msg); handled {
			m.cancelPrefix()
			return cmd, true
		}
	}
	return nil, false
}

func (m *Model) scrollTransient(key string) bool {
	page := 1
	if m.mode == modeHelp {
		sections := m.dispatcherCatalog()
		total := len(dispatcherCanvas(sections, m.width))
		page = dispatcherViewportHeight(m.width, m.height-4, total)
	} else if catalog, ok := m.activeTransientCatalog(); ok {
		page = max(1, transientViewportHeight(catalog, m.width, m.height-4))
	}
	switch key {
	case "down":
		m.transientOffset++
	case "up":
		m.transientOffset = max(0, m.transientOffset-1)
	case "pgdown":
		m.transientOffset += page
	case "pgup":
		m.transientOffset = max(0, m.transientOffset-page)
	default:
		return false
	}
	m.clampTransientOffset()
	return true
}

func (m *Model) clampTransientOffset() {
	if m.mode == modeHelp {
		m.transientOffset = min(max(0, m.transientOffset), dispatcherMaximumOffset(m.dispatcherCatalog(), m.width, m.height-4))
		return
	}
	catalog, ok := m.activeTransientCatalog()
	if !ok {
		m.transientOffset = 0
		return
	}
	m.transientOffset = min(max(0, m.transientOffset), transientMaximumOffset(catalog, m.width, m.height-4))
}

func (m *Model) activeTransientCatalog() (menuCatalog, bool) {
	if m.resolver.ActiveTransient() == "" {
		return menuCatalog{}, false
	}
	return m.transientCatalog(transientInvocationRoot(m.resolver.PendingPrefix()))
}

func transientRoot(pending string) (string, bool) {
	for prefix := range prefixCatalogs {
		if pending == prefix || strings.HasPrefix(pending, prefix+" ") {
			return prefix, true
		}
	}
	return "", false
}

func transientInvocationRoot(pending string) string { root, _ := transientRoot(pending); return root }

func (m *Model) validateTransientOptions(prefix string, suffix keymap.CommandID) error {
	for optionID, value := range m.transientOptions {
		if !value.Enabled && value.Value == "" {
			continue
		}
		name := m.resolver.ActiveTransient()
		for _, binding := range keymap.BindingsForTransient(schemeID(m.scheme), name) {
			if binding.Kind != keymap.KindInfix || binding.Command != optionID {
				continue
			}
			if !m.optionConsumers(prefix, binding)[suffix] {
				return fmt.Errorf("%s is not consumed by this suffix; disable it or choose a compatible command", binding.Label)
			}
		}
	}
	return nil
}

func (m *Model) editTransientOption(entry menuEntry) {
	if entry.TakesValue {
		copy := entry
		m.transientEdit = &copy
		m.transientEditOriginal = m.transientOptions[entry.Command]
		m.setMessage("Edit " + entry.Label + "; Enter applies, Esc cancels")
		return
	}
	value, set := m.transientOptions[entry.Command]
	if !set {
		value = entry.Option
	}
	value.Enabled = !value.Enabled
	m.transientOptions[entry.Command] = value
	m.setMessage(entry.Label + " " + map[bool]string{true: "enabled", false: "disabled"}[value.Enabled])
}

func (m *Model) performEntry(entry menuEntry, prefix string) tea.Cmd {
	command := WorkflowCommand{ID: entry.Command, Occurrence: entry.Occurrence, Prefix: prefix, Options: cloneOptions(m.transientOptions)}
	if cmd, handled := m.performWorkflow(command); handled {
		return cmd
	}
	return m.perform(entry.Command)
}

func (m *Model) handleAddRemoteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.setMode(modeStatus)
		m.setMessage("Add remote cancelled")
		return m, m.loadDetailCmd()
	case "tab", "down":
		m.remoteField = (m.remoteField + 1) % 2
	case "up":
		m.remoteField = (m.remoteField + 1) % 2
	case "ctrl+f":
		m.remoteFetch = !m.remoteFetch
	case "enter":
		return m, m.submitAddRemote()
	case "backspace", "ctrl+h":
		m.deleteRemoteInputRune()
	default:
		if text := msg.Key().Text; text != "" {
			m.remoteInput[m.remoteField] += text
		}
	}
	return m, nil
}

func (m *Model) submitAddRemote() tea.Cmd {
	if m.remoteField == 0 {
		m.remoteField = 1
		return nil
	}
	name, url, fetch := strings.TrimSpace(m.remoteInput[0]), m.remoteInput[1], m.remoteFetch
	if name == "" || url == "" {
		m.setError(errors.New("remote name and URL are required"))
		return nil
	}
	m.setMode(modeStatus)
	return m.startOperation("add remote", func(ctx context.Context) error { return m.addRemote(ctx, name, url, fetch) })
}

func (m *Model) deleteRemoteInputRune() {
	value := m.remoteInput[m.remoteField]
	if value == "" {
		return
	}
	_, size := utf8.DecodeLastRuneInString(value)
	m.remoteInput[m.remoteField] = value[:len(value)-size]
}

func (m *Model) handleRemoteKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		m.setMode(modeStatus)
		return m, m.loadDetailCmd()
	case "j", "n", "down":
		if m.remoteCursor+1 < len(m.snapshot.remotes) {
			m.remoteCursor++
		}
	case "k", "p", "up":
		if m.remoteCursor > 0 {
			m.remoteCursor--
		}
	case "enter":
		return m, m.submitSelectedRemote()
	}
	return m, nil
}

func (m *Model) submitSelectedRemote() tea.Cmd {
	if len(m.snapshot.remotes) == 0 {
		return nil
	}
	remote := m.snapshot.remotes[m.remoteCursor].Name
	m.setMode(modeStatus)
	switch m.remotePurpose {
	case remoteConfigureAndPush:
		return m.startOperation("push and set upstream", func(ctx context.Context) error { return m.pushSetUpstream(ctx, remote) })
	case remoteConfigurePush:
		return m.startOperation("configure and fetch push remote", func(ctx context.Context) error {
			if err := m.setPushRemote(ctx, remote); err != nil {
				return err
			}
			return m.fetchPush(ctx)
		})
	default:
		return m.startOperation("fetch "+sanitizeSingleLine(remote), func(ctx context.Context) error { return m.fetch(ctx, remote) })
	}
}

func (m *Model) cancelPrefix() {
	if m.resolver.PendingPrefix() != "" {
		if m.resolver.PendingPrefix() == "g" {
			m.vimGToken++
		}
		m.resolver.Reset()
	}
	m.transientOffset = 0
	m.transientEdit = nil
}

func (m *Model) handleCommitKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, m.cancelCommit()
	case "enter":
		return m, m.submitCommit()
	case "backspace", "ctrl+h":
		m.deleteCommitRune()
	default:
		m.appendCommitText(msg.Key().Text)
	}
	return m, nil
}

func (m *Model) cancelCommit() tea.Cmd {
	m.setMode(modeStatus)
	m.input = ""
	m.setMessage("Commit cancelled")
	return m.loadDetailCmd()
}

func (m *Model) submitCommit() tea.Cmd {
	message := strings.TrimSpace(m.input)
	if message == "" {
		m.setError(errors.New("commit message cannot be empty"))
		return nil
	}
	m.setMode(modeStatus)
	m.input = ""
	return m.startOperation("commit", func(ctx context.Context) error {
		_, err := m.repo.Commit(ctx, message)
		return err
	})
}

func (m *Model) deleteCommitRune() {
	if m.input == "" {
		return
	}
	_, size := utf8.DecodeLastRuneInString(m.input)
	m.input = m.input[:len(m.input)-size]
}

func (m *Model) appendCommitText(text string) {
	if text != "" {
		m.input += text
	}
}

func (m *Model) handleBranchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		return m, m.closeBranches()
	case "j", "n", "down":
		m.moveBranchCursor(1)
	case "k", "p", "up":
		m.moveBranchCursor(-1)
	case "enter":
		return m, m.switchSelectedBranch()
	}
	return m, nil
}

func (m *Model) closeBranches() tea.Cmd {
	m.setMode(modeStatus)
	return m.loadDetailCmd()
}

func (m *Model) moveBranchCursor(delta int) {
	m.branchCursor = min(max(0, m.branchCursor+delta), max(0, len(m.branches)-1))
}

func (m *Model) switchSelectedBranch() tea.Cmd {
	if len(m.branches) == 0 {
		return nil
	}
	branch := m.branches[m.branchCursor]
	m.setMode(modeStatus)
	return m.startOperation("switch branch", func(ctx context.Context) error { return m.repo.SwitchBranch(ctx, branch.Name) })
}

type detailScrollBehavior struct {
	action string
	amount int
}

var detailScrollBehaviors = map[string]detailScrollBehavior{
	"down":   {action: "lines", amount: 1},
	"up":     {action: "lines", amount: -1},
	"home":   {action: "home"},
	"end":    {action: "end"},
	"]":      {action: "hunks", amount: 1},
	"[":      {action: "hunks", amount: -1},
	"pgdown": {action: "pages", amount: 1},
	"ctrl+d": {action: "half-pages", amount: 1},
	"pgup":   {action: "pages", amount: -1},
	"ctrl+u": {action: "half-pages", amount: -1},
}

func (m *Model) handleDetailScroll(key string) (tea.Cmd, bool) {
	viewport := m.detailViewportHeight()
	behavior, handled := detailScrollBehaviors[key]
	if viewport <= 0 || !handled {
		return nil, false
	}
	m.applyDetailScrollBehavior(behavior, viewport)
	return nil, true
}

func (m *Model) applyDetailScrollBehavior(behavior detailScrollBehavior, viewport int) {
	switch behavior.action {
	case "lines":
		m.scrollDetail(behavior.amount)
	case "home":
		m.detailOffset = 0
	case "end":
		m.detailOffset = m.detailMaximumOffset()
	case "hunks":
		m.scrollDetailHunk(behavior.amount)
	case "pages":
		m.scrollDetail(behavior.amount * viewport)
	case "half-pages":
		m.scrollDetail(behavior.amount * max(1, viewport/2))
	}
}

func (m *Model) scrollDetail(delta int) {
	m.detailOffset += delta
	m.clampDetailOffset()
}

func (m *Model) detailMaximumOffset() int {
	viewport := m.detailViewportHeight()
	lineCount := len(m.detailLines())
	return max(0, lineCount-viewport)
}

func (m *Model) detailLines() []string {
	return strings.Split(strings.TrimSuffix(sanitizeDiff(m.detail), "\n"), "\n")
}

func (m *Model) scrollDetailHunk(direction int) {
	lines := m.detailLines()
	start := m.detailHunk
	if start < 0 {
		start = m.detailOffset
		if direction < 0 {
			start = len(lines)
		}
	}
	if direction > 0 {
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "@@") {
				m.detailHunk = i
				m.detailOffset = min(i, m.detailMaximumOffset())
				m.setMessage("Next hunk")
				return
			}
		}
		m.setMessage("No next hunk")
		return
	}
	for i := min(start-1, len(lines)-1); i >= 0; i-- {
		if strings.HasPrefix(lines[i], "@@") {
			m.detailHunk = i
			m.detailOffset = min(i, m.detailMaximumOffset())
			m.setMessage("Previous hunk")
			return
		}
	}
	m.setMessage("No previous hunk")
}

func (m *Model) clampDetailOffset() {
	m.detailOffset = min(max(0, m.detailOffset), m.detailMaximumOffset())
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

func (m *Model) perform(command keymap.CommandID) tea.Cmd {
	if cmd, handled := m.performWorkflow(WorkflowCommand{ID: command, Options: cloneOptions(m.transientOptions)}); handled {
		return cmd
	}
	if cmd, handled := m.performNavigationCommand(command); handled {
		return cmd
	}
	for _, handler := range []func(keymap.CommandID) (tea.Cmd, bool){
		m.performMovementCommand,
		m.performDisplayCommand,
		m.performChangeCommand,
		m.performRepositoryCommand,
		m.performTransferCommand,
	} {
		if cmd, handled := handler(command); handled {
			return cmd
		}
	}
	m.setError(fmt.Errorf("internal error: no UI handler for %s", command))
	return nil
}

func (m *Model) performMovementCommand(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandMoveDown:
		return m.move(1), true
	case keymap.CommandMoveUp:
		return m.move(-1), true
	case keymap.CommandFirst:
		return m.moveTo(0), true
	case keymap.CommandLast:
		return m.moveTo(len(m.tree.VisibleSectionIDs()) - 1), true
	}
	return nil, false
}

func (m *Model) performDisplayCommand(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandRefresh:
		if !m.canOperate() {
			return nil, true
		}
		m.busy = true
		m.setMessage("Refreshing…")
		return m.refreshOperationCmd("refresh", nil), true
	case keymap.CommandToggleSection:
		id := m.tree.Cursor()
		m.tree.ToggleFold(id)
		if section := m.tree.Section(id); section != nil && len(section.Children()) > 0 {
			m.foldPreferences[id] = m.tree.IsFolded(id)
		}
		return m.loadDetailCmd(), true
	case keymap.CommandDepth1, keymap.CommandDepth2, keymap.CommandDepth3:
		depth := int(command[len(command)-1] - '0')
		m.tree.SetGlobalDepth(depth)
		return m.loadDetailCmd(), true
	case keymap.CommandOpenDispatcher:
		m.transientOffset = 0
		m.setMode(modeHelp)
	case keymap.CommandScrollDown:
		m.scrollDetail(m.detailViewportHeight())
	case keymap.CommandScrollUp:
		m.scrollDetail(-m.detailViewportHeight())
	case keymap.CommandShowProcesses:
		m.openProcesses()
	default:
		return nil, false
	}
	return nil, true
}

func (m *Model) performChangeCommand(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandStage:
		return m.performFileChange("stage", m.repo.Stage, rowUnstaged, rowUntracked), true
	case keymap.CommandUnstage:
		return m.performFileChange("unstage", m.repo.Unstage, rowStaged), true
	case keymap.CommandStageAll:
		return m.performAggregateChange("stage all tracked changes", m.stageAll), true
	case keymap.CommandUnstageAll:
		return m.performAggregateChange("unstage all", m.unstageAll), true
	case keymap.CommandDiscard:
		m.performDiscardChange()
	case keymap.CommandCommit:
		if m.canOperate() {
			m.setMode(modeCommit)
			m.input = ""
		}
	default:
		return nil, false
	}
	return nil, true
}

func (m *Model) performFileChange(name string, operation func(context.Context, []string) error, kinds ...rowKind) tea.Cmd {
	paths := m.fileOperationPaths(kinds...)
	if len(paths) == 0 || !m.canOperate() {
		return nil
	}
	return m.startOperation(fileOperationName(name, paths), func(ctx context.Context) error {
		return operation(ctx, paths)
	})
}

func (m *Model) performAggregateChange(name string, operation func(context.Context) error) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	return m.startOperation(name, operation)
}

func (m *Model) performDiscardChange() {
	if m.scheme == schemeMagit {
		m.beginDiscard(rowUnstaged, rowUntracked, rowStaged)
		return
	}
	m.beginDiscard(rowUnstaged, rowUntracked)
}

func (m *Model) performRepositoryCommand(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandSwitchBranch:
		if m.canOperate() {
			m.busy = true
			m.setMessage("Loading branches…")
			return m.loadBranchesCmd(), true
		}
	case keymap.CommandAddRemote:
		if m.canOperate() {
			m.remoteInput, m.remoteField, m.remoteFetch = [2]string{}, 0, false
			m.setMode(modeAddRemote)
		}
	case keymap.CommandQuit:
		m.shutdown()
		return tea.Quit, true
	default:
		return nil, false
	}
	return nil, true
}

func (m *Model) performTransferCommand(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandFetchUpstream:
		if m.canOperate() {
			return m.startOperation("fetch upstream", m.fetchUpstream), true
		}
	case keymap.CommandFetchPush:
		return m.performFetchPush(), true
	case keymap.CommandFetchElsewhere:
		return m.openRemoteChooser(remoteFetchElsewhere, "fetch elsewhere"), true
	case keymap.CommandFetchAll:
		if m.canOperate() {
			return m.startOperation("fetch all", m.fetchAll), true
		}
	case keymap.CommandPush:
		return m.performPush(), true
	default:
		return nil, false
	}
	return nil, true
}

func (m *Model) performFetchPush() tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	if m.snapshot.pushRemote != "" {
		return m.startOperation("fetch push remote", m.fetchPush)
	}
	return m.openRemoteChooser(remoteConfigurePush, "configure push remote")
}

func (m *Model) openRemoteChooser(purpose remotePurpose, operation string) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	if len(m.snapshot.remotes) == 0 {
		m.setError(fmt.Errorf("%s: no remotes configured", operation))
		return nil
	}
	m.remoteCursor = 0
	m.remotePurpose = purpose
	m.setMode(modeRemotes)
	return nil
}

func (m *Model) performPush() tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	if m.snapshot.summary.Upstream != "" {
		return m.startOperation("push", m.push)
	}
	if m.snapshot.pushRemote != "" {
		remote := m.snapshot.pushRemote
		return m.startOperation("push and set upstream", func(ctx context.Context) error {
			return m.pushSetUpstream(ctx, remote)
		})
	}
	return m.openRemoteChooser(remoteConfigureAndPush, "push")
}

func (m *Model) beginDiscard(kinds ...rowKind) {
	if paths := m.fileOperationPaths(kinds...); len(paths) > 0 && m.canOperate() {
		m.setMode(modeConfirm)
		m.confirmPaths = paths
		m.confirmPath = paths[0]
	}
}

func (m *Model) canOperate() bool {
	if m.snapshotLoadActive() || m.busy || m.workflowLoading || m.workflow != nil && m.workflow.busy {
		m.setMessage("An operation is already in progress")
		return false
	}
	return true
}

func (m *Model) snapshotLoadActive() bool { return m.loading && m.snapshotRequest != 0 }

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
	ctx := keymap.Context{View: keymap.ViewStatus, Scheme: schemeID(m.scheme)}
	switch m.rows[m.tree.Cursor()].kind {
	case rowUntracked, rowUnstaged:
		ctx.Section = keymap.SectionUnstaged
	case rowStaged:
		ctx.Section = keymap.SectionStaged
	}
	return ctx
}

func schemeID(s keyScheme) keymap.Scheme {
	if s == schemeMagit {
		return keymap.SchemeMagit
	}
	return keymap.SchemeVim
}

func (m *Model) install(s snapshot) {
	m.bumpState()
	s = normalizeUpstreamSnapshot(s)
	m.snapshot = s
	roots, rows := project(s)
	m.rows = rows
	m.pruneFileMarks()
	m.tree.ReplaceSections(roots)
	for id, folded := range m.foldPreferences {
		if m.tree.Section(id) != nil && m.tree.IsFolded(id) != folded {
			m.tree.ToggleFold(id)
		}
	}
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
	m.cancelWorkflowLoad()
	if m.workflow != nil && m.workflow.cancel != nil {
		m.workflow.cancel()
	}
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

func (m *Model) resetDetailSelection() {
	m.detailHunk = -1
	m.detailLine = -1
	m.detailRangeStart = -1
	m.detailRangeEnd = -1
	m.detailSelectedHunks = nil
	m.detailSelections = nil
}

func (m *Model) bumpState() {
	m.cancelDetail()
	m.detailOffset = 0
	m.resetDetailSelection()
	m.stateGeneration++
	m.detailRequest++
}

func (m *Model) setMode(next mode) {
	if next != modeStatus {
		m.cancelPrefix()
	}
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
		s, err := m.snapshotLoader(ctx)
		return snapshotMsg{request: request, snapshot: s, err: err}
	}
}

type snapshotQueries struct {
	summary          func(context.Context) (gitbackend.Summary, error)
	status           func(context.Context) (gitbackend.Status, error)
	stashes          func(context.Context) ([]gitbackend.Stash, error)
	remotes          func(context.Context) ([]gitbackend.Remote, error)
	recentLog        func(context.Context, int) ([]gitbackend.Commit, error)
	upstreamLogLimit func(context.Context, int) (gitbackend.UpstreamRanges, error)
	pushRemote       func(context.Context) (string, error)
	operations       func(context.Context) (gitbackend.OperationState, error)
	sparse           func(context.Context) (gitbackend.SparseCheckoutState, error)
}

func loadSnapshot(ctx context.Context, repo *gitbackend.Repository) (snapshot, error) {
	return loadSnapshotWith(ctx, snapshotQueries{
		summary: repo.Summary, status: repo.Status, stashes: repo.Stashes, remotes: repo.Remotes,
		recentLog: repo.RecentLog, upstreamLogLimit: repo.UpstreamLogLimit,
		pushRemote: repo.PushRemote, operations: repo.QueryOperationState,
		sparse: repo.SparseCheckoutState,
	})
}

func loadSnapshotWith(ctx context.Context, queries snapshotQueries) (snapshot, error) {
	var s snapshot
	steps := []func(context.Context, *snapshot, snapshotQueries) error{
		loadSnapshotCore, loadSnapshotRepositoryState, loadSnapshotHistory,
	}
	for _, step := range steps {
		if err := step(ctx, &s, queries); err != nil {
			return s, err
		}
	}
	return normalizeUpstreamSnapshot(s), nil
}

func loadSnapshotCore(ctx context.Context, s *snapshot, queries snapshotQueries) error {
	var err error
	if s.summary, err = queries.summary(ctx); err != nil {
		return fmt.Errorf("summary: %w", err)
	}
	if s.status, err = queries.status(ctx); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	if queries.stashes != nil {
		if s.stashes, err = queries.stashes(ctx); err != nil {
			return fmt.Errorf("stashes: %w", err)
		}
	}
	if s.remotes, err = queries.remotes(ctx); err != nil {
		return fmt.Errorf("remotes: %w", err)
	}
	return nil
}

func loadSnapshotRepositoryState(ctx context.Context, s *snapshot, queries snapshotQueries) error {
	var err error
	if s.pushRemote, err = queries.pushRemote(ctx); err != nil && !errors.Is(err, gitbackend.ErrNoFetchRemote) {
		return fmt.Errorf("push remote: %w", err)
	}
	if queries.operations != nil {
		if s.operations, err = queries.operations(ctx); err != nil {
			return fmt.Errorf("operation state: %w", err)
		}
	}
	if queries.sparse != nil {
		if s.sparse, err = queries.sparse(ctx); err != nil {
			return fmt.Errorf("sparse checkout state: %w", err)
		}
	}
	return nil
}

func loadSnapshotHistory(ctx context.Context, s *snapshot, queries snapshotQueries) error {
	var err error
	if s.recent, err = queries.recentLog(ctx, 10); err != nil {
		return fmt.Errorf("recent log: %w", err)
	}
	if s.summary.Upstream == "" {
		return nil
	}
	if s.upstream, err = queries.upstreamLogLimit(ctx, 257); err != nil && !errors.Is(err, gitbackend.ErrNoUpstream) {
		return fmt.Errorf("upstream log: %w", err)
	}
	return nil
}

func (m *Model) startOperation(name string, operation func(context.Context) error) tea.Cmd {
	if m.busy || m.snapshotLoadActive() || m.workflowLoading || m.workflow != nil && m.workflow.busy {
		m.setMessage("An operation is already in progress")
		return nil
	}
	if operation == nil {
		m.busy = false
		m.setError(fmt.Errorf("internal error: %s operation is not configured", name))
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "operation"
	}
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
		var records []gitbackend.ProcessRecord
		if operation != nil {
			opCtx := gitbackend.WithProcessRecorder(ctx, func(record gitbackend.ProcessRecord) {
				records = append(records, cloneProcessRecord(record))
			})
			opErr = operation(opCtx)
		}
		var s snapshot
		var loadErr error
		if m.snapshotLoader != nil {
			s, loadErr = m.snapshotLoader(ctx)
		}
		return operationMsg{request: request, name: name, opErr: opErr, snapshot: s, loadErr: loadErr, records: records}
	}
}

func (m *Model) loadDetailCmd() tea.Cmd {
	if m.inspectionActive {
		return nil
	}
	m.detailHidden = false
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
		m.detailID, m.detail = r.id, "Loading commit/revision…"
		ctx, cancel := context.WithCancel(m.appCtx)
		m.detailCtx, m.detailCancel = ctx, cancel
		return func() tea.Msg {
			text, err := m.showCommit(ctx, r.commit.ID)
			return diffMsg{id: r.id, request: request, text: text, err: err, revision: true}
		}
	}
	if r.kind == rowStash {
		m.detailID, m.detail = r.id, "Loading stash…"
		ctx, cancel := context.WithCancel(m.appCtx)
		m.detailCtx, m.detailCancel = ctx, cancel
		return func() tea.Msg {
			details, err := m.showStash(ctx, r.stash.ID)
			text := ""
			if err == nil {
				text = stashDetailsText(details)
			}
			return diffMsg{id: r.id, request: request, text: text, err: err, stash: true}
		}
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
	contextLines := m.diffContext
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		var text string
		var err error
		if r.kind == rowStaged {
			text, err = m.repo.DiffStagedWithContext(ctx, r.path, contextLines)
		} else {
			text, err = m.repo.DiffWithContext(ctx, r.path, contextLines)
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
