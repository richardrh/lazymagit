package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const pullMergeChoiceLimit = 512

// These are the stable domain identities generated from Magit's command names.
// Registration, rather than edits to Model dispatch, remains the only UI hook.
const (
	pullPushRemoteID   keymap.CommandID = "pull.pull-from-pushremote"
	pullUpstreamID     keymap.CommandID = "pull.pull-from-upstream"
	pullElsewhereID    keymap.CommandID = "pull.pull-branch"
	pullFetchAllID     keymap.CommandID = "pull.fetch-all-no-prune"
	pullFetchPruneID   keymap.CommandID = "pull.fetch-all-prune"
	pullFetchBranchID  keymap.CommandID = "pull.fetch-branch"
	pullFetchRefspecID keymap.CommandID = "pull.fetch-refspec"
	pullFetchModulesID keymap.CommandID = "pull.fetch-modules"
	pullConfigureID    keymap.CommandID = "pull.branch-configure"

	mergePlainID    keymap.CommandID = "merge.merge-plain"
	mergeNoCommitID keymap.CommandID = "merge.merge-nocommit"
	mergePreviewID  keymap.CommandID = "merge.merge-preview"
	mergeSquashID   keymap.CommandID = "merge.merge-squash"
	mergeContinueID keymap.CommandID = "merge.commit-create"
	mergeAbortID    keymap.CommandID = "merge.merge-abort"
)

func init() {
	pullOptions := []string{"transient:magit-pull:--ff-only", "magit-pull:--rebase", "transient:magit-pull:--autostash", "transient:magit-pull:--force"}
	mergeOptions := []string{"transient:magit-merge:--ff-only", "transient:magit-merge:--no-ff"}
	RegisterWorkflowCapabilities(
		WorkflowCapability{ID: pullPushRemoteID, Transient: "magit-pull", UpstreamCommand: "magit-pull-from-pushremote", Consumes: pullOptions},
		WorkflowCapability{ID: pullUpstreamID, Transient: "magit-pull", UpstreamCommand: "magit-pull-from-upstream", Consumes: pullOptions},
		WorkflowCapability{ID: pullElsewhereID, Transient: "magit-pull", UpstreamCommand: "magit-pull-branch", Consumes: pullOptions},
		WorkflowCapability{ID: pullFetchAllID, Transient: "magit-pull", UpstreamCommand: "magit-fetch-all-no-prune"},
		WorkflowCapability{ID: pullFetchPruneID, Transient: "magit-pull", UpstreamCommand: "magit-fetch-all-prune"},
		WorkflowCapability{ID: pullFetchBranchID, Transient: "magit-pull", UpstreamCommand: "magit-fetch-branch"},
		WorkflowCapability{ID: pullFetchRefspecID, Transient: "magit-pull", UpstreamCommand: "magit-fetch-refspec"},
		WorkflowCapability{ID: pullFetchModulesID, Transient: "magit-pull", UpstreamCommand: "magit-fetch-modules"},
		WorkflowCapability{ID: pullConfigureID, Transient: "magit-pull", UpstreamCommand: "magit-branch-configure"},
		WorkflowCapability{ID: mergePlainID, Transient: "magit-merge", UpstreamCommand: "magit-merge-plain", Consumes: mergeOptions},
		WorkflowCapability{ID: mergeNoCommitID, Transient: "magit-merge", UpstreamCommand: "magit-merge-nocommit", Consumes: mergeOptions},
		WorkflowCapability{ID: mergePreviewID, Transient: "magit-merge", UpstreamCommand: "magit-merge-preview", Consumes: mergeOptions},
		WorkflowCapability{ID: mergeSquashID, Transient: "magit-merge", UpstreamCommand: "magit-merge-squash", Consumes: mergeOptions},
		WorkflowCapability{ID: mergeContinueID, Transient: "magit-merge", UpstreamCommand: "magit-commit-create"},
		WorkflowCapability{ID: mergeAbortID, Transient: "magit-merge", UpstreamCommand: "magit-merge-abort"},
	)
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			pullPushRemoteID: pullWorkflow(gitbackend.PullPushRemote),
			pullUpstreamID:   pullWorkflow(gitbackend.PullUpstream),
			pullElsewhereID:  pullWorkflow(gitbackend.PullRemoteBranch),
			pullFetchAllID:   pullFetchAll(false), pullFetchPruneID: pullFetchAll(true),
			pullFetchBranchID: fetchBranchWorkflow, pullFetchRefspecID: fetchRefspecWorkflow,
			pullFetchModulesID: fetchModulesWorkflow, pullConfigureID: branchConfigure,
			mergePlainID: mergeWorkflow(false, false), mergeNoCommitID: mergeWorkflow(true, false),
			mergePreviewID: mergeWorkflow(false, true), mergeSquashID: mergeSquashWorkflow,
			mergeContinueID: mergeContinueWorkflow, mergeAbortID: mergeAbortWorkflow,
		}
	})
}

