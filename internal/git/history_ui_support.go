package git

// This file is the reviewed UI boundary for history_workflows.go.  The
// low-level history API intentionally accepts plain arguments; the TUI needs a
// stronger, two-phase contract so a confirmation cannot be reused after the
// repository changes.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type HistoryUIAction string

const (
	HistoryUIRebaseStart       HistoryUIAction = "rebase-start"
	HistoryUIRebaseContinue    HistoryUIAction = "rebase-continue"
	HistoryUIRebaseSkip        HistoryUIAction = "rebase-skip"
	HistoryUIRebaseAbort       HistoryUIAction = "rebase-abort"
	HistoryUIRebaseInteractive HistoryUIAction = "rebase-interactive"
	HistoryUIRebaseTodo        HistoryUIAction = "rebase-todo"
	HistoryUIReset             HistoryUIAction = "reset"
	HistoryUICherryStart       HistoryUIAction = "cherry-pick-start"
	HistoryUICherryContinue    HistoryUIAction = "cherry-pick-continue"
	HistoryUICherrySkip        HistoryUIAction = "cherry-pick-skip"
	HistoryUICherryAbort       HistoryUIAction = "cherry-pick-abort"
	HistoryUIRevertStart       HistoryUIAction = "revert-start"
	HistoryUIRevertContinue    HistoryUIAction = "revert-continue"
	HistoryUIRevertSkip        HistoryUIAction = "revert-skip"
	HistoryUIRevertAbort       HistoryUIAction = "revert-abort"
	HistoryUIBisectStart       HistoryUIAction = "bisect-start"
	HistoryUIBisectGood        HistoryUIAction = "bisect-good"
	HistoryUIBisectBad         HistoryUIAction = "bisect-bad"
	HistoryUIBisectSkip        HistoryUIAction = "bisect-skip"
	HistoryUIBisectReset       HistoryUIAction = "bisect-reset"
)

// HistoryUIRequest is a closed, typed union. Fields not used by Action are
// ignored. Review resolves every revision to an OID before it creates a token.
type HistoryUIRequest struct {
	Action    HistoryUIAction
	Rebase    RebaseOptions
	Reset     ResetOptions
	Pick      PickOptions
	Revisions []string
	Bisect    BisectStartOptions
	Revision  string
}

type HistoryUIState struct {
	HEAD, Index, Worktree, Operation string
}

type ReviewedHistoryUIAction struct {
	Request HistoryUIRequest
	State   HistoryUIState
	Plan    []string
	Token   ConfirmationToken
}

func (r *Repository) ReviewHistoryUIAction(ctx context.Context, request HistoryUIRequest) (ReviewedHistoryUIAction, error) {
	canonical, plan, err := r.canonicalHistoryUIRequest(ctx, request)
	if err != nil {
		return ReviewedHistoryUIAction{}, err
	}
	state, err := r.historyUIState(ctx)
	if err != nil {
		return ReviewedHistoryUIAction{}, err
	}
	review := ReviewedHistoryUIAction{Request: canonical, State: state, Plan: plan}
	review.Token = NewConfirmationToken(historyUIIdentity(review))
	return review, nil
}

func (r *Repository) ExecuteReviewedHistoryUIAction(ctx context.Context, reviewed ReviewedHistoryUIAction) error {
	current, err := r.ReviewHistoryUIAction(ctx, reviewed.Request)
	if err != nil {
		return err
	}
	if !reviewedHistoryUIActionIsCurrent(reviewed, current) {
		return ErrStalePlan
	}
	if handled, err := r.executeReviewedRebaseUIAction(ctx, reviewed.Request); handled {
		return err
	}
	if handled, err := r.executeReviewedSequencerUIAction(ctx, reviewed.Request); handled {
		return err
	}
	if handled, err := r.executeReviewedBisectUIAction(ctx, reviewed.Request); handled {
		return err
	}
	return errors.New("invalid reviewed history action")
}

func reviewedHistoryUIActionIsCurrent(reviewed, current ReviewedHistoryUIAction) bool {
	return reviewed.Token.validFor(historyUIIdentity(current)) && reviewed.State == current.State && sameHistoryUIRequest(reviewed.Request, current.Request)
}

