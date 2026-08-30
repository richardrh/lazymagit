// Package git provides a small, command-line based Git backend.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMixedState             = errors.New("cannot discard a path with both staged and unstaged changes")
	ErrUnsupportedStagedState = errors.New("cannot discard an unsupported staged state")
	ErrNoUpstream             = errors.New("current branch has no upstream")
	ErrNoFetchRemote          = errors.New("cannot determine fetch remote")
	ErrNotRepository          = errors.New("not a git repository")
)

// CommandError is returned when git exits unsuccessfully. Standard error is
// kept separate from standard output so command output remains safe to parse.
type CommandError struct {
	Args            []string
	Err             error
	Stderr          string
	StderrTruncated bool
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Stderr)
}

func (e *CommandError) Unwrap() error { return e.Err }

// ProcessRecord describes one git command launched by a mutating repository
// operation. Args contains the logical git arguments and does not include the
// internal -C option.
type ProcessRecord struct {
	Dir             string
	Args            []string
	Started         time.Time
	Duration        time.Duration
	ExitCode        int
	Stdout          string
	Stderr          string
	StdoutTruncated bool
	StderrTruncated bool
}

type processRecorderKey struct{}
type gitEditorKey struct{}
type gitSequenceEditorKey struct{}
type gitExtraEnvKey struct{}

// WithProcessRecorder arranges for mutating git commands to be reported to
// recorder synchronously. Read-only query commands are not reported.
func WithProcessRecorder(ctx context.Context, recorder func(ProcessRecord)) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, processRecorderKey{}, recorder)
}

type Repository struct {
	gitDir     string
	workTree   string
	commandDir string
}

func Discover(path string) (*Repository, error) {
	return discover(context.Background(), path)
}

func discover(ctx context.Context, path string) (*Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	r := &Repository{commandDir: abs}
	bareText, err := r.output(ctx, "rev-parse", "--is-bare-repository")
	if err != nil {
		return nil, discoveryError("discover repository", err)
	}
	gitDir, err := r.output(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, discoveryError("discover git directory", err)
	}
	r.gitDir = filepath.Clean(trimLine(gitDir))
	if trimLine(bareText) == "true" {
		r.gitDir = lexicalPath(abs, r.gitDir)
		r.commandDir = r.gitDir
		return r, nil
	}
	top, err := r.output(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, discoveryError("discover work tree", err)
	}
	canonicalTop := filepath.Clean(trimLine(top))
	r.workTree = lexicalPath(abs, canonicalTop)
	if rel, relErr := filepath.Rel(canonicalTop, r.gitDir); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		r.gitDir = filepath.Clean(filepath.Join(r.workTree, rel))
	}
	r.commandDir = r.workTree
	return r, nil
}

func discoveryError(operation string, err error) error {
	wrapped := fmt.Errorf("%s: %w", operation, err)
	var commandErr *CommandError
	if errors.As(err, &commandErr) && strings.HasPrefix(commandErr.Stderr, "fatal: not a git repository (") {
		return fmt.Errorf("%w: %w", ErrNotRepository, wrapped)
	}
	return wrapped
}

// Init initializes path as a repository and discovers the resulting repository.
// The path must already exist and be a directory.
func Init(ctx context.Context, path string) (*Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("inspect initialization directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("initialization path is not a directory")
	}
	r := &Repository{commandDir: abs}
	if err := r.run(ctx, "init"); err != nil {
		return nil, fmt.Errorf("initialize repository: %w", err)
	}
	return discover(ctx, abs)
}

func trimLine(b []byte) string {
	return strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
}

// Git resolves symlinks while reporting repository paths. Keep the lexical
// spelling supplied by the caller when the repository is one of its parents.
func lexicalPath(input, discovered string) string {
	resolved, err := filepath.EvalSymlinks(input)
	if err != nil {
		return discovered
	}
	rel, err := filepath.Rel(discovered, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return discovered
	}
	result := input
	if rel != "." {
		for range strings.Split(rel, string(filepath.Separator)) {
			result = filepath.Dir(result)
		}
	}
	return filepath.Clean(result)
}

func (r *Repository) GitDir() string   { return r.gitDir }
func (r *Repository) WorkTree() string { return r.workTree }
func (r *Repository) IsBare() bool     { return r.workTree == "" }

func (r *Repository) output(ctx context.Context, args ...string) ([]byte, error) {
	return r.outputInput(ctx, nil, args...)
}

func (r *Repository) outputInput(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	stdout, stderr, err := r.execute(ctx, input, args...)
	if err != nil {
		return nil, &CommandError{Args: append([]string(nil), args...), Err: err, Stderr: strings.TrimSpace(string(stderr))}
	}
	return stdout, nil
}

