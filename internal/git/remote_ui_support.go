package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ReviewedRemoteChange is the immutable capability used by the UI for remote
// rename and removal.  Unlike RemoveRemoteArgs.Confirm, it is bound to the
// exact refs and push configuration shown to the user.
type ReviewedRemoteChange struct {
	Preflight RemoteChangePreflight
	NewName   string
	Token     ConfirmationToken
}

func (r *Repository) ReviewRemoteRename(ctx context.Context, oldName, newName string) (ReviewedRemoteChange, error) {
	p, err := r.RemoteChangePreflight(ctx, oldName)
	if err != nil {
		return ReviewedRemoteChange{}, err
	}
	if err := r.validateNewRemoteName(ctx, newName); err != nil {
		return ReviewedRemoteChange{}, err
	}
	review := ReviewedRemoteChange{Preflight: p, NewName: newName}
	review.Token = NewConfirmationToken(remoteChangeIdentity(review))
	return review, nil
}

func (r *Repository) RenameRemoteReviewed(ctx context.Context, reviewed ReviewedRemoteChange) error {
	current, err := r.ReviewRemoteRename(ctx, reviewed.Preflight.Remote, reviewed.NewName)
	if err != nil {
		return err
	}
	if !reviewed.Token.validFor(remoteChangeIdentity(current)) || !sameRemotePreflight(current.Preflight, reviewed.Preflight) {
		return ErrStalePlan
	}
	_, err = r.RenameRemote(ctx, RenameRemoteArgs{Old: reviewed.Preflight.Remote, New: reviewed.NewName})
	return err
}

func (r *Repository) ReviewRemoteRemoval(ctx context.Context, remote string) (ReviewedRemoteChange, error) {
	p, err := r.RemoteChangePreflight(ctx, remote)
	if err != nil {
		return ReviewedRemoteChange{}, err
	}
	review := ReviewedRemoteChange{Preflight: p}
	review.Token = NewConfirmationToken(remoteChangeIdentity(review))
	return review, nil
}

func (r *Repository) RemoveRemoteReviewed(ctx context.Context, reviewed ReviewedRemoteChange) error {
	current, err := r.ReviewRemoteRemoval(ctx, reviewed.Preflight.Remote)
	if err != nil {
		return err
	}
	if !reviewed.Token.validFor(remoteChangeIdentity(current)) || !sameRemotePreflight(current.Preflight, reviewed.Preflight) {
		return ErrStalePlan
	}
	_, err = r.RemoveRemoteWithArgs(ctx, RemoveRemoteArgs{Remote: reviewed.Preflight.Remote, Confirm: true})
	return err
}

func remoteChangeIdentity(p ReviewedRemoteChange) string {
	b, _ := json.Marshal(struct {
		Old, New, PushDefault                                                                          string
		Tracking, TrackingOIDs, TrackingSymbols, BranchPush, BranchRemotes, BranchMerges, RemoteConfig []string
	}{p.Preflight.Remote, p.NewName, fmt.Sprint(p.Preflight.UsesRemotePushDefault), p.Preflight.TrackingRefs, p.Preflight.TrackingRefOIDs, p.Preflight.TrackingRefSymbols, p.Preflight.BranchPushRemotes, p.Preflight.BranchRemotes, p.Preflight.BranchMerges, p.Preflight.RemoteConfig})
	return string(b)
}

func sameRemotePreflight(a, b RemoteChangePreflight) bool {
	return a.Remote == b.Remote && a.UsesRemotePushDefault == b.UsesRemotePushDefault &&
		sameStrings(a.TrackingRefs, b.TrackingRefs) && sameStrings(a.TrackingRefOIDs, b.TrackingRefOIDs) && sameStrings(a.TrackingRefSymbols, b.TrackingRefSymbols) && sameStrings(a.BranchPushRemotes, b.BranchPushRemotes) && sameStrings(a.BranchRemotes, b.BranchRemotes) && sameStrings(a.BranchMerges, b.BranchMerges) && sameStrings(a.RemoteConfig, b.RemoteConfig)
}

// RemoteConfiguration is a lossless snapshot. Nil means the key is absent;
// an empty non-nil slice means it is present with no values in a requested
// replacement. This mirrors RemoteConfigArgs' unchanged/clear distinction.
type RemoteConfiguration struct {
	FetchURL, PushURL           *string
	PushURLConfigured           bool
	FetchRefspecs, PushRefspecs []string
	TagOpt                      *RemoteTagOpt
	FollowRemoteHEAD            *RemoteFollowRemoteHEAD
}

