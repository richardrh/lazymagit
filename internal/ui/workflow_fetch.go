package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

const fetchChoiceLimit = 512

const (
	commandFetchBranch    keymap.CommandID = "fetch.fetch-branch"
	commandFetchRefspec   keymap.CommandID = "fetch.fetch-refspec"
	commandFetchModules   keymap.CommandID = "fetch.fetch-modules"
	commandFetchConfigure keymap.CommandID = "fetch.branch-configure"
	optionFetchPrune      keymap.CommandID = "fetch.--prune"
	optionFetchTags       keymap.CommandID = "fetch.--tags"
	optionFetchUnshallow  keymap.CommandID = "fetch.--unshallow"
	optionFetchForce      keymap.CommandID = "fetch.--force"
)

func init() {
	fetchOptions := []string{"transient:magit-fetch:--prune", "transient:magit-fetch:--tags", "transient:magit-fetch:--unshallow", "transient:magit-fetch:--force"}
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-fetch", map[string][]string{
		"magit-fetch-from-pushremote": fetchOptions,
		"magit-fetch-from-upstream":   fetchOptions,
		"magit-fetch-other":           fetchOptions,
		"magit-fetch-all":             fetchOptions,
		"magit-fetch-branch":          fetchOptions,
		"magit-fetch-refspec":         fetchOptions,
		"magit-fetch-modules":         fetchOptions,
	})...)
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			keymap.CommandFetchPush:      fetchPushWorkflow,
			keymap.CommandFetchUpstream:  fetchUpstreamWorkflow,
			keymap.CommandFetchElsewhere: fetchElsewhereWorkflow,
			keymap.CommandFetchAll:       fetchAllWorkflow,
			commandFetchBranch:           fetchBranchWorkflow,
			commandFetchRefspec:          fetchRefspecWorkflow,
			commandFetchModules:          fetchModulesWorkflow,
			commandFetchConfigure:        fetchConfigureWorkflow,
		}
	})
}

func fetchArgsFromCommand(command WorkflowCommand) gitbackend.FetchArgs {
	enabled := func(id keymap.CommandID) bool { return command.Options[id].Enabled }
	args := gitbackend.FetchArgs{
		Prune:     enabled(optionFetchPrune),
		Unshallow: enabled(optionFetchUnshallow),
		Force:     enabled(optionFetchForce),
	}
	if enabled(optionFetchTags) {
		args.Tags = gitbackend.FetchAllTags
	}
	return args
}

func fetchPushWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	if m.repo == nil {
		if m.snapshot.pushRemote != "" {
			return m.StartWorkflowOperation("fetch push remote", m.fetchPush)
		}
		if len(m.snapshot.remotes) == 0 {
			m.setError(errors.New("configure push remote: no remotes configured"))
			return nil
		}
		m.remoteCursor, m.remotePurpose = 0, remoteConfigurePush
		m.setMode(modeRemotes)
		return nil
	}
	args := fetchArgsFromCommand(command)
	if m.snapshot.pushRemote == "" {
		return fetchChoices(m, "configure push remote", command, func(choices gitbackend.FetchUIChoices, args gitbackend.FetchArgs) (WorkflowDialog, error) {
			field, err := remoteField(choices.Remotes)
			if err != nil {
				return WorkflowDialog{}, err
			}
			return WorkflowDialog{Title: "Configure push remote and fetch", Operation: "configure and fetch push remote", Fields: []WorkflowField{field}, Submit: func(ctx context.Context, values WorkflowValues) error {
				if err := m.setPushRemote(ctx, values["remote"]); err != nil {
					return err
				}
				return m.repo.FetchPushWithArgs(ctx, args)
			}}, nil
		})
	}
	return m.StartWorkflowOperation("fetch push remote", func(ctx context.Context) error { return m.repo.FetchPushWithArgs(ctx, args) })
}

func fetchUpstreamWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	if m.repo == nil {
		return m.StartWorkflowOperation("fetch upstream", m.fetchUpstream)
	}
	args := fetchArgsFromCommand(command)
	return m.StartWorkflowOperation("fetch upstream", func(ctx context.Context) error { return m.repo.FetchUpstreamWithArgs(ctx, args) })
}

func fetchAllWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	if m.repo == nil {
		return m.StartWorkflowOperation("fetch all remotes", m.fetchAll)
	}
	args := fetchArgsFromCommand(command)
	return m.StartWorkflowOperation("fetch all remotes", func(ctx context.Context) error { return m.repo.FetchAllWithArgs(ctx, args) })
}

func fetchModulesWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	args := fetchArgsFromCommand(command)
	return m.StartWorkflowOperation("fetch modules", func(ctx context.Context) error { return m.repo.FetchModulesWithArgs(ctx, args) })
}

