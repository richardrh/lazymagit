package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// PushUIArgs adds the two selectors which cannot be represented losslessly by
// PushArgs.  The ordinary switches and target continue to use PushArgs so the
// UI and non-UI push paths have the same option contract.
type PushUIArgs struct {
	PushArgs
	Refspecs []string
	NotesRef string
}

// ListNotesRefs returns fully-qualified local notes refs.  Keeping the full ref
// makes a singular selection unambiguous and avoids accidentally pushing the
// refs/notes/* wildcard.
func (r *Repository) ListNotesRefs(ctx context.Context) ([]string, error) {
	out, err := r.output(ctx, "for-each-ref", "--sort=refname", "--format=%(refname)", "refs/notes")
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, ref := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// ValidatePushUIArgs performs every repository-dependent check without
// running push.  It is used before a workflow review and, importantly, before
// persisting a newly selected pushRemote.
func (r *Repository) ValidatePushUIArgs(ctx context.Context, in PushUIArgs) error {
	_, err := r.pushUICommand(ctx, in)
	return err
}

// PushWithUIArgs validates all inputs and executes one git push process.  In
// particular, multiple refspecs remain multiple argv entries.
func (r *Repository) PushWithUIArgs(ctx context.Context, in PushUIArgs) error {
	args, err := r.pushUICommand(ctx, in)
	if err != nil {
		return err
	}
	return r.run(ctx, args...)
}

type pushUISelection struct {
	Refspecs []string
	AllTags  bool
}

func (r *Repository) pushUICommand(ctx context.Context, in PushUIArgs) ([]string, error) {
	remote, err := r.resolvePushTarget(ctx, in.Target, in.Remote)
	if err != nil {
		return nil, err
	}
	if err := validatePushUIRequest(in); err != nil {
		return nil, err
	}
	args, err := appendPushUIFlags([]string{"push"}, in)
	if err != nil {
		return nil, err
	}
	selection, err := r.compilePushUISelection(ctx, in)
	if err != nil {
		return nil, err
	}
	if selection.AllTags {
		args = append(args, "--tags")
	}
	args = append(args, "--", remote)
	return append(args, selection.Refspecs...), nil
}

func validatePushUIRequest(in PushUIArgs) error {
	if in.Tags && in.AllTags {
		return errors.New("push all-tags selectors are duplicated")
	}
	baseSelector := in.Refspec != "" || in.Source != "" || in.Destination != ""
	selectors := 0
	for _, set := range []bool{baseSelector, len(in.Refspecs) != 0, in.Matching, in.Tag != "", in.AllTags, in.Notes, in.NotesRef != ""} {
		if set {
			selectors++
		}
	}
	if selectors > 1 {
		return errors.New("push selectors are mutually exclusive")
	}
	if (in.Tags || in.AllTags) && selectors != 0 && !in.AllTags {
		return errors.New("push all-tags cannot be combined with another selector")
	}
	return nil
}

func appendPushUIFlags(args []string, in PushUIArgs) ([]string, error) {
	switch in.Force {
	case PushForceNone:
	case PushForceWithLease:
		args = append(args, "--force-with-lease")
	case PushForceUnconditionally:
		args = append(args, "--force")
	default:
		return nil, errors.New("invalid push force mode")
	}
	if in.NoVerify {
		args = append(args, "--no-verify")
	}
	if in.DryRun {
		args = append(args, "--dry-run")
	}
	if in.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if in.Tags {
		args = append(args, "--tags")
	}
	if in.FollowTags {
		args = append(args, "--follow-tags")
	}
	for _, option := range in.PushOptions {
		if option == "" {
			return nil, errors.New("push option is empty")
		}
		if strings.ContainsAny(option, "\x00\r\n") {
			return nil, errors.New("push option contains a control character")
		}
		args = append(args, "--push-option="+option)
	}
	return args, nil
}

func (r *Repository) compilePushUISelection(ctx context.Context, in PushUIArgs) (pushUISelection, error) {
	switch {
	case len(in.Refspecs) != 0:
		specs, err := r.compilePushUIRefspecs(ctx, in.Refspecs)
		return pushUISelection{Refspecs: specs}, err
	case in.Refspec != "":
		specs, err := r.compilePushUIRefspecs(ctx, []string{in.Refspec})
		return pushUISelection{Refspecs: specs}, err
	case in.Source != "" || in.Destination != "":
		spec, _, err := r.pushSourceSpec(ctx, in.PushArgs)
		return pushUISelection{Refspecs: []string{spec}}, err
	case in.Matching:
		return pushUISelection{Refspecs: []string{":"}}, nil
	case in.Tag != "":
		if err := r.validateTag(ctx, in.Tag); err != nil {
			return pushUISelection{}, err
		}
		return pushUISelection{Refspecs: []string{"refs/tags/" + in.Tag}}, nil
	case in.AllTags:
		return pushUISelection{AllTags: true}, nil
	case in.NotesRef != "":
		spec, err := r.compilePushUINotesRef(ctx, in.NotesRef)
		return pushUISelection{Refspecs: []string{spec}}, err
	case in.Notes:
		return pushUISelection{Refspecs: []string{"refs/notes/*:refs/notes/*"}}, nil
	default:
		spec, _, err := r.pushCurrentBranchSpec(ctx, in.Target)
		return pushUISelection{Refspecs: []string{spec}}, err
	}
}

func (r *Repository) compilePushUIRefspecs(ctx context.Context, specs []string) ([]string, error) {
	for _, spec := range specs {
		if err := r.validateRefspec(ctx, spec, false); err != nil {
			return nil, err
		}
	}
	return append([]string(nil), specs...), nil
}

func (r *Repository) compilePushUINotesRef(ctx context.Context, ref string) (string, error) {
	if !strings.HasPrefix(ref, "refs/notes/") {
		return "", fmt.Errorf("invalid notes ref %q", ref)
	}
	if _, err := r.output(ctx, "show-ref", "--verify", "--quiet", ref); err != nil {
		return "", fmt.Errorf("notes ref %q does not exist", ref)
	}
	return ref + ":" + ref, nil
}
