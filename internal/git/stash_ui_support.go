package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ReviewedStash identifies one stash by commit OID. The reflog selector is
// display metadata only; exact execution passes the immutable OID directly.
type ReviewedStash struct {
	Stash Stash
	token ConfirmationToken
}

// ReviewStash returns a reviewed identity suitable for exact apply or branch.
func (r *Repository) ReviewStash(ctx context.Context, ref string) (ReviewedStash, error) {
	stashes, err := r.Stashes(ctx)
	if err != nil {
		return ReviewedStash{}, err
	}
	want := defaultStashRef(ref)
	for _, stash := range stashes {
		if want == stash.Ref || want == stash.ID || want == stash.ShortID {
			return ReviewedStash{Stash: stash, token: NewConfirmationToken(stash.ID)}, nil
		}
	}
	return ReviewedStash{}, errors.New("stash does not exist")
}

func (r *Repository) validateReviewedStash(ctx context.Context, reviewed ReviewedStash) error {
	if reviewed.Stash.ID == "" || !reviewed.token.validFor(reviewed.Stash.ID) {
		return ErrStalePlan
	}
	stashes, err := r.Stashes(ctx)
	if err != nil {
		return err
	}
	for _, stash := range stashes {
		if stash.ID == reviewed.Stash.ID {
			return nil
		}
	}
	return ErrStalePlan
}

// ApplyReviewedStash applies exactly the reviewed commit OID. The stash reflog
// entry is retained regardless of success because this operation never uses a
// mutable stash selector.
func (r *Repository) ApplyReviewedStash(ctx context.Context, reviewed ReviewedStash, options StashApplyOptions) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	return r.stashApplyArgument(ctx, "apply", reviewed.Stash.ID, options)
}

// PopReviewedStash fails closed: stock Git cannot atomically apply and remove a
// stash reflog entry by immutable OID.
func (r *Repository) PopReviewedStash(ctx context.Context, reviewed ReviewedStash, options StashApplyOptions) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	return fmt.Errorf("pop reviewed stash: %w", ErrReviewedStashRemovalUnsupported)
}

// DropReviewedStash fails closed: git stash drop accepts only mutable reflog
// selectors, so exact reviewed removal is unavailable with stock Git.
func (r *Repository) DropReviewedStash(ctx context.Context, reviewed ReviewedStash) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	return fmt.Errorf("drop reviewed stash: %w", ErrReviewedStashRemovalUnsupported)
}

// BranchReviewedStash creates a branch from exactly the reviewed OID. The stash
// entry is intentionally retained: Git only drops selector-form arguments.
func (r *Repository) BranchReviewedStash(ctx context.Context, branch string, reviewed ReviewedStash) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	if err := r.validateStashBranchName(ctx, branch); err != nil {
		return err
	}
	return r.run(ctx, "stash", "branch", branch, reviewed.Stash.ID)
}

// ReviewedStashClear freezes the complete ordered stash OID set.
type ReviewedStashClear struct {
	Stashes []Stash
	token   ConfirmationToken
}

func stashClearIdentity(stashes []Stash) string {
	ids := make([]string, len(stashes))
	for i := range stashes {
		ids[i] = stashes[i].ID
	}
	return strings.Join(ids, "\x00")
}

// ReviewStashClear returns a plan bound to the complete current stash list.
func (r *Repository) ReviewStashClear(ctx context.Context) (ReviewedStashClear, error) {
	stashes, err := r.Stashes(ctx)
	if err != nil {
		return ReviewedStashClear{}, err
	}
	if len(stashes) == 0 {
		return ReviewedStashClear{}, errors.New("no stashes to clear")
	}
	return ReviewedStashClear{Stashes: append([]Stash(nil), stashes...), token: NewConfirmationToken(stashClearIdentity(stashes))}, nil
}

// ClearReviewedStashes rejects stale plans, then fails closed because stock Git
// cannot atomically clear the exact reviewed reflog set.
func (r *Repository) ClearReviewedStashes(ctx context.Context, reviewed ReviewedStashClear) error {
	identity := stashClearIdentity(reviewed.Stashes)
	if identity == "" || !reviewed.token.validFor(identity) {
		return ErrStalePlan
	}
	current, err := r.Stashes(ctx)
	if err != nil {
		return err
	}
	if stashClearIdentity(current) != identity {
		return ErrStalePlan
	}
	return fmt.Errorf("clear reviewed stashes: %w", ErrReviewedStashRemovalUnsupported)
}
