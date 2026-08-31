package ui

// Inspection workflows intentionally reuse the status detail pane.  It is a
// read-only, pageable result view already owned by Model, and diffMsg gives us
// the same cancellation/stale-result boundary as ordinary file details.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

const (
	inspectOutputLimit = 512 << 10
	inspectItemLimit   = 512
)

// These are the read-only portions of the vendored transients. Deliberately
// absent are Emacs buffer operations (save transient values, margins, buffer
// locks, hunk fontification/refinement), general mutation, reflogs, and
// external tools. Ediff display commands are adapted to terminal-safe unified
// comparisons below; the one reviewed conflict-resolution exception is wired
// separately to conflictResolutionWorkflow.
var inspectSuffixes = map[string]WorkflowHandler{
	// magit-diff
	"magit-diff-dwim":         inspectDiffDWIM,
	"magit-diff-range":        inspectDiffRange,
	"magit-diff-paths":        inspectDiffPaths,
	"magit-diff-unstaged":     inspectDiffUnstaged,
	"magit-diff-staged":       inspectDiffStaged,
	"magit-diff-working-tree": inspectDiffWorkingTree,
	"magit-show-commit":       inspectShowCommit,

	// The useful, stateless portion of magit-diff-refresh.
	"magit-diff-refresh": inspectDiffDWIM,

	// magit-ediff display commands, adapted to a unified terminal comparison.
	"magit-ediff-dwim":              inspectCompareDWIM,
	"magit-ediff-show-unstaged":     inspectCompareUnstaged,
	"magit-ediff-show-staged":       inspectCompareStaged,
	"magit-ediff-show-working-tree": inspectCompareWorkingTree,
	"magit-ediff-show-commit":       inspectCompareCommit,
	"magit-ediff-compare":           inspectCompareRange,
	"magit-ediff-show-stash":        inspectCompareStash,

	// magit-log and its portable, read-only terminal adaptations.
	"magit-log-current":           inspectLogCurrent,
	"magit-log-head":              inspectLogCurrent,
	"magit-log-other":             inspectLogOther,
	"magit-log-related":           inspectLogRelated,
	"magit-log-branches":          inspectLogBranches,
	"magit-log-all-branches":      inspectLogBranches,
	"magit-log-matching-branches": inspectLogMatchingBranches,
	"magit-log-matching-tags":     inspectLogMatchingTags,
	"magit-log-all":               inspectLogAll,
	"magit-log-reflog":            inspectReflogAll,
	"magit-reflog-current":        inspectReflogCurrent,
	"magit-reflog-other":          inspectReflogOther,
	"magit-reflog-head":           inspectReflogHead,
	"magit-shortlog-since":        inspectShortlogSince,
	"magit-shortlog-range":        inspectShortlogRange,
	"magit-log-refresh":           inspectLogCurrent,

	// magit-show-refs
	"magit-show-refs-head":    inspectRefs,
	"magit-show-refs-current": inspectRefs,
	"magit-show-refs-other":   inspectRefsOther,
}

var inspectTopLevel = map[string]WorkflowHandler{
	"magit-diff":             inspectDiffDWIM,
	"magit-diff-refresh":     inspectDiffDWIM,
	"magit-ediff-dwim":       inspectCompareDWIM,
	"magit-ediff":            inspectCompareDWIM,
	"magit-log":              inspectLogCurrent,
	"magit-log-refresh":      inspectLogCurrent,
	"magit-show-refs":        inspectRefs,
	"magit-cherry":           inspectCherries,
	"magit-describe-section": inspectSectionInfo,
}

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		handlers := make(map[keymap.CommandID]WorkflowHandler)
		for _, binding := range keymap.Registry() {
			if binding.Transient == "magit-git-mergetool" && binding.Kind == keymap.KindSuffix && binding.UpstreamCommand == "magit-git-mergetool" {
				handlers[binding.Command] = conflictResolutionWorkflow
				continue
			}
			handler := inspectSuffixes[binding.UpstreamCommand]
			// The recursively vendored dispatcher repeats top-level commands.
			// Give those occurrences the same safe behavior rather than only
			// connecting the status-map occurrence.
			if handler == nil {
				handler = inspectTopLevel[binding.UpstreamCommand]
			}
			if handler != nil {
				handlers[binding.Command] = handler
			}
		}
		// Ctrl-B is a portable terminal extension rather than a Magit status-map
		// binding; it is registered explicitly in keymap alongside both schemes.
		handlers[keymap.CommandBlame] = inspectBlame
		handlers[keymap.CommandGraph] = inspectGraph
		return handlers
	})
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-diff", map[string][]string{
		"magit-diff-dwim": {}, "magit-diff-range": {}, "magit-diff-paths": {},
		"magit-diff-unstaged": {}, "magit-diff-staged": {}, "magit-diff-working-tree": {}, "magit-show-commit": {},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-ediff", map[string][]string{
		"magit-ediff-dwim": {}, "magit-ediff-show-unstaged": {}, "magit-ediff-show-staged": {},
		"magit-ediff-show-working-tree": {}, "magit-ediff-show-commit": {}, "magit-ediff-compare": {}, "magit-ediff-show-stash": {},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-git-mergetool", map[string][]string{
		"magit-git-mergetool": {},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-log", map[string][]string{
		"magit-log-current": {}, "magit-log-other": {}, "magit-log-head": {}, "magit-log-related": {},
		"magit-log-branches": {}, "magit-log-all-branches": {}, "magit-log-all": {}, "magit-log-reflog": {},
		"magit-log-matching-branches": {}, "magit-log-matching-tags": {},
		"magit-reflog-current": {}, "magit-reflog-other": {}, "magit-reflog-head": {},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-shortlog", map[string][]string{
		"magit-shortlog-since": {"transient:magit-shortlog:--numbered", "transient:magit-shortlog:--summary", "transient:magit-shortlog:--email", "transient:magit-shortlog:--group=", "transient:magit-shortlog:--format=", "transient:magit-shortlog:-w"},
		"magit-shortlog-range": {"transient:magit-shortlog:--numbered", "transient:magit-shortlog:--summary", "transient:magit-shortlog:--email", "transient:magit-shortlog:--group=", "transient:magit-shortlog:--format=", "transient:magit-shortlog:-w"},
	})...)
	refOptions := []string{"magit-for-each-ref:--contains", "transient:magit-show-refs:--merged=", "transient:magit-show-refs:--merged", "transient:magit-show-refs:--no-merged=", "transient:magit-show-refs:--no-merged", "magit-for-each-ref:--sort"}
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-show-refs", map[string][]string{
		"magit-show-refs-head":    refOptions,
		"magit-show-refs-current": refOptions,
		"magit-show-refs-other":   refOptions,
	})...)
	// Log-refresh options are consumed by its connected read-only suffixes. Keep
	// this declaration occurrence-aware so the central transient validator can
	// pass the exact edited values to the inspection query.
	consumes := []string{"magit-log:-n", "transient:magit-log-refresh:--graph", "transient:magit-log-refresh:--decorate", "magit-log:--*-order"}
	seen := map[keymap.CommandID]bool{}
	var capabilities []WorkflowCapability
	for _, binding := range keymap.Registry() {
		if binding.Transient == "magit-log-refresh" && binding.Kind == keymap.KindSuffix && inspectSuffixes[binding.UpstreamCommand] != nil && !seen[binding.Command] {
			seen[binding.Command] = true
			capabilities = append(capabilities, WorkflowCapability{ID: binding.Command, Transient: binding.Transient, UpstreamCommand: binding.UpstreamCommand, Consumes: consumes})
		}
	}
	RegisterWorkflowCapabilities(capabilities...)
}

