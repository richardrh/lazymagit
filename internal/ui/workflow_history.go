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
	rebaseOptions := []string{"transient:magit-rebase:--keep-empty", "transient:magit-rebase:--rebase-merges=", "transient:magit-rebase:--update-refs", "transient:magit-rebase:--autostash", "transient:magit-rebase:--force-rebase", "magit-merge:--strategy", "magit:--signoff"}
	bisectOptions := []string{"transient:magit-bisect:--no-checkout", "transient:magit-bisect:--first-parent"}
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-cherry-pick", map[string][]string{
		"magit-cherry-copy":  pickOptions,
		"magit-cherry-apply": pickOptions,
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-revert", map[string][]string{
		"magit-revert-and-commit": {"magit-cherry-pick:--mainline", "transient:magit-revert:--no-edit", "magit-merge:--strategy", "magit:--signoff"},
		"magit-revert-no-commit":  {"magit-cherry-pick:--mainline", "transient:magit-revert:--no-edit", "magit-merge:--strategy", "magit:--signoff"},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-rebase", map[string][]string{
		"magit-rebase-branch":          rebaseOptions,
		"magit-rebase-subset":          rebaseOptions,
		"magit-rebase-onto-upstream":   rebaseOptions,
		"magit-rebase-onto-pushremote": rebaseOptions,
		"magit-rebase-interactive":     rebaseOptions,
		"magit-rebase-edit-commit":     rebaseOptions,
		"magit-rebase-reword-commit":   rebaseOptions,
		"magit-rebase-remove-commit":   rebaseOptions,
		"magit-rebase-edit":            nil,
		"magit-rebase-continue":        nil,
		"magit-rebase-skip":            nil,
		"magit-rebase-abort":           nil,
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-bisect", map[string][]string{
		"magit-bisect-start": bisectOptions,
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
			return reviewedHistoryAction(gitbackend.HistoryUICherryContinue)
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
			return reviewedHistoryAction(gitbackend.HistoryUIRevertContinue)
		case "magit-sequencer-skip":
			return reviewedHistoryAction(gitbackend.HistoryUIRevertSkip)
		case "magit-sequencer-abort":
			return reviewedHistoryAction(gitbackend.HistoryUIRevertAbort)
		}
	case "magit-rebase":
		switch upstream {
		case "magit-rebase-branch", "magit-rebase-subset":
			return openRebaseWorkflow
		case "magit-rebase-interactive", "magit-rebase-edit-commit", "magit-rebase-reword-commit", "magit-rebase-remove-commit":
			return openInteractiveRebaseWorkflow
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
		case "magit-rebase-edit":
			return openRebaseTodoWorkflow
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

func selectedHistoryRevision(m *Model) string {
	if current, ok := m.rows[m.tree.Cursor()]; ok && current.kind == rowCommit && current.commit.ID != "" {
		return current.commit.ID
	}
	return ""
}

func historyOptionUpstream(id keymap.CommandID) string {
	for _, binding := range keymap.Registry() {
		if binding.Command == id {
			return binding.UpstreamCommand
		}
	}
	return ""
}

func historyPickOptions(command WorkflowCommand) (gitbackend.PickOptions, error) {
	opts := gitbackend.PickOptions{NoEdit: true}
	for id, value := range command.Options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		upstream := historyOptionUpstream(id)
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
	defaultRevision := selectedHistoryRevision(m)
	title := "Review cherry-pick"
	action := gitbackend.HistoryUICherryStart
	if revert {
		title, action = "Review revert", gitbackend.HistoryUIRevertStart
	}
	return openReviewedHistoryDialog(m, title, []WorkflowField{{Name: "revision", Label: "Commit revision", Kind: WorkflowText, Value: defaultRevision, Required: true}}, func(values WorkflowValues) gitbackend.HistoryUIRequest {
		return gitbackend.HistoryUIRequest{Action: action, Pick: opts, Revisions: []string{values["revision"]}}
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

func rebaseOptions(command WorkflowCommand) (gitbackend.RebaseOptions, error) {
	var opts gitbackend.RebaseOptions
	for id, value := range command.Options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		switch historyOptionUpstream(id) {
		case "transient:magit-rebase:--keep-empty":
			opts.KeepEmpty = true
		case "transient:magit-rebase:--rebase-merges=":
			opts.RebaseMerges = true
		case "transient:magit-rebase:--update-refs":
			opts.UpdateRefs = true
		case "transient:magit-rebase:--autostash":
			opts.Autostash = true
		case "transient:magit-rebase:--force-rebase":
			opts.ForceRebase = true
		case "transient:magit-rebase:--interactive":
			// The suffix opens the bounded terminal todo editor; this infix is
			// represented by choosing that suffix, not passed through as argv.
		case "magit-merge:--strategy":
			opts.Strategy = value.Value
		case "magit:--signoff":
			opts.Signoff = true
		default:
			return opts, fmt.Errorf("%s is unavailable: no safe typed rebase option", historyOptionUpstream(id))
		}
	}
	return opts, nil
}

func openInteractiveRebaseWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	opts, err := rebaseOptions(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	upstream := m.snapshot.summary.Upstream
	if upstream == "" {
		upstream = "HEAD~1"
	}
	return m.LoadWorkflow("interactive rebase todo", func(ctx context.Context) (WorkflowDialog, error) {
		todo, err := m.repo.DefaultRebaseTodo(ctx, upstream)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return reviewedInteractiveRebaseDialog(m, "Review interactive rebase todo", []WorkflowField{
			{Name: "upstream", Label: "Upstream revision", Kind: WorkflowText, Value: upstream, Required: true},
			{Name: "onto", Label: "Onto revision (optional)", Kind: WorkflowText},
			{Name: "todo", Label: "Todo", Kind: WorkflowMultiline, Value: todo, Required: true},
		}, func(values WorkflowValues) gitbackend.HistoryUIRequest {
			rebase := opts
			rebase.Upstream, rebase.Onto, rebase.Todo = values["upstream"], values["onto"], values["todo"]
			return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseInteractive, Rebase: rebase}
		}), nil
	})
}

func openRebaseTodoWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("active rebase todo", func(ctx context.Context) (WorkflowDialog, error) {
		todo, err := m.repo.ReadRebaseTodo(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return reviewedInteractiveRebaseDialog(m, "Review active rebase todo", []WorkflowField{{Name: "todo", Label: "Todo", Kind: WorkflowMultiline, Value: todo, Required: true}}, func(values WorkflowValues) gitbackend.HistoryUIRequest {
			return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseTodo, Rebase: gitbackend.RebaseOptions{Todo: values["todo"]}}
		}), nil
	})
}

func reviewedInteractiveRebaseDialog(m *Model, title string, fields []WorkflowField, request func(WorkflowValues) gitbackend.HistoryUIRequest) WorkflowDialog {
	return WorkflowDialog{Title: title, Operation: "interactive rebase", Fields: fields,
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
		}}
}

func openRebaseWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	opts, err := rebaseOptions(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	upstream := m.snapshot.summary.Upstream
	if upstream == "" {
		upstream = "HEAD~1"
	}
	fields := []WorkflowField{{Name: "upstream", Label: "Upstream revision", Kind: WorkflowText, Value: upstream, Required: true}, {Name: "onto", Label: "Onto revision (optional)", Kind: WorkflowText}}
	return openReviewedHistoryDialog(m, "Review non-interactive rebase", fields, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		requestOptions := opts
		requestOptions.Upstream, requestOptions.Onto = v["upstream"], v["onto"]
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseStart, Rebase: requestOptions}
	})
}

func openRebaseUpstream(m *Model, command WorkflowCommand) tea.Cmd {
	opts, err := rebaseOptions(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	if m.snapshot.summary.Upstream == "" {
		m.setError(errors.New("rebase onto upstream requires a configured upstream"))
		return nil
	}
	return openReviewedHistoryDialog(m, "Review rebase onto upstream", nil, func(WorkflowValues) gitbackend.HistoryUIRequest {
		opts.Upstream = m.snapshot.summary.Upstream
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseStart, Rebase: opts}
	})
}

func openRebasePushRemote(m *Model, command WorkflowCommand) tea.Cmd {
	opts, err := rebaseOptions(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.LoadWorkflow("push remote", func(ctx context.Context) (WorkflowDialog, error) {
		remote, err := m.repo.PushRemote(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		branch := m.snapshot.summary.Branch
		if branch == "" || m.snapshot.summary.Detached {
			return WorkflowDialog{}, errors.New("rebase onto push remote requires a current local branch")
		}
		opts.Upstream = remote + "/" + branch
		return reviewedHistoryWorkflowDialog(m, "Review rebase onto push remote", gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIRebaseStart, Rebase: opts}), nil
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

func bisectOptions(command WorkflowCommand) (gitbackend.BisectStartOptions, error) {
	var opts gitbackend.BisectStartOptions
	for id, value := range command.Options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		switch historyOptionUpstream(id) {
		case "transient:magit-bisect:--no-checkout":
			opts.NoCheckout = true
		case "transient:magit-bisect:--first-parent":
			opts.FirstParent = true
		default:
			return opts, fmt.Errorf("%s is unavailable: no safe typed bisect option", historyOptionUpstream(id))
		}
	}
	return opts, nil
}

func openBisectStart(m *Model, command WorkflowCommand) tea.Cmd {
	opts, err := bisectOptions(command)
	if err != nil {
		m.setError(err)
		return nil
	}
	fields := []WorkflowField{{Name: "bad", Label: "Known bad revision", Kind: WorkflowText, Value: "HEAD", Required: true}, {Name: "good", Label: "Known good revision", Kind: WorkflowText, Required: true}}
	return openReviewedHistoryDialog(m, "Review bisect start", fields, func(v WorkflowValues) gitbackend.HistoryUIRequest {
		opts.Bad, opts.Good = v["bad"], v["good"]
		return gitbackend.HistoryUIRequest{Action: gitbackend.HistoryUIBisectStart, Bisect: opts}
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
