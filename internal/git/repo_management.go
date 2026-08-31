package git

// This file contains repository-management operations which intentionally sit
// below the UI.  Every Git invocation is an argv invocation; paths are passed
// after `--` wherever the underlying command supports it.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrDestructiveConfirmationRequired = ErrConfirmationRequired
	ErrUnsafeDestination               = errors.New("unsafe destination")
	ErrDirtyWorktree                   = errors.New("worktree has uncommitted changes")
	ErrPrimaryWorktree                 = errors.New("operation is not allowed on the primary worktree")
	ErrLockedWorktree                  = errors.New("worktree is locked")
	ErrSubtreeUnavailable              = errors.New("git subtree is unavailable")
)

// ConfirmedForce cannot accidentally become true through an omitted bool
// field. Destructive APIs require the named constant Confirmed.
type ConfirmedForce uint8

const (
	NotConfirmed ConfirmedForce = iota
	Confirmed
)

// GitCommandResult is the bounded process capture used by command prompts.
// Shell syntax is never interpreted. Use an explicitly unsafe integration
// boundary outside this package if a shell is really required.
type GitCommandResult struct {
	Stdout, Stderr                   string
	ExitCode                         int
	StdoutTruncated, StderrTruncated bool
	Duration                         time.Duration
}

// UnsafeRunGitCommand executes raw Git argv in this repository. Args must not
// contain the executable name. A non-zero Git exit is returned both in Result
// and as *CommandError.
func (r *Repository) UnsafeRunGitCommand(ctx context.Context, capability AllowUnsafeExecution, args []string) (GitCommandResult, error) {
	if !capability.allowed() {
		return GitCommandResult{}, ErrUnsafeExecution
	}
	if len(args) == 0 {
		return GitCommandResult{}, errors.New("git arguments are empty")
	}
	return executeRecordedGit(ctx, r.commandDir, nil, args)
}

func executeRecordedGit(ctx context.Context, dir string, input []byte, args []string) (GitCommandResult, error) {
	started := time.Now()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = gitCommandEnv()
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	stdout, stderr := new(headTailCapture), new(headTailCapture)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	duration := time.Since(started)
	recordedArgs, sensitive := redactManagementArgs(args)
	recordedOut, outRedactionTruncated := redactCaptured(stdout.String(), sensitive)
	recordedErr, errRedactionTruncated := redactCaptured(stderr.String(), sensitive)
	result := GitCommandResult{
		Stdout: recordedOut, Stderr: recordedErr, ExitCode: processExitCode(ctx, err), Duration: duration,
		StdoutTruncated: stdout.truncated || outRedactionTruncated,
		StderrTruncated: stderr.truncated || errRedactionTruncated,
	}
	if recorder, ok := ctx.Value(processRecorderKey{}).(func(ProcessRecord)); ok {
		recorder(ProcessRecord{Dir: dir, Args: append([]string(nil), recordedArgs...), Started: started,
			Duration: duration, ExitCode: result.ExitCode, Stdout: recordedOut, Stderr: recordedErr,
			StdoutTruncated: result.StdoutTruncated, StderrTruncated: result.StderrTruncated})
	}
	if err != nil {
		return result, &CommandError{Args: recordedArgs, Err: err, Stderr: strings.TrimSpace(recordedErr), StderrTruncated: result.StderrTruncated}
	}
	return result, nil
}

func redactManagementArgs(args []string) ([]string, []string) {
	redacted, sensitive := redactMutationArgs(args)
	for i, arg := range args {
		if credentialBearingURL(arg) {
			sensitive = appendSensitive(sensitive, arg)
			redacted[i] = redactionMarker
		}
		if (arg == "-m" || arg == "--message") && i+1 < len(args) {
			sensitive = appendSensitive(sensitive, args[i+1])
			redacted[i+1] = redactionMarker
		}
		if strings.HasPrefix(arg, "--message=") {
			value := strings.TrimPrefix(arg, "--message=")
			sensitive = appendSensitive(sensitive, value)
			redacted[i] = "--message=" + redactionMarker
		}
	}
	return redacted, sensitive
}

func (r *Repository) managementRun(ctx context.Context, args ...string) error {
	_, err := executeRecordedGit(ctx, r.commandDir, nil, args)
	return err
}

func managementOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitCommandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, &CommandError{Args: append([]string(nil), args...), Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.Bytes(), nil
}

func validateRepoRelative(path string, allowDot bool) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("repository path %q is not relative", path)
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if (!allowDot && clean == ".") || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repository path %q escapes the worktree", path)
	}
	return clean, nil
}

