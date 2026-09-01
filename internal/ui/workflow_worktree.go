package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

const (
	commandWorktreeAdd    keymap.CommandID = "worktree.worktree-checkout"
	commandWorktreeBranch keymap.CommandID = "worktree.worktree-branch"
	commandWorktreeMove   keymap.CommandID = "worktree.worktree-move"
	commandWorktreeRemove keymap.CommandID = "worktree.worktree-delete"
	commandWorktreeList   keymap.CommandID = "worktree.worktree-status"
	commandWorktreeLock   keymap.CommandID = "worktree.worktree-lock"
	commandWorktreeUnlock keymap.CommandID = "worktree.worktree-unlock"
	commandWorktreePrune  keymap.CommandID = "worktree.worktree-prune"
)

func init() {
	RegisterWorkflowCapabilities(
		WorkflowCapability{ID: commandWorktreeAdd, Transient: "magit-worktree", UpstreamCommand: "magit-worktree-checkout"},
		WorkflowCapability{ID: commandWorktreeBranch, Transient: "magit-worktree", UpstreamCommand: "magit-worktree-branch"},
		WorkflowCapability{ID: commandWorktreeMove, Transient: "magit-worktree", UpstreamCommand: "magit-worktree-move"},
		WorkflowCapability{ID: commandWorktreeRemove, Transient: "magit-worktree", UpstreamCommand: "magit-worktree-delete"},
		WorkflowCapability{ID: commandWorktreeList, Transient: "magit-worktree", UpstreamCommand: "magit-worktree-status"},
	)
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			commandWorktreeAdd: worktreeAddWorkflow, commandWorktreeBranch: worktreeBranchWorkflow,
			commandWorktreeMove: worktreeMoveWorkflow, commandWorktreeRemove: worktreeRemoveWorkflow,
			commandWorktreeList: worktreeListWorkflow, commandWorktreeLock: worktreeLockWorkflow,
			commandWorktreeUnlock: worktreeUnlockWorkflow, commandWorktreePrune: worktreePruneWorkflow,
		}
	})
}

func worktreeForce(v WorkflowValues) gitbackend.ConfirmedForce {
	if v["force"] == "true" {
		return gitbackend.Confirmed
	}
	return gitbackend.NotConfirmed
}

func worktreeAddWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("worktree add", func(context.Context) (WorkflowDialog, error) {
		return WorkflowDialog{
			Title: "Add worktree", Operation: "add worktree",
			Plan: []string{"Create a worktree at a reviewed destination", "Detached and force are explicit opt-ins"},
			Fields: []WorkflowField{
				{Name: "path", Label: "Destination", Kind: WorkflowText, Required: true},
				{Name: "revision", Label: "Branch or revision", Kind: WorkflowText, Value: "HEAD", Required: true},
				{Name: "detached", Label: "Detached HEAD", Kind: WorkflowBool},
				{Name: "force", Label: "Force", Kind: WorkflowBool},
			},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				opts := gitbackend.WorktreeAddOptions{Detach: v["detached"] == "true", Force: worktreeForce(v)}
				p, err := m.repo.ReviewWorktreeAdd(ctx, v["path"], v["revision"], "", opts)
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: []string{"destination: " + p.Path, "commit: " + p.OID, fmt.Sprintf("detached: %t; force: %t", opts.Detach, opts.Force == gitbackend.Confirmed)}, Confirmation: "Create exactly this worktree?", Data: p}, nil
			},
			SubmitReview: submitWorktreeAddReview(m),
		}, nil
	})
}

func worktreeBranchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("worktree branch", func(context.Context) (WorkflowDialog, error) {
		return WorkflowDialog{
			Title: "Add worktree with new branch", Operation: "add branch worktree",
			Fields: []WorkflowField{
				{Name: "path", Label: "Destination", Kind: WorkflowText, Required: true},
				{Name: "branch", Label: "New branch", Kind: WorkflowText, Required: true},
				{Name: "revision", Label: "Start point", Kind: WorkflowText, Value: "HEAD", Required: true},
				{Name: "force", Label: "Force", Kind: WorkflowBool},
			},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				opts := gitbackend.WorktreeAddOptions{Force: worktreeForce(v)}
				p, err := m.repo.ReviewWorktreeAdd(ctx, v["path"], v["revision"], v["branch"], opts)
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: []string{"destination: " + p.Path, "new branch: " + p.Branch, "start commit: " + p.OID, fmt.Sprintf("force: %t", opts.Force == gitbackend.Confirmed)}, Confirmation: "Create exactly this branch worktree?", Data: p}, nil
			},
			SubmitReview: submitWorktreeAddReview(m),
		}, nil
	})
}

func submitWorktreeAddReview(m *Model) func(context.Context, WorkflowValues, WorkflowReview) error {
	return func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
		p, ok := review.Data.(gitbackend.ReviewedWorktreeAdd)
		if !ok {
			return errors.New("invalid worktree add review")
		}
		return m.repo.AddWorktreeReviewed(ctx, p)
	}
}

func worktreeChoices(ctx context.Context, m *Model, locked *bool) ([]WorkflowChoice, []gitbackend.Worktree, error) {
	all, err := m.repo.Worktrees(ctx)
	if err != nil {
		return nil, nil, err
	}
	choices := make([]WorkflowChoice, 0, len(all))
	eligible := make([]gitbackend.Worktree, 0, len(all))
	for _, wt := range all {
		if wt.Primary || wt.Bare || locked != nil && wt.Locked != *locked {
			continue
		}
		label := wt.Path
		if wt.Branch != "" {
			label += " [" + wt.Branch + "]"
		} else {
			label += " [detached " + wt.HEAD + "]"
		}
		if wt.Locked {
			label += " (locked: " + wt.LockReason + ")"
		}
		choices, eligible = append(choices, WorkflowChoice{Value: wt.Path, Label: label}), append(eligible, wt)
	}
	if len(choices) == 0 {
		return nil, all, errors.New("no eligible linked worktrees")
	}
	return choices, eligible, nil
}

func worktreeSelectField(choices []WorkflowChoice) WorkflowField {
	return WorkflowField{Name: "worktree", Label: "Linked worktree", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices, Required: true}
}

func worktreeMoveWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("worktree move", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := worktreeChoices(ctx, m, nil)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Move worktree", Operation: "move worktree", Fields: []WorkflowField{
			worktreeSelectField(choices), {Name: "destination", Label: "New destination", Kind: WorkflowText, Required: true}, {Name: "force", Label: "Force dirty/locked move", Kind: WorkflowBool},
		}, ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
			p, err := m.repo.ReviewWorktreeMutation(ctx, v["worktree"], v["destination"], worktreeForce(v))
			if err != nil {
				return WorkflowReview{}, err
			}
			return WorkflowReview{Plan: worktreeMutationPlan(p, "move to "+p.Destination), Confirmation: "Move this exact reviewed worktree?", Data: p}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			p, ok := review.Data.(gitbackend.ReviewedWorktreeMutation)
			if !ok {
				return errors.New("invalid worktree move review")
			}
			return m.repo.MoveWorktreeReviewed(ctx, p)
		}}, nil
	})
}

func worktreeMutationPlan(p gitbackend.ReviewedWorktreeMutation, action string) []string {
	return []string{"worktree: " + p.Worktree.Path, "HEAD: " + p.Worktree.HEAD, "action: " + action, fmt.Sprintf("dirty: %t; locked: %t; force: %t", p.Dirty, p.Worktree.Locked, p.Force == gitbackend.Confirmed)}
}

func worktreeRemoveWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("worktree removal", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := worktreeChoices(ctx, m, nil)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Remove worktree", Operation: "remove worktree", Fields: []WorkflowField{worktreeSelectField(choices), {Name: "force", Label: "Force dirty/locked removal", Kind: WorkflowBool}},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				p, err := m.repo.ReviewWorktreeMutation(ctx, v["worktree"], "", worktreeForce(v))
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: worktreeMutationPlan(p, "remove"), Confirmation: "Remove this exact reviewed worktree and directory?", Data: p}, nil
			}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				p, ok := review.Data.(gitbackend.ReviewedWorktreeMutation)
				if !ok {
					return errors.New("invalid worktree removal review")
				}
				return m.repo.RemoveWorktreeReviewed(ctx, p)
			}}, nil
	})
}

func worktreeListWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("worktrees", func(ctx context.Context) (WorkflowDialog, error) {
		all, err := m.repo.Worktrees(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		choices := make([]WorkflowChoice, 0, len(all))
		current := filepath.Clean(m.repo.WorkTree())
		for _, wt := range all {
			kind := "linked"
			if filepath.Clean(wt.Path) == current {
				kind = "current"
			} else if wt.Primary {
				kind = "main"
			}
			state := wt.Branch
			if state == "" {
				state = "detached @ " + shortWorktreeHEAD(wt.HEAD)
			}
			if wt.Locked {
				state += "  🔒 " + worktreeLockLabel(wt.LockReason)
			}
			if wt.Prunable {
				state += "  ⚠ stale: " + wt.PruneReason
			}
			choices = append(choices, WorkflowChoice{Value: wt.Path, Label: "[" + kind + "] " + state + "  •  " + wt.Path})
		}
		if len(choices) == 0 {
			return WorkflowDialog{}, errors.New("no worktrees available")
		}
		return WorkflowDialog{
			Title: "Worktrees — separate checkouts", ActionLabel: "Close",
			Help: []string{
				"Each worktree is another folder checked out from this repository.",
				"[current] is this UI; [main] is the original checkout; [linked] is an additional checkout.",
				"From the Worktree menu: b add, c add with branch, m move, k remove, L lock, U unlock.",
			},
			Fields: []WorkflowField{{Name: "worktree", Label: "Filter worktrees", Kind: WorkflowSearch, Value: choices[0].Value, Choices: choices, Required: true}},
			Run:    func(WorkflowValues) tea.Cmd { return nil },
		}, nil
	})
}

func shortWorktreeHEAD(head string) string {
	if len(head) > 8 {
		return head[:8]
	}
	return head
}

func worktreeLockLabel(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "locked"
	}
	return reason
}

func worktreeLockWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	locked := false
	return m.LoadWorkflow("worktree lock", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := worktreeChoices(ctx, m, &locked)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Lock worktree", Operation: "lock worktree", Fields: []WorkflowField{worktreeSelectField(choices), {Name: "reason", Label: "Reason", Kind: WorkflowText}}, Submit: func(ctx context.Context, v WorkflowValues) error {
			return m.repo.LockWorktree(ctx, v["worktree"], v["reason"])
		}}, nil
	})
}

func worktreeUnlockWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	locked := true
	return m.LoadWorkflow("worktree unlock", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := worktreeChoices(ctx, m, &locked)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Unlock worktree", Operation: "unlock worktree", Fields: []WorkflowField{worktreeSelectField(choices)}, Submit: func(ctx context.Context, v WorkflowValues) error { return m.repo.UnlockWorktree(ctx, v["worktree"]) }}, nil
	})
}

func worktreePruneWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "Prune stale worktree records", Operation: "prune worktrees", Fields: []WorkflowField{{Name: "expire", Label: "Expiration", Kind: WorkflowText, Value: "now", Required: true}},
		ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
			p, err := m.repo.ReviewWorktreePrune(ctx, v["expire"])
			if err != nil {
				return WorkflowReview{}, err
			}
			lines := []string{"expiration: " + p.Expire}
			output := strings.TrimSpace(p.Output)
			if output == "" {
				lines = append(lines, "No stale worktree records")
			} else {
				lines = append(lines, strings.Split(output, "\n")...)
			}
			return WorkflowReview{Plan: lines, Confirmation: "Prune exactly this reviewed stale set?", Data: p}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			p, ok := review.Data.(gitbackend.ReviewedWorktreePrune)
			if !ok {
				return errors.New("invalid worktree prune review")
			}
			return m.repo.PruneWorktreesReviewed(ctx, p)
		}})
}
