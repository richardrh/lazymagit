package git

// This file contains the history-changing workflows.  Keep the argument
// construction here deliberately boring: user supplied revisions are resolved
// to object IDs before they are passed to a mutating command, and paths always
// follow `--`.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrWorkflowNotActive = errors.New("history workflow is not active")
)

// HistoryConfirmationRequiredError is returned before a destructive history
// command is run. It carries richer preflight data than the package's generic
// confirmation error.
// Preflight is suitable for presenting directly in a confirmation dialog.
type HistoryConfirmationRequiredError struct{ Preflight DestructivePreflight }

func (e *HistoryConfirmationRequiredError) Error() string {
	return ErrConfirmationRequired.Error() + ": " + e.Preflight.Summary
}
func (e *HistoryConfirmationRequiredError) Unwrap() error { return ErrConfirmationRequired }

type ConfirmOptions struct {
	Force     bool
	Confirmed bool
}

func (o ConfirmOptions) allowed() bool { return o.Force || o.Confirmed }

type DestructivePreflight struct {
	Operation     string
	Summary       string
	Target        string
	Paths         []string
	LosesHEAD     bool
	LosesIndex    bool
	LosesWorktree bool
}

type PickOptions struct {
	NoCommit     bool
	Mainline     int
	Strategy     string
	Signoff      bool
	NoEdit       bool
	FastForward  bool
	RecordOrigin bool
}

// The aliases make call sites self-documenting while retaining one shared set
// of flags (the two Git commands intentionally accept the same controls here).
type CherryPickOptions = PickOptions
type RevertOptions = PickOptions

func (r *Repository) CherryPickStart(ctx context.Context, commits []string, opts PickOptions) error {
	return r.historyApplyStart(ctx, "cherry-pick", commits, opts)
}

func (r *Repository) RevertStart(ctx context.Context, commits []string, opts PickOptions) error {
	return r.historyApplyStart(ctx, "revert", commits, opts)
}

func (r *Repository) historyApplyStart(ctx context.Context, verb string, revisions []string, opts PickOptions) error {
	if verb != "cherry-pick" && (opts.FastForward || opts.RecordOrigin) {
		return fmt.Errorf("%s does not support cherry-pick fast-forward or origin recording", verb)
	}
	if len(revisions) == 0 {
		return errors.New(verb + ": no commits supplied")
	}
	if opts.Mainline < 0 {
		return errors.New(verb + ": mainline must be positive")
	}
	if err := validateHistoryStrategy(opts.Strategy); err != nil {
		return err
	}
	oids := make([]string, len(revisions))
	for i, revision := range revisions {
		oid, err := r.resolveHistoryCommit(ctx, revision)
		if err != nil {
			return fmt.Errorf("%s commit %d: %w", verb, i+1, err)
		}
		oids[i] = oid
	}
	args := []string{verb}
	if opts.NoCommit {
		args = append(args, "--no-commit")
	}
	if opts.Mainline > 0 {
		args = append(args, "--mainline", strconv.Itoa(opts.Mainline))
	}
	if opts.Strategy != "" {
		args = append(args, "--strategy="+opts.Strategy)
	}
	if opts.Signoff {
		args = append(args, "--signoff")
	}
	if opts.NoEdit {
		args = append(args, "--no-edit")
	}
	if opts.FastForward {
		args = append(args, "--ff")
	}
	if opts.RecordOrigin {
		args = append(args, "-x")
	}
	if verb == "revert" && !opts.NoEdit {
		// Revert's backend default is deliberately non-interactive.
		args = append(args, "--no-edit")
	}
	args = append(args, "--")
	args = append(args, oids...)
	return r.run(ctx, args...)
}

func validateHistoryStrategy(strategy string) error {
	if strategy == "" {
		return nil
	}
	if strings.TrimSpace(strategy) != strategy || strings.HasPrefix(strategy, "-") || strings.ContainsAny(strategy, "\x00/\\") {
		return fmt.Errorf("invalid merge strategy %q", strategy)
	}
	for _, c := range strategy {
		if !(c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return fmt.Errorf("invalid merge strategy %q", strategy)
		}
	}
	return nil
}

