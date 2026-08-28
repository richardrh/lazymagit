package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

// conflictResolutionWorkflow is the terminal adaptation of E t m
// (magit-git-mergetool). It never starts an external tool: users inspect the
// three index stages with e, then select Git's portable ours/theirs checkout.
func conflictResolutionWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("resolve conflict", func(ctx context.Context) (WorkflowDialog, error) {
		paths, err := m.repo.UnmergedPaths(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		if len(paths) == 0 {
			return WorkflowDialog{}, errors.New("no unresolved paths")
		}
		choices := make([]WorkflowChoice, 0, len(paths))
		for _, path := range paths {
			choices = append(choices, WorkflowChoice{Value: path.Path, Label: sanitizeSingleLine(path.Path) + " (" + conflictStagesLabel(path.Stages) + ")"})
		}
		selected := choices[0].Value
		if row, ok := m.rows[m.tree.Cursor()]; ok && isUnmergedSnapshotPath(m, row.path) {
			selected = row.path
		}
		return WorkflowDialog{
			Title: "Resolve conflict", Operation: "resolve conflict",
			Plan: []string{
				"Inspect base / ours / theirs first with e on an unmerged path",
				"Replace the worktree path with Git's selected version and stage it",
				"Base is inspect-only: stock Git has no safe base checkout mode",
			},
			Fields: []WorkflowField{
				{Name: "path", Label: "Unresolved path", Kind: WorkflowSelect, Value: selected, Choices: choices, Required: true},
				{Name: "resolution", Label: "Resolution", Kind: WorkflowSelect, Value: "ours", Choices: []WorkflowChoice{{Value: "ours", Label: "Ours (Git stage 2)"}, {Value: "theirs", Label: "Theirs (Git stage 3)"}}, Required: true},
			},
			ReviewPreflight: func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
				resolution, err := conflictResolutionValue(values["resolution"])
				if err != nil {
					return WorkflowReview{}, err
				}
				review, err := m.repo.ReviewConflictResolution(ctx, values["path"], resolution)
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: []string{
					"Path: " + sanitizeSingleLine(review.Path),
					"Use Git's " + values["resolution"] + " version, then stage this path",
					"The exact unmerged index entries and worktree content must remain unchanged",
				}, Confirmation: "Press Enter again to resolve and stage", Data: review}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, transported WorkflowReview) error {
				review, ok := transported.Data.(gitbackend.ReviewedConflictResolution)
				if !ok {
					return errors.New("invalid conflict resolution review")
				}
				return m.repo.ExecuteReviewedConflictResolution(ctx, review)
			},
		}, nil
	})
}

func conflictResolutionValue(value string) (gitbackend.ConflictResolution, error) {
	switch value {
	case "ours":
		return gitbackend.ResolveOurs, nil
	case "theirs":
		return gitbackend.ResolveTheirs, nil
	case "base":
		return gitbackend.ResolveBase, fmt.Errorf("base resolution is unavailable: stock Git has no safe base checkout mode")
	default:
		return 0, errors.New("unknown conflict resolution")
	}
}

func conflictStagesLabel(stages []gitbackend.ConflictStage) string {
	labels := make([]string, 0, len(stages))
	for _, stage := range stages {
		switch stage {
		case gitbackend.ConflictBase:
			labels = append(labels, "base")
		case gitbackend.ConflictOurs:
			labels = append(labels, "ours")
		case gitbackend.ConflictTheirs:
			labels = append(labels, "theirs")
		}
	}
	return strings.Join(labels, "/")
}

func isUnmergedSnapshotPath(m *Model, path string) bool {
	for _, file := range m.snapshot.status.Files {
		if file.Path == path && (file.Staged == gitbackend.ChangeUnmerged || file.Unstaged == gitbackend.ChangeUnmerged) {
			return true
		}
	}
	return false
}

// inspectConflictVersions is bound to e / magit-ediff-dwim when the selected
// status row is unresolved. It renders every extant index stage in the detail
// pane without touching the worktree or index.
func inspectConflictVersions(m *Model, path string) tea.Cmd {
	return loadInspection(m, "Conflict versions: "+sanitizeSingleLine(path), func(ctx context.Context) (string, error) {
		inspection, err := m.repo.InspectConflict(ctx, path)
		if err != nil {
			return "", err
		}
		var text strings.Builder
		for _, blob := range inspection.Blobs {
			fmt.Fprintf(&text, "--- %s (stage %d, %s, %s) ---\n", conflictStageLabel(blob.Stage), blob.Stage, blob.Mode, blob.OID)
			if bytesAreBinary(blob.Content) {
				text.WriteString("[binary content; not rendered in terminal]\n")
			} else {
				text.Write(blob.Content)
				if len(blob.Content) != 0 && blob.Content[len(blob.Content)-1] != '\n' {
					text.WriteByte('\n')
				}
			}
			if blob.Truncated {
				text.WriteString("[... stage content truncated ...]\n")
			}
			text.WriteByte('\n')
		}
		return text.String(), nil
	})
}

func conflictStageLabel(stage gitbackend.ConflictStage) string {
	switch stage {
	case gitbackend.ConflictBase:
		return "base"
	case gitbackend.ConflictOurs:
		return "ours"
	case gitbackend.ConflictTheirs:
		return "theirs"
	default:
		return "unknown"
	}
}

func bytesAreBinary(content []byte) bool {
	return !utf8.Valid(content) || strings.IndexByte(string(content), 0) >= 0
}
