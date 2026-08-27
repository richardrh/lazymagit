package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

// History registration is derived from upstream identities. This intentionally
// contains no copied command IDs and remains correct for conditional duplicate
// rows in the Magit 4.7 manifest.
func init() {
	pickOptions := []string{"magit-cherry-pick:--mainline", "magit-merge:--strategy", "magit:--signoff"}
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-cherry-pick", map[string][]string{
		"magit-cherry-copy":  pickOptions,
		"magit-cherry-apply": pickOptions,
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-revert", map[string][]string{
		"magit-revert-and-commit": {"magit-cherry-pick:--mainline", "transient:magit-revert:--no-edit", "magit-merge:--strategy", "magit:--signoff"},
		"magit-revert-no-commit":  {"magit-cherry-pick:--mainline", "transient:magit-revert:--no-edit", "magit-merge:--strategy", "magit:--signoff"},
	})...)
	RegisterWorkflowDomain(func(m *Model) map[keymap.CommandID]WorkflowHandler {
		out := make(map[keymap.CommandID]WorkflowHandler)
		for _, binding := range keymap.Registry() {
			var handler WorkflowHandler
			switch binding.Context {
			case keymap.ContextStatus:
				handler = historyTopHandler(binding.UpstreamCommand)
			default:
				handler = historyTransientHandler(binding.Transient, binding.UpstreamCommand)
			}
			if handler != nil {
				out[binding.Command] = handler
			}
		}
		return out
	})
}

func historyTopHandler(upstream string) WorkflowHandler {
	switch upstream {
	case "magit-cherry-pick":
		return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, false, false, c) }
	case "magit-cherry-copy":
		return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, false, false, c) }
	case "magit-cherry-apply":
		return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, false, true, c) }
	case "magit-revert":
		return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, true, false, c) }
	case "magit-revert-no-commit":
		return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, true, true, c) }
	case "magit-rebase":
		return openRebaseWorkflow
	case "magit-reset", "magit-reset-quickly":
		return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetMixed, false) }
	case "magit-bisect":
		return openBisectStart
	}
	return nil
}

func historyTransientHandler(transient, upstream string) WorkflowHandler {
	switch transient {
	case "magit-cherry-pick":
		switch upstream {
		case "magit-cherry-copy":
			return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, false, false, c) }
		case "magit-cherry-apply":
			return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, false, true, c) }
		case "magit-sequencer-continue":
			return historyDirect("continue cherry-pick", (*gitbackend.Repository).CherryPickContinue)
		case "magit-sequencer-skip":
			return reviewedHistoryAction(gitbackend.HistoryUICherrySkip)
		case "magit-sequencer-abort":
			return reviewedHistoryAction(gitbackend.HistoryUICherryAbort)
		}
	case "magit-revert":
		switch upstream {
		case "magit-revert-and-commit":
			return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, true, false, c) }
		case "magit-revert-no-commit":
			return func(m *Model, c WorkflowCommand) tea.Cmd { return openPickWorkflow(m, true, true, c) }
		case "magit-sequencer-continue":
			return historyDirect("continue revert", (*gitbackend.Repository).RevertContinue)
		case "magit-sequencer-skip":
			return reviewedHistoryAction(gitbackend.HistoryUIRevertSkip)
		case "magit-sequencer-abort":
			return reviewedHistoryAction(gitbackend.HistoryUIRevertAbort)
		}
	case "magit-rebase":
		switch upstream {
		case "magit-rebase-branch", "magit-rebase-subset":
			return openRebaseWorkflow
		case "magit-rebase-onto-upstream":
			return openRebaseUpstream
		case "magit-rebase-onto-pushremote":
			return openRebasePushRemote
		case "magit-rebase-continue":
			return reviewedHistoryAction(gitbackend.HistoryUIRebaseContinue)
		case "magit-rebase-skip":
			return reviewedHistoryAction(gitbackend.HistoryUIRebaseSkip)
		case "magit-rebase-abort":
			return reviewedHistoryAction(gitbackend.HistoryUIRebaseAbort)
		case "magit-rebase-interactive", "magit-rebase-edit", "magit-rebase-edit-commit", "magit-rebase-reword-commit", "magit-rebase-remove-commit", "magit-rebase-autosquash":
			return historyUnavailable("Interactive rebase is adapted out: the bounded TUI field cannot safely preserve Magit's multiline todo semantics")
		}
	case "magit-reset":
		switch upstream {
		case "magit-reset-mixed", "magit-branch-reset":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetMixed, false) }
		case "magit-reset-soft":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetSoft, false) }
		case "magit-reset-hard":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetHard, false) }
		case "magit-reset-keep":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetKeep, false) }
		case "magit-reset-index":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetIndex, true) }
		case "magit-reset-worktree":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetWorktree, true) }
		case "magit-file-checkout":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openResetWorkflow(m, gitbackend.ResetFile, true) }
		}
	case "magit-bisect":
		switch upstream {
		case "magit-bisect-start":
			return openBisectStart
		case "magit-bisect-good":
			return func(m *Model, _ WorkflowCommand) tea.Cmd {
				return openBisectRevision(m, gitbackend.HistoryUIBisectGood)
			}
		case "magit-bisect-bad":
			return func(m *Model, _ WorkflowCommand) tea.Cmd { return openBisectRevision(m, gitbackend.HistoryUIBisectBad) }
		case "magit-bisect-skip":
			return func(m *Model, _ WorkflowCommand) tea.Cmd {
				return openBisectRevision(m, gitbackend.HistoryUIBisectSkip)
			}
		case "magit-bisect-mark":
			return openBisectMark
		case "magit-bisect-reset":
			return reviewedHistoryAction(gitbackend.HistoryUIBisectReset)
			// magit-bisect-run deliberately has no handler. Arbitrary argv execution
			// requires both an unsafe capability and a reviewed execution contract.
		}
	}
	return nil
}