func (r *Repository) requireWorkTree() error {
	if r.workTree == "" {
		return errors.New("operation requires a worktree")
	}
	return nil
}

// IgnoreTarget selects which ignore file receives a literal rule.
type IgnoreTarget uint8

const (
	IgnoreTopLevel IgnoreTarget = iota
	IgnoreSubdirectory
	IgnoreRepositoryExclude
	IgnoreGlobalExclude
)

// AddIgnoreRule appends one rule (embedded newlines are rejected). For a
// shared .gitignore, directory is repository-relative and the modified ignore
// file is staged. directory is ignored for non-subdirectory targets.
func (r *Repository) AddIgnoreRule(ctx context.Context, rule, directory string, target IgnoreTarget) (string, error) {
	if err := r.requireWorkTree(); err != nil {
		return "", err
	}
	if rule == "" || strings.ContainsAny(rule, "\r\n") {
		return "", errors.New("ignore rule must be one non-empty line")
	}
	path, stage, err := r.ignoreRulePath(ctx, directory, target)
	if err != nil {
		return "", err
	}
	if stage {
		if err := ensurePathParentWithin(r.workTree, path); err != nil {
			return "", err
		}
	}
	if err := appendRule(path, rule); err != nil {
		return "", err
	}
	if err := r.stageIgnoreRuleIfNeeded(ctx, path, rule, stage); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Repository) ignoreRulePath(ctx context.Context, directory string, target IgnoreTarget) (string, bool, error) {
	switch target {
	case IgnoreTopLevel:
		return filepath.Join(r.workTree, ".gitignore"), true, nil
	case IgnoreSubdirectory:
		dir, err := validateRepoRelative(directory, true)
		if err != nil {
			return "", false, err
		}
		return filepath.Join(r.workTree, dir, ".gitignore"), true, nil
	case IgnoreRepositoryExclude:
		return filepath.Join(r.gitDir, "info", "exclude"), false, nil
	case IgnoreGlobalExclude:
		configured, ok, err := r.configValue(ctx, "core.excludesFile")
		if err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, errors.New("core.excludesFile is not configured")
		}
		path, err := expandUserPath(configured)
		return path, false, err
	default:
		return "", false, errors.New("unknown ignore target")
	}
}

func (r *Repository) stageIgnoreRuleIfNeeded(ctx context.Context, path, rule string, stage bool) error {
	if !stage {
		return nil
	}
	rel, err := filepath.Rel(r.workTree, path)
	if err != nil {
		return err
	}
	return r.stageAppendedIgnoreRule(ctx, filepath.ToSlash(rel), rule)
}

// stageAppendedIgnoreRule updates only the index version of the appended rule;
// unrelated edits already present in the worktree remain unstaged.
func (r *Repository) stageAppendedIgnoreRule(ctx context.Context, rel, rule string) error {
	base, err := r.output(ctx, "show", ":"+rel)
	if err != nil && commandExitCode(err) != 128 {
		return err
	}
	if err != nil {
		base = nil
	}
	if len(base) > 0 && base[len(base)-1] != '\n' {
		base = append(base, '\n')
	}
	base = append(base, rule...)
	base = append(base, '\n')
	oid, err := r.runMutationOutput(ctx, base, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	return r.managementRun(ctx, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(oid)+","+rel)
}

func expandUserPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("global excludes path %q is not absolute", path)
	}
	return filepath.Clean(path), nil
}

func appendRule(path, rule string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, info.Size()-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	_, err = f.WriteString(rule + "\n")
	return err
}

// ensurePathParentWithin prevents a lexical repository-relative path from
// escaping through an existing directory symlink. Nonexistent trailing
// directories are safe because they will be created below the resolved
// existing ancestor.
func ensurePathParentWithin(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	ancestor := filepath.Dir(path)
	for {
		resolved, resolveErr := filepath.EvalSymlinks(ancestor)
		if resolveErr == nil {
			rel, relErr := filepath.Rel(resolvedRoot, resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%w: path escapes the worktree through a symlink", ErrUnsafeDestination)
			}
			return nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return resolveErr
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return fmt.Errorf("%w: cannot resolve destination parent", ErrUnsafeDestination)
		}
		ancestor = next
	}
}

type IndexFlag uint8

const (
	SkipWorktree IndexFlag = iota
	AssumeUnchanged
)

