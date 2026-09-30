package git

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// BranchSwitchDiff summarizes changes introduced by a branch after its common
// ancestor with the current HEAD.
type BranchSwitchDiff struct {
	Files, Insertions, Deletions int
}

// BranchSwitchPreview is a read-only snapshot used by the branch switch view.
// Ahead/behind counts are deliberately split: CurrentAhead/TargetAhead compare
// the target with the current HEAD, while UpstreamAhead/UpstreamBehind compare
// the target with its configured upstream.
type BranchSwitchPreview struct {
	Target, Current, TargetUpstream  string
	TargetID, CurrentID, BaseID      string
	TargetRemote, NoCommonAncestor   bool
	Latest                           LogEntry
	CurrentAhead, TargetAhead        int
	UpstreamAhead, UpstreamBehind    int
	HasUpstream, UpstreamUnavailable bool
	ChangeSummary                    BranchSwitchDiff
	Description                      ConfiguredValue
	OtherWorktree                    string
	PotentialBlockingPaths           []string
	GraphLines                       []string
	GraphTruncated                   bool
}

// PreviewBranchSwitch gathers only the selected target/current histories. It
// does not mutate refs, worktrees, or configuration and never includes an
// all-refs graph.
func (r *Repository) PreviewBranchSwitch(ctx context.Context, target string) (BranchSwitchPreview, error) {
	if strings.TrimSpace(target) == "" {
		return BranchSwitchPreview{}, errors.New("branch target is empty")
	}
	branches, err := r.Branches(ctx)
	if err != nil {
		return BranchSwitchPreview{}, fmt.Errorf("list branches: %w", err)
	}
	var selected Branch
	found := false
	for _, branch := range branches {
		if branch.Name == target {
			selected, found = branch, true
			break
		}
	}
	selectedName := target
	if !found {
		selected = Branch{Name: target}
	} else if !selected.Remote && selected.Upstream == "" {
		remote, remoteErr := r.output(ctx, "config", "--get", "branch."+target+".remote")
		merge, mergeErr := r.output(ctx, "config", "--get", "branch."+target+".merge")
		if remoteErr == nil && mergeErr == nil {
			selected.Upstream = trimLine(remote) + "/" + strings.TrimPrefix(trimLine(merge), "refs/heads/")
		}
	}
	selectedID := selected.ID
	if selectedID == "" {
		selectedID, err = r.resolveCommitOID(ctx, target)
		if err != nil {
			return BranchSwitchPreview{}, fmt.Errorf("resolve branch target: %w", err)
		}
	}
	summary, err := r.Summary(ctx)
	if err != nil {
		return BranchSwitchPreview{}, fmt.Errorf("read repository summary: %w", err)
	}
	currentID := summary.Head
	preview := BranchSwitchPreview{
		Target:         selectedName,
		Current:        summary.Branch,
		TargetUpstream: selected.Upstream,
		TargetID:       selectedID,
		TargetRemote:   selected.Remote,
	}
	if preview.Current == "" || preview.Current == "(detached)" {
		preview.Current = "HEAD"
	}
	latest, err := r.QueryLog(ctx, LogQuery{Revision: selectedID, Limit: 1, OutputLimit: 1 << 20})
	if err != nil {
		return BranchSwitchPreview{}, fmt.Errorf("read target commit: %w", err)
	}
	if len(latest.Items) > 0 {
		preview.Latest = latest.Items[0]
	}
	if !selected.Remote {
		if description, descErr := r.BranchDescription(ctx, selectedName); descErr == nil {
			preview.Description = description
		} else if !isExitError(descErr) {
			return BranchSwitchPreview{}, fmt.Errorf("read branch description: %w", descErr)
		}
	}
	if worktrees, wtErr := r.Worktrees(ctx); wtErr == nil {
		for _, worktree := range worktrees {
			if worktree.Branch == selectedName && worktree.Path != r.WorkTree() {
				preview.OtherWorktree = worktree.Path
				break
			}
		}
	} else if !isExitError(wtErr) {
		return BranchSwitchPreview{}, fmt.Errorf("read worktrees: %w", wtErr)
	}
	if err := r.populateTargetUpstreamPreview(ctx, &preview, selected.Upstream); err != nil {
		return BranchSwitchPreview{}, err
	}
	if currentID == "" || currentID == "(initial)" || summary.Unborn {
		var graphErr error
		preview.GraphLines, preview.GraphTruncated, graphErr = r.branchGraphLines(ctx, []string{selectedID}, selectedID, "", "")
		if graphErr != nil {
			return BranchSwitchPreview{}, graphErr
		}
		return preview, nil
	}
	currentID, err = r.resolveCommitOID(ctx, currentID)
	if err != nil {
		return BranchSwitchPreview{}, fmt.Errorf("resolve current HEAD: %w", err)
	}
	preview.CurrentID = currentID
	counts, err := r.output(ctx, "rev-list", "--left-right", "--count", currentID+"..."+selectedID)
	if err != nil {
		return BranchSwitchPreview{}, fmt.Errorf("compare branches: %w", err)
	}
	preview.CurrentAhead, preview.TargetAhead, err = parseBranchCounts(counts)
	if err != nil {
		return BranchSwitchPreview{}, err
	}
	base, baseErr := r.output(ctx, "merge-base", currentID, selectedID)
	if baseErr == nil {
		preview.BaseID = trimLine(base)
		if preview.BaseID != "" {
			preview.ChangeSummary, err = r.branchSwitchDiff(ctx, preview.BaseID, selectedID)
			if err != nil {
				return BranchSwitchPreview{}, err
			}
		}
	} else if !isExitError(baseErr) {
		return BranchSwitchPreview{}, fmt.Errorf("find shared base: %w", baseErr)
	} else {
		preview.NoCommonAncestor = true
	}
	preview.GraphLines, preview.GraphTruncated, err = r.branchGraphLines(ctx, []string{currentID, selectedID}, selectedID, currentID, preview.BaseID)
	if err != nil {
		return BranchSwitchPreview{}, err
	}
	if status, statusErr := r.Status(ctx); statusErr == nil && len(status.Files) > 0 && currentID != "" {
		changed, changedErr := r.output(ctx, "diff", "--name-only", "--no-renames", "-z", currentID, selectedID, "--")
		if changedErr != nil {
			return BranchSwitchPreview{}, fmt.Errorf("find checkout conflicts: %w", changedErr)
		}
		changedPaths := make(map[string]bool)
		for _, path := range strings.Split(string(changed), "\x00") {
			if path != "" {
				changedPaths[path] = true
			}
		}
		dirty, dirtyErr := r.output(ctx, "diff", "--name-only", "--no-renames", "-z", selectedID, "--")
		if dirtyErr != nil {
			return BranchSwitchPreview{}, fmt.Errorf("find local checkout changes: %w", dirtyErr)
		}
		dirtyPaths := make(map[string]bool)
		for _, path := range strings.Split(string(dirty), "\x00") {
			if path != "" {
				dirtyPaths[path] = true
			}
		}
		needTargetTree := false
		for _, file := range status.Files {
			if file.Unstaged == ChangeUntracked {
				needTargetTree = true
				break
			}
		}
		targetFiles := make(map[string]bool)
		if needTargetTree {
			targetTree, treeErr := r.output(ctx, "ls-tree", "-r", "--name-only", "-z", selectedID, "--")
			if treeErr != nil {
				return BranchSwitchPreview{}, fmt.Errorf("find target files: %w", treeErr)
			}
			for _, path := range strings.Split(string(targetTree), "\x00") {
				if path != "" {
					targetFiles[path] = true
				}
			}
		}
		for _, file := range status.Files {
			if file.Unstaged == ChangeUntracked {
				if pathTreeCollision(file.Path, targetFiles) {
					preview.PotentialBlockingPaths = append(preview.PotentialBlockingPaths, file.Path)
				}
				continue
			}
			if file.Staged == ChangeNone && file.Unstaged == ChangeNone {
				continue
			}
			if changedPaths[file.Path] && dirtyPaths[file.Path] || changedPaths[file.OriginalPath] && dirtyPaths[file.OriginalPath] {
				preview.PotentialBlockingPaths = append(preview.PotentialBlockingPaths, file.Path)
			}
		}
	} else if statusErr != nil && !isExitError(statusErr) {
		return BranchSwitchPreview{}, fmt.Errorf("read checkout status: %w", statusErr)
	}

	return preview, nil
}

