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

const (
	stashBothID        keymap.CommandID = "stash.stash-both"
	stashKeepIndexID   keymap.CommandID = "stash.stash-keep-index"
	stashPushID        keymap.CommandID = "stash-push.stash-push"
	stashApplyID       keymap.CommandID = "stash.stash-apply"
	stashListID        keymap.CommandID = "stash.stash-list"
	stashBranchID      keymap.CommandID = "stash.stash-branch"
	stashFormatPatchID keymap.CommandID = "stash.stash-format-patch"

	stashPathLimit  = 4096
	stashInputLimit = 64 << 10
	stashPathCount  = 256
)

func init() {
	topOptions := []string{"transient:magit-stash:--include-untracked", "transient:magit-stash:--all"}
	pushOptions := []string{"magit:--", "transient:magit-stash-push:--include-untracked", "transient:magit-stash-push:--all", "transient:magit-stash-push:--keep-index", "transient:magit-stash-push:--no-keep-index"}
	capabilities := []WorkflowCapability{
		WorkflowCapability{ID: stashBothID, Transient: "magit-stash", UpstreamCommand: "magit-stash-both", Consumes: topOptions},
		WorkflowCapability{ID: stashKeepIndexID, Transient: "magit-stash", UpstreamCommand: "magit-stash-keep-index", Consumes: topOptions},
		WorkflowCapability{ID: stashPushID, Transient: "magit-stash-push", UpstreamCommand: "magit-stash-push", Consumes: pushOptions},
		WorkflowCapability{ID: stashApplyID, Transient: "magit-stash", UpstreamCommand: "magit-stash-apply"},
		WorkflowCapability{ID: stashListID, Transient: "magit-stash", UpstreamCommand: "magit-stash-list"},
		WorkflowCapability{ID: stashBranchID, Transient: "magit-stash", UpstreamCommand: "magit-stash-branch"},
		WorkflowCapability{ID: stashFormatPatchID, Transient: "magit-stash", UpstreamCommand: "magit-stash-format-patch"},
	}
	seenShow := map[keymap.CommandID]bool{}
	for _, binding := range keymap.Registry() {
		if binding.UpstreamCommand == "magit-stash-show" && !seenShow[binding.Command] {
			seenShow[binding.Command] = true
			capabilities = append(capabilities, WorkflowCapability{ID: binding.Command, Transient: binding.Transient, UpstreamCommand: binding.UpstreamCommand})
		}
	}
	RegisterWorkflowCapabilities(capabilities...)
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		handlers := map[keymap.CommandID]WorkflowHandler{
			stashBothID: stashPushWorkflow(false), stashKeepIndexID: stashPushWorkflow(true), stashPushID: stashPushWorkflow(false),
			stashApplyID: stashApplyWorkflow,
			stashListID:  stashListWorkflow, stashBranchID: stashBranchWorkflow, stashFormatPatchID: stashFormatPatchWorkflow,
		}
		for _, binding := range keymap.Registry() {
			if binding.UpstreamCommand == "magit-stash-show" {
				handlers[binding.Command] = stashShowWorkflow
			}
		}
		return handlers
	})
}

func stashPushWorkflow(forceKeepIndex bool) WorkflowHandler {
	return func(m *Model, command WorkflowCommand) tea.Cmd {
		options, err := stashPushOptions(command, forceKeepIndex)
		if err != nil {
			m.setError(err)
			return nil
		}
		return m.OpenWorkflow(WorkflowDialog{
			Title: "Stash changes", Operation: "stash changes", Confirmation: "Create this stash",
			Fields: []WorkflowField{{Name: "message", Label: "Message (optional)", Kind: WorkflowText}},
			Validate: func(v WorkflowValues) error {
				if v["message"] == "" {
					return nil
				}
				return validBoundedText("stash message", v["message"])
			},
			Submit: func(ctx context.Context, v WorkflowValues) error {
				options.Message = v["message"]
				return m.repo.StashPush(ctx, options)
			},
		})
	}
}

