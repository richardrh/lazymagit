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
	commitTargetField  = "target"
	commitMessageField = "message"
	commitConsentField = "signing-consent"
)

type commitWorkflowSpec struct {
	upstream string
	title    string
	variant  gitbackend.CommitUIVariant
	target   bool
	message  bool
	required bool
}

var commitWorkflowSpecs = []commitWorkflowSpec{
	{"magit-commit-create", "Create commit", gitbackend.CommitUICreate, false, true, true},
	{"magit-commit-extend", "Extend HEAD", gitbackend.CommitUIExtend, false, false, false},
	{"magit-commit-amend", "Amend HEAD", gitbackend.CommitUIAmend, false, true, true},
	{"magit-commit-reword", "Reword HEAD", gitbackend.CommitUIReword, false, true, true},
	{"magit-commit-fixup", "Create fixup commit", gitbackend.CommitUIFixup, true, false, false},
	{"magit-commit-squash", "Create squash commit", gitbackend.CommitUISquash, true, true, false},
	{"magit-commit-alter", "Create alter commit", gitbackend.CommitUIAlter, true, true, true},
	{"magit-commit-augment", "Create augment commit", gitbackend.CommitUIAugment, true, true, true},
	{"magit-commit-revise", "Create revise commit", gitbackend.CommitUIRevise, true, true, true},
}

func init() {
	RegisterWorkflowDomain(func(m *Model) map[keymap.CommandID]WorkflowHandler {
		handlers := make(map[keymap.CommandID]WorkflowHandler, len(commitWorkflowSpecs))
		for _, spec := range commitWorkflowSpecs {
			id, ok := commitCommandID(spec.upstream)
			if !ok {
				panic("ui: missing Magit 4.7 commit command " + spec.upstream)
			}
			spec := spec
			handlers[id] = func(_ *Model, command WorkflowCommand) tea.Cmd {
				return openCommitWorkflow(m, spec, command)
			}
		}
		return handlers
	})
}

func commitCommandID(upstream string) (keymap.CommandID, bool) {
	for _, binding := range keymap.Registry() {
		if binding.Context == keymap.ContextTransient+"c" && binding.UpstreamCommand == upstream {
			return binding.Command, true
		}
	}
	return keymap.CommandNone, false
}

func commitOptionsFromWorkflow(values map[keymap.CommandID]OptionValue) (gitbackend.CommitOptions, error) {
	upstreamByID := make(map[keymap.CommandID]string)
	for _, binding := range keymap.Registry() {
		if binding.Context == keymap.ContextTransient+"c" && binding.Kind == keymap.KindInfix {
			upstreamByID[binding.Command] = binding.UpstreamCommand
		}
	}
	var out gitbackend.CommitOptions
	for id, value := range values {
		if !value.Enabled && value.Value == "" {
			continue
		}
		switch upstreamByID[id] {
		case "transient:magit-commit:--all":
			out.All = true
		case "transient:magit-commit:--allow-empty":
			out.AllowEmpty = true
		case "transient:magit-commit:--no-verify":
			out.NoVerify = true
		case "transient:magit-commit:--reset-author":
			out.ResetAuthor = true
		case "magit:--author":
			out.Author = value.Value
		case "magit-commit:--date":
			out.Date = value.Value
		case "magit:--signoff":
			out.Signoff = true
		case "magit-commit:--reuse-message":
			out.ReuseMessage = value.Value
		case "magit:--gpg-sign":
			out.Sign, out.SigningKey = value.Value == "", value.Value
		case "magit-commit:--reedit-message":
			out.ReeditMessage = value.Value
		case "transient:magit-commit:--verbose":
			return out, errors.New("verbose commit preview is unavailable")
		}
	}
	return out, nil
}

func openCommitWorkflow(m *Model, spec commitWorkflowSpec, command WorkflowCommand) tea.Cmd {
	options, err := commitOptionsFromWorkflow(command.Options)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.LoadWorkflow(spec.title, func(ctx context.Context) (WorkflowDialog, error) {
		var fields []WorkflowField
		if spec.target {
			commits, err := m.repo.RecentLog(ctx, 100)
			if err != nil {
				return WorkflowDialog{}, fmt.Errorf("load commit targets: %w", err)
			}
			if len(commits) == 0 {
				return WorkflowDialog{}, errors.New("commit target is unavailable in an unborn repository")
			}
			choices := make([]WorkflowChoice, 0, len(commits))
			for _, commit := range commits {
				choices = append(choices, WorkflowChoice{Value: commit.ID, Label: commit.ShortID + " " + commit.Subject})
			}
			fields = append(fields, WorkflowField{Name: commitTargetField, Label: "Target", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices, Required: true})
		}
		if spec.message || options.ReeditMessage != "" {
			required := (spec.required || options.ReeditMessage != "") && options.ReuseMessage == ""
			message := ""
			if options.ReeditMessage != "" {
				var err error
				message, err = m.repo.CommitMessageForUI(ctx, options.ReeditMessage)
				if err != nil {
					return WorkflowDialog{}, fmt.Errorf("load reedit message: %w", err)
				}
			}
			fields = append(fields, WorkflowField{Name: commitMessageField, Label: "Message", Kind: WorkflowText, Value: message, Required: required})
		}
		signing := options.Sign || options.SigningKey != ""
		if signing {
			fields = append(fields, WorkflowField{Name: commitConsentField, Label: "Allow Git signing program", Kind: WorkflowBool})
		}
		dialog := WorkflowDialog{
			Title: spec.title, Operation: strings.ToLower(spec.title), ActionLabel: "Review & Submit", Fields: fields,
			Confirmation: "Review commit details, then execute",
			Plan:         []string{spec.title},
			Validate: func(values WorkflowValues) error {
				if signing && values[commitConsentField] != "true" {
					return gitbackend.ErrCommitSigningConsentRequired
				}
				return nil
			},
			Submit: func(ctx context.Context, values WorkflowValues) error {
				_, err := m.repo.ExecuteCommitUI(ctx, spec.variant, values[commitTargetField], values[commitMessageField], options, values[commitConsentField] == "true")
				return err
			},
		}
		return dialog, nil
	})
}
