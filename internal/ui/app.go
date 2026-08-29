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

type graphInspection struct {
	detail  string
	id      sectionmodel.SectionID
	entries map[int]gitbackend.LogEntry
	cursor  int
	offset  int
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
	inspectionActive      bool
	graphActive           bool
	graphCursor           int
	graphEntries          map[int]gitbackend.LogEntry
	revisionActive        bool
	revisionID            string
	revisionParents       []string
	graphReturn           *graphInspection
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
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampDetailOffset()
		m.clampTransientOffset()
		m.clampProcessOffset()
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
		if msg.opErr != nil {
			m.setError(fmt.Errorf("%s: %w", msg.name, msg.opErr))
		} else if msg.loadErr != nil {
			m.setError(fmt.Errorf("%s succeeded; refresh failed: %w", msg.name, msg.loadErr))
		} else if stageNoOp {
			m.setMessage("No tracked unstaged changes to stage")
		} else {
			m.setMessage(msg.name + " complete")
		}
		return m, m.loadDetailCmd()
	case workflowPreflightMsg:
		if m.workflow == nil || msg.request != m.workflow.request || !m.appActive() {
			return m, nil
		}
		m.workflow.busy = false
		if m.workflow.cancel != nil {
			m.workflow.cancel()
			m.workflow.cancel = nil
		}
		if msg.err != nil {
			m.workflow.error = sanitizeSingleLine(msg.err.Error())
			return m, nil
		}
		return m, m.submitWorkflow(msg.values)
	case workflowReviewMsg:
		if m.workflow == nil || msg.request != m.workflow.request || !m.appActive() {
			return m, nil
		}
		m.workflow.busy = false
		if m.workflow.cancel != nil {
			m.workflow.cancel()
			m.workflow.cancel = nil
		}
		if msg.err != nil {
			m.workflow.error = sanitizeSingleLine(msg.err.Error())
			return m, nil
		}
		review := cloneWorkflowReview(msg.review)
		m.workflow.review = &review
		m.workflow.error = ""
		return m, nil
	case workflowLoadMsg:
		if msg.request != m.workflowRequest || msg.state != m.stateGeneration || !m.appActive() {
			return m, nil
		}
		m.workflowLoading = false
		if m.workflowLoadCancel != nil {
			m.workflowLoadCancel()
			m.workflowLoadCancel = nil
		}
		if msg.err != nil {
			m.setError(fmt.Errorf("load workflow: %w", msg.err))
			return m, nil
		}
		return m, m.OpenWorkflow(msg.dialog)
	case graphMsg:
		if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || m.mode != modeStatus || !m.appActive() {
			return m, nil
		}
		m.cancelDetail()
		m.detailID, m.detail = msg.id, sanitizeDiff("All refs graph\n\n"+msg.text)
		m.detailOffset, m.graphEntries, m.graphActive = 0, msg.entries, msg.err == nil && len(msg.entries) > 0
		m.graphCursor = -1
		if msg.err != nil {
			m.detail = "Unable to load graph:\n" + sanitizeSingleLine(msg.err.Error())
			m.graphEntries = nil
		} else {
			for line := range msg.entries {
				if m.graphCursor < 0 || line < m.graphCursor {
					m.graphCursor = line
				}
			}
			if m.graphCursor >= 0 {
				m.detailOffset = min(m.graphCursor, m.detailMaximumOffset())
			}
		}
		m.resetDetailSelection()
		return m, nil
	case revisionMsg:
		return m, m.handleRevisionMsg(msg)
	case diffMsg:
		if msg.request != m.detailRequest || msg.id != m.tree.Cursor() || (m.mode != modeStatus && m.mode != modeProcess) || !m.appActive() {
			return m, nil
		}
		m.cancelDetail()
		m.detailID = msg.id
		m.detailOffset = 0
		m.resetDetailSelection()
		if msg.err != nil {
			m.detail = "Unable to load " + msg.detailKind() + ":\n" + sanitizeSingleLine(msg.err.Error())
		} else if msg.text == "" {
			m.detail = msg.emptyDetail()
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
		m.cancelPrefix()
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
		return m, m.perform(keymap.CommandRefresh)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
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

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" && m.scheme == schemeVim {
		m.shutdown()
		return m, tea.Quit
	}
	if key == "esc" && m.revisionActive && m.graphReturn != nil && m.resolver.PendingPrefix() == "" {
		m.restoreGraphInspection()
		m.setMessage("Returned to graph")
		return m, nil
	}
	if key == "esc" && m.inspectionActive && m.resolver.PendingPrefix() == "" {
		m.inspectionActive, m.graphActive, m.graphEntries, m.graphCursor = false, false, nil, -1
		m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn = false, "", nil, nil
		m.setMessage("Inspection closed")
		return m, m.loadDetailCmd()
	}
	if key == "esc" && m.workflowLoading {
		m.cancelWorkflowLoad()
		m.setMessage("Workflow loading cancelled")
		return m, nil
	}
	if key == "f2" {
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
		return m, nil
	}
	switch m.mode {
	case modeCommit:
		return m.handleCommitKey(msg)
	case modeBranches:
		return m.handleBranchKey(key)
	case modeAddRemote:
		return m.handleAddRemoteKey(msg)
	case modeRemotes:
		return m.handleRemoteKey(key)
	case modeProcess:
		switch key {
		case "q", "esc", "$", "up", "down", "pgup", "pgdown", "y":
			return m.handleProcessKey(key)
		default:
			// The process pane is a status-view overlay, not a modal that should
			// consume ordinary commands. Close it and dispatch the key once in the
			// underlying status view.
			m.closeProcesses()
			return m.handleKey(msg)
		}
	case modeWorkflow:
		return m.handleWorkflowKey(msg)
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
		if m.scrollTransient(key) {
			return m, nil
		}
		if _, ok := prefixCatalogs[key]; ok {
			m.setMode(modeStatus)
			m.transientOffset = 0
			_ = m.resolver.Feed(m.keyContext(), key)
			return m, nil
		}
		entry, found := dispatcherEntry(m.dispatcherCatalog(), key)
		if found && entry.Available && entry.Command != keymap.CommandNone {
			m.setMode(modeStatus)
			m.transientOffset = 0
			return m, m.perform(entry.Command)
		}
		if found && !entry.Available {
			m.setMessage(entry.Label + " unavailable: " + entry.Reason)
		}
		return m, nil
	}

	if pending := m.resolver.PendingPrefix(); pending != "" && pending != "g" {
		if m.resolver.ActiveTransient() != "" {
			prefix := transientInvocationRoot(pending)
			if m.transientEdit != nil {
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
				return m, nil
			}
			if key == "q" || key == "esc" {
				m.cancelPrefix()
				m.setMessage("Transient cancelled")
				return m, nil
			}
			if m.scrollTransient(key) {
				return m, nil
			}
			catalog, _ := m.transientCatalog(prefix)
			relative := m.resolver.PendingSuffix()
			candidate := strings.TrimSpace(relative + " " + key)
			entry, found := catalog.entry(candidate)
			hasDescendant := catalog.hasDescendant(candidate)
			// A token can be both an infix and the first token of a longer
			// suffix. Continue through the resolver trie in that case; exact
			// suffixes still dispatch immediately. This is independent of depth.
			if hasDescendant && (!found || entry.Kind == keymap.KindInfix || !entry.Available) {
				result := m.resolver.Feed(m.keyContext(), key)
				if result.Pending {
					m.transientOffset = 0
					return m, nil
				}
			}
			if found && entry.Available {
				if entry.Prefix {
					if _, registered := m.workflowHandlers[entry.Command]; registered {
						if err := m.validateTransientOptions(prefix, entry.Command); err != nil {
							m.setError(err)
							return m, nil
						}
						m.cancelPrefix()
						return m, m.performEntry(entry, prefix)
					}
					result := m.resolver.Feed(m.keyContext(), key)
					if result.Pending {
						m.transientOffset = 0
						return m, nil
					}
				}
				if entry.Kind == keymap.KindInfix {
					m.resolver.ContinueTransient()
					m.editTransientOption(entry)
					return m, nil
				}
				if err := m.validateTransientOptions(prefix, entry.Command); err != nil {
					m.setError(err)
					return m, nil
				}
				m.cancelPrefix()
				return m, m.performEntry(entry, prefix)
			}
			// Magit 4.7 has no f f binding. Preserve the historical safe no-op.
			if prefix == "f" && key == "f" {
				m.cancelPrefix()
				return m, nil
			}
			if key == "?" {
				m.cancelPrefix()
				m.transientOffset = 0
				m.setMode(modeHelp)
				return m, nil
			}
			// A catalog suffix always wins, even when unavailable. Switching is only
			// allowed for a key absent from the active transient.
			if !found {
				if _, switches := prefixCatalogs[key]; switches {
					m.cancelPrefix()
					m.transientOffset = 0
					_ = m.resolver.Feed(m.keyContext(), key)
					return m, nil
				}
			}
			if !found || !entry.Available {
				label := key
				if found {
					label = entry.Label
				}
				if found && entry.Reason != "" {
					m.setMessage(label + " unavailable: " + entry.Reason)
				} else {
					m.setMessage("not implemented: " + label)
				}
				return m, nil
			}
		}
	}

	if m.handleStatusSearchKey(key, msg.Key().Text) {
		m.cancelPrefix()
		return m, m.loadDetailCmd()
	}
	if key == "q" {
		m.shutdown()
		return m, tea.Quit
	}
	if key == "?" {
		m.resolver.Reset()
		m.transientOffset = 0
		m.setMode(modeHelp)
		return m, nil
	}
	if m.handlePatchHunkSelectionKey(key) || m.handlePatchRangeKey(key) {
		m.cancelPrefix()
		return m, nil
	}
	if m.graphActive {
		if cmd, handled := m.handleGraphKey(key); handled {
			return m, cmd
		}
	}
	if m.revisionActive {
		if cmd, handled := m.handleRevisionKey(key); handled {
			return m, cmd
		}
	}
	if cmd, handled := m.handleDetailScroll(key); handled {
		m.cancelPrefix()
		return m, cmd
	}
	if request, ok := m.focusedHunkRequest(key); ok {
		m.cancelPrefix()
		return m, m.openInteractiveChange(request)
	}
	if binding, ok := keymap.Find(schemeID(m.scheme), keymap.ContextStatus, key); !ok || m.workflowHandlers[binding.Command] == nil {
		if cmd, handled := m.handleNavigationKey(msg); handled {
			m.cancelPrefix()
			return m, cmd
		}
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
		if m.scheme == schemeVim && key == "g" && m.resolver.PendingPrefix() == "g" {
			m.vimGToken++
			token := m.vimGToken
			return m, tea.Tick(vimGTimeout, func(time.Time) tea.Msg { return vimGTimeoutMsg{token: token} })
		}
		if !hadPrefix {
			if _, ok := m.transientCatalog(m.resolver.PendingPrefix()); ok {
				m.transientOptions = make(map[keymap.CommandID]OptionValue)
			}
		}
		m.transientOffset = 0
		return m, nil
	}
	if prefix == "g" {
		m.vimGToken++
	}
	if result.Command != keymap.CommandNone {
		return m, m.perform(result.Command)
	}
	if result.Handled && result.Binding != nil {
		if _, registered := m.workflowHandlers[result.Binding.Command]; registered {
			return m, m.perform(result.Binding.Command)
		}
		for id, capability := range m.workflowCapabilities {
			if capability.Transient == "" && capability.UpstreamCommand == result.Binding.UpstreamCommand {
				return m, m.perform(id)
			}
		}
	}
	if result.Handled && result.Binding != nil && result.Binding.Handler == keymap.HandlerUnsupported {
		m.setMessage(fmt.Sprintf("%s unavailable: %s (%s)", result.Binding.Display, result.Binding.UpstreamCommand, result.Binding.Parity))
		return m, nil
	}
	if result.Handled && result.Reason != "" {
		m.setMessage(result.Binding.Label + " unavailable: " + result.Reason)
	}
	return m, nil
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
		if m.remoteField == 0 {
			m.remoteField = 1
			return m, nil
		}
		name, url, fetch := strings.TrimSpace(m.remoteInput[0]), m.remoteInput[1], m.remoteFetch
		if name == "" || url == "" {
			m.setError(errors.New("remote name and URL are required"))
			return m, nil
		}
		m.setMode(modeStatus)
		return m, m.startOperation("add remote", func(ctx context.Context) error { return m.addRemote(ctx, name, url, fetch) })
	case "backspace", "ctrl+h":
		value := m.remoteInput[m.remoteField]
		if value != "" {
			_, size := utf8.DecodeLastRuneInString(value)
			m.remoteInput[m.remoteField] = value[:len(value)-size]
		}
	default:
		if text := msg.Key().Text; text != "" {
			m.remoteInput[m.remoteField] += text
		}
	}
	return m, nil
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
		if len(m.snapshot.remotes) == 0 {
			return m, nil
		}
		remote := m.snapshot.remotes[m.remoteCursor].Name
		m.setMode(modeStatus)
		if m.remotePurpose == remoteConfigureAndPush {
			return m, m.startOperation("push and set upstream", func(ctx context.Context) error {
				return m.pushSetUpstream(ctx, remote)
			})
		}
		if m.remotePurpose == remoteConfigurePush {
			return m, m.startOperation("configure and fetch push remote", func(ctx context.Context) error {
				if err := m.setPushRemote(ctx, remote); err != nil {
					return err
				}
				return m.fetchPush(ctx)
			})
		}
		return m, m.startOperation("fetch "+sanitizeSingleLine(remote), func(ctx context.Context) error { return m.fetch(ctx, remote) })
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
	m.transientOffset = 0
	m.transientEdit = nil
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
	case "down":
		m.scrollDetail(1)
	case "up":
		m.scrollDetail(-1)
	case "home":
		m.detailOffset = 0
	case "end":
		m.detailOffset = m.detailMaximumOffset()
	case "]":
		m.scrollDetailHunk(1)
	case "[":
		m.scrollDetailHunk(-1)
	case "pgdown":
		m.scrollDetail(page)
	case "ctrl+d":
		m.scrollDetail(halfPage)
	case "pgup":
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
	switch command {
	case keymap.CommandMoveDown:
		return m.move(1)
	case keymap.CommandMoveUp:
		return m.move(-1)
	case keymap.CommandFirst:
		return m.moveTo(0)
	case keymap.CommandLast:
		return m.moveTo(len(m.tree.VisibleSectionIDs()) - 1)
	case keymap.CommandRefresh:
		if !m.canOperate() {
			return nil
		}
		m.busy = true
		m.setMessage("Refreshing…")
		return m.refreshOperationCmd("refresh", nil)
	case keymap.CommandToggleSection:
		id := m.tree.Cursor()
		m.tree.ToggleFold(id)
		if section := m.tree.Section(id); section != nil && len(section.Children()) > 0 {
			m.foldPreferences[id] = m.tree.IsFolded(id)
		}
		return m.loadDetailCmd()
	case keymap.CommandDepth1, keymap.CommandDepth2, keymap.CommandDepth3:
		depth := int(command[len(command)-1] - '0')
		m.tree.SetGlobalDepth(depth)
		return m.loadDetailCmd()
	case keymap.CommandOpenDispatcher:
		m.transientOffset = 0
		m.setMode(modeHelp)
	case keymap.CommandScrollDown:
		m.scrollDetail(m.detailViewportHeight())
	case keymap.CommandScrollUp:
		m.scrollDetail(-m.detailViewportHeight())
	case keymap.CommandStage:
		if r, ok := m.selectedFile(rowUnstaged, rowUntracked); ok && m.canOperate() {
			return m.startOperation("stage", func(ctx context.Context) error { return m.repo.Stage(ctx, []string{r.path}) })
		}
	case keymap.CommandUnstage:
		if r, ok := m.selectedFile(rowStaged); ok && m.canOperate() {
			return m.startOperation("unstage", func(ctx context.Context) error { return m.repo.Unstage(ctx, []string{r.path}) })
		}
	case keymap.CommandStageAll:
		if m.canOperate() {
			if m.stageAll == nil {
				return m.startOperation("stage all tracked changes", nil)
			}
			return m.startOperation("stage all tracked changes", m.stageAll)
		}
	case keymap.CommandUnstageAll:
		if m.canOperate() {
			return m.startOperation("unstage all", m.unstageAll)
		}
	case keymap.CommandDiscard:
		if m.scheme == schemeMagit {
			m.beginDiscard(rowUnstaged, rowUntracked, rowStaged)
		} else {
			m.beginDiscard(rowUnstaged, rowUntracked)
		}
	case keymap.CommandCommit:
		if m.canOperate() {
			m.setMode(modeCommit)
			m.input = ""
		}
	case keymap.CommandSwitchBranch:
		if m.canOperate() {
			m.busy = true
			m.setMessage("Loading branches…")
			return m.loadBranchesCmd()
		}
	case keymap.CommandFetchUpstream:
		if m.canOperate() {
			return m.startOperation("fetch upstream", m.fetchUpstream)
		}
	case keymap.CommandFetchPush:
		if m.canOperate() {
			if m.snapshot.pushRemote != "" {
				return m.startOperation("fetch push remote", m.fetchPush)
			}
			if len(m.snapshot.remotes) == 0 {
				m.setError(errors.New("configure push remote: no remotes configured"))
				return nil
			}
			m.remoteCursor = 0
			m.remotePurpose = remoteConfigurePush
			m.setMode(modeRemotes)
		}
	case keymap.CommandFetchElsewhere:
		if m.canOperate() {
			if len(m.snapshot.remotes) == 0 {
				m.setError(errors.New("fetch elsewhere: no remotes configured"))
				return nil
			}
			m.remoteCursor = 0
			m.remotePurpose = remoteFetchElsewhere
			m.setMode(modeRemotes)
		}
	case keymap.CommandFetchAll:
		if m.canOperate() {
			return m.startOperation("fetch all", m.fetchAll)
		}
	case keymap.CommandAddRemote:
		if m.canOperate() {
			m.remoteInput, m.remoteField, m.remoteFetch = [2]string{}, 0, false
			m.setMode(modeAddRemote)
		}
	case keymap.CommandPush:
		if m.canOperate() {
			if m.snapshot.summary.Upstream != "" {
				return m.startOperation("push", m.push)
			}
			if m.snapshot.pushRemote != "" {
				remote := m.snapshot.pushRemote
				return m.startOperation("push and set upstream", func(ctx context.Context) error {
					return m.pushSetUpstream(ctx, remote)
				})
			}
			if len(m.snapshot.remotes) == 0 {
				m.setError(errors.New("push: no remotes configured"))
				return nil
			}
			m.remoteCursor = 0
			m.remotePurpose = remoteConfigureAndPush
			m.setMode(modeRemotes)
		}
	case keymap.CommandShowProcesses:
		m.openProcesses()
	case keymap.CommandQuit:
		m.shutdown()
		return tea.Quit
	default:
		m.setError(fmt.Errorf("internal error: no UI handler for %s", command))
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
	var err error
	if s.summary, err = queries.summary(ctx); err != nil {
		return s, fmt.Errorf("summary: %w", err)
	}
	if s.status, err = queries.status(ctx); err != nil {
		return s, fmt.Errorf("status: %w", err)
	}
	if queries.stashes != nil {
		if s.stashes, err = queries.stashes(ctx); err != nil {
			return s, fmt.Errorf("stashes: %w", err)
		}
	}
	if s.remotes, err = queries.remotes(ctx); err != nil {
		return s, fmt.Errorf("remotes: %w", err)
	}
	if s.pushRemote, err = queries.pushRemote(ctx); err != nil && !errors.Is(err, gitbackend.ErrNoFetchRemote) {
		return s, fmt.Errorf("push remote: %w", err)
	}
	if queries.operations != nil {
		if s.operations, err = queries.operations(ctx); err != nil {
			return s, fmt.Errorf("operation state: %w", err)
		}
	}
	if queries.sparse != nil {
		if s.sparse, err = queries.sparse(ctx); err != nil {
			return s, fmt.Errorf("sparse checkout state: %w", err)
		}
	}
	if s.recent, err = queries.recentLog(ctx, 10); err != nil {
		return s, fmt.Errorf("recent log: %w", err)
	}
	if s.summary.Upstream != "" {
		if s.upstream, err = queries.upstreamLogLimit(ctx, 257); err != nil && !errors.Is(err, gitbackend.ErrNoUpstream) {
			return s, fmt.Errorf("upstream log: %w", err)
		}
	}
	return normalizeUpstreamSnapshot(s), nil
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
