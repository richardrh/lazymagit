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
	read := func(name string) (string, bool, error) { return r.readGitAdmin(ctx, name, 64<<10) }
	appendHead := func(name string, kind OperationKind) error {
		text, ok, err := read(name)
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
	if err := appendHead("MERGE_HEAD", OperationMerge); err != nil {
		return OperationState{}, err
	}
	if err := appendHead("CHERRY_PICK_HEAD", OperationCherryPick); err != nil {
		return OperationState{}, err
	}
	if err := appendHead("REVERT_HEAD", OperationRevert); err != nil {
		return OperationState{}, err
	}

	// A multi-commit cherry-pick/revert can be between commits and therefore
	// have no *_HEAD. sequencer/todo remains the stable sentinel.
	if todo, ok, err := read("sequencer/todo"); err != nil {
		return OperationState{}, err
	} else if ok && !hasOperation(state.Items, OperationCherryPick) && !hasOperation(state.Items, OperationRevert) {
		kind, heads := parseSequencerTodo(todo)
		if kind != 0 {
			state.Items = append(state.Items, Operation{Kind: kind, Heads: heads})
		}
	}

	if _, ok, err := read("rebase-merge/interactive"); err != nil {
		return OperationState{}, err
	} else {
		if _, exists, e := read("rebase-merge/head-name"); e != nil {
			return OperationState{}, e
		} else if exists || ok {
			op, e := r.rebaseState(ctx, "rebase-merge", false)
			if e != nil {
				return OperationState{}, e
			}
			state.Items = append(state.Items, op)
		}
	}
	if _, applying, err := read("rebase-apply/applying"); err != nil {
		return OperationState{}, err
	} else if _, rebasing, e := read("rebase-apply/rebasing"); e != nil {
		return OperationState{}, e
	} else if applying || rebasing {
		op, e := r.rebaseState(ctx, "rebase-apply", applying)
		if e != nil {
			return OperationState{}, e
		}
		state.Items = append(state.Items, op)
	}
	if start, ok, err := read("BISECT_START"); err != nil {
		return OperationState{}, err
	} else if ok {
		state.Items = append(state.Items, Operation{Kind: OperationBisect, Detail: strings.TrimSpace(start)})
	}
	if notesRef, ok, err := read("NOTES_MERGE_REF"); err != nil {
		return OperationState{}, err
	} else if ok {
		state.Items = append(state.Items, Operation{Kind: OperationNotesMerge, Detail: strings.TrimSpace(notesRef)})
	}
	return state, nil
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
	for name, target := range map[string]*string{"head-name": &op.Branch, "onto": &op.Onto} {
		if value, ok, err := r.readGitAdmin(ctx, directory+"/"+name, 64<<10); err != nil {
			return Operation{}, err
		} else if ok {
			*target = strings.TrimSpace(value)
		}
	}
	for name, target := range map[string]*int{"msgnum": &op.Current, "next": &op.Current, "end": &op.Total, "last": &op.Total} {
		if value, ok, err := r.readGitAdmin(ctx, directory+"/"+name, 64); err != nil {
			return Operation{}, err
		} else if ok {
			n, parseErr := strconv.Atoi(strings.TrimSpace(value))
			if parseErr != nil || n < 0 {
				return Operation{}, fmt.Errorf("parse %s/%s", directory, name)
			}
			*target = n
		}
	}
	return op, nil
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
