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
	"sort"
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

// ReviewedApplyPatch binds the reviewed patch bytes and git-apply check to an
// execution. The patch is deliberately limited for UI review; callers that
// need unbounded automation can use ApplyPatch directly.
type ReviewedApplyPatch struct {
	Filename string
	Options  ApplyPatchOptions
	Size     int64
	Digest   string
	Token    ConfirmationToken
}

// ReviewedAMStart snapshots every selected mbox or maildir before git am is
// started. It intentionally does not predict merge conflicts; Git owns that
// stateful operation, but changing the selected mail after review is rejected.
type ReviewedAMStart struct {
	Inputs  []ReviewedPatchInput
	Options AMOptions
	Token   ConfirmationToken
}

type ReviewedPatchInput struct {
	Path   string
	Size   int64
	Digest string
}

// ReviewedFormatPatch holds a complete series generated in a private staging
// directory. Execution publishes those exact regular files without replacing
// any output already present at review time.
type ReviewedFormatPatch struct {
	Directory  string
	Range      string
	Options    FormatPatchOptions
	Revisions  []string
	Before     []string
	StagingDir string
	Files      []ReviewedPatchFile
	Token      ConfirmationToken
}

type ReviewedPatchFile struct {
	Name   string
	Size   int64
	Digest string
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
	size, digest, err := reviewedRegularFile(path, "existing patch output")
	if err != nil {
		return review, err
	}
	review.Exists, review.Size, review.Digest = true, size, digest
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

// ReviewApplyPatch verifies a bounded regular patch file and asks Git whether
// the exact requested application is currently possible. Execute repeats both
// checks, so changing either the patch or the index/worktree requires review
// again.
func (r *Repository) ReviewApplyPatch(ctx context.Context, filename string, options ApplyPatchOptions) (ReviewedApplyPatch, error) {
	if options.Index && options.Cached {
		return ReviewedApplyPatch{}, errors.New("git apply index and cached modes are mutually exclusive")
	}
	path, err := r.patchUIInputFile(filename)
	if err != nil {
		return ReviewedApplyPatch{}, err
	}
	size, digest, err := reviewedRegularFile(path, "patch input")
	if err != nil {
		return ReviewedApplyPatch{}, err
	}
	review := ReviewedApplyPatch{Filename: path, Options: options, Size: size, Digest: digest}
	if err := r.run(ctx, applyPatchArgs(path, options, true)...); err != nil {
		return review, fmt.Errorf("check patch application: %w", err)
	}
	review.Token = NewConfirmationToken(applyPatchReviewIdentity(review))
	return review, nil
}

func (r *Repository) ExecuteReviewedApplyPatch(ctx context.Context, review ReviewedApplyPatch) error {
	current, err := r.ReviewApplyPatch(ctx, review.Filename, review.Options)
	if err != nil {
		return err
	}
	if !review.Token.validFor(applyPatchReviewIdentity(current)) {
		return fmt.Errorf("patch or repository changed after review: %w", ErrStalePlan)
	}
	return r.ApplyPatch(ctx, review.Filename, review.Options)
}

func (r *Repository) ReviewAMStart(inputs []string, options AMOptions) (ReviewedAMStart, error) {
	if len(inputs) == 0 {
		return ReviewedAMStart{}, errors.New("git am requires at least one patch file or maildir")
	}
	review := ReviewedAMStart{Options: options}
	for _, input := range inputs {
		path, err := r.existingInputPath(input)
		if err != nil {
			return ReviewedAMStart{}, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return ReviewedAMStart{}, fmt.Errorf("inspect patch input: %w", err)
		}
		var size int64
		var digest string
		if info.Mode().IsRegular() {
			size, digest, err = reviewedRegularFile(path, "patch input")
		} else if info.IsDir() {
			size, digest, err = reviewedPatchDirectory(path)
		} else {
			err = errors.New("patch input is not a regular file or directory")
		}
		if err != nil {
			return ReviewedAMStart{}, err
		}
		review.Inputs = append(review.Inputs, ReviewedPatchInput{Path: path, Size: size, Digest: digest})
	}
	review.Token = NewConfirmationToken(amStartReviewIdentity(review))
	return review, nil
}

func (r *Repository) ExecuteReviewedAMStart(ctx context.Context, review ReviewedAMStart) error {
	inputs := make([]string, 0, len(review.Inputs))
	for _, input := range review.Inputs {
		inputs = append(inputs, input.Path)
	}
	current, err := r.ReviewAMStart(inputs, review.Options)
	if err != nil {
		return err
	}
	if !review.Token.validFor(amStartReviewIdentity(current)) {
		return fmt.Errorf("patch inputs changed after review: %w", ErrStalePlan)
	}
	return r.AMStart(ctx, inputs, review.Options)
}

// ReviewFormatPatchUI creates the exact bounded series in a private directory
// inside the selected output directory. This makes publication an atomic
// create-only operation rather than allowing git format-patch to overwrite a
// guessed filename.
func (r *Repository) ReviewFormatPatchUI(ctx context.Context, revisionRange string, options FormatPatchOptions) (ReviewedFormatPatch, error) {
	if err := safeRevisionArgument(revisionRange); err != nil {
		return ReviewedFormatPatch{}, err
	}
	revisions, err := r.formatPatchRevisions(ctx, revisionRange)
	if err != nil {
		return ReviewedFormatPatch{}, err
	}
	if len(revisions) == 0 {
		return ReviewedFormatPatch{}, errors.New("revision range contains no commits to format")
	}
	dir, before, err := r.patchUIOutputDirectory(options.OutputDirectory)
	if err != nil {
		return ReviewedFormatPatch{}, err
	}
	stage, err := os.MkdirTemp(dir, ".lazymagit-format-patch-")
	if err != nil {
		return ReviewedFormatPatch{}, fmt.Errorf("create format-patch staging directory: %w", err)
	}
	review := ReviewedFormatPatch{Directory: dir, Range: revisionRange, Options: cloneFormatPatchOptions(options), Revisions: revisions, Before: before, StagingDir: stage}
	stagedOptions := cloneFormatPatchOptions(options)
	stagedOptions.OutputDirectory = stage
	created, err := r.FormatPatch(ctx, revisionRange, stagedOptions)
	if err != nil {
		_ = os.RemoveAll(stage)
		return ReviewedFormatPatch{}, err
	}
	review.Files, err = reviewStagedFormatPatchFiles(created, before)
	if err != nil {
		_ = os.RemoveAll(stage)
		return ReviewedFormatPatch{}, err
	}
	if len(review.Files) == 0 {
		_ = os.RemoveAll(stage)
		return ReviewedFormatPatch{}, errors.New("format-patch created no patch files")
	}
	sort.Slice(review.Files, func(i, j int) bool { return review.Files[i].Name < review.Files[j].Name })
	review.Token = NewConfirmationToken(formatPatchReviewIdentity(review))
	return review, nil
}

func reviewStagedFormatPatchFiles(created []string, before []string) ([]ReviewedPatchFile, error) {
	files := make([]ReviewedPatchFile, 0, len(created))
	for _, path := range created {
		file, err := reviewStagedFormatPatchFile(path, before)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func reviewStagedFormatPatchFile(path string, before []string) (ReviewedPatchFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return ReviewedPatchFile{}, fmt.Errorf("inspect staged format-patch output: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ReviewedPatchFile{}, errors.New("format-patch created a non-regular output")
	}
	size, digest, err := reviewedRegularFile(path, "staged format-patch output")
	if err != nil {
		return ReviewedPatchFile{}, err
	}
	name := filepath.Base(path)
	if name != filepath.Base(filepath.Clean(name)) || name == "." || name == "" {
		return ReviewedPatchFile{}, errors.New("format-patch created an unsafe output name")
	}
	if containsPatchName(before, name) {
		return ReviewedPatchFile{}, fmt.Errorf("format-patch output %q already exists; choose an empty directory or rename the existing file", name)
	}
	return ReviewedPatchFile{Name: name, Size: size, Digest: digest}, nil
}

// ExecuteReviewedFormatPatch publishes only the reviewed staged files. A
// changed revision range, output directory listing, or staged file invalidates
// the review and leaves the destination untouched.
func (r *Repository) ExecuteReviewedFormatPatch(ctx context.Context, review ReviewedFormatPatch) error {
	defer r.DiscardReviewedFormatPatch(review)
	if err := r.validateReviewedFormatPatch(ctx, review); err != nil {
		return err
	}
	return publishReviewedFormatPatch(review)
}

func (r *Repository) validateReviewedFormatPatch(ctx context.Context, review ReviewedFormatPatch) error {
	revisions, err := r.formatPatchRevisions(ctx, review.Range)
	if err != nil {
		return err
	}
	if !samePatchStrings(revisions, review.Revisions) {
		return fmt.Errorf("revision range changed after review: %w", ErrStalePlan)
	}
	_, before, err := r.patchUIOutputDirectory(review.Directory)
	if err != nil {
		return err
	}
	// The private staging directory is the one intentional entry added by
	// review itself; every other listing change invalidates publication.
	before = withoutPatchName(before, filepath.Base(review.StagingDir))
	if !samePatchStrings(before, review.Before) {
		return fmt.Errorf("format-patch output directory changed after review: %w", ErrStalePlan)
	}
	current := review
	current.Files = nil
	for _, file := range review.Files {
		path := filepath.Join(review.StagingDir, file.Name)
		size, digest, hashErr := reviewedRegularFile(path, "staged format-patch output")
		if hashErr != nil {
			return hashErr
		}
		current.Files = append(current.Files, ReviewedPatchFile{Name: file.Name, Size: size, Digest: digest})
	}
	if !review.Token.validFor(formatPatchReviewIdentity(current)) {
		return fmt.Errorf("format-patch review changed after review: %w", ErrStalePlan)
	}
	return nil
}

func publishReviewedFormatPatch(review ReviewedFormatPatch) error {
	var installed []string
	for _, file := range review.Files {
		source, destination := filepath.Join(review.StagingDir, file.Name), filepath.Join(review.Directory, file.Name)
		if err := linkOrCopyExclusive(source, destination); err != nil {
			// Never blindly remove a path after a failed concurrent publication.
			// Linked files can be identified exactly and safely rolled back.
			for _, path := range installed {
				if sameFile(path, filepath.Join(review.StagingDir, filepath.Base(path))) {
					_ = os.Remove(path)
				}
			}
			return fmt.Errorf("publish format-patch output %q: %w", file.Name, err)
		}
		installed = append(installed, destination)
	}
	return nil
}

func (r *Repository) DiscardReviewedFormatPatch(review ReviewedFormatPatch) {
	_ = os.RemoveAll(review.StagingDir)
}

// FormatPatchUI applies a UI-specific series bound before creating files.
// New interactive callers should use ReviewFormatPatchUI and
// ExecuteReviewedFormatPatch so output publication is reviewed and stale-safe.
func (r *Repository) FormatPatchUI(ctx context.Context, revisionRange string, options FormatPatchOptions) ([]string, error) {
	if _, err := r.formatPatchRevisions(ctx, revisionRange); err != nil {
		return nil, err
	}
	return r.FormatPatch(ctx, revisionRange, options)
}

func (r *Repository) formatPatchRevisions(ctx context.Context, revisionRange string) ([]string, error) {
	if err := safeRevisionArgument(revisionRange); err != nil {
		return nil, err
	}
	out, err := r.output(ctx, "rev-list", "--reverse", revisionRange)
	if err != nil {
		return nil, fmt.Errorf("list format-patch commits: %w", err)
	}
	values := strings.Fields(string(out))
	if len(values) > maxUIPatchSeries {
		return nil, &TooLargeError{Resource: "format-patch series"}
	}
	return values, nil
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

func (r *Repository) patchUIInputFile(filename string) (string, error) {
	if filename == "" || strings.ContainsRune(filename, 0) {
		return "", errors.New("patch input filename is empty or invalid")
	}
	path := filename
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.commandDir, path)
	}
	path = filepath.Clean(path)
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect patch input: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("patch input is not a regular file")
	}
	return path, nil
}

func (r *Repository) patchUIOutputDirectory(name string) (string, []string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", nil, errors.New("format-patch output directory is empty or invalid")
	}
	dir := name
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.commandDir, dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Lstat(dir)
	if err != nil {
		return "", nil, fmt.Errorf("inspect format-patch output directory: %w", err)
	}
	if !info.IsDir() {
		return "", nil, errors.New("format-patch output directory is not a directory or is a symlink")
	}
	names, err := boundedDirectoryNames(dir, maxFormatPatchDirectoryEntries)
	if err != nil {
		return "", nil, fmt.Errorf("read format-patch output directory: %w", err)
	}
	sort.Strings(names)
	return dir, names, nil
}