func stashPushOptions(command WorkflowCommand, forceKeepIndex bool) (gitbackend.StashPushOptions, error) {
	options := gitbackend.StashPushOptions{KeepIndex: forceKeepIndex}
	keep, noKeep := false, false
	wantedTransient := "magit-stash"
	if command.ID == stashPushID {
		wantedTransient = "magit-stash-push"
	}
	for id, value := range command.Options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		transient, upstream := stashOptionIdentity(id)
		if transient != wantedTransient {
			continue
		}
		switch upstream {
		case "transient:magit-stash:--include-untracked", "transient:magit-stash-push:--include-untracked":
			options.IncludeUntracked = value.Enabled
		case "transient:magit-stash:--all", "transient:magit-stash-push:--all":
			options.All = value.Enabled
		case "transient:magit-stash-push:--keep-index":
			keep = value.Enabled
		case "transient:magit-stash-push:--no-keep-index":
			noKeep = value.Enabled
		case "magit:--":
			paths, err := stashPaths(value.Value)
			if err != nil {
				return options, err
			}
			options.Paths = paths
		}
	}
	if keep && noKeep {
		return options, errors.New("keep-index and no-keep-index are mutually exclusive")
	}
	if keep {
		options.KeepIndex = true
	}
	if noKeep && forceKeepIndex {
		return options, errors.New("no-keep-index is incompatible with keep-index stash")
	}
	return options, nil
}

func stashOptionIdentity(id keymap.CommandID) (string, string) {
	for _, binding := range keymap.Registry() {
		if binding.Command == id && binding.Kind == keymap.KindInfix {
			return binding.Transient, binding.UpstreamCommand
		}
	}
	return "", ""
}

func stashPaths(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > stashInputLimit {
		return nil, errors.New("stash path input exceeds 64 KiB")
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, path := range lines {
		if path == "" {
			continue
		}
		if len(path) > stashPathLimit || strings.ContainsAny(path, "\x00\r") {
			return nil, fmt.Errorf("stash path is invalid or exceeds %d bytes", stashPathLimit)
		}
		paths = append(paths, path)
		if len(paths) > stashPathCount {
			return nil, fmt.Errorf("stash path list exceeds %d entries", stashPathCount)
		}
	}
	return paths, nil
}

func stashChoices(ctx context.Context, m *Model) ([]WorkflowChoice, error) {
	stashes, err := m.repo.Stashes(ctx)
	if err != nil {
		return nil, err
	}
	if len(stashes) == 0 {
		return nil, errors.New("no stashes")
	}
	choices := make([]WorkflowChoice, len(stashes))
	for i, stash := range stashes {
		choices[i] = WorkflowChoice{Value: stash.ID, Label: stash.Ref + "  " + stash.ShortID + "  " + stash.Subject}
	}
	return choices, nil
}

func loadStashDialog(m *Model, name string, build func([]WorkflowChoice) WorkflowDialog) tea.Cmd {
	return m.LoadWorkflow(name, func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := stashChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return build(choices), nil
	})
}

func stashSelect(choices []WorkflowChoice) WorkflowField {
	return WorkflowField{Name: "stash", Label: "Stash", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices, Required: true}
}

func stashApplyWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadStashDialog(m, "stash apply", func(choices []WorkflowChoice) WorkflowDialog {
		return WorkflowDialog{Title: "Apply stash", Operation: "apply stash", Fields: []WorkflowField{stashSelect(choices), {Name: "index", Label: "Restore index state", Kind: WorkflowBool}}, ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
			reviewed, err := m.repo.ReviewStash(ctx, v["stash"])
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := reviewedStashApply{stash: reviewed, index: v["index"] == "true"}
			lines := []string{reviewed.Stash.Ref + "  " + reviewed.Stash.ID, reviewed.Stash.Subject, "stash entry will be retained"}
			if plan.index {
				lines = append(lines, "restore index state")
			}
			return WorkflowReview{Plan: lines, Confirmation: "Apply this exact stash and retain its entry?", Data: plan}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
			plan, ok := wr.Data.(reviewedStashApply)
			if !ok {
				return errors.New("invalid stash apply review")
			}
			return m.repo.ApplyReviewedStash(ctx, plan.stash, gitbackend.StashApplyOptions{Index: plan.index})
		}}
	})
}

type reviewedStashApply struct {
	stash gitbackend.ReviewedStash
	index bool
}

