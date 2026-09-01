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
	if err := r.validateFetchArgs(ctx, in); err != nil {
		return err
	}
	args, err := fetchArgs(in)
	if err != nil {
		return err
	}
	return r.run(ctx, args...)
}

func (r *Repository) validateFetchArgs(ctx context.Context, in FetchArgs) error {
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
	return nil
}

func fetchArgs(in FetchArgs) ([]string, error) {
	options, err := fetchOptionArgs(in)
	if err != nil {
		return nil, err
	}
	suffix, err := fetchSuffixArgs(in)
	if err != nil {
		return nil, err
	}
	return append(append([]string{"fetch"}, options...), suffix...), nil
}

func fetchOptionArgs(in FetchArgs) ([]string, error) {
	var args []string
	if in.Prune {
		args = append(args, "--prune")
	}
	tags, err := fetchTagsArgs(in.Tags)
	if err != nil {
		return nil, err
	}
	args = append(args, tags...)
	if in.Unshallow {
		args = append(args, "--unshallow")
	}
	if in.Force {
		args = append(args, "--force")
	}
	submodules, err := fetchSubmoduleArgs(in.RecurseSubmodules)
	if err != nil {
		return nil, err
	}
	return append(args, submodules...), nil
}

func fetchTagsArgs(mode FetchTags) ([]string, error) {
	switch mode {
	case FetchTagsDefault:
		return nil, nil
	case FetchAllTags:
		return []string{"--tags"}, nil
	case FetchNoTags:
		return []string{"--no-tags"}, nil
	default:
		return nil, errors.New("invalid fetch tags mode")
	}
}

func fetchSubmoduleArgs(mode SubmoduleRecursion) ([]string, error) {
	switch mode {
	case SubmodulesDefault:
		return nil, nil
	case SubmodulesOnDemand:
		return []string{"--recurse-submodules=on-demand"}, nil
	case SubmodulesYes:
		return []string{"--recurse-submodules=yes"}, nil
	case SubmodulesNo:
		return []string{"--recurse-submodules=no"}, nil
	default:
		return nil, errors.New("invalid submodule recursion mode")
	}
}

func fetchSuffixArgs(in FetchArgs) ([]string, error) {
	if in.Remote == "" {
		if in.Branch != "" || in.Refspec != "" {
			return nil, errors.New("fetch suffix requires a remote")
		}
		return nil, nil
	}
	args := []string{"--", in.Remote}
	if in.Branch != "" {
		args = append(args, in.Branch)
	}
	if in.Refspec != "" {
		args = append(args, in.Refspec)
	}
	return args, nil
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
		return r.resolveUpstreamPullTarget(ctx)
	case PullPushRemote:
		return r.resolvePushRemotePullTarget(ctx, in.Branch)
	case PullRemoteBranch:
		return r.resolveExplicitPullTarget(ctx, in.Remote, in.Branch)
	default:
		return "", "", errors.New("invalid pull target")
	}
}

func (r *Repository) resolveUpstreamPullTarget(ctx context.Context) (string, string, error) {
	branch, err := r.currentBranch(ctx)
	if err != nil || branch == "" {
		return "", "", pullUpstreamError(err)
	}
	remote, err := r.requiredPullConfig(ctx, "branch."+branch+".remote")
	if err != nil {
		return "", "", err
	}
	merge, err := r.requiredPullConfig(ctx, "branch."+branch+".merge")
	if err != nil {
		return "", "", err
	}
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return "", "", err
	}
	return remote, strings.TrimPrefix(merge, "refs/heads/"), nil
}

func pullUpstreamError(err error) error {
	if err != nil {
		return err
	}
	return ErrNoUpstream
}

func (r *Repository) requiredPullConfig(ctx context.Context, key string) (string, error) {
	value, ok, err := r.configValue(ctx, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNoUpstream
	}
	return value, nil
}

func (r *Repository) resolvePushRemotePullTarget(ctx context.Context, branch string) (string, string, error) {
	remote, err := r.PushRemote(ctx)
	if err != nil {
		return "", "", err
	}
	if branch == "" {
		branch, err = r.currentBranch(ctx)
	}
	if err != nil {
		return "", "", err
	}
	if branch == "" {
		return "", "", errors.New("cannot pull current branch from a detached HEAD")
	}
	if err := r.validateBranch(ctx, branch); err != nil {
		return "", "", err
	}
	return remote, branch, nil
}

