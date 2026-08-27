package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/richard/lazymagit/internal/keymap"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

// These identities are kept here so navigation can be implemented without
// changing the central keymap. The keymap should use the same values when its
// status rows are promoted from unsupported to executable.
const (
	commandSectionCycle       keymap.CommandID = "section.cycle"
	commandSectionCycleGlobal keymap.CommandID = "section.cycle-global"
	commandSectionParent      keymap.CommandID = "section.parent"
	commandSiblingPrevious    keymap.CommandID = "section.sibling-previous"
	commandSiblingNext        keymap.CommandID = "section.sibling-next"
	commandLocalDepth1        keymap.CommandID = "section.local-depth-1"
	commandLocalDepth2        keymap.CommandID = "section.local-depth-2"
	commandLocalDepth3        keymap.CommandID = "section.local-depth-3"
	commandLocalDepth4        keymap.CommandID = "section.local-depth-4"
	commandGlobalDepth1       keymap.CommandID = "section.global-depth-1"
	commandGlobalDepth2       keymap.CommandID = "section.global-depth-2"
	commandGlobalDepth3       keymap.CommandID = "section.global-depth-3"
	commandGlobalDepth4       keymap.CommandID = "section.global-depth-4"
	commandVisitThing         keymap.CommandID = "status.visit-thing"
	commandDetailBackward     keymap.CommandID = "detail.page-backward"
	commandDiffMoreContext    keymap.CommandID = "detail.more-context"
	commandDiffLessContext    keymap.CommandID = "detail.less-context"
	commandDiffDefaultContext keymap.CommandID = "detail.default-context"
	commandDescribeSection    keymap.CommandID = "section.describe"
	commandStatusJump         keymap.CommandID = "status.jump"
	commandDisplayRepository  keymap.CommandID = "status.display-repository"
	commandCopyThing          keymap.CommandID = "status.copy-thing"
	commandCopySectionValue   keymap.CommandID = "section.copy-value"
	commandCopyBufferRevision keymap.CommandID = "status.copy-revision"
)

var statusJumpSections = map[string]sectionmodel.SectionID{
	"magit-jump-to-stashes":                "status/stashes",
	"magit-jump-to-untracked":              "status/untracked",
	"magit-jump-to-unstaged":               "status/unstaged",
	"magit-jump-to-staged":                 "status/staged",
	"magit-jump-to-unpulled-from-upstream": "status/unpulled",
	"magit-jump-to-unpushed-to-upstream":   "status/unpushed",
}

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		out := map[keymap.CommandID]WorkflowHandler{}
		for _, binding := range keymap.Registry() {
			section, ok := statusJumpSections[binding.UpstreamCommand]
			if !ok || binding.Transient != "magit-status-jump" {
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

// handleNavigationKey is the raw-key side of the navigation domain. Keeping
// this separate from handleKey allows the central dispatcher to call it before
// the generic resolver while retaining Vim's j/k/g/G collisions. Multi-key
// sequences (C-c TAB and C-c C-w) must arrive through the keymap as commands.
func (m *Model) handleNavigationKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.mode != modeStatus || m.resolver.PendingPrefix() != "" {
		return nil, false
	}
	key := msg.String()
	commands := map[string]keymap.CommandID{
		"ctrl+tab": commandSectionCycle, "shift+tab": commandSectionCycleGlobal,
		"^": commandSectionParent, "alt+p": commandSiblingPrevious, "alt+n": commandSiblingNext,
		"1": commandLocalDepth1, "2": commandLocalDepth2, "3": commandLocalDepth3, "4": commandLocalDepth4,
		"alt+1": commandGlobalDepth1, "alt+2": commandGlobalDepth2, "alt+3": commandGlobalDepth3, "alt+4": commandGlobalDepth4,
		"enter": commandVisitThing, "ctrl+enter": commandVisitThing, "backspace": commandDetailBackward,
		"+": commandDiffMoreContext, "-": commandDiffLessContext, "0": commandDiffDefaultContext,
		"H": commandDescribeSection, "J": commandDisplayRepository,
		"ctrl+w": commandCopySectionValue, "alt+w": commandCopyBufferRevision,
	}
	command, ok := commands[key]
	if !ok {
		return nil, false
	}
	return m.performNavigationCommand(command)
}

// performNavigationCommand implements non-mutating status commands. It returns
// false for commands owned by another domain, making it suitable as a fallback
// from Model.perform.
func (m *Model) performNavigationCommand(command keymap.CommandID) (tea.Cmd, bool) {
	switch command {
	case commandSectionCycle:
		if m.tree.CycleLocal() {
			m.rememberNavigationFolds()
			m.bumpState()
		}
		return m.loadDetailCmd(), true
	case commandSectionCycleGlobal:
		if m.tree.CycleGlobal() {
			m.rememberNavigationFolds()
			m.bumpState()
		}
		return m.loadDetailCmd(), true
	case commandSectionParent:
		return m.finishNavigationMove(m.tree.MoveToParent()), true
	case commandSiblingPrevious:
		return m.finishNavigationMove(m.tree.MoveToPreviousSibling()), true
	case commandSiblingNext:
		return m.finishNavigationMove(m.tree.MoveToNextSibling()), true
	case commandLocalDepth1, commandLocalDepth2, commandLocalDepth3, commandLocalDepth4:
		depth := int(command[len(command)-1] - '0')
		if m.tree.RevealSelectedDepth(depth) {
			m.rememberNavigationFolds()
			m.bumpState()
		}
		return m.loadDetailCmd(), true
	case commandGlobalDepth1, commandGlobalDepth2, commandGlobalDepth3, commandGlobalDepth4:
		depth := int(command[len(command)-1] - '0')
		if m.tree.RevealGlobalDepth(depth) {
			m.rememberNavigationFolds()
			m.bumpState()
		}
		return m.loadDetailCmd(), true
	case commandVisitThing:
		return m.visitSelectedThing(), true
	case commandDetailBackward:
		m.scrollDetail(-max(1, m.detailViewportHeight()))
		return nil, true
	case commandDiffLessContext:
		if reduced, ok := lessDiffContext(m.detail); ok {
			m.detail = reduced
			m.clampDetailOffset()
			m.setMessage("Showing less diff context")
		} else {
			m.setMessage("No diff context to hide")
		}
		return nil, true
	case commandDiffMoreContext, commandDiffDefaultContext:
		// Reloading is the only safe way to recover context removed from the
		// rendered diff; loadDetailCmd issues only the selected typed Git query.
		m.setMessage(map[bool]string{true: "Default diff context", false: "More diff context"}[command == commandDiffDefaultContext])
		return m.loadDetailCmd(), true
	case commandDescribeSection:
		m.describeSelectedSection()
		return nil, true
	case commandStatusJump:
		return m.jumpToNextStatusSection(), true
	case commandDisplayRepository:
		m.displayRepositoryStatus()
		return nil, true
	case commandCopyThing, commandCopySectionValue:
		return m.copySelectedSection(), true
	case commandCopyBufferRevision:
		return m.copySelectedRevision(), true
	default:
		return nil, false
	}
}

func (m *Model) finishNavigationMove(moved bool) tea.Cmd {
	if moved {
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
	}
	// Files and revisions are visited in the terminal detail pane; no editor,
	// browser, or other ambient process is launched.
	return m.loadDetailCmd()
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

// lessDiffContext removes one outer context line from each displayed hunk.
// Change lines and metadata are never removed; a subsequent + or 0 reloads the
// selected typed diff query to recover the source text.
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
