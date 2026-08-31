package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func (m *Model) focusedHunkRequest(key string) (gitbackend.InteractiveChangeRequest, bool) {
	row, action, ok := m.focusedInteractiveAction(key)
	if !ok {
		return gitbackend.InteractiveChangeRequest{}, false
	}
	hunk, start, end, ok := m.focusedPatchCoordinates()
	if !ok {
		return gitbackend.InteractiveChangeRequest{}, false
	}
	request := gitbackend.InteractiveChangeRequest{
		Action: action, Scope: gitbackend.InteractiveChangeHunk, Path: row.path, Hunk: hunk,
		DiffContext: m.diffContext, DiffContextSet: true,
	}
	if len(m.detailSelectedHunks) != 0 || len(m.detailSelections) != 0 {
		request.Scope, request.Selections = gitbackend.InteractiveChangeSelections, m.patchSelections(hunk, start, end)
	} else if m.detailRangeStart >= 0 && m.detailRangeEnd >= 0 {
		request.Scope, request.Start, request.End = gitbackend.InteractiveChangeLines, start, end
	}
	return request, true
}

// patchSelections joins explicitly selected hunks with pinned and active line
// regions. The backend canonicalizes and validates this typed representation;
// the UI never derives a patch from its display text.
func (m *Model) patchSelections(hunk, start, end int) []gitbackend.InteractiveChangeSelection {
	selections := append([]gitbackend.InteractiveChangeSelection(nil), m.detailSelections...)
	for selectedHunk := range m.detailSelectedHunks {
		selections = append(selections, gitbackend.InteractiveChangeSelection{Hunk: selectedHunk, WholeHunk: true})
	}
	if m.detailRangeStart >= 0 && m.detailRangeEnd >= 0 {
		selections = append(selections, gitbackend.InteractiveChangeSelection{Hunk: hunk, Start: start, End: end})
	}
	return selections
}

func (m *Model) focusedInteractiveAction(key string) (selectedRow row, action gitbackend.InteractiveChangeAction, ok bool) {
	if m.repo == nil || m.detailHunk < 0 || m.detailID != m.tree.Cursor() || m.inspectionActive {
		return row{}, 0, false
	}
	selected, ok := m.rows[m.tree.Cursor()]
	if !ok || selected.path == "" {
		return row{}, 0, false
	}
	action, ok = interactiveChangeAction(m.scheme, key, selected.kind)
	return selected, action, ok
}

func interactiveChangeAction(scheme keyScheme, key string, kind rowKind) (gitbackend.InteractiveChangeAction, bool) {
	discardKey := map[keyScheme]string{schemeVim: "x", schemeMagit: "k"}[scheme]
	type actionKey struct {
		key  string
		kind rowKind
	}
	actions := map[actionKey]gitbackend.InteractiveChangeAction{
		{"s", rowUnstaged}:        gitbackend.InteractiveChangeStage,
		{"u", rowStaged}:          gitbackend.InteractiveChangeUnstage,
		{discardKey, rowUnstaged}: gitbackend.InteractiveChangeDiscardUnstaged,
		{discardKey, rowStaged}:   gitbackend.InteractiveChangeDiscardStaged,
	}
	action, ok := actions[actionKey{key, kind}]
	return action, ok
}

func (m *Model) focusedPatchCoordinates() (hunk, start, end int, ok bool) {
	lines := m.detailLines()
	low, high := min(m.detailRangeStart, m.detailRangeEnd), max(m.detailRangeStart, m.detailRangeEnd)
	start, end = patchRangeCoordinates(lines, low, high)
	hunk = focusedPatchHunk(lines, m.detailHunk)
	if hunk < 0 {
		return 0, 0, 0, false
	}
	if m.detailRangeStart < 0 || m.detailRangeEnd < 0 {
		return hunk, 0, 0, true
	}
	return hunk, start, end, start >= 0 && end > start
}

func patchRangeCoordinates(lines []string, low, high int) (start, end int) {
	hunk, lineIndex := -1, 0
	start, end = -1, -1
	for index, line := range lines {
		if strings.HasPrefix(line, "@@") {
			hunk++
			lineIndex = 0
			continue
		}
		if hunk < 0 || strings.HasPrefix(line, "\\ No newline at end of file") {
			continue
		}
		if index >= low && index <= high {
			if start < 0 {
				start = lineIndex
			}
			end = lineIndex + 1
		}
		lineIndex++
	}
	return start, end
}

