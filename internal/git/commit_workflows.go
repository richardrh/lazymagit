package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CommitOptions contains the infix options shared by commit workflows. Date is
// deliberately a string: Git accepts both strict timestamps and useful forms
// such as "yesterday".
type CommitOptions struct {
	All                     bool
	AllowEmpty              bool
	NoVerify                bool
	ResetAuthor             bool
	Author                  string
	Date                    string
	Signoff                 bool
	ReuseMessage            string
	ReeditMessage           string
	Sign                    bool
	SigningKey              string
	AllowInteractiveSigning bool
}

// UnsupportedCommitVariantError reports a workflow unavailable in the
// installed Git. Structured fixups were added in Git 2.32.
type UnsupportedCommitVariantError struct {
	Variant string
	Version string
}

func (e *UnsupportedCommitVariantError) Error() string {
	return fmt.Sprintf("commit variant %s is unsupported by %s", e.Variant, e.Version)
}

// InvalidRevisionError reports a revision which does not resolve to a commit.
type InvalidRevisionError struct {
	Revision string
	Err      error
}

func (e *InvalidRevisionError) Error() string {
	return fmt.Sprintf("invalid commit revision %q", e.Revision)
}
func (e *InvalidRevisionError) Unwrap() error { return e.Err }

// CommitOptionError reports incompatible options without launching a
// mutating Git command.
type CommitOptionError struct{ Message string }

func (e *CommitOptionError) Error() string { return e.Message }

// CreateCommit creates a commit from the index.
func (r *Repository) CreateCommit(ctx context.Context, message string, options CommitOptions) (Commit, error) {
	args, err := r.commitArgs(ctx, "create", nil, options)
	if err != nil {
		return Commit{}, err
	}
	if err := requireCommitMessage("create commit", message, options); err != nil {
		return Commit{}, err
	}
	args = appendMessage(args, message)
	return r.runCommit(ctx, args)
}

// ExtendCommit amends HEAD while retaining its message.
func (r *Repository) ExtendCommit(ctx context.Context, options CommitOptions) (Commit, error) {
	if options.ReuseMessage != "" {
		return Commit{}, &CommitOptionError{Message: "extend commit cannot reuse another message"}
	}
	args, err := r.commitArgs(ctx, "extend", []string{"--amend", "--no-edit"}, options)
	if err != nil {
		return Commit{}, err
	}
	return r.runCommit(ctx, args)
}

// AmendCommit amends HEAD and replaces its message with message. A reused
// message may be selected instead by leaving message empty and setting
// ReuseMessage.
func (r *Repository) AmendCommit(ctx context.Context, message string, options CommitOptions) (Commit, error) {
	args, err := r.commitArgs(ctx, "amend", []string{"--amend"}, options)
	if err != nil {
		return Commit{}, err
	}
	if err := requireCommitMessage("amend commit", message, options); err != nil {
		return Commit{}, err
	}
	args = appendMessage(args, message)
	return r.runCommit(ctx, args)
}

// RewordCommit changes only HEAD's message; staged changes remain staged.
func (r *Repository) RewordCommit(ctx context.Context, message string, options CommitOptions) (Commit, error) {
	if options.All {
		return Commit{}, &CommitOptionError{Message: "reword commit cannot use all while preserving the staged tree"}
	}
	args, err := r.commitArgs(ctx, "reword", []string{"--amend", "--only", "--allow-empty"}, options)
	if err != nil {
		return Commit{}, err
	}
	if err := requireCommitMessage("reword commit", message, options); err != nil {
		return Commit{}, err
	}
	args = appendMessage(args, message)
	return r.runCommit(ctx, args)
}

// FixupCommit creates a fixup! commit targeting revision.
func (r *Repository) FixupCommit(ctx context.Context, revision string, options CommitOptions) (Commit, error) {
	if options.ReuseMessage != "" {
		return Commit{}, &CommitOptionError{Message: "fixup commit cannot reuse another message"}
	}
	target, err := r.resolveCommitForCommitWorkflow(ctx, revision)
	if err != nil {
		return Commit{}, err
	}
	args, err := r.commitArgs(ctx, "fixup", []string{"--fixup=" + target}, options)
	if err != nil {
		return Commit{}, err
	}
	return r.runCommit(ctx, args)
}

// SquashCommit creates a squash! commit. If message is non-empty it is added
// to Git's generated subject without invoking an editor.
func (r *Repository) SquashCommit(ctx context.Context, revision, message string, options CommitOptions) (Commit, error) {
	if options.ReuseMessage != "" {
		return Commit{}, &CommitOptionError{Message: "squash commit cannot reuse another message"}
	}
	target, err := r.resolveCommitForCommitWorkflow(ctx, revision)
	if err != nil {
		return Commit{}, err
	}
	args, err := r.commitArgs(ctx, "squash", []string{"--squash=" + target}, options)
	if err != nil {
		return Commit{}, err
	}
	args = appendMessage(args, message)
	return r.runCommit(ctx, args)
}

