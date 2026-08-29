package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/richard/lazymagit/internal/keymap"
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "lazymagit"
	return v
}

func (m *Model) render() string {
	if m.width < 20 || m.height < 5 {
		return fitBlock("lazymagit\nTerminal too small", max(1, m.width), max(1, m.height))
	}
	header := m.renderHeader()
	footer := m.renderFooter()
	bodyHeight := m.height - 4
	var body string
	if m.mode == modeProcess && m.processPanelHeight() > 0 {
		processHeight := m.processPanelHeight()
		topHeight := bodyHeight - processHeight - 1
		body = m.renderMainBody(topHeight) + "\n" + m.renderProcessPanel(m.width, processHeight)
	} else {
		body = m.renderMainBody(bodyHeight)
	}
	_, cataloguedPrefix := m.activeTransientCatalog()
	if (m.mode != modeStatus && m.mode != modeProcess) || cataloguedPrefix {
		body = m.renderOverlay(bodyHeight)
	}
	return fitBlock(header+"\n"+body+"\n"+footer, m.width, m.height)
}

func (m *Model) renderMainBody(bodyHeight int) string {
	if m.compact {
		return m.renderCompactMainBody(bodyHeight)
	}
	var body string
	if bodyHeight < 3 {
		body = fitBlock("Repository status", m.width, bodyHeight)
	} else if m.width >= 96 {
		left := max(36, m.width*43/100)
		right := m.width - left
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderStatusPanel(left, bodyHeight),
			m.renderDetailPanel(right, bodyHeight),
		)
	} else if bodyHeight >= 7 {
		panelHeight := bodyHeight - 1 // The newline between stacked panels.
		statusHeight := max(3, panelHeight*55/100)
		statusHeight = min(statusHeight, panelHeight-3)
		detailHeight := panelHeight - statusHeight
		body = m.renderStatusPanel(m.width, statusHeight) + "\n" + m.renderDetailPanel(m.width, detailHeight)
	} else {
		body = m.renderStatusPanel(m.width, bodyHeight)
	}
	return body
}

func (m *Model) renderCompactMainBody(bodyHeight int) string {
	if bodyHeight <= 0 {
		return ""
	}
	if m.width < 72 {
		return m.renderCompactStatus(m.width, bodyHeight)
	}
	left := max(30, m.width*38/100)
	right := m.width - left - 1
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderCompactStatus(left, bodyHeight),
		lipgloss.NewStyle().Foreground(colorBorder).Render("│"),
		m.renderCompactDetail(right, bodyHeight),
	)
}

func (m *Model) renderCompactStatus(width, height int) string {
	ids := m.tree.VisibleSectionIDs()
	cursor := 0
	for i, id := range ids {
		if id == m.tree.Cursor() {
			cursor = i
			break
		}
	}
	start := min(max(0, cursor-height+1), max(0, len(ids)-height))
	var lines []string
	for _, id := range ids[start:min(len(ids), start+height)] {
		section, row := m.tree.Section(id), m.rows[id]
		prefix := "  "
		if row.kind == rowHeading {
			prefix = map[bool]string{true: "▸ ", false: "▾ "}[m.tree.IsFolded(id)]
		}
		style := lipgloss.NewStyle()
		switch {
		case row.kind == rowHeading:
			style = style.Foreground(colorGold).Bold(true)
		case row.kind == rowStaged:
			style = style.Foreground(colorGreen)
		case row.kind == rowUntracked || row.kind == rowUnstaged:
			style = style.Foreground(colorPurple)
		default:
			style = style.Foreground(colorText)
		}
		if m.statusSearchMatch(id) {
			style = style.Underline(true).Foreground(colorCyan)
		}
		if id == m.tree.Cursor() {
			style = style.Reverse(true).Bold(true)
		}
		lines = append(lines, style.Render(truncate(prefix+section.Title(), width)))
	}
	return fitBlock(strings.Join(lines, "\n"), width, height)
}

