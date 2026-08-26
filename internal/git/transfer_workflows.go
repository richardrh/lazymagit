package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type FetchTags uint8

const (
	FetchTagsDefault FetchTags = iota
	FetchAllTags
	FetchNoTags
)

type SubmoduleRecursion uint8

const (
	SubmodulesDefault SubmoduleRecursion = iota
	SubmodulesOnDemand
	SubmodulesYes
	SubmodulesNo
)

// FetchArgs models fetch's transient infixes and its optional suffix. Branch
// and Refspec are mutually exclusive; both are interpreted literally.
type FetchArgs struct {
	Remote            string
	Prune             bool
	Tags              FetchTags
	Unshallow         bool
	Force             bool
	Branch            string
	Refspec           string
	RecurseSubmodules SubmoduleRecursion
}

func (r *Repository) FetchWithArgs(ctx context.Context, in FetchArgs) error {
	if in.Branch != "" && in.Refspec != "" {
		return errors.New("fetch branch and refspec are mutually exclusive")
	}
	if in.Remote != "" {
		if err := r.validateTransferRemote(ctx, in.Remote); err != nil {
			return err
		}
	}
	if in.Branch != "" {
		if err := r.validateBranch(ctx, in.Branch); err != nil {
			return fmt.Errorf("fetch branch: %w", err)
		}
	}
	if in.Refspec != "" {
		if err := r.validateRefspec(ctx, in.Refspec, true); err != nil {
			return fmt.Errorf("fetch refspec: %w", err)
		}
	}
	args := []string{"fetch"}
	if in.Prune {
		args = append(args, "--prune")
	}
	switch in.Tags {
	case FetchTagsDefault:
	case FetchAllTags:
		args = append(args, "--tags")
	case FetchNoTags:
		args = append(args, "--no-tags")
	default:
		return errors.New("invalid fetch tags mode")
	}
	if in.Unshallow {
		args = append(args, "--unshallow")
	}
	if in.Force {
		args = append(args, "--force")
	}
	switch in.RecurseSubmodules {
	case SubmodulesDefault:
	case SubmodulesOnDemand:
		args = append(args, "--recurse-submodules=on-demand")
	case SubmodulesYes:
		args = append(args, "--recurse-submodules=yes")
	case SubmodulesNo:
		args = append(args, "--recurse-submodules=no")
	default:
		return errors.New("invalid submodule recursion mode")
	}
	if in.Remote != "" || in.Branch != "" || in.Refspec != "" {
		if in.Remote == "" {
			return errors.New("fetch suffix requires a remote")
		}
		args = append(args, "--", in.Remote)
		if in.Branch != "" {
			args = append(args, in.Branch)
		}
		if in.Refspec != "" {
			args = append(args, in.Refspec)
		}
	}
	return r.run(ctx, args...)
}

type PullTarget uint8

const (
	PullUpstream PullTarget = iota
	PullPushRemote
	PullRemoteBranch
)

type PullMode uint8

const (
	PullMerge PullMode = iota
	PullFFOnly
	PullRebase
)

type PullArgs struct {
	Target    PullTarget
	Remote    string
	Branch    string
	Mode      PullMode
	Autostash bool
	Force     bool
}

// PullWithArgs always makes the integration policy explicit. Merge pulls use
// --no-edit. A non-interactive rebase does not launch a sequence editor and
// stops at conflicts instead of entering an editor-driven continue step.
func (r *Repository) PullWithArgs(ctx context.Context, in PullArgs) error {
	remote, branch, err := r.resolvePullTarget(ctx, in)
	if err != nil {
		return err
	}
	args := []string{"pull"}
	switch in.Mode {
	case PullMerge:
		args = append(args, "--no-rebase", "--no-edit")
	case PullFFOnly:
		args = append(args, "--ff-only")
	case PullRebase:
		args = append(args, "--rebase")
	default:
		return errors.New("invalid pull mode")
	}
	if in.Autostash {
		args = append(args, "--autostash")
	}
	if in.Force {
		args = append(args, "--force")
	}
	args = append(args, "--", remote)
	if branch != "" {
		args = append(args, branch)
	}
	return r.run(ctx, args...)
}

