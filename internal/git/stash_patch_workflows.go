package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrNoAMInProgress = errors.New("no git am operation in progress")

type ConfirmationOptions struct{ Token ConfirmationToken }

type Stash struct {
	Ref, ID, ShortID, Subject, Author string
	Date                              time.Time
}

const stashListFormat = "%gd%x00%H%x00%h%x00%gs%x00%an%x00%aI"

const stashListOutputLimit = 8 << 20

// Stashes returns the stash reflog in newest-first order.
func (r *Repository) Stashes(ctx context.Context) ([]Stash, error) {
	out, truncated, err := r.outputLimited(ctx, stashListOutputLimit, "stash", "list", "--format="+stashListFormat)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, errors.New("git stash list output exceeds limit")
	}
	var stashes []Stash
	for _, line := range bytes.Split(bytes.TrimSuffix(out, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		fields := bytes.Split(line, []byte{0})
		if len(fields) != 6 {
			return nil, errors.New("malformed git stash list output")
		}
		date, err := time.Parse(time.RFC3339, string(fields[5]))
		if err != nil {
			return nil, fmt.Errorf("parse stash date: %w", err)
		}
		stashes = append(stashes, Stash{
			Ref: string(fields[0]), ID: string(fields[1]), ShortID: string(fields[2]),
			Subject: string(fields[3]), Author: string(fields[4]), Date: date,
		})
	}
	return stashes, nil
}

// ListStashes is an explicit-name alias for Stashes.
func (r *Repository) ListStashes(ctx context.Context) ([]Stash, error) { return r.Stashes(ctx) }

type StashDetails struct {
	Stash          Stash
	Patch          string
	PatchTruncated bool
}

const stashShowOutputLimit = 8 << 20
const maxFormatPatchDirectoryEntries = 100000

// ShowStash returns typed metadata and a bounded, terminal-neutral patch.
func (r *Repository) ShowStash(ctx context.Context, ref string) (StashDetails, error) {
	oid, err := r.resolveCommit(ctx, defaultStashRef(ref))
	if err != nil {
		return StashDetails{}, err
	}
	meta, err := r.output(ctx, "show", "-s", "--format=%H%x00%h%x00%s%x00%an%x00%aI", oid)
	if err != nil {
		return StashDetails{}, err
	}
	fields := bytes.Split(bytes.TrimSuffix(meta, []byte("\n")), []byte{0})
	if len(fields) != 5 {
		return StashDetails{}, errors.New("malformed stash metadata")
	}
	date, err := time.Parse(time.RFC3339, string(fields[4]))
	if err != nil {
		return StashDetails{}, fmt.Errorf("parse stash date: %w", err)
	}
	patch, truncated, err := r.outputLimited(ctx, stashShowOutputLimit,
		"--no-pager", "stash", "show", "--patch", "--stat", "--no-color", "--no-ext-diff", "--no-textconv", oid)
	if err != nil {
		return StashDetails{}, err
	}
	return StashDetails{
		Stash: Stash{Ref: defaultStashRef(ref), ID: string(fields[0]), ShortID: string(fields[1]), Subject: string(fields[2]), Author: string(fields[3]), Date: date},
		Patch: string(patch), PatchTruncated: truncated,
	}, nil
}

// StashPushOptions implements Git's ordinary "both index and worktree"
// stash. KeepIndex, IncludeUntracked, All, and literal path lists correspond to
// git-stash options. Magit's custom index-only/worktree-only plumbing is
// intentionally not represented here because it requires stronger testing.
type StashPushOptions struct {
	Message          string
	IncludeUntracked bool
	All              bool
	KeepIndex        bool
	Paths            []string
}

func (r *Repository) StashPush(ctx context.Context, options StashPushOptions) error {
	args := []string{"stash", "push"}
	if options.All {
		args = append(args, "--all")
	} else if options.IncludeUntracked {
		args = append(args, "--include-untracked")
	}
	if options.KeepIndex {
		args = append(args, "--keep-index")
	}
	if options.Message != "" {
		args = append(args, "--message", options.Message)
	}
	if len(options.Paths) > 0 {
		args = pathsArgs(args, options.Paths)
	} else {
		// git-stash's script uses the magic pathspec :/ internally when no
		// pathspec is supplied, which conflicts with our literal-path invariant.
		// An explicit repository-root path has the same scope without magic.
		args = pathsArgs(args, []string{"."})
	}
	return r.run(ctx, args...)
}