type inspectLoader func(context.Context) (string, error)

func loadInspection(m *Model, title string, loader inspectLoader) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	m.cancelDetail()
	m.inspectionActive, m.graphActive, m.graphEntries, m.graphCursor = true, false, nil, -1
	m.blameActive, m.blameEntries, m.blameCursor, m.blameReturn = false, nil, -1, nil
	m.conflictInspectPath, m.conflictResolution = "", ""
	m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn = false, "", nil, nil
	m.detailOffset = 0
	m.detailRequest++
	request, id := m.detailRequest, m.tree.Cursor()
	m.detailID, m.detail = id, "Loading "+sanitizeSingleLine(title)+"…"
	m.setMessage("Loading " + sanitizeSingleLine(title) + "…")
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		text, err := loader(ctx)
		if err != nil {
			// diffMsg's normal error wording is specific to file/revision details.
			// Render a typed inspection error in the same terminal-safe pane.
			text = "Inspection failed: " + err.Error()
		}
		if strings.TrimSpace(text) == "" {
			text = "No results."
		}
		return diffMsg{id: id, request: request, text: title + "\n\n" + text}
	}
}

func selectedInspectRevision(m *Model) string {
	if revision := activeInspectionRevision(m); revision != "" {
		return revision
	}
	if row, ok := m.rows[m.tree.Cursor()]; ok && row.kind == rowCommit && row.commit.ID != "" {
		return row.commit.ID
	}
	return "HEAD"
}

func activeInspectionRevision(m *Model) string {
	if m.graphActive {
		return m.graphEntries[m.graphCursor].ID
	}
	if m.revisionActive {
		return m.revisionID
	}
	if m.blameActive {
		return m.blameEntries[m.blameCursor].CommitID
	}
	return ""
}

func selectedInspectPath(m *Model) []string {
	if row, ok := m.rows[m.tree.Cursor()]; ok && row.path != "" {
		return []string{row.path}
	}
	return nil
}

func runDiffInspection(m *Model, title string, query gitbackend.DiffQuery) tea.Cmd {
	query.OutputLimit = inspectOutputLimit
	return loadInspection(m, title, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryDiff(ctx, query)
		if err != nil {
			return "", err
		}
		return truncationNote(result.Truncated) + result.Detail, nil
	})
}

func inspectDiffDWIM(m *Model, _ WorkflowCommand) tea.Cmd {
	row := m.rows[m.tree.Cursor()]
	switch row.kind {
	case rowStaged:
		return runDiffInspection(m, "Staged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffIndex, Files: selectedInspectPath(m)})
	case rowCommit:
		return inspectShowCommit(m, WorkflowCommand{})
	default:
		return runDiffInspection(m, "Unstaged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffWorktree, Files: selectedInspectPath(m)})
	}
}

func inspectDiffPaths(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Path diff", gitbackend.DiffQuery{Kind: gitbackend.DiffWorktree, Files: selectedInspectPath(m)})
}

func inspectDiffUnstaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Unstaged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffWorktree})
}

func inspectDiffStaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Staged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffIndex})
}

