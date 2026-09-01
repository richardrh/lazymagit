package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrBranchNotFound      = errors.New("local branch does not exist")
	ErrCurrentBranch       = errors.New("operation is not allowed on the current branch")
	ErrUnsupportedWorkflow = errors.New("branch workflow is not supported")
)

// ConfiguredValue distinguishes an unset setting from a configured empty
// value. Branch workflow callers generally need that distinction when drawing
// a transient and when deciding whether "unset" should be offered.
type ConfiguredValue struct {
	Value string
	Set   bool
}

// RebaseMode is a value accepted by branch.<name>.rebase and pull.rebase.
type RebaseMode string

const (
	RebaseFalse       RebaseMode = "false"
	RebaseTrue        RebaseMode = "true"
	RebaseMerges      RebaseMode = "merges"
	RebaseInteractive RebaseMode = "interactive"
)

type BranchConfiguration struct {
	Description ConfiguredValue
	Rebase      ConfiguredValue
	PushRemote  ConfiguredValue
}

func (r *Repository) validateBranchName(ctx context.Context, name string) error {
	if name == "" || strings.TrimSpace(name) != name || strings.HasPrefix(name, "-") {
		return errors.New("branch name is empty, option-like, or has surrounding whitespace")
	}
	// Validate the literal full ref instead of check-ref-format --branch, which
	// expands checkout shorthand such as @{-1} before validating it.
	if _, err := r.output(ctx, "check-ref-format", "refs/heads/"+name); err != nil {
		return fmt.Errorf("invalid branch name %q: %w", name, err)
	}
	return nil
}

func (r *Repository) localBranchOID(ctx context.Context, name string) (string, error) {
	if err := r.validateBranchName(ctx, name); err != nil {
		return "", err
	}
	out, err := r.output(ctx, "show-ref", "--verify", "--hash", "refs/heads/"+name)
	if err != nil {
		if commandExitCode(err) == 1 {
			return "", fmt.Errorf("%w: %q", ErrBranchNotFound, name)
		}
		return "", err
	}
	return trimLine(out), nil
}

func (r *Repository) resolveBranchCommit(ctx context.Context, revision string) (string, error) {
	if revision == "" || strings.TrimSpace(revision) != revision {
		return "", errors.New("revision is empty or has surrounding whitespace")
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve revision %q: %w", revision, err)
	}
	return trimLine(out), nil
}

// CheckoutOptions is the deliberately small portable subset of switch's
// checkout controls used by the branch transient.
type CheckoutOptions struct{ RecurseSubmodules bool }

// CheckoutRevision checks out a commit with detached HEAD. Resolving first
// prevents an option-like revision from becoming a switch argument.
func (r *Repository) CheckoutRevision(ctx context.Context, revision string) error {
	return r.CheckoutRevisionWithOptions(ctx, revision, CheckoutOptions{})
}

func (r *Repository) CheckoutRevisionWithOptions(ctx context.Context, revision string, options CheckoutOptions) error {
	oid, err := r.resolveBranchCommit(ctx, revision)
	if err != nil {
		return err
	}
	args := []string{"switch"}
	if options.RecurseSubmodules {
		args = append(args, "--recurse-submodules")
	}
	args = append(args, "--detach", oid)
	return r.run(ctx, args...)
}

func (r *Repository) CheckoutBranch(ctx context.Context, name string) error {
	return r.CheckoutBranchWithOptions(ctx, name, CheckoutOptions{})
}

func (r *Repository) CheckoutBranchWithOptions(ctx context.Context, name string, options CheckoutOptions) error {
	if _, err := r.localBranchOID(ctx, name); err != nil {
		return err
	}
	args := []string{"switch"}
	if options.RecurseSubmodules {
		args = append(args, "--recurse-submodules")
	}
	args = append(args, "--", name)
	return r.run(ctx, args...)
}

func (r *Repository) CreateAndCheckoutBranch(ctx context.Context, name, startPoint string) error {
	if err := r.validateBranchName(ctx, name); err != nil {
		return err
	}
	if startPoint == "" {
		startPoint = "HEAD"
	}
	oid, err := r.resolveBranchCommit(ctx, startPoint)
	if err != nil {
		return err
	}
	return r.run(ctx, "switch", "-c", name, oid)
}