func focusedPatchHunk(lines []string, detailHunk int) int {
	focused := -1
	for index, line := range lines {
		if strings.HasPrefix(line, "@@") {
			focused++
		}
		if index == detailHunk {
			break
		}
	}
	return focused
}

func (m *Model) handlePatchHunkSelectionKey(key string) bool {
	if key != "V" || m.detailHunk < 0 || m.detailID != m.tree.Cursor() {
		return false
	}
	if m.detailRangeStart >= 0 {
		m.setMessage("Finish or cancel line selection before selecting hunks")
		return true
	}
	hunk, _, _, ok := m.focusedPatchCoordinates()
	if !ok {
		return false
	}
	if m.detailSelectedHunks == nil {
		m.detailSelectedHunks = make(map[int]bool)
	}
	m.detailSelectedHunks[hunk] = !m.detailSelectedHunks[hunk]
	if !m.detailSelectedHunks[hunk] {
		delete(m.detailSelectedHunks, hunk)
	}
	m.setMessage(fmt.Sprintf("Hunk %d %s (%d selected); [/] navigate, V toggle, s/u/x review", hunk+1, map[bool]string{true: "selected", false: "unselected"}[m.detailSelectedHunks[hunk]], len(m.detailSelectedHunks)))
	return true
}

func (m *Model) handlePatchRangeKey(key string) bool {
	if m.detailHunk < 0 || m.detailID != m.tree.Cursor() {
		return false
	}
	if key == "v" {
		return m.togglePatchRange()
	}
	if key == "space" && m.detailRangeStart >= 0 {
		return m.pinPatchRange()
	}
	delta := patchRangeDelta(m.scheme, key)
	if delta == 0 {
		return false
	}
	if m.detailRangeStart >= 0 {
		m.movePatchRange(delta)
		return true
	}
	if len(m.detailSelections) != 0 {
		m.movePatchCursor(delta)
		return true
	}
	return false
}

func (m *Model) togglePatchRange() bool {
	if m.detailRangeStart >= 0 {
		m.detailRangeStart, m.detailRangeEnd = -1, -1
		m.setMessage("Line selection cancelled")
		return true
	}
	changed := m.focusedHunkChangedLines()
	if len(changed) == 0 {
		m.setMessage("Focused hunk has no selectable changed lines")
		return true
	}
	if !containsDetailLine(changed, m.detailLine) {
		m.detailLine = changed[0]
	}
	m.detailRangeStart, m.detailRangeEnd = m.detailLine, m.detailLine
	movement := "j/k"
	if m.scheme == schemeMagit {
		movement = "n/p"
	}
	m.setMessage("Line selection active; " + movement + " extend, Space add region, s/u/x review, v cancel")
	return true
}

func containsDetailLine(lines []int, line int) bool {
	for _, candidate := range lines {
		if candidate == line {
			return true
		}
	}
	return false
}

func (m *Model) pinPatchRange() bool {
	hunk, start, end, ok := m.focusedPatchCoordinates()
	if !ok {
		m.setMessage("Selected line range is invalid")
		return true
	}
	m.detailSelections = append(m.detailSelections, gitbackend.InteractiveChangeSelection{Hunk: hunk, Start: start, End: end})
	m.detailRangeStart, m.detailRangeEnd = -1, -1
	movement := "j/k"
	if m.scheme == schemeMagit {
		movement = "n/p"
	}
	m.setMessage(fmt.Sprintf("Region added (%d); %s move, v select another, s/u/x review", len(m.detailSelections), movement))
	return true
}

func patchRangeDelta(scheme keyScheme, key string) int {
	if scheme == schemeVim {
		return map[string]int{"j": 1, "k": -1}[key]
	}
	return map[string]int{"n": 1, "p": -1}[key]
}

func (m *Model) movePatchRange(delta int) {
	if m.movePatchCursor(delta) {
		m.detailRangeEnd = m.detailLine
	}
}