func inspectDiffWorkingTree(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Working tree diff", gitbackend.DiffQuery{Kind: gitbackend.DiffRevision, Base: "HEAD"})
}

func inspectDiffRange(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadInspection(m, "Revision range diff", func(ctx context.Context) (string, error) {
		base, target, err := inspectComparisonRevisions(ctx, m.repo, selectedInspectRevision(m))
		if err != nil {
			return "", err
		}
		result, err := m.repo.QueryDiff(ctx, gitbackend.DiffQuery{Kind: gitbackend.DiffRevisionRange, Base: base, Target: target, OutputLimit: inspectOutputLimit})
		return truncationNote(result.Truncated) + result.Detail, err
	})
}

func inspectShowCommit(m *Model, _ WorkflowCommand) tea.Cmd {
	return inspectRevision(m, selectedInspectRevision(m))
}

func inspectRevision(m *Model, revision string) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	m.cancelDetail()
	m.inspectionActive, m.graphActive, m.graphEntries, m.graphCursor = true, false, nil, -1
	m.revisionActive, m.revisionID, m.revisionParents = false, "", nil
	m.detailOffset = 0
	m.detailRequest++
	request, id := m.detailRequest, m.tree.Cursor()
	m.detailID, m.detail = id, "Loading commit "+sanitizeSingleLine(revision)+"…"
	m.setMessage("Loading commit " + sanitizeSingleLine(revision) + "…")
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		result, err := m.repo.QueryShowRevision(ctx, gitbackend.ShowRevisionQuery{Revision: revision, Stat: true, Patch: true, OutputLimit: inspectOutputLimit})
		if err != nil {
			return revisionMsg{id: id, request: request, err: err}
		}
		return revisionMsg{
			id: id, request: request, revision: result.Revision,
			text: "Commit " + result.Revision.ShortID + "\n\n" + truncationNote(result.Truncated) + result.Detail,
		}
	}
}

// inspectBlame displays current-worktree line provenance for the selected
// status file. It is read-only and remains in the standard cancellable detail
// pane; no editor, pager, or ambient Git configuration is invoked.
func inspectBlame(m *Model, _ WorkflowCommand) tea.Cmd {
	paths := selectedInspectPath(m)
	if len(paths) != 1 {
		m.setError(errors.New("select a tracked status file to blame"))
		return nil
	}
	path := paths[0]
	return loadBlameInspection(m, path)
}

func loadBlameInspection(m *Model, path string) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	m.cancelDetail()
	m.inspectionActive, m.blameActive, m.blameEntries, m.blameCursor = true, false, nil, -1
	m.graphActive, m.graphEntries, m.graphCursor = false, nil, -1
	m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn, m.blameReturn = false, "", nil, nil, nil
	m.detailOffset = 0
	m.detailRequest++
	request, id, title := m.detailRequest, m.tree.Cursor(), "Blame "+sanitizeSingleLine(path)
	m.detailID, m.detail = id, "Loading "+title+"…"
	m.setMessage("Loading " + title + "…")
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		result, err := m.repo.QueryBlame(ctx, gitbackend.BlameQuery{Path: path, OutputLimit: inspectOutputLimit})
		if err != nil {
			return blameMsg{id: id, request: request, title: title, err: err}
		}
		text, entries := blameResultText(result)
		return blameMsg{id: id, request: request, title: title, text: text, entries: entries}
	}
}

// inspectGraph is a bounded all-refs graph browser. Git computes lanes from
// the full ref topology, while this UI renders its sanitized ASCII graph and
// decorations in the pageable terminal detail pane.
func inspectGraph(m *Model, _ WorkflowCommand) tea.Cmd {
	query := gitbackend.LogQuery{All: true, Graph: true, Decorations: true, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit}
	return loadGraphInspection(m, "All refs graph", query)
}

func loadGraphInspection(m *Model, title string, query gitbackend.LogQuery) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	m.cancelDetail()
	m.inspectionActive, m.graphActive, m.graphEntries, m.graphCursor = true, false, nil, -1
	m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn = false, "", nil, nil
	m.detailOffset = 0
	m.detailRequest++
	request, id := m.detailRequest, m.tree.Cursor()
	title = sanitizeSingleLine(title)
	m.detailID, m.detail = id, "Loading "+title+"…"
	m.setMessage("Loading " + title + "…")
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return graphMsg{id: id, request: request, title: title, err: err}
		}
		text, entries := graphResultText(result)
		return graphMsg{id: id, request: request, title: title, text: text, entries: entries}
	}
}

func graphResultText(result gitbackend.LogResult) (string, map[int]gitbackend.LogEntry) {
	lines := make([]string, 0, len(result.Items)+1)
	entries := make(map[int]gitbackend.LogEntry, len(result.Items))
	if result.Truncated {
		lines = append(lines, strings.TrimSuffix(truncationNote(true), "\n"))
	}
	// The detail view prepends the title and a blank line, so graph rows begin
	// at line two. The map makes only real commit rows selectable.
	for _, item := range result.Items {
		entries[len(lines)+2] = item
		lines = append(lines, logEntryText(item))
	}
	return strings.Join(lines, "\n"), entries
}