// CreateBranchOnly implements Magit's b n without changing HEAD.
func (r *Repository) CreateBranchOnly(ctx context.Context, name, startPoint string) error {
	if err := r.validateBranchName(ctx, name); err != nil {
		return err
	}
	if startPoint == "" {
		startPoint = "HEAD"
	}
	oid, err := r.resolveBranchCommit(ctx, startPoint)
	if err != nil {
		return err
	}
	return r.run(ctx, "branch", "--", name, oid)
}

func (r *Repository) CreateOrphanBranch(ctx context.Context, name string) error {
	if err := r.validateBranchName(ctx, name); err != nil {
		return err
	}
	return r.run(ctx, "switch", "--orphan", name)
}

// RenameBranch relies on git-branch's atomic ref/config rename. In particular,
// branch.<old>.pushRemote moves to branch.<new>.pushRemote with the other branch
// settings instead of being copied and left stale.
func (r *Repository) RenameBranch(ctx context.Context, oldName, newName string) error {
	if _, err := r.localBranchOID(ctx, oldName); err != nil {
		return err
	}
	if err := r.validateBranchName(ctx, newName); err != nil {
		return err
	}
	return r.run(ctx, "branch", "-m", oldName, newName)
}

func (r *Repository) SetBranchUpstream(ctx context.Context, branch, upstream string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	if upstream == "" || strings.TrimSpace(upstream) != upstream {
		return errors.New("upstream branch is empty or has surrounding whitespace")
	}
	// Only an actual local or remote-tracking ref is a meaningful upstream.
	out, err := r.output(ctx, "rev-parse", "--verify", "--symbolic-full-name", "--end-of-options", upstream)
	if err != nil {
		return fmt.Errorf("resolve upstream %q: %w", upstream, err)
	}
	full := trimLine(out)
	if !strings.HasPrefix(full, "refs/heads/") && !strings.HasPrefix(full, "refs/remotes/") {
		return fmt.Errorf("upstream %q is not a branch", upstream)
	}
	return r.run(ctx, "branch", "--set-upstream-to="+full, branch)
}

func (r *Repository) UnsetBranchUpstream(ctx context.Context, branch string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	return r.run(ctx, "branch", "--unset-upstream", branch)
}

func (r *Repository) SetBranchPushRemote(ctx context.Context, branch, remote string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	if err := r.validateRemote(ctx, remote); err != nil {
		return err
	}
	return r.run(ctx, "config", "branch."+branch+".pushRemote", remote)
}

func (r *Repository) UnsetBranchPushRemote(ctx context.Context, branch string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	return r.unsetConfig(ctx, "branch."+branch+".pushRemote")
}

type BranchDeleteConfirmation uint8

const (
	BranchDeleteNoConfirmation BranchDeleteConfirmation = iota
	BranchDeleteConfirmUnmerged
	BranchDeleteSwitchCurrent
)

type LocalBranchDeleteResult struct {
	Name         string
	OID          string
	MergeTarget  string
	Current      bool
	Merged       bool
	Unmerged     bool
	Deleted      bool
	Confirmation BranchDeleteConfirmation
	Token        ConfirmationToken
}

// LocalBranchDeletePreflight is read-only and reports why deletion needs user
// confirmation. Merge is tested against the configured upstream when present,
// matching git branch -d, and otherwise against HEAD.
func (r *Repository) LocalBranchDeletePreflight(ctx context.Context, name string) (LocalBranchDeleteResult, error) {
	oid, err := r.localBranchOID(ctx, name)
	if err != nil {
		return LocalBranchDeleteResult{}, err
	}
	result := LocalBranchDeleteResult{Name: name, OID: oid}
	result.Token = NewConfirmationToken(oid)
	current, err := r.currentBranch(ctx)
	if err != nil {
		return result, err
	}
	result.Current = current == name
	target := "HEAD"
	if out, resolveErr := r.output(ctx, "rev-parse", "--verify", "--end-of-options", name+"@{upstream}^{commit}"); resolveErr == nil {
		target = trimLine(out)
	} else if commandExitCode(resolveErr) != 128 {
		return result, resolveErr
	}
	result.MergeTarget = target
	if _, err := r.output(ctx, "merge-base", "--is-ancestor", oid, target); err == nil {
		result.Merged = true
	} else if code := commandExitCode(err); code != 1 {
		return result, err
	}
	if !result.Merged {
		result.Unmerged = true
	}
	if result.Current {
		result.Confirmation = BranchDeleteSwitchCurrent
	} else if result.Unmerged {
		result.Confirmation = BranchDeleteConfirmUnmerged
	}
	return result, nil
}