func pullWorkflow(target gitbackend.PullTarget) WorkflowHandler {
	return func(m *Model, command WorkflowCommand) tea.Cmd {
		args, err := pullArgs(command.Options)
		if err != nil {
			m.setError(err)
			return nil
		}
		args.Target = target
		if target != gitbackend.PullRemoteBranch {
			return m.StartWorkflowOperation("pull", func(ctx context.Context) error { return m.repo.PullWithArgs(ctx, args) })
		}
		return m.LoadWorkflow("pull elsewhere", func(ctx context.Context) (WorkflowDialog, error) {
			choices, err := m.repo.FetchChoices(ctx, pullMergeChoiceLimit)
			if err != nil {
				return WorkflowDialog{}, err
			}
			field, err := branchChoiceField(choices.RemoteBranches)
			if err != nil {
				return WorkflowDialog{}, err
			}
			return WorkflowDialog{Title: "Pull elsewhere", Operation: "pull", Fields: []WorkflowField{field}, Submit: func(ctx context.Context, values WorkflowValues) error {
				selected, err := selectedRemoteBranch(values, choices.RemoteBranches)
				if err != nil {
					return err
				}
				selectedArgs := args
				selectedArgs.Remote, selectedArgs.Branch = selected.Remote, selected.Branch
				return m.repo.PullWithArgs(ctx, selectedArgs)
			}}, nil
		})
	}
}

func pullArgs(options map[keymap.CommandID]OptionValue) (gitbackend.PullArgs, error) {
	enabled := func(suffix string) bool {
		for id, value := range options {
			if strings.HasSuffix(string(id), suffix) && (value.Enabled || value.Value != "" && value.Value != "false") {
				return true
			}
		}
		return false
	}
	ff, rebase := enabled("--ff-only"), enabled("--rebase")
	if ff && rebase {
		return gitbackend.PullArgs{}, errors.New("fast-forward-only and rebase pull modes are mutually exclusive")
	}
	args := gitbackend.PullArgs{Mode: gitbackend.PullMerge, Autostash: enabled("--autostash"), Force: enabled("--force")}
	if ff {
		args.Mode = gitbackend.PullFFOnly
	} else if rebase {
		args.Mode = gitbackend.PullRebase
	}
	return args, nil
}

func pullFetchAll(prune bool) WorkflowHandler {
	return func(m *Model, _ WorkflowCommand) tea.Cmd {
		return m.StartWorkflowOperation("fetch all remotes", func(ctx context.Context) error {
			return m.repo.FetchAllWithArgs(ctx, gitbackend.FetchArgs{Prune: prune})
		})
	}
}