func fetchChoices(m *Model, title string, command WorkflowCommand, build func(gitbackend.FetchUIChoices, gitbackend.FetchArgs) (WorkflowDialog, error)) tea.Cmd {
	args := fetchArgsFromCommand(command)
	return m.LoadWorkflow(title, func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := m.repo.FetchChoices(ctx, fetchChoiceLimit)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return build(choices, args)
	})
}

func remoteField(remotes []string) (WorkflowField, error) {
	if len(remotes) == 0 {
		return WorkflowField{}, errors.New("fetch requires a configured remote")
	}
	choices := make([]WorkflowChoice, len(remotes))
	for i, remote := range remotes {
		choices[i] = WorkflowChoice{Value: remote, Label: remote}
	}
	return WorkflowField{Name: "remote", Label: "Remote", Kind: WorkflowSelect, Value: remotes[0], Choices: choices, Required: true}, nil
}

func fetchElsewhereWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	if m.repo == nil {
		if len(m.snapshot.remotes) == 0 {
			m.setError(errors.New("fetch elsewhere: no remotes configured"))
			return nil
		}
		m.remoteCursor, m.remotePurpose = 0, remoteFetchElsewhere
		m.setMode(modeRemotes)
		return nil
	}
	return fetchChoices(m, "fetch remote", command, func(choices gitbackend.FetchUIChoices, args gitbackend.FetchArgs) (WorkflowDialog, error) {
		field, err := remoteField(choices.Remotes)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Fetch elsewhere", Operation: "fetch remote", Fields: []WorkflowField{field}, Submit: func(ctx context.Context, values WorkflowValues) error {
			args.Remote = values["remote"]
			return m.repo.FetchWithArgs(ctx, args)
		}}, nil
	})
}

func branchChoiceField(branches []gitbackend.FetchRemoteBranch) (WorkflowField, error) {
	if len(branches) == 0 {
		return WorkflowField{}, errors.New("no configured remote branches are available; fetch a configured remote first")
	}
	choices := make([]WorkflowChoice, len(branches))
	for i, branch := range branches {
		choices[i] = WorkflowChoice{Value: strconv.Itoa(i), Label: branch.Remote + "/" + branch.Branch}
	}
	return WorkflowField{Name: "target", Label: "Remote branch", Kind: WorkflowSelect, Value: "0", Choices: choices, Required: true}, nil
}

func selectedRemoteBranch(values WorkflowValues, branches []gitbackend.FetchRemoteBranch) (gitbackend.FetchRemoteBranch, error) {
	i, err := strconv.Atoi(values["target"])
	if err != nil || i < 0 || i >= len(branches) {
		return gitbackend.FetchRemoteBranch{}, errors.New("selected remote branch is invalid")
	}
	return branches[i], nil
}

func fetchBranchWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return fetchChoices(m, "remote branches", command, func(choices gitbackend.FetchUIChoices, args gitbackend.FetchArgs) (WorkflowDialog, error) {
		field, err := branchChoiceField(choices.RemoteBranches)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Fetch another branch", Operation: "fetch branch", Fields: []WorkflowField{field}, Validate: func(values WorkflowValues) error {
			_, err := selectedRemoteBranch(values, choices.RemoteBranches)
			return err
		}, Submit: func(ctx context.Context, values WorkflowValues) error {
			target, err := selectedRemoteBranch(values, choices.RemoteBranches)
			if err != nil {
				return err
			}
			args.Remote, args.Branch = target.Remote, target.Branch
			return m.repo.FetchWithArgs(ctx, args)
		}}, nil
	})
}

func fetchRefspecWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return fetchChoices(m, "fetch refspec", command, func(choices gitbackend.FetchUIChoices, args gitbackend.FetchArgs) (WorkflowDialog, error) {
		remote, err := remoteField(choices.Remotes)
		if err != nil {
			return WorkflowDialog{}, err
		}
		refspec := WorkflowField{Name: "refspec", Label: "Refspec", Kind: WorkflowText, Required: true}
		return WorkflowDialog{Title: "Fetch explicit refspec", Operation: "fetch refspec", Fields: []WorkflowField{remote, refspec}, Validate: func(values WorkflowValues) error {
			if strings.TrimSpace(values["refspec"]) != values["refspec"] {
				return errors.New("refspec must not have surrounding whitespace")
			}
			return nil
		}, Preflight: func(ctx context.Context, values WorkflowValues) error {
			// FetchWithArgs performs authoritative check-ref-format validation before
			// mutation; this preflight intentionally launches no process mutation.
			if strings.ContainsAny(values["refspec"], "\x00\r\n") || strings.HasPrefix(values["refspec"], "-") {
				return fmt.Errorf("invalid refspec %q", values["refspec"])
			}
			return nil
		}, Submit: func(ctx context.Context, values WorkflowValues) error {
			args.Remote, args.Refspec = values["remote"], values["refspec"]
			return m.repo.FetchWithArgs(ctx, args)
		}}, nil
	})
}

func fetchConfigureWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return branchConfigure(m, command)
}