func (r *Repository) resolveExplicitPullTarget(ctx context.Context, remote, branch string) (string, string, error) {
	if remote == "" || branch == "" {
		return "", "", errors.New("explicit pull requires remote and branch")
	}
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return "", "", err
	}
	if err := r.validateBranch(ctx, branch); err != nil {
		return "", "", err
	}
	return remote, branch, nil
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
	plan, err := r.planPush(ctx, in)
	if err != nil {
		return err
	}
	return r.run(ctx, plan...)
}

func (r *Repository) planPush(ctx context.Context, in PushArgs) ([]string, error) {
	if err := validatePushSelectors(in); err != nil {
		return nil, err
	}
	remote, err := r.resolvePushTarget(ctx, in.Target, in.Remote)
	if err != nil {
		return nil, err
	}
	args, err := appendPushFlags([]string{"push"}, in)
	if err != nil {
		return nil, err
	}
	spec, allTags, err := r.pushSpec(ctx, in)
	if err != nil {
		return nil, err
	}
	if allTags {
		args = append(args, "--tags")
	}
	args = append(args, "--", remote)
	if spec != "" {
		args = append(args, spec)
	}
	return args, nil
}

func validatePushSelectors(in PushArgs) error {
	if in.Tags && in.AllTags {
		return errors.New("push all-tags selectors are duplicated")
	}
	if countPushSelectors(in) > 1 {
		return errors.New("push selectors are mutually exclusive")
	}
	if (in.Tags || in.AllTags) && hasNonAllTagsSelector(in) {
		return errors.New("push all-tags cannot be combined with another selector")
	}
	return nil
}

func countPushSelectors(in PushArgs) int {
	count := 0
	for _, set := range []bool{hasPushRefSelector(in), in.Matching, in.Tag != "", in.AllTags, in.Notes} {
		if set {
			count++
		}
	}
	return count
}

func hasPushRefSelector(in PushArgs) bool {
	return in.Refspec != "" || in.Source != "" || in.Destination != ""
}
func hasNonAllTagsSelector(in PushArgs) bool {
	return hasPushRefSelector(in) || in.Matching || in.Tag != "" || in.Notes
}

func appendPushFlags(args []string, in PushArgs) ([]string, error) {
	args, err := appendPushForce(args, in.Force)
	if err != nil {
		return nil, err
	}
	for _, option := range []struct {
		set  bool
		flag string
	}{{in.NoVerify, "--no-verify"}, {in.DryRun, "--dry-run"}, {in.SetUpstream, "--set-upstream"}, {in.Tags, "--tags"}, {in.FollowTags, "--follow-tags"}} {
		if option.set {
			args = append(args, option.flag)
		}
	}
	return appendPushOptions(args, in.PushOptions)
}

func appendPushForce(args []string, force PushForce) ([]string, error) {
	switch force {
	case PushForceNone:
		return args, nil
	case PushForceWithLease:
		return append(args, "--force-with-lease"), nil
	case PushForceUnconditionally:
		return append(args, "--force"), nil
	default:
		return nil, errors.New("invalid push force mode")
	}
}

func appendPushOptions(args, options []string) ([]string, error) {
	for _, option := range options {
		if strings.ContainsAny(option, "\x00\r\n") {
			return nil, errors.New("push option contains a control character")
		}
		args = append(args, "--push-option="+option)
	}
	return args, nil
}

func (r *Repository) pushSpec(ctx context.Context, in PushArgs) (string, bool, error) {
	switch {
	case in.Refspec != "":
		if err := r.validateRefspec(ctx, in.Refspec, false); err != nil {
			return "", false, err
		}
		return in.Refspec, false, nil
	case in.Source != "" || in.Destination != "":
		return r.pushSourceSpec(ctx, in)
	case in.Matching:
		return ":", false, nil
	case in.Tag != "":
		if err := r.validateTag(ctx, in.Tag); err != nil {
			return "", false, err
		}
		return "refs/tags/" + in.Tag, false, nil
	case in.AllTags:
		return "", true, nil
	case in.Notes:
		return "refs/notes/*:refs/notes/*", false, nil
	default:
		return r.pushCurrentBranchSpec(ctx, in.Target)
	}
}

