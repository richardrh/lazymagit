package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/richardrh/lazymagit/internal/keymap"
	sectionmodel "github.com/richardrh/lazymagit/internal/model"
)

const defaultDiffContext = 3

var statusJumpSections = map[string]sectionmodel.SectionID{
	"magit-jump-to-stashes":                  "status/stashes",
	"magit-jump-to-untracked":                "status/untracked",
	"magit-jump-to-tracked":                  "status/unstaged",
	"magit-jump-to-unstaged":                 "status/unstaged",
	"magit-jump-to-staged":                   "status/staged",
	"magit-jump-to-unpulled-from-upstream":   "status/unpulled",
	"magit-jump-to-unpulled-from-pushremote": "status/unpulled",
	"magit-jump-to-unpushed-to-upstream":     "status/unpushed",
	"magit-jump-to-unpushed-to-pushremote":   "status/unpushed",
}

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		out := map[keymap.CommandID]WorkflowHandler{}
		for _, binding := range keymap.Registry() {
			section, ok := statusJumpSections[binding.UpstreamCommand]
			if (binding.Scheme != keymap.SchemeMagit && binding.Scheme != keymap.SchemeDoom) || !ok || binding.Transient != "magit-status-jump" {
				continue
			}
			targetSection := section
			out[binding.Command] = func(m *Model, _ WorkflowCommand) tea.Cmd {
				if m.tree.Section(targetSection) == nil {
					m.setMessage("Status section is not present")
					return nil
				}
				return m.finishNavigationMove(m.tree.SetCursor(targetSection))
			}
		}
		return out
	})
}

// handleNavigationKey dispatches a single, already-classified portable
// top-level binding. Sequence prefixes remain owned by the resolver.
func (m *Model) handleNavigationKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.mode != modeStatus || m.resolver.PendingPrefix() != "" {
		return nil, false
	}
	binding, ok := keymap.Find(keymap.SchemeDoom, keymap.ContextStatus, msg.String())
	if !ok || binding.Handler != keymap.HandlerExecute || !navigationCommand(binding.Command) {
		return nil, false
	}
	return m.performNavigationCommand(binding.Command)
}

func navigationCommand(command keymap.CommandID) bool {
	switch command {
	case keymap.CommandSectionCycle, keymap.CommandSectionCycleGlobal,
		keymap.CommandSectionParent, keymap.CommandSiblingPrevious, keymap.CommandSiblingNext,
		keymap.CommandLocalDepth1, keymap.CommandLocalDepth2, keymap.CommandLocalDepth3, keymap.CommandLocalDepth4,
		keymap.CommandGlobalDepth1, keymap.CommandGlobalDepth2, keymap.CommandGlobalDepth3, keymap.CommandGlobalDepth4,
		keymap.CommandSectionOpen, keymap.CommandSectionClose,
		keymap.CommandSectionOpenRecursive, keymap.CommandSectionCloseRecursive,
		keymap.CommandViewportTop, keymap.CommandViewportCenter, keymap.CommandViewportBottom,
		keymap.CommandLineStart, keymap.CommandLineEnd,
		keymap.CommandHalfPageDown, keymap.CommandHalfPageUp,
		keymap.CommandPageDown, keymap.CommandPageUp,
		keymap.CommandVisitThing, keymap.CommandCycleDiffs, keymap.CommandDetailBackward,
		keymap.CommandDiffMoreContext, keymap.CommandDiffLessContext, keymap.CommandDiffDefaultContext,
		keymap.CommandDescribeSection, keymap.CommandStatusJump, keymap.CommandDisplayRepository,
		keymap.CommandCopyThing, keymap.CommandCopySectionValue, keymap.CommandCopyBufferRevision,
		keymap.CommandEditThing, keymap.CommandBrowseThing, keymap.CommandNextReference:
		return true
	default:
		return false
	}
}

// performNavigationCommand implements non-mutating status commands. It returns
// false for commands owned by another domain, making it suitable as a fallback
// from Model.perform.
func (m *Model) performNavigationCommand(command keymap.CommandID) (tea.Cmd, bool) {
	if cmd, handled := m.performFoldNavigation(command); handled {
		return cmd, true
	}
	if cmd, handled := m.performDepthNavigation(command); handled {
		return cmd, true
	}
	if cmd, handled := m.performViewportNavigation(command); handled {
		return cmd, true
	}
	if cmd, handled := m.performTerminalNavigation(command); handled {
		return cmd, true
	}
	return m.performBasicNavigation(command)
}