func (r *Repository) resolvePullTarget(ctx context.Context, in PullArgs) (string, string, error) {
	switch in.Target {
	case PullUpstream:
		branch, err := r.currentBranch(ctx)
		if err != nil {
			return "", "", err
		}
		if branch == "" {
			return "", "", ErrNoUpstream
		}
		remote, ok, err := r.configValue(ctx, "branch."+branch+".remote")
		if err != nil {
			return "", "", err
		}
		if !ok {
			return "", "", ErrNoUpstream
		}
		merge, ok, err := r.configValue(ctx, "branch."+branch+".merge")
		if err != nil {
			return "", "", err
		}
		if !ok {
			return "", "", ErrNoUpstream
		}
		if err := r.validateTransferRemote(ctx, remote); err != nil {
			return "", "", err
		}
		return remote, strings.TrimPrefix(merge, "refs/heads/"), nil
	case PullPushRemote:
		remote, err := r.PushRemote(ctx)
		if err != nil {
			return "", "", err
		}
		branch := in.Branch
		if branch == "" {
			branch, err = r.currentBranch(ctx)
			if err != nil {
				return "", "", err
			}
		}
		if branch == "" {
			return "", "", errors.New("cannot pull current branch from a detached HEAD")
		}
		if err := r.validateBranch(ctx, branch); err != nil {
			return "", "", err
		}
		return remote, branch, nil
	case PullRemoteBranch:
		if in.Remote == "" || in.Branch == "" {
			return "", "", errors.New("explicit pull requires remote and branch")
		}
		if err := r.validateTransferRemote(ctx, in.Remote); err != nil {
			return "", "", err
		}
		if err := r.validateBranch(ctx, in.Branch); err != nil {
			return "", "", err
		}
		return in.Remote, in.Branch, nil
	default:
		return "", "", errors.New("invalid pull target")
	}
}

type PushTarget uint8

const (
	PushToPushRemote PushTarget = iota
	PushToUpstream
	PushElsewhere
)

type PushForce uint8

const (
	PushForceNone PushForce = iota
	PushForceWithLease
	PushForceUnconditionally
)

type PushArgs struct {
	Target      PushTarget
	Remote      string
	Source      string
	Destination string
	Refspec     string
	Matching    bool
	Tag         string
	AllTags     bool
	Notes       bool
	Force       PushForce
	NoVerify    bool
	DryRun      bool
	SetUpstream bool
	Tags        bool
	FollowTags  bool
	PushOptions []string
}

func (r *Repository) PushWithArgs(ctx context.Context, in PushArgs) error {
	remote, err := r.resolvePushTarget(ctx, in.Target, in.Remote)
	if err != nil {
		return err
	}
	if in.Tags && in.AllTags {
		return errors.New("push all-tags selectors are duplicated")
	}
	selectors := 0
	for _, set := range []bool{in.Refspec != "" || in.Source != "" || in.Destination != "", in.Matching, in.Tag != "", in.AllTags, in.Notes} {
		if set {
			selectors++
		}
	}
	if selectors > 1 {
		return errors.New("push selectors are mutually exclusive")
	}
	if (in.Tags || in.AllTags) && (in.Refspec != "" || in.Source != "" || in.Destination != "" || in.Matching || in.Tag != "" || in.Notes) {
		return errors.New("push all-tags cannot be combined with another selector")
	}
	args := []string{"push"}
	switch in.Force {
	case PushForceNone:
	case PushForceWithLease:
		args = append(args, "--force-with-lease")
	case PushForceUnconditionally:
		args = append(args, "--force")
	default:
		return errors.New("invalid push force mode")
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
		if strings.ContainsAny(option, "\x00\r\n") {
			return errors.New("push option contains a control character")
		}
		args = append(args, "--push-option="+option)
	}
	var spec string
	switch {
	case in.Refspec != "":
		if err := r.validateRefspec(ctx, in.Refspec, false); err != nil {
			return err
		}
		spec = in.Refspec
	case in.Source != "" || in.Destination != "":
		if in.Source == "" {
			return errors.New("push source is empty")
		}
		if err := r.validatePushSource(ctx, in.Source); err != nil {
			return err
		}
		if in.Destination != "" {
			if err := r.validateBranch(ctx, in.Destination); err != nil {
				return err
			}
			spec = in.Source + ":" + in.Destination
		} else {
			spec = in.Source
		}
	case in.Matching:
		spec = ":"
	case in.Tag != "":
		if err := r.validateTag(ctx, in.Tag); err != nil {
			return err
		}
		spec = "refs/tags/" + in.Tag
	case in.AllTags:
		args = append(args, "--tags")
	case in.Notes:
		spec = "refs/notes/*:refs/notes/*"
	default:
		branch, err := r.currentBranch(ctx)
		if err != nil {
			return err
		}
		if branch == "" {
			return errors.New("cannot push current branch from detached HEAD")
		}
		spec = branch
		if in.Target == PushToUpstream {
			merge, ok, err := r.configValue(ctx, "branch."+branch+".merge")
			if err != nil {
				return err
			}
			if !ok || !strings.HasPrefix(merge, "refs/heads/") {
				return ErrNoUpstream
			}
			spec += ":" + strings.TrimPrefix(merge, "refs/heads/")
		}
	}
	args = append(args, "--", remote)
	if spec != "" {
		args = append(args, spec)
	}
	return r.run(ctx, args...)
}

