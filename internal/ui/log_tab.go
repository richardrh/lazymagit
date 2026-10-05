package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

// maxMarkedCommits bounds the Log tab's selection set. Two revisions are what a
// single comparison can express; a third mark would silently widen the diff
// range past what the user reviewed, so the third toggle is rejected instead.
const maxMarkedCommits = 2

// logMarkGlyph and logMarkBlank form the Log tab's selection gutter. The column
// is always present so that marking a commit never reflows the list.
const (
	logMarkGlyph = "✓"
	logMarkBlank = " "
)

// logTabWidth is the minimum width at which the Log tab splits into a list and
// a detail pane. Below it the tab stacks, matching the Status tab's behavior.
const logTabWidth = 96

// logGraphWidth bounds Git's ASCII graph lanes inside a commit row.
const logGraphWidth = 10

// logAuthorWidth is the author column budget in the commit list.
const logAuthorWidth = 18

// logSubjectBudget is the number of subject columns the layout always protects.
// It is a floor on the subject, not a target: a longer subject is truncated at
// render time, which is better than stripping every other column to fit.
const logSubjectBudget = 16

// logDecorationsWidth caps the ref column so one long branch name cannot push
// the subject out of the pane.
const logDecorationsWidth = 24

// logListShare is the percentage of the terminal width the commit list takes in
// the side-by-side Log tab layout.
const logListShare = 50

// Fixed column widths shared by every row in the Log tab.
const (
	logGutterWidth  = 2
	logShortIDWidth = 7
	logDateWidth    = 10
)

// minTabStripHeight is the shortest terminal that can show the tab strip
// without stealing the body a row. Shorter terminals drop the strip entirely
// so overlays and the Status tab keep the space they had before it existed.
const minTabStripHeight = 8

// tabStripRows reports how many rows the tab strip occupies at the current
// terminal height.
func (m *Model) tabStripRows() int {
	if m.height >= minTabStripHeight {
		return 1
	}
	return 0
}

// openLogTab loads a commit list into the Log tab. Every magit-log suffix and
// the terminal-native tab keys route here, so the tab is the single place that
// owns log presentation.
func openLogTab(m *Model, title string, query gitbackend.LogQuery) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	m.closeInspection()
	m.cancelLogDetail()
	m.logTab, m.logTitle, m.logQuery = true, sanitizeSingleLine(title), query
	m.logCursor, m.logOffset = 0, 0
	m.detailOffset = 0
	m.logListRequest++
	request := m.logListRequest
	m.setMessage("Loading " + m.logTitle + "…")
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return logTabMsg{request: request, title: m.logTitle, err: err}
		}
		return logTabMsg{request: request, title: m.logTitle, entries: result.Items}
	}
}

// logTabCurrent opens the tab on the revision the Status tab has selected, so
// the terminal-native key honors an active inspection instead of resetting to
// HEAD.
func logTabCurrent(m *Model, _ WorkflowCommand) tea.Cmd {
	query := gitbackend.LogQuery{Graph: true, Decorations: true, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit}
	query.Revision = selectedInspectRevision(m)
	return openLogTab(m, "Log", query)
}

// logTabAllRefs opens the tab across every reference, which is the terminal
// replacement for the old all-refs graph overlay.
func logTabAllRefs(m *Model, _ WorkflowCommand) tea.Cmd {
	query := gitbackend.LogQuery{All: true, Graph: true, Decorations: true, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit}
	return openLogTab(m, "All refs", query)
}

// handleLogTabMsg installs a loaded commit list and selects its first row.
func (m *Model) handleLogTabMsg(msg logTabMsg) tea.Cmd {
	if msg.request != m.logListRequest || !m.logTab || !m.appActive() {
		return nil
	}
	m.cancelDetail()
	if msg.err != nil {
		m.logEntries = nil
		m.markedCommits = nil
		m.logTitle = "Unable to load log"
		m.detail = "Unable to load log:\n" + sanitizeSingleLine(msg.err.Error())
		m.setError(fmt.Errorf("log failed: %w", msg.err))
		return nil
	}
	// A refresh must not throw the user back to the top of the list, so the
	// selection follows the commit it was on when that commit survives.
	selected := ""
	if entry, ok := m.selectedLogEntry(); ok {
		selected = entry.ID
	}
	m.logEntries = msg.entries
	m.logTitle = msg.title
	m.retainMarkedCommits()
	m.logCursor = 0
	for index, entry := range m.logEntries {
		if entry.ID == selected {
			m.logCursor = index
			break
		}
	}
	m.logOffset = 0
	m.setMessage(fmt.Sprintf("%s: %d commits", m.logTitle, len(m.logEntries)))
	return m.loadLogDetailCmd()
}