func (m *Model) renderCompactDetail(width, height int) string {
	lines := strings.Split(strings.TrimSuffix(sanitizeDiff(m.detailForDisplay()), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines[0] = "Select a file or commit to inspect details."
	}
	start := min(m.detailOffset, max(0, len(lines)-height))
	var styled []string
	rangeLow, rangeHigh := min(m.detailRangeStart, m.detailRangeEnd), max(m.detailRangeStart, m.detailRangeEnd)
	for visibleIndex, line := range lines[start:min(len(lines), start+height)] {
		absoluteIndex := start + visibleIndex
		style := lipgloss.NewStyle().Foreground(colorText)
		switch {
		case m.graphActive && absoluteIndex == m.graphCursor:
			style = style.Foreground(colorOnAccent).Background(colorCyan).Bold(true)
		case rangeLow >= 0 && absoluteIndex >= rangeLow && absoluteIndex <= rangeHigh:
			style = style.Foreground(colorOnAccent).Background(colorGold).Bold(absoluteIndex == m.detailLine)
		case absoluteIndex == m.detailHunk && strings.HasPrefix(line, "@@"):
			style = style.Foreground(colorOnAccent).Background(colorCyan).Bold(true)
		case strings.HasPrefix(line, "@@") && m.detailHunkSelected(absoluteIndex):
			style = style.Foreground(colorOnAccent).Background(colorPurple).Bold(true)
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			style = style.Foreground(colorGreen)
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			style = style.Foreground(colorRed)
		case strings.HasPrefix(line, "@@"):
			style = style.Foreground(colorCyan)
		case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "commit "):
			style = style.Foreground(colorPurple).Bold(true)
		}
		styled = append(styled, style.Render(truncate(line, width)))
	}
	return fitBlock(strings.Join(styled, "\n"), width, height)
}

func (m *Model) renderHeader() string {
	repo := sanitizeSingleLine(filepath.Base(m.repo.WorkTree()))
	branch := sanitizeSingleLine(m.snapshot.summary.Branch)
	if branch == "" {
		branch = "loading…"
	} else if m.snapshot.summary.Detached {
		branch = "detached@" + shortID(m.snapshot.summary.Head)
	}
	upstream := "no upstream"
	if m.snapshot.summary.Upstream != "" {
		upstream = fmt.Sprintf("%s  ↑%d ↓%d", sanitizeSingleLine(m.snapshot.summary.Upstream), m.snapshot.summary.Ahead, m.snapshot.summary.Behind)
	}
	left := lipgloss.NewStyle().Bold(true).Foreground(colorPurple).Render(" LAZYMAGIT ")
	// Keep the header legible in stock terminal fonts; Nerd Font glyphs render
	// as a replacement character in Terminal.app and make the branch unclear.
	meta := lipgloss.NewStyle().Foreground(colorCyan).Render(fmt.Sprintf(" %s  branch %s ", repo, branch))
	right := lipgloss.NewStyle().Foreground(colorMuted).Render(upstream)
	line := left + meta + right
	return lipgloss.NewStyle().Width(m.width).Background(lipgloss.Color("#172033")).Render(truncate(line, m.width))
}