func (r *Repository) execute(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
	cmdArgs := append([]string{"-C", r.commandDir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = gitCommandEnv()
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func gitCommandEnv() []string {
	env := os.Environ()
	filtered := env[:0]
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_TERMINAL_PROMPT", "GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "LC_ALL", "GIT_EDITOR", "GIT_SEQUENCE_EDITOR":
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "GIT_TERMINAL_PROMPT=0", "GIT_LITERAL_PATHSPECS=1", "LC_ALL=C", "GIT_EDITOR=true", "GIT_SEQUENCE_EDITOR=true")
}

func (r *Repository) run(ctx context.Context, args ...string) error {
	return r.runInput(ctx, nil, args...)
}

func (r *Repository) runInput(ctx context.Context, input []byte, args ...string) error {
	started := time.Now()
	stdout, stderr, err := r.executeMutation(ctx, input, args...)
	duration := time.Since(started)
	recordedArgs, sensitive := redactMutationArgs(args)
	recordedStdout, stdoutRedactionTruncated := redactCaptured(stdout.String(), sensitive)
	recordedStderr, stderrRedactionTruncated := redactCaptured(stderr.String(), sensitive)
	stdoutTruncated := stdout.truncated || stdoutRedactionTruncated
	stderrTruncated := stderr.truncated || stderrRedactionTruncated
	if recorder, ok := ctx.Value(processRecorderKey{}).(func(ProcessRecord)); ok {
		recorder(ProcessRecord{
			Dir:             r.commandDir,
			Args:            append([]string(nil), recordedArgs...),
			Started:         started,
			Duration:        duration,
			ExitCode:        processExitCode(ctx, err),
			Stdout:          recordedStdout,
			Stderr:          recordedStderr,
			StdoutTruncated: stdoutTruncated,
			StderrTruncated: stderrTruncated,
		})
	}
	if err != nil {
		return &CommandError{Args: append([]string(nil), recordedArgs...), Err: err, Stderr: strings.TrimSpace(recordedStderr), StderrTruncated: stderrTruncated}
	}
	return nil
}

const (
	mutationCaptureLimit     = 256 << 10
	mutationTruncationMarker = "\n[... output truncated ...]\n"
	redactionMarker          = "[REDACTED]"
)

// headTailCapture always reports complete writes, so os/exec continues to
// drain the pipe, while retaining an equally useful beginning and tail.
type headTailCapture struct {
	head, tail []byte
	tailPos    int
	tailLen    int
	truncated  bool
}

func (w *headTailCapture) Write(p []byte) (int, error) {
	n := len(p)
	headLimit := (mutationCaptureLimit - len(mutationTruncationMarker)) / 2
	tailLimit := mutationCaptureLimit - len(mutationTruncationMarker) - headLimit
	if len(w.head) < headLimit {
		take := min(len(p), headLimit-len(w.head))
		w.head = append(w.head, p[:take]...)
		p = p[take:]
	}
	if len(p) == 0 {
		return n, nil
	}
	if w.tail == nil {
		w.tail = make([]byte, tailLimit)
	}
	for _, b := range p {
		if w.tailLen < tailLimit {
			w.tail[w.tailLen] = b
			w.tailLen++
			w.tailPos = w.tailLen % tailLimit
			continue
		}
		w.truncated = true
		w.tail[w.tailPos] = b
		w.tailPos = (w.tailPos + 1) % tailLimit
	}
	return n, nil
}

func (w *headTailCapture) String() string {
	if !w.truncated {
		b := make([]byte, 0, len(w.head)+w.tailLen)
		b = append(b, w.head...)
		b = append(b, w.tail[:w.tailLen]...)
		return string(b)
	}
	b := make([]byte, 0, mutationCaptureLimit)
	b = append(b, w.head...)
	b = append(b, mutationTruncationMarker...)
	b = append(b, w.tail[w.tailPos:]...)
	b = append(b, w.tail[:w.tailPos]...)
	return string(b)
}

func (r *Repository) executeMutation(ctx context.Context, input []byte, args ...string) (*headTailCapture, *headTailCapture, error) {
	cmdArgs := append([]string{"-C", r.commandDir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = gitCommandEnv()
	if editor, ok := ctx.Value(gitEditorKey{}).(string); ok && editor != "" {
		cmd.Env = replaceGitEnv(cmd.Env, "GIT_EDITOR="+editor)
	}
	if editor, ok := ctx.Value(gitSequenceEditorKey{}).(string); ok && editor != "" {
		cmd.Env = replaceGitEnv(cmd.Env, "GIT_SEQUENCE_EDITOR="+editor)
	}
	if extra, ok := ctx.Value(gitExtraEnvKey{}).([]string); ok {
		for _, entry := range extra {
			cmd.Env = replaceGitEnv(cmd.Env, entry)
		}
	}
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	stdout, stderr := new(headTailCapture), new(headTailCapture)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	return stdout, stderr, err
}

func replaceGitEnv(env []string, entry string) []string {
	key, _, _ := strings.Cut(entry, "=")
	prefix := key + "="
	out := env[:0]
	for _, current := range env {
		if !strings.HasPrefix(current, prefix) {
			out = append(out, current)
		}
	}
	return append(out, entry)
}

func redactMutationArgs(args []string) ([]string, []string) {
	redacted := append([]string(nil), args...)
	var sensitive []string
	if len(args) > 0 && args[0] == "commit" {
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "-m" || args[i] == "--message":
				if i+1 < len(args) {
					sensitive = appendSensitive(sensitive, args[i+1])
					redacted[i+1] = redactionMarker
					i++
				}
			case strings.HasPrefix(args[i], "--message="):
				value := strings.TrimPrefix(args[i], "--message=")
				sensitive = appendSensitive(sensitive, value)
				redacted[i] = "--message=" + redactionMarker
			case strings.HasPrefix(args[i], "-m") && len(args[i]) > 2:
				value := args[i][2:]
				sensitive = appendSensitive(sensitive, value)
				redacted[i] = "-m" + redactionMarker
			}
		}
	}
	if len(args) > 1 && args[0] == "remote" && (args[1] == "add" || args[1] == "set-url") {
		for i, arg := range args {
			if credentialBearingURL(arg) {
				sensitive = appendSensitive(sensitive, arg)
				redacted[i] = redactionMarker
			}
		}
	}
	return redacted, sensitive
}

func appendSensitive(values []string, value string) []string {
	if value != "" {
		return append(values, value)
	}
	return values
}

func credentialBearingURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.User != nil || u.RawQuery != "")
}