// retainMarkedCommits drops marks that no longer appear in the loaded list so a
// comparison can never resolve a revision the user cannot see.
func (m *Model) retainMarkedCommits() {
	if len(m.markedCommits) == 0 {
		return
	}
	present := make(map[string]bool, len(m.logEntries))
	for _, entry := range m.logEntries {
		present[entry.ID] = true
	}
	retained := m.markedCommits[:0]
	for _, id := range m.markedCommits {
		if present[id] {
			retained = append(retained, id)
		}
	}
	m.markedCommits = retained
}

// loadLogDetailCmd requests the selected commit's patch for the detail pane.
func (m *Model) loadLogDetailCmd() tea.Cmd {
	m.cancelLogDetail()
	entry, ok := m.selectedLogEntry()
	if !ok {
		m.detail = "No commits to inspect."
		return nil
	}
	m.logDetailRequest++
	request := m.logDetailRequest
	m.detailOffset = 0
	m.detail = "Loading " + entry.ShortID + "…"
	ctx, cancel := context.WithCancel(m.appCtx)
	m.logDetailID, m.logDetailCancel = entry.ID, cancel
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		text, err := m.showCommit(ctx, entry.ID)
		return logDetailMsg{request: request, id: entry.ID, text: text, err: err}
	}
}

func (m *Model) cancelLogDetail() {
	if m.logDetailCancel != nil {
		m.logDetailCancel()
		m.logDetailCancel = nil
	}
}

// handleLogDetailMsg installs the selected commit's patch, discarding results
// for rows the user has already scrolled past.
func (m *Model) handleLogDetailMsg(msg logDetailMsg) {
	if msg.request != m.logDetailRequest || !m.logTab || !m.appActive() {
		return
	}
	m.cancelLogDetail()
	if msg.id != m.logDetailID {
		return
	}
	if msg.err != nil {
		m.detail = "Unable to load commit:\n" + sanitizeSingleLine(msg.err.Error())
		m.setError(fmt.Errorf("commit detail failed: %w", msg.err))
		return
	}
	m.detail = sanitizeDiff(msg.text)
	m.detailOffset = 0
}

// selectedLogEntry returns the commit under the Log tab cursor.
func (m *Model) selectedLogEntry() (gitbackend.LogEntry, bool) {
	if m.logCursor < 0 || m.logCursor >= len(m.logEntries) {
		return gitbackend.LogEntry{}, false
	}
	return m.logEntries[m.logCursor], true
}

// activeLogRevision returns the Log tab selection for callers that already
// accept an inspected revision, or "" when the tab has no rows.
func (m *Model) activeLogRevision() string {
	if !m.logTab {
		return ""
	}
	entry, ok := m.selectedLogEntry()
	if !ok {
		return ""
	}
	return entry.ID
}

// logCommitMarked reports whether id is one of the marked revisions.
func (m *Model) logCommitMarked(id string) bool {
	for _, marked := range m.markedCommits {
		if marked == id {
			return true
		}
	}
	return false
}

// toggleLogCommitMark marks or unmarks the selected commit. The set is capped
// at two revisions because that is what one comparison can express.
func (m *Model) toggleLogCommitMark() tea.Cmd {
	entry, ok := m.selectedLogEntry()
	if !ok {
		m.setMessage("No commit selected to mark")
		return nil
	}
	if m.logCommitMarked(entry.ID) {
		remaining := m.markedCommits[:0]
		for _, id := range m.markedCommits {
			if id != entry.ID {
				remaining = append(remaining, id)
			}
		}
		m.markedCommits = remaining
		m.setMessage(fmt.Sprintf("Unmarked %s (%d marked)", entry.ShortID, len(m.markedCommits)))
		return nil
	}
	if len(m.markedCommits) >= maxMarkedCommits {
		m.setMessage(fmt.Sprintf("Two commits are already marked; unmark one before marking %s", entry.ShortID))
		return nil
	}
	m.markedCommits = append(m.markedCommits, entry.ID)
	m.setMessage(fmt.Sprintf("Marked %s (%d marked; alt+c compares)", entry.ShortID, len(m.markedCommits)))
	return nil
}