func (m *Model) performViewportNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandLineStart:
		m.horizontalOffset = 0
		return nil, true
	case keymap.CommandLineEnd:
		m.horizontalOffset = m.maximumHorizontalOffset(m.lineViewportWidth())
		return nil, true
	case keymap.CommandHalfPageDown:
		return m.move(max(1, m.statusPanelRows()/2)), true
	case keymap.CommandHalfPageUp:
		return m.move(-max(1, m.statusPanelRows()/2)), true
	case keymap.CommandPageDown:
		return m.move(max(1, m.statusPanelRows())), true
	case keymap.CommandPageUp:
		return m.move(-max(1, m.statusPanelRows())), true
	case keymap.CommandViewportTop, keymap.CommandViewportCenter, keymap.CommandViewportBottom:
		ids, cursor := m.visibleStatusCursor()
		height := max(1, m.statusPanelRows())
		offset := cursor
		switch command {
		case keymap.CommandViewportCenter:
			offset = cursor - height/2
		case keymap.CommandViewportBottom:
			offset = cursor - height + 1
		}
		m.statusViewportOffset = min(max(0, offset), max(0, len(ids)-height))
		return nil, true
	default:
		return nil, false
	}
}

func (m *Model) statusPanelRows() int {
	body := max(1, m.height-4)
	if m.mode == modeProcess && m.processPanelHeight() > 0 {
		body = max(1, body-m.processPanelHeight()-1)
	}
	if m.compact {
		return body
	}
	if m.width >= 96 || body < 7 {
		return max(1, body-2)
	}
	panelHeight := body - 1
	statusHeight := max(3, panelHeight*55/100)
	return max(1, min(statusHeight, panelHeight-3)-2)
}

func (m *Model) lineViewportWidth() int {
	if m.compact {
		return max(1, m.width-max(30, m.width*38/100)-1)
	}
	if m.width >= 96 {
		return max(1, m.width-max(36, m.width*43/100)-2)
	}
	return max(1, m.width-2)
}

func (m *Model) maximumHorizontalOffset(width int) int {
	maximum := 0
	for _, line := range m.detailLines() {
		maximum = max(maximum, ansi.StringWidth(line)-width)
	}
	return max(0, maximum)
}

func (m *Model) performBasicNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	if cmd, handled := m.performCursorNavigation(command); handled {
		return cmd, true
	}
	if cmd, handled := m.performDetailNavigation(command); handled {
		return cmd, true
	}
	return m.performStatusNavigation(command)
}

func (m *Model) performCursorNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandSectionParent:
		return m.finishNavigationMove(m.tree.MoveToParent()), true
	case keymap.CommandSiblingPrevious:
		return m.moveSibling(-1), true
	case keymap.CommandSiblingNext:
		return m.moveSibling(1), true
	case keymap.CommandVisitThing:
		return m.visitSelectedThing(), true
	default:
		return nil, false
	}
}

func (m *Model) moveSibling(direction int) tea.Cmd {
	if _, file := m.selectedFile(rowUnstaged, rowStaged); file && m.detailID == m.tree.Cursor() && strings.Contains(m.detail, "\n@@") {
		m.scrollDetailHunk(direction)
		return nil
	}
	if direction < 0 {
		return m.finishNavigationMove(m.tree.MoveToPreviousSibling())
	}
	return m.finishNavigationMove(m.tree.MoveToNextSibling())
}

func (m *Model) performDetailNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandCycleDiffs:
		return m.cycleSelectedDetail(), true
	case keymap.CommandDetailBackward:
		m.scrollDetail(-max(1, m.detailViewportHeight()))
		return nil, true
	case keymap.CommandDiffLessContext:
		return m.adjustDiffContext(-3, "Showing less diff context"), true
	case keymap.CommandDiffMoreContext:
		return m.adjustDiffContext(3, "Showing more diff context"), true
	case keymap.CommandDiffDefaultContext:
		return m.resetDiffContext(), true
	default:
		return nil, false
	}
}