type StashApplyOptions struct{ Index bool }

func (r *Repository) StashApply(ctx context.Context, ref string, options StashApplyOptions) error {
	_, oid, err := r.stashIdentity(ctx, ref)
	if err != nil {
		return err
	}
	return r.stashApplyArgument(ctx, "apply", oid, options)
}

func (r *Repository) StashPop(ctx context.Context, ref string, options StashApplyOptions) error {
	return r.stashApplyOrPop(ctx, "pop", ref, options)
}

func (r *Repository) stashApplyOrPop(ctx context.Context, operation, ref string, options StashApplyOptions) error {
	argument := ""
	if ref != "" {
		selector, err := r.stashSelector(ctx, ref)
		if err != nil {
			return err
		}
		argument = selector
	}
	return r.stashApplyArgument(ctx, operation, argument, options)
}

func (r *Repository) stashApplyArgument(ctx context.Context, operation, argument string, options StashApplyOptions) error {
	args := []string{"stash", operation}
	if options.Index {
		args = append(args, "--index")
	}
	if argument != "" {
		args = append(args, argument)
	}
	return r.run(ctx, args...)
}

func (r *Repository) StashDrop(ctx context.Context, ref string, options ConfirmationOptions) error {
	selector, oid, err := r.stashIdentity(ctx, ref)
	if err != nil {
		return err
	}
	if !options.Token.validFor(oid) {
		return &ConfirmationRequiredError{Operation: "drop stash", Identity: oid}
	}
	return r.run(ctx, "stash", "drop", selector)
}

func (r *Repository) StashClear(ctx context.Context, options ConfirmationOptions) error {
	if !options.Token.validFor("all-stashes") {
		return &ConfirmationRequiredError{Operation: "clear stashes"}
	}
	return r.run(ctx, "stash", "clear")
}

// StashBranch creates and switches to branch at a stash's original base. Git
// drops the stash after applying it successfully.
func (r *Repository) StashBranch(ctx context.Context, branch, ref string) error {
	if err := r.validateStashBranchName(ctx, branch); err != nil {
		return err
	}
	selector, err := r.stashSelector(ctx, ref)
	if err != nil {
		return err
	}
	return r.run(ctx, "stash", "branch", branch, selector)
}

func (r *Repository) validateStashBranchName(ctx context.Context, branch string) error {
	if _, err := r.output(ctx, "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("invalid stash branch name: %w", err)
	}
	return nil
}

func defaultStashRef(ref string) string {
	if ref == "" {
		return "stash@{0}"
	}
	return ref
}

func (r *Repository) stashSelector(ctx context.Context, ref string) (string, error) {
	selector, _, err := r.stashIdentity(ctx, ref)
	return selector, err
}

func (r *Repository) stashIdentity(ctx context.Context, ref string) (string, string, error) {
	want := defaultStashRef(ref)
	stashes, err := r.Stashes(ctx)
	if err != nil {
		return "", "", err
	}
	for _, stash := range stashes {
		if want == stash.Ref || want == stash.ID || want == stash.ShortID {
			return stash.Ref, stash.ID, nil
		}
	}
	return "", "", fmt.Errorf("stash %q does not exist", want)
}