func (m *Model) handleGraphKey(key string) (tea.Cmd, bool) {
	lines := make([]int, 0, len(m.graphEntries))
	for line := range m.graphEntries {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	if len(lines) == 0 {
		return nil, false
	}
	index := sort.SearchInts(lines, m.graphCursor)
	if index == len(lines) || lines[index] != m.graphCursor {
		index = 0
	}
	switch key {
	case "up", "k", "p":
		index = max(0, index-1)
	case "down", "j", "n":
		index = min(len(lines)-1, index+1)
	case "enter":
		entry := m.graphEntries[m.graphCursor]
		m.graphReturn = m.captureGraphInspection()
		m.graphActive, m.graphEntries, m.graphCursor = false, nil, -1
		return inspectRevision(m, entry.ID), true
	default:
		return nil, false
	}
	m.graphCursor = lines[index]
	m.detailOffset = min(m.graphCursor, m.detailMaximumOffset())
	m.setMessage("Graph commit " + m.graphEntries[m.graphCursor].ShortID + " selected; Enter inspects, c cherry-picks, V reverts, X resets")
	return nil, true
}

func (m *Model) handleInspectionNavigationKey(key string) (tea.Cmd, bool) {
	handlers := []struct {
		active bool
		handle func(string) (tea.Cmd, bool)
	}{
		{m.graphActive, m.handleGraphKey},
		{m.blameActive, m.handleBlameKey},
		{m.conflictInspectPath != "", m.handleConflictInspectionKey},
		{m.revisionActive, m.handleRevisionKey},
	}
	for _, candidate := range handlers {
		if candidate.active {
			if cmd, handled := candidate.handle(key); handled {
				return cmd, true
			}
		}
	}
	return nil, false
}

func (m *Model) handleRevisionKey(key string) (tea.Cmd, bool) {
	if key != "p" {
		return nil, false
	}
	if len(m.revisionParents) == 0 {
		m.setMessage("This commit has no parent")
		return nil, true
	}
	m.setMessage("Opening first parent commit")
	return inspectRevision(m, m.revisionParents[0]), true
}

func firstBlameLine(entries map[int]gitbackend.BlameLine) int {
	first := -1
	for line := range entries {
		if first < 0 || line < first {
			first = line
		}
	}
	return first
}

func (m *Model) handleBlameKey(key string) (tea.Cmd, bool) {
	lines := make([]int, 0, len(m.blameEntries))
	for line := range m.blameEntries {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	if len(lines) == 0 {
		return nil, false
	}
	index := sort.SearchInts(lines, m.blameCursor)
	if index == len(lines) || lines[index] != m.blameCursor {
		index = 0
	}
	switch key {
	case "up", "k", "p":
		index = max(0, index-1)
	case "down", "j", "n":
		index = min(len(lines)-1, index+1)
	case "enter":
		return m.openSelectedBlameCommit(), true
	default:
		return nil, false
	}
	m.blameCursor = lines[index]
	m.detailOffset = min(m.blameCursor, m.detailMaximumOffset())
	entry := m.blameEntries[m.blameCursor]
	m.setMessage(fmt.Sprintf("Blame line %d selected; Enter inspects %s", entry.Line, shortID(entry.CommitID)))
	return nil, true
}

func (m *Model) openSelectedBlameCommit() tea.Cmd {
	entry := m.blameEntries[m.blameCursor]
	if strings.Trim(entry.CommitID, "0") == "" {
		m.setMessage("Selected line has not been committed yet")
		return nil
	}
	m.blameReturn = m.captureBlameInspection()
	m.blameActive, m.blameEntries, m.blameCursor = false, nil, -1
	return inspectRevision(m, entry.CommitID)
}

func (m *Model) captureBlameInspection() *blameInspection {
	entries := make(map[int]gitbackend.BlameLine, len(m.blameEntries))
	for line, entry := range m.blameEntries {
		entries[line] = entry
	}
	return &blameInspection{detail: m.detail, id: m.detailID, entries: entries, cursor: m.blameCursor, offset: m.detailOffset}
}

func (m *Model) restoreBlameInspection() {
	if m.blameReturn == nil {
		return
	}
	state := m.blameReturn
	m.detail, m.detailID, m.detailOffset = state.detail, state.id, state.offset
	m.blameEntries, m.blameCursor, m.blameActive = state.entries, state.cursor, true
	m.revisionActive, m.revisionID, m.revisionParents, m.blameReturn = false, "", nil, nil
	m.clampDetailOffset()
}

func (m *Model) captureGraphInspection() *graphInspection {
	entries := make(map[int]gitbackend.LogEntry, len(m.graphEntries))
	for line, entry := range m.graphEntries {
		entries[line] = entry
	}
	return &graphInspection{detail: m.detail, id: m.detailID, entries: entries, cursor: m.graphCursor, offset: m.detailOffset}
}

func (m *Model) restoreGraphInspection() {
	if m.graphReturn == nil {
		return
	}
	state := m.graphReturn
	m.detail, m.detailID, m.detailOffset = state.detail, state.id, state.offset
	m.graphEntries, m.graphCursor, m.graphActive = state.entries, state.cursor, true
	m.revisionActive, m.revisionID, m.revisionParents, m.graphReturn = false, "", nil, nil
	m.clampDetailOffset()
}

func formatBlameResult(result gitbackend.BlameResult) string {
	text, _ := blameResultText(result)
	return text
}

func blameResultText(result gitbackend.BlameResult) (string, map[int]gitbackend.BlameLine) {
	lines := make([]string, 0, len(result.Lines))
	entries := make(map[int]gitbackend.BlameLine, len(result.Lines))
	if result.Truncated {
		lines = append(lines, strings.TrimSuffix(truncationNote(true), "\n"))
	}
	for _, line := range result.Lines {
		id := line.CommitID
		if len(id) > 12 {
			id = id[:12]
		}
		author := strings.TrimSpace(line.Author)
		if author == "" {
			author = "unknown"
		}
		date := "unknown date"
		if !line.AuthorTime.IsZero() {
			date = line.AuthorTime.Format("2006-01-02")
		}
		summary := strings.TrimSpace(line.Summary)
		if summary != "" {
			summary = " " + summary
		}
		entries[len(lines)+2] = line
		lines = append(lines, fmt.Sprintf("%6d  %s  %s  %s%s | %s", line.Line, id, author, date, summary, line.Content))
	}
	return strings.Join(lines, "\n"), entries
}

func inspectShowLatestStash(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadInspection(m, "Stash", func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryShowRevision(ctx, gitbackend.ShowRevisionQuery{Revision: "stash", Stat: true, Patch: true, OutputLimit: inspectOutputLimit})
		return truncationNote(result.Truncated) + result.Detail, err
	})
}

func inspectComparisonRevisions(ctx context.Context, repo *gitbackend.Repository, target string) (string, string, error) {
	revision, err := repo.ResolveRevision(ctx, target)
	if err != nil {
		return "", "", err
	}
	if len(revision.ParentIDs) == 0 {
		return "", "", fmt.Errorf("revision %s has no parent to compare", revision.ShortID)
	}
	return revision.ParentIDs[0], revision.ID, nil
}

func runUnifiedComparison(m *Model, title string, kind gitbackend.DiffKind) tea.Cmd {
	query := gitbackend.DiffQuery{Kind: kind, Files: selectedInspectPath(m), OutputLimit: inspectOutputLimit}
	if kind == gitbackend.DiffRevision {
		query.Base = "HEAD"
	}
	return runDiffInspection(m, "Terminal comparison (unified): "+title, query)
}

func inspectCompareDWIM(m *Model, _ WorkflowCommand) tea.Cmd {
	row := m.rows[m.tree.Cursor()]
	if row.path != "" && isUnmergedSnapshotPath(m, row.path) {
		return inspectConflictVersions(m, row.path)
	}
	switch row.kind {
	case rowStaged:
		return runUnifiedComparison(m, "staged", gitbackend.DiffIndex)
	case rowCommit:
		return inspectCompareCommit(m, WorkflowCommand{})
	default:
		return runUnifiedComparison(m, "unstaged", gitbackend.DiffWorktree)
	}
}
func inspectCompareUnstaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runUnifiedComparison(m, "unstaged", gitbackend.DiffWorktree)
}
func inspectCompareStaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runUnifiedComparison(m, "staged", gitbackend.DiffIndex)
}
func inspectCompareWorkingTree(m *Model, _ WorkflowCommand) tea.Cmd {
	return runUnifiedComparison(m, "working tree", gitbackend.DiffRevision)
}
func inspectCompareCommit(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectShowCommit(m, command)
}
func inspectCompareStash(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectShowLatestStash(m, command)
}
func inspectCompareRange(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectDiffRange(m, command)
}

