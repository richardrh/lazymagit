package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type OperationKind uint8

const (
	OperationMerge OperationKind = iota + 1
	OperationCherryPick
	OperationRevert
	OperationRebase
	OperationBisect
	OperationApplyMailbox
	OperationNotesMerge
)

type Operation struct {
	Kind           OperationKind
	Heads          []string
	Branch, Onto   string
	Current, Total int
	// Detail contains a stable administrative value such as BISECT_START or
	// NOTES_MERGE_REF, never an unbounded message or patch.
	Detail string
}

type OperationState struct{ Items []Operation }

func (s OperationState) InProgress() bool { return len(s.Items) != 0 }

// QueryOperationState only examines administrative paths documented or used
// as Git's stable operation sentinels. Message files and transient lock files
// are intentionally ignored.
func (r *Repository) QueryOperationState(ctx context.Context) (OperationState, error) {
	var state OperationState
	for _, head := range []struct {
		name string
		kind OperationKind
	}{{"MERGE_HEAD", OperationMerge}, {"CHERRY_PICK_HEAD", OperationCherryPick}, {"REVERT_HEAD", OperationRevert}} {
		if err := r.appendOperationHead(ctx, &state, head.name, head.kind); err != nil {
			return OperationState{}, err
		}
	}
	if err := r.appendSequencerOperation(ctx, &state); err != nil {
		return OperationState{}, err
	}
	if err := r.appendRebaseOperations(ctx, &state); err != nil {
		return OperationState{}, err
	}
	if err := r.appendDetailOperation(ctx, &state, "BISECT_START", OperationBisect); err != nil {
		return OperationState{}, err
	}
	if err := r.appendDetailOperation(ctx, &state, "NOTES_MERGE_REF", OperationNotesMerge); err != nil {
		return OperationState{}, err
	}
	return state, nil
}

func (r *Repository) appendOperationHead(ctx context.Context, state *OperationState, name string, kind OperationKind) error {
	text, ok, err := r.readGitAdmin(ctx, name, 64<<10)
	if err != nil || !ok {
		return err
	}
	heads, err := parseAdminOIDs(text)
	if err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	state.Items = append(state.Items, Operation{Kind: kind, Heads: heads})
	return nil
}

func (r *Repository) appendSequencerOperation(ctx context.Context, state *OperationState) error {
	todo, ok, err := r.readGitAdmin(ctx, "sequencer/todo", 64<<10)
	if err != nil || !ok || hasOperation(state.Items, OperationCherryPick) || hasOperation(state.Items, OperationRevert) {
		return err
	}
	kind, heads := parseSequencerTodo(todo)
	if kind != 0 {
		state.Items = append(state.Items, Operation{Kind: kind, Heads: heads})
	}
	return nil
}

func (r *Repository) appendRebaseOperations(ctx context.Context, state *OperationState) error {
	merge, err := r.rebaseMarker(ctx, "rebase-merge/interactive", "rebase-merge/head-name")
	if err != nil {
		return err
	}
	if merge {
		if err := r.appendRebaseOperation(ctx, state, "rebase-merge", false); err != nil {
			return err
		}
	}
	apply, err := r.rebaseMarker(ctx, "rebase-apply/applying", "rebase-apply/rebasing")
	if err != nil {
		return err
	}
	if apply {
		return r.appendRebaseOperation(ctx, state, "rebase-apply", operationMarkerExists(r, ctx, "rebase-apply/applying"))
	}
	return nil
}

func (r *Repository) rebaseMarker(ctx context.Context, first, second string) (bool, error) {
	_, a, err := r.readGitAdmin(ctx, first, 64<<10)
	if err != nil {
		return false, err
	}
	_, b, err := r.readGitAdmin(ctx, second, 64<<10)
	return a || b, err
}

func operationMarkerExists(r *Repository, ctx context.Context, name string) bool {
	_, ok, _ := r.readGitAdmin(ctx, name, 64<<10)
	return ok
}

func (r *Repository) appendRebaseOperation(ctx context.Context, state *OperationState, dir string, applying bool) error {
	op, err := r.rebaseState(ctx, dir, applying)
	if err == nil {
		state.Items = append(state.Items, op)
	}
	return err
}

func (r *Repository) appendDetailOperation(ctx context.Context, state *OperationState, name string, kind OperationKind) error {
	detail, ok, err := r.readGitAdmin(ctx, name, 64<<10)
	if err == nil && ok {
		state.Items = append(state.Items, Operation{Kind: kind, Detail: strings.TrimSpace(detail)})
	}
	return err
}

func hasOperation(items []Operation, kind OperationKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func parseAdminOIDs(text string) ([]string, error) {
	var ids []string
	for _, field := range strings.Fields(text) {
		if !isHexOID(field) {
			return nil, fmt.Errorf("invalid object ID %q", field)
		}
		ids = append(ids, field)
	}
	if len(ids) == 0 {
		return nil, errors.New("missing object ID")
	}
	return ids, nil
}

func parseSequencerTodo(text string) (OperationKind, []string) {
	var kind OperationKind
	var heads []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		lineKind := OperationCherryPick
		if fields[0] == "revert" {
			lineKind = OperationRevert
		} else if fields[0] != "pick" {
			continue
		}
		if kind == 0 {
			kind = lineKind
		}
		if kind != lineKind {
			heads = nil
		} // A mixed todo is still active but has no single useful set.
		if isHexOID(fields[1]) {
			heads = append(heads, fields[1])
		}
	}
	return kind, heads
}

func (r *Repository) rebaseState(ctx context.Context, directory string, am bool) (Operation, error) {
	kind := OperationRebase
	if am {
		kind = OperationApplyMailbox
	}
	op := Operation{Kind: kind}
	if err := r.readRebaseStrings(ctx, directory, &op); err != nil {
		return Operation{}, err
	}
	if err := r.readRebaseNumbers(ctx, directory, &op); err != nil {
		return Operation{}, err
	}
	return op, nil
}

func (r *Repository) readRebaseStrings(ctx context.Context, directory string, op *Operation) error {
	for name, target := range map[string]*string{"head-name": &op.Branch, "onto": &op.Onto} {
		value, ok, err := r.readGitAdmin(ctx, directory+"/"+name, 64<<10)
		if err != nil {
			return err
		}
		if ok {
			*target = strings.TrimSpace(value)
		}
	}
	return nil
}

func (r *Repository) readRebaseNumbers(ctx context.Context, directory string, op *Operation) error {
	for name, target := range map[string]*int{"msgnum": &op.Current, "next": &op.Current, "end": &op.Total, "last": &op.Total} {
		if value, ok, err := r.readGitAdmin(ctx, directory+"/"+name, 64); err != nil {
			return err
		} else if ok {
			n, parseErr := parseRebaseNumber(value)
			if parseErr != nil {
				return fmt.Errorf("parse %s/%s", directory, name)
			}
			*target = n
		}
	}
	return nil
}

func parseRebaseNumber(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0, errors.New("invalid rebase progress")
	}
	return n, nil
}

func (r *Repository) readGitAdmin(ctx context.Context, name string, limit int64) (string, bool, error) {
	out, err := r.output(ctx, "rev-parse", "--git-path", name)
	if err != nil {
		return "", false, err
	}
	path := trimLine(out)
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.commandDir, path)
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("open git administrative file %s: %w", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return "", false, fmt.Errorf("read git administrative file %s: %w", name, err)
	}
	if int64(len(b)) > limit {
		return "", false, fmt.Errorf("git administrative file %s exceeds %d bytes", name, limit)
	}
	return string(b), true, nil
}