func (r *Repository) resolveCommit(ctx context.Context, revision string) (string, error) {
	if strings.TrimSpace(revision) == "" {
		return "", errors.New("revision is empty")
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	return trimLine(out), nil
}

type AMOptions struct {
	ThreeWay bool
	KeepCR   bool
	Scissors bool
	Signoff  bool
}

type AMState struct {
	InProgress bool
	Current    int
	Last       int
}

// AMState distinguishes git-am from the other operations which can also use a
// rebase-apply directory.
func (r *Repository) AMState() (AMState, error) {
	dir := filepath.Join(r.gitDir, "rebase-apply")
	if _, err := os.Stat(filepath.Join(dir, "applying")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AMState{}, nil
		}
		return AMState{}, fmt.Errorf("inspect git am state: %w", err)
	}
	state := AMState{InProgress: true}
	for name, target := range map[string]*int{"next": &state.Current, "last": &state.Last} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return AMState{}, fmt.Errorf("read git am %s: %w", name, err)
		}
		if len(data) > 0 {
			value, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				return AMState{}, fmt.Errorf("parse git am %s: %w", name, err)
			}
			*target = value
		}
	}
	return state, nil
}

// AMStatus is the context-shaped query variant used by callers whose backend
// queries uniformly carry a context. Detection itself performs no process.
func (r *Repository) AMStatus(context.Context) (AMState, error) { return r.AMState() }

// AMStart applies one or more mbox files or maildir directories. Inputs are
// passed as literal filesystem arguments, never through a shell.
func (r *Repository) AMStart(ctx context.Context, inputs []string, options AMOptions) error {
	if len(inputs) == 0 {
		return errors.New("git am requires at least one patch file or maildir")
	}
	args := []string{"am"}
	if options.ThreeWay {
		args = append(args, "--3way")
	}
	if options.KeepCR {
		args = append(args, "--keep-cr")
	}
	if options.Scissors {
		args = append(args, "--scissors")
	}
	if options.Signoff {
		args = append(args, "--signoff")
	}
	args = append(args, "--")
	for _, input := range inputs {
		path, err := r.existingInputPath(input)
		if err != nil {
			return err
		}
		args = append(args, path)
	}
	return r.run(ctx, args...)
}

func (r *Repository) AMContinue(ctx context.Context) error { return r.amControl(ctx, "--continue") }
func (r *Repository) AMSkip(ctx context.Context) error     { return r.amControl(ctx, "--skip") }
func (r *Repository) AMAbort(ctx context.Context) error    { return r.amControl(ctx, "--abort") }

func (r *Repository) amControl(ctx context.Context, operation string) error {
	state, err := r.AMState()
	if err != nil {
		return err
	}
	if !state.InProgress {
		return ErrNoAMInProgress
	}
	return r.run(ctx, "am", operation)
}

type ApplyPatchOptions struct {
	Index    bool
	Cached   bool
	ThreeWay bool
}

// ApplyPatch runs plain git-apply for one patch file.
func (r *Repository) ApplyPatch(ctx context.Context, patchFile string, options ApplyPatchOptions) error {
	if options.Index && options.Cached {
		return errors.New("git apply index and cached modes are mutually exclusive")
	}
	path, err := r.existingInputPath(patchFile)
	if err != nil {
		return err
	}
	return r.run(ctx, applyPatchArgs(path, options, false)...)
}

func applyPatchArgs(path string, options ApplyPatchOptions, check bool) []string {
	args := []string{"apply"}
	if check {
		args = append(args, "--check")
	}
	if options.Index {
		args = append(args, "--index")
	}
	if options.Cached {
		args = append(args, "--cached")
	}
	if options.ThreeWay {
		args = append(args, "--3way")
	}
	return append(args, "--", path)
}

func (r *Repository) existingInputPath(name string) (string, error) {
	if name == "" {
		return "", errors.New("input path is empty")
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.commandDir, path)
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("inspect input %q: %w", name, err)
	}
	return path, nil
}

const (
	maxFormatPatchHeaderBytes = 998 // RFC 5322's physical header-line limit.
	maxFormatPatchRecipients  = 128
	maxFormatPatchCoverBody   = 64 << 10
	maxFormatPatchRevision    = 4096
)

var messageIDRE = regexp.MustCompile(`^<[^<>[:space:]@]+@[^<>[:space:]]+>$`)