// AlterCommit creates an amend! fixup containing the staged tree. message is
// the message the target commit should have after autosquash.
func (r *Repository) AlterCommit(ctx context.Context, revision, message string, options CommitOptions) (Commit, error) {
	return r.structuredFixup(ctx, "alter", "amend", revision, message, options)
}

// AugmentCommit is an editable squash workflow. Supplying the edited message
// avoids launching an interactive editor.
func (r *Repository) AugmentCommit(ctx context.Context, revision, message string, options CommitOptions) (Commit, error) {
	if message == "" {
		return Commit{}, &EditorRequiredError{Operation: "augment commit"}
	}
	return r.SquashCommit(ctx, revision, message, options)
}

// ReviseCommit creates a reword fixup and leaves the staged tree untouched.
// message is the message the target commit should have after autosquash.
func (r *Repository) ReviseCommit(ctx context.Context, revision, message string, options CommitOptions) (Commit, error) {
	if options.All {
		return Commit{}, &CommitOptionError{Message: "revise commit cannot use all while preserving the staged tree"}
	}
	return r.structuredFixup(ctx, "revise", "reword", revision, message, options)
}

func requireCommitMessage(operation, message string, options CommitOptions) error {
	if options.ReuseMessage != "" && options.ReeditMessage != "" {
		return &CommitOptionError{Message: operation + " cannot combine reuse-message and reedit-message"}
	}
	if message != "" && options.ReuseMessage != "" {
		return &CommitOptionError{Message: operation + " cannot combine message text and reuse-message"}
	}
	if message == "" && options.ReuseMessage == "" {
		return &EditorRequiredError{Operation: operation}
	}
	return nil
}

func appendMessage(args []string, message string) []string {
	if message != "" {
		return append(args, "-m", message)
	}
	return args
}

func (r *Repository) commitArgs(ctx context.Context, operation string, initial []string, options CommitOptions) ([]string, error) {
	if options.ReuseMessage != "" && options.ReeditMessage != "" {
		return nil, &CommitOptionError{Message: operation + " cannot combine reuse-message and reedit-message"}
	}
	if (options.Sign || options.SigningKey != "") && !options.AllowInteractiveSigning {
		return nil, fmt.Errorf("%w: commit signing", ErrInteractiveRequired)
	}
	args := append([]string{"commit"}, initial...)
	if options.All {
		args = append(args, "--all")
	}
	if options.AllowEmpty {
		args = append(args, "--allow-empty")
	}
	if options.NoVerify {
		args = append(args, "--no-verify")
	}
	if options.ResetAuthor {
		args = append(args, "--reset-author")
	}
	if options.Author != "" {
		args = append(args, "--author="+options.Author)
	}
	if options.Date != "" {
		args = append(args, "--date="+options.Date)
	}
	if options.Signoff {
		args = append(args, "--signoff")
	}
	if options.ReuseMessage != "" {
		reuse, err := r.resolveCommitForCommitWorkflow(ctx, options.ReuseMessage)
		if err != nil {
			return nil, err
		}
		args = append(args, "--reuse-message="+reuse)
	}
	if options.ReeditMessage != "" {
		reedit, err := r.resolveCommitForCommitWorkflow(ctx, options.ReeditMessage)
		if err != nil {
			return nil, err
		}
		args = append(args, "--reedit-message="+reedit)
	}
	if options.SigningKey != "" {
		args = append(args, "--gpg-sign="+options.SigningKey)
	} else if options.Sign {
		args = append(args, "--gpg-sign")
	}
	return args, nil
}

// CommitMessageForUI loads a bounded editable message after resolving the
// exact source commit used by --reedit-message.
func (r *Repository) CommitMessageForUI(ctx context.Context, revision string) (string, error) {
	oid, err := r.resolveCommitForCommitWorkflow(ctx, revision)
	if err != nil {
		return "", err
	}
	out, truncated, err := r.outputLimited(ctx, 64<<10, "show", "-s", "--format=%B", oid)
	if err != nil {
		return "", err
	}
	if truncated {
		return "", &TooLargeError{Resource: "commit message"}
	}
	return string(out), nil
}

func (r *Repository) resolveCommitForCommitWorkflow(ctx context.Context, revision string) (string, error) {
	if revision == "" {
		return "", &InvalidRevisionError{Revision: revision, Err: errors.New("empty revision")}
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", &InvalidRevisionError{Revision: revision, Err: err}
	}
	oid := trimLine(out)
	if !isCommitWorkflowOID(oid) {
		return "", &InvalidRevisionError{Revision: revision, Err: errors.New("Git returned an invalid object ID")}
	}
	return oid, nil
}

func isCommitWorkflowOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (r *Repository) runCommit(ctx context.Context, args []string) (Commit, error) {
	if err := r.run(ctx, args...); err != nil {
		return Commit{}, err
	}
	oid, err := r.readMutatedHEAD()
	if err != nil {
		return Commit{}, fmt.Errorf("commit succeeded but HEAD cannot be resolved: %w", err)
	}
	commits, err := r.RecentLog(ctx, 1)
	if err != nil {
		return Commit{ID: oid}, &CommitMutationError{CommitOID: oid, Err: err}
	}
	if len(commits) == 0 {
		return Commit{ID: oid}, &CommitMutationError{CommitOID: oid, Err: errors.New("HEAD log entry is missing")}
	}
	return commits[0], nil
}

func (r *Repository) readMutatedHEAD() (string, error) {
	head, err := readFileLimited(filepath.Join(r.gitDir, "HEAD"), 4<<10)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(head))
	if strings.HasPrefix(value, "ref: ") {
		ref := strings.TrimPrefix(value, "ref: ")
		if !strings.HasPrefix(ref, "refs/") || strings.Contains(ref, "..") {
			return "", errors.New("HEAD contains an unsafe symbolic ref")
		}
		data, err := readFileLimited(filepath.Join(r.gitDir, filepath.FromSlash(ref)), 4<<10)
		if errors.Is(err, os.ErrNotExist) {
			if common, commonErr := readFileLimited(filepath.Join(r.gitDir, "commondir"), 4<<10); commonErr == nil {
				commonDir := strings.TrimSpace(string(common))
				if !filepath.IsAbs(commonDir) {
					commonDir = filepath.Join(r.gitDir, commonDir)
				}
				data, err = readFileLimited(filepath.Join(filepath.Clean(commonDir), filepath.FromSlash(ref)), 4<<10)
			}
		}
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(string(data))
	}
	if !isCommitWorkflowOID(value) {
		return "", errors.New("HEAD contains an invalid object ID")
	}
	return value, nil
}

func (r *Repository) structuredFixup(ctx context.Context, operation, mode, revision, message string, options CommitOptions) (Commit, error) {
	target, err := r.resolveCommitForCommitWorkflow(ctx, revision)
	if err != nil {
		return Commit{}, err
	}
	if message == "" {
		return Commit{}, &EditorRequiredError{Operation: operation + " commit"}
	}
	if options.ReuseMessage != "" || options.ReeditMessage != "" {
		return Commit{}, &CommitOptionError{Message: operation + " commit cannot combine an autosquash message and reuse-message"}
	}
	if err := r.requireStructuredFixup(ctx, operation); err != nil {
		return Commit{}, err
	}
	subjectBytes, err := r.output(ctx, "show", "-s", "--format=%s", target)
	if err != nil {
		return Commit{}, err
	}
	args, err := r.commitArgs(ctx, operation, []string{"--fixup=" + mode + ":" + target}, options)
	if err != nil {
		return Commit{}, err
	}
	edited := "amend! " + trimLine(subjectBytes) + "\n\n" + message + "\n"
	return r.runCommitWithEditor(ctx, args, edited)
}

var gitVersionPattern = regexp.MustCompile(`(?m)^git version (\d+)\.(\d+)`)

func (r *Repository) requireStructuredFixup(ctx context.Context, variant string) error {
	out, err := r.output(ctx, "version")
	if err != nil {
		return err
	}
	match := gitVersionPattern.FindSubmatch(out)
	if len(match) != 3 {
		return &UnsupportedCommitVariantError{Variant: variant, Version: strings.TrimSpace(string(out))}
	}
	major, _ := strconv.Atoi(string(match[1]))
	minor, _ := strconv.Atoi(string(match[2]))
	if major < 2 || major == 2 && minor < 32 {
		return &UnsupportedCommitVariantError{Variant: variant, Version: strings.TrimSpace(string(out))}
	}
	return nil
}

// runCommitWithEditor supplies an edited message through a private temporary
// editor. This preserves Git's structured-fixup semantics and hooks without
// ever opening a terminal editor.
func (r *Repository) runCommitWithEditor(ctx context.Context, args []string, message string) (Commit, error) {
	dir, err := os.MkdirTemp("", "lazymagit-commit-")
	if err != nil {
		return Commit{}, err
	}
	defer os.RemoveAll(dir)
	messagePath := dir + "/message"
	editorPath := dir + "/editor"
	if err := os.WriteFile(messagePath, []byte(message), 0o600); err != nil {
		return Commit{}, err
	}
	script := "#!/bin/sh\ncat " + commitShellQuote(messagePath) + " > \"$1\"\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o700); err != nil {
		return Commit{}, err
	}
	ctx = context.WithValue(ctx, gitEditorKey{}, editorPath)
	return r.runCommit(ctx, args)
}

func commitShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
