package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

const mergeReviewConfigLimit = 1 << 20

// ReviewedMerge is the immutable backend half of the merge confirmation shown
// by the TUI.  It binds approval to HEAD, the resolved target and effective Git
// configuration; callers cannot reuse an approval after any of those change.
type ReviewedMerge struct {
	Args      MergeArgs
	Preflight MergePreflight
	HeadOID   string
	Config    []byte
}

func (r *Repository) ReviewMerge(ctx context.Context, args MergeArgs) (ReviewedMerge, error) {
	preflight, err := r.MergePreflight(ctx, args.Target)
	if err != nil {
		return ReviewedMerge{}, err
	}
	if preflight.State.InProgress {
		return ReviewedMerge{}, errors.New("a merge is already in progress")
	}
	head, config, err := r.mergeReviewIdentity(ctx)
	if err != nil {
		return ReviewedMerge{}, err
	}
	args.Target = preflight.Target
	return ReviewedMerge{Args: args, Preflight: preflight, HeadOID: head, Config: config}, nil
}

func (r *Repository) ExecuteReviewedMerge(ctx context.Context, reviewed ReviewedMerge) (MergePreflight, error) {
	if reviewed.HeadOID == "" || reviewed.Preflight.TargetOID == "" || reviewed.Args.Target == "" {
		return MergePreflight{}, ErrStalePlan
	}
	head, config, err := r.mergeReviewIdentity(ctx)
	if err != nil {
		return MergePreflight{}, err
	}
	current, err := r.MergePreflight(ctx, reviewed.Args.Target)
	if err != nil {
		return current, err
	}
	if head != reviewed.HeadOID || !bytes.Equal(config, reviewed.Config) ||
		current.TargetOID != reviewed.Preflight.TargetOID || current.State.InProgress ||
		current.State.Dirty != reviewed.Preflight.State.Dirty {
		return current, ErrStalePlan
	}
	return r.MergeWithArgs(ctx, reviewed.Args)
}

// ReviewedMergeAbort binds destructive merge abort approval to the exact merge
// heads, current HEAD and effective configuration displayed to the user.
type ReviewedMergeAbort struct {
	HeadOID    string
	MergeHeads []string
	Config     []byte
}

func (r *Repository) ReviewMergeAbort(ctx context.Context) (ReviewedMergeAbort, error) {
	state, err := r.QueryOperationState(ctx)
	if err != nil {
		return ReviewedMergeAbort{}, err
	}
	heads, ok := operationHeads(state, OperationMerge)
	if !ok {
		return ReviewedMergeAbort{}, errors.New("no merge is in progress")
	}
	head, config, err := r.mergeReviewIdentity(ctx)
	if err != nil {
		return ReviewedMergeAbort{}, err
	}
	return ReviewedMergeAbort{HeadOID: head, MergeHeads: append([]string(nil), heads...), Config: config}, nil
}

func (r *Repository) ExecuteReviewedMergeAbort(ctx context.Context, reviewed ReviewedMergeAbort) error {
	if reviewed.HeadOID == "" || len(reviewed.MergeHeads) == 0 {
		return ErrStalePlan
	}
	state, err := r.QueryOperationState(ctx)
	if err != nil {
		return err
	}
	heads, ok := operationHeads(state, OperationMerge)
	head, config, identityErr := r.mergeReviewIdentity(ctx)
	if identityErr != nil {
		return identityErr
	}
	if !ok || head != reviewed.HeadOID || !sameStrings(heads, reviewed.MergeHeads) || !bytes.Equal(config, reviewed.Config) {
		return ErrStalePlan
	}
	return r.AbortMerge(ctx)
}

func operationHeads(state OperationState, kind OperationKind) ([]string, bool) {
	for _, operation := range state.Items {
		if operation.Kind == kind {
			return operation.Heads, true
		}
	}
	return nil, false
}

func (r *Repository) mergeReviewIdentity(ctx context.Context) (string, []byte, error) {
	head, err := r.output(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	config, truncated, err := r.outputLimited(ctx, mergeReviewConfigLimit, "config", "--null", "--list", "--show-origin", "--show-scope")
	if err != nil {
		return "", nil, fmt.Errorf("snapshot Git configuration: %w", err)
	}
	if truncated {
		return "", nil, &TooLargeError{Resource: "Git configuration"}
	}
	return trimLine(head), append([]byte(nil), config...), nil
}