func (m *Model) renderStatusPanel(width, height int) string {
	if width < 3 || height < 3 {
		return fitBlock("Status", width, height)
	}
	innerW, innerH := width-2, height-2
	ids := m.tree.VisibleSectionIDs()
	cursor := 0
	for i, id := range ids {
		if id == m.tree.Cursor() {
			cursor = i
		}
	}
	start := 0
	if cursor >= innerH {
		start = cursor - innerH + 1
	}
	end := min(len(ids), start+innerH)
	lines := make([]string, 0, innerH)
	for _, id := range ids[start:end] {
		section := m.tree.Section(id)
		r := m.rows[id]
		selected := id == m.tree.Cursor()
		prefix := "  "
		if r.kind == rowHeading {
			if len(section.Children()) > 0 {
				if m.tree.IsFolded(id) {
					prefix = "▸ "
				} else {
					prefix = "▾ "
				}
			} else {
				prefix = "  "
			}
		} else {
			prefix = "    "
		}
		text := truncate(prefix+section.Title(), innerW)
		style := lipgloss.NewStyle().Width(innerW)
		if r.kind == rowHeading {
			style = style.Bold(true).Foreground(colorPurple)
		} else if r.kind == rowUntracked || r.kind == rowUnstaged {
			style = style.Foreground(colorGold)
		} else if r.kind == rowStaged {
			style = style.Foreground(colorGreen)
		} else {
			style = style.Foreground(colorCyan)
		}
		if m.statusSearchMatch(id) {
			style = style.Underline(true).Foreground(colorCyan)
		}
		if selected {
			style = style.Reverse(true).Bold(true)
		}
		lines = append(lines, style.Render(text))
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return panelStyle(width, height).Render(strings.Join(lines, "\n"))
}

func (m *Model) renderDetailPanel(width, height int) string {
	if width < 3 || height < 3 {
		return fitBlock(m.detailForDisplay(), width, height)
	}
	innerW, innerH := width-2, height-2
	lines := strings.Split(strings.TrimSuffix(sanitizeDiff(m.detailForDisplay()), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines[0] = "Select a file to inspect its diff."
	}
	start := min(m.detailOffset, max(0, len(lines)-innerH))
	lines = lines[start:]
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	styled := make([]string, 0, innerH)
	rangeLow, rangeHigh := min(m.detailRangeStart, m.detailRangeEnd), max(m.detailRangeStart, m.detailRangeEnd)
	for visibleIndex, line := range lines {
		absoluteIndex := start + visibleIndex
		style := lipgloss.NewStyle()
		switch {
		case m.graphActive && absoluteIndex == m.graphCursor:
			style = style.Foreground(colorOnAccent).Background(colorCyan).Bold(true)
		case rangeLow >= 0 && absoluteIndex >= rangeLow && absoluteIndex <= rangeHigh:
			style = style.Foreground(colorOnAccent).Background(colorGold).Bold(absoluteIndex == m.detailLine)
		case absoluteIndex == m.detailHunk && strings.HasPrefix(line, "@@"):
			style = style.Foreground(colorOnAccent).Background(colorCyan).Bold(true)
		case strings.HasPrefix(line, "@@") && m.detailHunkSelected(absoluteIndex):
			style = style.Foreground(colorOnAccent).Background(colorPurple).Bold(true)
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			style = style.Foreground(colorGreen)
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			style = style.Foreground(colorRed)
		case strings.HasPrefix(line, "@@"):
			style = style.Foreground(colorCyan)
		case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "commit "):
			style = style.Foreground(colorPurple).Bold(true)
		default:
			style = style.Foreground(colorText)
		}
		styled = append(styled, style.Render(truncate(line, innerW)))
	}
	for len(styled) < innerH {
		styled = append(styled, "")
	}
	return panelStyle(width, height).Render(strings.Join(styled, "\n"))
}

func (m *Model) detailHunkSelected(lineIndex int) bool {
	hunk := -1
	for index, line := range m.detailLines() {
		if strings.HasPrefix(line, "@@") {
			hunk++
		}
		if index == lineIndex {
			break
		}
	}
	return m.detailSelectedHunks[hunk]
}

func (m *Model) detailForDisplay() string {
	if m.detailHidden {
		return "Diff hidden\n\nPress Alt+Tab to show the selected row's detail."
	}
	return m.detail
}

func (m *Model) renderProcessPanel(width, height int) string {
	if width < 3 || height < 3 {
		return fitBlock("Git processes", width, height)
	}
	innerW, innerH := width-2, height-2
	lines := m.processLinesAtWidth(innerW)
	start := min(m.processOffset, max(0, len(lines)-max(0, innerH-1)))
	end := min(len(lines), start+max(0, innerH-1))
	visible := make([]string, 0, innerH)
	visible = append(visible, lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render("Git processes"))
	for _, line := range lines[start:end] {
		visible = append(visible, truncate(line, innerW))
	}
	return panelStyle(width, height).Render(fitBlock(strings.Join(visible, "\n"), innerW, innerH))
}

func panelStyle(outerWidth, outerHeight int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(outerWidth).Height(outerHeight).
		Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder)
}

