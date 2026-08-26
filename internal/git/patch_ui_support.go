package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxUIPatchSeries = 1000
	maxReviewedPatch = 64 << 20
)

// ReviewedDiffPatch binds an overwrite confirmation to the exact destination
// observed by the user. The token also covers all arguments used to produce
// the patch.
type ReviewedDiffPatch struct {
	Filename string
	Options  DiffPatchOptions
	Exists   bool
	Size     int64
	Digest   string
	Token    ConfirmationToken
}

// ReviewDiffPatch validates the output path and snapshots an existing regular
// file without following a final-component symlink.
func (r *Repository) ReviewDiffPatch(filename string, options DiffPatchOptions) (ReviewedDiffPatch, error) {
	path, err := r.patchUIOutputPath(filename)
	if err != nil {
		return ReviewedDiffPatch{}, err
	}
	review := ReviewedDiffPatch{Filename: path, Options: cloneDiffPatchOptions(options)}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		review.Token = NewConfirmationToken(diffPatchReviewIdentity(review))
		return review, nil
	}
	if err != nil {
		return review, fmt.Errorf("inspect patch output: %w", err)
	}
	if !info.Mode().IsRegular() {
		return review, errors.New("patch output already exists and is not a regular file")
	}
	if info.Size() > maxReviewedPatch {
		return review, &TooLargeError{Resource: "existing patch output"}
	}
	f, err := os.Open(path)
	if err != nil {
		return review, fmt.Errorf("open patch output for review: %w", err)
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, io.LimitReader(f, maxReviewedPatch+1))
	closeErr := f.Close()
	if copyErr != nil {
		return review, fmt.Errorf("read patch output for review: %w", copyErr)
	}
	if closeErr != nil {
		return review, fmt.Errorf("close patch output review: %w", closeErr)
	}
	review.Exists, review.Size, review.Digest = true, info.Size(), hex.EncodeToString(h.Sum(nil))
	review.Token = NewConfirmationToken(diffPatchReviewIdentity(review))
	return review, nil
}

// ExecuteReviewedDiffPatch rejects a changed destination. SaveDiffPatch then
// installs the complete temporary file with its atomic create/rename path.
func (r *Repository) ExecuteReviewedDiffPatch(ctx context.Context, review ReviewedDiffPatch) error {
	current, err := r.ReviewDiffPatch(review.Filename, review.Options)
	if err != nil {
		return err
	}
	if !review.Token.validFor(diffPatchReviewIdentity(current)) {
		return fmt.Errorf("patch output changed after review: %w", ErrStalePlan)
	}
	if current.Exists && !review.Options.Overwrite {
		return errors.New("patch output exists; enable overwrite and review again")
	}
	return r.SaveDiffPatch(ctx, review.Filename, review.Options)
}

// FormatPatchUI applies a UI-specific series bound before creating files.
func (r *Repository) FormatPatchUI(ctx context.Context, revisionRange string, options FormatPatchOptions) ([]string, error) {
	if err := safeRevisionArgument(revisionRange); err != nil {
		return nil, err
	}
	out, err := r.output(ctx, "rev-list", "--count", revisionRange)
	if err != nil {
		return nil, fmt.Errorf("count format-patch commits: %w", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, fmt.Errorf("parse format-patch commit count: %w", err)
	}
	if count > maxUIPatchSeries {
		return nil, &TooLargeError{Resource: "format-patch series"}
	}
	return r.FormatPatch(ctx, revisionRange, options)
}

func (r *Repository) patchUIOutputPath(filename string) (string, error) {
	if filename == "" || strings.ContainsRune(filename, 0) {
		return "", errors.New("patch output filename is empty or invalid")
	}
	path := filename
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.commandDir, path)
	}
	path = filepath.Clean(path)
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("inspect patch output directory: %w", err)
	}
	if !parent.IsDir() {
		return "", errors.New("patch output parent is not a directory")
	}
	return path, nil
}

func cloneDiffPatchOptions(in DiffPatchOptions) DiffPatchOptions {
	in.Paths = append([]string(nil), in.Paths...)
	return in
}

func diffPatchReviewIdentity(review ReviewedDiffPatch) string {
	return strings.Join([]string{
		review.Filename, strconv.FormatBool(review.Exists), strconv.FormatInt(review.Size, 10), review.Digest,
		strconv.FormatBool(review.Options.Cached), review.Options.Range,
		strings.Join(review.Options.Paths, "\x00"), strconv.FormatBool(review.Options.Overwrite),
	}, "\x01")
}
