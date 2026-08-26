package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrRawCommandAlias    = errors.New("Git aliases are not allowed by raw command execution")
	ErrExternalGitCommand = errors.New("external Git subcommand requires separate confirmation")
)

// ReviewedCommand is an immutable command produced by a review preflight. A
// zero value cannot be executed. argv never passes through a shell.
type ReviewedCommand struct {
	kind     reviewedCommandKind
	argv     []string
	external bool
	marker   *struct{}
}

type reviewedCommandKind uint8

const (
	reviewedGit reviewedCommandKind = iota + 1
	reviewedRun
)

// Args returns a defensive, credential-redacted copy suitable for a review
// screen or audit log. The executable boundary retains the private original.
func (p ReviewedCommand) Args() []string {
	args, _ := redactCommandUIArgs(p.argv)
	return args
}

// ExternalGit reports whether the plan invokes a git-<name> helper directly.
func (p ReviewedCommand) ExternalGit() bool { return p.external }

const maxReviewedCommandArgs = 256
const maxReviewedCommandBytes = 32 << 10

// ReviewGitCommand validates raw Git argv. argv excludes the leading "git".
// Global options (including -c) are rejected. Known aliases are always
// rejected. Unknown git-<subcommand> helpers require allowExternal and are run
// directly after review, so a subsequently-created alias cannot be substituted.
func (r *Repository) ReviewGitCommand(ctx context.Context, argv []string, allowExternal bool) (ReviewedCommand, error) {
	argv, err := validateReviewedArgv(argv)
	if err != nil {
		return ReviewedCommand{}, err
	}
	if strings.HasPrefix(argv[0], "-") || argv[0] == "git" {
		return ReviewedCommand{}, errors.New("enter a Git subcommand first; global options and a leading git are not allowed")
	}
	if blockedGitSubcommands[argv[0]] {
		return ReviewedCommand{}, fmt.Errorf("Git subcommand %q can launch an interactive or credential program and is unsupported", argv[0])
	}
	if argv[0] == "bisect" && len(argv) > 1 && argv[1] == "run" {
		return ReviewedCommand{}, errors.New("bisect run is history-owned and unavailable from raw command execution")
	}
	if alias, truncated, err := r.outputLimited(ctx, 64<<10, "config", "--get", "alias."+argv[0]); err == nil && (truncated || strings.TrimSpace(string(alias)) != "") {
		return ReviewedCommand{}, fmt.Errorf("%w: %s", ErrRawCommandAlias, argv[0])
	} else if err != nil && !isExitCode(err, 1) {
		return ReviewedCommand{}, fmt.Errorf("check Git alias: %w", err)
	}
	external := !builtinGitSubcommands[argv[0]]
	if external && !allowExternal {
		return ReviewedCommand{}, fmt.Errorf("%w: %s", ErrExternalGitCommand, argv[0])
	}
	return ReviewedCommand{kind: reviewedGit, argv: argv, external: external, marker: &struct{}{}}, nil
}

// ReviewRunCommand adapts Magit's shell-oriented run facility to direct argv
// execution. It deliberately provides no pipes, redirects, expansion, or shell.
func ReviewRunCommand(argv []string) (ReviewedCommand, error) {
	argv, err := validateReviewedArgv(argv)
	if err != nil {
		return ReviewedCommand{}, err
	}
	executable := strings.ToLower(filepath.Base(argv[0]))
	if blockedDirectExecutables[executable] || strings.HasPrefix(executable, "git-") {
		return ReviewedCommand{}, fmt.Errorf("direct executable %q is unsupported; shells and Git bypasses are not allowed", executable)
	}
	return ReviewedCommand{kind: reviewedRun, argv: argv, marker: &struct{}{}}, nil
}

func validateReviewedArgv(argv []string) ([]string, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("command requires explicit argv")
	}
	if len(argv) > maxReviewedCommandArgs {
		return nil, errors.New("command has too many arguments")
	}
	copy := append([]string(nil), argv...)
	total := 0
	for _, arg := range copy {
		if strings.ContainsRune(arg, 0) {
			return nil, errors.New("command argv contains NUL")
		}
		total += len(arg)
	}
	if total > maxReviewedCommandBytes {
		return nil, errors.New("command argv is too large")
	}
	return copy, nil
}

func isExitCode(err error, code int) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == code
}