func (m *Model) renderFooter() string {
	prefix := m.resolver.PendingPrefix()
	scheme := "Vim"
	if m.scheme == schemeMagit {
		scheme = "Magit"
	}
	var left string
	if prefix != "" {
		if m.resolver.ActiveTransient() != "" {
			catalog, _ := m.activeTransientCatalog()
			left = lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render("[" + scheme + "] " + catalog.Title + " — choose a suffix")
		} else {
			left = lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render("[" + scheme + "] g …  g → first")
		}
	} else if m.mode == modeStatus && m.graphActive {
		left = lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render("Graph") + lipgloss.NewStyle().Foreground(colorMuted).Render("  ↑/↓ or j/k select  Enter inspect commit  Esc close")
	} else if m.mode == modeStatus && m.revisionActive {
		controls := "  p first parent  Esc close"
		if m.graphReturn != nil {
			controls = "  p first parent  Esc return graph"
		}
		left = lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render("Revision") + lipgloss.NewStyle().Foreground(colorMuted).Render(controls)
	} else if m.mode == modeStatus {
		workflow := func(key, label string) string {
			return lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render(key) + " " + label
		}
		var primary []string
		for _, binding := range keymap.PrimaryBindings(schemeID(m.scheme)) {
			primary = append(primary, workflow(binding.Display, binding.Label))
		}
		left = strings.Join(primary, "  ")
		optional := []string{"↑/↓ detail  [ prev  ] next  V hunks  v lines", "Ctrl-B Blame", "$ Processes", "Ctrl-G Graph", "? Commands"}
		if m.scheme == schemeMagit {
			optional = append(optional, "[Magit] F2 Vim", "n/p move")
		} else {
			optional = append(optional, "[Vim] F2 Magit", "j/k move")
		}
		for _, item := range optional {
			candidate := left + "  " + lipgloss.NewStyle().Foreground(colorMuted).Render(item)
			if ansi.StringWidth(candidate) <= m.width {
				left = candidate
			}
		}
	} else {
		switch m.mode {
		case modeCommit, modeConfirm, modeAddRemote:
			left = lipgloss.NewStyle().Foreground(colorMuted).Render("Esc cancel")
		case modeWorkflow:
			left = lipgloss.NewStyle().Foreground(colorMuted).Render("Tab/↑/↓ field  Enter edit/submit  Esc cancel")
		case modeHelp:
			left = lipgloss.NewStyle().Foreground(colorMuted).Render("q/Esc close  ↑/↓ PageUp/PageDown")
		case modeProcess:
			left = lipgloss.NewStyle().Foreground(colorMuted).Render("y Copy output  $/q Close  ↑/↓ PageUp/PageDown")
		default:
			left = lipgloss.NewStyle().Foreground(colorMuted).Render("q/Esc back")
		}
	}
	messageStyle := lipgloss.NewStyle().Foreground(colorCyan)
	if m.isError {
		messageStyle = messageStyle.Foreground(colorRed).Bold(true)
	}
	message := messageStyle.Render(sanitizeSingleLine(m.message))
	if m.isError && ansi.StringWidth(left)+2+ansi.StringWidth(message) > m.width {
		return truncate(message, m.width)
	}
	available := max(0, m.width-ansi.StringWidth(left)-2)
	return truncate(left+"  "+truncate(message, available), m.width)
}

func (m *Model) renderOverlay(height int) string {
	if pending := m.resolver.PendingPrefix(); pending != "" && pending != "g" {
		if m.resolver.ActiveTransient() != "" {
			prefix := transientInvocationRoot(pending)
			catalog, _ := m.transientCatalog(prefix)
			return renderTransient(catalog, m.width, height, m.transientOffset)
		}
	}
	if m.mode == modeRemotes {
		return m.renderRemoteOverlay(height)
	}
	if m.width < 4 || height < 3 {
		return fitBlock("", m.width, height)
	}
	width := m.width
	innerW, innerH := width-4, height-2 // Horizontal padding and border; vertical border.
	var title, content string
	switch m.mode {
	case modeCommit:
		title = " Commit message "
		content = "Create commit\n\n> " + sanitizeSingleLine(m.input) + "█\n\nEnter commit  •  Esc cancel"
	case modeConfirm:
		title = " Confirm discard "
		content = "Permanently discard changes to:\n\n" + sanitizeSingleLine(m.confirmPath) + "\n\n[y] yes    [n] no"
	case modeBranches:
		title = " Switch branch "
		var lines []string
		start := max(0, m.branchCursor-(innerH-4)/2)
		end := min(len(m.branches), start+max(1, innerH-4))
		for i := start; i < end; i++ {
			mark := "  "
			if m.branches[i].Current {
				mark = "* "
			}
			line := mark + sanitizeSingleLine(m.branches[i].Name)
			if i == m.branchCursor {
				line = lipgloss.NewStyle().Reverse(true).Bold(true).Render(truncate(line, innerW))
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			lines = append(lines, "No local branches")
		}
		content = strings.Join(lines, "\n") + "\n\nEnter switch  •  q back"
	case modeAddRemote:
		title = " Add remote "
		fields := [2]string{"Name", "URL"}
		var lines []string
		for i, label := range fields {
			mark := "  "
			if i == m.remoteField {
				mark = "▶ "
			}
			value := sanitizeSingleLine(m.remoteInput[i])
			if i == m.remoteField {
				value += "█"
			}
			lines = append(lines, mark+label+": "+value)
		}
		fetch := "yes"
		if !m.remoteFetch {
			fetch = "no"
		}
		content = strings.Join(lines, "\n\n") + "\n\nFetch after add: " + fetch + " (Ctrl-f toggle)\n\nTab/↑/↓ field  •  Enter next/add  •  Esc cancel"
	case modeHelp:
		return renderDispatcher(m.dispatcherCatalog(), m.width, height, m.transientOffset)
	case modeWorkflow:
		return m.renderWorkflowOverlay(width, height)
	}
	heading := lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(title)
	text := fitBlock(heading+"\n\n"+content, innerW, innerH)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).
		Border(lipgloss.DoubleBorder()).BorderForeground(colorPurple).Render(text)
}