type FormatPatchOptions struct {
	OutputDirectory string
	Numbered        bool
	CoverLetter     bool
	Signoff         bool
	Thread          bool
	// ThreadStyle is deliberately limited to Git's portable styles. An empty
	// value means Git's --thread default when Thread is set.
	ThreadStyle   string
	RFC           bool
	SubjectPrefix string
	RerollCount   int
	StartNumber   int
	From          string
	InReplyTo     string
	Base          string
	To            []string
	Cc            []string
	// CoverLetterBody is edited in the terminal and replaces Git's generated
	// cover-letter placeholder. A non-empty body implies CoverLetter.
	CoverLetterBody string
}

// FormatPatch writes a revision range into an existing output directory and
// returns the patch files created there. Existing files are not reported.
func (r *Repository) FormatPatch(ctx context.Context, revisionRange string, options FormatPatchOptions) ([]string, error) {
	if err := safeRevisionArgument(revisionRange); err != nil {
		return nil, err
	}
	if err := validateFormatPatchOptions(options); err != nil {
		return nil, err
	}
	if options.CoverLetterBody != "" {
		options.CoverLetter = true
	}
	dir, before, err := r.patchOutputDirectory(options.OutputDirectory)
	if err != nil {
		return nil, err
	}
	args := formatPatchArgs(dir, revisionRange, options)
	if err := r.run(ctx, args...); err != nil {
		return nil, err
	}
	created, err := newFormatPatchFiles(dir, before)
	if err != nil {
		return nil, err
	}
	if options.CoverLetterBody != "" {
		if err := replaceFormatPatchCoverLetter(created, options.CoverLetterBody); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func formatPatchArgs(dir, revisionRange string, options FormatPatchOptions) []string {
	args := []string{"format-patch", "--quiet", "--output-directory=" + dir}
	if options.Numbered {
		args = append(args, "--numbered")
	}
	if options.CoverLetter {
		args = append(args, "--cover-letter")
	}
	if options.Signoff {
		args = append(args, "--signoff")
	}
	if options.ThreadStyle != "" {
		args = append(args, "--thread="+options.ThreadStyle)
	} else if options.Thread {
		args = append(args, "--thread")
	}
	if options.RFC {
		args = append(args, "--rfc")
	}
	if options.SubjectPrefix != "" {
		args = append(args, "--subject-prefix="+options.SubjectPrefix)
	}
	if options.RerollCount > 0 {
		args = append(args, "--reroll-count="+strconv.Itoa(options.RerollCount))
	}
	if options.StartNumber > 0 {
		args = append(args, "--start-number="+strconv.Itoa(options.StartNumber))
	}
	if options.From != "" {
		args = append(args, "--from="+options.From)
	}
	if options.InReplyTo != "" {
		args = append(args, "--in-reply-to="+options.InReplyTo)
	}
	if options.Base != "" {
		args = append(args, "--base="+options.Base)
	}
	for _, address := range options.To {
		args = append(args, "--to="+address)
	}
	for _, address := range options.Cc {
		args = append(args, "--cc="+address)
	}
	return append(args, revisionRange)
}

func newFormatPatchFiles(dir string, before map[string]bool) ([]string, error) {
	entries, err := boundedDirectoryNames(dir, maxFormatPatchDirectoryEntries)
	if err != nil {
		return nil, fmt.Errorf("read format-patch output: %w", err)
	}
	var created []string
	for _, name := range entries {
		if !before[name] && strings.HasSuffix(name, ".patch") {
			path := filepath.Join(dir, name)
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return nil, fmt.Errorf("inspect format-patch output: %w", statErr)
			}
			if !info.Mode().IsRegular() {
				return nil, errors.New("format-patch created a non-regular output")
			}
			created = append(created, path)
		}
	}
	sort.Strings(created)
	return created, nil
}

func validateFormatPatchOptions(options FormatPatchOptions) error {
	if err := validateFormatPatchScalars(options); err != nil {
		return err
	}
	if err := validFormatPatchHeader("subject prefix", options.SubjectPrefix, true); err != nil {
		return err
	}
	if err := validFormatPatchAddress("from", options.From, true); err != nil {
		return err
	}
	if err := validateFormatPatchReplyAndBase(options); err != nil {
		return err
	}
	if err := validFormatPatchAddresses("to", options.To); err != nil {
		return err
	}
	if err := validFormatPatchAddresses("cc", options.Cc); err != nil {
		return err
	}
	return validateFormatPatchCoverBody(options.CoverLetterBody)
}

func validateFormatPatchScalars(options FormatPatchOptions) error {
	if options.RerollCount < 0 || options.StartNumber < 0 {
		return errors.New("format-patch numeric options cannot be negative")
	}
	if options.ThreadStyle != "" && options.ThreadStyle != "shallow" && options.ThreadStyle != "deep" {
		return errors.New("format-patch thread style must be shallow or deep")
	}
	return nil
}

func validateFormatPatchReplyAndBase(options FormatPatchOptions) error {
	if options.InReplyTo != "" && (!messageIDRE.MatchString(options.InReplyTo) || validFormatPatchHeader("in-reply-to", options.InReplyTo, false) != nil) {
		return errors.New("format-patch in-reply-to must be a bounded message ID")
	}
	if options.Base == "" {
		return nil
	}
	if err := safeRevisionArgument(options.Base); err != nil {
		return fmt.Errorf("format-patch base: %w", err)
	}
	return nil
}

func validateFormatPatchCoverBody(body string) error {
	if len(body) > maxFormatPatchCoverBody || !utf8.ValidString(body) || strings.ContainsRune(body, 0) || strings.ContainsRune(body, '\r') {
		return errors.New("format-patch cover letter body is invalid or exceeds 64 KiB")
	}
	for _, r := range body {
		if r < 32 && r != '\n' || r == 127 {
			return errors.New("format-patch cover letter body contains a control character")
		}
	}
	return nil
}

func validFormatPatchAddresses(name string, addresses []string) error {
	if len(addresses) > maxFormatPatchRecipients {
		return fmt.Errorf("format-patch %s recipients exceed %d", name, maxFormatPatchRecipients)
	}
	for _, address := range addresses {
		if err := validFormatPatchAddress(name, address, false); err != nil {
			return err
		}
	}
	return nil
}

func validFormatPatchAddress(name, value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if err := validFormatPatchHeader(name, value, false); err != nil {
		return err
	}
	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("format-patch %s recipient is invalid", name)
	}
	return nil
}

