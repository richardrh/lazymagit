package ui

// Inspection workflows intentionally reuse the status detail pane.  It is a
// read-only, pageable result view already owned by Model, and diffMsg gives us
// the same cancellation/stale-result boundary as ordinary file details.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const (
	inspectOutputLimit = 512 << 10
	inspectItemLimit   = 512
)

// These are the read-only portions of the vendored transients.  Deliberately
// absent are Emacs buffer operations (save transient values, margins, buffer
// locks, hunk fontification/refinement), mutation (stage/resolve), reflogs and
// external tools (Ediff and mergetool).  Ediff display commands are adapted to
// terminal-safe unified comparisons below.
var inspectSuffixes = map[string]WorkflowHandler{
	// magit-diff
	"magit-diff-dwim":         inspectDiffDWIM,
	"magit-diff-range":        inspectDiffRange,
	"magit-diff-paths":        inspectDiffPaths,
	"magit-diff-unstaged":     inspectDiffUnstaged,
	"magit-diff-staged":       inspectDiffStaged,
	"magit-diff-working-tree": inspectDiffWorkingTree,
	"magit-show-commit":       inspectShowCommit,
	"magit-stash-show":        inspectShowStash,

	// The useful, stateless portion of magit-diff-refresh.
	"magit-diff-refresh": inspectDiffDWIM,

	// magit-ediff display commands, adapted to a unified terminal comparison.
	"magit-ediff-dwim":              inspectCompareDWIM,
	"magit-ediff-show-unstaged":     inspectCompareUnstaged,
	"magit-ediff-show-staged":       inspectCompareStaged,
	"magit-ediff-show-working-tree": inspectCompareWorkingTree,
	"magit-ediff-show-commit":       inspectCompareCommit,
	"magit-ediff-compare":           inspectCompareRange,
	"magit-ediff-show-stash":        inspectCompareStash,

	// magit-log. Commands needing an Emacs prompt, reflog, shortlog, WIP refs,
	// or --merged support remain explicitly unregistered.
	"magit-log-current":      inspectLogCurrent,
	"magit-log-head":         inspectLogCurrent,
	"magit-log-related":      inspectLogRelated,
	"magit-log-branches":     inspectLogBranches,
	"magit-log-all-branches": inspectLogBranches,
	"magit-log-all":          inspectLogAll,
	"magit-log-refresh":      inspectLogCurrent,

	// magit-show-refs
	"magit-show-refs-head":    inspectRefs,
	"magit-show-refs-current": inspectRefs,
}

var inspectTopLevel = map[string]WorkflowHandler{
	"magit-diff":             inspectDiffDWIM,
	"magit-diff-refresh":     inspectDiffDWIM,
	"magit-ediff-dwim":       inspectCompareDWIM,
	"magit-ediff":            inspectCompareDWIM,
	"magit-log":              inspectLogCurrent,
	"magit-log-refresh":      inspectLogCurrent,
	"magit-show-refs":        inspectRefs,
	"magit-cherry":           inspectCherries,
	"magit-describe-section": inspectSectionInfo,
}

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		handlers := make(map[keymap.CommandID]WorkflowHandler)
		for _, binding := range keymap.Registry() {
			handler := inspectSuffixes[binding.UpstreamCommand]
			// The recursively vendored dispatcher repeats top-level commands.
			// Give those occurrences the same safe behavior rather than only
			// connecting the status-map occurrence.
			if handler == nil {
				handler = inspectTopLevel[binding.UpstreamCommand]
			}
			if handler != nil {
				handlers[binding.Command] = handler
			}
		}
		return handlers
	})
	// Log-refresh options are consumed by its connected read-only suffixes. Keep
	// this declaration occurrence-aware so the central transient validator can
	// pass the exact edited values to the inspection query.
	consumes := []string{"magit-log:-n", "transient:magit-log-refresh:--graph", "transient:magit-log-refresh:--decorate", "magit-log:--*-order"}
	seen := map[keymap.CommandID]bool{}
	var capabilities []WorkflowCapability
	for _, binding := range keymap.Registry() {
		if binding.Transient == "magit-log-refresh" && binding.Kind == keymap.KindSuffix && inspectSuffixes[binding.UpstreamCommand] != nil && !seen[binding.Command] {
			seen[binding.Command] = true
			capabilities = append(capabilities, WorkflowCapability{ID: binding.Command, Transient: binding.Transient, UpstreamCommand: binding.UpstreamCommand, Consumes: consumes})
		}
	}
	RegisterWorkflowCapabilities(capabilities...)
}