// redactCaptured replaces exact sensitive values without allowing replacement
// expansion to exceed the mutation capture cap.
func redactCaptured(text string, sensitive []string) (string, bool) {
	w := new(headTailCapture)
	for len(text) > 0 {
		at, matched := -1, ""
		for _, secret := range sensitive {
			if i := strings.Index(text, secret); i >= 0 && (at < 0 || i < at || i == at && len(secret) > len(matched)) {
				at, matched = i, secret
			}
		}
		if at < 0 {
			_, _ = w.Write([]byte(text))
			break
		}
		_, _ = w.Write([]byte(text[:at]))
		_, _ = w.Write([]byte(redactionMarker))
		text = text[at+len(matched):]
	}
	return redactCredentialURLs(w.String()), w.truncated
}

var capturedURLPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s'"\x00-\x1f]+`)

// redactCredentialURLs also protects later fetch/push diagnostics, where Git
// can echo a configured URL even though that URL was not part of this command's
// arguments.
func redactCredentialURLs(text string) string {
	return capturedURLPattern.ReplaceAllStringFunc(text, func(candidate string) string {
		if credentialBearingURL(candidate) {
			return redactionMarker
		}
		return candidate
	})
}

func processExitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}
	if ctx.Err() != nil {
		return -1
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

type Change uint8

const (
	ChangeNone Change = iota
	ChangeModified
	ChangeAdded
	ChangeDeleted
	ChangeRenamed
	ChangeCopied
	ChangeTypeChanged
	ChangeUnmerged
	ChangeUntracked
)

type FileStatus struct {
	Path         string
	OriginalPath string
	Staged       Change
	Unstaged     Change
}

type Status struct {
	Files []FileStatus
}

func change(code byte) Change {
	switch code {
	case '.', ' ':
		return ChangeNone
	case 'M':
		return ChangeModified
	case 'A':
		return ChangeAdded
	case 'D':
		return ChangeDeleted
	case 'R':
		return ChangeRenamed
	case 'C':
		return ChangeCopied
	case 'T':
		return ChangeTypeChanged
	case 'U':
		return ChangeUnmerged
	case '?':
		return ChangeUntracked
	default:
		return ChangeNone
	}
}

// Status parses porcelain v2's NUL-delimited form; paths are never split on
// whitespace or interpreted as quoted strings.
func (r *Repository) Status(ctx context.Context) (Status, error) {
	out, truncated, err := r.outputLimited(ctx, 16<<20, "status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	if truncated {
		return Status{}, &TooLargeError{Resource: "repository status"}
	}
	return parsePorcelainV2Status(out)
}

func parsePorcelainV2Status(out []byte) (Status, error) {
	var status Status
	records := bytes.Split(out, []byte{0})
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 {
			continue
		}
		switch record[0] {
		case '?':
			if len(record) >= 3 {
				status.Files = append(status.Files, FileStatus{Path: string(record[2:]), Unstaged: ChangeUntracked})
			}
		case '1', '2', 'u':
			// Ordinary records have eight fixed fields before the path,
			// rename records have nine, and unmerged records have ten.
			fixed := 8
			if record[0] == '2' {
				fixed = 9
			} else if record[0] == 'u' {
				fixed = 10
			}
			parts := bytes.SplitN(record, []byte{' '}, fixed+1)
			if len(parts) != fixed+1 || len(parts[1]) < 2 {
				return Status{}, fmt.Errorf("malformed porcelain v2 status record %q", record)
			}
			file := FileStatus{Path: string(parts[fixed]), Staged: change(parts[1][0]), Unstaged: change(parts[1][1])}
			if record[0] == '2' {
				if i+1 >= len(records) {
					return Status{}, errors.New("malformed porcelain v2 rename record")
				}
				i++
				file.OriginalPath = string(records[i])
			}
			status.Files = append(status.Files, file)
		}
	}
	return status, nil
}

