package git

// Terminal-native conflict resolution.  Git's index owns the three conflict
// versions, so this API reads blobs by object ID instead of constructing
// revision:path arguments from a filename.  That keeps unusual filenames
// literal and avoids invoking an editor, mergetool, or shell.

import (
	"bytes"
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

const conflictBlobLimit = 512 << 10

var ErrConflictResolutionUnsupported = errors.New("requested conflict resolution is not supported by Git")

type ConflictStage uint8

const (
	ConflictBase   ConflictStage = 1
	ConflictOurs   ConflictStage = 2
	ConflictTheirs ConflictStage = 3
)

type ConflictBlob struct {
	Stage     ConflictStage
	OID, Mode string
	Content   []byte
	Truncated bool
}

// ConflictPath identifies one currently unmerged index path.  Blobs are
// deliberately not populated by UnmergedPaths; callers that render a path use
// InspectConflict, which has a bounded output contract.
type ConflictPath struct {
	Path   string
	Stages []ConflictStage
}

// ConflictInspection contains the base, ours, and theirs blobs that actually
// exist for a path.  Delete/modify conflicts need not have all three stages.
type ConflictInspection struct {
	Path  string
	Blobs []ConflictBlob
}

type ConflictResolution uint8

const (
	ResolveOurs ConflictResolution = iota + 1
	ResolveTheirs
	ResolveBase
)

// ReviewedConflictResolution is opaque approval for one exact unmerged path
// and its worktree content.  It is intentionally a value so the UI can carry
// it through its normal reviewed-mutation channel.
type ReviewedConflictResolution struct {
	Path       string
	Resolution ConflictResolution
	identity   string
}

// UnmergedPaths lists paths directly from the index's unmerged entries.  It
// does not infer conflicts from presentation-oriented porcelain fields.
func (r *Repository) UnmergedPaths(ctx context.Context) ([]ConflictPath, error) {
	entries, err := r.conflictEntries(ctx)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]ConflictPath, 0, len(paths))
	for _, path := range paths {
		stages := make([]ConflictStage, 0, len(entries[path]))
		for _, stage := range []ConflictStage{ConflictBase, ConflictOurs, ConflictTheirs} {
			if _, ok := entries[path][stage]; ok {
				stages = append(stages, stage)
			}
		}
		out = append(out, ConflictPath{Path: path, Stages: stages})
	}
	return out, nil
}

// InspectConflict returns bounded raw blob content for terminal rendering.
// Blob object IDs came from ls-files, never from a caller-controlled revision
// expression.
func (r *Repository) InspectConflict(ctx context.Context, path string) (ConflictInspection, error) {
	entries, err := r.conflictEntries(ctx)
	if err != nil {
		return ConflictInspection{}, err
	}
	stages, ok := entries[path]
	if !ok {
		return ConflictInspection{}, fmt.Errorf("%q is not an unresolved path", path)
	}
	result := ConflictInspection{Path: path}
	for _, stage := range []ConflictStage{ConflictBase, ConflictOurs, ConflictTheirs} {
		entry, ok := stages[stage]
		if !ok {
			continue
		}
		content, truncated, err := r.outputLimited(ctx, conflictBlobLimit, "cat-file", "blob", entry.oid)
		if err != nil {
			return ConflictInspection{}, fmt.Errorf("read conflict stage %d: %w", stage, err)
		}
		result.Blobs = append(result.Blobs, ConflictBlob{Stage: stage, OID: entry.oid, Mode: entry.mode, Content: append([]byte(nil), content...), Truncated: truncated})
	}
	return result, nil
}

// ReviewConflictResolution validates that Git has a checkout-able stage and
// captures both index entries and the currently displayed worktree file.  Git
// supports --ours and --theirs here; a base-only checkout is intentionally not
// claimed because stock checkout has no such mode.
func (r *Repository) ReviewConflictResolution(ctx context.Context, path string, resolution ConflictResolution) (ReviewedConflictResolution, error) {
	if resolution == ResolveBase {
		return ReviewedConflictResolution{}, fmt.Errorf("base: %w", ErrConflictResolutionUnsupported)
	}
	stage, err := resolutionStage(resolution)
	if err != nil {
		return ReviewedConflictResolution{}, err
	}
	identity, entries, err := r.conflictIdentity(ctx, path)
	if err != nil {
		return ReviewedConflictResolution{}, err
	}
	if _, ok := entries[stage]; !ok {
		return ReviewedConflictResolution{}, fmt.Errorf("%s version is unavailable for %q", conflictStageName(stage), path)
	}
	return ReviewedConflictResolution{Path: path, Resolution: resolution, identity: identity}, nil
}

// ExecuteReviewedConflictResolution replaces the worktree path with Git's
// selected stage and stages that exact path.  The preflight is repeated before
// either mutation, so a changed conflict index or worktree is rejected.
func (r *Repository) ExecuteReviewedConflictResolution(ctx context.Context, reviewed ReviewedConflictResolution) error {
	if reviewed.Path == "" || reviewed.identity == "" {
		return ErrStalePlan
	}
	current, err := r.ReviewConflictResolution(ctx, reviewed.Path, reviewed.Resolution)
	if err != nil {
		if errors.Is(err, ErrConflictResolutionUnsupported) {
			return err
		}
		return ErrStalePlan
	}
	if current.identity != reviewed.identity {
		return ErrStalePlan
	}
	choice := "--ours"
	if reviewed.Resolution == ResolveTheirs {
		choice = "--theirs"
	}
	if err := r.run(ctx, "checkout", choice, "--", reviewed.Path); err != nil {
		return err
	}
	return r.run(ctx, "add", "--", reviewed.Path)
}

