package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
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
	upstreamByID := commitOptionUpstreams()
	var out gitbackend.CommitOptions
	for id, value := range values {
		if !optionValueActive(value) {
			continue
		}
		if err := applyCommitWorkflowOption(&out, upstreamByID[id], value); err != nil {
			return out, err
		}
	}
	return out, nil
}

func commitOptionUpstreams() map[keymap.CommandID]string {
	upstreamByID := make(map[keymap.CommandID]string)
	for _, binding := range keymap.Registry() {
		if binding.Context == keymap.ContextTransient+"c" && binding.Kind == keymap.KindInfix {
			upstreamByID[binding.Command] = binding.UpstreamCommand
		}
	}
	return upstreamByID
}

func applyCommitWorkflowOption(out *gitbackend.CommitOptions, upstream string, value OptionValue) error {
	switch upstream {
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
		return errors.New("verbose commit preview is unavailable")
	}
	return nil
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
			selected := selectedHistoryRevision(m)
			if selected == "HEAD" {
				selected = choices[0].Value
			}
			fields = append(fields, WorkflowField{Name: commitTargetField, Label: "Target", Kind: WorkflowSelect, Value: selected, Choices: choices, AllowCustom: true, Required: true})
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
			fields = append(fields, WorkflowField{Name: commitMessageField, Label: "Message (internal editor)", Kind: WorkflowMultiline, Value: message, Required: required})
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
		}
		if spec.target {
			dialog.ReviewPreflight = func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
				review, err := m.repo.ReviewCommitUI(ctx, gitbackend.CommitUIRequest{Variant: spec.variant, Target: values[commitTargetField], Message: values[commitMessageField], Options: options, SigningConsent: values[commitConsentField] == "true"})
				if err != nil {
					return WorkflowReview{}, err
				}
				plan := append([]string(nil), review.Plan...)
				plan = append(plan, "HEAD "+review.State.HEAD, "Index "+review.State.Index, "Worktree "+review.State.Worktree)
				return WorkflowReview{Plan: plan, Confirmation: "Execute only if the exact reviewed repository state is unchanged", Data: review}, nil
			}
			dialog.SubmitReview = func(ctx context.Context, _ WorkflowValues, transported WorkflowReview) error {
				review, ok := transported.Data.(gitbackend.ReviewedCommitUI)
				if !ok {
					return errors.New("invalid reviewed commit token")
				}
				_, err := m.repo.ExecuteReviewedCommitUI(ctx, review)
				return err
			}
		} else {
			dialog.Submit = func(ctx context.Context, values WorkflowValues) error {
				_, err := m.repo.ExecuteCommitUI(ctx, spec.variant, values[commitTargetField], values[commitMessageField], options, values[commitConsentField] == "true")
				return err
			}
		}
		return dialog, nil
	})
}