func pathsArgs(prefix []string, paths []string) []string {
	args := make([]string, 0, len(prefix)+1+len(paths))
	args = append(args, prefix...)
	args = append(args, "--")
	return append(args, paths...)
}

func (r *Repository) Stage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	status, err := r.Status(ctx)
	if err != nil {
		return err
	}
	var requiresIndexUpdate bool
	paths, requiresIndexUpdate = expandRenamePaths(status, paths)
	if !requiresIndexUpdate {
		wanted := make(map[string]bool, len(paths))
		for _, path := range paths {
			wanted[path] = true
		}
		for _, file := range status.Files {
			if wanted[file.Path] && file.Unstaged == ChangeDeleted {
				requiresIndexUpdate = true
				break
			}
		}
	}
	if requiresIndexUpdate {
		input := []byte(strings.Join(paths, "\x00") + "\x00")
		return r.runInput(ctx, input, "update-index", "--add", "--remove", "-z", "--stdin")
	}
	return r.run(ctx, pathsArgs([]string{"add"}, paths)...)
}

// StageAll stages every tracked path with an unstaged change. Untracked paths
// are intentionally left alone. It reports whether there was anything to stage.
func (r *Repository) StageAll(ctx context.Context) (bool, error) {
	status, err := r.Status(ctx)
	if err != nil {
		return false, err
	}
	for _, file := range status.Files {
		if file.Unstaged != ChangeNone && file.Unstaged != ChangeUntracked {
			return true, r.run(ctx, "add", "--update")
		}
	}
	return false, nil
}

func expandRenamePaths(status Status, paths []string) ([]string, bool) {
	expanded := append([]string(nil), paths...)
	detected := false
	wanted := make(map[string]bool, len(paths))
	for _, path := range paths {
		wanted[path] = true
	}
	for _, file := range status.Files {
		if file.Staged != ChangeRenamed && file.Unstaged != ChangeRenamed {
			continue
		}
		if !wanted[file.Path] && !wanted[file.OriginalPath] {
			continue
		}
		detected = true
		for _, path := range []string{file.OriginalPath, file.Path} {
			if path != "" && !wanted[path] {
				expanded = append(expanded, path)
				wanted[path] = true
			}
		}
	}
	return expanded, detected
}

func (r *Repository) Unstage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	status, err := r.Status(ctx)
	if err != nil {
		return err
	}
	paths, _ = expandRenamePaths(status, paths)
	if _, err := r.output(ctx, "rev-parse", "--verify", "HEAD"); err == nil {
		return r.run(ctx, pathsArgs([]string{"restore", "--staged"}, paths)...)
	} else if !isExitError(err) {
		return err
	}
	// restore --staged requires HEAD. On an unborn branch, removing paths from
	// the index is equivalent and -f also handles files edited after staging.
	return r.run(ctx, pathsArgs([]string{"rm", "--cached", "-f", "--ignore-unmatch"}, paths)...)
}

// UnstageAll unstages every path currently changed in the index.
func (r *Repository) UnstageAll(ctx context.Context) error {
	status, err := r.Status(ctx)
	if err != nil {
		return err
	}
	hasStaged := false
	for _, file := range status.Files {
		if file.Staged != ChangeNone {
			hasStaged = true
			break
		}
	}
	if !hasStaged {
		return nil
	}
	if _, err := r.output(ctx, "rev-parse", "--verify", "HEAD"); err == nil {
		return r.run(ctx, "reset", "--mixed", "--quiet", "HEAD")
	} else if !isExitError(err) {
		return err
	}
	return r.run(ctx, "read-tree", "--empty")
}

func isExitError(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit)
}