// logComparison is the resolved diff a mark set describes. One mark compares a
// commit against its first parent. Two marks compare two arbitrary revisions
// from their merge base, so neither mark order nor commit timestamps can
// change the result or silently invert the diff.
type logComparison struct {
	Base, Target string
	TripleDot    bool
	Label        string
}

// logCompareRevisions resolves the marked set into an explicit diff.
func logCompareRevisions(m *Model) (logComparison, error) {
	switch len(m.markedCommits) {
	case 0:
		return logComparison{}, fmt.Errorf("mark one or two commits first (alt+m)")
	case 1:
		return logCompareAgainstParent(m, m.markedCommits[0])
	case maxMarkedCommits:
		return logCompareMarkedPair(m, m.markedCommits[0], m.markedCommits[1])
	default:
		return logComparison{}, fmt.Errorf("too many marked commits")
	}
}

// logCompareAgainstParent compares one marked commit with its first parent.
func logCompareAgainstParent(m *Model, marked string) (logComparison, error) {
	revision, err := m.repo.ResolveRevision(m.appCtx, marked)
	if err != nil {
		return logComparison{}, err
	}
	if len(revision.ParentIDs) == 0 {
		return logComparison{}, fmt.Errorf("commit %s has no parent to compare", shortID(revision.ID))
	}
	return logComparison{Base: revision.ParentIDs[0], Target: revision.ID, Label: shortID(revision.ID)}, nil
}

// logCompareMarkedPair compares two marked commits from their merge base, so
// the diff is the work between them whichever order they were marked in.
func logCompareMarkedPair(m *Model, first, second string) (logComparison, error) {
	a, err := m.repo.ResolveRevision(m.appCtx, first)
	if err != nil {
		return logComparison{}, err
	}
	b, err := m.repo.ResolveRevision(m.appCtx, second)
	if err != nil {
		return logComparison{}, err
	}
	base, target, err := m.orderByAncestry(a, b)
	if err != nil {
		return logComparison{}, err
	}
	return logComparison{Base: base.ID, Target: target.ID, TripleDot: true, Label: shortID(base.ID) + "..." + shortID(target.ID)}, nil
}

// orderByAncestry puts the older revision first. Commit timestamps cannot do
// this: git records one-second precision, so two commits made in the same
// second compare as neither being older. Unrelated branches keep mark order,
// which merge-base semantics makes harmless.
func (m *Model) orderByAncestry(a, b gitbackend.Revision) (gitbackend.Revision, gitbackend.Revision, error) {
	ancestor, err := m.repo.IsAncestor(m.appCtx, a.ID, b.ID)
	if err != nil {
		return a, b, err
	}
	if ancestor {
		return a, b, nil
	}
	reverse, err := m.repo.IsAncestor(m.appCtx, b.ID, a.ID)
	if err != nil {
		return a, b, err
	}
	if reverse {
		return b, a, nil
	}
	return a, b, nil
}

// compareMarkedCommits renders the diff the marked revisions describe. Both the
// base and the target are explicit resolved object IDs, unlike the ediff
// comparison which always falls back to the selected commit's first parent.
func compareMarkedCommits(m *Model) tea.Cmd {
	comparison, err := logCompareRevisions(m)
	if err != nil {
		m.setError(err)
		return nil
	}
	m.logTitle = "Compare " + comparison.Label
	return loadInspection(m, m.logTitle, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryDiff(ctx, gitbackend.DiffQuery{
			Kind:        gitbackend.DiffRevisionRange,
			Base:        comparison.Base,
			Target:      comparison.Target,
			TripleDot:   comparison.TripleDot,
			OutputLimit: inspectOutputLimit,
		})
		if err != nil {
			return "", err
		}
		return truncationNote(result.Truncated) + result.Detail, nil
	})
}