func (r *Repository) CherryPickContinue(ctx context.Context) error {
	return r.continueHistorySequence(ctx, "cherry-pick", "--continue")
}
func (r *Repository) CherryPickSkip(ctx context.Context) error {
	return r.continueHistorySequence(ctx, "cherry-pick", "--skip")
}
func (r *Repository) CherryPickAbort(ctx context.Context) error {
	return r.continueHistorySequence(ctx, "cherry-pick", "--abort")
}
func (r *Repository) RevertContinue(ctx context.Context) error {
	return r.continueHistorySequence(ctx, "revert", "--continue")
}
func (r *Repository) RevertSkip(ctx context.Context) error {
	return r.continueHistorySequence(ctx, "revert", "--skip")
}
func (r *Repository) RevertAbort(ctx context.Context) error {
	return r.continueHistorySequence(ctx, "revert", "--abort")
}

func (r *Repository) continueHistorySequence(ctx context.Context, verb, action string) error {
	if !r.historySequenceActive(verb) {
		return fmt.Errorf("%w: %s", ErrWorkflowNotActive, verb)
	}
	return r.run(ctx, verb, action)
}

func (r *Repository) historySequenceActive(verb string) bool {
	marker := "CHERRY_PICK_HEAD"
	if verb == "revert" {
		marker = "REVERT_HEAD"
	}
	if historyPathExists(filepath.Join(r.gitDir, marker)) {
		return true
	}
	b, err := readFileLimited(filepath.Join(r.gitDir, "sequencer", "todo"), 1<<20)
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(b)), map[string]string{"cherry-pick": "pick ", "revert": "revert "}[verb])
}

type RebaseOptions struct {
	Upstream string
	Onto     string
	Branch   string
	// These non-interactive flags are deliberately a closed typed set. In
	// particular there is no Interactive or Exec field: the TUI cannot safely
	// model Magit's editable multiline todo or arbitrary command semantics.
	KeepEmpty    bool
	RebaseMerges bool
	UpdateRefs   bool
	Autostash    bool
	ForceRebase  bool
	Strategy     string
	Signoff      bool
	// Todo is accepted only by RebaseInteractive. It is deliberately not an
	// argv fragment: every line is parsed and its revision is resolved before
	// Git sees it.
	Todo string
}

// RebaseStart performs a non-interactive rebase. Upstream is required; Onto
// and Branch are optional and are independently revision-validated.
func (r *Repository) RebaseStart(ctx context.Context, opts RebaseOptions) error {
	if strings.TrimSpace(opts.Upstream) == "" {
		return errors.New("rebase upstream is empty")
	}
	if err := validateHistoryStrategy(opts.Strategy); err != nil {
		return err
	}
	upstream, err := r.resolveHistoryCommit(ctx, opts.Upstream)
	if err != nil {
		return fmt.Errorf("rebase upstream: %w", err)
	}
	args := []string{"rebase"}
	if opts.KeepEmpty {
		args = append(args, "--keep-empty")
	}
	if opts.RebaseMerges {
		args = append(args, "--rebase-merges")
	}
	if opts.UpdateRefs {
		args = append(args, "--update-refs")
	}
	if opts.Autostash {
		args = append(args, "--autostash")
	}
	if opts.ForceRebase {
		args = append(args, "--force-rebase")
	}
	if opts.Strategy != "" {
		args = append(args, "--strategy="+opts.Strategy)
	}
	if opts.Signoff {
		args = append(args, "--signoff")
	}
	if opts.Onto != "" {
		onto, err := r.resolveHistoryCommit(ctx, opts.Onto)
		if err != nil {
			return fmt.Errorf("rebase onto: %w", err)
		}
		args = append(args, "--onto", onto)
	}
	args = append(args, "--", upstream)
	if opts.Branch != "" {
		branch, err := r.resolveHistoryBranch(ctx, opts.Branch)
		if err != nil {
			return fmt.Errorf("rebase branch: %w", err)
		}
		args = append(args, branch)
	}
	return r.run(ctx, args...)
}

func (r *Repository) RebaseUpstream(ctx context.Context, upstream string) error {
	return r.RebaseStart(ctx, RebaseOptions{Upstream: upstream})
}

func (r *Repository) RebaseOnto(ctx context.Context, onto, upstream string) error {
	return r.RebaseStart(ctx, RebaseOptions{Onto: onto, Upstream: upstream})
}

