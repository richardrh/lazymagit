package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const mergeReviewConfigLimit = 1 << 20

// ReviewedMerge is the immutable backend half of the merge confirmation shown
// by the TUI.  It binds approval to HEAD, the resolved target and effective Git
// configuration; callers cannot reuse an approval after any of those change.
type ReviewedMerge struct {
	Args         MergeArgs
	ArgsIdentity string
	Preflight    MergePreflight
	HeadOID      string
	Config       []byte
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
	args.StrategyOptions = append([]string(nil), args.StrategyOptions...)
	return ReviewedMerge{Args: args, ArgsIdentity: mergeArgsIdentity(args), Preflight: preflight, HeadOID: head, Config: config}, nil
}

func (r *Repository) ExecuteReviewedMerge(ctx context.Context, reviewed ReviewedMerge) (MergePreflight, error) {
	if reviewed.HeadOID == "" || reviewed.Preflight.TargetOID == "" || reviewed.Args.Target == "" || reviewed.ArgsIdentity == "" || reviewed.ArgsIdentity != mergeArgsIdentity(reviewed.Args) {
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

func mergeArgsIdentity(args MergeArgs) string {
	return strings.Join([]string{args.Target, strconv.Itoa(int(args.Mode)), strconv.FormatBool(args.NoCommit), strconv.FormatBool(args.Squash), strconv.FormatBool(args.ConfirmDirty), args.Strategy, strings.Join(args.StrategyOptions, "\x01"), strconv.FormatBool(args.Signoff)}, "\x00")
}

// ReviewedMergeAbort binds destructive merge abort approval to the exact merge
// heads, current HEAD and effective configuration displayed to the user.
type ReviewedMergeAbort struct {
	HeadOID    string
	MergeHeads []string
	Config     []byte
}

// ReviewedMergeContinue binds the final merge commit to the exact prepared
// index tree, merge heads, HEAD, and effective configuration shown at review.
// Unlike a plain `commit`, it cannot silently commit a different resolution.
type ReviewedMergeContinue struct {
	HeadOID    string
	MergeHeads []string
	IndexTree  string
	Config     []byte
}

func (r *Repository) ReviewMergeContinue(ctx context.Context) (ReviewedMergeContinue, error) {
	state, err := r.MergeState(ctx)
	if err != nil {
		return ReviewedMergeContinue{}, err
	}
	if !state.InProgress {
		return ReviewedMergeContinue{}, errors.New("no merge is in progress")
	}
	if len(state.Conflicts) != 0 {
		return ReviewedMergeContinue{}, fmt.Errorf("cannot continue merge with unresolved conflicts: %s", strings.Join(state.Conflicts, ", "))
	}
	head, config, err := r.mergeReviewIdentity(ctx)
	if err != nil {
		return ReviewedMergeContinue{}, err
	}
	tree, err := r.output(ctx, "write-tree")
	if err != nil {
		return ReviewedMergeContinue{}, fmt.Errorf("snapshot merge index: %w", err)
	}
	return ReviewedMergeContinue{HeadOID: head, MergeHeads: append([]string(nil), state.Heads...), IndexTree: trimLine(tree), Config: config}, nil
}

func (r *Repository) ExecuteReviewedMergeContinue(ctx context.Context, reviewed ReviewedMergeContinue) error {
	if reviewed.HeadOID == "" || reviewed.IndexTree == "" || len(reviewed.MergeHeads) == 0 {
		return ErrStalePlan
	}
	current, err := r.ReviewMergeContinue(ctx)
	if err != nil {
		return err
	}
	if current.HeadOID != reviewed.HeadOID || current.IndexTree != reviewed.IndexTree || !sameStrings(current.MergeHeads, reviewed.MergeHeads) || !bytes.Equal(current.Config, reviewed.Config) {
		return ErrStalePlan
	}
	return r.ContinueMerge(ctx)
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