func (m *Model) renderWorkflowOverlay(width, height int) string {
	if m.workflow == nil {
		return fitBlock("", width, height)
	}
	if width < 4 || height < 3 {
		return fitBlock(sanitizeSingleLine(m.workflow.dialog.Title), width, height)
	}
	innerW, innerH := width-4, height-2
	w := m.workflow
	lines := []string{lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(" " + sanitizeSingleLine(w.dialog.Title) + " ")}
	lines = append(lines, renderWorkflowFields(w)...)
	lines = append(lines, renderWorkflowReview(w)...)
	lines = append(lines, "", renderWorkflowAction(workflowActionLabel(w), w.field >= len(w.dialog.Fields), w.busy), "", "Tab field  •  ↑/↓ choose  •  Enter select/submit  •  Ctrl-J todo line  •  Esc cancel")
	if w.error != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(sanitizeSingleLine(w.error)))
	}
	text := fitBlock(strings.Join(lines, "\n"), innerW, innerH)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Border(lipgloss.DoubleBorder()).BorderForeground(colorPurple).Render(text)
}

func renderWorkflowFields(w *workflowState) []string {
	var lines []string
	for index, field := range w.dialog.Fields {
		lines = append(lines, renderWorkflowField(field, index == w.field)...)
	}
	return lines
}

func renderWorkflowField(field WorkflowField, selected bool) []string {
	mark := "  "
	if selected {
		mark = "▶ "
	}
	if field.Kind == WorkflowSearch {
		return renderWorkflowSearchField(field, mark, selected)
	}
	if field.Kind == WorkflowMultiline {
		return renderWorkflowMultilineField(field, mark, selected)
	}
	value := workflowFieldValue(field)
	if field.Kind == WorkflowMultiline {
		lines := []string{mark + sanitizeSingleLine(field.Label) + " (Ctrl-J new line):"}
		for _, line := range strings.Split(value, "\n") {
			lines = append(lines, "    "+sanitizeSingleLine(line))
		}
		if selected {
			lines[len(lines)-1] += "█"
		}
		return lines
	}
	if selected && (field.Kind == WorkflowText || field.Kind == WorkflowConfirm) {
		value += "█"
	}
	return []string{mark + sanitizeSingleLine(field.Label) + ": " + sanitizeSingleLine(value)}
}

func renderWorkflowMultilineField(field WorkflowField, mark string, selected bool) []string {
	lines := []string{mark + sanitizeSingleLine(field.Label) + ":"}
	body := strings.Split(field.Value, "\n")
	if len(body) == 1 && body[0] == "" {
		body[0] = ""
	}
	for i, line := range body {
		cursor := ""
		if selected && i == len(body)-1 {
			cursor = "█"
		}
		lines = append(lines, "    "+sanitizeSingleLine(line)+cursor)
	}
	return lines
}

func workflowFieldValue(field WorkflowField) string {
	if field.Kind == WorkflowBool {
		return map[bool]string{true: "yes", false: "no"}[field.Bool]
	}
	if field.Kind == WorkflowEnum || field.Kind == WorkflowSelect {
		for _, choice := range field.Choices {
			if choice.Value == field.Value {
				return choice.Label
			}
		}
	}
	return field.Value
}