type reviewedStashBranch struct {
	stash  gitbackend.ReviewedStash
	branch string
}

func stashBranchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadStashDialog(m, "stash branch", func(choices []WorkflowChoice) WorkflowDialog {
		return WorkflowDialog{Title: "Branch from stash", Operation: "branch from stash", Fields: []WorkflowField{stashSelect(choices), {Name: "branch", Label: "New branch", Kind: WorkflowText, Required: true}}, Validate: func(v WorkflowValues) error {
			return validBoundedText("branch name", strings.TrimSpace(v["branch"]))
		}, ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
			branch := strings.TrimSpace(v["branch"])
			reviewed, err := m.repo.ReviewStash(ctx, v["stash"])
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := reviewedStashBranch{stash: reviewed, branch: branch}
			return WorkflowReview{Plan: []string{"branch: " + branch, reviewed.Stash.Ref + "  " + reviewed.Stash.ID, reviewed.Stash.Subject, "stash entry will be retained"}, Confirmation: "Create this branch from the exact reviewed stash and retain its entry?", Data: plan}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
			plan, ok := wr.Data.(reviewedStashBranch)
			if !ok {
				return errors.New("invalid stash branch review")
			}
			return m.repo.BranchReviewedStash(ctx, plan.branch, plan.stash)
		}}
	})
}

func stashListWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadInspection(m, "Stashes", func(ctx context.Context) (string, error) {
		stashes, err := m.repo.Stashes(ctx)
		if err != nil {
			return "", err
		}
		lines := make([]string, len(stashes))
		for i, stash := range stashes {
			lines[i] = stash.Ref + "  " + stash.ShortID + "  " + stash.Subject
		}
		return strings.Join(lines, "\n"), nil
	})
}

func stashDetailsText(details gitbackend.StashDetails) string {
	metadata := fmt.Sprintf("%s  %s\n%s\n%s  %s", details.Stash.ShortID, details.Stash.ID, details.Stash.Subject, details.Stash.Author, details.Stash.Date.Format("2006-01-02 15:04:05 -0700"))
	if details.PatchTruncated {
		metadata += "\n\n[patch output truncated]"
	}
	return metadata + "\n\n" + details.Patch
}

func stashShowWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadStashDialog(m, "stash show", func(choices []WorkflowChoice) WorkflowDialog {
		return WorkflowDialog{Title: "Show stash", Fields: []WorkflowField{stashSelect(choices)}, Run: func(v WorkflowValues) tea.Cmd {
			oid := v["stash"]
			return loadInspection(m, "Stash "+oid, func(ctx context.Context) (string, error) {
				details, err := m.repo.ShowStash(ctx, oid)
				if err != nil {
					return "", err
				}
				return stashDetailsText(details), nil
			})
		}}
	})
}

func stashFormatPatchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadStashDialog(m, "stash format patch", func(choices []WorkflowChoice) WorkflowDialog {
		return WorkflowDialog{Title: "Format stash as patch", Operation: "format stash patch", Fields: []WorkflowField{
			stashSelect(choices), {Name: "directory", Label: "Output directory", Kind: WorkflowText, Value: ".", Required: true},
			{Name: "numbered", Label: "Number patch", Kind: WorkflowBool}, {Name: "cover", Label: "Create cover letter", Kind: WorkflowBool},
			{Name: "signoff", Label: "Add signoff", Kind: WorkflowBool},
			{Name: "thread", Label: "Thread messages", Kind: WorkflowBool}, {Name: "subject", Label: "Subject prefix", Kind: WorkflowText},
		}, Validate: func(v WorkflowValues) error {
			if err := validPatchPath(v["directory"]); err != nil {
				return err
			}
			if v["subject"] != "" {
				return validBoundedText("subject prefix", v["subject"])
			}
			return nil
		}, Submit: func(ctx context.Context, v WorkflowValues) error {
			_, err := m.repo.FormatPatchFromStash(ctx, v["stash"], gitbackend.FormatPatchOptions{OutputDirectory: v["directory"], Numbered: v["numbered"] == "true", CoverLetter: v["cover"] == "true", Signoff: v["signoff"] == "true", Thread: v["thread"] == "true", SubjectPrefix: v["subject"]})
			return err
		}}
	})
}