func (m *Model) performStatusNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandDescribeSection:
		m.describeSelectedSection()
		return nil, true
	case keymap.CommandStatusJump:
		return m.jumpToNextStatusSection(), true
	case keymap.CommandDisplayRepository:
		m.displayRepositoryStatus()
		return nil, true
	case keymap.CommandNextReference:
		return m.nextVisibleReference(), true
	default:
		return nil, false
	}
}

func (m *Model) performTerminalNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case keymap.CommandCopyLine:
		return m.copySelectedLine(), true
	case keymap.CommandCopyThing, keymap.CommandCopySectionValue:
		return m.copySelectedSection(), true
	case keymap.CommandCopyBufferRevision:
		return m.copySelectedRevision(), true
	case keymap.CommandEditThing:
		return m.openSelectedTerminalThing("Edit"), true
	case keymap.CommandBrowseThing:
		return m.openSelectedTerminalThing("Browse"), true
	default:
		return nil, false
	}
}

func (m *Model) performFoldNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	var changed bool
	switch command {
	case keymap.CommandSectionCycle:
		changed = m.tree.CycleLocal()
	case keymap.CommandSectionCycleGlobal:
		changed = m.tree.CycleGlobal()
	case keymap.CommandSectionOpen:
		changed = m.tree.OpenSelected()
	case keymap.CommandSectionClose:
		changed = m.tree.CloseSelected()
	case keymap.CommandSectionOpenRecursive:
		changed = m.tree.OpenSelectedRecursive()
	case keymap.CommandSectionCloseRecursive:
		changed = m.tree.CloseSelectedRecursive()
	default:
		return nil, false
	}
	m.finishFoldNavigation(changed)
	return m.loadDetailCmd(), true
}

func (m *Model) performDepthNavigation(command keymap.CommandID) (tea.Cmd, bool) {
	local := command >= keymap.CommandLocalDepth1 && command <= keymap.CommandLocalDepth4
	global := command >= keymap.CommandGlobalDepth1 && command <= keymap.CommandGlobalDepth4
	if !local && !global {
		return nil, false
	}
	depth := int(command[len(command)-1] - '0')
	changed := m.tree.RevealGlobalDepth(depth)
	if local {
		changed = m.tree.RevealSelectedDepth(depth)
	}
	m.finishFoldNavigation(changed)
	return m.loadDetailCmd(), true
}

func (m *Model) finishFoldNavigation(changed bool) {
	if !changed {
		return
	}
	m.rememberNavigationFolds()
	m.bumpState()
}

func (m *Model) finishNavigationMove(moved bool) tea.Cmd {
	if moved {
		m.statusViewportOffset = -1
		m.horizontalOffset = 0
		m.bumpState()
	}
	return m.loadDetailCmd()
}

func (m *Model) rememberNavigationFolds() {
	var walk func([]*sectionmodel.Section)
	walk = func(sections []*sectionmodel.Section) {
		for _, section := range sections {
			if section == nil {
				continue
			}
			if len(section.Children()) > 0 {
				m.foldPreferences[section.ID()] = m.tree.IsFolded(section.ID())
			}
			walk(section.Children())
		}
	}
	walk(m.tree.Sections())
}

func (m *Model) visitSelectedThing() tea.Cmd {
	id := m.tree.Cursor()
	section := m.tree.Section(id)
	if section == nil {
		m.setMessage("Nothing to visit")
		return nil
	}
	if len(section.Children()) > 0 {
		m.tree.ToggleFold(id)
		m.foldPreferences[id] = m.tree.IsFolded(id)
		m.bumpState()
		return m.loadDetailCmd()
	}
	if r, ok := m.rows[id]; ok && (r.kind == rowUntracked || r.kind == rowUnstaged || r.kind == rowStaged || r.kind == rowCommit || r.kind == rowStash) {
		// Terminal rows are visited in the terminal detail pane. Do not launch an
		// editor, browser, or other ambient process from the status UI.
		m.detailHidden = false
		m.setMessage("Opened " + sanitizeSingleLine(section.Title()))
		return m.loadDetailCmd()
	}
	m.setMessage("Nothing to visit")
	return nil
}

