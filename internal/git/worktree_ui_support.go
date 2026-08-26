package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReviewedWorktreeAdd binds a worktree creation confirmation to its resolved
// commit, destination, options, and (for -b) branch name.
type ReviewedWorktreeAdd struct {
	Path, Revision, OID, Branch string
	Options                     WorktreeAddOptions
	Token                       ConfirmationToken
}

func worktreeAddIdentity(p ReviewedWorktreeAdd) string {
	b, _ := json.Marshal(struct {
		Path, OID, Branch string
		Options           WorktreeAddOptions
	}{p.Path, p.OID, p.Branch, p.Options})
	return string(b)
}

func (r *Repository) ReviewWorktreeAdd(ctx context.Context, path, revision, branch string, opts WorktreeAddOptions) (ReviewedWorktreeAdd, error) {
	if err := safeNewDirectory(path); err != nil {
		return ReviewedWorktreeAdd{}, err
	}
	if branch != "" {
		if opts.Detach || opts.Checkout != nil || opts.Lock || opts.LockReason != "" {
			return ReviewedWorktreeAdd{}, errors.New("new-branch worktree does not support detached, checkout, or lock options")
		}
		if err := r.validateBranchName(ctx, branch); err != nil {
			return ReviewedWorktreeAdd{}, err
		}
	}
	if revision == "" {
		revision = "HEAD"
	}
	resolved, err := r.ResolveRevision(ctx, revision)
	if err != nil {
		return ReviewedWorktreeAdd{}, err
	}
	p := ReviewedWorktreeAdd{Path: path, Revision: revision, OID: resolved.ID, Branch: branch, Options: opts}
	p.Token = NewConfirmationToken(worktreeAddIdentity(p))
	return p, nil
}

func (r *Repository) AddWorktreeReviewed(ctx context.Context, reviewed ReviewedWorktreeAdd) error {
	if !reviewed.Token.validFor(worktreeAddIdentity(reviewed)) {
		return ErrStalePlan
	}
	current, err := r.ResolveRevision(ctx, reviewed.Revision)
	if err != nil || current.ID != reviewed.OID {
		return ErrStalePlan
	}
	if err := safeNewDirectory(reviewed.Path); err != nil {
		return err
	}
	if reviewed.Branch != "" {
		if err := r.validateBranchName(ctx, reviewed.Branch); err != nil {
			return err
		}
		return r.AddWorktreeWithBranch(ctx, reviewed.Path, reviewed.Branch, reviewed.OID, reviewed.Options.Force)
	}
	return r.AddWorktree(ctx, reviewed.Path, reviewed.OID, reviewed.Options)
}

// ReviewedWorktreeMutation is an exact snapshot used for move and removal.
type ReviewedWorktreeMutation struct {
	Worktree    Worktree
	Destination string
	Force       ConfirmedForce
	Dirty       bool
	Token       ConfirmationToken
}

func worktreeMutationIdentity(p ReviewedWorktreeMutation) string {
	b, _ := json.Marshal(struct {
		Worktree    Worktree
		Destination string
		Force       ConfirmedForce
		Dirty       bool
	}{p.Worktree, p.Destination, p.Force, p.Dirty})
	return string(b)
}

func (r *Repository) ReviewWorktreeMutation(ctx context.Context, path, destination string, force ConfirmedForce) (ReviewedWorktreeMutation, error) {
	wt, err := r.worktreeByPath(ctx, path)
	if err != nil {
		return ReviewedWorktreeMutation{}, err
	}
	if wt.Primary || wt.Bare {
		return ReviewedWorktreeMutation{}, ErrPrimaryWorktree
	}
	// The UI passes paths obtained from Worktrees. Reject aliases, roots and
	// symlinks so review and execution name the same filesystem object.
	if filepath.Clean(wt.Path) == string(filepath.Separator) {
		return ReviewedWorktreeMutation{}, fmt.Errorf("%w: worktree path is filesystem root", ErrUnsafeDestination)
	}
	if info, err := os.Lstat(wt.Path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ReviewedWorktreeMutation{}, fmt.Errorf("%w: worktree path is a symbolic link", ErrUnsafeDestination)
	}
	if destination != "" {
		if err := safeNewDirectory(destination); err != nil {
			return ReviewedWorktreeMutation{}, err
		}
	}
	dirty, err := worktreeDirty(ctx, wt.Path)
	if err != nil {
		return ReviewedWorktreeMutation{}, err
	}
	p := ReviewedWorktreeMutation{Worktree: wt, Destination: destination, Force: force, Dirty: dirty}
	p.Token = NewConfirmationToken(worktreeMutationIdentity(p))
	return p, nil
}

func (r *Repository) reviewedWorktreeMutationCurrent(ctx context.Context, reviewed ReviewedWorktreeMutation) error {
	if !reviewed.Token.validFor(worktreeMutationIdentity(reviewed)) {
		return ErrStalePlan
	}
	current, err := r.ReviewWorktreeMutation(ctx, reviewed.Worktree.Path, reviewed.Destination, reviewed.Force)
	if err != nil {
		return err
	}
	if worktreeMutationIdentity(current) != worktreeMutationIdentity(reviewed) {
		return ErrStalePlan
	}
	return nil
}