func (r *Repository) resolvePushTarget(ctx context.Context, target PushTarget, explicit string) (string, error) {
	switch target {
	case PushToPushRemote:
		return r.PushRemote(ctx)
	case PushToUpstream:
		branch, err := r.currentBranch(ctx)
		if err != nil {
			return "", err
		}
		if branch == "" {
			return "", ErrNoUpstream
		}
		remote, ok, err := r.configValue(ctx, "branch."+branch+".remote")
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrNoUpstream
		}
		if err := r.validateTransferRemote(ctx, remote); err != nil {
			return "", err
		}
		return remote, nil
	case PushElsewhere:
		if explicit == "" {
			return "", errors.New("push remote is empty")
		}
		if err := r.validateTransferRemote(ctx, explicit); err != nil {
			return "", err
		}
		return explicit, nil
	default:
		return "", errors.New("invalid push target")
	}
}

func (r *Repository) validateTransferRemote(ctx context.Context, name string) error {
	if err := validateToken("remote", name); err != nil {
		return err
	}
	return r.validateRemote(ctx, name)
}

func validateToken(kind, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", kind)
	}
	if strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("invalid %s %q", kind, value)
	}
	return nil
}

func (r *Repository) validateBranch(ctx context.Context, name string) error {
	if err := validateToken("branch", name); err != nil {
		return err
	}
	if _, err := r.output(ctx, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("invalid branch %q", name)
	}
	return nil
}

func (r *Repository) validateTag(ctx context.Context, name string) error {
	if err := validateToken("tag", name); err != nil {
		return err
	}
	if _, err := r.output(ctx, "check-ref-format", "refs/tags/"+name); err != nil {
		return fmt.Errorf("invalid tag %q", name)
	}
	return nil
}

func (r *Repository) validatePushSource(ctx context.Context, source string) error {
	if err := validateToken("push source", source); err != nil {
		return err
	}
	if _, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", source+"^{object}"); err != nil {
		return fmt.Errorf("invalid push source %q: %w", source, err)
	}
	return nil
}

func (r *Repository) validateRefspec(ctx context.Context, spec string, fetch bool) error {
	if err := validateToken("refspec", spec); err != nil {
		return err
	}
	value := strings.TrimPrefix(spec, "+")
	if strings.Count(value, ":") > 1 {
		return fmt.Errorf("invalid refspec %q", spec)
	}
	source, destination, hasColon := strings.Cut(value, ":")
	if fetch && (!hasColon || source == "" || destination == "") {
		return fmt.Errorf("fetch refspec requires source and destination")
	}
	for _, ref := range []string{source, destination} {
		if ref == "" {
			continue
		}
		if strings.HasPrefix(ref, "refs/") {
			if _, err := r.output(ctx, "check-ref-format", "--refspec-pattern", ref); err != nil {
				return fmt.Errorf("invalid refspec ref %q", ref)
			}
			continue
		}
		if fetch {
			return fmt.Errorf("fetch refspec ref %q is not fully qualified", ref)
		}
	}
	if !fetch {
		if source != "" && !strings.HasPrefix(source, "refs/") {
			if err := r.validatePushSource(ctx, source); err != nil {
				return err
			}
		}
		if destination != "" && !strings.HasPrefix(destination, "refs/") {
			if err := r.validateBranch(ctx, destination); err != nil {
				return err
			}
		}
	}
	return nil
}