func reviewedPatchDirectory(path string) (int64, string, error) {
	h := sha256.New()
	var total int64
	entries := 0
	err := filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if candidate == path {
			return nil
		}
		entries++
		if entries > maxUIPatchSeries {
			return &TooLargeError{Resource: "maildir input"}
		}
		rel, err := filepath.Rel(path, candidate)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("maildir input contains a symlink")
		}
		if entry.IsDir() {
			_, _ = io.WriteString(h, "d\x00"+rel+"\x00")
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("maildir input contains a non-regular file")
		}
		size, digest, err := reviewedRegularFile(candidate, "maildir patch input")
		if err != nil {
			return err
		}
		total += size
		_, _ = io.WriteString(h, "f\x00"+rel+"\x00"+strconv.FormatInt(size, 10)+"\x00"+digest+"\x00")
		return nil
	})
	if err != nil {
		return 0, "", fmt.Errorf("review maildir input: %w", err)
	}
	return total, hex.EncodeToString(h.Sum(nil)), nil
}

func reviewedRegularFile(path, resource string) (int64, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, "", fmt.Errorf("inspect %s: %w", resource, err)
	}
	if !info.Mode().IsRegular() {
		return 0, "", fmt.Errorf("%s is not a regular file", resource)
	}
	if info.Size() > maxReviewedPatch {
		return 0, "", &TooLargeError{Resource: resource}
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("open %s: %w", resource, err)
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, io.LimitReader(f, maxReviewedPatch+1))
	closeErr := f.Close()
	if copyErr != nil {
		return 0, "", fmt.Errorf("read %s: %w", resource, copyErr)
	}
	if closeErr != nil {
		return 0, "", fmt.Errorf("close %s: %w", resource, closeErr)
	}
	return info.Size(), hex.EncodeToString(h.Sum(nil)), nil
}

