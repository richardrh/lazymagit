package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

const (
	commandFileUntrack       keymap.CommandID = "file.untrack"
	commandFileRename        keymap.CommandID = "file.rename"
	commandIgnoreTop         keymap.CommandID = "gitignore.gitignore-in-topdir"
	commandIgnoreSubdir      keymap.CommandID = "gitignore.gitignore-in-subdir"
	commandIgnoreRepository  keymap.CommandID = "gitignore.gitignore-in-gitdir"
	commandIgnoreGlobal      keymap.CommandID = "gitignore.gitignore-on-system"
	commandSkipWorktree      keymap.CommandID = "gitignore.skip-worktree"
	commandNoSkipWorktree    keymap.CommandID = "gitignore.no-skip-worktree"
	commandAssumeUnchanged   keymap.CommandID = "gitignore.assume-unchanged"
	commandNoAssumeUnchanged keymap.CommandID = "gitignore.no-assume-unchanged"
)

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		byUpstream := map[string]WorkflowHandler{
			"magit-file-untrack":        fileUntrackWorkflow,
			"magit-file-rename":         fileRenameWorkflow,
			"magit-gitignore-in-topdir": ignoreTopWorkflow,
			"magit-gitignore-in-subdir": ignoreSubdirWorkflow,
			"magit-gitignore-in-gitdir": ignoreRepositoryWorkflow,
			"magit-gitignore-on-system": ignoreGlobalWorkflow,
			"magit-skip-worktree":       skipWorktreeWorkflow,
			"magit-no-skip-worktree":    noSkipWorktreeWorkflow,
			"magit-assume-unchanged":    assumeUnchangedWorkflow,
			"magit-no-assume-unchanged": noAssumeUnchangedWorkflow,
		}
		registered := map[keymap.CommandID]WorkflowHandler{
			commandFileUntrack: fileUntrackWorkflow, commandFileRename: fileRenameWorkflow,
			commandIgnoreTop: ignoreTopWorkflow, commandIgnoreSubdir: ignoreSubdirWorkflow,
			commandIgnoreRepository: ignoreRepositoryWorkflow, commandIgnoreGlobal: ignoreGlobalWorkflow,
			commandSkipWorktree: skipWorktreeWorkflow, commandNoSkipWorktree: noSkipWorktreeWorkflow,
			commandAssumeUnchanged: assumeUnchangedWorkflow, commandNoAssumeUnchanged: noAssumeUnchangedWorkflow,
		}
		// The keymap owns command identities. Looking them up by the lossless
		// upstream identity keeps this domain compatible with generated IDs and
		// with top-level bindings whose IDs are intentionally hand assigned.
		for _, binding := range keymap.Registry() {
			if handler := byUpstream[binding.UpstreamCommand]; handler != nil {
				registered[binding.Command] = handler
			}
		}
		return registered
	})
}

type selectedFileContext struct {
	path string
	kind rowKind
}

func selectedFileForWorkflow(m *Model, kinds ...rowKind) (selectedFileContext, error) {
	r, ok := m.selectedFile(kinds...)
	if !ok {
		return selectedFileContext{}, errors.New("select an applicable status file")
	}
	if strings.ContainsRune(r.path, '\x00') {
		return selectedFileContext{}, errors.New("paths containing NUL are unsupported")
	}
	return selectedFileContext{path: r.path, kind: r.kind}, nil
}

func (s selectedFileContext) verify(ctx context.Context, repo *gitbackend.Repository, trackedOnly bool) error {
	status, err := repo.Status(ctx)
	if err != nil {
		return err
	}
	for _, file := range status.Files {
		if file.Path != s.path {
			continue
		}
		if !selectedFileStatusPresent(s.kind, file) {
			break
		}
		if trackedOnly && file.Unstaged == gitbackend.ChangeUntracked {
			return errors.New("operation requires a tracked file")
		}
		return nil
	}
	return fmt.Errorf("selected status row for %q is stale; refresh and select it again", sanitizeSingleLine(s.path))
}

func selectedFileStatusPresent(kind rowKind, file gitbackend.FileStatus) bool {
	switch kind {
	case rowUntracked:
		return file.Unstaged == gitbackend.ChangeUntracked
	case rowUnstaged:
		return file.Unstaged != gitbackend.ChangeNone && file.Unstaged != gitbackend.ChangeUntracked
	case rowStaged:
		return file.Staged != gitbackend.ChangeNone
	default:
		return false
	}
}

func fileUntrackWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	selected, err := selectedFileForWorkflow(m, rowUnstaged, rowStaged)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Stop tracking file", Operation: "untrack file",
		Plan: []string{
			"Remove only " + sanitizeSingleLine(selected.path) + " from the Git index",
			"Keep the worktree file on disk (this is not delete/discard)",
			"Preserve unrelated staged content",
		},
		Confirmation: "Press Enter to review; Enter again to stop tracking",
		ReviewPreflight: func(ctx context.Context, _ WorkflowValues) (WorkflowReview, error) {
			if err := selected.verify(ctx, m.repo, true); err != nil {
				return WorkflowReview{}, err
			}
			return WorkflowReview{Plan: []string{"Keep worktree file", "Remove from index: " + sanitizeSingleLine(selected.path)}, Confirmation: "This does not delete the file", Data: selected}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			token, ok := review.Data.(selectedFileContext)
			if !ok || token != selected {
				return errors.New("invalid untrack review token")
			}
			if err := selected.verify(ctx, m.repo, true); err != nil {
				return err
			}
			return m.repo.Untrack(ctx, []string{selected.path})
		},
	})
}

func fileRenameWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	selected, err := selectedFileForWorkflow(m, rowUntracked, rowUnstaged, rowStaged)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Rename file", Operation: "rename file",
		Plan:   []string{"Rename the selected path without overwriting an existing destination", "Tracked renames are staged; untracked renames remain untracked"},
		Fields: []WorkflowField{{Name: "destination", Label: "Repository-relative destination", Kind: WorkflowText, Value: selected.path, Required: true}},
		Validate: func(values WorkflowValues) error {
			destination := values["destination"]
			if destination == selected.path {
				return errors.New("destination must differ from the selected path")
			}
			if strings.ContainsAny(destination, "\x00\r\n") {
				return errors.New("destination must be a single-line path")
			}
			return nil
		},
		ReviewPreflight: func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
			if err := selected.verify(ctx, m.repo, false); err != nil {
				return WorkflowReview{}, err
			}
			destination := values["destination"]
			return WorkflowReview{Plan: []string{"From: " + sanitizeSingleLine(selected.path), "To:   " + sanitizeSingleLine(destination), "Existing destinations are never replaced"}, Confirmation: "Press Enter again to rename", Data: destination}, nil
		},
		SubmitReview: func(ctx context.Context, values WorkflowValues, review WorkflowReview) error {
			destination, ok := review.Data.(string)
			if !ok || destination != values["destination"] {
				return errors.New("invalid rename review token")
			}
			if err := selected.verify(ctx, m.repo, false); err != nil {
				return err
			}
			return m.repo.RenamePath(ctx, selected.path, destination)
		},
	})
}

func ignoreTopWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return ignoreWorkflow(m, command, gitbackend.IgnoreTopLevel)
}
func ignoreSubdirWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return ignoreWorkflow(m, command, gitbackend.IgnoreSubdirectory)
}
func ignoreRepositoryWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return ignoreWorkflow(m, command, gitbackend.IgnoreRepositoryExclude)
}
func ignoreGlobalWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return ignoreWorkflow(m, command, gitbackend.IgnoreGlobalExclude)
}

