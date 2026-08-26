package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const (
	lifecycleCloneUpstream = "magit-clone"
	lifecycleInitUpstream  = "magit-init"
)

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		handlers := make(map[keymap.CommandID]WorkflowHandler, 6)
		for _, binding := range keymap.Registry() {
			// Register both the effective top-level I identity and Magit's I row
			// in the dispatcher transient. The status binding remains dependent
			// on the central registry classifying it as executable.
			if binding.UpstreamCommand == lifecycleInitUpstream && binding.Handler != keymap.HandlerPrefix {
				handlers[binding.Command] = initRepositoryWorkflow
				continue
			}
			if binding.Transient != lifecycleCloneUpstream || binding.Kind != keymap.KindSuffix {
				continue
			}
			var mode lifecycleCloneMode
			switch binding.UpstreamCommand {
			case "magit-clone-regular":
				mode = cloneRegular
			case "magit-clone-shallow":
				mode = cloneShallow
			case "magit-clone-bare":
				mode = cloneBare
			case "magit-clone-mirror":
				mode = cloneMirror
			default:
				continue // since/exclude and sparse have no typed backend contract.
			}
			selectedMode := mode
			handlers[binding.Command] = func(m *Model, command WorkflowCommand) tea.Cmd {
				return cloneRepositoryDialog(m, command, selectedMode)
			}
		}
		return handlers
	})
}

type lifecycleCloneMode uint8

const (
	cloneRegular lifecycleCloneMode = iota
	cloneShallow
	cloneBare
	cloneMirror
)

func cloneRepositoryWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return cloneRepositoryDialog(m, command, cloneRegular)
}

func cloneRepositoryDialog(m *Model, _ WorkflowCommand, mode lifecycleCloneMode) tea.Cmd {
	title := "Clone repository"
	depth := ""
	if mode == cloneShallow {
		title, depth = "Shallow clone repository", "1"
	} else if mode == cloneBare {
		title = "Clone bare repository"
	} else if mode == cloneMirror {
		title = "Mirror repository"
	}
	dialog := WorkflowDialog{
		Title: title, Operation: "clone repository",
		Plan: []string{
			"The destination must be absent or empty; existing content is never overwritten",
			"The current repository model will not switch; restart lazymagit in the completed path",
			"A newly-created partial destination is removed if clone fails or is cancelled",
		},
		Fields: []WorkflowField{
			{Name: "source", Label: "Repository URL or source path", Kind: WorkflowText, Required: true},
			{Name: "path", Label: "New destination path", Kind: WorkflowText, Required: true},
			{Name: "branch", Label: "Branch (optional)", Kind: WorkflowText},
			{Name: "origin", Label: "Remote name (optional)", Kind: WorkflowText},
			{Name: "depth", Label: "Depth (optional positive integer)", Kind: WorkflowText, Value: depth},
			{Name: "bare", Label: "Bare clone", Kind: WorkflowBool, Bool: mode == cloneBare},
			{Name: "recurse", Label: "Recurse submodules", Kind: WorkflowBool},
			{Name: "single", Label: "Single branch", Kind: WorkflowBool},
			{Name: "no-checkout", Label: "Do not checkout", Kind: WorkflowBool},
		},
	}
	dialog.Validate = func(values WorkflowValues) error {
		if err := validateLifecycleText(values["source"], "source"); err != nil {
			return err
		}
		if err := validateLifecycleText(values["path"], "destination"); err != nil {
			return err
		}
		if err := validateLifecycleText(values["branch"], "branch"); err != nil {
			return err
		}
		if err := validateLifecycleText(values["origin"], "remote name"); err != nil {
			return err
		}
		if credentialURL(values["source"]) {
			return errors.New("source URL contains credentials or a query; use a Git credential helper")
		}
		if _, err := lifecycleDepth(values["depth"]); err != nil {
			return err
		}
		if mode == cloneMirror && values["bare"] == "true" {
			return errors.New("mirror clone already implies bare; disable the separate bare option")
		}
		absolute, err := filepath.Abs(values["path"])
		if err != nil {
			return fmt.Errorf("resolve clone destination: %w", err)
		}
		if err := gitbackend.ValidateCloneDestination(absolute); err != nil {
			return err
		}
		// submitWorkflow reads Operation after validation. Include the exact path
		// in both progress and completion messages without pretending m.repo moved.
		m.workflow.dialog.Operation = "clone to " + absolute + "; restart lazymagit there (current repository unchanged)"
		return nil
	}
	dialog.Submit = func(ctx context.Context, values WorkflowValues) error {
		depth, err := lifecycleDepth(values["depth"])
		if err != nil {
			return err
		}
		return gitbackend.CloneRepositoryForUI(ctx, values["source"], values["path"], gitbackend.CloneOptions{
			Branch: values["branch"], Origin: values["origin"], Depth: depth, Bare: values["bare"] == "true" || mode == cloneBare,
			Mirror:            mode == cloneMirror,
			RecurseSubmodules: values["recurse"] == "true", SingleBranch: values["single"] == "true",
			NoCheckout: values["no-checkout"] == "true",
		})
	}
	return m.OpenWorkflow(dialog)
}

func initRepositoryWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	dialog := WorkflowDialog{
		Title: "Initialize repository", Operation: "initialize repository",
		Plan: []string{
			"Initialize an existing directory which does not already contain .git",
			"Existing ordinary files are preserved; repository reinitialization is refused",
			"The current repository model will not switch; restart lazymagit in the completed path",
			"Bare initialization is unsupported by the typed Init backend",
		},
		Fields: []WorkflowField{{Name: "path", Label: "Existing directory path", Kind: WorkflowText, Required: true}},
	}
	dialog.Validate = func(values WorkflowValues) error {
		if err := validateLifecycleText(values["path"], "initialization path"); err != nil {
			return err
		}
		absolute, err := filepath.Abs(values["path"])
		if err != nil {
			return fmt.Errorf("resolve initialization path: %w", err)
		}
		if err := gitbackend.ValidateInitDestination(absolute); err != nil {
			return err
		}
		m.workflow.dialog.Operation = "initialized " + absolute + "; restart lazymagit there (current repository unchanged)"
		return nil
	}
	dialog.Submit = func(ctx context.Context, values WorkflowValues) error {
		return gitbackend.InitRepositoryForUI(ctx, values["path"])
	}
	return m.OpenWorkflow(dialog)
}

func lifecycleDepth(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	depth, err := strconv.Atoi(value)
	if err != nil || depth <= 0 {
		return 0, errors.New("depth must be a positive integer")
	}
	return depth, nil
}

func validateLifecycleText(value, name string) error {
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must be a single-line value", name)
	}
	return nil
}

func credentialURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && (parsed.User != nil || parsed.RawQuery != "")
}
