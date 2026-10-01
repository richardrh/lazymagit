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
	selected, err := r.previewBranchTarget(ctx, target)
	if err != nil {
		return BranchSwitchPreview{}, err
	}
	selectedID := selected.ID
	if selectedID == "" {
		selectedID, err = r.resolveCommitOID(ctx, target)
		if err != nil {
			return BranchSwitchPreview{}, fmt.Errorf("resolve branch target: %w", err)
		}
	}
	preview, summary, err := r.previewBranchMetadata(ctx, selected, selectedID)
	if err != nil {
		return BranchSwitchPreview{}, err
	}
	if err := r.populateTargetUpstreamPreview(ctx, &preview, selected.Upstream); err != nil {
		return BranchSwitchPreview{}, err
	}
	if err := r.populateBranchSwitchHistory(ctx, &preview, summary); err != nil {
		return BranchSwitchPreview{}, err
	}
	if err := r.populateBranchSwitchConflicts(ctx, &preview); err != nil {
		return BranchSwitchPreview{}, err
	}
	return preview, nil
}

func (r *Repository) previewBranchTarget(ctx context.Context, target string) (Branch, error) {
	branches, err := r.Branches(ctx)
	if err != nil {
		return Branch{}, fmt.Errorf("list branches: %w", err)
	}
	for _, branch := range branches {
		if branch.Name == target {
			if !branch.Remote && branch.Upstream == "" {
				branch.Upstream = r.configuredBranchUpstream(ctx, target)
			}
			return branch, nil
		}
	}
	return Branch{Name: target}, nil
}

func (r *Repository) configuredBranchUpstream(ctx context.Context, target string) string {
	remote, remoteErr := r.output(ctx, "config", "--get", "branch."+target+".remote")
	merge, mergeErr := r.output(ctx, "config", "--get", "branch."+target+".merge")
	if remoteErr != nil || mergeErr != nil {
		return ""
	}
	return trimLine(remote) + "/" + strings.TrimPrefix(trimLine(merge), "refs/heads/")
}

func (r *Repository) previewBranchMetadata(ctx context.Context, selected Branch, selectedID string) (BranchSwitchPreview, Summary, error) {
	summary, err := r.Summary(ctx)
	if err != nil {
		return BranchSwitchPreview{}, Summary{}, fmt.Errorf("read repository summary: %w", err)
	}
	current := summary.Branch
	if current == "" || current == "(detached)" {
		current = "HEAD"
	}
	preview := BranchSwitchPreview{
		Target:         selected.Name,
		Current:        current,
		TargetUpstream: selected.Upstream,
		TargetID:       selectedID,
		TargetRemote:   selected.Remote,
	}
	if err := r.populatePreviewLatest(ctx, &preview, selectedID); err != nil {
		return BranchSwitchPreview{}, Summary{}, err
	}
	if err := r.populatePreviewDescription(ctx, &preview, selected); err != nil {
		return BranchSwitchPreview{}, Summary{}, err
	}
	if err := r.populatePreviewWorktree(ctx, &preview, selected.Name); err != nil {
		return BranchSwitchPreview{}, Summary{}, err
	}
	return preview, summary, nil
}

func (r *Repository) populatePreviewLatest(ctx context.Context, preview *BranchSwitchPreview, selectedID string) error {
	latest, err := r.QueryLog(ctx, LogQuery{Revision: selectedID, Limit: 1, OutputLimit: 1 << 20})
	if err != nil {
		return fmt.Errorf("read target commit: %w", err)
	}
	if len(latest.Items) > 0 {
		preview.Latest = latest.Items[0]
	}
	return nil
}

func (r *Repository) populatePreviewDescription(ctx context.Context, preview *BranchSwitchPreview, selected Branch) error {
	if selected.Remote {
		return nil
	}
	description, err := r.BranchDescription(ctx, selected.Name)
	if err == nil {
		preview.Description = description
		return nil
	}
	if isExitError(err) {
		return nil
	}
	return fmt.Errorf("read branch description: %w", err)
}

func (r *Repository) populatePreviewWorktree(ctx context.Context, preview *BranchSwitchPreview, selectedName string) error {
	worktrees, err := r.Worktrees(ctx)
	if err != nil {
		if isExitError(err) {
			return nil
		}
		return fmt.Errorf("read worktrees: %w", err)
	}
	for _, worktree := range worktrees {
		if worktree.Branch == selectedName && worktree.Path != r.WorkTree() {
			preview.OtherWorktree = worktree.Path
			break
		}
	}
	return nil
}