func (r *Repository) SetIndexFlag(ctx context.Context, flag IndexFlag, paths []string, set bool) error {
	if len(paths) == 0 {
		return nil
	}
	option := ""
	switch flag {
	case SkipWorktree:
		if set {
			option = "--skip-worktree"
		} else {
			option = "--no-skip-worktree"
		}
	case AssumeUnchanged:
		if set {
			option = "--assume-unchanged"
		} else {
			option = "--no-assume-unchanged"
		}
	default:
		return errors.New("unknown index flag")
	}
	return r.managementRun(ctx, pathsArgs([]string{"update-index", option}, paths)...)
}

func (r *Repository) ListIndexFlag(ctx context.Context, flag IndexFlag) ([]string, error) {
	out, err := r.output(ctx, "ls-files", "-v", "-z")
	if err != nil {
		return nil, err
	}
	var result []string
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) < 3 || record[1] != ' ' {
			continue
		}
		tag := record[0]
		if flag == SkipWorktree && (tag == 'S' || tag == 's') || flag == AssumeUnchanged && tag >= 'a' && tag <= 'z' {
			result = append(result, string(record[2:]))
		}
	}
	if flag != SkipWorktree && flag != AssumeUnchanged {
		return nil, errors.New("unknown index flag")
	}
	return result, nil
}

// Untrack removes paths from the index while preserving all worktree files.
func (r *Repository) Untrack(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	// -f applies only to the index safety check. --cached guarantees that even
	// staged content which differs from both HEAD and disk remains on disk.
	return r.managementRun(ctx, pathsArgs([]string{"rm", "--cached", "-r", "-f"}, paths)...)
}