func (r *Repository) RebaseContinue(ctx context.Context) error {
	return r.rebaseAction(ctx, "--continue")
}
func (r *Repository) RebaseSkip(ctx context.Context) error  { return r.rebaseAction(ctx, "--skip") }
func (r *Repository) RebaseAbort(ctx context.Context) error { return r.rebaseAction(ctx, "--abort") }
func (r *Repository) rebaseAction(ctx context.Context, action string) error {
	if _, err := r.historyRebaseAdminDir(); err != nil {
		return err
	}
	return r.run(ctx, "rebase", action)
}

// DefaultRebaseTodo returns the bounded, terminal-editable todo for replaying
// the current branch above upstream. Subjects are intentionally omitted: Git
// needs only the resolved object name, and the absence of free-form comments
// makes the reviewed plan unambiguous.
func (r *Repository) DefaultRebaseTodo(ctx context.Context, upstream string) (string, error) {
	base, err := r.resolveHistoryCommit(ctx, upstream)
	if err != nil {
		return "", fmt.Errorf("rebase upstream: %w", err)
	}
	out, err := r.output(ctx, "rev-list", "--reverse", "--no-merges", base+"..HEAD")
	if err != nil {
		return "", fmt.Errorf("list rebase commits: %w", err)
	}
	var todo strings.Builder
	for _, oid := range strings.Fields(string(out)) {
		todo.WriteString("pick ")
		todo.WriteString(oid)
		todo.WriteByte('\n')
	}
	if todo.Len() == 0 {
		return "", errors.New("interactive rebase has no non-merge commits to replay")
	}
	return todo.String(), nil
}

// RebaseInteractive starts Git's sequencer with a todo which has already been
// parsed, revision-resolved, and reviewed by the TUI. Git requires a sequence
// editor callback; this uses only lazymagit's sealed, one-shot helper rather
// than an external editor or a user-controlled shell command.
func (r *Repository) RebaseInteractive(ctx context.Context, opts RebaseOptions) error {
	if strings.TrimSpace(opts.Upstream) == "" {
		return errors.New("rebase upstream is empty")
	}
	if opts.RebaseMerges {
		return errors.New("interactive todo editor does not support merge topology commands")
	}
	if err := validateHistoryStrategy(opts.Strategy); err != nil {
		return err
	}
	upstream, err := r.resolveHistoryCommit(ctx, opts.Upstream)
	if err != nil {
		return fmt.Errorf("rebase upstream: %w", err)
	}
	expected, err := r.rebaseTodoCommits(ctx, upstream)
	if err != nil {
		return err
	}
	todo, err := r.canonicalRebaseTodo(ctx, opts.Todo, expected)
	if err != nil {
		return err
	}
	editor, extraEnv, cleanup, err := newRebaseTodoEditor(todo, r.gitDir)
	if err != nil {
		return err
	}
	defer cleanup()
	args := []string{"rebase", "--interactive"}
	if opts.KeepEmpty {
		args = append(args, "--keep-empty")
	}
	if opts.UpdateRefs {
		args = append(args, "--update-refs")
	}
	if opts.Autostash {
		args = append(args, "--autostash")
	}
	if opts.ForceRebase {
		args = append(args, "--force-rebase")
	}
	if opts.Strategy != "" {
		args = append(args, "--strategy="+opts.Strategy)
	}
	if opts.Signoff {
		args = append(args, "--signoff")
	}
	if opts.Onto != "" {
		onto, e := r.resolveHistoryCommit(ctx, opts.Onto)
		if e != nil {
			return fmt.Errorf("rebase onto: %w", e)
		}
		args = append(args, "--onto", onto)
	}
	args = append(args, "--", upstream)
	ctx = context.WithValue(ctx, gitSequenceEditorKey{}, editor)
	ctx = context.WithValue(ctx, gitExtraEnvKey{}, extraEnv)
	return r.run(ctx, args...)
}

func (r *Repository) rebaseTodoCommits(ctx context.Context, upstream string) ([]string, error) {
	out, err := r.output(ctx, "rev-list", "--reverse", "--no-merges", upstream+"..HEAD")
	if err != nil {
		return nil, fmt.Errorf("list rebase commits: %w", err)
	}
	commits := strings.Fields(string(out))
	if len(commits) == 0 {
		return nil, errors.New("interactive rebase has no non-merge commits to replay")
	}
	return commits, nil
}

