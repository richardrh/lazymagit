package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type RefKind uint8

const (
	RefLocal RefKind = iota
	RefRemote
	RefTag
)

type Ref struct {
	Kind                 RefKind
	Name, FullName       string
	ID, PeeledID         string
	Subject              string
	Upstream, PushTarget string
	Symref               string
	Current              bool
}

type RefQuery struct {
	// Focus accepts a revision and defaults to the current HEAD. Ahead/Behind
	// compare it with its upstream, when Focus names a local branch.
	Focus       string
	MergedTo    string
	NoMergedTo  string
	Limit       int
	OutputLimit int
}

type RefResult struct {
	Local, Remote, Tags []Ref
	Focus               *Ref
	Ahead, Behind       int
	Truncated           bool
}

func (r *Repository) QueryRefs(ctx context.Context, q RefQuery) (RefResult, error) {
	if q.Limit < 0 {
		return RefResult{}, errors.New("ref limit cannot be negative")
	}
	limit := q.Limit
	if limit == 0 {
		limit = 1000
	}
	truncated := false
	if limit > inspectionItemLimit {
		limit, truncated = inspectionItemLimit, true
	}
	byteLimit := q.OutputLimit
	if byteLimit == 0 {
		byteLimit = inspectionOutputLimit
	}
	if byteLimit < 0 || byteLimit > inspectionOutputLimit {
		return RefResult{}, fmt.Errorf("ref output limit must be between 0 and %d", inspectionOutputLimit)
	}
	format := "%00%(HEAD)%00%(refname)%00%(refname:short)%00%(objectname)%00%(*objectname)%00%(upstream:short)%00%(push:short)%00%(symref)%00%(subject)%00"
	args := []string{"for-each-ref", "--count=" + strconv.Itoa(limit+1), "--sort=refname", "--format=" + format}
	if q.MergedTo != "" {
		oid, resolveErr := r.resolveCommitOID(ctx, q.MergedTo)
		if resolveErr != nil {
			return RefResult{}, resolveErr
		}
		args = append(args, "--merged="+oid)
	}
	if q.NoMergedTo != "" {
		oid, resolveErr := r.resolveCommitOID(ctx, q.NoMergedTo)
		if resolveErr != nil {
			return RefResult{}, resolveErr
		}
		args = append(args, "--no-merged="+oid)
	}
	args = append(args, "refs/heads", "refs/remotes", "refs/tags")
	out, byteTruncated, err := r.outputLimited(ctx, byteLimit, args...)
	if err != nil {
		return RefResult{}, err
	}
	refs, incomplete, err := parseInspectionRefs(out)
	if err != nil {
		return RefResult{}, err
	}
	truncated = truncated || byteTruncated || incomplete
	if len(refs) > limit {
		refs, truncated = refs[:limit], true
	}
	result := RefResult{Truncated: truncated}
	for _, ref := range refs {
		switch ref.Kind {
		case RefLocal:
			result.Local = append(result.Local, ref)
		case RefRemote:
			result.Remote = append(result.Remote, ref)
		case RefTag:
			result.Tags = append(result.Tags, ref)
		}
	}
	focusOID := ""
	if q.Focus != "" {
		focusOID, err = r.resolveCommitOID(ctx, q.Focus)
		if err != nil {
			return RefResult{}, err
		}
	} else {
		focusOID, err = r.resolveCommitOID(ctx, "HEAD")
		if err != nil {
			if isExitError(err) {
				return result, nil
			}
			return RefResult{}, err
		}
	}
	for i := range refs {
		exactName := q.Focus != "" && (refs[i].Name == q.Focus || refs[i].FullName == q.Focus)
		matchingOID := refs[i].ID == focusOID || refs[i].PeeledID == focusOID
		if exactName || matchingOID {
			candidate := refs[i]
			// An explicitly named ref wins; otherwise prefer the current branch
			// when several names point at HEAD.
			if result.Focus == nil || exactName || q.Focus == "" && candidate.Current {
				result.Focus = &candidate
			}
			if exactName {
				break
			}
		}
	}
	if result.Focus != nil && result.Focus.Kind == RefLocal && result.Focus.Upstream != "" {
		upstreamOID, resolveErr := r.resolveCommitOID(ctx, result.Focus.Upstream)
		if resolveErr != nil {
			return RefResult{}, resolveErr
		}
		counts, countErr := r.output(ctx, "rev-list", "--left-right", "--count", focusOID+"..."+upstreamOID)
		if countErr != nil {
			return RefResult{}, countErr
		}
		if _, scanErr := fmt.Sscanf(trimLine(counts), "%d\t%d", &result.Ahead, &result.Behind); scanErr != nil {
			return RefResult{}, errors.New("malformed ahead/behind counts")
		}
	}
	return result, nil
}