// RenamePath renames a tracked or untracked path without overwriting a
// destination. Tracked paths use git mv; purely untracked paths use os.Rename.
func (r *Repository) RenamePath(ctx context.Context, source, destination string) error {
	if err := r.requireWorkTree(); err != nil {
		return err
	}
	src, err := validateRepoRelative(source, false)
	if err != nil {
		return err
	}
	dst, err := validateRepoRelative(destination, false)
	if err != nil {
		return err
	}
	srcFull, dstFull := filepath.Join(r.workTree, src), filepath.Join(r.workTree, dst)
	if err := ensurePathParentWithin(r.workTree, srcFull); err != nil {
		return err
	}
	if err := ensurePathParentWithin(r.workTree, dstFull); err != nil {
		return err
	}
	if _, err := os.Lstat(srcFull); err != nil {
		return fmt.Errorf("inspect rename source: %w", err)
	}
	if _, err := os.Lstat(dstFull); err == nil {
		return fmt.Errorf("%w: destination already exists", ErrUnsafeDestination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tracked, err := r.output(ctx, "ls-files", "-z", "--", filepath.ToSlash(src))
	if err != nil {
		return err
	}
	if len(tracked) > 0 {
		return r.managementRun(ctx, "mv", "--", filepath.ToSlash(src), filepath.ToSlash(dst))
	}
	if err := os.MkdirAll(filepath.Dir(dstFull), 0o755); err != nil {
		return err
	}
	return os.Rename(srcFull, dstFull)
}

type Worktree struct {
	Path, HEAD, Branch, LockReason, PruneReason string
	Bare, Detached, Locked, Prunable, Primary   bool
}

func (r *Repository) Worktrees(ctx context.Context) ([]Worktree, error) {
	out, err := r.output(ctx, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var result []Worktree
	var current *Worktree
	for _, raw := range bytes.Split(out, []byte{0}) {
		line := string(raw)
		if line == "" {
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		if key == "worktree" {
			result = append(result, Worktree{Path: value})
			current = &result[len(result)-1]
			continue
		}
		if current == nil {
			return nil, errors.New("malformed worktree listing")
		}
		switch key {
		case "HEAD":
			current.HEAD = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			current.Bare = true
		case "detached":
			current.Detached = true
		case "locked":
			current.Locked, current.LockReason = true, value
		case "prunable":
			current.Prunable, current.PruneReason = true, value
		}
	}
	for i := range result {
		// Git documents the first entry as the main worktree. This remains
		// correct when Repository was discovered from a linked worktree.
		result[i].Primary = i == 0
	}
	return result, nil
}

func samePath(a, b string) bool {
	aa, ea := filepath.Abs(a)
	bb, eb := filepath.Abs(b)
	if ea != nil || eb != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(aa); err == nil {
		aa = resolved
	}
	if resolved, err := filepath.EvalSymlinks(bb); err == nil {
		bb = resolved
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

type WorktreeAddOptions struct {
	Detach     bool
	Force      ConfirmedForce
	Checkout   *bool
	Lock       bool
	LockReason string
}

func (r *Repository) AddWorktree(ctx context.Context, path, revision string, opts WorktreeAddOptions) error {
	if err := safeNewDirectory(path); err != nil {
		return err
	}
	args := []string{"worktree", "add"}
	if opts.Detach {
		args = append(args, "--detach")
	}
	if opts.Force == Confirmed {
		args = append(args, "--force")
	}
	if opts.Checkout != nil {
		if *opts.Checkout {
			args = append(args, "--checkout")
		} else {
			args = append(args, "--no-checkout")
		}
	}
	if opts.Lock {
		args = append(args, "--lock")
		if opts.LockReason != "" {
			args = append(args, "--reason", opts.LockReason)
		}
	}
	args = append(args, "--", path)
	if revision != "" {
		args = append(args, revision)
	}
	return r.managementRun(ctx, args...)
}

func (r *Repository) AddWorktreeWithBranch(ctx context.Context, path, branch, startPoint string, force ConfirmedForce) error {
	if branch == "" {
		return errors.New("branch name is empty")
	}
	if err := safeNewDirectory(path); err != nil {
		return err
	}
	args := []string{"worktree", "add", "-b", branch}
	if force == Confirmed {
		args = append(args, "--force")
	}
	args = append(args, "--", path)
	if startPoint != "" {
		args = append(args, startPoint)
	}
	return r.managementRun(ctx, args...)
}

func safeNewDirectory(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: destination is empty", ErrUnsafeDestination)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if abs == string(filepath.Separator) {
		return fmt.Errorf("%w: destination is filesystem root", ErrUnsafeDestination)
	}
	if err := validateNewDirectoryDestination(abs); err != nil {
		return err
	}
	return validateNewDirectoryParent(filepath.Dir(abs))
}

func validateNewDirectoryDestination(abs string) error {
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: destination is a symbolic link", ErrUnsafeDestination)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info, err := os.Stat(abs); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%w: destination exists and is not a directory", ErrUnsafeDestination)
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("%w: destination is not empty", ErrUnsafeDestination)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func validateNewDirectoryParent(parent string) error {
	for {
		if info, err := os.Stat(parent); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%w: parent is not a directory", ErrUnsafeDestination)
			}
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("%w: no existing parent", ErrUnsafeDestination)
		}
		parent = next
	}
	return nil
}

func (r *Repository) worktreeByPath(ctx context.Context, path string) (Worktree, error) {
	all, err := r.Worktrees(ctx)
	if err != nil {
		return Worktree{}, err
	}
	for _, wt := range all {
		if samePath(wt.Path, path) {
			return wt, nil
		}
	}
	return Worktree{}, fmt.Errorf("worktree %q is not registered", path)
}

func worktreeDirty(ctx context.Context, path string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	cmd.Env = gitCommandEnv()
	stdout := &limitedCapture{remaining: 1}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	if err := cmd.Run(); err != nil {
		return false, &CommandError{Args: []string{"status"}, Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.buf.Len() != 0 || stdout.truncated, nil
}

func (r *Repository) MoveWorktree(ctx context.Context, source, destination string, force ConfirmedForce) error {
	wt, err := r.worktreeByPath(ctx, source)
	if err != nil {
		return err
	}
	if err := validateWorktreeMove(ctx, wt, force); err != nil {
		return err
	}
	if err := safeNewDirectory(destination); err != nil {
		return err
	}
	return r.managementRun(ctx, worktreeMoveArgs(source, destination, force, wt.Locked)...)
}

func validateWorktreeMove(ctx context.Context, wt Worktree, force ConfirmedForce) error {
	if wt.Primary {
		return ErrPrimaryWorktree
	}
	if wt.Locked && force != Confirmed {
		return fmt.Errorf("%w: %s", ErrLockedWorktree, wt.LockReason)
	}
	dirty, err := worktreeDirty(ctx, wt.Path)
	if err != nil {
		return err
	}
	if dirty && force != Confirmed {
		return ErrDirtyWorktree
	}
	return nil
}

func worktreeMoveArgs(source, destination string, force ConfirmedForce, locked bool) []string {
	args := []string{"worktree", "move"}
	if force == Confirmed {
		args = append(args, "--force")
	}
	if force == Confirmed && locked {
		args = append(args, "--force")
	}
	return append(args, "--", source, destination)
}

func (r *Repository) RemoveWorktree(ctx context.Context, path string, force ConfirmedForce) error {
	wt, err := r.worktreeByPath(ctx, path)
	if err != nil {
		return err
	}
	if wt.Primary {
		return ErrPrimaryWorktree
	}
	if wt.Locked && force != Confirmed {
		return fmt.Errorf("%w: %s", ErrLockedWorktree, wt.LockReason)
	}
	dirty, err := worktreeDirty(ctx, wt.Path)
	if err != nil {
		return err
	}
	if dirty && force != Confirmed {
		return ErrDirtyWorktree
	}
	args := []string{"worktree", "remove"}
	if force == Confirmed {
		args = append(args, "--force")
		if wt.Locked {
			args = append(args, "--force")
		}
	}
	args = append(args, "--", path)
	return r.managementRun(ctx, args...)
}

type WorktreePruneOptions struct {
	DryRun, Verbose bool
	Expire          string
	Force           ConfirmedForce
	Plan            *WorktreePrunePlan
	Token           ConfirmationToken
}

type WorktreePrunePlan struct {
	Expire string
	Output string
	Token  ConfirmationToken
}

func (r *Repository) WorktreePrunePreflight(ctx context.Context, expire string) (WorktreePrunePlan, error) {
	if strings.ContainsAny(expire, "\x00\r\n") || strings.HasPrefix(expire, "-") {
		return WorktreePrunePlan{}, errors.New("invalid worktree prune expiration")
	}
	args := []string{"worktree", "prune", "--dry-run", "--verbose"}
	if expire != "" {
		args = append(args, "--expire", expire)
	}
	out, truncated, err := r.outputLimited(ctx, 1<<20, args...)
	if err != nil {
		return WorktreePrunePlan{}, err
	}
	if truncated {
		return WorktreePrunePlan{}, &TooLargeError{Resource: "worktree prune plan"}
	}
	p := WorktreePrunePlan{Expire: expire, Output: string(out)}
	p.Token = NewConfirmationToken(expire + "\x00" + p.Output)
	return p, nil
}

func (r *Repository) PruneWorktrees(ctx context.Context, opts WorktreePruneOptions) error {
	expire := opts.Expire
	if opts.Force == Confirmed && expire == "" {
		expire = "now"
	}
	if !opts.DryRun && expire != "" {
		if opts.Plan == nil || opts.Plan.Expire != expire {
			return &ConfirmationRequiredError{Operation: "prune worktrees with explicit expiration", Identity: expire}
		}
		current, err := r.WorktreePrunePreflight(ctx, expire)
		if err != nil {
			return err
		}
		identity := expire + "\x00" + current.Output
		if current.Output != opts.Plan.Output || !opts.Token.validFor(identity) {
			return ErrStalePlan
		}
	}
	args := []string{"worktree", "prune"}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	if expire != "" {
		args = append(args, "--expire", expire)
	}
	return r.managementRun(ctx, args...)
}

type SparseCheckoutState struct {
	Enabled, Cone bool
	Patterns      []string
}

func (r *Repository) SparseCheckoutState(ctx context.Context) (SparseCheckoutState, error) {
	var state SparseCheckoutState
	v, ok, err := r.configValue(ctx, "core.sparseCheckout")
	if err != nil {
		return state, err
	}
	state.Enabled = ok && strings.EqualFold(v, "true")
	v, ok, err = r.configValue(ctx, "core.sparseCheckoutCone")
	if err != nil {
		return state, err
	}
	state.Cone = ok && strings.EqualFold(v, "true")
	if !state.Enabled {
		return state, nil
	}
	path := filepath.Join(r.gitDir, "info", "sparse-checkout")
	b, err := readFileLimited(path, 4<<20)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.Patterns = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(state.Patterns) > 100000 {
		return SparseCheckoutState{}, &TooLargeError{Resource: "sparse-checkout pattern count"}
	}
	if len(state.Patterns) == 1 && state.Patterns[0] == "" {
		state.Patterns = nil
	}
	return state, nil
}

func (r *Repository) EnableSparseCheckoutCone(ctx context.Context) error {
	return r.managementRun(ctx, "sparse-checkout", "init", "--cone")
}
func (r *Repository) DisableSparseCheckout(ctx context.Context) error {
	return r.managementRun(ctx, "sparse-checkout", "disable")
}
func (r *Repository) ReapplySparseCheckout(ctx context.Context) error {
	return r.managementRun(ctx, "sparse-checkout", "reapply")
}
func (r *Repository) SetSparseCheckout(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return errors.New("sparse checkout paths are empty")
	}
	return r.managementRun(ctx, append([]string{"sparse-checkout", "set", "--"}, paths...)...)
}
func (r *Repository) AddSparseCheckout(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return r.managementRun(ctx, append([]string{"sparse-checkout", "add", "--"}, paths...)...)
}

type Submodule struct {
	Name, Path, URL, Commit string
	Initialized             bool
}

func (r *Repository) Submodules(ctx context.Context) ([]Submodule, error) {
	out, err := r.output(ctx, "config", "-z", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		if commandExitCode(err) == 1 {
			return nil, nil
		}
		return nil, err
	}
	var result []Submodule
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		key, path, ok := strings.Cut(string(record), "\n")
		if !ok {
			key, path, ok = strings.Cut(string(record), " ")
		}
		if !ok {
			return nil, errors.New("malformed .gitmodules output")
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "submodule."), ".path")
		urlOut, _ := r.output(ctx, "config", "--file", ".gitmodules", "--get", "submodule."+name+".url")
		commitOut, commitErr := r.output(ctx, "rev-parse", "--verify", ":"+path)
		result = append(result, Submodule{Name: name, Path: path, URL: trimLine(urlOut), Commit: trimLine(commitOut), Initialized: commitErr == nil && isDirectory(filepath.Join(r.workTree, filepath.FromSlash(path), ".git"))})
	}
	return result, nil
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir() || err == nil && info.Mode().IsRegular()
}

type SubmoduleAddOptions struct {
	Name, Branch string
	Depth        int
	Force        bool
}

func (r *Repository) AddSubmodule(ctx context.Context, repositoryURL, path string, opts SubmoduleAddOptions) error {
	if repositoryURL == "" || path == "" {
		return errors.New("submodule URL and path are required")
	}
	args := []string{"submodule", "add"}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	if opts.Depth < 0 {
		return errors.New("submodule depth cannot be negative")
	}
	if opts.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(opts.Depth))
	}
	if opts.Force {
		args = append(args, "--force")
	}
	args = append(args, "--", repositoryURL, path)
	return r.managementRun(ctx, args...)
}
func (r *Repository) InitSubmodules(ctx context.Context, paths []string) error {
	return r.submodulePaths(ctx, []string{"submodule", "init"}, paths)
}

