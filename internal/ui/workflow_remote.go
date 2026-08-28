package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const (
	commandRemoteRename        keymap.CommandID = "remote.remote-rename"
	commandRemoteRemove        keymap.CommandID = "remote.remote-remove"
	commandRemoteConfigure     keymap.CommandID = "remote.remote-configure"
	commandRemotePrune         keymap.CommandID = "remote.remote-prune"
	commandRemoteUnshallow     keymap.CommandID = "remote.remote-unshallow"
	commandRemoteDefaultBranch keymap.CommandID = "remote.update-default-branch"
	commandRemoteFetchAfterAdd keymap.CommandID = "remote.-f"
)

func init() {
	RegisterWorkflowDomain(func(m *Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			keymap.CommandAddRemote:    remoteAddWorkflow,
			commandRemoteRename:        remoteRenameWorkflow,
			commandRemoteRemove:        remoteRemoveWorkflow,
			commandRemoteConfigure:     remoteConfigureWorkflow,
			commandRemotePrune:         remotePruneWorkflow,
			commandRemoteUnshallow:     remoteUnshallowWorkflow,
			commandRemoteDefaultBranch: remoteDefaultBranchWorkflow,
		}
	})
}

func remoteAddWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	option, explicitlySet := command.Options[commandRemoteFetchAfterAdd]
	if !explicitlySet {
		// Untouched -f means add only. Fetch is an explicit opt-in.
		m.remoteInput, m.remoteField, m.remoteFetch = [2]string{}, 0, false
		m.setMode(modeAddRemote)
		return nil
	}
	fetch := option.Enabled
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Add remote", Operation: "add remote",
		Fields: []WorkflowField{
			{Name: "name", Label: "Remote name", Kind: WorkflowText, Required: true},
			{Name: "url", Label: "Fetch URL", Kind: WorkflowText, Required: true},
			{Name: "fetch", Label: "Fetch after add (-f)", Kind: WorkflowBool, Bool: fetch},
		},
		Validate: func(v WorkflowValues) error {
			if strings.ContainsAny(v["name"], "\x00\r\n") || strings.ContainsAny(v["url"], "\x00\r\n") {
				return errors.New("remote name and URL must be single-line values")
			}
			return nil
		},
		Submit: func(ctx context.Context, v WorkflowValues) error {
			return m.repo.AddRemote(ctx, strings.TrimSpace(v["name"]), v["url"], v["fetch"] == "true")
		},
	})
}

func remoteChoices(ctx context.Context, m *Model) ([]WorkflowChoice, error) {
	remotes, err := m.repo.Remotes(ctx)
	if err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return nil, errors.New("no remotes configured")
	}
	choices := make([]WorkflowChoice, 0, len(remotes))
	for _, remote := range remotes {
		choices = append(choices, WorkflowChoice{Value: remote.Name, Label: remote.Name})
	}
	return choices, nil
}

func remoteSelectField(choices []WorkflowChoice) WorkflowField {
	return WorkflowField{Name: "remote", Label: "Remote", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices, Required: true}
}

func remoteRenameWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("remote rename", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{
			Title: "Rename remote", Operation: "rename remote",
			Fields: []WorkflowField{remoteSelectField(choices), {Name: "new", Label: "New name", Kind: WorkflowText, Required: true}},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				plan, err := m.repo.ReviewRemoteRename(ctx, v["remote"], strings.TrimSpace(v["new"]))
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: gitbackend.RemoteChangePlanLines(plan), Confirmation: "Review affected refs and configuration", Data: plan}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				plan, ok := review.Data.(gitbackend.ReviewedRemoteChange)
				if !ok {
					return errors.New("invalid remote rename review")
				}
				return m.repo.RenameRemoteReviewed(ctx, plan)
			},
		}, nil
	})
}

func remoteRemoveWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("remote removal", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{
			Title: "Remove remote", Operation: "remove remote", Fields: []WorkflowField{remoteSelectField(choices)},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				plan, err := m.repo.ReviewRemoteRemoval(ctx, v["remote"])
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: gitbackend.RemoteChangePlanLines(plan), Confirmation: "Remove this remote and the listed refs/config?", Data: plan}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				plan, ok := review.Data.(gitbackend.ReviewedRemoteChange)
				if !ok {
					return errors.New("invalid remote removal review")
				}
				return m.repo.RemoveRemoteReviewed(ctx, plan)
			},
		}, nil
	})
}

func remotePruneWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("remote prune", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{
			Title: "Prune stale remote branches", Operation: "prune remote", Fields: []WorkflowField{remoteSelectField(choices)},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				plan, err := m.repo.RemotePruneComparison(ctx, v["remote"])
				if err != nil {
					return WorkflowReview{}, err
				}
				lines := []string{"remote: " + plan.Remote}
				for _, ref := range plan.StaleTrackingRefs {
					lines = append(lines, "delete tracking ref: "+ref)
				}
				if len(plan.StaleTrackingRefs) == 0 {
					lines = append(lines, "No stale tracking refs")
				}
				return WorkflowReview{Plan: lines, Confirmation: "Prune exactly this reviewed set?", Data: plan}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				plan, ok := review.Data.(gitbackend.RemotePrunePlan)
				if !ok {
					return errors.New("invalid remote prune review")
				}
				_, err := m.repo.PruneRemote(ctx, plan, plan.Token)
				return err
			},
		}, nil
	})
}

func remoteUnshallowWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("remote unshallow", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{
			Title: "Unshallow remote", Operation: "unshallow remote", Fields: []WorkflowField{remoteSelectField(choices)},
			Submit: func(ctx context.Context, v WorkflowValues) error {
				return m.repo.FetchWithArgs(ctx, gitbackend.FetchArgs{Remote: v["remote"], Unshallow: true})
			},
		}, nil
	})
}

var configureModeChoices = []WorkflowChoice{{Value: "unchanged", Label: "unchanged"}, {Value: "replace", Label: "replace"}, {Value: "clear", Label: "clear"}}

func remoteConfigureWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("remote configuration", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		// Choices are asynchronous; configuration itself is loaded during reviewed
		// preflight so changing the selected remote cannot retain stale defaults.
		follow := command.Options["remote.remote.followremotehead"].Value
		return WorkflowDialog{
			Title: "Configure remote", Operation: "configure remote",
			Fields: []WorkflowField{
				remoteSelectField(choices),
				{Name: "u-mode", Label: "u Fetch URL action", Kind: WorkflowEnum, Value: "unchanged", Choices: configureModeChoices[:2]},
				{Name: "u", Label: "u Fetch URL", Kind: WorkflowText},
				{Name: "U-mode", Label: "U Fetch refspec action", Kind: WorkflowEnum, Value: "unchanged", Choices: configureModeChoices},
				{Name: "U", Label: "U Fetch refspecs (JSON array)", Kind: WorkflowText, Value: "[]"},
				{Name: "s-mode", Label: "s Push URL action", Kind: WorkflowEnum, Value: "unchanged", Choices: configureModeChoices[:2]},
				{Name: "s", Label: "s Push URL", Kind: WorkflowText},
				{Name: "S-mode", Label: "S Push refspec action", Kind: WorkflowEnum, Value: "unchanged", Choices: configureModeChoices},
				{Name: "S", Label: "S Push refspecs (JSON array)", Kind: WorkflowText, Value: "[]"},
				{Name: "O", Label: "O Tag behavior", Kind: WorkflowEnum, Value: "unchanged", Choices: []WorkflowChoice{{Value: "unchanged", Label: "unchanged"}, {Value: "default", Label: "default"}, {Value: "all", Label: "all tags"}, {Value: "none", Label: "no tags"}}},
				{Name: "h", Label: "h Follow remote HEAD", Kind: WorkflowEnum, Value: follow, Choices: []WorkflowChoice{{Value: "unchanged", Label: "unchanged"}, {Value: "default", Label: "default"}, {Value: "never", Label: "never"}, {Value: "create", Label: "create"}, {Value: "warn", Label: "warn"}, {Value: "always", Label: "always"}}},
			},
			Validate: func(v WorkflowValues) error { _, err := remoteConfigArgs(v); return err },
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				args, err := remoteConfigArgs(v)
				if err != nil {
					return WorkflowReview{}, err
				}
				plan, err := m.repo.ReviewRemoteConfiguration(ctx, args)
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: append([]string{"remote: " + args.Remote}, plan.Changes...), Confirmation: "Apply exactly these reviewed configuration changes?", Data: plan}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				plan, ok := review.Data.(gitbackend.ReviewedRemoteConfiguration)
				if !ok {
					return errors.New("invalid remote configuration review")
				}
				return m.repo.ConfigureRemoteReviewed(ctx, plan)
			},
		}, nil
	})
}