func (r *Repository) executeReviewedRebaseUIAction(ctx context.Context, q HistoryUIRequest) (bool, error) {
	switch q.Action {
	case HistoryUIRebaseStart:
		return true, r.RebaseStart(ctx, q.Rebase)
	case HistoryUIRebaseContinue:
		return true, r.RebaseContinue(ctx)
	case HistoryUIRebaseSkip:
		return true, r.RebaseSkip(ctx)
	case HistoryUIRebaseAbort:
		return true, r.RebaseAbort(ctx)
	case HistoryUIRebaseInteractive:
		return true, r.RebaseInteractive(ctx, q.Rebase)
	case HistoryUIRebaseTodo:
		return true, r.WriteRebaseTodo(ctx, q.Rebase.Todo)
	case HistoryUIReset:
		q.Reset.Confirmed = true
		return true, r.Reset(ctx, q.Reset)
	default:
		return false, nil
	}
}

func (r *Repository) executeReviewedSequencerUIAction(ctx context.Context, q HistoryUIRequest) (bool, error) {
	if handled, err := r.executeReviewedCherryPickUIAction(ctx, q); handled {
		return true, err
	}
	return r.executeReviewedRevertUIAction(ctx, q)
}

func (r *Repository) executeReviewedCherryPickUIAction(ctx context.Context, q HistoryUIRequest) (bool, error) {
	switch q.Action {
	case HistoryUICherryStart:
		return true, r.CherryPickStart(ctx, q.Revisions, q.Pick)
	case HistoryUICherryContinue:
		return true, r.CherryPickContinue(ctx)
	case HistoryUICherrySkip:
		return true, r.CherryPickSkip(ctx)
	case HistoryUICherryAbort:
		return true, r.CherryPickAbort(ctx)
	default:
		return false, nil
	}
}

func (r *Repository) executeReviewedRevertUIAction(ctx context.Context, q HistoryUIRequest) (bool, error) {
	switch q.Action {
	case HistoryUIRevertStart:
		return true, r.RevertStart(ctx, q.Revisions, q.Pick)
	case HistoryUIRevertContinue:
		return true, r.RevertContinue(ctx)
	case HistoryUIRevertSkip:
		return true, r.RevertSkip(ctx)
	case HistoryUIRevertAbort:
		return true, r.RevertAbort(ctx)
	default:
		return false, nil
	}
}

func (r *Repository) executeReviewedBisectUIAction(ctx context.Context, q HistoryUIRequest) (bool, error) {
	switch q.Action {
	case HistoryUIBisectStart:
		return true, r.BisectStart(ctx, q.Bisect)
	case HistoryUIBisectGood:
		return true, r.BisectGood(ctx, q.Revision)
	case HistoryUIBisectBad:
		return true, r.BisectBad(ctx, q.Revision)
	case HistoryUIBisectSkip:
		return true, r.executeBisectSkip(ctx, q.Revision)
	case HistoryUIBisectReset:
		return true, r.BisectReset(ctx)
	default:
		return false, nil
	}
}

func (r *Repository) executeBisectSkip(ctx context.Context, revision string) error {
	if revision == "" {
		return r.BisectSkip(ctx)
	}
	return r.BisectSkip(ctx, revision)
}

func (r *Repository) canonicalHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	q = cloneHistoryUIRequest(q)
	if isRebaseHistoryUIAction(q.Action) {
		return r.canonicalRebaseHistoryUIRequest(ctx, q)
	}
	if q.Action == HistoryUIReset {
		return r.canonicalResetHistoryUIRequest(ctx, q)
	}
	if isSequencerHistoryUIAction(q.Action) {
		return r.canonicalSequencerHistoryUIRequest(ctx, q)
	}
	if isBisectHistoryUIAction(q.Action) {
		return r.canonicalBisectHistoryUIRequest(ctx, q)
	}
	return q, nil, errors.New("invalid reviewed history action")
}

func cloneHistoryUIRequest(q HistoryUIRequest) HistoryUIRequest {
	q.Reset.Paths = append([]string(nil), q.Reset.Paths...)
	q.Revisions = append([]string(nil), q.Revisions...)
	q.Bisect.Paths = append([]string(nil), q.Bisect.Paths...)
	return q
}

func isRebaseHistoryUIAction(action HistoryUIAction) bool {
	switch action {
	case HistoryUIRebaseStart, HistoryUIRebaseContinue, HistoryUIRebaseSkip, HistoryUIRebaseAbort, HistoryUIRebaseInteractive, HistoryUIRebaseTodo:
		return true
	default:
		return false
	}
}

func isSequencerHistoryUIAction(action HistoryUIAction) bool {
	switch action {
	case HistoryUICherryStart, HistoryUICherryContinue, HistoryUICherrySkip, HistoryUICherryAbort, HistoryUIRevertStart, HistoryUIRevertContinue, HistoryUIRevertSkip, HistoryUIRevertAbort:
		return true
	default:
		return false
	}
}