// cycleSelectedDetail is the safe terminal adaptation of cycling inline Magit
// diff sections: the selected terminal row's detail pane is hidden or shown
// without changing repository state or spawning an external viewer.
func (m *Model) cycleSelectedDetail() tea.Cmd {
	r, ok := m.rows[m.tree.Cursor()]
	if !ok || (r.kind != rowUnstaged && r.kind != rowStaged && r.kind != rowCommit && r.kind != rowStash) {
		m.setMessage("No terminal diff to cycle")
		return nil
	}
	m.detailHidden = !m.detailHidden
	m.detailOffset = 0
	m.setMessage(map[bool]string{true: "Diff hidden", false: "Diff shown"}[m.detailHidden])
	return nil
}

// openSelectedTerminalThing adapts Magit's remappable edit/browse placeholders
// to the status pane. It deliberately does not consult EDITOR, BROWSER, or a
// URL handler: opening an ambient process from a TUI is neither observable nor
// safely cancellable here.
func (m *Model) openSelectedTerminalThing(action string) tea.Cmd {
	r, ok := m.rows[m.tree.Cursor()]
	if !ok || (r.kind != rowUntracked && r.kind != rowUnstaged && r.kind != rowStaged && r.kind != rowCommit && r.kind != rowStash) {
		m.setMessage("No terminal item to " + strings.ToLower(action))
		return nil
	}
	m.detailHidden = false
	m.setMessage(action + " opened selected item in terminal detail (read-only)")
	return m.loadDetailCmd()
}

// nextVisibleReference follows the rendered status rows rather than parsing
// arbitrary detail text. Commit decorations are the only typed Git references
// represented in this status buffer; folded rows are not currently visible.
func (m *Model) nextVisibleReference() tea.Cmd {
	ids := m.tree.VisibleSectionIDs()
	current := -1
	for i, id := range ids {
		if id == m.tree.Cursor() {
			current = i
			break
		}
	}
	for _, id := range ids[current+1:] {
		r := m.rows[id]
		if r.kind == rowCommit && strings.TrimSpace(r.commit.Refs) != "" {
			m.setMessage("Next visible reference: " + sanitizeSingleLine(r.commit.Refs))
			return m.finishNavigationMove(m.tree.SetCursor(id))
		}
	}
	m.setMessage("No more visible references")
	return nil
}

func (m *Model) jumpToNextStatusSection() tea.Cmd {
	ids := m.tree.VisibleSectionIDs()
	if len(ids) == 0 {
		return nil
	}
	current := 0
	for i, id := range ids {
		if id == m.tree.Cursor() {
			current = i
			break
		}
	}
	for step := 1; step <= len(ids); step++ {
		id := ids[(current+step)%len(ids)]
		if m.rows[id].kind == rowHeading {
			return m.finishNavigationMove(m.tree.SetCursor(id))
		}
	}
	return nil
}

func (m *Model) describeSelectedSection() {
	id := m.tree.Cursor()
	section := m.tree.Section(id)
	if section == nil {
		m.detail = "No section selected."
		return
	}
	r := m.rows[id]
	m.detailID = id
	m.detailOffset = 0
	m.detail = fmt.Sprintf("Section information\n\nTitle: %s\nID: %s\nDepth: %d\nChildren: %d\nFolded: %t",
		sanitizeSingleLine(section.Title()), id, r.depth, len(section.Children()), m.tree.IsFolded(id))
	m.setMessage("Section information")
}

func (m *Model) displayRepositoryStatus() {
	summary := m.snapshot.summary
	branch := sanitizeSingleLine(summary.Branch)
	if branch == "" {
		branch = "(unknown)"
	}
	m.detailOffset = 0
	m.detail = fmt.Sprintf("Repository status\n\nBranch: %s\nHEAD: %s\nUpstream: %s\nAhead: %d\nBehind: %d",
		branch, sanitizeSingleLine(summary.Head), sanitizeSingleLine(summary.Upstream), summary.Ahead, summary.Behind)
	m.setMessage("Repository status")
}