func historyUnavailable(reason string) WorkflowHandler {
	return func(m *Model, _ WorkflowCommand) tea.Cmd { m.setError(errors.New(reason)); return nil }
}

func historyDirect(name string, action func(*gitbackend.Repository, context.Context) error) WorkflowHandler {
	return func(m *Model, _ WorkflowCommand) tea.Cmd {
		return m.StartWorkflowOperation(name, func(ctx context.Context) error { return action(m.repo, ctx) })
	}
}

func selectedHistoryRevision(m *Model) string {
	if current, ok := m.rows[m.tree.Cursor()]; ok && current.kind == rowCommit && current.commit.ID != "" {
		return current.commit.ID
	}
	return ""
}

func historyPickOptions(command WorkflowCommand) (gitbackend.PickOptions, error) {
	opts := gitbackend.PickOptions{NoEdit: true}
	for id, value := range command.Options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		upstream := ""
		for _, b := range keymap.Registry() {
			if b.Command == id {
				upstream = b.UpstreamCommand
				break
			}
		}
		switch upstream {
		case "magit-cherry-pick:--mainline":
			n, err := strconv.Atoi(value.Value)
			if err != nil || n <= 0 {
				return opts, errors.New("mainline must be a positive integer")
			}
			opts.Mainline = n
		case "magit-merge:--strategy":
			opts.Strategy = value.Value
		case "magit:--signoff":
			opts.Signoff = true
		case "transient:magit-cherry-pick:--edit":
			return opts, errors.New("cherry-pick edit is adapted out: the TUI cannot safely preserve the editor session")
		case "transient:magit-revert:--no-edit":
			opts.NoEdit = true
		default:
			return opts, fmt.Errorf("%s is unavailable: no safe typed history option", upstream)
		}
	}
	return opts, nil
}

func openPickWorkflow(m *Model, revert, noCommit bool, command WorkflowCommand) tea.Cmd {
	opts, err := historyPickOptions(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	opts.NoCommit = noCommit
	name := "cherry-pick"
	if revert {
		name = "revert"
	}
	defaultRevision := selectedHistoryRevision(m)
	return m.LoadWorkflow(name, func(context.Context) (WorkflowDialog, error) {
		title := "Cherry-pick commit"
		if revert {
			title = "Revert commit"
		}
		return WorkflowDialog{Title: title, Operation: name,
			Plan:   []string{"Apply one explicitly resolved commit", "Git editors are disabled"},
			Fields: []WorkflowField{{Name: "revision", Label: "Commit revision", Kind: WorkflowText, Value: defaultRevision, Required: true}},
			Submit: func(ctx context.Context, values WorkflowValues) error {
				if revert {
					return m.repo.RevertStart(ctx, []string{values["revision"]}, opts)
				}
				return m.repo.CherryPickStart(ctx, []string{values["revision"]}, opts)
			}}, nil
	})
}

func reviewedHistoryAction(action gitbackend.HistoryUIAction) WorkflowHandler {
	return func(m *Model, _ WorkflowCommand) tea.Cmd {
		return openReviewedHistoryDialog(m, "Review "+string(action), nil, func(WorkflowValues) gitbackend.HistoryUIRequest { return gitbackend.HistoryUIRequest{Action: action} })
	}
}

func openReviewedHistoryDialog(m *Model, title string, fields []WorkflowField, request func(WorkflowValues) gitbackend.HistoryUIRequest) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: title, Operation: string(request(WorkflowValues{}).Action), Fields: fields,
		ReviewPreflight: func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
			review, err := m.repo.ReviewHistoryUIAction(ctx, request(values))
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := append([]string(nil), review.Plan...)
			plan = append(plan, "HEAD "+review.State.HEAD, "Index "+review.State.Index, "Worktree "+review.State.Worktree, "Operation "+review.State.Operation)
			return WorkflowReview{Plan: plan, Confirmation: "Execute only if the exact reviewed repository state is unchanged", Data: review}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, transported WorkflowReview) error {
			review, ok := transported.Data.(gitbackend.ReviewedHistoryUIAction)
			if !ok {
				return errors.New("invalid reviewed history token")
			}
			return m.repo.ExecuteReviewedHistoryUIAction(ctx, review)
		}})
}

func openRebaseWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	upstream := m.snapshot.summary.Upstream
	if upstream == "" {
		upstream = "HEAD~1"
	}
	fields := []WorkflowField{{Name: "upstream", Label: "Upstream revision", Kind: WorkflowText, Value: upstream, Required: true}, {Name: "onto", Label: "Onto revision (optional)", Kind: WorkflowText}}
	return openReviewedHistoryDialog(m, "Review non-interactive rebase", fields, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseStart, Rebase: gitbackend.RebaseOptions{Upstream: v["upstream"], Onto: v["onto"]}}
	})
}

func openRebaseUpstream(m *Model, c WorkflowCommand) tea.Cmd {
	if m.snapshot.summary.Upstream == "" {
		m.setError(errors.New("rebase onto upstream requires a configured upstream"))
		return nil
	}
	return openReviewedHistoryDialog(m, "Review rebase onto upstream", nil, func(WorkflowValues) gitbackend.HistoryUIRequest {
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseStart, Rebase: gitbackend.RebaseOptions{Upstream: m.snapshot.summary.Upstream}}
	})
}

func openRebasePushRemote(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("push remote", func(ctx context.Context) (WorkflowDialog, error) {
		remote, err := m.repo.PushRemote(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		branch := m.snapshot.summary.Branch
		if branch == "" || m.snapshot.summary.Detached {
			return WorkflowDialog{}, errors.New("rebase onto push remote requires a current local branch")
		}
		target := remote + "/" + branch
		return reviewedHistoryWorkflowDialog(m, "Review rebase onto push remote", gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseStart, Rebase: gitbackend.RebaseOptions{Upstream: target}}), nil
	})
}

func reviewedHistoryWorkflowDialog(m *Model, title string, request gitbackend.HistoryUIRequest) WorkflowDialog {
	return WorkflowDialog{Title: title, Operation: string(request.Action),
		ReviewPreflight: func(ctx context.Context, _ WorkflowValues) (WorkflowReview, error) {
			review, err := m.repo.ReviewHistoryUIAction(ctx, request)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := append([]string(nil), review.Plan...)
			plan = append(plan, "HEAD "+review.State.HEAD, "Index "+review.State.Index, "Worktree "+review.State.Worktree, "Operation "+review.State.Operation)
			return WorkflowReview{Plan: plan, Confirmation: "Execute only if the exact reviewed repository state is unchanged", Data: review}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, transported WorkflowReview) error {
			review, ok := transported.Data.(gitbackend.ReviewedHistoryUIAction)
			if !ok {
				return errors.New("invalid reviewed history token")
			}
			return m.repo.ExecuteReviewedHistoryUIAction(ctx, review)
		}}
}

func openResetWorkflow(m *Model, mode gitbackend.ResetMode, pathMode bool) tea.Cmd {
	fields := []WorkflowField{{Name: "target", Label: "Target revision", Kind: WorkflowText, Value: "HEAD", Required: true}}
	defaultPath := ""
	if row, ok := m.rows[m.tree.Cursor()]; ok {
		defaultPath = row.path
	}
	if pathMode {
		fields = append(fields, WorkflowField{Name: "path", Label: "Repository path", Kind: WorkflowText, Value: defaultPath, Required: true})
	}
	return openReviewedHistoryDialog(m, "Review reset", fields, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		var paths []string
		if pathMode {
			paths = []string{v["path"]}
		}
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIReset, Reset: gitbackend.ResetOptions{Mode: mode, Target: v["target"], Paths: paths}}
	})
}

func openBisectStart(m *Model, _ WorkflowCommand) tea.Cmd {
	fields := []WorkflowField{{Name: "bad", Label: "Known bad revision", Kind: WorkflowText, Value: "HEAD", Required: true}, {Name: "good", Label: "Known good revision", Kind: WorkflowText, Required: true}}
	return openReviewedHistoryDialog(m, "Review bisect start", fields, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIBisectStart, Bisect: gitbackend.BisectStartOptions{Bad: v["bad"], Good: v["good"]}}
	})
}

func openBisectRevision(m *Model, action gitbackend.HistoryUIAction) tea.Cmd {
	return openReviewedHistoryDialog(m, "Review "+string(action), []WorkflowField{{Name: "revision", Label: "Revision (blank means current HEAD)", Kind: WorkflowText}}, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		return gitbackend.HistoryUIRequest{Action: action, Revision: v["revision"]}
	})
}

func openBisectMark(m *Model, _ WorkflowCommand) tea.Cmd {
	fields := []WorkflowField{{Name: "mark", Label: "Mark current revision", Kind: WorkflowEnum, Value: "good", Choices: []WorkflowChoice{{Value: "bad", Label: "Bad"}, {Value: "good", Label: "Good"}}}}
	return openReviewedHistoryDialog(m, "Review bisect mark", fields, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		action := gitbackend.HistoryUIBisectGood
		if v["mark"] == "bad" {
			action = gitbackend.HistoryUIBisectBad
		}
		return gitbackend.HistoryUIRequest{Action: action}
	})
}