// ExecuteReviewedCommand is the only raw-command execution boundary. It
// requires both a review token and the application's explicit unsafe capability.
func (r *Repository) ExecuteReviewedCommand(ctx context.Context, capability AllowUnsafeExecution, plan ReviewedCommand) error {
	if !capability.allowed() || plan.marker == nil {
		return ErrUnsafeExecution
	}
	argv := append([]string(nil), plan.argv...)
	switch plan.kind {
	case reviewedGit:
		if plan.external {
			return r.executeDirectReviewed(ctx, "git-"+argv[0], argv[1:], []string{"external-git", argv[0]})
		}
		return r.executeDirectReviewed(ctx, "git", argv, nil)
	case reviewedRun:
		return r.executeDirectReviewed(ctx, argv[0], argv[1:], []string{"run", argv[0]})
	default:
		return ErrUnsafeExecution
	}
}

func (r *Repository) executeDirectReviewed(ctx context.Context, executable string, args, recordedPrefix []string) error {
	started := time.Now()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = r.commandDir
	cmd.Env = commandUIEnv()
	stdout, stderr := new(headTailCapture), new(headTailCapture)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	recordedArgs, sensitive := redactCommandUIArgs(append(append([]string(nil), recordedPrefix...), args...))
	recordedStdout, stdoutRedacted := redactCommandUIOutput(stdout.String(), sensitive)
	recordedStderr, stderrRedacted := redactCommandUIOutput(stderr.String(), sensitive)
	if recorder, ok := ctx.Value(processRecorderKey{}).(func(ProcessRecord)); ok {
		recorder(ProcessRecord{Dir: r.commandDir, Args: recordedArgs, Started: started, Duration: time.Since(started), ExitCode: processExitCode(ctx, err), Stdout: recordedStdout, Stderr: recordedStderr, StdoutTruncated: stdout.truncated || stdoutRedacted, StderrTruncated: stderr.truncated || stderrRedacted})
	}
	if err != nil {
		return &CommandError{Args: recordedArgs, Err: err, Stderr: strings.TrimSpace(recordedStderr), StderrTruncated: stderr.truncated || stderrRedacted}
	}
	return nil
}

func commandUIEnv() []string {
	out := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "CREDENTIAL") || strings.Contains(upper, "AUTHORIZATION") || strings.Contains(upper, "ASKPASS") || key == "GIT_TERMINAL_PROMPT" || key == "GIT_EDITOR" || key == "GIT_SEQUENCE_EDITOR" {
			continue
		}
		out = append(out, entry)
	}
	return append(out, "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true", "SSH_ASKPASS=true", "GIT_EDITOR=true", "GIT_SEQUENCE_EDITOR=true")
}

func redactCommandUIArgs(args []string) ([]string, []string) {
	out := append([]string(nil), args...)
	var sensitive []string
	for i, arg := range out {
		lower := strings.ToLower(arg)
		previousSensitiveFlag := i > 0 && commandUISensitiveFlag(strings.ToLower(args[i-1]))
		if credentialBearingURL(arg) || previousSensitiveFlag || strings.Contains(lower, "password=") || strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") || strings.Contains(lower, "authorization=") {
			sensitive = appendSensitive(sensitive, arg)
			out[i] = redactionMarker
		}
	}
	return out, sensitive
}

func commandUISensitiveFlag(value string) bool {
	value = strings.TrimLeft(value, "-")
	return value == "password" || value == "passwd" || value == "token" || value == "secret" || value == "credential" || value == "authorization"
}

var commandUICredentialPattern = regexp.MustCompile(`(?i)\b(password|passwd|token|secret|authorization)\s*[:=]\s*[^\s]+`)

func redactCommandUIOutput(text string, sensitive []string) (string, bool) {
	text, truncated := redactCaptured(text, sensitive)
	return commandUICredentialPattern.ReplaceAllString(text, "$1="+redactionMarker), truncated
}

var blockedGitSubcommands = map[string]bool{
	"credential": true, "credential-cache": true, "credential-store": true,
	"difftool": true, "mergetool": true, "gui": true, "citool": true,
	"instaweb": true, "web--browse": true, "shell": true,
}

var blockedDirectExecutables = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true,
	"fish": true, "csh": true, "tcsh": true, "cmd": true, "cmd.exe": true,
	"powershell": true, "powershell.exe": true, "pwsh": true, "pwsh.exe": true,
	"env": true, "nohup": true, "git": true,
}

var builtinGitSubcommands = func() map[string]bool {
	// Static by design: discovering helpers from PATH during review would turn a
	// typo into executable behavior. Additions should be reviewed here.
	values := strings.Fields("add am apply archive bisect blame branch bundle checkout cherry cherry-pick clean clone commit describe diff fetch format-patch fsck gc grep hash-object help init log merge merge-base mv notes pull push range-diff rebase reflog remote repack replace reset restore revert rm show show-branch sparse-checkout stash status submodule switch tag verify-commit verify-tag worktree")
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}()