func parseInspectionRefs(out []byte) ([]Ref, bool, error) {
	var refs []Ref
	for len(out) > 0 {
		start := bytes.IndexByte(out, 0)
		if start < 0 {
			return refs, len(bytes.TrimSpace(out)) != 0, nil
		}
		out = out[start+1:]
		fields := make([][]byte, 9)
		complete := true
		for i := range fields {
			end := bytes.IndexByte(out, 0)
			if end < 0 {
				complete = false
				break
			}
			fields[i], out = out[:end], out[end+1:]
		}
		if !complete {
			return refs, true, nil
		}
		full := string(fields[1])
		ref := Ref{FullName: full, Name: string(fields[2]), ID: string(fields[3]), PeeledID: string(fields[4]), Upstream: string(fields[5]), PushTarget: string(fields[6]), Symref: string(fields[7]), Subject: string(fields[8]), Current: string(fields[0]) == "*"}
		switch {
		case strings.HasPrefix(full, "refs/heads/"):
			ref.Kind = RefLocal
		case strings.HasPrefix(full, "refs/remotes/"):
			ref.Kind = RefRemote
			// Remote HEAD aliases duplicate their target and are navigation aids,
			// not independent refs in a generic inspection list.
			if ref.Symref != "" {
				continue
			}
		case strings.HasPrefix(full, "refs/tags/"):
			ref.Kind = RefTag
		default:
			return nil, false, fmt.Errorf("unexpected ref %q", full)
		}
		refs = append(refs, ref)
	}
	return refs, false, nil
}

type CherryQuery struct {
	Upstream, Head string
	Limit          int
	OutputLimit    int
}

type CherryPatch struct {
	Equivalent  bool
	ID, Subject string
}

type CherryResult struct {
	Items     []CherryPatch
	Truncated bool
}

func (r *Repository) QueryCherry(ctx context.Context, q CherryQuery) (CherryResult, error) {
	if q.Limit < 0 {
		return CherryResult{}, errors.New("cherry limit cannot be negative")
	}
	limit := q.Limit
	if limit == 0 {
		limit = 1000
	}
	truncated := false
	if limit > inspectionItemLimit {
		limit, truncated = inspectionItemLimit, true
	}
	upstream, err := r.resolveCommitOID(ctx, q.Upstream)
	if err != nil {
		return CherryResult{}, fmt.Errorf("resolve cherry upstream: %w", err)
	}
	head := q.Head
	if head == "" {
		head = "HEAD"
	}
	headOID, err := r.resolveCommitOID(ctx, head)
	if err != nil {
		return CherryResult{}, fmt.Errorf("resolve cherry head: %w", err)
	}
	byteLimit := q.OutputLimit
	if byteLimit == 0 {
		byteLimit = inspectionOutputLimit
	}
	if byteLimit < 0 || byteLimit > inspectionOutputLimit {
		return CherryResult{}, fmt.Errorf("cherry output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, byteTruncated, err := r.outputLimited(ctx, byteLimit, "cherry", "-v", upstream, headOID)
	if err != nil {
		return CherryResult{}, err
	}
	result := CherryResult{Truncated: truncated || byteTruncated}
	lines := bytes.Split(out, []byte{'\n'})
	if byteTruncated && len(lines) != 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte{' '}, 3)
		if len(parts) < 2 || len(parts[0]) != 1 || (parts[0][0] != '+' && parts[0][0] != '-') || !isHexOID(string(parts[1])) {
			return CherryResult{}, fmt.Errorf("malformed git cherry record %q", line)
		}
		subject := ""
		if len(parts) == 3 {
			subject = string(parts[2])
		}
		result.Items = append(result.Items, CherryPatch{Equivalent: parts[0][0] == '-', ID: string(parts[1]), Subject: subject})
	}
	if len(result.Items) > limit {
		result.Items, result.Truncated = result.Items[:limit], true
	}
	return result, nil
}