func isBisectHistoryUIAction(action HistoryUIAction) bool {
	switch action {
	case HistoryUIBisectStart, HistoryUIBisectGood, HistoryUIBisectBad, HistoryUIBisectSkip, HistoryUIBisectReset:
		return true
	default:
		return false
	}
}

func (r *Repository) resolveOptionalHistoryCommit(ctx context.Context, label, revision string) (string, error) {
	if revision == "" {
		return "", nil
	}
	oid, err := r.resolveHistoryCommit(ctx, revision)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return oid, nil
}

func (r *Repository) canonicalRebaseHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	switch q.Action {
	case HistoryUIRebaseStart, HistoryUIRebaseInteractive:
		return r.canonicalRebaseStartHistoryUIRequest(ctx, q)
	case HistoryUIRebaseTodo:
		return r.canonicalRebaseTodoHistoryUIRequest(ctx, q)
	default:
		return q, []string{"Execute " + string(q.Action), "Bind to current rebase administrative state"}, nil
	}
}

func (r *Repository) canonicalRebaseStartHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	interactive := q.Action == HistoryUIRebaseInteractive
	if err := validateReviewedRebaseStart(q.Rebase, interactive); err != nil {
		return q, nil, err
	}
	canonical, err := r.resolveReviewedRebaseStart(ctx, q.Rebase)
	if err != nil {
		return q, nil, err
	}
	q.Rebase = canonical
	if interactive {
		return r.canonicalInteractiveRebasePlan(ctx, q)
	}
	plan := []string{"Rebase current branch", "Upstream " + q.Rebase.Upstream, optionalPlan("Onto ", q.Rebase.Onto)}
	return q, append(plan, rebaseOptionPlan(q.Rebase)...), nil
}

func validateReviewedRebaseStart(options RebaseOptions, interactive bool) error {
	if options.Branch != "" {
		return errors.New("reviewed UI rebase of a non-current branch is unavailable")
	}
	if interactive && options.RebaseMerges {
		return errors.New("interactive todo editor does not support merge topology commands")
	}
	return validateHistoryStrategy(options.Strategy)
}

func (r *Repository) resolveReviewedRebaseStart(ctx context.Context, options RebaseOptions) (RebaseOptions, error) {
	var err error
	options.Upstream, err = r.resolveOptionalHistoryCommit(ctx, "rebase upstream", options.Upstream)
	if err != nil {
		return options, err
	}
	if options.Upstream == "" {
		return options, errors.New("rebase upstream is empty")
	}
	options.Onto, err = r.resolveOptionalHistoryCommit(ctx, "rebase onto", options.Onto)
	return options, err
}

func (r *Repository) canonicalInteractiveRebasePlan(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	expected, err := r.rebaseTodoCommits(ctx, q.Rebase.Upstream)
	if err != nil {
		return q, nil, err
	}
	q.Rebase.Todo, err = r.canonicalRebaseTodo(ctx, q.Rebase.Todo, expected)
	if err != nil {
		return q, nil, err
	}
	plan := []string{"Start interactive rebase of current branch", "Upstream " + q.Rebase.Upstream, "Validated terminal todo:"}
	plan = append(plan, rebaseTodoPlan(q.Rebase.Todo)...)
	return q, append(plan, rebaseOptionPlan(q.Rebase)...), nil
}

func (r *Repository) canonicalRebaseTodoHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	current, err := r.ReadRebaseTodo(ctx)
	if err != nil {
		return q, nil, err
	}
	expected, err := r.rebaseTodoIDs(ctx, current)
	if err != nil {
		return q, nil, err
	}
	q.Rebase.Todo, err = r.canonicalRebaseTodo(ctx, q.Rebase.Todo, expected)
	if err != nil {
		return q, nil, err
	}
	return q, append([]string{"Replace only the current rebase todo", "Validated terminal todo:"}, rebaseTodoPlan(q.Rebase.Todo)...), nil
}

func (r *Repository) canonicalResetHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	preflight, err := r.ResetPreflight(ctx, q.Reset)
	if err != nil {
		return q, nil, err
	}
	q.Reset.Target = preflight.Target
	q.Reset.Confirmed, q.Reset.Force = false, false
	return q, []string{preflight.Summary, historyLossPlan(preflight)}, nil
}