func (r *Repository) historyRebaseAdminDir() (string, error) {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		p := filepath.Join(r.gitDir, name)
		if historyPathExists(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: rebase", ErrWorkflowNotActive)
}

func (r *Repository) ReadRebaseTodo(ctx context.Context) (string, error) {
	_ = ctx
	dir, err := r.historyRebaseAdminDir()
	if err != nil {
		return "", err
	}
	b, err := readFileLimited(filepath.Join(dir, "git-rebase-todo"), 1<<20)
	if err != nil {
		return "", fmt.Errorf("read rebase todo: %w", err)
	}
	if bytes.Count(b, []byte{'\n'}) > 100000 {
		return "", &TooLargeError{Resource: "rebase todo item count"}
	}
	return string(b), nil
}

// ValidateRebaseTodo enforces the terminal editor's closed instruction set.
// exec is deliberately rejected: Git executes its payload through a shell.
func ValidateRebaseTodo(todo string) error {
	if len(todo) > 1<<20 {
		return &TooLargeError{Resource: "rebase todo"}
	}
	if strings.Count(todo, "\n") > 100000 {
		return &TooLargeError{Resource: "rebase todo item count"}
	}
	for n, raw := range strings.Split(todo, "\n") {
		if strings.ContainsRune(raw, '\x00') || strings.ContainsRune(raw, '\r') {
			return fmt.Errorf("invalid rebase todo line %d: control character", n+1)
		}
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("invalid rebase todo line %d: %q", n+1, line)
		}
		switch fields[0] {
		case "pick", "reword", "edit", "squash", "fixup", "drop":
		default:
			return fmt.Errorf("invalid rebase todo line %d: command %q is not permitted (exec is never permitted)", n+1, fields[0])
		}
	}
	return nil
}

func (r *Repository) canonicalRebaseTodo(ctx context.Context, todo string, expected []string) (string, error) {
	if err := ValidateRebaseTodo(todo); err != nil {
		return "", err
	}
	allowed := make(map[string]bool, len(expected))
	for _, oid := range expected {
		allowed[oid] = true
	}
	seen := make(map[string]bool, len(expected))
	var out strings.Builder
	applied := false
	for lineNumber, raw := range strings.Split(todo, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		oid, err := r.resolveHistoryCommit(ctx, fields[1])
		if err != nil || !allowed[oid] {
			return "", fmt.Errorf("rebase todo line %d: revision %q is not a commit selected for this rebase", lineNumber+1, fields[1])
		}
		if seen[oid] {
			return "", fmt.Errorf("rebase todo line %d: commit %s appears more than once", lineNumber+1, oid)
		}
		if (fields[0] == "squash" || fields[0] == "fixup") && !applied {
			return "", fmt.Errorf("rebase todo line %d: %s requires an earlier replayed commit", lineNumber+1, fields[0])
		}
		seen[oid] = true
		if fields[0] != "drop" {
			applied = true
		}
		out.WriteString(fields[0])
		out.WriteByte(' ')
		out.WriteString(oid)
		if len(fields) > 2 {
			out.WriteByte(' ')
			out.WriteString(strings.Join(fields[2:], " "))
		}
		out.WriteByte('\n')
	}
	if len(seen) != len(expected) {
		return "", errors.New("rebase todo must contain every selected commit exactly once")
	}
	return out.String(), nil
}