// handleLogTabKey owns raw navigation keys while the Log tab is active. It runs
// before the keymap resolver so the tab keeps list motion even though the Magit
// bindings it inherits are section-oriented.
func (m *Model) handleLogTabKey(key string) (tea.Cmd, bool) {
	if len(m.logEntries) == 0 {
		return nil, false
	}
	page := max(1, m.detailViewportHeight())
	switch key {
	case "up", "k":
		return m.moveLogCursor(m.logCursor - 1), true
	case "down", "j":
		return m.moveLogCursor(m.logCursor + 1), true
	case "ctrl+u":
		return m.moveLogCursor(m.logCursor - max(1, page/2)), true
	case "ctrl+d":
		return m.moveLogCursor(m.logCursor + max(1, page/2)), true
	case "ctrl+b", "pageup":
		return m.moveLogCursor(m.logCursor - page), true
	case "ctrl+f", "pagedown":
		return m.moveLogCursor(m.logCursor + page), true
	case "home":
		return m.moveLogCursor(0), true
	case "end":
		return m.moveLogCursor(len(m.logEntries) - 1), true
	}
	return nil, false
}

// moveLogCursor moves the selection, scrolls the list to keep the row visible,
// and reloads the detail pane for the newly selected commit.
func (m *Model) moveLogCursor(target int) tea.Cmd {
	target = clamp(0, target, len(m.logEntries)-1)
	if target == m.logCursor {
		return nil
	}
	m.logCursor = target
	m.clampLogOffset()
	return m.loadLogDetailCmd()
}

// clampLogOffset scrolls the minimum amount that keeps the cursor visible.
func (m *Model) clampLogOffset() {
	height := max(1, m.logListViewportHeight())
	if m.logCursor < m.logOffset {
		m.logOffset = m.logCursor
	}
	if m.logCursor >= m.logOffset+height {
		m.logOffset = m.logCursor - height + 1
	}
	m.logOffset = clamp(0, m.logOffset, max(0, len(m.logEntries)-height))
}

// logListViewportHeight returns the visible row count of the commit list.
func (m *Model) logListViewportHeight() int {
	body := m.height - 4 - m.tabStripRows()
	if m.width < logTabWidth {
		body -= body / 2
	}
	return max(1, body-2)
}

// renderLogTabBody renders the Log tab's list and detail panes. The layout
// mirrors the Status tab so switching views does not change the terminal
// geometry the user is used to.
func (m *Model) renderLogTabBody(bodyHeight int) string {
	if m.compact {
		return m.renderCompactLogBody(bodyHeight)
	}
	if bodyHeight < 3 {
		return fitBlock("Log", m.width, bodyHeight)
	}
	if m.width >= logTabWidth && !m.splitHorizontal {
		// The list is this tab's primary surface, and its columns are what make
		// refs and topology readable, so it takes a wider share than the Status
		// tab gives its section tree.
		left := max(36, m.width*logListShare/100)
		right := m.width - left
		return lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderLogListPanel(left, bodyHeight),
			m.renderDetailPanel(right, bodyHeight),
		)
	}
	if bodyHeight >= 7 {
		panelHeight := bodyHeight - 1
		listHeight := max(3, panelHeight*45/100)
		listHeight = min(listHeight, panelHeight-3)
		return m.renderLogListPanel(m.width, listHeight) + "\n" + m.renderDetailPanel(m.width, panelHeight-listHeight)
	}
	return m.renderLogListPanel(m.width, bodyHeight)
}

func (m *Model) renderCompactLogBody(bodyHeight int) string {
	if bodyHeight <= 0 {
		return ""
	}
	listHeight := max(1, bodyHeight/2)
	if listHeight >= bodyHeight {
		return m.renderLogListPanel(m.width, bodyHeight)
	}
	return m.renderLogListPanel(m.width, listHeight) + "\n" + m.renderDetailPanel(m.width, bodyHeight-listHeight)
}

// renderLogListPanel draws the commit list with its selection gutter.
func (m *Model) renderLogListPanel(width, height int) string {
	if width < 3 || height < 3 {
		return fitBlock("Log", width, height)
	}
	innerW, innerH := width-2, height-2
	lines := m.logListLines(innerW)
	start := clamp(0, m.logOffset, max(0, len(lines)-innerH))
	end := min(len(lines), start+innerH)
	rows := make([]string, 0, innerH)
	if len(lines) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(truncate("No commits", innerW)))
	}
	for index := start; index < end; index++ {
		rows = append(rows, lines[index])
	}
	for len(rows) < innerH {
		rows = append(rows, "")
	}
	return panelStyle(width, height).Render(strings.Join(rows, "\n"))
}