type ReviewedRemoteConfiguration struct {
	Args    RemoteConfigArgs
	Before  RemoteConfiguration
	Changes []string
	Token   ConfirmationToken
}

func (r *Repository) RemoteConfiguration(ctx context.Context, remote string) (RemoteConfiguration, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return RemoteConfiguration{}, err
	}
	var c RemoteConfiguration
	if v, ok, err := r.configValue(ctx, "remote."+remote+".url"); err != nil {
		return c, err
	} else if ok {
		c.FetchURL = &v
	}
	if values, err := r.configValues(ctx, "remote."+remote+".pushurl"); err != nil {
		return c, err
	} else if len(values) != 0 {
		v := values[0]
		c.PushURL = &v
		c.PushURLConfigured = true
	} else if c.FetchURL != nil {
		v := *c.FetchURL
		c.PushURL = &v
	}
	var err error
	if c.FetchRefspecs, err = r.configValues(ctx, "remote."+remote+".fetch"); err != nil {
		return c, err
	}
	if c.PushRefspecs, err = r.configValues(ctx, "remote."+remote+".push"); err != nil {
		return c, err
	}
	if v, ok, err := r.configValue(ctx, "remote."+remote+".tagopt"); err != nil {
		return c, err
	} else if ok {
		opt := RemoteTagsDefault
		switch v {
		case "--tags":
			opt = RemoteTagsAll
		case "--no-tags":
			opt = RemoteTagsNone
		default:
			return c, fmt.Errorf("unsupported remote tag option %q", v)
		}
		c.TagOpt = &opt
	}
	if v, ok, err := r.configValue(ctx, "remote."+remote+".followRemoteHEAD"); err != nil {
		return c, err
	} else if ok {
		modes := map[string]RemoteFollowRemoteHEAD{"never": RemoteFollowRemoteHEADNever, "create": RemoteFollowRemoteHEADCreate, "warn": RemoteFollowRemoteHEADWarn, "always": RemoteFollowRemoteHEADAlways}
		mode, valid := modes[v]
		if !valid {
			return c, fmt.Errorf("unsupported remote followRemoteHEAD option %q", v)
		}
		c.FollowRemoteHEAD = &mode
	}
	return c, nil
}

func (r *Repository) configValues(ctx context.Context, key string) ([]string, error) {
	out, err := r.output(ctx, "config", "--get-all", key)
	if err != nil {
		if commandExitCode(err) == 1 {
			return nil, nil
		}
		return nil, err
	}
	text := trimLine(out)
	if text == "" {
		return []string{""}, nil
	}
	return strings.Split(text, "\n"), nil
}

func (r *Repository) ReviewRemoteConfiguration(ctx context.Context, in RemoteConfigArgs) (ReviewedRemoteConfiguration, error) {
	before, err := r.RemoteConfiguration(ctx, in.Remote)
	if err != nil {
		return ReviewedRemoteConfiguration{}, err
	}
	if err := r.validateRemoteConfigurationRequest(ctx, in); err != nil {
		return ReviewedRemoteConfiguration{}, err
	}
	review := ReviewedRemoteConfiguration{Args: cloneRemoteConfigArgs(in), Before: cloneRemoteConfiguration(before)}
	review.Changes = remoteConfigurationChanges(before, in)
	review.Token = NewConfirmationToken(remoteConfigurationIdentity(review.Before, review.Args))
	return review, nil
}

func (r *Repository) ConfigureRemoteReviewed(ctx context.Context, reviewed ReviewedRemoteConfiguration) error {
	current, err := r.RemoteConfiguration(ctx, reviewed.Args.Remote)
	if err != nil {
		return err
	}
	if !reviewed.Token.validFor(remoteConfigurationIdentity(current, reviewed.Args)) {
		return ErrStalePlan
	}
	if err := r.validateRemoteConfigurationRequest(ctx, reviewed.Args); err != nil {
		return err
	}
	return r.ConfigureRemote(ctx, cloneRemoteConfigArgs(reviewed.Args))
}