type SubmoduleUpdateOptions struct {
	Init, Recursive, Remote, Rebase, Merge, Checkout, NoFetch bool
	Force                                                     ConfirmedForce
	Depth, Jobs                                               int
}

func (r *Repository) UpdateSubmodules(ctx context.Context, paths []string, o SubmoduleUpdateOptions) error {
	if err := validateSubmoduleUpdateOptions(o); err != nil {
		return err
	}
	return r.submodulePaths(ctx, submoduleUpdateArgs(o), paths)
}

func validateSubmoduleUpdateOptions(o SubmoduleUpdateOptions) error {
	if o.Depth < 0 || o.Jobs < 0 {
		return errors.New("depth and jobs cannot be negative")
	}
	modes := boolCount(o.Rebase, o.Merge, o.Checkout)
	if modes > 1 {
		return errors.New("submodule update modes are mutually exclusive")
	}
	return nil
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func submoduleUpdateArgs(o SubmoduleUpdateOptions) []string {
	args := []string{"submodule", "update"}
	if o.Init {
		args = append(args, "--init")
	}
	if o.Recursive {
		args = append(args, "--recursive")
	}
	if o.Remote {
		args = append(args, "--remote")
	}
	if o.Rebase {
		args = append(args, "--rebase")
	}
	if o.Merge {
		args = append(args, "--merge")
	}
	if o.Checkout {
		args = append(args, "--checkout")
	}
	if o.Force == Confirmed {
		args = append(args, "--force")
	}
	if o.NoFetch {
		args = append(args, "--no-fetch")
	}
	if o.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(o.Depth))
	}
	if o.Jobs > 0 {
		args = append(args, "--jobs", strconv.Itoa(o.Jobs))
	}
	return args
}
func (r *Repository) SyncSubmodules(ctx context.Context, paths []string, recursive bool) error {
	args := []string{"submodule", "sync"}
	if recursive {
		args = append(args, "--recursive")
	}
	return r.submodulePaths(ctx, args, paths)
}
func (r *Repository) DeinitSubmodules(ctx context.Context, paths []string, force ConfirmedForce) error {
	if len(paths) == 0 {
		return errors.New("deinit submodules requires paths; use DeinitAllSubmodules for all")
	}
	args := []string{"submodule", "deinit"}
	if force == Confirmed {
		args = append(args, "--force")
	}
	return r.submodulePaths(ctx, args, paths)
}

