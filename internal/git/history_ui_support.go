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
	if !reviewed.Token.validFor(historyUIIdentity(current)) || reviewed.State != current.State || !sameHistoryUIRequest(reviewed.Request, current.Request) {
		return ErrStalePlan
	}
	q := reviewed.Request
	switch q.Action {
	case HistoryUIRebaseStart:
		return r.RebaseStart(ctx, q.Rebase)
	case HistoryUIRebaseContinue:
		return r.RebaseContinue(ctx)
	case HistoryUIRebaseSkip:
		return r.RebaseSkip(ctx)
	case HistoryUIRebaseAbort:
		return r.RebaseAbort(ctx)
	case HistoryUIRebaseInteractive:
		return r.RebaseInteractive(ctx, q.Rebase)
	case HistoryUIRebaseTodo:
		return r.WriteRebaseTodo(ctx, q.Rebase.Todo)
	case HistoryUIReset:
		q.Reset.Confirmed = true
		return r.Reset(ctx, q.Reset)
	case HistoryUICherryStart:
		return r.CherryPickStart(ctx, q.Revisions, q.Pick)
	case HistoryUICherryContinue:
		return r.CherryPickContinue(ctx)
	case HistoryUICherrySkip:
		return r.CherryPickSkip(ctx)
	case HistoryUICherryAbort:
		return r.CherryPickAbort(ctx)
	case HistoryUIRevertStart:
		return r.RevertStart(ctx, q.Revisions, q.Pick)
	case HistoryUIRevertContinue:
		return r.RevertContinue(ctx)
	case HistoryUIRevertSkip:
		return r.RevertSkip(ctx)
	case HistoryUIRevertAbort:
		return r.RevertAbort(ctx)
	case HistoryUIBisectStart:
		return r.BisectStart(ctx, q.Bisect)
	case HistoryUIBisectGood:
		return r.BisectGood(ctx, q.Revision)
	case HistoryUIBisectBad:
		return r.BisectBad(ctx, q.Revision)
	case HistoryUIBisectSkip:
		if q.Revision == "" {
			return r.BisectSkip(ctx)
		}
		return r.BisectSkip(ctx, q.Revision)
	case HistoryUIBisectReset:
		return r.BisectReset(ctx)
	default:
		return errors.New("invalid reviewed history action")
	}
}

