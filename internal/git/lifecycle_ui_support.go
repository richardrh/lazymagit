package git

// This file contains the small safety boundary needed by repository lifecycle
// UIs. It deliberately builds on CloneRepository and Init rather than exposing
// arbitrary git arguments.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateCloneDestination verifies that path is suitable for a clone without
// creating it. CloneRepository repeats the check immediately before invoking
// Git, so callers must still handle a race as an ordinary operation failure.
func ValidateCloneDestination(path string) error {
	return safeNewDirectory(path)
}

// CloneRepositoryForUI gives a UI an explicit partial-destination contract. If
// destination did not exist before this call and clone fails or is cancelled,
// the newly-created partial destination is removed. A caller-owned, initially
// empty directory is never removed. CloneRepository supplies argv-only
// execution, destination validation, bounded output, and credential redaction.
func CloneRepositoryForUI(ctx context.Context, source, destination string, options CloneOptions) (err error) {
	abs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	_, statErr := os.Lstat(abs)
	destinationAbsent := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !destinationAbsent {
		return fmt.Errorf("inspect clone destination: %w", statErr)
	}
	if err := ValidateCloneDestination(abs); err != nil {
		return err
	}
	var owned os.FileInfo
	if destinationAbsent {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("create clone destination parent: %w", err)
		}
		if err := os.Mkdir(abs, 0o755); err != nil {
			return fmt.Errorf("reserve clone destination: %w", err)
		}
		owned, err = os.Lstat(abs)
		if err != nil {
			return fmt.Errorf("inspect reserved clone destination: %w", err)
		}
	}
	err = CloneRepository(ctx, source, abs, options)
	if err == nil || !destinationAbsent {
		return err
	}
	if cleanupErr := removeOwnedDirectory(abs, owned); cleanupErr != nil {
		return errors.Join(err, fmt.Errorf("remove partial clone destination: %w", cleanupErr))
	}
	return err
}

// ValidateInitDestination refuses repository reinitialization and requires an
// existing directory, matching Init's contract. Existing ordinary files are
// preserved by git init; an existing .git entry is never implicitly reused.
func ValidateInitDestination(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: initialization path is empty", ErrUnsafeDestination)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if abs == string(filepath.Separator) {
		return fmt.Errorf("%w: initialization path is filesystem root", ErrUnsafeDestination)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("inspect initialization directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: initialization path is not a directory", ErrUnsafeDestination)
	}
	if _, err := os.Lstat(filepath.Join(abs, ".git")); err == nil {
		return fmt.Errorf("%w: destination is already a repository", ErrUnsafeDestination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect initialization metadata: %w", err)
	}
	return nil
}

// InitRepositoryForUI initializes an existing directory without changing any
// caller Repository. On failure or cancellation it removes only the .git entry
// which was absent at preflight; ordinary destination contents are untouched.
func InitRepositoryForUI(ctx context.Context, path string) (err error) {
	if err := ValidateInitDestination(path); err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	metadata := filepath.Join(abs, ".git")
	if err := os.Mkdir(metadata, 0o755); err != nil {
		return fmt.Errorf("reserve initialization metadata: %w", err)
	}
	owned, err := os.Lstat(metadata)
	if err != nil {
		return fmt.Errorf("inspect reserved initialization metadata: %w", err)
	}
	_, err = Init(ctx, abs)
	if err == nil {
		return nil
	}
	if cleanupErr := removeOwnedDirectory(metadata, owned); cleanupErr != nil {
		return errors.Join(err, fmt.Errorf("remove partial initialization metadata: %w", cleanupErr))
	}
	return err
}

func removeOwnedDirectory(path string, owned os.FileInfo) error {
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if owned == nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(owned, current) {
		return fmt.Errorf("%w: partial destination identity changed; cleanup refused", ErrUnsafeDestination)
	}
	return os.RemoveAll(path)
}