func mergeWorkflow(noCommit, preview bool) WorkflowHandler {
	return func(m *Model, command WorkflowCommand) tea.Cmd {
		return m.LoadWorkflow("merge", func(ctx context.Context) (WorkflowDialog, error) {
			state, err := m.repo.QueryOperationState(ctx)
			if err != nil {
				return WorkflowDialog{}, err
			}
			if mergeOperationActive(state) {
				if noCommit || preview {
					return WorkflowDialog{}, errors.New("a merge is already in progress")
				}
				return continueMergeDialog(m.repo), nil
			}
			args, err := mergeArgs(command.Options)
			if err != nil {
				return WorkflowDialog{}, err
			}
			args.NoCommit = noCommit
			return loadMergeTargetDialog(ctx, m.repo, args, preview)
		})
	}
}

func mergeSquashWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("squash merge", func(ctx context.Context) (WorkflowDialog, error) {
		args, err := mergeArgs(command.Options)
		if err != nil {
			return WorkflowDialog{}, err
		}
		args.Squash = true
		return loadMergeTargetDialog(ctx, m.repo, args, false)
	})
}

func mergeArgs(options map[keymap.CommandID]OptionValue) (gitbackend.MergeArgs, error) {
	has := func(suffix string) bool {
		for id, value := range options {
			if strings.HasSuffix(string(id), suffix) && (value.Enabled || value.Value != "") {
				return true
			}
		}
		return false
	}
	ff, noFF := has("--ff-only"), has("--no-ff")
	if ff && noFF {
		return gitbackend.MergeArgs{}, errors.New("fast-forward-only and no-fast-forward are mutually exclusive")
	}
	// MergeWithArgs intentionally has no strategy/signing contract. Keeping these
	// options unavailable is safer than silently dropping an argv-affecting infix.
	for id, value := range options {
		name := string(id)
		if (strings.Contains(name, "strategy") || strings.Contains(name, "ignore-space") || strings.Contains(name, "diff-algorithm") || strings.Contains(name, "gpg-sign") || strings.Contains(name, "signoff")) && (value.Enabled || value.Value != "") {
			return gitbackend.MergeArgs{}, fmt.Errorf("%s is unsupported by the typed merge backend", id)
		}
	}
	args := gitbackend.MergeArgs{Mode: gitbackend.MergePlain}
	if ff {
		args.Mode = gitbackend.MergeFFOnly
	} else if noFF {
		args.Mode = gitbackend.MergeNoFF
	}
	return args, nil
}

func loadMergeTargetDialog(ctx context.Context, repo *gitbackend.Repository, args gitbackend.MergeArgs, preview bool) (WorkflowDialog, error) {
	refs, err := repo.QueryRefs(ctx, gitbackend.RefQuery{Limit: pullMergeChoiceLimit})
	if err != nil {
		return WorkflowDialog{}, err
	}
	var choices []WorkflowChoice
	add := func(refs []gitbackend.Ref) {
		for _, ref := range refs {
			if !ref.Current && ref.Symref == "" {
				choices = append(choices, WorkflowChoice{Value: ref.FullName, Label: ref.Name})
			}
		}
	}
	add(refs.Local)
	add(refs.Remote)
	add(refs.Tags)
	if len(choices) == 0 {
		return WorkflowDialog{}, errors.New("no merge targets are available")
	}
	d := WorkflowDialog{Title: "Merge", Operation: "merge", Fields: []WorkflowField{selectField("target", "Target", choices)}}
	if preview {
		d.Title, d.Operation = "Preview merge", "preview merge"
		d.ReviewPreflight = func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
			p, err := repo.MergePreflight(ctx, values["target"])
			if err != nil {
				return WorkflowReview{}, err
			}
			return WorkflowReview{Plan: mergePlan(p, args), Confirmation: "Close preview without changing the repository", Data: p}, nil
		}
		d.SubmitReview = func(context.Context, WorkflowValues, WorkflowReview) error { return nil }
		return d, nil
	}
	d.ReviewPreflight = func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
		reviewArgs := args
		reviewArgs.Target, reviewArgs.ConfirmDirty = values["target"], true
		reviewed, err := repo.ReviewMerge(ctx, reviewArgs)
		if err != nil {
			return WorkflowReview{}, err
		}
		return WorkflowReview{Plan: mergePlan(reviewed.Preflight, reviewArgs), Confirmation: "Confirm reviewed merge", Data: reviewed}, nil
	}
	d.SubmitReview = func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
		reviewed, ok := review.Data.(gitbackend.ReviewedMerge)
		if !ok {
			return errors.New("merge review is invalid")
		}
		_, err := repo.ExecuteReviewedMerge(ctx, reviewed)
		return err
	}
	return d, nil
}