func remoteConfigArgs(v WorkflowValues) (gitbackend.RemoteConfigArgs, error) {
	args := gitbackend.RemoteConfigArgs{Remote: v["remote"]}
	if v["u-mode"] == "replace" {
		value := v["u"]
		args.FetchURL = &value
	}
	if v["s-mode"] == "replace" {
		value := v["s"]
		args.PushURL = &value
	}
	for _, f := range []struct {
		mode, value string
		dst         *[]string
	}{{"U-mode", "U", &args.FetchRefspecs}, {"S-mode", "S", &args.PushRefspecs}} {
		switch v[f.mode] {
		case "unchanged":
		case "clear":
			*f.dst = []string{}
		case "replace":
			if err := json.Unmarshal([]byte(v[f.value]), f.dst); err != nil {
				return args, fmt.Errorf("%s must be a JSON string array: %w", f.value, err)
			}
			if *f.dst == nil {
				return args, fmt.Errorf("%s must be an array, not null", f.value)
			}
		default:
			return args, fmt.Errorf("invalid %s", f.mode)
		}
	}
	switch v["O"] {
	case "unchanged":
	case "default":
		value := gitbackend.RemoteTagsDefault
		args.TagOpt = &value
	case "all":
		value := gitbackend.RemoteTagsAll
		args.TagOpt = &value
	case "none":
		value := gitbackend.RemoteTagsNone
		args.TagOpt = &value
	default:
		return args, errors.New("invalid O tag behavior")
	}
	followModes := map[string]gitbackend.RemoteFollowRemoteHEAD{"default": gitbackend.RemoteFollowRemoteHEADDefault, "never": gitbackend.RemoteFollowRemoteHEADNever, "create": gitbackend.RemoteFollowRemoteHEADCreate, "warn": gitbackend.RemoteFollowRemoteHEADWarn, "always": gitbackend.RemoteFollowRemoteHEADAlways}
	if v["h"] != "" && v["h"] != "unchanged" {
		mode, ok := followModes[v["h"]]
		if !ok {
			return args, errors.New("invalid h follow remote HEAD behavior")
		}
		args.FollowRemoteHEAD = &mode
	}
	return args, nil
}

func remoteDefaultBranchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("remote default branch", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{
			Title: "Update remote default branch", Operation: "update remote default branch", Fields: []WorkflowField{remoteSelectField(choices)},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				plan, err := m.repo.ReviewRemoteDefaultBranch(ctx, v["remote"])
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: []string{"remote: " + plan.Remote, "tracking HEAD: " + plan.PreviousRef + " -> " + plan.NewRef}, Confirmation: "Update this symbolic tracking ref?", Data: plan}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				plan, ok := review.Data.(gitbackend.RemoteDefaultBranchPlan)
				if !ok {
					return errors.New("invalid default branch review")
				}
				return m.repo.UpdateRemoteDefaultBranch(ctx, plan)
			},
		}, nil
	})
}