func (r *Repository) DeinitAllSubmodules(ctx context.Context, token ConfirmationToken, force ConfirmedForce) error {
	if !token.validFor("all-submodules") {
		return &ConfirmationRequiredError{Operation: "deinit all submodules", Identity: "all-submodules"}
	}
	args := []string{"submodule", "deinit"}
	if force == Confirmed {
		args = append(args, "--force")
	}
	return r.managementRun(ctx, append(args, "--all")...)
}
func (r *Repository) submodulePaths(ctx context.Context, args, paths []string) error {
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	return r.managementRun(ctx, args...)
}

// RemoveSubmodule removes the gitlink and worktree. It always requires explicit
// confirmation and refuses a dirty initialized submodule before any mutation.
func (r *Repository) RemoveSubmodule(ctx context.Context, path string, confirm ConfirmedForce) error {
	if confirm != Confirmed {
		return ErrDestructiveConfirmationRequired
	}
	rel, err := validateRepoRelative(path, false)
	if err != nil {
		return err
	}
	modules, err := r.Submodules(ctx)
	if err != nil {
		return err
	}
	moduleName := configuredSubmoduleName(modules, rel)
	if moduleName == "" {
		return fmt.Errorf("path %q is not a configured submodule", path)
	}
	metadataName, err := validateRepoRelative(moduleName, false)
	if err != nil {
		return fmt.Errorf("unsafe submodule metadata name: %w", err)
	}
	if err := ensureCleanInitializedSubmodule(ctx, filepath.Join(r.workTree, rel)); err != nil {
		return err
	}
	if err := r.managementRun(ctx, "submodule", "deinit", "--force", "--", filepath.ToSlash(rel)); err != nil {
		return err
	}
	if err := r.managementRun(ctx, "rm", "-f", "--", filepath.ToSlash(rel)); err != nil {
		return err
	}
	// git rm intentionally keeps the private clone for easy recovery. A Magit
	// remove operation is a full removal, so delete it only after confirmation
	// and only when its configured name cannot escape $GIT_DIR/modules.
	if err := os.RemoveAll(filepath.Join(r.gitDir, "modules", metadataName)); err != nil {
		return fmt.Errorf("remove submodule metadata: %w", err)
	}
	return nil
}