func (m *Model) copySelectedSection() tea.Cmd {
	id := m.tree.Cursor()
	r, ok := m.rows[id]
	value := ""
	if ok {
		switch {
		case r.path != "":
			value = r.path
		case r.kind == rowCommit:
			value = r.commit.ID
		case r.kind == rowStash:
			value = r.stash.ID
		}
	}
	if value == "" {
		if section := m.tree.Section(id); section != nil {
			value = section.Title()
		}
	}
	if value == "" {
		m.setMessage("Nothing to copy")
		return nil
	}
	m.setMessage("Section value copied with OSC52")
	return tea.SetClipboard(value)
}
func (m *Model) copySelectedLine() tea.Cmd {
	id := m.tree.Cursor()
	section := m.tree.Section(id)
	if section == nil {
		m.setMessage("Nothing to copy")
		return nil
	}
	line := m.statusRowPrefix(id, m.rows[id]) + section.Title()
	if m.inspectionActive || m.detailLine >= 0 {
		index := m.detailOffset
		switch {
		case m.graphActive:
			index = m.graphCursor
		case m.blameActive:
			index = m.blameCursor
		case m.detailLine >= 0:
			index = m.detailLine
		}
		lines := m.detailLines()
		if index >= 0 && index < len(lines) {
			line = lines[index]
		}
	}
	m.setMessage("Displayed line copied with OSC52")
	return tea.SetClipboard(sanitizeSingleLine(line) + "\n")
}

func (m *Model) copySelectedRevision() tea.Cmd {
	revision := ""
	if r, ok := m.rows[m.tree.Cursor()]; ok && r.kind == rowCommit {
		revision = r.commit.ID
	}
	if revision == "" {
		revision = m.snapshot.summary.Head
	}
	if revision == "" {
		m.setMessage("No revision to copy")
		return nil
	}
	m.setMessage("Revision copied with OSC52")
	return tea.SetClipboard(revision)
}

// adjustDiffContext reloads only selected file diffs with a literal Git
// --unified value. This is the terminal-safe equivalent of Magit's context
// controls. Non-file details have no authoritative source to reload, so -
// falls back to removing already-rendered context without touching Git state.
func (m *Model) adjustDiffContext(delta int, message string) tea.Cmd {
	r, isFile := m.rows[m.tree.Cursor()]
	if isFile && (r.kind == rowUnstaged || r.kind == rowStaged) && m.repo != nil {
		m.diffContext = max(0, m.diffContext+delta)
		m.setMessage(fmt.Sprintf("%s (%d lines)", message, m.diffContext))
		return m.loadDetailCmd()
	}
	if delta < 0 {
		if reduced, ok := lessDiffContext(m.detail); ok {
			m.detail = reduced
			m.clampDetailOffset()
			m.setMessage(message)
			return nil
		}
	}
	m.setMessage("Diff context is available for selected file diffs")
	return nil
}

func (m *Model) resetDiffContext() tea.Cmd {
	r, isFile := m.rows[m.tree.Cursor()]
	if !isFile || (r.kind != rowUnstaged && r.kind != rowStaged) || m.repo == nil {
		m.setMessage("Diff context is available for selected file diffs")
		return nil
	}
	m.diffContext = defaultDiffContext
	m.setMessage("Default diff context (3 lines)")
	return m.loadDetailCmd()
}

// lessDiffContext removes one outer context line from each displayed hunk.
// Change lines and metadata are never removed.
func lessDiffContext(detail string) (string, bool) {
	lines := strings.Split(strings.TrimSuffix(detail, "\n"), "\n")
	changed := false
	for start := 0; start < len(lines); {
		if !strings.HasPrefix(lines[start], "@@") {
			start++
			continue
		}
		end := start + 1
		for end < len(lines) && !strings.HasPrefix(lines[end], "@@") && !strings.HasPrefix(lines[end], "diff ") {
			end++
		}
		first, last := start+1, end-1
		if first <= last && strings.HasPrefix(lines[first], " ") {
			lines = append(lines[:first], lines[first+1:]...)
			end--
			last--
			changed = true
		}
		if last >= start+1 && strings.HasPrefix(lines[last], " ") {
			lines = append(lines[:last], lines[last+1:]...)
			end--
			changed = true
		}
		start = end
	}
	if !changed {
		return detail, false
	}
	return strings.Join(lines, "\n"), true
}