func (r *Repository) branchGraphLines(ctx context.Context, revisions []string, targetID, currentID, baseID string) ([]string, bool, error) {
	args := []string{"--no-pager", "log", "--no-color", "--graph", "--max-count=13", "--format=%H%x00%s"}
	args = append(args, revisions...)
	out, err := r.output(ctx, args...)
	if err != nil {
		return nil, false, fmt.Errorf("read branch graph: %w", err)
	}
	raw := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	lines := make([]string, 0, min(24, len(raw)))
	commits := 0
	truncated := false
	for _, line := range raw {
		marker := strings.IndexByte(line, 0)
		if marker < 0 {
			lines = append(lines, line)
			continue
		}
		commits++
		if commits > 12 {
			truncated = true
			break
		}
		prefix, subject := line[:marker], line[marker+1:]
		fields := strings.Fields(prefix)
		if len(fields) == 0 {
			continue
		}
		fullOID := fields[len(fields)-1]
		label := ""
		if fullOID == targetID {
			label += " [target]"
		}
		if fullOID == currentID {
			label += " [current]"
		}
		if fullOID == baseID {
			label += " [base]"
		}
		short := fullOID
		if len(short) > 8 {
			short = short[:8]
		}
		lane := prefix
		if at := strings.LastIndex(prefix, fullOID); at >= 0 {
			lane = prefix[:at]
		}
		lines = append(lines, strings.TrimRight(lane, " \t")+" "+short+label+" "+subject)
	}
	return lines, truncated, nil
}