func ignoreWorkflow(m *Model, _ WorkflowCommand, target gitbackend.IgnoreTarget) tea.Cmd {
	selected, err := selectedFileForWorkflow(m, rowUntracked, rowUnstaged, rowStaged)
	if err != nil {
		m.setError(err)
		return nil
	}
	if strings.ContainsAny(selected.path, "\r\n") {
		m.setError(errors.New("gitignore cannot represent a filename containing a newline"))
		return nil
	}
	directory := ""
	rulePath := filepath.ToSlash(selected.path)
	if target == gitbackend.IgnoreSubdirectory {
		directory = filepath.ToSlash(filepath.Dir(selected.path))
		if directory == "." {
			directory = ""
		}
		rulePath = filepath.Base(selected.path)
	}
	rule := "/" + escapeIgnoreLiteral(filepath.ToSlash(rulePath))
	if target == gitbackend.IgnoreGlobalExclude {
		// A global excludes file has no repository root against which an anchored
		// path can be interpreted. Default to the literal basename instead.
		rule = escapeIgnoreLiteral(filepath.Base(selected.path))
	}
	targetLabel := map[gitbackend.IgnoreTarget]string{
		gitbackend.IgnoreTopLevel: ".gitignore", gitbackend.IgnoreSubdirectory: "nearest .gitignore",
		gitbackend.IgnoreRepositoryExclude: ".git/info/exclude", gitbackend.IgnoreGlobalExclude: "global excludes file",
	}[target]
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Add ignore rule", Operation: "add ignore rule",
		Plan:   []string{"Append one literal-safe rule to " + targetLabel, "Shared .gitignore updates stage only the appended rule"},
		Fields: []WorkflowField{{Name: "rule", Label: "Ignore pattern", Kind: WorkflowText, Value: rule, Required: true}},
		Validate: func(values WorkflowValues) error {
			if strings.ContainsAny(values["rule"], "\x00\r\n") {
				return errors.New("ignore pattern must be one line")
			}
			return nil
		},
		Preflight: func(ctx context.Context, _ WorkflowValues) error { return selected.verify(ctx, m.repo, false) },
		Submit: func(ctx context.Context, values WorkflowValues) error {
			if err := selected.verify(ctx, m.repo, false); err != nil {
				return err
			}
			_, err := m.repo.AddIgnoreRule(ctx, values["rule"], directory, target)
			return err
		},
	})
}

func escapeIgnoreLiteral(path string) string {
	var result strings.Builder
	for _, r := range path {
		switch r {
		case '\\', '*', '?', '[', ']', '#', '!', ' ':
			result.WriteByte('\\')
		}
		result.WriteRune(r)
	}
	return result.String()
}

func skipWorktreeWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return indexFlagWorkflow(m, command, gitbackend.SkipWorktree, true)
}
func noSkipWorktreeWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return indexFlagWorkflow(m, command, gitbackend.SkipWorktree, false)
}
func assumeUnchangedWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return indexFlagWorkflow(m, command, gitbackend.AssumeUnchanged, true)
}
func noAssumeUnchangedWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return indexFlagWorkflow(m, command, gitbackend.AssumeUnchanged, false)
}

func indexFlagWorkflow(m *Model, _ WorkflowCommand, flag gitbackend.IndexFlag, set bool) tea.Cmd {
	selected, err := selectedFileForWorkflow(m, rowUnstaged, rowStaged)
	if err != nil {
		m.setError(err)
		return nil
	}
	name := map[gitbackend.IndexFlag]string{gitbackend.SkipWorktree: "skip-worktree", gitbackend.AssumeUnchanged: "assume-unchanged"}[flag]
	action := "Set "
	if !set {
		action = "Clear "
	}
	check := func(ctx context.Context) error {
		if err := selected.verify(ctx, m.repo, true); err != nil {
			return err
		}
		paths, err := m.repo.ListIndexFlag(ctx, flag)
		if err != nil {
			return err
		}
		currentlySet := slices.Contains(paths, selected.path)
		if currentlySet == set {
			return fmt.Errorf("%s is already %s for %q", name, map[bool]string{true: "set", false: "clear"}[set], sanitizeSingleLine(selected.path))
		}
		return nil
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: action + name, Operation: strings.ToLower(action) + name,
		Plan: []string{action + name + " for " + sanitizeSingleLine(selected.path), "The worktree file and index content are not deleted"},
		ReviewPreflight: func(ctx context.Context, _ WorkflowValues) (WorkflowReview, error) {
			if err := check(ctx); err != nil {
				return WorkflowReview{}, err
			}
			return WorkflowReview{Plan: []string{action + name + ": " + sanitizeSingleLine(selected.path)}, Confirmation: "Press Enter again to update the index flag", Data: selected}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			if token, ok := review.Data.(selectedFileContext); !ok || token != selected {
				return errors.New("invalid index-flag review token")
			}
			if err := check(ctx); err != nil {
				return err
			}
			return m.repo.SetIndexFlag(ctx, flag, []string{selected.path}, set)
		},
	})
}
