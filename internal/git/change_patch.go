package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Change diffs are deliberately loaded independently of the bounded strings
// used by the UI.  A truncated document must never become mutation input.
const changeDiffLimit = 64 << 20

var (
	ErrChangeDiffTooLarge       = errors.New("change diff exceeds mutation limit")
	ErrMalformedChangePatch     = errors.New("malformed change patch")
	ErrUnsupportedChangePatch   = errors.New("change patch contains an unsupported file change")
	ErrInvalidChangePatchRegion = errors.New("invalid change patch region")
)

type DiffRange struct {
	Start int
	Count int
}

type DiffLineKind byte

const (
	DiffLineContext DiffLineKind = ' '
	DiffLineAdded   DiffLineKind = '+'
	DiffLineDeleted DiffLineKind = '-'
)

// DiffLine contains the text after the unified-diff prefix. A zero line
// number means that the line does not exist on that side of the change.
type DiffLine struct {
	Kind      DiffLineKind
	Text      string
	OldLine   int
	NewLine   int
	NoNewline bool
}

type DiffHunk struct {
	OldRange DiffRange
	NewRange DiffRange
	Heading  string
	Lines    []DiffLine
}

type DiffFile struct {
	OldPath string
	NewPath string

	OldMode string
	NewMode string
	NewFile bool
	Deleted bool
	Binary  bool

	Rename     bool
	RenameFrom string
	RenameTo   string

	Hunks []DiffHunk

	// header is retained verbatim so quoted, Unicode, and option-like paths
	// can be reconstructed without lossy path formatting.
	header []string
}

type DiffDocument struct {
	Files []DiffFile
}

