package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorPurple = lipgloss.Color("#A78BFA")
	colorCyan   = lipgloss.Color("#67E8F9")
	colorGreen  = lipgloss.Color("#86EFAC")
	colorRed    = lipgloss.Color("#FDA4AF")
	colorGold   = lipgloss.Color("#FDE68A")
	colorMuted  = lipgloss.Color("#94A3B8")
	colorBorder = lipgloss.Color("#475569")
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
	if m.mode != modeStatus {
		body = m.renderOverlay(bodyHeight)
	}
	return fitBlock(header+"\n"+body+"\n"+footer, m.width, m.height)
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
	meta := lipgloss.NewStyle().Foreground(colorCyan).Render(fmt.Sprintf(" %s   %s ", repo, branch))
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
		return fitBlock(m.detail, width, height)
	}
	innerW, innerH := width-2, height-2
	lines := strings.Split(strings.TrimSuffix(sanitizeDiff(m.detail), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines[0] = "Select a file to inspect its diff."
	}
	start := min(m.detailOffset, max(0, len(lines)-innerH))
	lines = lines[start:]
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	styled := make([]string, 0, innerH)
	for _, line := range lines {
		style := lipgloss.NewStyle()
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			style = style.Foreground(colorGreen)
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			style = style.Foreground(colorRed)
		case strings.HasPrefix(line, "@@"):
			style = style.Foreground(colorCyan)
		case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "commit "):
			style = style.Foreground(colorPurple).Bold(true)
		default:
			style = style.Foreground(lipgloss.Color("#CBD5E1"))
		}
		styled = append(styled, style.Render(truncate(line, innerW)))
	}
	for len(styled) < innerH {
		styled = append(styled, "")
	}
	return panelStyle(width, height).Render(strings.Join(styled, "\n"))
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
		suffix := map[string]string{
			"g": "g → first",
			"c": "c → commit",
			"b": "b → switch branch",
			"f": "f → fetch",
			"P": "p → push",
		}[prefix]
		left = lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render("[" + scheme + "] " + prefix + " …  " + suffix)
	} else if m.scheme == schemeMagit {
		left = lipgloss.NewStyle().Foreground(colorMuted).Render("[Magit] F2 Vim  n/p move  g/G refresh  k discard  x reserved  ? help")
	} else {
		left = lipgloss.NewStyle().Foreground(colorMuted).Render("[Vim] F2 Magit  j/k move  gg/G first/last  x discard  ? help")
	}
	messageStyle := lipgloss.NewStyle().Foreground(colorCyan)
	if m.isError {
		messageStyle = messageStyle.Foreground(colorRed).Bold(true)
	}
	message := messageStyle.Render(sanitizeSingleLine(m.message))
	available := max(0, m.width-ansi.StringWidth(left)-2)
	return truncate(left+"  "+truncate(message, available), m.width)
}

func (m *Model) renderOverlay(height int) string {
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
	case modeHelp:
		title = " Help "
		scheme, navigation, collisions := "Vim (F2 → Magit)", "  j/k         move        gg/G   first/last", "  x           discard untracked/unstaged (confirm)"
		if m.scheme == schemeMagit {
			scheme, navigation, collisions = "Magit (F2 → Vim)", "  n/p         move", "  g/G         refresh     k      discard whole file (confirm)\n  x           reserved reset; unsupported"
		}
		content = strings.Join([]string{
			"KEY SCHEME", "  Active: " + scheme, "", "NAVIGATION", navigation, "  Tab         fold        1/2/3  global depth", "",
			"DETAIL", "  Space/PageDown/Ctrl-d    scroll down", "  Shift-Space/PageUp/Ctrl-u scroll up", "",
			"STATUS", collisions, "  s           stage file  u      unstage", "",
			"COMMANDS", "  c c         commit      b b    switch branch", "  f f         fetch       P p    push", "  q           quit/back   ?      toggle help",
		}, "\n")
	}
	heading := lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(title)
	text := fitBlock(heading+"\n\n"+content, innerW, innerH)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).
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