func (r *Repository) canonicalHistoryUIRequest(ctx context.Context, q HistoryUIRequest) (HistoryUIRequest, []string, error) {
	q.Reset.Paths = append([]string(nil), q.Reset.Paths...)
	q.Revisions = append([]string(nil), q.Revisions...)
	q.Bisect.Paths = append([]string(nil), q.Bisect.Paths...)
	resolveOptional := func(label, revision string) (string, error) {
		if revision == "" {
			return "", nil
		}
		oid, err := r.resolveHistoryCommit(ctx, revision)
		if err != nil {
			return "", fmt.Errorf("%s: %w", label, err)
		}
		return oid, nil
	}
	var err error
	switch q.Action {
	case HistoryUIRebaseStart:
		if q.Rebase.Branch != "" {
			return q, nil, errors.New("reviewed UI rebase of a non-current branch is unavailable")
		}
		if err := validateHistoryStrategy(q.Rebase.Strategy); err != nil {
			return q, nil, err
		}
		if q.Rebase.Upstream, err = resolveOptional("rebase upstream", q.Rebase.Upstream); err != nil || q.Rebase.Upstream == "" {
			if err == nil {
				err = errors.New("rebase upstream is empty")
			}
			return q, nil, err
		}
		if q.Rebase.Onto, err = resolveOptional("rebase onto", q.Rebase.Onto); err != nil {
			return q, nil, err
		}
		plan := []string{"Rebase current branch", "Upstream " + q.Rebase.Upstream, optionalPlan("Onto ", q.Rebase.Onto)}
		plan = append(plan, rebaseOptionPlan(q.Rebase)...)
		return q, plan, nil
	case HistoryUIRebaseInteractive:
		if q.Rebase.Branch != "" {
			return q, nil, errors.New("reviewed UI rebase of a non-current branch is unavailable")
		}
		if q.Rebase.RebaseMerges {
			return q, nil, errors.New("interactive todo editor does not support merge topology commands")
		}
		if err := validateHistoryStrategy(q.Rebase.Strategy); err != nil {
			return q, nil, err
		}
		if q.Rebase.Upstream, err = resolveOptional("rebase upstream", q.Rebase.Upstream); err != nil || q.Rebase.Upstream == "" {
			if err == nil {
				err = errors.New("rebase upstream is empty")
			}
			return q, nil, err
		}
		if q.Rebase.Onto, err = resolveOptional("rebase onto", q.Rebase.Onto); err != nil {
			return q, nil, err
		}
		expected, e := r.rebaseTodoCommits(ctx, q.Rebase.Upstream)
		if e != nil {
			return q, nil, e
		}
		if q.Rebase.Todo, e = r.canonicalRebaseTodo(ctx, q.Rebase.Todo, expected); e != nil {
			return q, nil, e
		}
		plan := []string{"Start interactive rebase of current branch", "Upstream " + q.Rebase.Upstream, "Validated terminal todo:"}
		plan = append(plan, rebaseTodoPlan(q.Rebase.Todo)...)
		plan = append(plan, rebaseOptionPlan(q.Rebase)...)
		return q, plan, nil
	case HistoryUIRebaseTodo:
		current, e := r.ReadRebaseTodo(ctx)
		if e != nil {
			return q, nil, e
		}
		expected, e := r.rebaseTodoIDs(ctx, current)
		if e != nil {
			return q, nil, e
		}
		if q.Rebase.Todo, e = r.canonicalRebaseTodo(ctx, q.Rebase.Todo, expected); e != nil {
			return q, nil, e
		}
		return q, append([]string{"Replace only the current rebase todo", "Validated terminal todo:"}, rebaseTodoPlan(q.Rebase.Todo)...), nil
	case HistoryUIRebaseContinue, HistoryUIRebaseSkip, HistoryUIRebaseAbort:
		return q, []string{"Execute " + string(q.Action), "Bind to current rebase administrative state"}, nil
	case HistoryUIReset:
		preflight, e := r.ResetPreflight(ctx, q.Reset)
		if e != nil {
			return q, nil, e
		}
		q.Reset.Target = preflight.Target
		q.Reset.Confirmed, q.Reset.Force = false, false
		return q, []string{preflight.Summary, historyLossPlan(preflight)}, nil
	case HistoryUICherryStart, HistoryUIRevertStart:
		verb := "Cherry-pick"
		if q.Action == HistoryUIRevertStart {
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
			if q.Revisions[i], err = resolveOptional(fmt.Sprintf("%s commit %d", strings.ToLower(verb), i+1), revision); err != nil {
				return q, nil, err
			}
		}
		plan := []string{verb + " explicitly resolved commit"}
		for _, revision := range q.Revisions {
			plan = append(plan, "Commit "+revision)
		}
		if q.Pick.NoCommit {
			plan = append(plan, "Apply without creating a commit")
		}
		if q.Pick.Mainline > 0 {
			plan = append(plan, "Mainline parent "+strconv.Itoa(q.Pick.Mainline))
		}
		if q.Pick.Strategy != "" {
			plan = append(plan, "Strategy "+q.Pick.Strategy)
		}
		if q.Pick.Signoff {
			plan = append(plan, "Add Signed-off-by trailer")
		}
		if q.Pick.NoEdit || q.Action == HistoryUIRevertStart {
			plan = append(plan, "Git editors are disabled")
		}
		return q, plan, nil
	case HistoryUICherryContinue, HistoryUIRevertContinue:
		return q, []string{"Execute " + string(q.Action), "Bind to current sequencer state"}, nil
	case HistoryUICherrySkip, HistoryUIRevertSkip:
		return q, []string{"Skip the current sequencer item", "Discard its current index and worktree conflict state"}, nil
	case HistoryUICherryAbort, HistoryUIRevertAbort:
		return q, []string{"Abort active sequencer", "Restore the sequencer's reviewed pre-operation state"}, nil
	case HistoryUIBisectStart:
		if q.Bisect.Bad, err = resolveOptional("bisect bad", q.Bisect.Bad); err != nil {
			return q, nil, err
		}
		if q.Bisect.Good, err = resolveOptional("bisect good", q.Bisect.Good); err != nil {
			return q, nil, err
		}
		plan := []string{"Start bisect", optionalPlan("Bad ", q.Bisect.Bad), optionalPlan("Good ", q.Bisect.Good)}
		if q.Bisect.NoCheckout {
			plan = append(plan, "Do not check out trial revisions")
		}
		if q.Bisect.FirstParent {
			plan = append(plan, "Follow only first parents")
		}
		return q, plan, nil
	case HistoryUIBisectGood, HistoryUIBisectBad, HistoryUIBisectSkip:
		if q.Revision, err = resolveOptional("bisect revision", q.Revision); err != nil {
			return q, nil, err
		}
		return q, []string{"Execute " + string(q.Action), optionalPlan("Revision ", q.Revision)}, nil
	case HistoryUIBisectReset:
		return q, []string{"End bisect and restore BISECT_START", "Bind to current bisect administrative state"}, nil
	default:
		return q, nil, errors.New("invalid reviewed history action")
	}
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
		strings.Join(q.Revisions, "\x01"), strconv.FormatBool(q.Pick.NoCommit), strconv.Itoa(q.Pick.Mainline), q.Pick.Strategy, strconv.FormatBool(q.Pick.Signoff), strconv.FormatBool(q.Pick.NoEdit),
		q.Bisect.Bad, q.Bisect.Good, strings.Join(q.Bisect.Paths, "\x01"), strconv.FormatBool(q.Bisect.NoCheckout), strconv.FormatBool(q.Bisect.FirstParent), q.Revision}, "\x00")
}