var hunkHeaderRE = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(.*)$`)

// ParseUnifiedDiff parses the output of git diff --patch. It validates every
// hunk's declared old/new line counts, which also detects truncated patches.
func ParseUnifiedDiff(patch []byte) (*DiffDocument, error) {
	lines, err := completePatchLines(patch)
	if err != nil {
		return nil, err
	}
	doc := new(DiffDocument)
	for i := 0; i < len(lines); {
		if lines[i] == "" {
			i++
			continue
		}
		if !strings.HasPrefix(lines[i], "diff --git ") {
			return nil, malformed(i+1, "expected diff --git header")
		}
		f := DiffFile{header: []string{lines[i]}}
		i++
		for i < len(lines) && !strings.HasPrefix(lines[i], "diff --git ") {
			line := lines[i]
			if strings.HasPrefix(line, "@@ ") {
				h, next, parseErr := parseDiffHunk(lines, i)
				if parseErr != nil {
					return nil, parseErr
				}
				f.Hunks = append(f.Hunks, h)
				i = next
				continue
			}
			if len(f.Hunks) != 0 {
				return nil, malformed(i+1, "unexpected data after hunk")
			}
			f.header = append(f.header, line)
			parseFileMetadata(&f, line)
			i++
		}
		if err := validateDiffFile(&f); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformedChangePatch, err)
		}
		doc.Files = append(doc.Files, f)
	}
	return doc, nil
}

func completePatchLines(patch []byte) ([]string, error) {
	if len(patch) == 0 {
		return nil, nil
	}
	if patch[len(patch)-1] != '\n' {
		return nil, malformed(0, "patch ends in a partial line")
	}
	s := string(patch[:len(patch)-1])
	if s == "" {
		return nil, nil
	}
	// Do not trim carriage returns: in a CRLF file the CR is part of the hunk
	// line payload and is required for a byte-exact reconstructed patch.
	return strings.Split(s, "\n"), nil
}

func parseDiffHunk(lines []string, at int) (DiffHunk, int, error) {
	m := hunkHeaderRE.FindStringSubmatch(lines[at])
	if m == nil {
		return DiffHunk{}, at, malformed(at+1, "invalid hunk header")
	}
	oldStart, err := strconv.Atoi(m[1])
	if err != nil {
		return DiffHunk{}, at, malformed(at+1, "invalid old start")
	}
	newStart, err := strconv.Atoi(m[3])
	if err != nil {
		return DiffHunk{}, at, malformed(at+1, "invalid new start")
	}
	oldCount, err := rangeCount(m[2])
	if err != nil {
		return DiffHunk{}, at, malformed(at+1, "invalid old range")
	}
	newCount, err := rangeCount(m[4])
	if err != nil {
		return DiffHunk{}, at, malformed(at+1, "invalid new range")
	}
	h := DiffHunk{OldRange: DiffRange{oldStart, oldCount}, NewRange: DiffRange{newStart, newCount}, Heading: m[5]}
	oldLine, newLine := oldStart, newStart
	oldSeen, newSeen := 0, 0
	i := at + 1
	for oldSeen < oldCount || newSeen < newCount {
		if i >= len(lines) {
			return DiffHunk{}, at, malformed(at+1, "truncated hunk")
		}
		line := lines[i]
		if line == "" {
			return DiffHunk{}, at, malformed(i+1, "unprefixed hunk line")
		}
		dl := DiffLine{Kind: DiffLineKind(line[0]), Text: line[1:]}
		switch dl.Kind {
		case DiffLineContext:
			dl.OldLine, dl.NewLine = oldLine, newLine
			oldLine, newLine, oldSeen, newSeen = oldLine+1, newLine+1, oldSeen+1, newSeen+1
		case DiffLineDeleted:
			dl.OldLine = oldLine
			oldLine, oldSeen = oldLine+1, oldSeen+1
		case DiffLineAdded:
			dl.NewLine = newLine
			newLine, newSeen = newLine+1, newSeen+1
		default:
			return DiffHunk{}, at, malformed(i+1, "invalid hunk line prefix")
		}
		if oldSeen > oldCount || newSeen > newCount {
			return DiffHunk{}, at, malformed(i+1, "hunk exceeds declared range")
		}
		h.Lines = append(h.Lines, dl)
		i++
		if i < len(lines) && lines[i] == `\ No newline at end of file` {
			h.Lines[len(h.Lines)-1].NoNewline = true
			i++
		}
	}
	return h, i, nil
}

func rangeCount(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, errors.New("bad count")
	}
	return n, nil
}

func parseFileMetadata(f *DiffFile, line string) {
	switch {
	case strings.HasPrefix(line, "--- "):
		f.OldPath = parsePatchPath(strings.TrimPrefix(line, "--- "), "a/")
	case strings.HasPrefix(line, "+++ "):
		f.NewPath = parsePatchPath(strings.TrimPrefix(line, "+++ "), "b/")
	case strings.HasPrefix(line, "old mode "):
		f.OldMode = strings.TrimPrefix(line, "old mode ")
	case strings.HasPrefix(line, "new mode "):
		f.NewMode = strings.TrimPrefix(line, "new mode ")
	case strings.HasPrefix(line, "new file mode "):
		f.NewFile, f.NewMode = true, strings.TrimPrefix(line, "new file mode ")
	case strings.HasPrefix(line, "deleted file mode "):
		f.Deleted, f.OldMode = true, strings.TrimPrefix(line, "deleted file mode ")
	case strings.HasPrefix(line, "rename from "):
		f.Rename, f.RenameFrom = true, parsePatchPath(strings.TrimPrefix(line, "rename from "), "")
	case strings.HasPrefix(line, "rename to "):
		f.Rename, f.RenameTo = true, parsePatchPath(strings.TrimPrefix(line, "rename to "), "")
	case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
		f.Binary = true
	}
}

func parsePatchPath(value, prefix string) string {
	if value == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
	}
	return strings.TrimPrefix(value, prefix)
}

func validateDiffFile(f *DiffFile) error {
	if len(f.header) == 0 {
		return errors.New("missing file header")
	}
	if len(f.Hunks) > 0 && !hasPrefixLine(f.header, "--- ") {
		return errors.New("missing old file marker")
	}
	if len(f.Hunks) > 0 && !hasPrefixLine(f.header, "+++ ") {
		return errors.New("missing new file marker")
	}
	return nil
}

func hasPrefixLine(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func malformed(line int, why string) error {
	if line > 0 {
		return fmt.Errorf("%w: line %d: %s", ErrMalformedChangePatch, line, why)
	}
	return fmt.Errorf("%w: %s", ErrMalformedChangePatch, why)
}

type InteractiveChangeAction uint8

const (
	InteractiveChangeStage InteractiveChangeAction = iota
	InteractiveChangeUnstage
	InteractiveChangeDiscardUnstaged
	InteractiveChangeDiscardStaged
)

type InteractiveChangeScope uint8

const (
	InteractiveChangeFile InteractiveChangeScope = iota
	InteractiveChangeHunk
	InteractiveChangeLines
)

type InteractiveChangeRequest struct {
	Action     InteractiveChangeAction
	Scope      InteractiveChangeScope
	Path       string
	Hunk       int
	Start, End int
}

type ReviewedInteractiveChange struct {
	Request      InteractiveChangeRequest
	HunkHeading  string
	ChangedLines int
	PatchHash    string
	token        ConfirmationToken
}

// FilePatch reconstructs every hunk for one mutable text file.
func (d *DiffDocument) FilePatch(fileIndex int) ([]byte, error) {
	if d == nil || fileIndex < 0 || fileIndex >= len(d.Files) {
		return nil, ErrInvalidChangePatchRegion
	}
	f := &d.Files[fileIndex]
	if err := mutableFile(f); err != nil {
		return nil, err
	}
	var b strings.Builder
	writeFileHeader(&b, f)
	for _, hunk := range f.Hunks {
		writeHunk(&b, hunk)
	}
	return []byte(b.String()), nil
}

// HunkPatch reconstructs one complete hunk and its file header.
func (d *DiffDocument) HunkPatch(fileIndex, hunkIndex int) ([]byte, error) {
	if d == nil || fileIndex < 0 || fileIndex >= len(d.Files) || hunkIndex < 0 || hunkIndex >= len(d.Files[fileIndex].Hunks) {
		return nil, ErrInvalidChangePatchRegion
	}
	f := &d.Files[fileIndex]
	if err := mutableFile(f); err != nil {
		return nil, err
	}
	return renderPatch(f, f.Hunks[hunkIndex]), nil
}

// ChangedLineRegionPatch selects changed lines whose zero-based hunk indexes
// are in [start,end). Context within that interval is allowed. Unselected
// deletions become context and unselected additions disappear, matching the
// partial-application semantics used by change-oriented Git clients.
func (d *DiffDocument) ChangedLineRegionPatch(fileIndex, hunkIndex, start, end int) ([]byte, error) {
	if d == nil || fileIndex < 0 || fileIndex >= len(d.Files) || hunkIndex < 0 || hunkIndex >= len(d.Files[fileIndex].Hunks) {
		return nil, ErrInvalidChangePatchRegion
	}
	f := &d.Files[fileIndex]
	if err := mutableFile(f); err != nil {
		return nil, err
	}
	source := f.Hunks[hunkIndex]
	if start < 0 || end > len(source.Lines) || start >= end {
		return nil, ErrInvalidChangePatchRegion
	}
	selected := false
	lines := make([]DiffLine, 0, len(source.Lines))
	for i, line := range source.Lines {
		inside := i >= start && i < end
		if inside && (line.Kind == DiffLineAdded || line.Kind == DiffLineDeleted) {
			selected = true
		}
		switch {
		case line.Kind == DiffLineAdded && !inside:
			continue
		case line.Kind == DiffLineDeleted && !inside:
			line.Kind = DiffLineContext
			line.NewLine = line.OldLine
		}
		lines = append(lines, line)
	}
	if !selected {
		return nil, ErrInvalidChangePatchRegion
	}
	h := source
	h.Lines = lines
	h.OldRange.Count, h.NewRange.Count = countHunkSides(lines)
	return renderPatch(f, h), nil
}

func mutableFile(f *DiffFile) error {
	if f.Binary || f.Rename {
		return ErrUnsupportedChangePatch
	}
	if len(f.Hunks) == 0 {
		return ErrUnsupportedChangePatch
	}
	return nil
}

func countHunkSides(lines []DiffLine) (old, new int) {
	for _, line := range lines {
		if line.Kind != DiffLineAdded {
			old++
		}
		if line.Kind != DiffLineDeleted {
			new++
		}
	}
	return
}

func renderPatch(f *DiffFile, h DiffHunk) []byte {
	var b strings.Builder
	writeFileHeader(&b, f)
	writeHunk(&b, h)
	return []byte(b.String())
}

func writeFileHeader(b *strings.Builder, f *DiffFile) {
	for _, line := range f.header {
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func writeHunk(b *strings.Builder, h DiffHunk) {
	fmt.Fprintf(b, "@@ -%d,%d +%d,%d @@%s\n", h.OldRange.Start, h.OldRange.Count, h.NewRange.Start, h.NewRange.Count, h.Heading)
	for _, line := range h.Lines {
		b.WriteByte(byte(line.Kind))
		b.WriteString(line.Text)
		b.WriteByte('\n')
		if line.NoNewline {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
}

func (r *Repository) LoadUnstagedDiffDocument(ctx context.Context, path string) (*DiffDocument, error) {
	return r.loadChangeDiffDocument(ctx, path, false)
}

func (r *Repository) LoadStagedDiffDocument(ctx context.Context, path string) (*DiffDocument, error) {
	return r.loadChangeDiffDocument(ctx, path, true)
}

func (r *Repository) loadChangeDiffDocument(ctx context.Context, path string, staged bool) (*DiffDocument, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--patch", "--full-index"}
	if staged {
		args = append(args, "--cached")
	}
	if path != "" {
		args = append(args, "--", path)
	}
	out, truncated, err := r.outputLimited(ctx, changeDiffLimit, args...)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, ErrChangeDiffTooLarge
	}
	return ParseUnifiedDiff(out)
}

func (r *Repository) selectedInteractivePatch(ctx context.Context, request InteractiveChangeRequest) ([]byte, string, int, error) {
	if strings.TrimSpace(request.Path) == "" {
		return nil, "", 0, errors.New("interactive change path is empty")
	}
	staged := request.Action == InteractiveChangeUnstage || request.Action == InteractiveChangeDiscardStaged
	if request.Action > InteractiveChangeDiscardStaged {
		return nil, "", 0, errors.New("unknown interactive change action")
	}
	doc, err := r.loadChangeDiffDocument(ctx, request.Path, staged)
	if err != nil {
		return nil, "", 0, err
	}
	if len(doc.Files) != 1 {
		return nil, "", 0, ErrInvalidChangePatchRegion
	}
	file := &doc.Files[0]
	var patch []byte
	heading := ""
	changed := 0
	switch request.Scope {
	case InteractiveChangeFile:
		patch, err = doc.FilePatch(0)
		for _, hunk := range file.Hunks {
			changed += changedLineCount(hunk.Lines)
		}
	case InteractiveChangeHunk:
		patch, err = doc.HunkPatch(0, request.Hunk)
		if request.Hunk >= 0 && request.Hunk < len(file.Hunks) {
			heading = file.Hunks[request.Hunk].Heading
			changed = changedLineCount(file.Hunks[request.Hunk].Lines)
		}
	case InteractiveChangeLines:
		patch, err = doc.ChangedLineRegionPatch(0, request.Hunk, request.Start, request.End)
		if request.Hunk >= 0 && request.Hunk < len(file.Hunks) {
			heading = file.Hunks[request.Hunk].Heading
			for i := request.Start; i < request.End && i < len(file.Hunks[request.Hunk].Lines); i++ {
				if i >= 0 && file.Hunks[request.Hunk].Lines[i].Kind != DiffLineContext {
					changed++
				}
			}
		}
	default:
		return nil, "", 0, errors.New("unknown interactive change scope")
	}
	if err != nil {
		return nil, "", 0, err
	}
	return patch, heading, changed, nil
}

func changedLineCount(lines []DiffLine) int {
	count := 0
	for _, line := range lines {
		if line.Kind != DiffLineContext {
			count++
		}
	}
	return count
}

func interactiveChangeIdentity(request InteractiveChangeRequest, patch []byte) string {
	sum := sha256.Sum256(patch)
	return fmt.Sprintf("%d\x00%d\x00%s\x00%d\x00%d\x00%d\x00%x", request.Action, request.Scope, request.Path, request.Hunk, request.Start, request.End, sum)
}

func (r *Repository) ReviewInteractiveChange(ctx context.Context, request InteractiveChangeRequest) (ReviewedInteractiveChange, error) {
	patch, heading, changed, err := r.selectedInteractivePatch(ctx, request)
	if err != nil {
		return ReviewedInteractiveChange{}, err
	}
	identity := interactiveChangeIdentity(request, patch)
	sum := sha256.Sum256(patch)
	return ReviewedInteractiveChange{Request: request, HunkHeading: heading, ChangedLines: changed, PatchHash: fmt.Sprintf("%x", sum), token: NewConfirmationToken(identity)}, nil
}

func (r *Repository) ExecuteReviewedInteractiveChange(ctx context.Context, reviewed ReviewedInteractiveChange) error {
	patch, _, _, err := r.selectedInteractivePatch(ctx, reviewed.Request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStalePlan, err)
	}
	identity := interactiveChangeIdentity(reviewed.Request, patch)
	if !reviewed.token.validFor(identity) {
		return ErrStalePlan
	}
	operation := ChangePatchStage
	switch reviewed.Request.Action {
	case InteractiveChangeStage:
		operation = ChangePatchStage
	case InteractiveChangeUnstage:
		operation = ChangePatchUnstage
	case InteractiveChangeDiscardUnstaged:
		operation = ChangePatchReverse
	case InteractiveChangeDiscardStaged:
		operation = ChangePatchDiscardStaged
	default:
		return errors.New("unknown interactive change action")
	}
	return r.ApplyChangePatch(ctx, patch, operation)
}

type ChangePatchOperation uint8

const (
	ChangePatchStage ChangePatchOperation = iota
	ChangePatchUnstage
	ChangePatchApply
	ChangePatchReverse
	ChangePatchDiscardStaged
)

// ApplyChangePatch validates patch structure before passing it on stdin. Patch
// paths are never command arguments, so names beginning with '-' stay literal.
func (r *Repository) ApplyChangePatch(ctx context.Context, patch []byte, operation ChangePatchOperation) error {
	if len(patch) > changeDiffLimit {
		return ErrChangeDiffTooLarge
	}
	doc, err := ParseUnifiedDiff(patch)
	if err != nil {
		return err
	}
	if len(doc.Files) == 0 {
		return ErrMalformedChangePatch
	}
	for i := range doc.Files {
		if err := mutableFile(&doc.Files[i]); err != nil {
			return err
		}
	}
	args := []string{"apply"}
	switch operation {
	case ChangePatchStage:
		args = append(args, "--cached")
	case ChangePatchUnstage:
		args = append(args, "--cached", "--reverse")
	case ChangePatchApply:
	case ChangePatchReverse:
		args = append(args, "--reverse")
	case ChangePatchDiscardStaged:
		args = append(args, "--index", "--reverse")
	default:
		return errors.New("unknown change patch operation")
	}
	args = append(args, "--whitespace=nowarn", "-")
	return r.runInput(ctx, patch, args...)
}

func (r *Repository) StageChangePatch(ctx context.Context, patch []byte) error {
	return r.ApplyChangePatch(ctx, patch, ChangePatchStage)
}

func (r *Repository) UnstageChangePatch(ctx context.Context, patch []byte) error {
	return r.ApplyChangePatch(ctx, patch, ChangePatchUnstage)
}

func (r *Repository) ApplyWorktreeChangePatch(ctx context.Context, patch []byte) error {
	return r.ApplyChangePatch(ctx, patch, ChangePatchApply)
}

func (r *Repository) ReverseWorktreeChangePatch(ctx context.Context, patch []byte) error {
	return r.ApplyChangePatch(ctx, patch, ChangePatchReverse)
}