func (r *Repository) validateRemoteConfigurationRequest(ctx context.Context, in RemoteConfigArgs) error {
	if err := r.validateTransferRemote(ctx, in.Remote); err != nil {
		return err
	}
	for label, value := range map[string]*string{"fetch": in.FetchURL, "push": in.PushURL} {
		if value != nil && (*value == "" || strings.ContainsAny(*value, "\x00\r\n")) {
			return fmt.Errorf("%s URL is empty or contains a control character", label)
		}
	}
	for _, pair := range []struct {
		values []string
		fetch  bool
	}{{in.FetchRefspecs, true}, {in.PushRefspecs, false}} {
		for _, spec := range pair.values {
			if err := r.validateRefspec(ctx, spec, pair.fetch); err != nil {
				return err
			}
		}
	}
	if in.TagOpt != nil && *in.TagOpt != RemoteTagsDefault && *in.TagOpt != RemoteTagsAll && *in.TagOpt != RemoteTagsNone {
		return errors.New("invalid remote tag option")
	}
	if in.FollowRemoteHEAD != nil && (*in.FollowRemoteHEAD < RemoteFollowRemoteHEADDefault || *in.FollowRemoteHEAD > RemoteFollowRemoteHEADAlways) {
		return errors.New("invalid remote followRemoteHEAD option")
	}
	return nil
}

func cloneRemoteConfigArgs(in RemoteConfigArgs) RemoteConfigArgs {
	out := in
	if in.FetchURL != nil {
		v := *in.FetchURL
		out.FetchURL = &v
	}
	if in.PushURL != nil {
		v := *in.PushURL
		out.PushURL = &v
	}
	out.FetchRefspecs = append([]string(nil), in.FetchRefspecs...)
	if in.FetchRefspecs != nil && out.FetchRefspecs == nil {
		out.FetchRefspecs = []string{}
	}
	out.PushRefspecs = append([]string(nil), in.PushRefspecs...)
	if in.PushRefspecs != nil && out.PushRefspecs == nil {
		out.PushRefspecs = []string{}
	}
	if in.TagOpt != nil {
		v := *in.TagOpt
		out.TagOpt = &v
	}
	if in.FollowRemoteHEAD != nil {
		v := *in.FollowRemoteHEAD
		out.FollowRemoteHEAD = &v
	}
	return out
}

func cloneRemoteConfiguration(in RemoteConfiguration) RemoteConfiguration {
	out := in
	if in.FetchURL != nil {
		v := *in.FetchURL
		out.FetchURL = &v
	}
	if in.PushURL != nil {
		v := *in.PushURL
		out.PushURL = &v
	}
	out.FetchRefspecs = append([]string(nil), in.FetchRefspecs...)
	out.PushRefspecs = append([]string(nil), in.PushRefspecs...)
	if in.TagOpt != nil {
		v := *in.TagOpt
		out.TagOpt = &v
	}
	if in.FollowRemoteHEAD != nil {
		v := *in.FollowRemoteHEAD
		out.FollowRemoteHEAD = &v
	}
	return out
}

func remoteConfigurationIdentity(before RemoteConfiguration, in RemoteConfigArgs) string {
	b, _ := json.Marshal(struct {
		Before RemoteConfiguration
		Args   RemoteConfigArgs
	}{before, cloneRemoteConfigArgs(in)})
	return string(b)
}

func remoteConfigurationChanges(before RemoteConfiguration, in RemoteConfigArgs) []string {
	var out []string
	show := func(key, old, next string) {
		if old != next {
			out = append(out, key+": "+old+" -> "+next)
		}
	}
	ptr := func(v *string) string {
		if v == nil {
			return "<unset>"
		}
		return *v
	}
	if in.FetchURL != nil {
		show("remote."+in.Remote+".url", ptr(before.FetchURL), *in.FetchURL)
	}
	if in.PushURL != nil {
		old := ptr(before.PushURL)
		if !before.PushURLConfigured {
			old += " (inherited)"
		}
		show("remote."+in.Remote+".pushurl", old, *in.PushURL)
	}
	if in.FetchRefspecs != nil {
		show("remote."+in.Remote+".fetch", strings.Join(before.FetchRefspecs, ", "), strings.Join(in.FetchRefspecs, ", "))
	}
	if in.PushRefspecs != nil {
		show("remote."+in.Remote+".push", strings.Join(before.PushRefspecs, ", "), strings.Join(in.PushRefspecs, ", "))
	}
	if in.TagOpt != nil {
		old := "<unset>"
		if before.TagOpt != nil {
			old = fmt.Sprint(*before.TagOpt)
		}
		show("remote."+in.Remote+".tagopt", old, fmt.Sprint(*in.TagOpt))
	}
	if in.FollowRemoteHEAD != nil {
		old := "<unset>"
		if before.FollowRemoteHEAD != nil {
			old = fmt.Sprint(*before.FollowRemoteHEAD)
		}
		show("remote."+in.Remote+".followRemoteHEAD", old, fmt.Sprint(*in.FollowRemoteHEAD))
	}
	if len(out) == 0 {
		out = append(out, "No configuration changes")
	}
	return out
}