func hasUnmergedInspectionPath(m *Model) bool {
	for _, file := range m.snapshot.status.Files {
		if file.Staged == gitbackend.ChangeUnmerged || file.Unstaged == gitbackend.ChangeUnmerged {
			return true
		}
	}
	return false
}

// E t m is wired to conflictResolutionWorkflow, a reviewed terminal-native
// resolver. We intentionally do not launch git mergetool or an ambient editor.

func logQueryFromCommand(command WorkflowCommand) gitbackend.LogQuery {
	query := gitbackend.LogQuery{Limit: 128, Graph: true, Decorations: true, OutputLimit: inspectOutputLimit}
	if value, ok := inspectOption(command, "magit-log:-n"); ok && value.Value != "" {
		if n, err := strconv.Atoi(value.Value); err == nil && n > 0 {
			query.Limit = min(n, inspectItemLimit)
		}
	}
	if value, ok := inspectOption(command, "transient:magit-log-refresh:--graph"); ok {
		query.Graph = value.Enabled
	}
	if value, ok := inspectOption(command, "transient:magit-log-refresh:--decorate"); ok {
		query.Decorations = value.Enabled
	}
	if value, ok := inspectOption(command, "magit-log:--*-order"); ok {
		switch value.Value {
		case "date", "--date-order":
			query.Order = gitbackend.LogOrderDate
		case "author-date", "--author-date-order":
			query.Order = gitbackend.LogOrderAuthorDate
		case "topo", "--topo-order":
			query.Order = gitbackend.LogOrderTopo
		}
	}
	return query
}

func inspectOption(command WorkflowCommand, upstream string) (OptionValue, bool) {
	for _, binding := range keymap.Registry() {
		if binding.UpstreamCommand == upstream {
			if value, ok := command.Options[binding.Command]; ok {
				return value, true
			}
		}
	}
	return OptionValue{}, false
}

func logResultText(result gitbackend.LogResult) string {
	lines := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		lines = append(lines, logEntryText(item))
	}
	return truncationNote(result.Truncated) + strings.Join(lines, "\n")
}