type inspectLoader func(context.Context) (string, error)

func loadInspection(m *Model, title string, loader inspectLoader) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	m.cancelDetail()
	m.inspectionActive = true
	m.detailOffset = 0
	m.detailRequest++
	request, id := m.detailRequest, m.tree.Cursor()
	m.detailID, m.detail = id, "Loading "+sanitizeSingleLine(title)+"…"
	m.setMessage("Loading " + sanitizeSingleLine(title) + "…")
	ctx, cancel := context.WithCancel(m.appCtx)
	m.detailCtx, m.detailCancel = ctx, cancel
	return func() tea.Msg {
		text, err := loader(ctx)
		if err != nil {
			// diffMsg's normal error wording is specific to file/revision details.
			// Render a typed inspection error in the same terminal-safe pane.
			text = "Inspection failed: " + err.Error()
		}
		if strings.TrimSpace(text) == "" {
			text = "No results."
		}
		return diffMsg{id: id, request: request, text: title + "\n\n" + text}
	}
}

func selectedInspectRevision(m *Model) string {
	if row, ok := m.rows[m.tree.Cursor()]; ok && row.kind == rowCommit && row.commit.ID != "" {
		return row.commit.ID
	}
	return "HEAD"
}

func selectedInspectPath(m *Model) []string {
	if row, ok := m.rows[m.tree.Cursor()]; ok && row.path != "" {
		return []string{row.path}
	}
	return nil
}

func runDiffInspection(m *Model, title string, query gitbackend.DiffQuery) tea.Cmd {
	query.OutputLimit = inspectOutputLimit
	return loadInspection(m, title, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryDiff(ctx, query)
		if err != nil {
			return "", err
		}
		return truncationNote(result.Truncated) + result.Detail, nil
	})
}

func inspectDiffDWIM(m *Model, _ WorkflowCommand) tea.Cmd {
	row := m.rows[m.tree.Cursor()]
	switch row.kind {
	case rowStaged:
		return runDiffInspection(m, "Staged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffIndex, Files: selectedInspectPath(m)})
	case rowCommit:
		return inspectShowCommit(m, WorkflowCommand{})
	default:
		return runDiffInspection(m, "Unstaged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffWorktree, Files: selectedInspectPath(m)})
	}
}

func inspectDiffPaths(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Path diff", gitbackend.DiffQuery{Kind: gitbackend.DiffWorktree, Files: selectedInspectPath(m)})
}

func inspectDiffUnstaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Unstaged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffWorktree})
}

func inspectDiffStaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Staged diff", gitbackend.DiffQuery{Kind: gitbackend.DiffIndex})
}

func inspectDiffWorkingTree(m *Model, _ WorkflowCommand) tea.Cmd {
	return runDiffInspection(m, "Working tree diff", gitbackend.DiffQuery{Kind: gitbackend.DiffRevision, Base: "HEAD"})
}

func inspectDiffRange(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadInspection(m, "Revision range diff", func(ctx context.Context) (string, error) {
		base, target, err := inspectComparisonRevisions(ctx, m.repo, selectedInspectRevision(m))
		if err != nil {
			return "", err
		}
		result, err := m.repo.QueryDiff(ctx, gitbackend.DiffQuery{Kind: gitbackend.DiffRevisionRange, Base: base, Target: target, OutputLimit: inspectOutputLimit})
		return truncationNote(result.Truncated) + result.Detail, err
	})
}

func inspectShowCommit(m *Model, _ WorkflowCommand) tea.Cmd {
	revision := selectedInspectRevision(m)
	return loadInspection(m, "Commit "+revision, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryShowRevision(ctx, gitbackend.ShowRevisionQuery{Revision: revision, Stat: true, Patch: true, OutputLimit: inspectOutputLimit})
		return truncationNote(result.Truncated) + result.Detail, err
	})
}

func inspectShowStash(m *Model, _ WorkflowCommand) tea.Cmd {
	return loadInspection(m, "Stash", func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryShowRevision(ctx, gitbackend.ShowRevisionQuery{Revision: "stash", Stat: true, Patch: true, OutputLimit: inspectOutputLimit})
		return truncationNote(result.Truncated) + result.Detail, err
	})
}

