package git

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	ErrConfirmationRequired            = errors.New("confirmation required")
	ErrStalePlan                       = errors.New("workflow plan is stale")
	ErrEditorRequired                  = errors.New("editor required")
	ErrTooLarge                        = errors.New("workflow input or output is too large")
	ErrInteractiveRequired             = errors.New("interactive execution requires explicit permission")
	ErrUnsafeExecution                 = errors.New("unsafe execution requires explicit permission")
	ErrReviewedStashRemovalUnsupported = errors.New("exact reviewed stash removal is unsupported by stock Git")
)

// ConfirmationToken binds a confirmation to the immutable identity that was
// displayed during preflight. Its zero value is never valid.
type ConfirmationToken struct{ identity string }

func NewConfirmationToken(identity string) ConfirmationToken {
	if identity == "" {
		return ConfirmationToken{}
	}
	return ConfirmationToken{identity: identity}
}

func (t ConfirmationToken) validFor(identity string) bool {
	return identity != "" && t.identity == identity
}

type ConfirmationRequiredError struct {
	Operation string
	Identity  string
}

func (e *ConfirmationRequiredError) Error() string {
	return fmt.Sprintf("%s: %v", e.Operation, ErrConfirmationRequired)
}
func (e *ConfirmationRequiredError) Unwrap() error { return ErrConfirmationRequired }

// EditorRequiredError is the sole editor-required error contract.
type EditorRequiredError struct {
	Operation string
	Err       error
}

func (e *EditorRequiredError) Error() string        { return e.Operation + ": editor required" }
func (e *EditorRequiredError) Unwrap() error        { return e.Err }
func (e *EditorRequiredError) Is(target error) bool { return target == ErrEditorRequired }

type TooLargeError struct{ Resource string }

func (e *TooLargeError) Error() string { return e.Resource + ": too large" }
func (e *TooLargeError) Unwrap() error { return ErrTooLarge }

func readFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, &TooLargeError{Resource: path}
	}
	return b, nil
}

// AllowUnsafeExecution is an unforgeable-by-zero-value capability. Construct
// it only at the application boundary after an explicit unsafe-mode opt-in.
type AllowUnsafeExecution struct{ marker *struct{} }

func NewAllowUnsafeExecution() AllowUnsafeExecution { return AllowUnsafeExecution{marker: &struct{}{}} }
func (c AllowUnsafeExecution) allowed() bool        { return c.marker != nil }

type CommitMutationError struct {
	CommitOID string
	Err       error
}

func (e *CommitMutationError) Error() string {
	return fmt.Sprintf("commit %s was created; post-commit query failed: %v", e.CommitOID, e.Err)
}
func (e *CommitMutationError) Unwrap() error { return e.Err }

type PartialMutationError struct {
	Operation string
	Cause     error
	Rollback  error
	State     []string
}

func (e *PartialMutationError) Error() string {
	return fmt.Sprintf("%s partially mutated state: %v; rollback failed: %v (%s)", e.Operation, e.Cause, e.Rollback, strings.Join(e.State, ", "))
}
func (e *PartialMutationError) Unwrap() error { return e.Cause }