func mergePlan(p gitbackend.MergePreflight, args gitbackend.MergeArgs) []string {
	plan := []string{"target: " + p.Target, "resolved target: " + p.TargetOID}
	if p.AlreadyUpToDate {
		plan = append(plan, "Already up to date")
	}
	if p.FastForwardPossible {
		plan = append(plan, "Fast-forward is possible")
	}
	if args.Mode == gitbackend.MergeNoFF {
		plan = append(plan, "Create a merge commit even when fast-forward is possible")
	}
	if args.Mode == gitbackend.MergeFFOnly {
		plan = append(plan, "Refuse unless fast-forward is possible")
	}
	if args.NoCommit {
		plan = append(plan, "Stop before creating the merge commit")
	}
	if args.Squash {
		plan = append(plan, "Squash changes without creating a merge commit")
	}
	if p.State.Dirty {
		plan = append(plan, "WARNING: merge into a dirty worktree")
	}
	return plan
}

func mergeContinueWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("continue merge", func(ctx context.Context) (WorkflowDialog, error) {
		state, err := m.repo.QueryOperationState(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		if !mergeOperationActive(state) {
			return WorkflowDialog{}, errors.New("no merge is in progress")
		}
		return continueMergeDialog(m.repo), nil
	})
}

func continueMergeDialog(repo *gitbackend.Repository) WorkflowDialog {
	return WorkflowDialog{Title: "Continue merge", Operation: "continue merge", ReviewPreflight: func(ctx context.Context, _ WorkflowValues) (WorkflowReview, error) {
		reviewed, err := repo.ReviewMergeContinue(ctx)
		if err != nil {
			return WorkflowReview{}, err
		}
		return WorkflowReview{Plan: []string{
			"Commit the resolved merge using the existing message",
			"Merge heads: " + strings.Join(reviewed.MergeHeads, ", "),
			"Prepared index tree: " + reviewed.IndexTree,
		}, Confirmation: "Press Enter again only if the reviewed resolution is unchanged", Data: reviewed}, nil
	}, SubmitReview: func(ctx context.Context, _ WorkflowValues, transported WorkflowReview) error {
		reviewed, ok := transported.Data.(gitbackend.ReviewedMergeContinue)
		if !ok {
			return errors.New("merge continue review is invalid")
		}
		return repo.ExecuteReviewedMergeContinue(ctx, reviewed)
	}}
}

func mergeAbortWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("abort merge", func(ctx context.Context) (WorkflowDialog, error) {
		reviewed, err := m.repo.ReviewMergeAbort(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Abort merge", Operation: "abort merge", ReviewPreflight: func(context.Context, WorkflowValues) (WorkflowReview, error) {
			return WorkflowReview{Plan: []string{"Restore HEAD and worktree to their pre-merge state", "merge heads: " + strings.Join(reviewed.MergeHeads, ", ")}, Confirmation: "DANGER: confirm destructive merge abort", Data: reviewed}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			approved, ok := review.Data.(gitbackend.ReviewedMergeAbort)
			if !ok {
				return errors.New("merge abort review is invalid")
			}
			return m.repo.ExecuteReviewedMergeAbort(ctx, approved)
		}}, nil
	})
}

func mergeOperationActive(state gitbackend.OperationState) bool {
	for _, operation := range state.Items {
		if operation.Kind == gitbackend.OperationMerge {
			return true
		}
	}
	return false
}