func inspectComparisonRevisions(ctx context.Context, repo *gitbackend.Repository, target string) (string, string, error) {
	revision, err := repo.ResolveRevision(ctx, target)
	if err != nil {
		return "", "", err
	}
	if len(revision.ParentIDs) == 0 {
		return "", "", fmt.Errorf("revision %s has no parent to compare", revision.ShortID)
	}
	return revision.ParentIDs[0], revision.ID, nil
}

func runUnifiedComparison(m *Model, title string, kind gitbackend.DiffKind) tea.Cmd {
	query := gitbackend.DiffQuery{Kind: kind, Files: selectedInspectPath(m), OutputLimit: inspectOutputLimit}
	if kind == gitbackend.DiffRevision {
		query.Base = "HEAD"
	}
	return runDiffInspection(m, "Terminal comparison (unified): "+title, query)
}

func inspectCompareDWIM(m *Model, _ WorkflowCommand) tea.Cmd {
	row := m.rows[m.tree.Cursor()]
	switch row.kind {
	case rowStaged:
		return runUnifiedComparison(m, "staged", gitbackend.DiffIndex)
	case rowCommit:
		return inspectCompareCommit(m, WorkflowCommand{})
	default:
		return runUnifiedComparison(m, "unstaged", gitbackend.DiffWorktree)
	}
}
func inspectCompareUnstaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runUnifiedComparison(m, "unstaged", gitbackend.DiffWorktree)
}
func inspectCompareStaged(m *Model, _ WorkflowCommand) tea.Cmd {
	return runUnifiedComparison(m, "staged", gitbackend.DiffIndex)
}
func inspectCompareWorkingTree(m *Model, _ WorkflowCommand) tea.Cmd {
	return runUnifiedComparison(m, "working tree", gitbackend.DiffRevision)
}
func inspectCompareCommit(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectShowCommit(m, command)
}
func inspectCompareStash(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectShowStash(m, command)
}
func inspectCompareRange(m *Model, command WorkflowCommand) tea.Cmd {
	return inspectDiffRange(m, command)
}

func logQueryFromCommand(command WorkflowCommand) gitbackend.LogQuery {
	query := gitbackend.LogQuery{Limit: 128, Graph: true, Decorations: true, OutputLimit: inspectOutputLimit}
	if value, ok := inspectOption(command, "magit-log:-n"); ok && value.Value != "" {
		if n, err := strconv.Atoi(value.Value); err == nil && n > 0 {
			query.Limit = min(n, inspectItemLimit)
		}
	}
	if value, ok := inspectOption(command, "transient:magit-log-refresh:--graph"); ok {
		query.Graph = value.Enabled
	}
	if value, ok := inspectOption(command, "transient:magit-log-refresh:--decorate"); ok {
		query.Decorations = value.Enabled
	}
	if value, ok := inspectOption(command, "magit-log:--*-order"); ok {
		switch value.Value {
		case "date", "--date-order":
			query.Order = gitbackend.LogOrderDate
		case "author-date", "--author-date-order":
			query.Order = gitbackend.LogOrderAuthorDate
		case "topo", "--topo-order":
			query.Order = gitbackend.LogOrderTopo
		}
	}
	return query
}

func inspectOption(command WorkflowCommand, upstream string) (OptionValue, bool) {
	for _, binding := range keymap.Registry() {
		if binding.UpstreamCommand == upstream {
			if value, ok := command.Options[binding.Command]; ok {
				return value, true
			}
		}
	}
	return OptionValue{}, false
}