func (r *Repository) MoveWorktreeReviewed(ctx context.Context, reviewed ReviewedWorktreeMutation) error {
	if reviewed.Destination == "" {
		return errors.New("worktree move destination is empty")
	}
	if err := r.reviewedWorktreeMutationCurrent(ctx, reviewed); err != nil {
		return err
	}
	return r.MoveWorktree(ctx, reviewed.Worktree.Path, reviewed.Destination, reviewed.Force)
}

func (r *Repository) RemoveWorktreeReviewed(ctx context.Context, reviewed ReviewedWorktreeMutation) error {
	if reviewed.Destination != "" {
		return errors.New("worktree removal review contains a destination")
	}
	if err := r.reviewedWorktreeMutationCurrent(ctx, reviewed); err != nil {
		return err
	}
	return r.RemoveWorktree(ctx, reviewed.Worktree.Path, reviewed.Force)
}

func (r *Repository) LockWorktree(ctx context.Context, path, reason string) error {
	wt, err := r.worktreeByPath(ctx, path)
	if err != nil {
		return err
	}
	if wt.Primary || wt.Bare {
		return ErrPrimaryWorktree
	}
	if wt.Locked {
		return ErrLockedWorktree
	}
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	return r.managementRun(ctx, append(args, "--", wt.Path)...)
}

func (r *Repository) UnlockWorktree(ctx context.Context, path string) error {
	wt, err := r.worktreeByPath(ctx, path)
	if err != nil {
		return err
	}
	if wt.Primary || wt.Bare {
		return ErrPrimaryWorktree
	}
	if !wt.Locked {
		return errors.New("worktree is not locked")
	}
	return r.managementRun(ctx, "worktree", "unlock", "--", wt.Path)
}

// ReviewedWorktreePrune binds the dry-run output to the complete registered
// worktree state. This catches changes even on Git versions that emit no
// verbose dry-run text for newly missing worktrees.
type ReviewedWorktreePrune struct {
	Expire, Output string
	Worktrees      []Worktree
	Token          ConfirmationToken
}

func reviewedWorktreePruneIdentity(p ReviewedWorktreePrune) string {
	b, _ := json.Marshal(struct {
		Expire, Output string
		Worktrees      []Worktree
	}{p.Expire, p.Output, p.Worktrees})
	return string(b)
}

// ReviewWorktreePrune captures both output streams because Git writes the
// verbose dry-run plan to stderr. WorktreePrunePreflight predates that detail
// and is retained for callers that only need command validation.
func (r *Repository) ReviewWorktreePrune(ctx context.Context, expire string) (ReviewedWorktreePrune, error) {
	if strings.ContainsAny(expire, "\x00\r\n") || strings.HasPrefix(expire, "-") {
		return ReviewedWorktreePrune{}, errors.New("invalid worktree prune expiration")
	}
	worktrees, err := r.Worktrees(ctx)
	if err != nil {
		return ReviewedWorktreePrune{}, err
	}
	args := []string{"worktree", "prune", "--dry-run", "--verbose"}
	if expire != "" {
		args = append(args, "--expire", expire)
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.commandDir}, args...)...)
	cmd.Env = gitCommandEnv()
	stdout, stderr := &limitedCapture{remaining: 1 << 20}, &limitedCapture{remaining: 1 << 20}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return ReviewedWorktreePrune{}, &CommandError{Args: args, Err: err, Stderr: strings.TrimSpace(stderr.buf.String()), StderrTruncated: stderr.truncated}
	}
	if stdout.truncated || stderr.truncated {
		return ReviewedWorktreePrune{}, &TooLargeError{Resource: "worktree prune plan"}
	}
	output := append(append([]byte(nil), stdout.buf.Bytes()...), stderr.buf.Bytes()...)
	output = bytes.TrimSpace(output)
	p := ReviewedWorktreePrune{Expire: expire, Output: string(output), Worktrees: worktrees}
	p.Token = NewConfirmationToken(reviewedWorktreePruneIdentity(p))
	return p, nil
}

func (r *Repository) PruneWorktreesReviewed(ctx context.Context, reviewed ReviewedWorktreePrune) error {
	if !reviewed.Token.validFor(reviewedWorktreePruneIdentity(reviewed)) {
		return ErrStalePlan
	}
	current, err := r.ReviewWorktreePrune(ctx, reviewed.Expire)
	if err != nil {
		return err
	}
	if reviewedWorktreePruneIdentity(current) != reviewedWorktreePruneIdentity(reviewed) {
		return ErrStalePlan
	}
	args := []string{"worktree", "prune"}
	if reviewed.Expire != "" {
		args = append(args, "--expire", reviewed.Expire)
	}
	return r.managementRun(ctx, args...)
}