// DeleteLocalBranch performs only an unforced deletion. If confirmation or a
// branch switch is required it returns that typed requirement without mutating.
func (r *Repository) DeleteLocalBranch(ctx context.Context, name string) (LocalBranchDeleteResult, error) {
	result, err := r.LocalBranchDeletePreflight(ctx, name)
	if err != nil || result.Confirmation != BranchDeleteNoConfirmation {
		return result, err
	}
	err = r.run(ctx, "branch", "-d", "--", name)
	result.Deleted = err == nil
	return result, err
}

// DeleteLocalBranchConfirmed is the explicit post-prompt operation. It repeats
// preflight to avoid acting on a branch which became current in the meantime.
func (r *Repository) DeleteLocalBranchConfirmed(ctx context.Context, name string, token ConfirmationToken) (LocalBranchDeleteResult, error) {
	result, err := r.LocalBranchDeletePreflight(ctx, name)
	if err != nil || result.Confirmation == BranchDeleteSwitchCurrent {
		return result, err
	}
	if !token.validFor(result.OID) {
		return result, fmt.Errorf("%w: local branch %s moved", ErrStalePlan, name)
	}
	// update-ref performs the identity comparison while holding the ref lock;
	// unlike `branch -D`, a move between preflight and deletion cannot cause a
	// different commit to be deleted.
	err = r.run(ctx, "update-ref", "-d", "refs/heads/"+name, result.OID)
	result.Deleted = err == nil
	if err != nil {
		if r.localBranchMoved(ctx, name, result.OID) {
			return result, fmt.Errorf("%w: local branch %s moved", ErrStalePlan, name)
		}
		return result, err
	}
	// Match git-branch's cleanup of branch-local configuration after the atomic
	// ref deletion. Missing configuration is normal.
	if err := r.removeBranchConfiguration(ctx, name); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Repository) localBranchMoved(ctx context.Context, name, expectedOID string) bool {
	current, err := r.localBranchOID(ctx, name)
	return err == nil && current != expectedOID
}

func (r *Repository) removeBranchConfiguration(ctx context.Context, name string) error {
	err := r.run(ctx, "config", "--remove-section", "branch."+name)
	if err == nil || commandExitCode(err) == 5 {
		return nil
	}
	var commandErr *CommandError
	if errors.As(err, &commandErr) && strings.Contains(commandErr.Stderr, "no such section") {
		return nil
	}
	return err
}

func (r *Repository) DeleteRemoteBranch(ctx context.Context, remote, branch string) error {
	if err := r.validateRemote(ctx, remote); err != nil {
		return err
	}
	if err := r.validateBranchName(ctx, branch); err != nil {
		return err
	}
	return r.run(ctx, "push", "--delete", "--", remote, branch)
}

// ResetBranch moves only a local branch ref. git branch -f supplies Git's
// checked-out-worktree protection and leaves the index/worktree untouched.
func (r *Repository) ResetBranch(ctx context.Context, branch, revision string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	current, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	if current == branch {
		return fmt.Errorf("%w: %q", ErrCurrentBranch, branch)
	}
	oid, err := r.resolveBranchCommit(ctx, revision)
	if err != nil {
		return err
	}
	return r.run(ctx, "branch", "-f", "--", branch, oid)
}

func (r *Repository) BranchConfiguration(ctx context.Context, branch string) (BranchConfiguration, error) {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return BranchConfiguration{}, err
	}
	read := func(suffix string) (ConfiguredValue, error) {
		return r.workflowConfigValue(ctx, "branch."+branch+"."+suffix)
	}
	description, err := read("description")
	if err != nil {
		return BranchConfiguration{}, err
	}
	rebase, err := read("rebase")
	if err != nil {
		return BranchConfiguration{}, err
	}
	pushRemote, err := read("pushRemote")
	return BranchConfiguration{Description: description, Rebase: rebase, PushRemote: pushRemote}, err
}

