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
	parser := blamePorcelainParser{}
	for _, raw := range strings.Split(string(out), "\n") {
		line, incomplete, err := parser.consume(raw)
		if err != nil {
			return nil, false, err
		}
		if incomplete {
			return lines, true, nil
		}
		if line != nil {
			lines = append(lines, *line)
		}
	}
	return lines, parser.haveHeader, nil
}

type blamePorcelainParser struct {
	current    BlameLine
	haveHeader bool
}

func (p *blamePorcelainParser) consume(raw string) (*BlameLine, bool, error) {
	if raw == "" {
		return nil, false, nil
	}
	if raw[0] == '\t' {
		return p.consumeContent(raw)
	}
	fields := strings.Fields(raw)
	if len(fields) >= 3 && isBlameOID(fields[0]) {
		incomplete, err := p.consumeHeader(fields)
		return nil, incomplete, err
	}
	if !p.haveHeader {
		return nil, false, errors.New("blame porcelain has an invalid header")
	}
	return nil, false, p.consumeMetadata(raw)
}

func (p *blamePorcelainParser) consumeContent(raw string) (*BlameLine, bool, error) {
	if !p.haveHeader {
		return nil, false, errors.New("blame porcelain content has no header")
	}
	p.current.Content = raw[1:]
	line := p.current
	p.haveHeader = false
	return &line, false, nil
}

func (p *blamePorcelainParser) consumeHeader(fields []string) (bool, error) {
	if p.haveHeader {
		return true, nil
	}
	line, err := strconv.Atoi(fields[2])
	if err != nil || line < 1 {
		return false, errors.New("blame porcelain has an invalid final line number")
	}
	p.current = BlameLine{Line: line, CommitID: strings.TrimPrefix(fields[0], "^")}
	p.haveHeader = true
	return false, nil
}

func (p *blamePorcelainParser) consumeMetadata(raw string) error {
	key, value, found := strings.Cut(raw, " ")
	if !found {
		return nil // Unknown future boolean porcelain field.
	}
	switch key {
	case "author":
		p.current.Author = value
	case "author-mail":
		p.current.AuthorMail = strings.Trim(value, "<>")
	case "author-time":
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.New("blame porcelain has an invalid author timestamp")
		}
		p.current.AuthorTime = time.Unix(seconds, 0).UTC()
	case "summary":
		p.current.Summary = value
	}
	return nil
}

func isBlameOID(value string) bool {
	return isHexOID(strings.TrimPrefix(value, "^"))
}