func (r *Repository) Discard(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	status, err := r.Status(ctx)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(paths))
	for _, path := range paths {
		wanted[path] = true
	}
	var tracked, untracked, stagedTracked, stagedAdded []string
	for _, file := range status.Files {
		requestedRenameSource := file.Staged == ChangeRenamed && wanted[file.OriginalPath]
		if !wanted[file.Path] && !requestedRenameSource {
			continue
		}
		if file.Unstaged == ChangeDeleted {
			// Status still reports a tracked deletion when another filesystem
			// object, such as an ignored directory, replaces the tracked file.
			// Refuse before restore can remove or overwrite that replacement.
			deleted := filepath.Join(r.workTree, filepath.FromSlash(file.Path))
			if _, statErr := os.Lstat(deleted); statErr == nil {
				return fmt.Errorf("%w: deleted path %s was replaced", ErrMixedState, file.Path)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("inspect deleted path %s: %w", file.Path, statErr)
			}
		}
		if file.Staged == ChangeRenamed {
			// A staged rename normally leaves its source absent. Restoring HEAD
			// would overwrite a source path recreated after staging, including an
			// ignored path that status does not report.
			source := filepath.Join(r.workTree, filepath.FromSlash(file.OriginalPath))
			if _, statErr := os.Lstat(source); statErr == nil {
				return fmt.Errorf("%w: rename source %s was recreated", ErrMixedState, file.OriginalPath)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("inspect rename source %s: %w", file.OriginalPath, statErr)
			}
		}
		if file.Staged == ChangeDeleted {
			// A staged deletion makes the path untracked from the index's point
			// of view. In particular, an ignored recreation is absent from status.
			// Refuse before restore can overwrite any such worktree object.
			deleted := filepath.Join(r.workTree, filepath.FromSlash(file.Path))
			if _, statErr := os.Lstat(deleted); statErr == nil {
				return fmt.Errorf("%w: deleted path %s was recreated", ErrMixedState, file.Path)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("inspect deleted path %s: %w", file.Path, statErr)
			}
		}
		switch file.Staged {
		case ChangeNone, ChangeModified, ChangeAdded, ChangeDeleted, ChangeRenamed, ChangeTypeChanged:
			// Handled below after every requested path has been inspected.
		default:
			return fmt.Errorf("%w: %s", ErrUnsupportedStagedState, file.Path)
		}
		if file.Staged != ChangeNone && file.Unstaged != ChangeNone {
			return fmt.Errorf("%w: %s", ErrMixedState, file.Path)
		}
		if file.Unstaged == ChangeUntracked {
			untracked = append(untracked, file.Path)
		} else if file.Unstaged != ChangeNone {
			tracked = append(tracked, file.Path)
		} else if file.Staged == ChangeAdded {
			stagedAdded = append(stagedAdded, file.Path)
		} else if file.Staged == ChangeRenamed {
			stagedTracked = append(stagedTracked, file.OriginalPath, file.Path)
		} else if file.Staged == ChangeModified || file.Staged == ChangeDeleted || file.Staged == ChangeTypeChanged {
			stagedTracked = append(stagedTracked, file.Path)
		}
	}
	// All paths have been inspected before any command can mutate the tree.
	if len(stagedTracked) > 0 {
		if err := r.run(ctx, pathsArgs([]string{"restore", "--source=HEAD", "--staged", "--worktree"}, stagedTracked)...); err != nil {
			return err
		}
	}
	if len(tracked) > 0 {
		if err := r.run(ctx, pathsArgs([]string{"restore", "--worktree"}, tracked)...); err != nil {
			return err
		}
	}
	if len(stagedAdded) > 0 {
		if err := r.run(ctx, pathsArgs([]string{"rm", "--cached", "-f", "--ignore-unmatch"}, stagedAdded)...); err != nil {
			return err
		}
		// A staged addition may have been force-added despite an ignore rule.
		if err := r.run(ctx, pathsArgs([]string{"clean", "-f", "-d", "-x"}, stagedAdded)...); err != nil {
			return err
		}
	}
	if len(untracked) > 0 {
		if err := r.run(ctx, pathsArgs([]string{"clean", "-f", "-d"}, untracked)...); err != nil {
			return err
		}
	}
	return nil
}

type Commit struct {
	ID      string
	ShortID string
	// Refs is Git's raw %D decoration. Git currently separates decorations
	// with comma-space; callers must not treat that convention as escaping.
	Refs    string
	Subject string
	Author  string
	Date    time.Time
}

const commitLogFormat = "%H%x00%h%x00%D%x00%s%x00%an%x00%aI"

func (r *Repository) Commit(ctx context.Context, message string) (Commit, error) {
	return r.runCommit(ctx, []string{"commit", "-m", message})
}

func (r *Repository) RecentLog(ctx context.Context, limit int) ([]Commit, error) {
	if limit <= 0 {
		return nil, nil
	}
	out, err := r.output(ctx, "log", "-n", strconv.Itoa(limit), "--format="+commitLogFormat, "-z")
	if err != nil {
		if isExitError(err) { // An unborn repository has no log.
			if _, verifyErr := r.output(ctx, "rev-parse", "--verify", "HEAD"); isExitError(verifyErr) {
				return nil, nil
			}
		}
		return nil, err
	}
	return parseCommits(out)
}

func parseCommits(out []byte) ([]Commit, error) {
	fields := bytes.Split(out, []byte{0})
	for len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%6 != 0 {
		return nil, errors.New("malformed git log output")
	}
	commits := make([]Commit, 0, len(fields)/6)
	for i := 0; i < len(fields); i += 6 {
		date, err := time.Parse(time.RFC3339, string(fields[i+5]))
		if err != nil {
			return nil, fmt.Errorf("parse commit date: %w", err)
		}
		commits = append(commits, Commit{
			ID: string(fields[i]), ShortID: string(fields[i+1]), Refs: string(fields[i+2]),
			Subject: string(fields[i+3]), Author: string(fields[i+4]), Date: date,
		})
	}
	return commits, nil
}

type UpstreamRanges struct{ Ahead, Behind []Commit }

func (r *Repository) UpstreamLog(ctx context.Context) (UpstreamRanges, error) {
	return r.upstreamLog(ctx, nil)
}

// UpstreamLogLimit returns at most limit commits from each divergent range.
// UpstreamLog remains the unrestricted, backwards-compatible API.
func (r *Repository) UpstreamLogLimit(ctx context.Context, limit int) (UpstreamRanges, error) {
	return r.upstreamLog(ctx, &limit)
}