type conflictIndexEntry struct{ mode, oid string }

func (r *Repository) conflictEntries(ctx context.Context) (map[string]map[ConflictStage]conflictIndexEntry, error) {
	out, truncated, err := r.outputLimited(ctx, 16<<20, "ls-files", "--unmerged", "-z")
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, &TooLargeError{Resource: "unmerged index entries"}
	}
	return parseConflictEntries(out)
}

func parseConflictEntries(out []byte) (map[string]map[ConflictStage]conflictIndexEntry, error) {
	entries := make(map[string]map[ConflictStage]conflictIndexEntry)
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		path, stage, entry, err := parseConflictEntry(record)
		if err != nil {
			return nil, err
		}
		if entries[path] == nil {
			entries[path] = make(map[ConflictStage]conflictIndexEntry)
		}
		if _, duplicate := entries[path][stage]; duplicate {
			return nil, fmt.Errorf("duplicate unmerged index stage for %q", path)
		}
		entries[path][stage] = entry
	}
	return entries, nil
}

func parseConflictEntry(record []byte) (string, ConflictStage, conflictIndexEntry, error) {
	tab := bytes.IndexByte(record, '\t')
	if tab < 0 || tab == len(record)-1 {
		return "", 0, conflictIndexEntry{}, fmt.Errorf("malformed unmerged index entry %q", record)
	}
	fields := strings.Fields(string(record[:tab]))
	if len(fields) != 3 || !isHexOID(fields[1]) {
		return "", 0, conflictIndexEntry{}, fmt.Errorf("malformed unmerged index entry %q", record)
	}
	stageNumber, err := strconv.Atoi(fields[2])
	if err != nil || stageNumber < int(ConflictBase) || stageNumber > int(ConflictTheirs) {
		return "", 0, conflictIndexEntry{}, fmt.Errorf("malformed unmerged index stage %q", fields[2])
	}
	return string(record[tab+1:]), ConflictStage(stageNumber), conflictIndexEntry{mode: fields[0], oid: fields[1]}, nil
}

func resolutionStage(resolution ConflictResolution) (ConflictStage, error) {
	switch resolution {
	case ResolveOurs:
		return ConflictOurs, nil
	case ResolveTheirs:
		return ConflictTheirs, nil
	case ResolveBase:
		return 0, fmt.Errorf("base: %w", ErrConflictResolutionUnsupported)
	default:
		return 0, errors.New("unknown conflict resolution")
	}
}

func conflictStageName(stage ConflictStage) string {
	return map[ConflictStage]string{ConflictBase: "base", ConflictOurs: "ours", ConflictTheirs: "theirs"}[stage]
}

func (r *Repository) conflictIdentity(ctx context.Context, path string) (string, map[ConflictStage]conflictIndexEntry, error) {
	entries, err := r.conflictEntries(ctx)
	if err != nil {
		return "", nil, err
	}
	stages, ok := entries[path]
	if !ok {
		return "", nil, fmt.Errorf("%q is not an unresolved path", path)
	}
	worktree, err := r.conflictWorktreeIdentity(ctx, path)
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(path))
	for _, stage := range []ConflictStage{ConflictBase, ConflictOurs, ConflictTheirs} {
		if entry, exists := stages[stage]; exists {
			_, _ = fmt.Fprintf(h, "\x00%d\x00%s\x00%s", stage, entry.mode, entry.oid)
		}
	}
	_, _ = h.Write([]byte("\x00worktree\x00" + worktree))
	return hex.EncodeToString(h.Sum(nil)), stages, nil
}

func (r *Repository) conflictWorktreeIdentity(ctx context.Context, path string) (string, error) {
	full, err := r.worktreePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		return "absent", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect conflict worktree path: %w", err)
	}
	// Never let hash-object follow a user-replaced symlink outside the worktree.
	// Its link text, not its target, is the relevant worktree state here.
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(full)
		if err != nil {
			return "", fmt.Errorf("read conflict worktree symlink: %w", err)
		}
		sum := sha256.Sum256([]byte(target))
		return fmt.Sprintf("symlink:%o:%x", info.Mode().Perm(), sum), nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("%s:%o", conflictFileKind(info), info.Mode().Perm()), nil
	}
	// hash-object provides Git's byte-level view without trusting timestamps.
	hash, err := r.output(ctx, "hash-object", "--no-filters", "--", path)
	if err != nil {
		return "", fmt.Errorf("hash conflict worktree path: %w", err)
	}
	return fmt.Sprintf("file:%o:%s", info.Mode().Perm(), trimLine(hash)), nil
}

func conflictFileKind(info os.FileInfo) string {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return "symlink"
	case info.Mode().IsRegular():
		return "file"
	default:
		return "other"
	}
}

func (r *Repository) worktreePath(path string) (string, error) {
	if r.workTree == "" {
		return "", errors.New("conflict resolution requires a worktree")
	}
	if filepath.IsAbs(path) {
		return "", errors.New("invalid absolute conflict path")
	}
	full := filepath.Clean(filepath.Join(r.workTree, filepath.FromSlash(path)))
	rel, err := filepath.Rel(r.workTree, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid conflict path outside worktree")
	}
	return full, nil
}