func newRebaseTodoEditor(todo, gitDir string) (string, []string, func(), error) {
	executable, err := os.Executable()
	if err != nil {
		return "", nil, nil, fmt.Errorf("locate internal rebase editor: %w", err)
	}
	dir, err := os.MkdirTemp("", "lazymagit-rebase-todo-")
	if err != nil {
		return "", nil, nil, err
	}
	source := filepath.Join(dir, "todo")
	if err := os.WriteFile(source, []byte(todo), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if strings.HasSuffix(executable, ".test") {
		// Go test binaries parse their arguments before TestMain. Use a normal
		// test selector and a private environment transport for integration
		// tests; production always uses the sealed helper argv below.
		return shellQuoteArgs([]string{executable, "-test.run=^TestRebaseTodoEditorHelper$"}), []string{"LAZYMAGIT_REBASE_TODO_SOURCE=" + source, "LAZYMAGIT_REBASE_TODO_GIT_DIR=" + filepath.Clean(gitDir)}, cleanup, nil
	}
	args := []string{executable, "--lazymagit-rebase-todo-editor", source, filepath.Clean(gitDir)}
	return shellQuoteArgs(args), nil, cleanup, nil
}

func shellQuoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\\"'\\\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

// RunRebaseTodoEditor is the sealed helper mode used by the running lazymagit
// binary. It accepts only its private source and Git's exact todo destination.
func RunRebaseTodoEditor(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "--lazymagit-rebase-todo-editor" {
		return false, nil
	}
	if len(args) != 4 {
		return true, errors.New("invalid internal rebase editor invocation")
	}
	source, admin, destination := args[1], args[2], args[3]
	admin = filepath.Clean(admin)
	expected := filepath.Join(admin, "rebase-merge", "git-rebase-todo")
	// macOS commonly presents /var through the /private/var symlink to Git.
	// Compare resolved paths, while retaining the supplied destination for the
	// atomic replacement below.
	if resolved, err := filepath.EvalSymlinks(expected); err == nil {
		expected = resolved
	}
	actual := filepath.Clean(destination)
	if resolved, err := filepath.EvalSymlinks(actual); err == nil {
		actual = resolved
	}
	if actual != expected {
		return true, errors.New("internal rebase editor refused an unexpected destination")
	}
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return true, errors.New("internal rebase editor source is unsafe")
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return true, err
	}
	if err := ValidateRebaseTodo(string(contents)); err != nil {
		return true, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), "lazymagit-todo-")
	if err != nil {
		return true, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(contents); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, destination)
	}
	return true, err
}

func (r *Repository) WriteRebaseTodo(ctx context.Context, todo string) error {
	dir, err := r.historyRebaseAdminDir()
	if err != nil {
		return err
	}
	current, err := r.ReadRebaseTodo(ctx)
	if err != nil {
		return err
	}
	expected, err := r.rebaseTodoIDs(ctx, current)
	if err != nil {
		return err
	}
	todo, err = r.canonicalRebaseTodo(ctx, todo, expected)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "git-rebase-todo")
	// The todo is Git administrative state, not a worktree path. Atomic replace
	// prevents an interrupted caller from leaving a partial instruction.
	tmp, err := os.CreateTemp(dir, "lazymagit-todo-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.WriteString(todo); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return nil
}

type ResetMode uint8

const (
	ResetMixed ResetMode = iota
	ResetSoft
	ResetHard
	ResetKeep
	ResetIndex
	ResetWorktree
	ResetFile
)

type ResetOptions struct {
	Mode   ResetMode
	Target string
	Paths  []string
	ConfirmOptions
}

func (r *Repository) ResetPreflight(ctx context.Context, opts ResetOptions) (DestructivePreflight, error) {
	if opts.Mode > ResetFile {
		return DestructivePreflight{}, errors.New("invalid reset mode")
	}
	target := opts.Target
	if target == "" {
		target = "HEAD"
	}
	oid, err := r.resolveHistoryCommit(ctx, target)
	if err != nil {
		return DestructivePreflight{}, fmt.Errorf("reset target: %w", err)
	}
	if opts.Mode == ResetFile && len(opts.Paths) != 1 {
		return DestructivePreflight{}, errors.New("file reset requires exactly one path")
	}
	if opts.Mode != ResetFile && opts.Mode != ResetWorktree && opts.Mode != ResetIndex && len(opts.Paths) != 0 {
		return DestructivePreflight{}, errors.New("paths are only valid for index/worktree/file reset")
	}
	p := DestructivePreflight{Operation: "reset", Target: oid, Paths: append([]string(nil), opts.Paths...), Summary: "reset repository state to " + oid}
	switch opts.Mode {
	case ResetSoft:
		p.LosesHEAD = true
	case ResetMixed:
		p.LosesHEAD = true
		p.LosesIndex = true
	case ResetHard, ResetKeep:
		p.LosesHEAD = true
		p.LosesIndex = true
		p.LosesWorktree = true
	case ResetIndex:
		p.LosesIndex = true
	case ResetWorktree, ResetFile:
		p.LosesWorktree = true
	}
	return p, nil
}