func (r *Repository) upstreamLog(ctx context.Context, limit *int) (UpstreamRanges, error) {
	if _, err := r.output(ctx, "rev-parse", "--verify", "@{upstream}"); err != nil {
		if isExitError(err) {
			return UpstreamRanges{}, ErrNoUpstream
		}
		return UpstreamRanges{}, err
	}
	if limit != nil && *limit <= 0 {
		return UpstreamRanges{}, nil
	}
	readRange := func(spec string) ([]Commit, error) {
		args := []string{"log"}
		if limit != nil {
			args = append(args, "-n", strconv.Itoa(*limit))
		}
		args = append(args, "--format="+commitLogFormat, "-z", spec)
		out, err := r.output(ctx, args...)
		if err != nil {
			return nil, err
		}
		return parseCommits(out)
	}
	ahead, err := readRange("@{upstream}..HEAD")
	if err != nil {
		return UpstreamRanges{}, err
	}
	behind, err := readRange("HEAD..@{upstream}")
	return UpstreamRanges{Ahead: ahead, Behind: behind}, err
}

type Branch struct {
	Name, Upstream, ID, Subject string
	Current, Remote             bool
}

func (r *Repository) Branches(ctx context.Context) ([]Branch, error) {
	format := "%(HEAD)%09%(refname:short)%09%(upstream:short)%09%(objectname)%09%(refname)%09%(symref)%09%(subject)"
	out, err := r.output(ctx, "for-each-ref", "--format="+format, "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var branches []Branch
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 7)
		if len(parts) != 7 {
			return nil, fmt.Errorf("malformed branch record %q", line)
		}
		name := parts[1]
		remote := strings.HasPrefix(parts[4], "refs/remotes/")
		if remote && parts[5] != "" { // Omit aliases such as origin/HEAD.
			continue
		}
		branches = append(branches, Branch{Name: name, Upstream: parts[2], ID: parts[3], Subject: parts[6], Current: parts[0] == "*", Remote: remote})
	}
	return branches, nil
}

func (r *Repository) CreateBranch(ctx context.Context, name, startPoint string) error {
	args := []string{"branch", "--", name}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	return r.run(ctx, args...)
}

func (r *Repository) SwitchBranch(ctx context.Context, name string) error {
	return r.run(ctx, "switch", "--", name)
}

type Summary struct {
	Head, Branch, Upstream string
	Ahead, Behind          int
	Detached, Unborn       bool
}

func (r *Repository) Summary(ctx context.Context) (Summary, error) {
	out, err := r.output(ctx, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=no")
	if err != nil {
		return Summary{}, err
	}
	return parseSummary(out), nil
}

func parseSummary(out []byte) Summary {
	var s Summary
	for _, record := range bytes.Split(out, []byte{0}) {
		applySummaryRecord(&s, string(record))
	}
	return s
}

func applySummaryRecord(s *Summary, line string) {
	const oid, head, upstream, counts = "# branch.oid ", "# branch.head ", "# branch.upstream ", "# branch.ab "
	switch {
	case strings.HasPrefix(line, oid):
		s.Head = strings.TrimPrefix(line, oid)
		s.Unborn = s.Head == "(initial)"
	case strings.HasPrefix(line, head):
		s.Branch = strings.TrimPrefix(line, head)
		s.Detached = s.Branch == "(detached)"
	case strings.HasPrefix(line, upstream):
		s.Upstream = strings.TrimPrefix(line, upstream)
	case strings.HasPrefix(line, counts):
		_, _ = fmt.Sscanf(strings.TrimPrefix(line, counts), "+%d -%d", &s.Ahead, &s.Behind)
	}
}

func (r *Repository) Diff(ctx context.Context, path string) (string, error) {
	return r.diffLimit(ctx, path, diffOutputLimit)
}

// DiffWithContext loads an unstaged file diff with an explicit non-negative
// number of unified context lines. It keeps the same terminal-safe options and
// output bound as Diff.
func (r *Repository) DiffWithContext(ctx context.Context, path string, lines int) (string, error) {
	if lines < 0 {
		return "", fmt.Errorf("diff context must be non-negative")
	}
	return r.loadDiffLimit(ctx, path, diffOutputLimit, "--unified="+strconv.Itoa(lines))
}

func (r *Repository) DiffStaged(ctx context.Context, path string) (string, error) {
	return r.diffStagedLimit(ctx, path, diffOutputLimit)
}

// DiffStagedWithContext is DiffWithContext for the index.
func (r *Repository) DiffStagedWithContext(ctx context.Context, path string, lines int) (string, error) {
	if lines < 0 {
		return "", fmt.Errorf("diff context must be non-negative")
	}
	return r.loadDiffLimit(ctx, path, diffOutputLimit, "--cached", "--unified="+strconv.Itoa(lines))
}

const (
	diffOutputLimit      = 8 << 20
	diffTruncationMarker = "\n[... diff output truncated ...]\n"
	// showCommitOutputLimit bounds retained stdout at a practical 8 MiB. The
	// process is still fully drained so a large patch cannot block on its pipe.
	showCommitOutputLimit      = 8 << 20
	showCommitTruncationMarker = "\n[... commit output truncated ...]\n"
)

func (r *Repository) diffLimit(ctx context.Context, path string, limit int) (string, error) {
	return r.loadDiffLimit(ctx, path, limit)
}

func (r *Repository) diffStagedLimit(ctx context.Context, path string, limit int) (string, error) {
	return r.loadDiffLimit(ctx, path, limit, "--cached")
}

func (r *Repository) loadDiffLimit(ctx context.Context, path string, limit int, extraArgs ...string) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv"}
	args = append(args, extraArgs...)
	args = append(args, "--", path)
	out, truncated, err := r.outputLimited(ctx, limit, args...)
	if err != nil {
		return "", err
	}
	if truncated {
		out = append(out, diffTruncationMarker...)
	}
	return string(out), nil
}

