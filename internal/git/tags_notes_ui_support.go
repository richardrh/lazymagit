package git

// This file contains the small, review-oriented adapters needed by the TUI.
// The existing workflow methods remain the source of mutation semantics; these
// adapters only bind a displayed review to repository object identities.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const MaxNotesUIMessageBytes = 64 << 10

type ReviewedTagCreate struct {
	Args       CreateTagArgs
	PreviousID string
}

func (r *Repository) ReviewTagCreate(ctx context.Context, args CreateTagArgs) (ReviewedTagCreate, error) {
	p, err := r.TagCreatePreflight(ctx, args.Name)
	if err != nil {
		return ReviewedTagCreate{}, err
	}
	review := ReviewedTagCreate{Args: args}
	if p.Exists {
		tags, err := r.ListTags(ctx)
		if err != nil {
			return review, err
		}
		for _, tag := range tags {
			if tag.Name == args.Name {
				review.PreviousID = tag.ObjectID
				break
			}
		}
		if review.PreviousID == "" {
			return review, errors.New("tag changed while preparing review")
		}
	}
	return review, nil
}

func (r *Repository) CreateTagReviewed(ctx context.Context, review ReviewedTagCreate) error {
	now, err := r.ReviewTagCreate(ctx, review.Args)
	if err != nil {
		return err
	}
	if now.PreviousID != review.PreviousID {
		return errors.New("tag review is stale")
	}
	args := review.Args
	args.ConfirmReplace = args.Force && review.PreviousID != ""
	_, err = r.CreateTagWithArgs(ctx, args)
	return err
}

type ReviewedTagDelete struct {
	Names     []string
	ObjectIDs map[string]string
}

func (r *Repository) ReviewTagDelete(ctx context.Context, names []string) (ReviewedTagDelete, error) {
	tags, err := r.ListTags(ctx)
	if err != nil {
		return ReviewedTagDelete{}, err
	}
	available := make(map[string]string, len(tags))
	for _, tag := range tags {
		available[tag.Name] = tag.ObjectID
	}
	review := ReviewedTagDelete{ObjectIDs: map[string]string{}}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		oid, ok := available[name]
		if !ok {
			return review, fmt.Errorf("tag %q does not exist", name)
		}
		review.Names = append(review.Names, name)
		review.ObjectIDs[name] = oid
	}
	if len(review.Names) == 0 {
		return review, errors.New("no tags selected")
	}
	sort.Strings(review.Names)
	return review, nil
}

func (r *Repository) DeleteTagsReviewed(ctx context.Context, review ReviewedTagDelete) error {
	now, err := r.ReviewTagDelete(ctx, review.Names)
	if err != nil {
		return err
	}
	for _, name := range review.Names {
		if now.ObjectIDs[name] != review.ObjectIDs[name] {
			return fmt.Errorf("tag review is stale: %s", name)
		}
	}
	_, err = r.DeleteTags(ctx, DeleteTagsArgs{Names: append([]string(nil), review.Names...), Confirm: true})
	return err
}

type ReviewedRemoteTagPrune struct {
	Comparison RemoteTagComparison
	ObjectIDs  map[string]string
}

func (r *Repository) ReviewRemoteTagPrune(ctx context.Context, remote string) (ReviewedRemoteTagPrune, error) {
	comparison, err := r.CompareRemoteTags(ctx, remote)
	if err != nil {
		return ReviewedRemoteTagPrune{}, err
	}
	tags, err := r.ListTags(ctx)
	if err != nil {
		return ReviewedRemoteTagPrune{}, err
	}
	review := ReviewedRemoteTagPrune{Comparison: comparison, ObjectIDs: map[string]string{}}
	wanted := map[string]bool{}
	for _, name := range comparison.LocalOnly {
		wanted[name] = true
	}
	for _, tag := range tags {
		if wanted[tag.Name] {
			review.ObjectIDs[tag.Name] = tag.ObjectID
		}
	}
	return review, nil
}

func (r *Repository) PruneRemoteTagsReviewed(ctx context.Context, review ReviewedRemoteTagPrune) error {
	now, err := r.ReviewRemoteTagPrune(ctx, review.Comparison.Remote)
	if err != nil {
		return err
	}
	if strings.Join(now.Comparison.LocalOnly, "\x00") != strings.Join(review.Comparison.LocalOnly, "\x00") {
		return errors.New("remote tag prune review is stale")
	}
	for _, name := range review.Comparison.LocalOnly {
		if now.ObjectIDs[name] != review.ObjectIDs[name] {
			return fmt.Errorf("remote tag prune review is stale: %s", name)
		}
	}
	_, err = r.PruneRemoteTags(ctx, review.Comparison.Remote, true)
	return err
}