func (r *Repository) BranchDescription(ctx context.Context, branch string) (ConfiguredValue, error) {
	configuration, err := r.BranchConfiguration(ctx, branch)
	return configuration.Description, err
}

func (r *Repository) BranchRebase(ctx context.Context, branch string) (ConfiguredValue, error) {
	configuration, err := r.BranchConfiguration(ctx, branch)
	return configuration.Rebase, err
}

func (r *Repository) BranchPushRemote(ctx context.Context, branch string) (ConfiguredValue, error) {
	configuration, err := r.BranchConfiguration(ctx, branch)
	return configuration.PushRemote, err
}

func (r *Repository) SetBranchDescription(ctx context.Context, branch, description string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	return r.run(ctx, "config", "branch."+branch+".description", description)
}

func (r *Repository) UnsetBranchDescription(ctx context.Context, branch string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	return r.unsetConfig(ctx, "branch."+branch+".description")
}

func validRebaseMode(mode RebaseMode) bool {
	switch mode {
	case RebaseFalse, RebaseTrue, RebaseMerges, RebaseInteractive:
		return true
	default:
		return false
	}
}

func (r *Repository) SetBranchRebase(ctx context.Context, branch string, mode RebaseMode) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	if !validRebaseMode(mode) {
		return fmt.Errorf("invalid rebase mode %q", mode)
	}
	return r.run(ctx, "config", "branch."+branch+".rebase", string(mode))
}

func (r *Repository) UnsetBranchRebase(ctx context.Context, branch string) error {
	if _, err := r.localBranchOID(ctx, branch); err != nil {
		return err
	}
	return r.unsetConfig(ctx, "branch."+branch+".rebase")
}

func (r *Repository) PullRebase(ctx context.Context) (ConfiguredValue, error) {
	return r.workflowConfigValue(ctx, "pull.rebase")
}

func (r *Repository) SetPullRebase(ctx context.Context, mode RebaseMode) error {
	if !validRebaseMode(mode) {
		return fmt.Errorf("invalid rebase mode %q", mode)
	}
	return r.run(ctx, "config", "pull.rebase", string(mode))
}

func (r *Repository) UnsetPullRebase(ctx context.Context) error {
	return r.unsetConfig(ctx, "pull.rebase")
}

func (r *Repository) RemotePushDefault(ctx context.Context) (ConfiguredValue, error) {
	return r.workflowConfigValue(ctx, "remote.pushDefault")
}

func (r *Repository) SetRemotePushDefault(ctx context.Context, remote string) error {
	if err := r.validateRemote(ctx, remote); err != nil {
		return err
	}
	return r.run(ctx, "config", "remote.pushDefault", remote)
}

func (r *Repository) UnsetRemotePushDefault(ctx context.Context) error {
	return r.unsetConfig(ctx, "remote.pushDefault")
}

func (r *Repository) unsetConfig(ctx context.Context, key string) error {
	configured, err := r.workflowConfigValue(ctx, key)
	set := configured.Set
	if err != nil || !set {
		return err
	}
	return r.run(ctx, "config", "--unset-all", key)
}

func (r *Repository) workflowConfigValue(ctx context.Context, key string) (ConfiguredValue, error) {
	out, err := r.output(ctx, "config", "--get", key)
	if err != nil {
		if commandExitCode(err) == 1 {
			return ConfiguredValue{}, nil
		}
		return ConfiguredValue{}, err
	}
	return ConfiguredValue{Value: trimLine(out), Set: true}, nil
}

type BranchWorkflowSupport struct {
	Supported bool
	Reason    error
}

// SpinOffSupport and SpinOutSupport are deliberately typed as unsupported.
// Their Magit semantics include branch-point and upstream rewrites which must
// not be approximated by a checkout plus reset.
func (r *Repository) SpinOffSupport() BranchWorkflowSupport {
	return BranchWorkflowSupport{Reason: fmt.Errorf("%w: spin-off", ErrUnsupportedWorkflow)}
}

func (r *Repository) SpinOutSupport() BranchWorkflowSupport {
	return BranchWorkflowSupport{Reason: fmt.Errorf("%w: spin-out", ErrUnsupportedWorkflow)}
}
