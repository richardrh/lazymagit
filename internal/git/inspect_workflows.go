package git

// This file contains read-only building blocks used by inspection UIs.  The
// query structs deliberately expose values, not command-line fragments: this
// keeps option parsing at this package boundary and makes every revision and
// path safe to pass to Git.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	inspectionOutputLimit = 8 << 20
	inspectionItemLimit   = 5000
)

type DiffKind uint8

const (
	DiffWorktree DiffKind = iota
	DiffIndex
	DiffRevision
	DiffRevisionRange
)

type DiffAlgorithm uint8

const (
	DiffAlgorithmDefault DiffAlgorithm = iota
	DiffAlgorithmMinimal
	DiffAlgorithmPatience
	DiffAlgorithmHistogram
)

// DiffQuery represents the useful, non-mutating subset of Magit's diff
// transient. Base is used by DiffRevision; Base and Target are used by
// DiffRevisionRange. TripleDot selects merge-base...Target range semantics.
type DiffQuery struct {
	Kind              DiffKind
	Base, Target      string
	Files             []string
	Context           int
	ContextSet        bool
	Algorithm         DiffAlgorithm
	Stat, NameOnly    bool
	NameStatus        bool
	Reverse           bool
	IgnoreAllSpace    bool
	IgnoreSpaceChange bool
	IgnoreBlankLines  bool
	WordDiff          bool
	DetectRenames     bool
	DetectCopies      bool
	FirstParent       bool
	Binary            bool
	OutputLimit       int
	TripleDot         bool
}

type DiffResult struct {
	Detail    string
	Truncated bool
}