func (r *Repository) Reset(ctx context.Context, opts ResetOptions) error {
	p, err := r.ResetPreflight(ctx, opts)
	if err != nil {
		return err
	}
	if !opts.allowed() {
		return &HistoryConfirmationRequiredError{Preflight: p}
	}
	switch opts.Mode {
	case ResetMixed:
		return r.run(ctx, "reset", "--mixed", p.Target)
	case ResetSoft:
		return r.run(ctx, "reset", "--soft", p.Target)
	case ResetHard:
		return r.run(ctx, "reset", "--hard", p.Target)
	case ResetKeep:
		return r.run(ctx, "reset", "--keep", p.Target)
	case ResetIndex:
		paths := p.Paths
		if len(paths) == 0 {
			paths = []string{"."}
		}
		return r.run(ctx, pathsArgs([]string{"reset", p.Target}, paths)...)
	case ResetWorktree:
		paths := p.Paths
		if len(paths) == 0 {
			paths = []string{"."}
		}
		return r.run(ctx, pathsArgs([]string{"restore", "--worktree", "--source=" + p.Target}, paths)...)
	case ResetFile:
		return r.run(ctx, pathsArgs([]string{"restore", "--staged", "--worktree", "--source=" + p.Target}, p.Paths)...)
	default:
		return errors.New("invalid reset mode")
	}
}

type BisectStartOptions struct {
	Bad, Good   string
	Paths       []string
	NoCheckout  bool
	FirstParent bool
}

func (r *Repository) BisectStart(ctx context.Context, o BisectStartOptions) error {
	if r.historyBisectActive() {
		return errors.New("bisect is already active")
	}
	if o.Good != "" && o.Bad == "" {
		return errors.New("bisect start cannot supply good without bad")
	}
	args := []string{"bisect", "start"}
	if o.NoCheckout {
		args = append(args, "--no-checkout")
	}
	if o.FirstParent {
		args = append(args, "--first-parent")
	}
	if o.Bad != "" {
		bad, err := r.resolveHistoryCommit(ctx, o.Bad)
		if err != nil {
			return fmt.Errorf("bisect bad: %w", err)
		}
		args = append(args, bad)
	}
	if o.Good != "" {
		good, err := r.resolveHistoryCommit(ctx, o.Good)
		if err != nil {
			return fmt.Errorf("bisect good: %w", err)
		}
		args = append(args, good)
	}
	if len(o.Paths) > 0 {
		args = append(args, "--")
		args = append(args, o.Paths...)
	}
	return r.run(ctx, args...)
}
func (r *Repository) BisectGood(ctx context.Context, revision string) error {
	return r.bisectMark(ctx, "good", revision)
}
func (r *Repository) BisectBad(ctx context.Context, revision string) error {
	return r.bisectMark(ctx, "bad", revision)
}
func (r *Repository) BisectSkip(ctx context.Context, revisions ...string) error {
	if !r.historyBisectActive() {
		return fmt.Errorf("%w: bisect", ErrWorkflowNotActive)
	}
	args := []string{"bisect", "skip"}
	for _, rev := range revisions {
		oid, err := r.resolveHistoryCommit(ctx, rev)
		if err != nil {
			return err
		}
		args = append(args, oid)
	}
	return r.run(ctx, args...)
}
func (r *Repository) bisectMark(ctx context.Context, mark, revision string) error {
	if !r.historyBisectActive() {
		return fmt.Errorf("%w: bisect", ErrWorkflowNotActive)
	}
	args := []string{"bisect", mark}
	if revision != "" {
		oid, err := r.resolveHistoryCommit(ctx, revision)
		if err != nil {
			return err
		}
		args = append(args, oid)
	}
	return r.run(ctx, args...)
}
func (r *Repository) BisectReset(ctx context.Context) error {
	if !r.historyBisectActive() {
		return fmt.Errorf("%w: bisect", ErrWorkflowNotActive)
	}
	return r.run(ctx, "bisect", "reset")
}
func (r *Repository) UnsafeBisectRun(ctx context.Context, capability AllowUnsafeExecution, argv []string) error {
	if !capability.allowed() {
		return ErrUnsafeExecution
	}
	if !r.historyBisectActive() {
		return fmt.Errorf("%w: bisect", ErrWorkflowNotActive)
	}
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("bisect run requires explicit command argv")
	}
	for _, arg := range argv {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("bisect run argv contains NUL")
		}
	}
	return r.run(ctx, append([]string{"bisect", "run"}, argv...)...)
}