func validFormatPatchHeader(name, value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if value == "" || len(value) > maxFormatPatchHeaderBytes || !utf8.ValidString(value) {
		return fmt.Errorf("format-patch %s header is empty, invalid, or too long", name)
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return fmt.Errorf("format-patch %s header contains a control character", name)
		}
	}
	return nil
}

func replaceFormatPatchCoverLetter(paths []string, body string) error {
	const placeholder = "*** BLURB HERE ***"
	var cover string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read generated cover letter: %w", err)
		}
		if bytes.Count(data, []byte(placeholder)) == 1 {
			if cover != "" {
				return errors.New("format-patch generated multiple cover letters")
			}
			cover = path
		}
	}
	if cover == "" {
		return errors.New("format-patch did not generate an editable cover letter")
	}
	info, err := os.Lstat(cover)
	if err != nil {
		return fmt.Errorf("inspect generated cover letter: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("generated cover letter is not a regular file")
	}
	data, err := os.ReadFile(cover)
	if err != nil {
		return fmt.Errorf("read generated cover letter: %w", err)
	}
	updated := bytes.Replace(data, []byte(placeholder), []byte(body), 1)
	// Do not reopen the final pathname for writing: a concurrent replacement
	// with a symlink must never redirect a cover-letter edit outside its output
	// directory. Rename replaces that final entry rather than following it.
	temporary, err := os.CreateTemp(filepath.Dir(cover), ".lazymagit-cover-")
	if err != nil {
		return fmt.Errorf("create generated cover letter replacement: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(info.Mode().Perm()); err == nil {
		_, err = temporary.Write(updated)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write generated cover letter replacement: %w", err)
	}
	if err := os.Rename(temporaryName, cover); err != nil {
		return fmt.Errorf("install generated cover letter replacement: %w", err)
	}
	return nil
}

func (r *Repository) FormatPatchFromStash(ctx context.Context, ref string, options FormatPatchOptions) ([]string, error) {
	oid, err := r.resolveCommit(ctx, defaultStashRef(ref))
	if err != nil {
		return nil, err
	}
	// A stash is a merge commit and format-patch intentionally omits merges.
	// Create an unreachable, single-parent commit with the stash's final tree so
	// format-patch can faithfully emit its tracked index/worktree content. The
	// stash's optional untracked parent is not part of that tree.
	tree, err := r.output(ctx, "rev-parse", "--verify", oid+"^{tree}")
	if err != nil {
		return nil, err
	}
	parent, err := r.output(ctx, "rev-parse", "--verify", oid+"^1")
	if err != nil {
		return nil, err
	}
	treeOID, parentOID := trimLine(tree), trimLine(parent)
	subject, err := r.output(ctx, "show", "-s", "--format=%s", oid)
	if err != nil {
		return nil, err
	}
	message := trimLine(subject) + "\n"
	synthetic, err := r.runMutationOutput(ctx, []byte(message), "commit-tree", treeOID, "-p", parentOID)
	if err != nil {
		return nil, err
	}
	return r.FormatPatch(ctx, parentOID+".."+strings.TrimSpace(synthetic), options)
}

func safeRevisionArgument(revision string) error {
	if strings.TrimSpace(revision) == "" {
		return errors.New("revision range is empty")
	}
	if len(revision) > maxFormatPatchRevision || strings.HasPrefix(revision, "-") || !utf8.ValidString(revision) || strings.ContainsRune(revision, 0) || strings.ContainsAny(revision, "\r\n") {
		return fmt.Errorf("unsafe revision range %q", revision)
	}
	for _, r := range revision {
		if r < 32 || r == 127 {
			return fmt.Errorf("unsafe revision range %q", revision)
		}
	}
	return nil
}

func (r *Repository) patchOutputDirectory(name string) (string, map[string]bool, error) {
	if name == "" {
		return "", nil, errors.New("format-patch output directory is empty")
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
		return "", nil, errors.New("format-patch output path is not a directory or is a symlink")
	}
	entries, err := boundedDirectoryNames(dir, maxFormatPatchDirectoryEntries)
	if err != nil {
		return "", nil, fmt.Errorf("read format-patch output directory: %w", err)
	}
	before := make(map[string]bool, len(entries))
	for _, name := range entries {
		before[name] = true
	}
	return dir, before, nil
}

func boundedDirectoryNames(dir string, limit int) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > limit {
		return nil, &TooLargeError{Resource: "format-patch output directory"}
	}
	return names, nil
}