func cloneDiffPatchOptions(in DiffPatchOptions) DiffPatchOptions {
	in.Paths = append([]string(nil), in.Paths...)
	return in
}

func cloneFormatPatchOptions(in FormatPatchOptions) FormatPatchOptions {
	in.To, in.Cc = append([]string(nil), in.To...), append([]string(nil), in.Cc...)
	return in
}

func diffPatchReviewIdentity(review ReviewedDiffPatch) string {
	return strings.Join([]string{review.Filename, strconv.FormatBool(review.Exists), strconv.FormatInt(review.Size, 10), review.Digest, strconv.FormatBool(review.Options.Cached), review.Options.Range, strings.Join(review.Options.Paths, "\x00"), strconv.FormatBool(review.Options.Overwrite)}, "\x01")
}

func applyPatchReviewIdentity(review ReviewedApplyPatch) string {
	return strings.Join([]string{review.Filename, strconv.FormatInt(review.Size, 10), review.Digest, strconv.FormatBool(review.Options.Index), strconv.FormatBool(review.Options.Cached), strconv.FormatBool(review.Options.ThreeWay)}, "\x01")
}

func amStartReviewIdentity(review ReviewedAMStart) string {
	parts := []string{strconv.FormatBool(review.Options.ThreeWay), strconv.FormatBool(review.Options.KeepCR), strconv.FormatBool(review.Options.Scissors), strconv.FormatBool(review.Options.Signoff)}
	for _, input := range review.Inputs {
		parts = append(parts, input.Path, strconv.FormatInt(input.Size, 10), input.Digest)
	}
	return strings.Join(parts, "\x01")
}