func logEntryText(item gitbackend.LogEntry) string {
	line := strings.TrimSpace(item.Graph + " " + item.ShortID)
	if item.Decorations != "" {
		line += " (" + item.Decorations + ")"
	}
	return line + " " + item.Subject
}

func runLogInspection(m *Model, title string, query gitbackend.LogQuery) tea.Cmd {
	return loadGraphInspection(m, title, query)
}

func inspectLogCurrent(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	query.Revision = selectedInspectRevision(m)
	return runLogInspection(m, "Log", query)
}

func inspectLogReflog(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	query.Reflog = true
	return runLogInspection(m, "Log reflog objects", query)
}

func reflogResultText(result gitbackend.ReflogResult) string {
	lines := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		lines = append(lines, item.ShortID+" "+item.Selector+" "+item.Subject+"  "+item.AuthorName)
	}
	return truncationNote(result.Truncated) + strings.Join(lines, "\n")
}

func runReflogInspection(m *Model, title string, query gitbackend.ReflogQuery) tea.Cmd {
	query.OutputLimit = inspectOutputLimit
	return loadInspection(m, title, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryReflog(ctx, query)
		if err != nil {
			return "", err
		}
		return reflogResultText(result), nil
	})
}

func inspectReflogAll(m *Model, _ WorkflowCommand) tea.Cmd {
	return runReflogInspection(m, "All reflogs", gitbackend.ReflogQuery{All: true, Limit: 256})
}

func inspectReflogCurrent(m *Model, _ WorkflowCommand) tea.Cmd {
	revision := m.snapshot.summary.Branch
	if revision == "" || m.snapshot.summary.Detached {
		revision = "HEAD"
	}
	return runReflogInspection(m, "Reflog "+revision, gitbackend.ReflogQuery{Revision: revision, Limit: 256})
}

func inspectReflogHead(m *Model, _ WorkflowCommand) tea.Cmd {
	return runReflogInspection(m, "HEAD reflog", gitbackend.ReflogQuery{Revision: "HEAD", Limit: 256})
}

func inspectReflogOther(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "Reflog for another ref", Fields: []WorkflowField{{Name: "revision", Label: "Ref or revision", Kind: WorkflowText, Value: "HEAD", Required: true}}, Run: func(values WorkflowValues) tea.Cmd {
		revision := strings.TrimSpace(values["revision"])
		return runReflogInspection(m, "Reflog "+revision, gitbackend.ReflogQuery{Revision: revision, Limit: 256})
	}})
}

func shortlogQueryFromCommand(command WorkflowCommand) (gitbackend.ShortlogQuery, error) {
	query := gitbackend.ShortlogQuery{Numbered: true, Summary: true, OutputLimit: inspectOutputLimit}
	for _, binding := range keymap.BindingsForTransient(schemeID(schemeMagit), "magit-shortlog") {
		value, ok := command.Options[binding.Command]
		if !ok {
			continue
		}
		if err := applyShortlogOption(&query, binding.UpstreamCommand, value); err != nil {
			return query, err
		}
	}
	return query, nil
}

func applyShortlogOption(query *gitbackend.ShortlogQuery, upstream string, value OptionValue) error {
	switch upstream {
	case "transient:magit-shortlog:--numbered":
		query.Numbered = value.Enabled
	case "transient:magit-shortlog:--summary":
		query.Summary = value.Enabled
	case "transient:magit-shortlog:--email":
		query.Email = value.Enabled
	case "transient:magit-shortlog:--group=":
		query.Group = value.Value
	case "transient:magit-shortlog:--format=":
		query.Format = value.Value
	case "transient:magit-shortlog:-w":
		return applyShortlogWrap(query, value.Value)
	default:
		return fmt.Errorf("unsupported shortlog option %s", upstream)
	}
	return nil
}

func applyShortlogWrap(query *gitbackend.ShortlogQuery, value string) error {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > 3 {
		return errors.New("shortlog wrap accepts width and at most two indents")
	}
	parsers := []func(string) error{
		func(part string) error { return setShortlogWidth(query, part) },
		func(part string) error {
			return setShortlogIndent(&query.WrapIndent1, &query.WrapIndent1Set, "first", part)
		},
		func(part string) error {
			return setShortlogIndent(&query.WrapIndent2, &query.WrapIndent2Set, "second", part)
		},
	}
	for index, part := range parts {
		if part != "" {
			if err := parsers[index](part); err != nil {
				return err
			}
		}
	}
	return nil
}

func setShortlogWidth(query *gitbackend.ShortlogQuery, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return errors.New("shortlog width must be a positive integer")
	}
	query.WrapWidth = n
	return nil
}

func setShortlogIndent(indent *int, set *bool, name, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return fmt.Errorf("shortlog %s indent must be a non-negative integer", name)
	}
	*indent, *set = n, true
	return nil
}

func runShortlogInspection(m *Model, title string, query gitbackend.ShortlogQuery) tea.Cmd {
	return loadInspection(m, title, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryShortlog(ctx, query)
		if err != nil {
			return "", err
		}
		return truncationNote(result.Truncated) + result.Detail, nil
	})
}