func (r *Repository) NotesMessage(ctx context.Context, ref, object string) (string, bool, error) {
	oid, err := r.resolveNotesObject(ctx, object)
	if err != nil {
		return "", false, err
	}
	out, truncated, err := r.outputLimited(ctx, MaxNotesUIMessageBytes+1, historyNotesArgs(ref, "show", "--", oid)...)
	if err != nil {
		if commandExitCode(err) == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	if truncated || len(out) > MaxNotesUIMessageBytes {
		return "", false, &TooLargeError{Resource: "note message"}
	}
	return string(out), true, nil
}

func (r *Repository) NotesWriteMessage(ctx context.Context, ref, object, message string, replace bool) error {
	if len(message) > MaxNotesUIMessageBytes {
		return &TooLargeError{Resource: "note message"}
	}
	if strings.ContainsRune(message, '\x00') {
		return errors.New("note message contains NUL")
	}
	oid, err := r.resolveNotesObject(ctx, object)
	if err != nil {
		return err
	}
	args := historyNotesArgs(ref, "add")
	if replace {
		args = append(args, "--force")
	}
	args = append(args, "--file=-", "--", oid)
	return r.runInput(ctx, []byte(message), args...)
}

func (r *Repository) resolveNotesObject(ctx context.Context, object string) (string, error) {
	if strings.TrimSpace(object) == "" || strings.ContainsRune(object, '\x00') {
		return "", errors.New("notes object is empty or invalid")
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", object+"^{object}")
	if err != nil {
		return "", err
	}
	return trimLine(out), nil
}

type ReviewedNotesRemoval struct {
	Ref       string
	Objects   []string
	ObjectIDs []string
	NotesOID  string
}

func (r *Repository) ReviewNotesRemoval(ctx context.Context, ref string, objects []string) (ReviewedNotesRemoval, error) {
	if len(objects) == 0 {
		return ReviewedNotesRemoval{}, errors.New("notes remove requires objects")
	}
	p := ReviewedNotesRemoval{Ref: ref, Objects: append([]string(nil), objects...)}
	for _, object := range objects {
		oid, err := r.resolveNotesObject(ctx, object)
		if err != nil {
			return p, err
		}
		p.ObjectIDs = append(p.ObjectIDs, oid)
	}
	fullRef := ref
	if fullRef == "" {
		fullRef = "refs/notes/commits"
	} else if !strings.HasPrefix(fullRef, "refs/notes/") {
		fullRef = "refs/notes/" + fullRef
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--quiet", fullRef)
	if err == nil {
		p.NotesOID = trimLine(out)
	} else if commandExitCode(err) != 1 {
		return p, err
	}
	return p, nil
}

func (r *Repository) RemoveNotesReviewed(ctx context.Context, review ReviewedNotesRemoval) error {
	now, err := r.ReviewNotesRemoval(ctx, review.Ref, review.Objects)
	if err != nil {
		return err
	}
	if now.NotesOID != review.NotesOID || strings.Join(now.ObjectIDs, "\x00") != strings.Join(review.ObjectIDs, "\x00") {
		return errors.New("notes removal review is stale")
	}
	return r.NotesRemove(ctx, NotesRemoveOptions{Ref: review.Ref, Objects: append([]string(nil), review.ObjectIDs...), ConfirmOptions: ConfirmOptions{Confirmed: true}})
}

type ReviewedNotesPrune struct {
	Ref      string
	NotesOID string
	Objects  []string
}

func (r *Repository) ReviewNotesPrune(ctx context.Context, ref string) (ReviewedNotesPrune, error) {
	p := ReviewedNotesPrune{Ref: ref}
	fullRef := ref
	if fullRef == "" {
		fullRef = "refs/notes/commits"
	} else if !strings.HasPrefix(fullRef, "refs/notes/") {
		fullRef = "refs/notes/" + fullRef
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--quiet", fullRef)
	if err == nil {
		p.NotesOID = trimLine(out)
	} else if commandExitCode(err) != 1 {
		return p, err
	}
	out, truncated, err := r.outputLimited(ctx, 8<<20, historyNotesArgs(ref, "prune", "--dry-run")...)
	if err != nil {
		return p, err
	}
	if truncated {
		return p, &TooLargeError{Resource: "notes prune plan"}
	}
	for _, line := range strings.Split(trimLine(out), "\n") {
		if line != "" {
			p.Objects = append(p.Objects, line)
		}
	}
	sort.Strings(p.Objects)
	return p, nil
}

func (r *Repository) PruneNotesReviewed(ctx context.Context, review ReviewedNotesPrune) error {
	now, err := r.ReviewNotesPrune(ctx, review.Ref)
	if err != nil {
		return err
	}
	if now.NotesOID != review.NotesOID || strings.Join(now.Objects, "\x00") != strings.Join(review.Objects, "\x00") {
		return errors.New("notes prune review is stale")
	}
	return r.NotesPrune(ctx, review.Ref, ConfirmOptions{Confirmed: true})
}
