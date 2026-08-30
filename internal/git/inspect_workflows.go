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
	// DiffConflicts renders Git's combined diff for unresolved paths. It is a
	// read-only terminal alternative to launching an external mergetool.
	DiffConflicts
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
	args, limit, err := r.compileDiffQuery(ctx, q)
	if err != nil {
		return DiffResult{}, err
	}
	out, truncated, err := r.outputLimited(ctx, limit, args...)
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Detail: string(out), Truncated: truncated}, nil
}

func (r *Repository) compileDiffQuery(ctx context.Context, q DiffQuery) ([]string, int, error) {
	if err := validateDiffQuery(q); err != nil {
		return nil, 0, err
	}
	args := []string{"--no-pager", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/"}
	options, err := diffQueryOptions(q)
	if err != nil {
		return nil, 0, err
	}
	args = append(args, options...)
	revisions, err := r.diffQueryRevisions(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	args = append(args, revisions...)
	args = append(args, "--")
	args = append(args, q.Files...)
	limit, err := inspectionByteLimit("diff", q.OutputLimit)
	return args, limit, err
}

func validateDiffQuery(q DiffQuery) error {
	if q.Context < 0 || q.Context > 10000 {
		return errors.New("diff context must be between 0 and 10000")
	}
	if q.Stat && (q.NameOnly || q.NameStatus) || q.NameOnly && q.NameStatus {
		return errors.New("diff stat, name-only, and name-status modes are mutually exclusive")
	}
	return nil
}

func diffQueryOptions(q DiffQuery) ([]string, error) {
	var args []string
	if q.ContextSet || q.Context != 0 {
		args = append(args, "--unified="+strconv.Itoa(q.Context))
	}
	algorithms := map[DiffAlgorithm]string{DiffAlgorithmDefault: "", DiffAlgorithmMinimal: "--minimal", DiffAlgorithmPatience: "--patience", DiffAlgorithmHistogram: "--histogram"}
	algorithm, ok := algorithms[q.Algorithm]
	if !ok {
		return nil, errors.New("unknown diff algorithm")
	}
	if algorithm != "" {
		args = append(args, algorithm)
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
	return args, nil
}

func (r *Repository) diffQueryRevisions(ctx context.Context, q DiffQuery) ([]string, error) {
	resolve := func(value, label string) (string, error) {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("diff %s revision is empty", label)
		}
		return r.resolveCommitOID(ctx, value)
	}
	switch q.Kind {
	case DiffWorktree:
		if q.Base != "" || q.Target != "" {
			return nil, errors.New("worktree diff does not accept revisions")
		}
		return nil, nil
	case DiffIndex:
		if q.Base != "" || q.Target != "" {
			return nil, errors.New("index diff does not accept revisions")
		}
		return []string{"--cached"}, nil
	case DiffRevision:
		base, err := resolve(q.Base, "base")
		if err != nil {
			return nil, err
		}
		return []string{base}, nil
	case DiffRevisionRange:
		base, err := resolve(q.Base, "base")
		if err != nil {
			return nil, err
		}
		target, err := resolve(q.Target, "target")
		if err != nil {
			return nil, err
		}
		separator := ".."
		if q.TripleDot {
			separator = "..."
		}
		return []string{base + separator + target}, nil
	case DiffConflicts:
		if q.Base != "" || q.Target != "" || q.TripleDot {
			return nil, errors.New("conflict diff does not accept revisions")
		}
		return []string{"--cc"}, nil
	default:
		return nil, errors.New("unknown diff kind")
	}
}

func inspectionByteLimit(resource string, requested int) (int, error) {
	limit := requested
	if limit == 0 {
		limit = inspectionOutputLimit
	}
	if limit < 0 || limit > inspectionOutputLimit {
		return 0, fmt.Errorf("%s output limit must be between 0 and %d", resource, inspectionOutputLimit)
	}
	return limit, nil
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
type ReflogQuery struct {
	Revision    string
	All         bool
	Limit       int
	OutputLimit int
}

type ReflogEntry struct {
	ID, ShortID string
	Selector    string
	Subject     string
	AuthorName  string
}

type ReflogResult struct {
	Items     []ReflogEntry
	Truncated bool
}

type ShortlogQuery struct {
	Revision          string
	Range             string
	Since             bool
	Summary, Numbered bool
	Email             bool
	Group, Format     string
	WrapWidth         int
	WrapIndent1       int
	WrapIndent2       int
	WrapIndent1Set    bool
	WrapIndent2Set    bool
	OutputLimit       int
}

type ShortlogResult struct {
	Detail    string
	Truncated bool
}

const inspectionReflogFormat = "%x1e%H%x00%h%x00%gd%x00%gs%x00%aN%x00"

func (r *Repository) QueryReflog(ctx context.Context, q ReflogQuery) (ReflogResult, error) {
	if q.Limit < 0 {
		return ReflogResult{}, errors.New("reflog limit cannot be negative")
	}
	limit := q.Limit
	if limit == 0 {
		limit = 256
	}
	truncatedByLimit := false
	if limit > inspectionItemLimit {
		limit, truncatedByLimit = inspectionItemLimit, true
	}
	if q.All && q.Revision != "" {
		return ReflogResult{}, errors.New("reflog selectors are ambiguous")
	}
	revision := q.Revision
	if revision == "" && !q.All {
		revision = "HEAD"
	}
	if strings.HasPrefix(revision, "-") || strings.ContainsAny(revision, "\x00\r\n") {
		return ReflogResult{}, errors.New("invalid reflog ref")
	}
	if !q.All {
		if revision != "HEAD" {
			resolved, err := r.output(ctx, "rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", revision)
			if err != nil {
				return ReflogResult{}, err
			}
			revision = trimLine(resolved)
			if revision == "" {
				return ReflogResult{}, errors.New("reflog ref does not resolve to a named ref")
			}
		}
		if _, err := r.output(ctx, "reflog", "exists", revision); err != nil {
			return ReflogResult{}, err
		}
	}
	args := []string{"--no-pager", "reflog", "show", "--no-color", "--date=raw", "--format=" + inspectionReflogFormat, "-n", strconv.Itoa(limit + 1)}
	if q.All {
		args = append(args, "--all")
	} else {
		args = append(args, revision)
	}
	args = append(args, "--")
	byteLimit := q.OutputLimit
	if byteLimit == 0 {
		byteLimit = inspectionOutputLimit
	}
	if byteLimit < 0 || byteLimit > inspectionOutputLimit {
		return ReflogResult{}, fmt.Errorf("reflog output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, byteTruncated, err := r.outputLimited(ctx, byteLimit, args...)
	if err != nil {
		return ReflogResult{}, err
	}
	items, incomplete := parseInspectionReflog(out)
	truncated := truncatedByLimit || byteTruncated || incomplete
	if len(items) > limit {
		items, truncated = items[:limit], true
	}
	return ReflogResult{Items: items, Truncated: truncated}, nil
}

func parseInspectionReflog(out []byte) ([]ReflogEntry, bool) {
	var items []ReflogEntry
	for cursor := 0; ; {
		startRel := bytes.IndexByte(out[cursor:], 0x1e)
		if startRel < 0 {
			break
		}
		position := cursor + startRel + 1
		fields := make([][]byte, 5)
		for i := range fields {
			endRel := bytes.IndexByte(out[position:], 0)
			if endRel < 0 {
				return items, true
			}
			end := position + endRel
			fields[i], position = out[position:end], end+1
		}
		items = append(items, ReflogEntry{ID: string(fields[0]), ShortID: string(fields[1]), Selector: string(fields[2]), Subject: string(fields[3]), AuthorName: string(fields[4])})
		cursor = position
	}
	return items, false
}

func (r *Repository) QueryShortlog(ctx context.Context, q ShortlogQuery) (ShortlogResult, error) {
	args, byteLimit, err := r.compileShortlogQuery(ctx, q)
	if err != nil {
		return ShortlogResult{}, err
	}
	out, truncated, err := r.outputLimited(ctx, byteLimit, args...)
	if err != nil {
		return ShortlogResult{}, err
	}
	return ShortlogResult{Detail: string(out), Truncated: truncated}, nil
}

func (r *Repository) compileShortlogQuery(ctx context.Context, q ShortlogQuery) ([]string, int, error) {
	if q.Revision != "" && q.Range != "" || q.Since && q.Range != "" {
		return nil, 0, errors.New("shortlog revision selectors are ambiguous")
	}
	args := []string{"--no-pager", "shortlog"}
	options, err := shortlogQueryOptions(q)
	if err != nil {
		return nil, 0, err
	}
	args = append(args, options...)
	selector, err := r.shortlogQuerySelector(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	args = append(args, selector)
	byteLimit, err := inspectionByteLimit("shortlog", q.OutputLimit)
	return args, byteLimit, err
}

func shortlogQueryOptions(q ShortlogQuery) ([]string, error) {
	var args []string
	for _, option := range []struct {
		enabled bool
		value   string
	}{{q.Numbered, "--numbered"}, {q.Summary, "--summary"}, {q.Email, "--email"}} {
		if option.enabled {
			args = append(args, option.value)
		}
	}
	if q.Group != "" {
		if strings.ContainsAny(q.Group, "\x00\r\n") || strings.HasPrefix(q.Group, "-") {
			return nil, errors.New("invalid shortlog group")
		}
		if q.Group != "author" && q.Group != "committer" && !strings.HasPrefix(q.Group, "trailer:") {
			return nil, errors.New("shortlog group must be author, committer, or trailer:<field>")
		}
		args = append(args, "--group="+q.Group)
	}
	if q.Format != "" {
		if strings.ContainsAny(q.Format, "\x00\r\n") {
			return nil, errors.New("invalid shortlog format")
		}
		args = append(args, "--format="+q.Format)
	}
	wrap, err := shortlogWrapOption(q)
	if err != nil {
		return nil, err
	}
	if wrap != "" {
		args = append(args, wrap)
	}
	return args, nil
}

func shortlogWrapOption(q ShortlogQuery) (string, error) {
	if q.WrapWidth < 0 || q.WrapIndent1 < 0 || q.WrapIndent2 < 0 {
		return "", errors.New("shortlog wrap values cannot be negative")
	}
	if q.WrapWidth > 10000 || q.WrapIndent1 > 10000 || q.WrapIndent2 > 10000 {
		return "", errors.New("shortlog wrap values cannot exceed 10000")
	}
	if q.WrapIndent2Set && !q.WrapIndent1Set {
		return "", errors.New("shortlog second indent requires the first indent")
	}
	if q.WrapWidth > 0 {
		wrap := "-w" + strconv.Itoa(q.WrapWidth)
		if q.WrapIndent1Set {
			wrap += "," + strconv.Itoa(q.WrapIndent1)
			if q.WrapIndent2Set {
				wrap += "," + strconv.Itoa(q.WrapIndent2)
			}
		}
		return wrap, nil
	}
	return "", nil
}

func (r *Repository) shortlogQuerySelector(ctx context.Context, q ShortlogQuery) (string, error) {
	selector := q.Revision
	if q.Range != "" {
		resolved, err := r.resolveRevisionRange(ctx, q.Range)
		if err != nil {
			return "", err
		}
		return resolved, nil
	}
	if selector == "" {
		selector = "HEAD"
	}
	oid, err := r.resolveCommitOID(ctx, selector)
	if err != nil {
		return "", err
	}
	if q.Since {
		oid += "..HEAD"
	}
	return oid, nil
}

type RequestPullQuery struct {
	Start       string
	URL         string
	End         string
	OutputLimit int
}

type RequestPullResult struct {
	Detail    string
	Truncated bool
}

func (r *Repository) QueryRequestPull(ctx context.Context, q RequestPullQuery) (RequestPullResult, error) {
	if strings.TrimSpace(q.URL) == "" {
		return RequestPullResult{}, errors.New("request-pull URL is empty")
	}
	if len(q.URL) > 16<<10 || strings.HasPrefix(q.URL, "-") || strings.ContainsAny(q.URL, "\x00\r\n") {
		return RequestPullResult{}, errors.New("invalid request-pull URL")
	}
	start, err := r.resolveCommitOID(ctx, q.Start)
	if err != nil {
		return RequestPullResult{}, fmt.Errorf("resolve request-pull start: %w", err)
	}
	endRevision := q.End
	if endRevision == "" {
		endRevision = "HEAD"
	}
	end, err := r.resolveCommitOID(ctx, endRevision)
	if err != nil {
		return RequestPullResult{}, fmt.Errorf("resolve request-pull end: %w", err)
	}
	limit := q.OutputLimit
	if limit == 0 {
		limit = inspectionOutputLimit
	}
	if limit < 0 || limit > inspectionOutputLimit {
		return RequestPullResult{}, fmt.Errorf("request-pull output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, truncated, err := r.outputLimited(ctx, limit, "--no-pager", "request-pull", start, q.URL, end)
	if err != nil {
		return RequestPullResult{}, err
	}
	return RequestPullResult{Detail: string(out), Truncated: truncated}, nil
}

func (r *Repository) resolveRevisionRange(ctx context.Context, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("invalid revision or range")
	}
	separator := ""
	position := strings.Index(value, "...")
	if position >= 0 {
		separator = "..."
	} else if position = strings.Index(value, ".."); position >= 0 {
		separator = ".."
	}
	if separator == "" {
		return r.resolveCommitOID(ctx, value)
	}
	left, right := value[:position], value[position+len(separator):]
	if strings.Contains(left, "..") || strings.Contains(right, "..") {
		return "", errors.New("revision range has multiple separators")
	}
	if left == "" {
		left = "HEAD"
	}
	if right == "" {
		right = "HEAD"
	}
	leftOID, err := r.resolveCommitOID(ctx, left)
	if err != nil {
		return "", err
	}
	rightOID, err := r.resolveCommitOID(ctx, right)
	if err != nil {
		return "", err
	}
	return leftOID + separator + rightOID, nil
}

type LogQuery struct {
	Revision             string
	From, To             string
	Symmetric            bool
	Revisions            []string
	Files                []string
	BranchPattern        string
	TagPattern           string
	Reflog               bool
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
	args, limit, truncatedByLimit, err := r.compileLogQuery(ctx, q)
	if err != nil {
		return LogResult{}, err
	}
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
		if q.Revision == "" && q.From == "" && len(q.Revisions) == 0 && q.BranchPattern == "" && q.TagPattern == "" && !q.Reflog && isExitError(err) {
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

func (r *Repository) compileLogQuery(ctx context.Context, q LogQuery) ([]string, int, bool, error) {
	if err := validateLogQuery(q); err != nil {
		return nil, 0, false, err
	}
	limit, truncatedByLimit := logItemLimit(q.Limit)
	args := []string{"--no-pager", "log", "--no-color", "--no-ext-diff", "--no-textconv", "-n", strconv.Itoa(limit + 1), "--format=" + inspectionLogFormat}
	options, err := logQueryOptions(q)
	if err != nil {
		return nil, 0, false, err
	}
	args = append(args, options...)
	selectors, err := r.logQuerySelectors(ctx, q)
	if err != nil {
		return nil, 0, false, err
	}
	args = append(args, selectors...)
	args = append(args, "--")
	args = append(args, q.Files...)
	return args, limit, truncatedByLimit, nil
}

func validateLogQuery(q LogQuery) error {
	if q.Limit < 0 {
		return errors.New("log limit cannot be negative")
	}
	if q.MergesOnly && q.NoMerges {
		return errors.New("merges-only and no-merges are mutually exclusive")
	}
	if q.BranchPattern != "" && q.TagPattern != "" {
		return errors.New("branch and tag patterns are mutually exclusive")
	}
	for _, pattern := range []string{q.BranchPattern, q.TagPattern} {
		if strings.ContainsAny(pattern, "\x00\r\n") || strings.HasPrefix(pattern, "-") {
			return errors.New("invalid log ref pattern")
		}
	}
	return nil
}

func logItemLimit(requested int) (int, bool) {
	if requested == 0 {
		return 256, false
	}
	if requested > inspectionItemLimit {
		return inspectionItemLimit, true
	}
	return requested, false
}

func logQueryOptions(q LogQuery) ([]string, error) {
	var args []string
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
		{q.All, "--all"}, {q.Reflog, "--reflog"}, {q.FirstParent, "--first-parent"}, {q.MergesOnly, "--merges"},
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
		return nil, errors.New("unknown log order")
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
	if q.BranchPattern != "" {
		args = append(args, "HEAD", "--branches="+q.BranchPattern)
	}
	if q.TagPattern != "" {
		args = append(args, "HEAD", "--tags="+q.TagPattern)
	}
	return args, nil
}

func (r *Repository) logQuerySelectors(ctx context.Context, q LogQuery) ([]string, error) {
	if invalidLogSelectors(q) {
		return nil, errors.New("log revision selectors are ambiguous or incomplete")
	}
	if q.Revision != "" {
		oid, err := r.resolveCommitOID(ctx, q.Revision)
		return singleSelector(oid, err)
	}
	if q.From != "" {
		return r.resolveLogRange(ctx, q)
	}
	if q.BranchPattern == "" && q.TagPattern == "" {
		return r.resolveLogRevisions(ctx, q.Revisions)
	}
	return nil, nil
}

func invalidLogSelectors(q LogQuery) bool {
	return q.Revision != "" && (q.From != "" || q.To != "" || len(q.Revisions) != 0 || q.BranchPattern != "" || q.TagPattern != "") || (q.From != "") != (q.To != "")
}

func singleSelector(oid string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	return []string{oid}, nil
}

func (r *Repository) resolveLogRange(ctx context.Context, q LogQuery) ([]string, error) {
	from, err := r.resolveCommitOID(ctx, q.From)
	if err != nil {
		return nil, err
	}
	to, err := r.resolveCommitOID(ctx, q.To)
	if err != nil {
		return nil, err
	}
	separator := ".."
	if q.Symmetric {
		separator = "..."
	}
	return []string{from + separator + to}, nil
}

func (r *Repository) resolveLogRevisions(ctx context.Context, revisions []string) ([]string, error) {
	var args []string
	for _, revision := range revisions {
		oid, err := r.resolveCommitOID(ctx, revision)
		if err != nil {
			return nil, err
		}
		args = append(args, oid)
	}
	return args, nil
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