func (r *Repository) canonicalSequencerHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	switch q.Action {
	case HistoryUICherryStart, HistoryUIRevertStart:
		return r.canonicalSequencerStartHistoryUIRequest(ctx, q)
	case HistoryUICherryContinue, HistoryUIRevertContinue:
		return q, []string{"Execute " + string(q.Action), "Bind to current sequencer state"}, nil
	case HistoryUICherrySkip, HistoryUIRevertSkip:
		return q, []string{"Skip the current sequencer item", "Discard its current index and worktree conflict state"}, nil
	default:
		return q, []string{"Abort active sequencer", "Restore the sequencer's reviewed pre-operation state"}, nil
	}
}

func (r *Repository) canonicalSequencerStartHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	verb := "Cherry-pick"
	if q.Action == HistoryUIRevertStart {
		if q.Pick.FastForward || q.Pick.RecordOrigin {
			return q, nil, errors.New("revert does not support cherry-pick fast-forward or origin recording")
		}
		verb = "Revert"
	}
	if len(q.Revisions) == 0 {
		return q, nil, errors.New(strings.ToLower(verb) + ": no commits supplied")
	}
	if q.Pick.Mainline < 0 {
		return q, nil, errors.New(strings.ToLower(verb) + ": mainline must be positive")
	}
	if err := validateHistoryStrategy(q.Pick.Strategy); err != nil {
		return q, nil, err
	}
	for i, revision := range q.Revisions {
		oid, err := r.resolveOptionalHistoryCommit(ctx, fmt.Sprintf("%s commit %d", strings.ToLower(verb), i+1), revision)
		if err != nil {
			return q, nil, err
		}
		q.Revisions[i] = oid
	}
	return q, sequencerStartPlan(q, verb), nil
}

func sequencerStartPlan(q HistoryUIRequest, verb string) []string {
	plan := []string{verb + " explicitly resolved commit"}
	for _, revision := range q.Revisions {
		plan = append(plan, "Commit "+revision)
	}
	options := []struct {
		include bool
		text    string
	}{
		{q.Pick.NoCommit, "Apply without creating a commit"},
		{q.Pick.Mainline > 0, "Mainline parent " + strconv.Itoa(q.Pick.Mainline)},
		{q.Pick.Strategy != "", "Strategy " + q.Pick.Strategy},
		{q.Pick.Signoff, "Add Signed-off-by trailer"},
		{q.Pick.FastForward, "Fast-forward when the picked commit is a direct descendant"},
		{q.Pick.RecordOrigin, "Record the source commit in the commit message"},
		{q.Pick.NoEdit || q.Action == HistoryUIRevertStart, "Git editors are disabled"},
	}
	for _, option := range options {
		if option.include {
			plan = append(plan, option.text)
		}
	}
	return plan
}

func (r *Repository) canonicalBisectHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	switch q.Action {
	case HistoryUIBisectStart:
		return r.canonicalBisectStartHistoryUIRequest(ctx, q)
	case HistoryUIBisectGood, HistoryUIBisectBad, HistoryUIBisectSkip:
		var err error
		q.Revision, err = r.resolveOptionalHistoryCommit(ctx, "bisect revision", q.Revision)
		if err != nil {
			return q, nil, err
		}
		return q, []string{"Execute " + string(q.Action), optionalPlan("Revision ", q.Revision)}, nil
	default:
		return q, []string{"End bisect and restore BISECT_START", "Bind to current bisect administrative state"}, nil
	}
}

func (r *Repository) canonicalBisectStartHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	var err error
	q.Bisect.Bad, err = r.resolveOptionalHistoryCommit(ctx, "bisect bad", q.Bisect.Bad)
	if err != nil {
		return q, nil, err
	}
	q.Bisect.Good, err = r.resolveOptionalHistoryCommit(ctx, "bisect good", q.Bisect.Good)
	if err != nil {
		return q, nil, err
	}
	plan := []string{"Start bisect", optionalPlan("Bad ", q.Bisect.Bad), optionalPlan("Good ", q.Bisect.Good)}
	if q.Bisect.NoCheckout {
		plan = append(plan, "Do not check out trial revisions")
	}
	if q.Bisect.FirstParent {
		plan = append(plan, "Follow only first parents")
	}
	if err := validateBisectTerms(q.Bisect.TermOld, q.Bisect.TermNew); err != nil {
		return q, nil, err
	}
	if q.Bisect.TermOld != "" {
		plan = append(plan, "Old term "+q.Bisect.TermOld, "New term "+q.Bisect.TermNew)
	}
	return q, plan, nil
}