func runLogInspection(m *Model, title string, query gitbackend.LogQuery) tea.Cmd {
	return loadInspection(m, title, func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			line := strings.TrimSpace(item.Graph + " " + item.ShortID)
			if item.Decorations != "" {
				line += " (" + item.Decorations + ")"
			}
			lines = append(lines, line+" "+item.Subject)
		}
		return truncationNote(result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectLogCurrent(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	query.Revision = selectedInspectRevision(m)
	return runLogInspection(m, "Log", query)
}

func inspectLogAll(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	query.All = true
	return runLogInspection(m, "Log all refs", query)
}

func inspectLogBranches(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	return loadInspection(m, "Branch log", func(ctx context.Context) (string, error) {
		refs, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		for _, ref := range refs.Local {
			query.Revisions = append(query.Revisions, ref.FullName)
		}
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			lines = append(lines, item.ShortID+" "+item.Subject)
		}
		return truncationNote(refs.Truncated || result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectLogRelated(m *Model, command WorkflowCommand) tea.Cmd {
	query := logQueryFromCommand(command)
	focus := selectedInspectRevision(m)
	return loadInspection(m, "Related log", func(ctx context.Context) (string, error) {
		refs, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Focus: focus, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		if refs.Focus == nil || refs.Focus.Upstream == "" {
			return "", fmt.Errorf("current revision has no configured upstream")
		}
		query.From, query.To, query.Symmetric = refs.Focus.Upstream, focus, true
		result, err := m.repo.QueryLog(ctx, query)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			lines = append(lines, item.ShortID+" "+item.Subject)
		}
		return truncationNote(result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectRefs(m *Model, _ WorkflowCommand) tea.Cmd {
	focus := selectedInspectRevision(m)
	return loadInspection(m, "References", func(ctx context.Context) (string, error) {
		result, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Focus: focus, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		var lines []string
		appendRefs := func(label string, refs []gitbackend.Ref) {
			if len(refs) == 0 {
				return
			}
			lines = append(lines, label)
			for _, ref := range refs {
				mark := "  "
				if ref.Current {
					mark = "* "
				}
				lines = append(lines, mark+ref.Name+" "+shortInspectID(ref.ID)+" "+ref.Subject)
			}
		}
		appendRefs("Local branches", result.Local)
		appendRefs("Remote branches", result.Remote)
		appendRefs("Tags", result.Tags)
		return truncationNote(result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectCherries(m *Model, _ WorkflowCommand) tea.Cmd {
	head := selectedInspectRevision(m)
	return loadInspection(m, "Cherries", func(ctx context.Context) (string, error) {
		refs, err := m.repo.QueryRefs(ctx, gitbackend.RefQuery{Focus: head, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		upstream := ""
		if refs.Focus != nil {
			upstream = refs.Focus.Upstream
		}
		if upstream == "" {
			revision, resolveErr := m.repo.ResolveRevision(ctx, head)
			if resolveErr != nil {
				return "", resolveErr
			}
			if len(revision.ParentIDs) == 0 {
				return "", fmt.Errorf("current revision has no upstream or parent")
			}
			upstream = revision.ParentIDs[0]
		}
		result, err := m.repo.QueryCherry(ctx, gitbackend.CherryQuery{Upstream: upstream, Head: head, Limit: inspectItemLimit, OutputLimit: inspectOutputLimit})
		if err != nil {
			return "", err
		}
		var lines []string
		for _, item := range result.Items {
			mark := "+"
			if item.Equivalent {
				mark = "-"
			}
			lines = append(lines, mark+" "+shortInspectID(item.ID)+" "+item.Subject)
		}
		return truncationNote(result.Truncated) + strings.Join(lines, "\n"), nil
	})
}

func inspectSectionInfo(m *Model, _ WorkflowCommand) tea.Cmd {
	row, section := m.rows[m.tree.Cursor()], m.tree.Section(m.tree.Cursor())
	title, folded := "", false
	if section != nil {
		title, folded = section.Title(), m.tree.IsFolded(row.id)
	}
	return loadInspection(m, "Section information", func(ctx context.Context) (string, error) {
		state, err := m.repo.QueryOperationState(ctx)
		if err != nil {
			return "", err
		}
		lines := []string{fmt.Sprintf("Section: %s", row.section), fmt.Sprintf("Depth: %d", row.depth)}
		if title != "" {
			lines = append(lines, "Title: "+title, fmt.Sprintf("Folded: %t", folded))
		}
		if row.path != "" {
			lines = append(lines, "Path: "+row.path)
		}
		if row.commit.ID != "" {
			revision, resolveErr := m.repo.ResolveRevision(ctx, row.commit.ID)
			if resolveErr != nil {
				return "", resolveErr
			}
			lines = append(lines, "Revision: "+revision.ID, "Subject: "+revision.Subject, "Author: "+revision.AuthorName+" <"+revision.AuthorEmail+">")
		}
		if !state.InProgress() {
			lines = append(lines, "Repository operation: none")
		} else {
			lines = append(lines, fmt.Sprintf("Repository operations in progress: %d", len(state.Items)))
		}
		return strings.Join(lines, "\n"), nil
	})
}

func truncationNote(truncated bool) string {
	if truncated {
		return "[result truncated]\n"
	}
	return ""
}

func shortInspectID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
