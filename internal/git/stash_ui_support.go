package git

import (
	"context"
	"errors"
	"strings"
)

// ReviewedStash identifies one stash by commit OID.  The reflog selector is
// display metadata only: execution resolves the immutable OID again so a new
// stash cannot redirect a reviewed destructive operation.
type ReviewedStash struct {
	Stash Stash
	token ConfirmationToken
}

// ReviewStash returns a reviewed identity suitable for pop, drop, or branch.
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

// PopReviewedStash applies and removes exactly the reviewed OID. Git retains
// the stash when application conflicts.
func (r *Repository) PopReviewedStash(ctx context.Context, reviewed ReviewedStash, options StashApplyOptions) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	return r.StashPop(ctx, reviewed.Stash.ID, options)
}

// DropReviewedStash removes exactly the reviewed OID.
func (r *Repository) DropReviewedStash(ctx context.Context, reviewed ReviewedStash) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	return r.StashDrop(ctx, reviewed.Stash.ID, ConfirmationOptions{Token: reviewed.token})
}

// BranchReviewedStash creates a branch from exactly the reviewed stash.
func (r *Repository) BranchReviewedStash(ctx context.Context, branch string, reviewed ReviewedStash) error {
	if err := r.validateReviewedStash(ctx, reviewed); err != nil {
		return err
	}
	return r.StashBranch(ctx, branch, reviewed.Stash.ID)
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

// ClearReviewedStashes rejects additions, removals, and reorderings after
// review, then delegates to the existing confirmed clear operation.
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
	return r.StashClear(ctx, ConfirmationOptions{Token: NewConfirmationToken("all-stashes")})
}