type DiffPatchOptions struct {
	Cached    bool
	Range     string
	Paths     []string
	Overwrite bool
}

// SaveDiffPatch streams Git's complete binary-capable diff to filename. Only
// the process record is bounded; the patch file is never made from display or
// truncated capture output.
func (r *Repository) SaveDiffPatch(ctx context.Context, filename string, options DiffPatchOptions) error {
	if options.Range != "" {
		if err := safeRevisionArgument(options.Range); err != nil {
			return err
		}
	}
	path, err := r.diffPatchOutputPath(filename)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lazymagit-patch-*")
	if err != nil {
		return fmt.Errorf("create patch output: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := r.runMutationToWriter(ctx, tmp, diffPatchArgs(options)...); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync patch output: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close patch output: %w", err)
	}
	if err := installDiffPatch(tmpName, path, filename, options.Overwrite); err != nil {
		return err
	}
	keep = true
	return nil
}

func (r *Repository) diffPatchOutputPath(filename string) (string, error) {
	if filename == "" {
		return "", errors.New("patch output filename is empty")
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
		return "", errors.New("inspect patch output directory: parent is not a directory")
	}
	return path, nil
}

func diffPatchArgs(options DiffPatchOptions) []string {
	args := []string{"diff", "--binary", "--full-index", "--no-color", "--no-ext-diff", "--no-textconv"}
	if options.Cached {
		args = append(args, "--cached")
	}
	if options.Range != "" {
		args = append(args, options.Range)
	}
	return pathsArgs(args, options.Paths)
}