func inspectShortlogSince(m *Model, command WorkflowCommand) tea.Cmd {
	query, err := shortlogQueryFromCommand(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.OpenWorkflow(WorkflowDialog{Title: "Shortlog since revision", Fields: []WorkflowField{{Name: "revision", Label: "Revision", Kind: WorkflowText, Value: "HEAD", Required: true}}, Run: func(values WorkflowValues) tea.Cmd {
		selected := query
		selected.Revision = strings.TrimSpace(values["revision"])
		selected.Since = true
		return runShortlogInspection(m, "Shortlog "+selected.Revision, selected)
	}})
}

func inspectShortlogRange(m *Model, command WorkflowCommand) tea.Cmd {
	query, err := shortlogQueryFromCommand(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.OpenWorkflow(WorkflowDialog{Title: "Shortlog revision or range", Fields: []WorkflowField{{Name: "revision", Label: "Revision or range", Kind: WorkflowText, Value: "HEAD", Required: true}}, Run: func(values WorkflowValues) tea.Cmd {
		selected := query
		selected.Range = strings.TrimSpace(values["revision"])
		return runShortlogInspection(m, "Shortlog "+selected.Range, selected)
	}})
}

func inspectLogOther(m *Model, command WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "Log another revision", Fields: []WorkflowField{{Name: "revision", Label: "Revision", Kind: WorkflowText, Value: "HEAD", Required: true}}, Run: func(values WorkflowValues) tea.Cmd {
		query := logQueryFromCommand(command)
		query.Revision = strings.TrimSpace(values["revision"])
		return runLogInspection(m, "Log "+query.Revision, query)
	}})
}

func inspectLogMatchingRefs(m *Model, command WorkflowCommand, tags bool) tea.Cmd {
	kind := "branches"
	if tags {
		kind = "tags"
	}
	return m.OpenWorkflow(WorkflowDialog{Title: "Log matching " + kind, Fields: []WorkflowField{{Name: "pattern", Label: "Git shell glob", Kind: WorkflowText, Required: true}}, Run: func(values WorkflowValues) tea.Cmd {
		pattern := strings.TrimSpace(values["pattern"])
		query := logQueryFromCommand(command)
		if tags {
			query.TagPattern = pattern
		} else {
			query.BranchPattern = pattern
		}
		return runLogInspection(m, "Log matching "+kind, query)
	}})
}

func inspectLogMatchingBranches(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectLogMatchingRefs(m, command, false)
}

func inspectLogMatchingTags(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectLogMatchingRefs(m, command, true)
}

func inspectLogAll(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	query.All = true
	return runLogInspection(m, "Log all refs", query)
}