func (r *Repository) historyUIState(ctx context.Context) (HistoryUIState, error) {
	head := "(unborn)"
	if out, err := r.output(ctx, "rev-parse", "--verify", "HEAD"); err == nil {
		head = trimLine(out)
	}
	index, truncated, err := r.outputLimited(ctx, 32<<20, "ls-files", "--stage", "-z")
	if err != nil {
		return HistoryUIState{}, err
	}
	if truncated {
		return HistoryUIState{}, &TooLargeError{Resource: "reviewed index"}
	}
	status, truncated, err := r.outputLimited(ctx, 32<<20, "status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return HistoryUIState{}, err
	}
	if truncated {
		return HistoryUIState{}, &TooLargeError{Resource: "reviewed worktree status"}
	}
	listed, pathsTruncated, err := r.outputLimited(ctx, 32<<20, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return HistoryUIState{}, err
	}
	if pathsTruncated {
		return HistoryUIState{}, &TooLargeError{Resource: "reviewed worktree paths"}
	}
	workHash := sha256.New()
	workHash.Write(status)
	var paths []string
	for _, path := range strings.Split(string(listed), "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var total int64
	for _, path := range paths {
		if strings.ContainsRune(path, '\x00') {
			return HistoryUIState{}, errors.New("worktree path contains NUL")
		}
		full := filepath.Join(r.workTree, filepath.FromSlash(path))
		info, statErr := os.Lstat(full)
		workHash.Write([]byte("\x00" + path + "\x00"))
		if errors.Is(statErr, os.ErrNotExist) {
			workHash.Write([]byte("deleted"))
			continue
		}
		if statErr != nil {
			return HistoryUIState{}, statErr
		}
		workHash.Write([]byte(info.Mode().String()))
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(full)
			if e != nil {
				return HistoryUIState{}, e
			}
			workHash.Write([]byte("symlink\x00" + target))
			continue
		}
		if !info.Mode().IsRegular() {
			workHash.Write([]byte(info.Mode().String()))
			continue
		}
		total += info.Size()
		if info.Size() > 16<<20 || total > 64<<20 {
			return HistoryUIState{}, &TooLargeError{Resource: "reviewed changed worktree content"}
		}
		content, e := os.ReadFile(full)
		if e != nil {
			return HistoryUIState{}, e
		}
		workHash.Write(content)
	}
	operations, err := r.QueryOperationState(ctx)
	if err != nil {
		return HistoryUIState{}, err
	}
	operation, err := r.historyUIOperationFingerprint(operations)
	if err != nil {
		return HistoryUIState{}, err
	}
	return HistoryUIState{HEAD: head, Index: hashHistoryBytes(index), Worktree: hex.EncodeToString(workHash.Sum(nil)), Operation: operation}, nil
}

func (r *Repository) historyUIOperationFingerprint(state OperationState) (string, error) {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%#v", state.Items)))
	entries, err := os.ReadDir(r.gitDir)
	if err != nil {
		return "", err
	}
	var roots []string
	for _, entry := range entries {
		name := entry.Name()
		if name == "sequencer" || name == "rebase-merge" || name == "rebase-apply" ||
			name == "CHERRY_PICK_HEAD" || name == "REVERT_HEAD" || name == "MERGE_HEAD" ||
			strings.HasPrefix(name, "BISECT_") {
			roots = append(roots, filepath.Join(r.gitDir, name))
		}
	}
	sort.Strings(roots)
	var total int64
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
			total += info.Size()
			if info.Size() > 2<<20 || total > 8<<20 {
				return &TooLargeError{Resource: "reviewed operation state"}
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(r.gitDir, path)
			if err != nil {
				return err
			}
			h.Write([]byte("\x00" + filepath.ToSlash(rel) + "\x00"))
			h.Write(content)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashHistoryBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