func (r *Repository) rebaseTodoIDs(ctx context.Context, todo string) ([]string, error) {
	if err := ValidateRebaseTodo(todo); err != nil {
		return nil, err
	}
	var ids []string
	for _, raw := range strings.Split(todo, "\n") {
		fields := strings.Fields(raw)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		oid, err := r.resolveHistoryCommit(ctx, fields[1])
		if err != nil {
			return nil, fmt.Errorf("active rebase todo revision %q: %w", fields[1], err)
		}
		ids = append(ids, oid)
	}
	return ids, nil
}

func rebaseTodoPlan(todo string) []string {
	var plan []string
	for _, raw := range strings.Split(strings.TrimSpace(todo), "\n") {
		if line := strings.TrimSpace(raw); line != "" && !strings.HasPrefix(line, "#") {
			plan = append(plan, line)
		}
	}
	return plan
}

func rebaseOptionPlan(o RebaseOptions) []string {
	var plan []string
	if o.KeepEmpty {
		plan = append(plan, "Keep empty commits")
	}
	if o.RebaseMerges {
		plan = append(plan, "Preserve merge topology")
	}
	if o.UpdateRefs {
		plan = append(plan, "Update affected branch references")
	}
	if o.Autostash {
		plan = append(plan, "Autostash local changes during rebase")
	}
	if o.ForceRebase {
		plan = append(plan, "Replay even if fast-forward is possible")
	}
	if o.Strategy != "" {
		plan = append(plan, "Strategy "+o.Strategy)
	}
	if o.Signoff {
		plan = append(plan, "Add Signed-off-by trailers")
	}
	return plan
}

func optionalPlan(prefix, value string) string {
	if value == "" {
		return prefix + "(not set)"
	}
	return prefix + value
}

func historyLossPlan(p DestructivePreflight) string {
	var state []string
	if p.LosesHEAD {
		state = append(state, "HEAD")
	}
	if p.LosesIndex {
		state = append(state, "index")
	}
	if p.LosesWorktree {
		state = append(state, "worktree")
	}
	if len(state) == 0 {
		return "No repository component is discarded"
	}
	return "May discard " + strings.Join(state, ", ")
}

func sameHistoryUIRequest(a, b HistoryUIRequest) bool {
	return historyUIRequestIdentity(a) == historyUIRequestIdentity(b)
}

func historyUIIdentity(r ReviewedHistoryUIAction) string {
	return historyUIRequestIdentity(r.Request) + "\x00" + r.State.HEAD + "\x00" + r.State.Index + "\x00" + r.State.Worktree + "\x00" + r.State.Operation
}

func historyUIRequestIdentity(q HistoryUIRequest) string {
	return strings.Join([]string{string(q.Action), q.Rebase.Upstream, q.Rebase.Onto, q.Rebase.Branch, q.Rebase.Todo,
		strconv.FormatBool(q.Rebase.KeepEmpty), strconv.FormatBool(q.Rebase.RebaseMerges), strconv.FormatBool(q.Rebase.UpdateRefs),
		strconv.FormatBool(q.Rebase.Autostash), strconv.FormatBool(q.Rebase.ForceRebase), q.Rebase.Strategy, strconv.FormatBool(q.Rebase.Signoff),
		strconv.Itoa(int(q.Reset.Mode)), q.Reset.Target, strings.Join(q.Reset.Paths, "\x01"),
		strings.Join(q.Revisions, "\x01"), strconv.FormatBool(q.Pick.NoCommit), strconv.Itoa(q.Pick.Mainline), q.Pick.Strategy, strconv.FormatBool(q.Pick.Signoff), strconv.FormatBool(q.Pick.NoEdit), strconv.FormatBool(q.Pick.FastForward), strconv.FormatBool(q.Pick.RecordOrigin),
		q.Bisect.Bad, q.Bisect.Good, q.Bisect.TermOld, q.Bisect.TermNew, strings.Join(q.Bisect.Paths, "\x01"), strconv.FormatBool(q.Bisect.NoCheckout), strconv.FormatBool(q.Bisect.FirstParent), q.Revision}, "\x00")
}