func installDiffPatch(tmpName, path, filename string, overwrite bool) error {
	if overwrite {
		if err := os.Rename(tmpName, path); err != nil {
			return fmt.Errorf("install patch output: %w", err)
		}
		return nil
	}
	if err := linkOrCopyDiffPatch(tmpName, path, filename); err != nil {
		return err
	}
	if err := os.Remove(tmpName); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("finalize patch output: %w", err)
	}
	return nil
}

func linkOrCopyDiffPatch(tmpName, path, filename string) error {
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("patch output %q already exists", filename)
		}
		fallbackErr := copyFileExclusive(tmpName, path)
		if errors.Is(fallbackErr, os.ErrExist) {
			return fmt.Errorf("patch output %q already exists", filename)
		}
		if fallbackErr != nil {
			return fmt.Errorf("install patch output without replacement: link: %v; exclusive fallback: %w", err, fallbackErr)
		}
	}
	return nil
}

func copyFileExclusive(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		_ = out.Close()
		if !keep {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	keep = true
	return nil
}

// runMutationToWriter mirrors run's recording and bounded diagnostics while
// teeing complete stdout to a caller-owned destination.
func (r *Repository) runMutationToWriter(ctx context.Context, destination io.Writer, args ...string) error {
	started := time.Now()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.commandDir}, args...)...)
	cmd.Env = gitCommandEnv()
	stdout, stderr := new(headTailCapture), new(headTailCapture)
	cmd.Stdout = io.MultiWriter(destination, stdout)
	cmd.Stderr = stderr
	err := cmd.Run()
	duration := time.Since(started)
	recordedArgs, sensitive := redactMutationArgs(args)
	recordedStdout, stdoutRedactionTruncated := redactCaptured(stdout.String(), sensitive)
	recordedStderr, stderrRedactionTruncated := redactCaptured(stderr.String(), sensitive)
	stdoutTruncated := stdout.truncated || stdoutRedactionTruncated
	stderrTruncated := stderr.truncated || stderrRedactionTruncated
	if recorder, ok := ctx.Value(processRecorderKey{}).(func(ProcessRecord)); ok {
		recorder(ProcessRecord{Dir: r.commandDir, Args: append([]string(nil), recordedArgs...), Started: started,
			Duration: duration, ExitCode: processExitCode(ctx, err), Stdout: recordedStdout, Stderr: recordedStderr,
			StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated})
	}
	if err != nil {
		return &CommandError{Args: append([]string(nil), recordedArgs...), Err: err, Stderr: strings.TrimSpace(recordedStderr), StderrTruncated: stderrTruncated}
	}
	return nil
}

func (r *Repository) runMutationOutput(ctx context.Context, input []byte, args ...string) (string, error) {
	started := time.Now()
	stdout, stderr, err := r.executeMutation(ctx, input, args...)
	duration := time.Since(started)
	recordedArgs, sensitive := redactMutationArgs(args)
	recordedStdout, stdoutRedactionTruncated := redactCaptured(stdout.String(), sensitive)
	recordedStderr, stderrRedactionTruncated := redactCaptured(stderr.String(), sensitive)
	stdoutTruncated := stdout.truncated || stdoutRedactionTruncated
	stderrTruncated := stderr.truncated || stderrRedactionTruncated
	if recorder, ok := ctx.Value(processRecorderKey{}).(func(ProcessRecord)); ok {
		recorder(ProcessRecord{Dir: r.commandDir, Args: append([]string(nil), recordedArgs...), Started: started,
			Duration: duration, ExitCode: processExitCode(ctx, err), Stdout: recordedStdout, Stderr: recordedStderr,
			StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated})
	}
	if err != nil {
		return "", &CommandError{Args: append([]string(nil), recordedArgs...), Err: err, Stderr: strings.TrimSpace(recordedStderr), StderrTruncated: stderrTruncated}
	}
	if stdoutTruncated {
		return "", errors.New("git mutation output was unexpectedly truncated")
	}
	return recordedStdout, nil
}