func (r *Repository) pushSourceSpec(ctx context.Context, in PushArgs) (string, bool, error) {
	if in.Source == "" {
		return "", false, errors.New("push source is empty")
	}
	if err := r.validatePushSource(ctx, in.Source); err != nil {
		return "", false, err
	}
	if in.Destination == "" {
		return in.Source, false, nil
	}
	if err := r.validateBranch(ctx, in.Destination); err != nil {
		return "", false, err
	}
	return in.Source + ":" + in.Destination, false, nil
}

func (r *Repository) pushCurrentBranchSpec(ctx context.Context, target PushTarget) (string, bool, error) {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return "", false, err
	}
	if branch == "" {
		return "", false, errors.New("cannot push current branch from detached HEAD")
	}
	if target != PushToUpstream {
		return branch, false, nil
	}
	merge, ok, err := r.configValue(ctx, "branch."+branch+".merge")
	if err != nil {
		return "", false, err
	}
	if !ok || !strings.HasPrefix(merge, "refs/heads/") {
		return "", false, ErrNoUpstream
	}
	return branch + ":" + strings.TrimPrefix(merge, "refs/heads/"), false, nil
}

func (r *Repository) resolvePushTarget(ctx context.Context, target PushTarget, explicit string) (string, error) {
	switch target {
	case PushToPushRemote:
		return r.PushRemote(ctx)
	case PushToUpstream:
		return r.upstreamPushRemote(ctx)
	case PushElsewhere:
		return r.explicitPushRemote(ctx, explicit)
	default:
		return "", errors.New("invalid push target")
	}
}

func (r *Repository) upstreamPushRemote(ctx context.Context) (string, error) {
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
}

func (r *Repository) explicitPushRemote(ctx context.Context, remote string) (string, error) {
	if remote == "" {
		return "", errors.New("push remote is empty")
	}
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return "", err
	}
	return remote, nil
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
	source, destination, hasColon, err := splitRefspec(spec)
	if err != nil {
		return err
	}
	if err := validateFetchRefspecParts(source, destination, hasColon, fetch); err != nil {
		return err
	}
	if fetch {
		return r.validateFetchRefspec(ctx, source, destination, hasColon)
	}
	for _, ref := range []string{source, destination} {
		if err := r.validateRefspecRef(ctx, ref, false); err != nil {
			return err
		}
	}
	return r.validatePushRefspec(ctx, source, destination)
}

func validateFetchRefspecParts(source, destination string, hasColon, fetch bool) error {
	if fetch && (!hasColon || source == "" || destination == "") {
		return fmt.Errorf("fetch refspec requires source and destination")
	}
	return nil
}

func (r *Repository) validateFetchRefspec(ctx context.Context, source, destination string, hasColon bool) error {
	if err := validateFetchRefspecParts(source, destination, hasColon, true); err != nil {
		return err
	}
	for _, ref := range []string{source, destination} {
		if err := r.validateRefspecRef(ctx, ref, true); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) validatePushRefspec(ctx context.Context, source, destination string) error {
	if source != "" && !strings.HasPrefix(source, "refs/") {
		if err := r.validatePushSource(ctx, source); err != nil {
			return err
		}
	}
	if destination != "" && !strings.HasPrefix(destination, "refs/") {
		return r.validateBranch(ctx, destination)
	}
	return nil
}

func splitRefspec(spec string) (source, destination string, hasColon bool, err error) {
	value := strings.TrimPrefix(spec, "+")
	if strings.Count(value, ":") > 1 {
		return "", "", false, fmt.Errorf("invalid refspec %q", spec)
	}
	source, destination, hasColon = strings.Cut(value, ":")
	return source, destination, hasColon, nil
}

func (r *Repository) validateRefspecRef(ctx context.Context, ref string, fetch bool) error {
	if ref == "" {
		return nil
	}
	if !strings.HasPrefix(ref, "refs/") {
		if fetch {
			return fmt.Errorf("fetch refspec ref %q is not fully qualified", ref)
		}
		return nil
	}
	if _, err := r.output(ctx, "check-ref-format", "--refspec-pattern", ref); err != nil {
		return fmt.Errorf("invalid refspec ref %q", ref)
	}
	return nil
}
