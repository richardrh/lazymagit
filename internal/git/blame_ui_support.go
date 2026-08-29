package git

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BlameQuery is a bounded, read-only request for line provenance in the
// current worktree. Path is always repository-relative and passed after --.
type BlameQuery struct {
	Path        string
	OutputLimit int
}

// BlameLine describes one displayed source line and the commit that last
// changed it. Content is deliberately kept as data; the UI sanitizes it before
// sending it to the terminal.
type BlameLine struct {
	Line       int
	CommitID   string
	Author     string
	AuthorMail string
	AuthorTime time.Time
	Summary    string
	Content    string
}

// BlameResult is incomplete only when Git output reaches the configured bound
// or ends partway through a porcelain record.
type BlameResult struct {
	Lines     []BlameLine
	Truncated bool
}

// QueryBlame uses Git's machine-readable porcelain format rather than its
// human display format. This keeps fields unambiguous, avoids color/config
// variation, and leaves terminal rendering entirely to the UI.
func (r *Repository) QueryBlame(ctx context.Context, q BlameQuery) (BlameResult, error) {
	if err := r.requireWorkTree(); err != nil {
		return BlameResult{}, err
	}
	path, err := validateRepoRelative(q.Path, false)
	if err != nil {
		return BlameResult{}, fmt.Errorf("blame path: %w", err)
	}
	limit := q.OutputLimit
	if limit == 0 {
		limit = inspectionOutputLimit
	}
	if limit < 0 || limit > inspectionOutputLimit {
		return BlameResult{}, fmt.Errorf("blame output limit must be between 0 and %d", inspectionOutputLimit)
	}
	out, truncated, err := r.outputLimited(ctx, limit, "--no-pager", "blame", "--line-porcelain", "--", path)
	if err != nil {
		return BlameResult{}, err
	}
	lines, incomplete, err := parseBlamePorcelain(out)
	if err != nil {
		return BlameResult{}, err
	}
	return BlameResult{Lines: lines, Truncated: truncated || incomplete}, nil
}

func parseBlamePorcelain(out []byte) ([]BlameLine, bool, error) {
	var lines []BlameLine
	var current BlameLine
	haveHeader := false
	for _, raw := range strings.Split(string(out), "\n") {
		if raw == "" {
			continue
		}
		if raw[0] == '\t' {
			if !haveHeader {
				return nil, false, errors.New("blame porcelain content has no header")
			}
			current.Content = raw[1:]
			lines = append(lines, current)
			haveHeader = false
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) >= 3 && isBlameOID(fields[0]) {
			if haveHeader {
				return lines, true, nil
			}
			line, err := strconv.Atoi(fields[2])
			if err != nil || line < 1 {
				return nil, false, errors.New("blame porcelain has an invalid final line number")
			}
			current = BlameLine{Line: line, CommitID: strings.TrimPrefix(fields[0], "^")}
			haveHeader = true
			continue
		}
		if !haveHeader {
			return nil, false, errors.New("blame porcelain has an invalid header")
		}
		key, value, found := strings.Cut(raw, " ")
		if !found {
			continue // Unknown future boolean porcelain field.
		}
		switch key {
		case "author":
			current.Author = value
		case "author-mail":
			current.AuthorMail = strings.Trim(value, "<>")
		case "author-time":
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, false, errors.New("blame porcelain has an invalid author timestamp")
			}
			current.AuthorTime = time.Unix(seconds, 0).UTC()
		case "summary":
			current.Summary = value
		}
	}
	return lines, haveHeader, nil
}

func isBlameOID(value string) bool {
	return isHexOID(strings.TrimPrefix(value, "^"))
}
