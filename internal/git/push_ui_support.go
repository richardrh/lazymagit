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

func (r *Repository) pushUICommand(ctx context.Context, in PushUIArgs) ([]string, error) {
	remote, err := r.resolvePushTarget(ctx, in.Target, in.Remote)
	if err != nil {
		return nil, err
	}
	if in.Tags && in.AllTags {
		return nil, errors.New("push all-tags selectors are duplicated")
	}
	baseSelector := in.Refspec != "" || in.Source != "" || in.Destination != ""
	selectors := 0
	for _, set := range []bool{baseSelector, len(in.Refspecs) != 0, in.Matching, in.Tag != "", in.AllTags, in.Notes, in.NotesRef != ""} {
		if set {
			selectors++
		}
	}
	if selectors > 1 {
		return nil, errors.New("push selectors are mutually exclusive")
	}
	if (in.Tags || in.AllTags) && selectors != 0 && !in.AllTags {
		return nil, errors.New("push all-tags cannot be combined with another selector")
	}

	args := []string{"push"}
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

	var specs []string
	switch {
	case len(in.Refspecs) != 0:
		for _, spec := range in.Refspecs {
			if err := r.validateRefspec(ctx, spec, false); err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		}
	case in.Refspec != "":
		if err := r.validateRefspec(ctx, in.Refspec, false); err != nil {
			return nil, err
		}
		specs = append(specs, in.Refspec)
	case in.Source != "" || in.Destination != "":
		if in.Source == "" {
			return nil, errors.New("push source is empty")
		}
		if err := r.validatePushSource(ctx, in.Source); err != nil {
			return nil, err
		}
		spec := in.Source
		if in.Destination != "" {
			if err := r.validateBranch(ctx, in.Destination); err != nil {
				return nil, err
			}
			spec += ":" + in.Destination
		}
		specs = append(specs, spec)
	case in.Matching:
		specs = append(specs, ":")
	case in.Tag != "":
		if err := r.validateTag(ctx, in.Tag); err != nil {
			return nil, err
		}
		specs = append(specs, "refs/tags/"+in.Tag)
	case in.AllTags:
		args = append(args, "--tags")
	case in.NotesRef != "":
		if !strings.HasPrefix(in.NotesRef, "refs/notes/") {
			return nil, fmt.Errorf("invalid notes ref %q", in.NotesRef)
		}
		if _, err := r.output(ctx, "show-ref", "--verify", "--quiet", in.NotesRef); err != nil {
			return nil, fmt.Errorf("notes ref %q does not exist", in.NotesRef)
		}
		specs = append(specs, in.NotesRef+":"+in.NotesRef)
	case in.Notes:
		specs = append(specs, "refs/notes/*:refs/notes/*")
	default:
		branch, err := r.currentBranch(ctx)
		if err != nil {
			return nil, err
		}
		if branch == "" {
			return nil, errors.New("cannot push current branch from detached HEAD")
		}
		spec := branch
		if in.Target == PushToUpstream {
			merge, ok, err := r.configValue(ctx, "branch."+branch+".merge")
			if err != nil {
				return nil, err
			}
			if !ok || !strings.HasPrefix(merge, "refs/heads/") {
				return nil, ErrNoUpstream
			}
			spec += ":" + strings.TrimPrefix(merge, "refs/heads/")
		}
		specs = append(specs, spec)
	}
	args = append(args, "--", remote)
	args = append(args, specs...)
	return args, nil
}