func configuredSubmoduleName(modules []Submodule, rel string) string {
	for _, module := range modules {
		modulePath, err := validateRepoRelative(module.Path, false)
		if err == nil && modulePath == rel {
			return module.Name
		}
	}
	return ""
}

func ensureCleanInitializedSubmodule(ctx context.Context, full string) error {
	_, err := os.Stat(filepath.Join(full, ".git"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	dirty, err := worktreeDirty(ctx, full)
	if err != nil {
		return err
	}
	if dirty {
		return ErrDirtyWorktree
	}
	return nil
}

func (r *Repository) HasSubtree(ctx context.Context) (bool, error) {
	_, err := r.output(ctx, "subtree")
	if err == nil {
		return true, nil
	}
	var ce *CommandError
	if errors.As(err, &ce) && (strings.Contains(ce.Stderr, "not a git command") || strings.Contains(ce.Stderr, "unknown subcommand")) {
		return false, nil
	}
	// Installed subtree prints usage with status 129 when called without args.
	if commandExitCode(err) == 129 {
		return true, nil
	}
	return false, err
}

type SubtreeOptions struct {
	Prefix, Repository, Ref, Message, Branch string
	Squash                                   bool
}

func (r *Repository) subtreeRun(ctx context.Context, action string, o SubtreeOptions) error {
	if err := validateSubtreeOptions(action, o); err != nil {
		return err
	}
	ok, err := r.HasSubtree(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSubtreeUnavailable
	}
	return r.managementRun(ctx, subtreeArgs(action, o)...)
}

func validateSubtreeOptions(action string, o SubtreeOptions) error {
	if o.Prefix == "" {
		return errors.New("subtree prefix is required")
	}
	if err := validateSubtreeTokens(action, o); err != nil {
		return err
	}
	var message string
	switch action {
	case "add":
		message = validateSubtreeAdd(o)
	case "merge":
		message = validateSubtreeMerge(o)
	case "pull", "push":
		message = validateSubtreeTransfer(action, o)
	case "split":
		message = validateSubtreeSplit(o)
	default:
		message = "unknown operation"
	}
	if message != "" {
		return fmt.Errorf("subtree %s: %s", action, message)
	}
	return nil
}

func validateSubtreeAdd(o SubtreeOptions) string {
	if o.Ref == "" {
		return "ref is required"
	}
	if o.Branch != "" {
		return "branch is not supported"
	}
	return ""
}

func validateSubtreeMerge(o SubtreeOptions) string {
	if o.Ref == "" {
		return "commit is required"
	}
	if o.Repository != "" || o.Branch != "" {
		return "repository and branch are not supported"
	}
	return ""
}

func validateSubtreeTransfer(action string, o SubtreeOptions) string {
	if o.Repository == "" || o.Ref == "" {
		return "repository and ref are required"
	}
	if o.Branch != "" || action == "push" && (o.Squash || o.Message != "") {
		return "unsupported options"
	}
	return ""
}

func validateSubtreeSplit(o SubtreeOptions) string {
	if o.Repository != "" || o.Ref != "" || o.Squash || o.Message != "" {
		return "repository, ref, squash, and message are not supported"
	}
	return ""
}

func validateSubtreeTokens(action string, o SubtreeOptions) error {
	for name, value := range map[string]string{"repository": o.Repository, "ref": o.Ref, "branch": o.Branch} {
		if value != "" && (strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n")) {
			return fmt.Errorf("subtree %s: %s is option-like or contains a control character", action, name)
		}
	}
	return nil
}

func subtreeArgs(action string, o SubtreeOptions) []string {
	args := []string{"subtree", action, "--prefix", o.Prefix}
	if o.Repository != "" {
		args = append(args, o.Repository)
	}
	if o.Ref != "" {
		args = append(args, o.Ref)
	}
	if o.Squash {
		args = append(args, "--squash")
	}
	if o.Message != "" {
		args = append(args, "--message", o.Message)
	}
	if o.Branch != "" {
		args = append(args, "--branch", o.Branch)
	}
	return args
}
func (r *Repository) AddSubtree(ctx context.Context, o SubtreeOptions) error {
	return r.subtreeRun(ctx, "add", o)
}
func (r *Repository) MergeSubtree(ctx context.Context, o SubtreeOptions) error {
	return r.subtreeRun(ctx, "merge", o)
}
func (r *Repository) PullSubtree(ctx context.Context, o SubtreeOptions) error {
	return r.subtreeRun(ctx, "pull", o)
}
func (r *Repository) SplitSubtree(ctx context.Context, o SubtreeOptions) error {
	return r.subtreeRun(ctx, "split", o)
}
func (r *Repository) PushSubtree(ctx context.Context, o SubtreeOptions) error {
	return r.subtreeRun(ctx, "push", o)
}

type CloneOptions struct {
	Bare, Mirror, NoCheckout, SingleBranch, NoTags bool
	Branch                                         string
	Depth                                          int
	RecurseSubmodules                              bool
	Jobs                                           int
	Origin                                         string
}

func appendCloneNoTags(args []string, noTags bool) []string {
	if noTags {
		return append(args, "--no-tags")
	}
	return args
}

func validateCloneRequest(source, destination string, o CloneOptions) error {
	if source == "" {
		return errors.New("clone source is empty")
	}
	if o.Bare && o.Mirror {
		return errors.New("bare and mirror are mutually exclusive")
	}
	if o.Depth < 0 || o.Jobs < 0 {
		return errors.New("depth and jobs cannot be negative")
	}
	return safeNewDirectory(destination)
}

func cloneArgs(source, destination string, o CloneOptions) []string {
	args := appendCloneModes([]string{"clone"}, o)
	args = appendCloneValues(args, o)
	return append(args, "--", source, destination)
}

func appendCloneModes(args []string, o CloneOptions) []string {
	for _, option := range []struct {
		set  bool
		flag string
	}{{o.Bare, "--bare"}, {o.Mirror, "--mirror"}, {o.NoCheckout, "--no-checkout"}, {o.SingleBranch, "--single-branch"}, {o.NoTags, "--no-tags"}, {o.RecurseSubmodules, "--recurse-submodules"}} {
		if option.set {
			args = append(args, option.flag)
		}
	}
	return args
}

func appendCloneValues(args []string, o CloneOptions) []string {
	for _, option := range []struct{ flag, value string }{{"--branch", o.Branch}, {"--depth", positiveInt(o.Depth)}, {"--jobs", positiveInt(o.Jobs)}, {"--origin", o.Origin}} {
		if option.value != "" {
			args = append(args, option.flag, option.value)
		}
	}
	return args
}

func positiveInt(value int) string {
	if value > 0 {
		return strconv.Itoa(value)
	}
	return ""
}

// CloneRepository clones into destination without discovering it and therefore
// cannot alter any existing Repository value.
func CloneRepository(ctx context.Context, source, destination string, o CloneOptions) error {
	if err := validateCloneRequest(source, destination, o); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	_, err = executeRecordedGit(ctx, cwd, nil, cloneArgs(source, destination, o))
	return err
}