func inspectLogBranches(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	return loadInspection(m, "Branch log", func(ctx context.Context) (string, error) {
		refs, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		for _, ref := range refs.Local {
			query.Revisions = append(query.Revisions, ref.FullName)
		}
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			lines = append(lines, item.ShortID+" "+item.Subject)
		}
		return truncationNote(refs.Truncated || result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectLogRelated(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	focus := selectedInspectRevision(m)
	return loadInspection(m, "Related log", func(ctx context.Context) (string, error) {
		refs, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Focus: focus, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		if refs.Focus == nil || refs.Focus.Upstream == "" {
			return "", fmt.Errorf("current revision has no configured upstream")
		}
		query.From, query.To, query.Symmetric = refs.Focus.Upstream, focus, true
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			lines = append(lines, item.ShortID+" "+item.Subject)
		}
		return truncationNote(result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func refsResultText(result gitbackend.RefResult) string {
	var lines []string
	appendRefs := func(label string, refs []gitbackend.Ref) {
		if len(refs) == 0 {
			return
		}
		lines = append(lines, label)
		for _, ref := range refs {
			mark := "  "
			if ref.Current {
				mark = "* "
			}
			lines = append(lines, mark+ref.Name+" "+shortInspectID(ref.ID)+" "+ref.Subject)
		}
	}
	appendRefs("Local branches", result.Local)
	appendRefs("Remote branches", result.Remote)
	appendRefs("Tags", result.Tags)
	return truncationNote(result.Truncated) + strings.Join(lines, "\n")
}

func refSortFromOption(value string) (gitbackend.RefSort, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "--sort=")
	sorts := map[string]gitbackend.RefSort{
		"refname": gitbackend.RefSortName, "-refname": gitbackend.RefSortNameReverse,
		"version:refname": gitbackend.RefSortVersionName, "-version:refname": gitbackend.RefSortVersionNameReverse,
		"creatordate": gitbackend.RefSortCreatorDate, "-creatordate": gitbackend.RefSortCreatorDateReverse,
		"authordate": gitbackend.RefSortAuthorDate, "-authordate": gitbackend.RefSortAuthorDateReverse,
		"committerdate": gitbackend.RefSortCommitterDate, "-committerdate": gitbackend.RefSortCommitterDateReverse,
		"taggerdate": gitbackend.RefSortTaggerDate, "-taggerdate": gitbackend.RefSortTaggerDateReverse,
		"objecttype": gitbackend.RefSortObjectType, "-objecttype": gitbackend.RefSortObjectTypeReverse,
		"subject": gitbackend.RefSortSubject, "-subject": gitbackend.RefSortSubjectReverse,
	}
	sort, ok := sorts[value]
	if !ok {
		return gitbackend.RefSortName, fmt.Errorf("unsupported ref sort %q", value)
	}
	return sort, nil
}

func refQueryFromCommand(command WorkflowCommand, focus string) (gitbackend.RefQuery, error) {
	query := gitbackend.RefQuery{Focus: focus, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit}
	for _, binding := range keymap.BindingsForTransient(schemeID(schemeMagit), "magit-show-refs") {
		value, ok := command.Options[binding.Command]
		if !ok || !value.Enabled && value.Value == "" {
			continue
		}
		switch binding.UpstreamCommand {
		case "magit-for-each-ref:--contains":
			query.Contains = value.Value
		case "transient:magit-show-refs:--merged=":
			query.MergedTo = value.Value
		case "transient:magit-show-refs:--merged":
			query.MergedTo = "HEAD"
		case "transient:magit-show-refs:--no-merged=":
			query.NoMergedTo = value.Value
		case "transient:magit-show-refs:--no-merged":
			query.NoMergedTo = "HEAD"
		case "magit-for-each-ref:--sort":
			sort, err := refSortFromOption(value.Value)
			if err != nil {
				return query, err
			}
			query.Sort = sort
		default:
			return query, fmt.Errorf("unsupported refs option %s", binding.UpstreamCommand)
		}
	}
	return query, nil
}

func runRefsInspection(m *Model, title string, command WorkflowCommand, focus string) tea.Cmd {
	query, err := refQueryFromCommand(command, focus)
	if err != nil {
		m.setError(err)
		return nil
	}
	return loadInspection(m, title, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryRefs(ctx, query)
		if err != nil {
			return "", err
		}
		return refsResultText(result), nil
	})
}

func inspectRefsHead(m *Model, command WorkflowCommand) tea.Cmd {
	return runRefsInspection(m, "References compared with HEAD", command, "HEAD")
}

func inspectRefsCurrent(m *Model, command WorkflowCommand) tea.Cmd {
	focus := m.snapshot.summary.Branch
	if focus == "" || m.snapshot.summary.Detached {
		focus = "HEAD"
	}
	return runRefsInspection(m, "References compared with "+focus, command, focus)
}

func inspectRefs(m *Model, command WorkflowCommand) tea.Cmd {
	focus := selectedInspectRevision(m)
	query, err := refQueryFromCommand(command, focus)
	if err != nil {
		m.setError(err)
		return nil
	}
	return loadInspection(m, "References", func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryRefs(ctx, query)
		if err != nil {
			return "", err
		}
		return refsResultText(result), nil
	})
}

func inspectRefsOther(m *Model, command WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "References for another revision", Fields: []WorkflowField{{Name: "revision", Label: "Revision", Kind: WorkflowText, Value: "HEAD", Required: true}}, Run: func(values WorkflowValues) tea.Cmd {
		focus := strings.TrimSpace(values["revision"])
		query, err := refQueryFromCommand(command, focus)
		if err != nil {
			m.setError(err)
			return nil
		}
		return loadInspection(m, "References for "+focus, func(ctx context.Context) (string, error) {
			result, err := m.repo.QueryRefs(ctx, query)
			if err != nil {
				return "", err
			}
			return refsResultText(result), nil
		})
	}})
}

func inspectCherries(m *Model, _ WorkflowCommand) tea.Cmd {
	head := selectedInspectRevision(m)
	return loadInspection(m, "Cherries", func(ctx context.Context) (string, error) {
		refs, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Focus: head, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		upstream := ""
		if refs.Focus != nil {
			upstream = refs.Focus.Upstream
		}
		if upstream == "" {
			revision, resolveErr := m.repo.ResolveRevision(ctx, head)
			if resolveErr != nil {
				return "", resolveErr
			}
			if len(revision.ParentIDs) == 0 {
				return "", fmt.Errorf("current revision has no upstream or parent")
			}
			upstream = revision.ParentIDs[0]
		}
		result, err := m.repo.QueryCherry(ctx, gitbackend.CherryQuery{Upstream: upstream, Head: head, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			mark := "+"
			if item.Equivalent {
				mark = "-"
			}
			lines = append(lines, mark+" "+shortInspectID(item.ID)+" "+item.Subject)
		}
		return truncationNote(result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectSectionInfo(m *Model, _ WorkflowCommand) tea.Cmd {
	row, section := m.rows[m.tree.Cursor()], m.tree.Section(m.tree.Cursor())
	title, folded := "", false
	if section != nil {
		title, folded = section.Title(), m.tree.IsFolded(row.id)
	}
	return loadInspection(m, "Section information", func(ctx context.Context) (string, error) {
		state, err := m.repo.QueryOperationState(ctx)
		if err != nil {
			return "", err
		}
		lines := []string{fmt.Sprintf("Section: %s", row.section), fmt.Sprintf("Depth: %d", row.depth)}
		if title != "" {
			lines = append(lines, "Title: "+title, fmt.Sprintf("Folded: %t", folded))
		}
		if row.path != "" {
			lines = append(lines, "Path: "+row.path)
		}
		if row.commit.ID != "" {
			revision, resolveErr := m.repo.ResolveRevision(ctx, row.commit.ID)
			if resolveErr != nil {
				return "", resolveErr
			}
			lines = append(lines, "Revision: "+revision.ID, "Subject: "+revision.Subject, "Author: "+revision.AuthorName+" <"+revision.AuthorEmail+">")
		}
		if !state.InProgress() {
			lines = append(lines, "Repository operation: none")
		} else {
			lines = append(lines, fmt.Sprintf("Repository operations in progress: %d", len(state.Items)))
		}
		return strings.Join(lines, "\n"), nil
	})
}

func truncationNote(truncated bool) string {
	if truncated {
		return "[result truncated]\n"
	}
	return ""
}

func shortInspectID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