func (r *Repository) historyUIState(ctx context.Context) (HistoryUIState, error) {
	head := r.historyUIHead(ctx)
	index, err := r.historyUIOutput(ctx, "reviewed index", "ls-files", "--stage", "-z")
	if err != nil {
		return HistoryUIState{}, err
	}
	status, err := r.historyUIOutput(ctx, "reviewed worktree status", "status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return HistoryUIState{}, err
	}
	listed, err := r.historyUIOutput(ctx, "reviewed worktree paths", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return HistoryUIState{}, err
	}
	worktree, err := r.historyUIWorktreeHash(status, listed)
	if err != nil {
		return HistoryUIState{}, err
	}
	operations, err := r.QueryOperationState(ctx)
	if err != nil {
		return HistoryUIState{}, err
	}
	operation, err := r.historyUIOperationFingerprint(operations)
	if err != nil {
		return HistoryUIState{}, err
	}
	return HistoryUIState{HEAD: head, Index: hashHistoryBytes(index), Worktree: worktree, Operation: operation}, nil
}

func (r *Repository) historyUIHead(ctx context.Context) string {
	out, err := r.output(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "(unborn)"
	}
	return trimLine(out)
}

func (r *Repository) historyUIOutput(ctx context.Context, resource string, args ...string) ([]byte, error) {
	out, truncated, err := r.outputLimited(ctx, 32<<20, args...)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, &TooLargeError{Resource: resource}
	}
	return out, nil
}

func (r *Repository) historyUIWorktreeHash(status, listed []byte) (string, error) {
	h := sha256.New()
	h.Write(status)
	paths := nonEmptyNULFields(listed)
	sort.Strings(paths)
	var total int64
	for _, path := range paths {
		var err error
		total, err = r.hashHistoryUIPath(h, path, total)
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func nonEmptyNULFields(listed []byte) []string {
	var paths []string
	for _, path := range strings.Split(string(listed), "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func (r *Repository) hashHistoryUIPath(h interface{ Write([]byte) (int, error) }, path string, total int64) (int64, error) {
	full := filepath.Join(r.workTree, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	h.Write([]byte("\x00" + path + "\x00"))
	if errors.Is(err, os.ErrNotExist) {
		h.Write([]byte("deleted"))
		return total, nil
	}
	if err != nil {
		return total, err
	}
	h.Write([]byte(info.Mode().String()))
	if info.Mode()&os.ModeSymlink != 0 {
		return total, hashHistoryUISymlink(h, full)
	}
	if !info.Mode().IsRegular() {
		h.Write([]byte(info.Mode().String()))
		return total, nil
	}
	return hashHistoryUIRegular(h, full, info.Size(), total)
}

func hashHistoryUISymlink(h interface{ Write([]byte) (int, error) }, path string) error {
	target, err := os.Readlink(path)
	if err == nil {
		h.Write([]byte("symlink\x00" + target))
	}
	return err
}

func hashHistoryUIRegular(h interface{ Write([]byte) (int, error) }, path string, size, total int64) (int64, error) {
	total += size
	if size > 16<<20 || total > 64<<20 {
		return total, &TooLargeError{Resource: "reviewed changed worktree content"}
	}
	content, err := os.ReadFile(path)
	if err == nil {
		h.Write(content)
	}
	return total, err
}

func (r *Repository) historyUIOperationFingerprint(state OperationState) (string, error) {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%#v", state.Items)))
	entries, err := os.ReadDir(r.gitDir)
	if err != nil {
		return "", err
	}
	roots := historyUIOperationRoots(r.gitDir, entries)
	sort.Strings(roots)
	var total int64
	for _, root := range roots {
		err := filepath.WalkDir(root, historyUIOperationHasher(r.gitDir, h, &total))
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func historyUIOperationRoots(gitDir string, entries []os.DirEntry) []string {
	var roots []string
	for _, entry := range entries {
		name := entry.Name()
		if name == "sequencer" || name == "rebase-merge" || name == "rebase-apply" || name == "CHERRY_PICK_HEAD" || name == "REVERT_HEAD" || name == "MERGE_HEAD" || strings.HasPrefix(name, "BISECT_") {
			roots = append(roots, filepath.Join(gitDir, name))
		}
	}
	return roots
}

func historyUIOperationHasher(gitDir string, h interface{ Write([]byte) (int, error) }, total *int64) func(string, os.DirEntry, error) error {
	return func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("operation state contains a symbolic link")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		*total += info.Size()
		if info.Size() > 2<<20 || *total > 8<<20 {
			return &TooLargeError{Resource: "reviewed operation state"}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(gitDir, path)
		if err != nil {
			return err
		}
		h.Write([]byte("\x00" + filepath.ToSlash(rel) + "\x00"))
		h.Write(content)
		return nil
	}
}

func hashHistoryBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