func (r *Repository) populateTargetUpstreamPreview(ctx context.Context, preview *BranchSwitchPreview, upstream string) error {
	if upstream == "" {
		return nil
	}
	preview.HasUpstream = true
	upstreamID, err := r.resolveCommitOID(ctx, upstream)
	if err != nil {
		if isExitError(err) {
			preview.HasUpstream = false
			preview.UpstreamUnavailable = true
			return nil
		}
		return fmt.Errorf("resolve target upstream: %w", err)
	}
	counts, err := r.output(ctx, "rev-list", "--left-right", "--count", preview.TargetID+"..."+upstreamID)
	if err != nil {
		return fmt.Errorf("compare target upstream: %w", err)
	}
	preview.UpstreamAhead, preview.UpstreamBehind, err = parseBranchCounts(counts)
	return err
}

func pathTreeCollision(path string, targetFiles map[string]bool) bool {
	if targetFiles[path] {
		return true
	}
	prefix := path + "/"
	for target := range targetFiles {
		if strings.HasPrefix(target, prefix) {
			return true
		}
	}
	return false
}

func parseBranchCounts(output []byte) (int, int, error) {
	parts := strings.Fields(string(output))
	if len(parts) != 2 {
		return 0, 0, errors.New("malformed branch divergence counts")
	}
	left, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse branch divergence count: %w", err)
	}
	right, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse branch divergence count: %w", err)
	}
	return left, right, nil
}

func (r *Repository) branchSwitchDiff(ctx context.Context, base, target string) (BranchSwitchDiff, error) {
	out, err := r.output(ctx, "diff", "--numstat", "--no-renames", base, target, "--")
	if err != nil {
		return BranchSwitchDiff{}, fmt.Errorf("summarize target changes: %w", err)
	}
	var summary BranchSwitchDiff
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		summary.Files++
		if parts[0] != "-" {
			summary.Insertions += atoiOrZero(parts[0])
		}
		if parts[1] != "-" {
			summary.Deletions += atoiOrZero(parts[1])
		}
	}
	return summary, nil
}

func atoiOrZero(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