func formatPatchReviewIdentity(review ReviewedFormatPatch) string {
	parts := []string{review.Directory, review.Range, patchIdentity(review.Revisions...), patchIdentity(review.Before...), formatPatchOptionsIdentity(review.Options)}
	for _, file := range review.Files {
		parts = append(parts, file.Name, strconv.FormatInt(file.Size, 10), file.Digest)
	}
	return patchIdentity(parts...)
}

// patchIdentity length-prefixes every value, including editable cover text.
// Delimiter joins would let a control character in a user-provided body blur
// the boundary between fields covered by a confirmation token.
func patchIdentity(parts ...string) string {
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

func formatPatchOptionsIdentity(options FormatPatchOptions) string {
	parts := []string{strconv.FormatBool(options.Numbered), strconv.FormatBool(options.CoverLetter), strconv.FormatBool(options.Signoff), strconv.FormatBool(options.Thread), options.ThreadStyle, strconv.FormatBool(options.RFC), options.SubjectPrefix, strconv.Itoa(options.RerollCount), strconv.Itoa(options.StartNumber), options.From, options.InReplyTo, options.Base, strconv.Itoa(len(options.To))}
	parts = append(parts, options.To...)
	parts = append(parts, strconv.Itoa(len(options.Cc)))
	parts = append(parts, options.Cc...)
	parts = append(parts, options.CoverLetterBody)
	return patchIdentity(parts...)
}

func containsPatchName(names []string, name string) bool {
	i := sort.SearchStrings(names, name)
	return i < len(names) && names[i] == name
}

func withoutPatchName(names []string, name string) []string {
	i := sort.SearchStrings(names, name)
	if i == len(names) || names[i] != name {
		return names
	}
	return append(append([]string(nil), names[:i]...), names[i+1:]...)
}

func samePatchStrings(left, right []string) bool {
	return len(left) == len(right) && strings.Join(left, "\x00") == strings.Join(right, "\x00")
}

func linkOrCopyExclusive(source, destination string) error {
	if err := os.Link(source, destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return copyFileExclusive(source, destination)
	} else {
		return err
	}
}

func sameFile(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}