func (r *Repository) QueryDiff(ctx context.Context, q DiffQuery) (DiffResult, error) {
	if q.Context < 0 || q.Context > 10000 {
		return DiffResult{}, errors.New("diff context must be between 0 and 10000")
	}
	if q.Stat && (q.NameOnly || q.NameStatus) || q.NameOnly && q.NameStatus {
		return DiffResult{}, errors.New("diff stat, name-only, and name-status modes are mutually exclusive")
	}
	args := []string{"--no-pager", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/"}
	if q.ContextSet || q.Context != 0 {
		args = append(args, "--unified="+strconv.Itoa(q.Context))
	}
	switch q.Algorithm {
	case DiffAlgorithmDefault:
	case DiffAlgorithmMinimal:
		args = append(args, "--minimal")
	case DiffAlgorithmPatience:
		args = append(args, "--patience")
	case DiffAlgorithmHistogram:
		args = append(args, "--histogram")
	default:
		return DiffResult{}, errors.New("unknown diff algorithm")
	}
	options := []struct {
		enabled bool
		option  string
	}{
		{q.Stat, "--stat"}, {q.NameOnly, "--name-only"}, {q.NameStatus, "--name-status"},
		{q.Reverse, "--reverse"}, {q.IgnoreAllSpace, "--ignore-all-space"},
		{q.IgnoreSpaceChange, "--ignore-space-change"}, {q.IgnoreBlankLines, "--ignore-blank-lines"},
		{q.WordDiff, "--word-diff=plain"}, {q.DetectRenames, "--find-renames"},
		{q.DetectCopies, "--find-copies"}, {q.FirstParent, "--first-parent"}, {q.Binary, "--binary"},
	}
	for _, candidate := range options {
		if candidate.enabled {
			args = append(args, candidate.option)
		}
	}
	resolve := func(value, label string) (string, error) {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("diff %s revision is empty", label)
		}
		return r.resolveCommitOID(ctx, value)
	}
	switch q.Kind {
	case DiffWorktree:
		if q.Base != "" || q.Target != "" {
			return DiffResult{}, errors.New("worktree diff does not accept revisions")
		}
	case DiffIndex:
		if q.Base != "" || q.Target != "" {
			return DiffResult{}, errors.New("index diff does not accept revisions")
		}
		args = append(args, "--cached")
	case DiffRevision:
		base, err := resolve(q.Base, "base")
		if err != nil {
			return DiffResult{}, err
		}
		args = append(args, base)
	case DiffRevisionRange:
		base, err := resolve(q.Base, "base")
		if err != nil {
			return DiffResult{}, err
		}
		target, err := resolve(q.Target, "target")
		if err != nil {
			return DiffResult{}, err
		}
		separator := ".."
		if q.TripleDot {
			separator = "..."
		}
		args = append(args, base+separator+target)
	default:
		return DiffResult{}, errors.New("unknown diff kind")
	}
	args = append(args, "--")
	args = append(args, q.Files...)
	limit := q.OutputLimit
	if limit == 0 {
		limit = inspectionOutputLimit
	}
	if limit < 0 || limit > inspectionOutputLimit {
		return DiffResult{}, fmt.Errorf("diff output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, truncated, err := r.outputLimited(ctx, limit, args...)
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Detail: string(out), Truncated: truncated}, nil
}

type LogOrder uint8

const (
	LogOrderDefault LogOrder = iota
	LogOrderDate
	LogOrderAuthorDate
	LogOrderTopo
)

// LogQuery supports a single revision, a two-dot or three-dot range, or a
// union of independently resolved Revisions. Empty revisions means HEAD.
type LogQuery struct {
	Revision             string
	From, To             string
	Symmetric            bool
	Revisions            []string
	Files                []string
	Limit                int
	Graph, Decorations   bool
	All, FirstParent     bool
	MergesOnly, NoMerges bool
	Reverse              bool
	Order                LogOrder
	Author, Grep         string
	Since, Until         *time.Time
	OutputLimit          int
}

type LogEntry struct {
	ID, ShortID string
	ParentIDs   []string
	Decorations string
	Subject     string
	AuthorName  string
	AuthorEmail string
	AuthorDate  time.Time
	CommitDate  time.Time
	Graph       string
}

type LogResult struct {
	Items     []LogEntry
	Truncated bool
}

const inspectionLogFormat = "%x1e%H%x00%h%x00%P%x00%D%x00%s%x00%an%x00%ae%x00%aI%x00%cI%x00"

func (r *Repository) QueryLog(ctx context.Context, q LogQuery) (LogResult, error) {
	if q.Limit < 0 {
		return LogResult{}, errors.New("log limit cannot be negative")
	}
	if q.MergesOnly && q.NoMerges {
		return LogResult{}, errors.New("merges-only and no-merges are mutually exclusive")
	}
	limit := q.Limit
	if limit == 0 {
		limit = 256
	}
	truncatedByLimit := false
	if limit > inspectionItemLimit {
		limit, truncatedByLimit = inspectionItemLimit, true
	}
	args := []string{"--no-pager", "log", "--no-color", "--no-ext-diff", "--no-textconv", "-n", strconv.Itoa(limit + 1), "--format=" + inspectionLogFormat}
	if q.Graph {
		args = append(args, "--graph")
	}
	if q.Decorations {
		args = append(args, "--decorate=short")
	} else {
		args = append(args, "--no-decorate")
	}
	for _, candidate := range []struct {
		enabled bool
		option  string
	}{
		{q.All, "--all"}, {q.FirstParent, "--first-parent"}, {q.MergesOnly, "--merges"},
		{q.NoMerges, "--no-merges"}, {q.Reverse, "--reverse"},
	} {
		if candidate.enabled {
			args = append(args, candidate.option)
		}
	}
	switch q.Order {
	case LogOrderDefault:
	case LogOrderDate:
		args = append(args, "--date-order")
	case LogOrderAuthorDate:
		args = append(args, "--author-date-order")
	case LogOrderTopo:
		args = append(args, "--topo-order")
	default:
		return LogResult{}, errors.New("unknown log order")
	}
	if q.Author != "" {
		args = append(args, "--author="+q.Author)
	}
	if q.Grep != "" {
		args = append(args, "--fixed-strings", "--grep="+q.Grep)
	}
	if q.Since != nil {
		args = append(args, "--since="+q.Since.UTC().Format(time.RFC3339Nano))
	}
	if q.Until != nil {
		args = append(args, "--until="+q.Until.UTC().Format(time.RFC3339Nano))
	}
	if q.Revision != "" && (q.From != "" || q.To != "" || len(q.Revisions) != 0) || (q.From != "") != (q.To != "") {
		return LogResult{}, errors.New("log revision selectors are ambiguous or incomplete")
	}
	if q.Revision != "" {
		oid, err := r.resolveCommitOID(ctx, q.Revision)
		if err != nil {
			return LogResult{}, err
		}
		args = append(args, oid)
	} else if q.From != "" {
		from, err := r.resolveCommitOID(ctx, q.From)
		if err != nil {
			return LogResult{}, err
		}
		to, err := r.resolveCommitOID(ctx, q.To)
		if err != nil {
			return LogResult{}, err
		}
		separator := ".."
		if q.Symmetric {
			separator = "..."
		}
		args = append(args, from+separator+to)
	} else {
		for _, revision := range q.Revisions {
			oid, err := r.resolveCommitOID(ctx, revision)
			if err != nil {
				return LogResult{}, err
			}
			args = append(args, oid)
		}
	}
	args = append(args, "--")
	args = append(args, q.Files...)
	byteLimit := q.OutputLimit
	if byteLimit == 0 {
		byteLimit = inspectionOutputLimit
	}
	if byteLimit < 0 || byteLimit > inspectionOutputLimit {
		return LogResult{}, fmt.Errorf("log output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, byteTruncated, err := r.outputLimited(ctx, byteLimit, args...)
	if err != nil {
		// An unborn repository has no default log, but an explicitly bad selector
		// has already failed resolution and must not be hidden here.
		if q.Revision == "" && q.From == "" && len(q.Revisions) == 0 && isExitError(err) {
			if _, verifyErr := r.output(ctx, "rev-parse", "--verify", "HEAD"); isExitError(verifyErr) {
				return LogResult{}, nil
			}
		}
		return LogResult{}, err
	}
	items, incomplete, err := parseInspectionLog(out)
	if err != nil {
		return LogResult{}, err
	}
	truncated := truncatedByLimit || byteTruncated || incomplete
	if len(items) > limit {
		items, truncated = items[:limit], true
	}
	return LogResult{Items: items, Truncated: truncated}, nil
}

func parseInspectionLog(out []byte) ([]LogEntry, bool, error) {
	var entries []LogEntry
	for cursor := 0; ; {
		startRel := bytes.IndexByte(out[cursor:], 0x1e)
		if startRel < 0 {
			break
		}
		start := cursor + startRel
		position := start + 1
		fields := make([][]byte, 9)
		for i := range fields {
			endRel := bytes.IndexByte(out[position:], 0)
			if endRel < 0 {
				return entries, true, nil
			}
			end := position + endRel
			fields[i], position = out[position:end], end+1
		}
		authorDate, err := time.Parse(time.RFC3339, string(fields[7]))
		if err != nil {
			return nil, false, fmt.Errorf("parse log author date: %w", err)
		}
		commitDate, err := time.Parse(time.RFC3339, string(fields[8]))
		if err != nil {
			return nil, false, fmt.Errorf("parse log commit date: %w", err)
		}
		graphBytes := out[cursor:start]
		if at := bytes.LastIndexByte(graphBytes, '\n'); at >= 0 {
			graphBytes = graphBytes[at+1:]
		}
		graphBytes = bytes.Trim(graphBytes, "\x00\r\n")
		entry := LogEntry{ID: string(fields[0]), ShortID: string(fields[1]), Decorations: string(fields[3]), Subject: string(fields[4]), AuthorName: string(fields[5]), AuthorEmail: string(fields[6]), AuthorDate: authorDate, CommitDate: commitDate, Graph: string(graphBytes)}
		if len(fields[2]) != 0 {
			entry.ParentIDs = strings.Fields(string(fields[2]))
		}
		entries = append(entries, entry)
		cursor = position
	}
	return entries, false, nil
}

func (r *Repository) resolveCommitOID(ctx context.Context, revision string) (string, error) {
	if strings.TrimSpace(revision) == "" {
		return "", errors.New("revision is empty")
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	oid := trimLine(out)
	if !isHexOID(oid) {
		return "", errors.New("git returned an invalid object ID")
	}
	return oid, nil
}

func isHexOID(value string) bool {
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