func (r *Repository) historyBisectActive() bool {
	return historyPathExists(filepath.Join(r.gitDir, "BISECT_START"))
}

type NotesRemoveOptions struct {
	Ref     string
	Objects []string
	ConfirmOptions
}

func (r *Repository) NotesRemove(ctx context.Context, o NotesRemoveOptions) error {
	if len(o.Objects) == 0 {
		return errors.New("notes remove requires objects")
	}
	p := DestructivePreflight{Operation: "notes remove", Summary: "remove notes from selected objects"}
	if !o.allowed() {
		return &HistoryConfirmationRequiredError{Preflight: p}
	}
	args := historyNotesArgs(o.Ref, "remove")
	args = append(args, "--")
	args = append(args, o.Objects...)
	return r.run(ctx, args...)
}
func (r *Repository) NotesPrune(ctx context.Context, ref string, confirm ConfirmOptions) error {
	p := DestructivePreflight{Operation: "notes prune", Summary: "prune unreachable notes"}
	if !confirm.allowed() {
		return &HistoryConfirmationRequiredError{Preflight: p}
	}
	return r.run(ctx, historyNotesArgs(ref, "prune")...)
}
func (r *Repository) NotesMergeStart(ctx context.Context, ref, notesRef, strategy string) error {
	if strings.TrimSpace(notesRef) == "" {
		return errors.New("notes merge ref is empty")
	}
	if !validNotesMergeStrategy(strategy) {
		return fmt.Errorf("invalid notes merge strategy %q", strategy)
	}
	args := historyNotesArgs(ref, "merge")
	if strategy != "" {
		args = append(args, "--strategy="+strategy)
	}
	args = append(args, "--", notesRef)
	return r.run(ctx, args...)
}

func validNotesMergeStrategy(strategy string) bool {
	switch strategy {
	case "", "manual", "ours", "theirs", "union", "cat_sort_uniq":
		return true
	default:
		return false
	}
}
func (r *Repository) NotesMergeContinue(ctx context.Context, ref string) error {
	if !r.historyNotesMergeActive() {
		return fmt.Errorf("%w: notes merge", ErrWorkflowNotActive)
	}
	return r.run(ctx, historyNotesArgs(ref, "merge", "--commit")...)
}
func (r *Repository) NotesMergeAbort(ctx context.Context, ref string) error {
	if !r.historyNotesMergeActive() {
		return fmt.Errorf("%w: notes merge", ErrWorkflowNotActive)
	}
	return r.run(ctx, historyNotesArgs(ref, "merge", "--abort")...)
}
func (r *Repository) NotesEdit(ctx context.Context, ref, object string) error {
	return &EditorRequiredError{Operation: "notes edit"}
}
func historyNotesArgs(ref string, rest ...string) []string {
	a := []string{"notes"}
	if ref != "" {
		a = append(a, "--ref", ref)
	}
	return append(a, rest...)
}

func (r *Repository) historyNotesMergeActive() bool {
	return historyPathExists(filepath.Join(r.gitDir, "NOTES_MERGE_PARTIAL")) || historyPathExists(filepath.Join(r.gitDir, "NOTES_MERGE_REF"))
}

func (r *Repository) resolveHistoryCommit(ctx context.Context, revision string) (string, error) {
	if strings.TrimSpace(revision) == "" || strings.ContainsRune(revision, '\x00') {
		return "", errors.New("revision is empty or invalid")
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	oid := trimLine(out)
	if oid == "" {
		return "", errors.New("resolved revision is empty")
	}
	return oid, nil
}

func (r *Repository) resolveHistoryBranch(ctx context.Context, branch string) (string, error) {
	if strings.TrimSpace(branch) != branch || branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsRune(branch, '\x00') {
		return "", errors.New("branch name is empty or invalid")
	}
	// Prefixing refs/heads makes option-like input inert and ensures a remote or
	// arbitrary commit is not silently detached when a branch was requested.
	if _, err := r.resolveHistoryCommit(ctx, "refs/heads/"+branch); err != nil {
		return "", err
	}
	return branch, nil
}

func historyPathExists(path string) bool { _, err := os.Stat(path); return err == nil }