// logListLines renders every commit row, including those scrolled out of view,
// so the cursor can be positioned without re-laying out the whole list.
func (m *Model) logListLines(width int) []string {
	if width < 12 {
		return nil
	}
	layout := m.logLayout(width)
	lines := make([]string, 0, len(m.logEntries))
	for index, entry := range m.logEntries {
		marked := m.logCommitMarked(entry.ID)
		lines = append(lines, m.logRowStyle(entry, index == m.logCursor, marked).Render(m.logRowText(entry, width, layout, marked)))
	}
	return lines
}

func (m *Model) logRowStyle(entry gitbackend.LogEntry, selected, marked bool) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(colorText)
	switch {
	case selected && marked:
		return style.Foreground(colorOnAccent).Background(colorPurple).Bold(true)
	case selected:
		return style.Foreground(colorOnAccent).Background(colorCyan).Bold(true)
	case marked:
		return style.Foreground(colorPurple).Bold(true)
	case entry.Decorations != "":
		return style.Foreground(colorGold)
	}
	return style
}

// logLayout holds the column widths shared by every row in the Log tab.
// Measuring the list once per render is what keeps the columns aligned: picking
// a column set per row would let a long subject drop the date on one row and
// keep it on the next.
type logLayout struct {
	graph       int
	decorations int
	date        bool
	author      int
}

// logLayout measures the loaded list and drops columns, least important first,
// until the fixed columns leave room for a useful subject. The subject is the
// one column a log row cannot render without, so it is never dropped. The
// layout reserves a subject *budget* rather than demanding that the widest
// subject fit: requiring a full fit would strip every column in any repository
// that has one long commit message, which is nearly all of them.
func (m *Model) logLayout(width int) logLayout {
	layout := logLayout{}
	widestSubject := 0
	for _, entry := range m.logEntries {
		layout.graph = max(layout.graph, ansi.StringWidth(strings.TrimSpace(entry.Graph)))
		layout.decorations = max(layout.decorations, ansi.StringWidth(entry.Decorations))
		layout.author = max(layout.author, ansi.StringWidth(strings.TrimSpace(entry.AuthorName)))
		if !entry.AuthorDate.IsZero() {
			layout.date = true
		}
		widestSubject = max(widestSubject, ansi.StringWidth(strings.TrimSpace(entry.Subject)))
	}
	layout.graph = min(layout.graph, logGraphWidth)
	layout.decorations = min(layout.decorations, logDecorationsWidth)
	layout.author = min(layout.author, logAuthorWidth)
	subject := min(widestSubject, logSubjectBudget)
	// Which branch or tag a commit is on is worth more than who wrote it, and
	// topology is worth more than either, so refs and lanes are the last columns
	// to go. The date goes first: a commit list is scanned by subject and ref far
	// more often than by when the commit landed.
	for layout.width()+1+subject > width {
		switch {
		case layout.date:
			layout.date = false
		case layout.author > 0:
			layout.author = 0
		case layout.decorations > 0:
			layout.decorations = 0
		case layout.graph > 0:
			layout.graph = 0
		default:
			return layout
		}
	}
	return layout
}

// width is the fixed portion of a row, excluding the subject.
func (l logLayout) width() int {
	width := logGutterWidth + logShortIDWidth
	if l.graph > 0 {
		width += 1 + l.graph
	}
	if l.decorations > 0 {
		// The decoration column carries its own brackets.
		width += 1 + l.decorations + 2
	}
	if l.date {
		width += 1 + logDateWidth
	}
	if l.author > 0 {
		width += 1 + l.author
	}
	return width
}

// logRowText renders one commit row against the list's shared layout.
func (m *Model) logRowText(entry gitbackend.LogEntry, width int, layout logLayout, marked bool) string {
	mark := logMarkBlank
	if marked {
		mark = logMarkGlyph
	}
	head := mark + " "
	if layout.graph > 0 {
		head += fitCell(strings.TrimSpace(entry.Graph), layout.graph) + " "
	}
	head += fitCell(entry.ShortID, logShortIDWidth)
	parts := []string{head}
	// Refs sit beside the short id so that truncating a long subject can never
	// hide which branch or tag a commit is on.
	if layout.decorations > 0 {
		column := strings.Repeat(" ", layout.decorations+2)
		if entry.Decorations != "" {
			column = "(" + padRight(fitDecorations(entry.Decorations, layout.decorations), layout.decorations) + ")"
		}
		parts = append(parts, column)
	}
	if layout.date && !entry.AuthorDate.IsZero() {
		parts = append(parts, entry.AuthorDate.Format("2006-01-02"))
	}
	if layout.author > 0 {
		parts = append(parts, fitCell(strings.TrimSpace(entry.AuthorName), layout.author))
	}
	parts = append(parts, strings.TrimSpace(entry.Subject))
	return truncate(strings.Join(parts, " "), width)
}