// ShowCommit returns a terminal-neutral, fuller representation of a commit,
// including its parents, file summary, and patch. At most 8 MiB of Git stdout
// is retained, followed by a truncation marker when needed. Resolve the
// caller-supplied revision first so it cannot be interpreted as an option.
func (r *Repository) ShowCommit(ctx context.Context, id string) (string, error) {
	return r.showCommitLimit(ctx, id, showCommitOutputLimit)
}

func (r *Repository) showCommitLimit(ctx context.Context, id string, limit int) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("commit revision is empty")
	}
	resolved, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", id+"^{commit}")
	if err != nil {
		return "", err
	}
	oid := trimLine(resolved)
	out, truncated, err := r.outputLimited(ctx, limit,
		"--no-pager", "show", "--no-color", "--no-ext-diff", "--no-textconv", "--format=fuller",
		"--parents", "--stat", "--patch", "--diff-merges=first-parent", oid,
	)
	if err != nil {
		return "", err
	}
	if truncated {
		out = append(out, showCommitTruncationMarker...)
	}
	return string(out), nil
}

type limitedCapture struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func (w *limitedCapture) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > w.remaining {
		p = p[:w.remaining]
		w.truncated = true
	}
	if len(p) > 0 {
		_, _ = w.buf.Write(p)
		w.remaining -= len(p)
	}
	return n, nil
}

