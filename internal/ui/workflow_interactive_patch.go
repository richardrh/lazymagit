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
	request := gitbackend.InteractiveChangeRequest{Action: action, Scope: gitbackend.InteractiveChangeHunk, Path: row.path, Hunk: hunk}
	if m.detailRangeStart >= 0 && m.detailRangeEnd >= 0 {
		request.Scope, request.Start, request.End = gitbackend.InteractiveChangeLines, start, end
	}
	return request, true
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
	hunk, lineIndex := -1, 0
	start, end = -1, -1
	low, high := min(m.detailRangeStart, m.detailRangeEnd), max(m.detailRangeStart, m.detailRangeEnd)
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
	focused := -1
	for index, line := range lines {
		if strings.HasPrefix(line, "@@") {
			focused++
		}
		if index == m.detailHunk {
			break
		}
	}
	if focused < 0 {
		return 0, 0, 0, false
	}
	if m.detailRangeStart < 0 || m.detailRangeEnd < 0 {
		return focused, 0, 0, true
	}
	return focused, start, end, start >= 0 && end > start
}

func (m *Model) handlePatchRangeKey(key string) bool {
	if m.detailHunk < 0 || m.detailID != m.tree.Cursor() {
		return false
	}
	if key == "v" {
		return m.togglePatchRange()
	}
	if m.detailRangeStart < 0 {
		return false
	}
	delta := patchRangeDelta(m.scheme, key)
	if delta == 0 {
		return false
	}
	m.movePatchRange(delta)
	return true
}

func (m *Model) togglePatchRange() bool {
	if m.detailRangeStart >= 0 {
		m.detailLine, m.detailRangeStart, m.detailRangeEnd = -1, -1, -1
		m.setMessage("Line selection cancelled")
		return true
	}
	changed := m.focusedHunkChangedLines()
	if len(changed) == 0 {
		m.setMessage("Focused hunk has no selectable changed lines")
		return true
	}
	m.detailLine, m.detailRangeStart, m.detailRangeEnd = changed[0], changed[0], changed[0]
	movement := "j/k"
	if m.scheme == schemeMagit {
		movement = "n/p"
	}
	m.setMessage("Line selection active; " + movement + " extend, s/u/x review, v cancel")
	return true
}

func patchRangeDelta(scheme keyScheme, key string) int {
	if scheme == schemeVim {
		return map[string]int{"j": 1, "k": -1}[key]
	}
	return map[string]int{"n": 1, "p": -1}[key]
}

func (m *Model) movePatchRange(delta int) {
	changed := m.focusedHunkChangedLines()
	for index, line := range changed {
		if line != m.detailLine {
			continue
		}
		next := index + delta
		if next >= 0 && next < len(changed) {
			m.detailLine = changed[next]
			m.detailRangeEnd = m.detailLine
			m.detailOffset = min(m.detailLine, m.detailMaximumOffset())
		}
		return
	}
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
	if request.Scope == gitbackend.InteractiveChangeLines {
		title = strings.Replace(title, "focused hunk", "selected lines", 1)
		operation = strings.Replace(operation, "hunk", "lines", 1)
		selection = "selected changed-line range"
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: title, Operation: operation,
		Plan: []string{"Select the exact " + selection, "Revalidate the source diff immediately before mutation"},
		ReviewPreflight: func(ctx context.Context, _ WorkflowValues) (WorkflowReview, error) {
			reviewed, err := m.repo.ReviewInteractiveChange(ctx, request)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := []string{
				"path: " + sanitizeSingleLine(request.Path),
				fmt.Sprintf("hunk: %d; changed lines: %d", request.Hunk+1, reviewed.ChangedLines),
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