// fitCell truncates a column value to its width and then pads it, so one
// over-long value cannot widen that column for every other row.
func fitCell(value string, width int) string {
	return padRight(truncate(value, width), width)
}

// fitDecorations keeps whole ref entries and drops the tail that will not fit,
// so a ref list never ends mid-name. Truncating the raw string would cut
// "HEAD -> main, tag: v0.1.0, origin/main" into "HEAD -> main, tag: v0.0", which
// reads as a different ref than the one Git reported.
func fitDecorations(decorations string, width int) string {
	if ansi.StringWidth(decorations) <= width {
		return decorations
	}
	parts := strings.Split(decorations, ", ")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := append(append([]string{}, kept...), part)
		if ansi.StringWidth(strings.Join(candidate, ", "))+1 > width {
			break
		}
		kept = candidate
	}
	if len(kept) == 0 {
		return "…"
	}
	return strings.Join(kept, ", ") + "…"
}

// padRight pads a column to an exact display width so the next column starts in
// the same place on every row.
func padRight(value string, width int) string {
	if gap := width - ansi.StringWidth(value); gap > 0 {
		return value + strings.Repeat(" ", gap)
	}
	return value
}

// renderTabStrip draws the view switcher. There are no number keys because the
// Doom scheme deliberately leaves Magit's M-1..M-4 unbound, so the strip stays
// a label pair and the footer carries the keys that change views.
func (m *Model) renderTabStrip() string {
	active := lipgloss.NewStyle().Foreground(colorOnAccent).Background(colorPurple).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(colorMuted)
	divider := lipgloss.NewStyle().Foreground(colorBorder)
	status := inactive.Render("Status")
	if !m.logTab {
		status = active.Render("Status")
	}
	logTab := inactive.Render("Log")
	if m.logTab {
		logTab = active.Render("Log")
	}
	summary := ""
	if m.logTab && len(m.logEntries) > 0 {
		label := fmt.Sprintf("%d commits", len(m.logEntries))
		if m.logTitle != "" && m.logTitle != "Log" {
			label = m.logTitle + "  " + label
		}
		summary = inactive.Render("  " + label)
	}
	strip := status + divider.Render("  |  ") + logTab + summary
	return lipgloss.NewStyle().Width(m.width).Render(truncate(strip, m.width))
}

// logTabFooter describes the Log tab's own keys instead of the Status tab's
// global hint line, so the footer stays relevant to what is on screen.
func (m *Model) logTabFooter() string {
	gold := lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	// Esc closes an open comparison before it leaves the tab, so the hint has to
	// say which one the next Esc will do.
	escape := "  Esc status"
	if m.inspectionActive {
		escape = "  Esc close comparison"
	}
	hints := gold.Render("Log") + muted.Render("  j/k select  alt+m mark  alt+c compare"+escape+"  alt+| swap split  q close")
	if len(m.markedCommits) > 0 {
		hints += muted.Render("  (" + m.markedCommitLabel() + " marked)")
	}
	return hints
}

func (m *Model) markedCommitLabel() string {
	labels := make([]string, 0, len(m.markedCommits))
	for _, id := range m.markedCommits {
		labels = append(labels, shortID(id))
	}
	return strings.Join(labels, " ")
}

// clamp constrains value to [low, high] without the max/min ordering trap of
// hand-written bounds when the caller passes an inverted range.
func clamp(low, value, high int) int {
	if high < low {
		return low
	}
	return min(max(low, value), high)
}

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			keymap.CommandLogTab:         logTabCurrent,
			keymap.CommandMarkCommit:     func(m *Model, _ WorkflowCommand) tea.Cmd { return m.toggleLogCommitMark() },
			keymap.CommandCompareCommits: func(m *Model, _ WorkflowCommand) tea.Cmd { return compareMarkedCommits(m) },
			keymap.CommandGraph:          logTabAllRefs,
		}
	})
}