func (r *Repository) outputLimited(ctx context.Context, limit int, args ...string) ([]byte, bool, error) {
	if limit < 0 {
		limit = 0
	}
	cmdArgs := append([]string{"-C", r.commandDir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = gitCommandEnv()
	stdout := &limitedCapture{remaining: limit}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, false, &CommandError{Args: append([]string(nil), args...), Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.buf.Bytes(), stdout.truncated, nil
}

type Remote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// Remotes lists each configured remote with the effective fetch and push URL.
func (r *Repository) Remotes(ctx context.Context) ([]Remote, error) {
	out, err := r.output(ctx, "remote")
	if err != nil {
		return nil, err
	}
	var remotes []Remote
	for _, name := range strings.Split(trimLine(out), "\n") {
		if name == "" {
			continue
		}
		// `git remote get-url` treats a URL-less remote's name as its URL.
		// Check the actual URL setting first so incomplete remote configuration
		// remains listable and is represented honestly.
		if _, configured, err := r.configValue(ctx, "remote."+name+".url"); err != nil {
			return nil, err
		} else if !configured {
			remotes = append(remotes, Remote{Name: name})
			continue
		}
		fetchURL, err := r.output(ctx, "remote", "get-url", name)
		if err != nil && commandExitCode(err) != 2 {
			return nil, err
		}
		pushURL, err := r.output(ctx, "remote", "get-url", "--push", name)
		if err != nil && commandExitCode(err) != 2 {
			return nil, err
		}
		remotes = append(remotes, Remote{Name: name, FetchURL: trimLine(fetchURL), PushURL: trimLine(pushURL)})
	}
	return remotes, nil
}

// AddRemote adds a remote and, when fetch is true, immediately fetches it in
// the same operation (Magit's default M a behavior).
func (r *Repository) AddRemote(ctx context.Context, name, url string, fetch bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("remote name is empty")
	}
	if url == "" {
		return errors.New("remote URL is empty")
	}
	args := []string{"remote", "add"}
	if fetch {
		args = append(args, "-f")
	}
	args = append(args, "--", name, url)
	return r.run(ctx, args...)
}

func (r *Repository) Fetch(ctx context.Context, remote ...string) error {
	if len(remote) > 1 {
		return errors.New("fetch accepts at most one remote")
	}
	args := []string{"fetch"}
	if len(remote) == 1 {
		args = append(args, "--", remote[0])
	}
	return r.run(ctx, args...)
}

// FetchUpstream fetches the current branch's upstream remote. Without one it
// follows Magit's primary-remote fallback: the sole remote, or the configured
// primary, upstream, or origin when multiple remotes exist.
func (r *Repository) FetchUpstream(ctx context.Context) error {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	remote, err := r.upstreamOrPrimaryRemote(ctx, branch)
	if err != nil {
		return err
	}
	return r.run(ctx, "fetch", "--", remote)
}

// FetchPush fetches the explicitly configured push remote. It intentionally
// does not fall back to an upstream or primary remote.
func (r *Repository) FetchPush(ctx context.Context) error {
	remote, err := r.PushRemote(ctx)
	if err != nil {
		return err
	}
	return r.run(ctx, "fetch", "--", remote)
}

// PushRemote resolves branch.<current>.pushRemote before remote.pushDefault
// and requires the selected value to name a configured remote.
func (r *Repository) PushRemote(ctx context.Context) (string, error) {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return "", err
	}
	var remote string
	var ok bool
	if branch != "" {
		remote, ok, err = r.configValue(ctx, "branch."+branch+".pushRemote")
		if err != nil {
			return "", err
		}
	}
	if !ok {
		remote, ok, err = r.configValue(ctx, "remote.pushDefault")
		if err != nil {
			return "", err
		}
	}
	if !ok {
		return "", fmt.Errorf("%w: no push remote is configured", ErrNoFetchRemote)
	}
	if err := r.validateRemote(ctx, remote); err != nil {
		return "", err
	}
	return remote, nil
}

// SetPushRemote configures branch.<current>.pushRemote after validating name.
func (r *Repository) SetPushRemote(ctx context.Context, name string) error {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	if branch == "" {
		return fmt.Errorf("%w: HEAD is detached", ErrNoFetchRemote)
	}
	if err := r.validateRemote(ctx, name); err != nil {
		return err
	}
	return r.run(ctx, "config", "branch."+branch+".pushRemote", name)
}

func (r *Repository) validateRemote(ctx context.Context, name string) error {
	out, err := r.output(ctx, "remote")
	if err != nil {
		return err
	}
	for _, configured := range strings.Split(trimLine(out), "\n") {
		if configured == name {
			return nil
		}
	}
	return fmt.Errorf("%w: remote %q is not configured", ErrNoFetchRemote, name)
}

func (r *Repository) currentBranch(ctx context.Context) (string, error) {
	out, err := r.output(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if commandExitCode(err) == 1 { // Detached HEAD has no current branch.
			return "", nil
		}
		return "", err
	}
	return trimLine(out), nil
}

func (r *Repository) configValue(ctx context.Context, key string) (string, bool, error) {
	out, err := r.output(ctx, "config", "--get", key)
	if err != nil {
		if commandExitCode(err) == 1 { // The key is not configured.
			return "", false, nil
		}
		return "", false, err
	}
	value := trimLine(out)
	return value, value != "", nil
}

func commandExitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

func (r *Repository) upstreamOrPrimaryRemote(ctx context.Context, branch string) (string, error) {
	out, err := r.output(ctx, "remote")
	if err != nil {
		return "", err
	}
	var remotes []string
	for _, remote := range strings.Split(trimLine(out), "\n") {
		if remote != "" {
			remotes = append(remotes, remote)
		}
	}
	if branch != "" {
		if remote, ok, err := r.configValue(ctx, "branch."+branch+".remote"); err != nil {
			return "", err
		} else if ok {
			return remote, nil
		}
	}
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	primary, ok, err := r.configValue(ctx, "magit.primaryRemote")
	if err != nil {
		return "", err
	}
	if ok && containsString(remotes, primary) {
		return primary, nil
	}
	for _, primary := range []string{"upstream", "origin"} {
		if containsString(remotes, primary) {
			return primary, nil
		}
	}
	if len(remotes) == 0 {
		return "", fmt.Errorf("%w: no remotes are configured", ErrNoFetchRemote)
	}
	return "", fmt.Errorf("%w: current branch has no configured remote and no primary remote exists", ErrNoFetchRemote)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// FetchAll updates every configured remote using Git's configured fetch
// refspecs and remote update policy.
func (r *Repository) FetchAll(ctx context.Context) error {
	return r.run(ctx, "fetch", "--all")
}

func (r *Repository) Push(ctx context.Context, remoteAndBranch ...string) error {
	if len(remoteAndBranch) > 2 {
		return errors.New("push accepts at most a remote and branch")
	}
	args := []string{"push"}
	if len(remoteAndBranch) > 0 {
		args = append(args, "--")
		args = append(args, remoteAndBranch...)
	}
	return r.run(ctx, args...)
}

// PushSetUpstream pushes the current branch to remote and configures it as the
// branch's upstream.
func (r *Repository) PushSetUpstream(ctx context.Context, remote string) error {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	if branch == "" {
		return errors.New("cannot set push upstream: HEAD is detached")
	}
	if _, err := r.output(ctx, "rev-parse", "--verify", "HEAD"); err != nil {
		if isExitError(err) {
			return fmt.Errorf("cannot set push upstream: current branch %q is unborn", branch)
		}
		return err
	}
	if err := r.validateRemote(ctx, remote); err != nil {
		return err
	}
	return r.run(ctx, "push", "--set-upstream", "--", remote, branch)
}