func renderWorkflowSearchField(field WorkflowField, mark string, selected bool) []string {
	query := field.Search
	if selected {
		query += "█"
	}
	lines := []string{mark + sanitizeSingleLine(field.Label) + ": " + sanitizeSingleLine(query)}
	matches := workflowSearchChoices(field)
	if len(matches) == 0 {
		label := "No matches"
		if field.AllowCustom && strings.TrimSpace(field.Search) != "" {
			label = "Use revision: " + strings.TrimSpace(field.Search)
		}
		return append(lines, "    "+sanitizeSingleLine(label))
	}
	start := min(max(0, field.Choice-3), max(0, len(matches)-7))
	for choiceIndex, choice := range matches[start:min(len(matches), start+7)] {
		prefix := "    "
		if start+choiceIndex == field.Choice {
			prefix = "  › "
		}
		lines = append(lines, prefix+sanitizeSingleLine(choice.Label))
	}
	return lines
}

func renderWorkflowReview(w *workflowState) []string {
	confirmation, plan := w.dialog.Confirmation, w.dialog.Plan
	if w.review != nil {
		confirmation, plan = w.review.Confirmation, w.review.Plan
	}
	var lines []string
	if confirmation != "" {
		lines = append(lines, "", sanitizeSingleLine(confirmation))
	}
	if len(plan) > 0 {
		lines = append(lines, "", "Plan:")
		for _, step := range plan {
			lines = append(lines, "  • "+sanitizeSingleLine(step))
		}
	}
	return lines
}

func workflowActionLabel(w *workflowState) string {
	if w.busy {
		return "Checking…"
	}
	if w.dialog.ReviewPreflight != nil {
		if w.review != nil {
			return "Execute"
		}
		return "Review"
	}
	if label := strings.TrimSpace(w.dialog.ActionLabel); label != "" {
		return label
	}
	if w.dialog.Run != nil {
		return "Open"
	}
	return "Submit"
}

func renderWorkflowAction(label string, selected, busy bool) string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 2).Foreground(colorOnAccent).Background(colorCyan)
	if strings.Contains(label, "Review") {
		style = style.Background(colorGold)
	}
	if label == "Execute" {
		style = style.Background(colorGreen)
	}
	if busy {
		style = style.Foreground(colorMuted).Background(colorBorder)
	}
	button := style.Render(sanitizeSingleLine(label))
	if selected {
		return "▶ " + button
	}
	return "  " + button
}

func (m *Model) renderRemoteOverlay(height int) string {
	title, hint := " Fetch elsewhere ", "Enter fetch"
	if m.remotePurpose == remoteConfigurePush {
		title, hint = " Configure push remote ", "Enter configure and fetch"
	} else if m.remotePurpose == remoteConfigureAndPush {
		title, hint = " Push and set upstream ", "Enter choose destination"
	}
	selected := "No remotes"
	cursor := 0
	if len(m.snapshot.remotes) > 0 {
		cursor = min(max(0, m.remoteCursor), len(m.snapshot.remotes)-1)
		selected = sanitizeSingleLine(m.snapshot.remotes[cursor].Name)
	}
	compact := lipgloss.NewStyle().Reverse(true).Bold(true).Render(selected) + "  •  " + hint
	if m.width < 4 || height < 3 {
		return fitBlock(compact, m.width, height)
	}
	innerW, innerH := m.width-4, height-2
	var text string
	if innerH < 5 {
		text = fitBlock(compact, innerW, innerH)
	} else {
		available := max(1, innerH-4)
		start := max(0, cursor-available/2)
		start = min(start, max(0, len(m.snapshot.remotes)-available))
		end := min(len(m.snapshot.remotes), start+available)
		lines := make([]string, 0, available)
		for i := start; i < end; i++ {
			line := "  " + sanitizeSingleLine(m.snapshot.remotes[i].Name)
			if i == cursor {
				line = lipgloss.NewStyle().Reverse(true).Bold(true).Render(truncate(line, innerW))
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			lines = append(lines, "No remotes")
		}
		heading := lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(title)
		text = fitBlock(heading+"\n"+strings.Join(lines, "\n")+"\n\n"+hint+"  •  q back", innerW, innerH)
	}
	return lipgloss.NewStyle().Width(m.width).Height(height).Padding(0, 1).
		Border(lipgloss.DoubleBorder()).BorderForeground(colorPurple).Render(text)
}

func shortID(id string) string {
	runes := []rune(sanitizeSingleLine(id))
	if len(runes) > 8 {
		runes = runes[:8]
	}
	return string(runes)
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// fitBlock is the final layout boundary: it preserves ANSI styling while
// ensuring asynchronous or unusually long content cannot grow the terminal
// frame beyond its requested outer dimensions.
func fitBlock(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		line = ansi.Truncate(line, width, "")
		if padding := width - ansi.StringWidth(line); padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