func (m *Model) movePatchCursor(delta int) bool {
	changed := m.focusedHunkChangedLines()
	for index, line := range changed {
		if line != m.detailLine {
			continue
		}
		next := index + delta
		if next >= 0 && next < len(changed) {
			m.detailLine = changed[next]
			m.detailOffset = min(m.detailLine, m.detailMaximumOffset())
		}
		return true
	}
	if len(changed) != 0 {
		m.detailLine = changed[0]
		m.detailOffset = min(m.detailLine, m.detailMaximumOffset())
		return true
	}
	return false
}

func (m *Model) focusedHunkChangedLines() []int {
	lines := m.detailLines()
	var changed []int
	inside := false
	for index, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if index == m.detailHunk {
				inside = true
				continue
			}
			if inside {
				break
			}
		}
		if inside && (strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")) {
			changed = append(changed, index)
		}
	}
	return changed
}

func (m *Model) openInteractiveChange(request gitbackend.InteractiveChangeRequest) tea.Cmd {
	title, operation := interactiveChangeLabels(request.Action)
	selection := "focused hunk"
	switch request.Scope {
	case gitbackend.InteractiveChangeLines:
		title = strings.Replace(title, "focused hunk", "selected lines", 1)
		operation = strings.Replace(operation, "hunk", "lines", 1)
		selection = "selected changed-line range"
	case gitbackend.InteractiveChangeSelections:
		whole, regions := 0, 0
		for _, selected := range request.Selections {
			if selected.WholeHunk {
				whole++
			} else {
				regions++
			}
		}
		name := "selected regions"
		if whole != 0 && regions == 0 {
			name = "selected hunk"
			if whole != 1 {
				name = "selected hunks"
			}
		} else if whole != 0 {
			name = "selected hunks and regions"
		}
		title = interactiveSelectionTitle(title, name)
		operation = strings.Replace(operation, "hunk", strings.TrimPrefix(name, "selected "), 1)
		selection = name
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: title, Operation: operation,
		Plan: []string{"Select the exact " + selection, "Revalidate the source diff immediately before mutation"},
		ReviewPreflight: func(ctx context.Context, _ WorkflowValues) (WorkflowReview, error) {
			reviewed, err := m.repo.ReviewInteractiveChange(ctx, request)
			if err != nil {
				return WorkflowReview{}, err
			}
			location := fmt.Sprintf("hunk: %d", request.Hunk+1)
			if request.Scope == gitbackend.InteractiveChangeSelections {
				location = fmt.Sprintf("hunks: %d; selections: %d", len(reviewed.HunkHeadings), len(request.Selections))
			}
			plan := []string{
				"path: " + sanitizeSingleLine(request.Path),
				location + fmt.Sprintf("; changed lines: %d", reviewed.ChangedLines),
				"patch: " + reviewed.PatchHash[:min(12, len(reviewed.PatchHash))],
			}
			if heading := strings.TrimSpace(reviewed.HunkHeading); heading != "" {
				plan = append(plan, "heading: "+sanitizeSingleLine(heading))
			}
			return WorkflowReview{Plan: plan, Confirmation: "Execute this exact reviewed hunk mutation?", Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			reviewed, ok := review.Data.(gitbackend.ReviewedInteractiveChange)
			if !ok {
				return errors.New("invalid interactive change review")
			}
			return m.repo.ExecuteReviewedInteractiveChange(ctx, reviewed)
		},
	})
}

func interactiveSelectionTitle(title, selection string) string {
	if strings.Contains(title, "focused unstaged hunk") {
		return strings.Replace(title, "focused unstaged hunk", strings.Replace(selection, "selected ", "selected unstaged ", 1), 1)
	}
	if strings.Contains(title, "focused staged hunk") {
		return strings.Replace(title, "focused staged hunk", strings.Replace(selection, "selected ", "selected staged ", 1), 1)
	}
	return strings.Replace(title, "focused hunk", selection, 1)
}

func interactiveChangeLabels(action gitbackend.InteractiveChangeAction) (string, string) {
	switch action {
	case gitbackend.InteractiveChangeStage:
		return "Stage focused hunk", "stage hunk"
	case gitbackend.InteractiveChangeUnstage:
		return "Unstage focused hunk", "unstage hunk"
	case gitbackend.InteractiveChangeDiscardUnstaged:
		return "Discard focused unstaged hunk", "discard unstaged hunk"
	case gitbackend.InteractiveChangeDiscardStaged:
		return "Discard focused staged hunk", "discard staged hunk"
	default:
		return "Interactive change", "interactive change"
	}
}