type Revision struct {
	ID, ShortID                      string
	ParentIDs                        []string
	Subject, AuthorName, AuthorEmail string
	AuthorDate, CommitDate           time.Time
}

func (r *Repository) ResolveRevision(ctx context.Context, value string) (Revision, error) {
	oid, err := r.resolveCommitOID(ctx, value)
	if err != nil {
		return Revision{}, err
	}
	out, err := r.output(ctx, "show", "--no-patch", "--no-decorate", "--format="+inspectionLogFormat, oid)
	if err != nil {
		return Revision{}, err
	}
	entries, incomplete, err := parseInspectionLog(out)
	if err != nil {
		return Revision{}, err
	}
	if incomplete || len(entries) != 1 {
		return Revision{}, errors.New("malformed revision metadata")
	}
	e := entries[0]
	return Revision{ID: e.ID, ShortID: e.ShortID, ParentIDs: e.ParentIDs, Subject: e.Subject, AuthorName: e.AuthorName, AuthorEmail: e.AuthorEmail, AuthorDate: e.AuthorDate, CommitDate: e.CommitDate}, nil
}

// RevisionParent resolves a one-based parent index, matching Git's ^1 syntax
// without ever concatenating caller input into a revision expression.
func (r *Repository) RevisionParent(ctx context.Context, value string, parent int) (Revision, error) {
	if parent <= 0 {
		return Revision{}, errors.New("parent index must be positive")
	}
	revision, err := r.ResolveRevision(ctx, value)
	if err != nil {
		return Revision{}, err
	}
	if parent > len(revision.ParentIDs) {
		return Revision{}, fmt.Errorf("revision has no parent %d", parent)
	}
	return r.ResolveRevision(ctx, revision.ParentIDs[parent-1])
}

type ShowRevisionQuery struct {
	Revision    string
	Stat, Patch bool
	FirstParent bool
	OutputLimit int
}

type ShowRevisionResult struct {
	Revision  Revision
	Detail    string
	Truncated bool
}

func (r *Repository) QueryShowRevision(ctx context.Context, q ShowRevisionQuery) (ShowRevisionResult, error) {
	revision, err := r.ResolveRevision(ctx, q.Revision)
	if err != nil {
		return ShowRevisionResult{}, err
	}
	args := []string{"--no-pager", "show", "--no-color", "--no-ext-diff", "--no-textconv", "--format=fuller", "--parents"}
	if q.Stat {
		args = append(args, "--stat")
	}
	if q.Patch {
		args = append(args, "--patch")
	} else {
		args = append(args, "--no-patch")
	}
	if q.FirstParent {
		args = append(args, "--diff-merges=first-parent")
	}
	args = append(args, revision.ID)
	limit := q.OutputLimit
	if limit == 0 {
		limit = inspectionOutputLimit
	}
	if limit < 0 || limit > inspectionOutputLimit {
		return ShowRevisionResult{}, fmt.Errorf("show output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, truncated, err := r.outputLimited(ctx, limit, args...)
	if err != nil {
		return ShowRevisionResult{}, err
	}
	return ShowRevisionResult{Revision: revision, Detail: string(out), Truncated: truncated}, nil
}
