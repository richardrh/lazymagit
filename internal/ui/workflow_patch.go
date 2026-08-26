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
	amApplyMaildirID keymap.CommandID = "am.am-apply-maildir"
	amApplyPatchesID keymap.CommandID = "am.am-apply-patches"
	amContinueID     keymap.CommandID = "am.am-continue"
	amSkipID         keymap.CommandID = "am.am-skip"
	amAbortID        keymap.CommandID = "am.am-abort"
	patchApplyID     keymap.CommandID = "patch.patch-apply"
	patchCreateID    keymap.CommandID = "patch.patch-create"
	patchSaveID      keymap.CommandID = "patch.patch-save"

	patchPathLimit  = 4096
	patchInputLimit = 64 << 10
	patchFileLimit  = 256
)

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		handlers := map[keymap.CommandID]WorkflowHandler{
			amApplyMaildirID: amStartWorkflow,
			amApplyPatchesID: amStartWorkflow,
			amContinueID:     amControlWorkflow("continue", (*gitbackend.Repository).AMContinue),
			amSkipID:         amControlWorkflow("skip", (*gitbackend.Repository).AMSkip),
			amAbortID:        amControlWorkflow("abort", (*gitbackend.Repository).AMAbort),
			patchApplyID:     applyPatchWorkflow,
			patchCreateID:    formatPatchWorkflow,
			patchSaveID:      savePatchWorkflow,
		}
		// magit-patch-create is both the W c recursive transient edge and the
		// child's final c suffix. The terminal adaptation opens the same exact,
		// typed format-patch workflow at either occurrence.
		for _, binding := range keymap.Registry() {
			switch binding.UpstreamCommand {
			case "magit-patch-create":
				handlers[binding.Command] = formatPatchWorkflow
			case "magit-patch-apply":
				handlers[binding.Command] = applyPatchWorkflow
			}
		}
		return handlers
	})
}

func amControlWorkflow(name string, operation func(*gitbackend.Repository, context.Context) error) WorkflowHandler {
	return func(m *Model, _ WorkflowCommand) tea.Cmd {
		return m.StartWorkflowOperation("am "+name, func(ctx context.Context) error { return operation(m.repo, ctx) })
	}
}

func amStartWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	options, err := amOptions(command.Options)
	if err != nil {
		m.setError(err)
		return nil
	}
	title := "Apply patch series"
	if command.ID == amApplyMaildirID {
		title = "Apply maildir"
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: title, Operation: "am start", Confirmation: "Apply commits from these paths",
		Fields: []WorkflowField{{Name: "paths", Label: "Patch paths (one per line)", Kind: WorkflowText, Required: true}},
		Validate: func(values WorkflowValues) error {
			_, err := patchPaths(values["paths"])
			return err
		},
		Submit: func(ctx context.Context, values WorkflowValues) error {
			paths, err := patchPaths(values["paths"])
			if err != nil {
				return err
			}
			return m.repo.AMStart(ctx, paths, options)
		},
	})
}

func applyPatchWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	threeWay := optionEnabled(command.Options, "transient:magit-am:--3way")
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Apply patch", Operation: "apply patch", Confirmation: "Apply this file without creating a commit",
		Fields: []WorkflowField{
			{Name: "path", Label: "Patch file", Kind: WorkflowText, Required: true},
			{Name: "index", Label: "Apply to worktree and index", Kind: WorkflowBool},
			{Name: "cached", Label: "Apply to index only", Kind: WorkflowBool},
			{Name: "three-way", Label: "Use three-way merge", Kind: WorkflowBool, Bool: threeWay},
		},
		Validate: func(values WorkflowValues) error {
			if err := validPatchPath(values["path"]); err != nil {
				return err
			}
			if values["index"] == "true" && values["cached"] == "true" {
				return errors.New("index and cached modes are mutually exclusive")
			}
			return nil
		},
		Submit: func(ctx context.Context, values WorkflowValues) error {
			return m.repo.ApplyPatch(ctx, values["path"], gitbackend.ApplyPatchOptions{Index: values["index"] == "true", Cached: values["cached"] == "true", ThreeWay: values["three-way"] == "true"})
		},
	})
}

func formatPatchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Format patches", Operation: "format patches", Confirmation: "Create a bounded patch series in this existing directory",
		Fields: []WorkflowField{
			{Name: "range", Label: "Revision range", Kind: WorkflowText, Value: "HEAD~1..HEAD", Required: true},
			{Name: "directory", Label: "Output directory", Kind: WorkflowText, Value: ".", Required: true},
			{Name: "numbered", Label: "Number patches", Kind: WorkflowBool},
			{Name: "cover", Label: "Create cover letter", Kind: WorkflowBool},
			{Name: "signoff", Label: "Add signoff", Kind: WorkflowBool},
			{Name: "thread", Label: "Thread messages", Kind: WorkflowBool},
			{Name: "subject", Label: "Subject prefix", Kind: WorkflowText},
		},
		Validate: func(values WorkflowValues) error {
			if err := validBoundedText("revision range", values["range"]); err != nil {
				return err
			}
			if err := validPatchPath(values["directory"]); err != nil {
				return err
			}
			if values["subject"] != "" {
				return validBoundedText("subject prefix", values["subject"])
			}
			return nil
		},
		Submit: func(ctx context.Context, values WorkflowValues) error {
			_, err := m.repo.FormatPatchUI(ctx, values["range"], gitbackend.FormatPatchOptions{
				OutputDirectory: values["directory"], Numbered: values["numbered"] == "true",
				CoverLetter: values["cover"] == "true", Signoff: values["signoff"] == "true",
				Thread: values["thread"] == "true", SubjectPrefix: values["subject"],
			})
			return err
		},
	})
}

func savePatchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Save diff as patch", Operation: "save patch",
		Fields: []WorkflowField{
			{Name: "path", Label: "Output file", Kind: WorkflowText, Required: true},
			{Name: "range", Label: "Revision range (optional)", Kind: WorkflowText},
			{Name: "cached", Label: "Save staged changes", Kind: WorkflowBool},
			{Name: "overwrite", Label: "Overwrite existing regular file", Kind: WorkflowBool},
		},
		Validate: func(values WorkflowValues) error {
			if err := validPatchPath(values["path"]); err != nil {
				return err
			}
			if values["range"] != "" {
				return validBoundedText("revision range", values["range"])
			}
			return nil
		},
		ReviewPreflight: func(_ context.Context, values WorkflowValues) (WorkflowReview, error) {
			options := gitbackend.DiffPatchOptions{Cached: values["cached"] == "true", Range: values["range"], Overwrite: values["overwrite"] == "true"}
			reviewed, err := m.repo.ReviewDiffPatch(values["path"], options)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := []string{"destination: " + reviewed.Filename, "source: working tree diff"}
			confirmation := "Create patch file"
			if reviewed.Exists {
				plan = append(plan, fmt.Sprintf("existing regular file: %d bytes, sha256 %s", reviewed.Size, reviewed.Digest))
				if !options.Overwrite {
					return WorkflowReview{}, errors.New("output exists; enable overwrite to replace it")
				}
				confirmation = "Confirm atomic replacement of the reviewed file"
			}
			return WorkflowReview{Plan: plan, Confirmation: confirmation, Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			reviewed, ok := review.Data.(gitbackend.ReviewedDiffPatch)
			if !ok {
				return errors.New("patch output review is invalid")
			}
			return m.repo.ExecuteReviewedDiffPatch(ctx, reviewed)
		},
	})
}

func amOptions(options map[keymap.CommandID]OptionValue) (gitbackend.AMOptions, error) {
	var out gitbackend.AMOptions
	for id, value := range options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		upstream, belongs := patchOptionUpstream(id)
		if !belongs {
			continue
		}
		switch upstream {
		case "transient:magit-am:--3way":
			out.ThreeWay = true
		case "transient:magit-am:--scissors":
			out.Scissors = true
		case "magit:--signoff":
			out.Signoff = true
		default:
			return out, fmt.Errorf("%s is unavailable: the patch backend does not safely support this option", upstream)
		}
	}
	return out, nil
}

func optionEnabled(options map[keymap.CommandID]OptionValue, upstream string) bool {
	for id, value := range options {
		if candidate, ok := patchOptionUpstream(id); ok && candidate == upstream && value.Enabled {
			return true
		}
	}
	return false
}

func patchOptionUpstream(id keymap.CommandID) (string, bool) {
	for _, binding := range keymap.Registry() {
		if binding.Command == id && binding.Context == keymap.ContextTransient+"w" && binding.Kind == keymap.KindInfix {
			return binding.UpstreamCommand, true
		}
	}
	return "", false
}

func patchPaths(value string) ([]string, error) {
	if len(value) > patchInputLimit {
		return nil, errors.New("patch path input exceeds 64 KiB")
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if err := validPatchPath(path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
		if len(paths) > patchFileLimit {
			return nil, fmt.Errorf("patch series exceeds %d input paths", patchFileLimit)
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("at least one patch path is required")
	}
	return paths, nil
}

func validPatchPath(value string) error {
	if err := validBoundedText("path", value); err != nil {
		return err
	}
	if len(value) > patchPathLimit {
		return fmt.Errorf("path exceeds %d bytes", patchPathLimit)
	}
	return nil
}

func validBoundedText(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if len(value) > patchInputLimit || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s is invalid or too long", name)
	}
	return nil
}