func (r *Repository) populateBranchSwitchHistory(ctx context.Context, preview *BranchSwitchPreview, summary Summary) error {
	currentID := summary.Head
	if currentID == "" || currentID == "(initial)" || summary.Unborn {
		var err error
		preview.GraphLines, preview.GraphTruncated, err = r.branchGraphLines(ctx, []string{preview.TargetID}, preview.TargetID, "", "")
		return err
	}
	currentID, err := r.populateBranchSwitchDivergence(ctx, preview, currentID)
	if err != nil {
		return err
	}
	return r.populateBranchSwitchBase(ctx, preview, currentID)
}

func (r *Repository) populateBranchSwitchDivergence(ctx context.Context, preview *BranchSwitchPreview, currentID string) (string, error) {
	currentID, err := r.resolveCommitOID(ctx, currentID)
	if err != nil {
		return "", fmt.Errorf("resolve current HEAD: %w", err)
	}
	preview.CurrentID = currentID
	counts, err := r.output(ctx, "rev-list", "--left-right", "--count", currentID+"..."+preview.TargetID)
	if err != nil {
		return "", fmt.Errorf("compare branches: %w", err)
	}
	preview.CurrentAhead, preview.TargetAhead, err = parseBranchCounts(counts)
	if err != nil {
		return "", err
	}
	return currentID, nil
}

func (r *Repository) populateBranchSwitchBase(ctx context.Context, preview *BranchSwitchPreview, currentID string) error {
	base, baseErr := r.output(ctx, "merge-base", currentID, preview.TargetID)
	if baseErr == nil {
		preview.BaseID = trimLine(base)
		if preview.BaseID != "" {
			var err error
			preview.ChangeSummary, err = r.branchSwitchDiff(ctx, preview.BaseID, preview.TargetID)
			if err != nil {
				return err
			}
		}
	} else if !isExitError(baseErr) {
		return fmt.Errorf("find shared base: %w", baseErr)
	} else {
		preview.NoCommonAncestor = true
	}
	var err error
	preview.GraphLines, preview.GraphTruncated, err = r.branchGraphLines(ctx, []string{currentID, preview.TargetID}, preview.TargetID, currentID, preview.BaseID)
	return err
}

func (r *Repository) populateBranchSwitchConflicts(ctx context.Context, preview *BranchSwitchPreview) error {
	if preview.CurrentID == "" {
		return nil
	}
	status, err := r.Status(ctx)
	if err != nil {
		if isExitError(err) {
			return nil
		}
		return fmt.Errorf("read checkout status: %w", err)
	}
	if len(status.Files) == 0 {
		return nil
	}
	changed, err := r.output(ctx, "diff", "--name-only", "--no-renames", "-z", preview.CurrentID, preview.TargetID, "--")
	if err != nil {
		return fmt.Errorf("find checkout conflicts: %w", err)
	}
	dirty, err := r.output(ctx, "diff", "--name-only", "--no-renames", "-z", preview.TargetID, "--")
	if err != nil {
		return fmt.Errorf("find local checkout changes: %w", err)
	}
	targetFiles, err := r.targetTreeFiles(ctx, preview.TargetID, status.Files)
	if err != nil {
		return err
	}
	preview.PotentialBlockingPaths = branchSwitchBlockingPaths(status.Files, nulPaths(changed), nulPaths(dirty), targetFiles)
	return nil
}

func (r *Repository) targetTreeFiles(ctx context.Context, targetID string, files []FileStatus) (map[string]bool, error) {
	needTargetTree := false
	for _, file := range files {
		if file.Unstaged == ChangeUntracked {
			needTargetTree = true
			break
		}
	}
	if !needTargetTree {
		return nil, nil
	}
	targetTree, err := r.output(ctx, "ls-tree", "-r", "--name-only", "-z", targetID, "--")
	if err != nil {
		return nil, fmt.Errorf("find target files: %w", err)
	}
	return nulPaths(targetTree), nil
}

func nulPaths(output []byte) map[string]bool {
	paths := make(map[string]bool)
	for _, path := range strings.Split(string(output), "\x00") {
		if path != "" {
			paths[path] = true
		}
	}
	return paths
}

func branchSwitchBlockingPaths(files []FileStatus, changedPaths, dirtyPaths, targetFiles map[string]bool) []string {
	var blocking []string
	for _, file := range files {
		if branchSwitchPathBlocked(file, changedPaths, dirtyPaths, targetFiles) {
			blocking = append(blocking, file.Path)
		}
	}
	return blocking
}

func branchSwitchPathBlocked(file FileStatus, changedPaths, dirtyPaths, targetFiles map[string]bool) bool {
	if file.Unstaged == ChangeUntracked {
		return pathTreeCollision(file.Path, targetFiles)
	}
	if file.Staged == ChangeNone && file.Unstaged == ChangeNone {
		return false
	}
	return changedPaths[file.Path] && dirtyPaths[file.Path] || changedPaths[file.OriginalPath] && dirtyPaths[file.OriginalPath]
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