type RemoteDefaultBranchPlan struct {
	Remote, PreviousRef, NewRef string
	Token                       ConfirmationToken
}

func (r *Repository) ReviewRemoteDefaultBranch(ctx context.Context, remote string) (RemoteDefaultBranchPlan, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return RemoteDefaultBranchPlan{}, err
	}
	plan := RemoteDefaultBranchPlan{Remote: remote}
	if out, err := r.output(ctx, "symbolic-ref", "-q", "refs/remotes/"+remote+"/HEAD"); err == nil {
		plan.PreviousRef = trimLine(out)
	} else if commandExitCode(err) != 1 {
		return plan, err
	}
	out, err := r.output(ctx, "ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return plan, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "ref: ") && strings.HasSuffix(line, "\tHEAD") {
			branch := strings.TrimSuffix(strings.TrimPrefix(line, "ref: "), "\tHEAD")
			if strings.HasPrefix(branch, "refs/heads/") {
				plan.NewRef = "refs/remotes/" + remote + "/" + strings.TrimPrefix(branch, "refs/heads/")
			}
		}
	}
	if plan.NewRef == "" {
		return plan, errors.New("remote HEAD is not a symbolic branch")
	}
	plan.Token = NewConfirmationToken(plan.Remote + "\x00" + plan.PreviousRef + "\x00" + plan.NewRef)
	return plan, nil
}

func (r *Repository) UpdateRemoteDefaultBranch(ctx context.Context, reviewed RemoteDefaultBranchPlan) error {
	current, err := r.ReviewRemoteDefaultBranch(ctx, reviewed.Remote)
	if err != nil {
		return err
	}
	identity := current.Remote + "\x00" + current.PreviousRef + "\x00" + current.NewRef
	if !reviewed.Token.validFor(identity) || reviewed.PreviousRef != current.PreviousRef || reviewed.NewRef != current.NewRef {
		return ErrStalePlan
	}
	return r.run(ctx, "remote", "set-head", reviewed.Remote, "--auto")
}

func RemoteChangePlanLines(p ReviewedRemoteChange) []string {
	lines := []string{"remote: " + p.Preflight.Remote}
	if p.NewName != "" {
		lines = append(lines, "new name: "+p.NewName)
	}
	for _, ref := range p.Preflight.TrackingRefs {
		if p.NewName == "" {
			lines = append(lines, "delete tracking ref: "+ref)
		} else {
			next := strings.Replace(ref, "refs/remotes/"+p.Preflight.Remote+"/", "refs/remotes/"+p.NewName+"/", 1)
			lines = append(lines, "tracking ref: "+ref+" -> "+next)
		}
	}
	if p.Preflight.UsesRemotePushDefault {
		next := "<unset>"
		if p.NewName != "" {
			next = p.NewName
		}
		lines = append(lines, "config remote.pushDefault: "+p.Preflight.Remote+" -> "+next)
	}
	for _, branch := range p.Preflight.BranchPushRemotes {
		next := "<unset>"
		if p.NewName != "" {
			next = p.NewName
		}
		lines = append(lines, "config branch."+branch+".pushRemote: "+p.Preflight.Remote+" -> "+next)
	}
	for _, branch := range p.Preflight.BranchRemotes {
		next := "<unset>"
		if p.NewName != "" {
			next = p.NewName
		}
		lines = append(lines, "config branch."+branch+".remote: "+p.Preflight.Remote+" -> "+next)
	}
	for _, relationship := range p.Preflight.BranchMerges {
		lines = append(lines, "upstream merge relationship: "+relationship)
	}
	for _, config := range p.Preflight.RemoteConfig {
		lines = append(lines, "remote config: "+strings.Replace(config, "\x00", " = ", 1))
	}
	sort.Strings(lines[1:])
	return lines
}
