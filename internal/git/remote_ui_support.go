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
	if a.Remote != b.Remote || a.UsesRemotePushDefault != b.UsesRemotePushDefault {
		return false
	}
	return sameRemotePreflightLists(a, b)
}

func sameRemotePreflightLists(a, b RemoteChangePreflight) bool {
	aLists := [][]string{a.TrackingRefs, a.TrackingRefOIDs, a.TrackingRefSymbols, a.BranchPushRemotes, a.BranchRemotes, a.BranchMerges, a.RemoteConfig}
	bLists := [][]string{b.TrackingRefs, b.TrackingRefOIDs, b.TrackingRefSymbols, b.BranchPushRemotes, b.BranchRemotes, b.BranchMerges, b.RemoteConfig}
	for i := range aLists {
		if !sameStrings(aLists[i], bLists[i]) {
			return false
		}
	}
	return true
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
	if err := r.loadRemoteURLs(ctx, remote, &c); err != nil {
		return c, err
	}
	if err := r.loadRemoteRefspecs(ctx, remote, &c); err != nil {
		return c, err
	}
	if err := r.loadRemoteOptions(ctx, remote, &c); err != nil {
		return c, err
	}
	return c, nil
}

func (r *Repository) loadRemoteURLs(ctx context.Context, remote string, c *RemoteConfiguration) error {
	v, ok, err := r.configValue(ctx, "remote."+remote+".url")
	if err != nil {
		return err
	}
	if ok {
		c.FetchURL = &v
	}
	values, err := r.configValues(ctx, "remote."+remote+".pushurl")
	if err != nil {
		return err
	}
	if len(values) != 0 {
		v := values[0]
		c.PushURL = &v
		c.PushURLConfigured = true
		return nil
	}
	if c.FetchURL != nil {
		v := *c.FetchURL
		c.PushURL = &v
	}
	return nil
}

func (r *Repository) loadRemoteRefspecs(ctx context.Context, remote string, c *RemoteConfiguration) error {
	var err error
	c.FetchRefspecs, err = r.configValues(ctx, "remote."+remote+".fetch")
	if err != nil {
		return err
	}
	c.PushRefspecs, err = r.configValues(ctx, "remote."+remote+".push")
	return err
}

func (r *Repository) loadRemoteOptions(ctx context.Context, remote string, c *RemoteConfiguration) error {
	value, ok, err := r.configValue(ctx, "remote."+remote+".tagopt")
	if err != nil {
		return err
	}
	if ok {
		c.TagOpt, err = parseRemoteTagOpt(value)
		if err != nil {
			return err
		}
	}
	value, ok, err = r.configValue(ctx, "remote."+remote+".followRemoteHEAD")
	if err != nil {
		return err
	}
	if ok {
		c.FollowRemoteHEAD, err = parseRemoteFollowRemoteHEAD(value)
	}
	return err
}

func parseRemoteTagOpt(value string) (*RemoteTagOpt, error) {
	options := map[string]RemoteTagOpt{"--tags": RemoteTagsAll, "--no-tags": RemoteTagsNone}
	option, ok := options[value]
	if !ok {
		return nil, fmt.Errorf("unsupported remote tag option %q", value)
	}
	return &option, nil
}

func parseRemoteFollowRemoteHEAD(value string) (*RemoteFollowRemoteHEAD, error) {
	modes := map[string]RemoteFollowRemoteHEAD{"never": RemoteFollowRemoteHEADNever, "create": RemoteFollowRemoteHEADCreate, "warn": RemoteFollowRemoteHEADWarn, "always": RemoteFollowRemoteHEADAlways}
	mode, ok := modes[value]
	if !ok {
		return nil, fmt.Errorf("unsupported remote followRemoteHEAD option %q", value)
	}
	return &mode, nil
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
	if err := validateRemoteURLs(in); err != nil {
		return err
	}
	if err := r.validateRemoteRefspecs(ctx, in); err != nil {
		return err
	}
	return validateRemoteOptions(in)
}

func validateRemoteURLs(in RemoteConfigArgs) error {
	for label, value := range map[string]*string{"fetch": in.FetchURL, "push": in.PushURL} {
		if value != nil && (*value == "" || strings.ContainsAny(*value, "\x00\r\n")) {
			return fmt.Errorf("%s URL is empty or contains a control character", label)
		}
	}
	return nil
}

func (r *Repository) validateRemoteRefspecs(ctx context.Context, in RemoteConfigArgs) error {
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
	return nil
}

func validateRemoteOptions(in RemoteConfigArgs) error {
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
	for _, change := range remoteConfigurationChangeValues(before, in) {
		if change.old != change.next {
			out = append(out, change.key+": "+change.old+" -> "+change.next)
		}
	}
	if len(out) == 0 {
		out = append(out, "No configuration changes")
	}
	return out
}

type remoteConfigurationChange struct{ key, old, next string }

func remoteConfigurationChangeValues(before RemoteConfiguration, in RemoteConfigArgs) []remoteConfigurationChange {
	prefix := "remote." + in.Remote + "."
	var changes []remoteConfigurationChange
	if in.FetchURL != nil {
		changes = append(changes, remoteConfigurationChange{prefix + "url", stringPointerValue(before.FetchURL), *in.FetchURL})
	}
	if in.PushURL != nil {
		old := stringPointerValue(before.PushURL)
		if !before.PushURLConfigured {
			old += " (inherited)"
		}
		changes = append(changes, remoteConfigurationChange{prefix + "pushurl", old, *in.PushURL})
	}
	if in.FetchRefspecs != nil {
		changes = append(changes, remoteConfigurationChange{prefix + "fetch", strings.Join(before.FetchRefspecs, ", "), strings.Join(in.FetchRefspecs, ", ")})
	}
	if in.PushRefspecs != nil {
		changes = append(changes, remoteConfigurationChange{prefix + "push", strings.Join(before.PushRefspecs, ", "), strings.Join(in.PushRefspecs, ", ")})
	}
	if in.TagOpt != nil {
		changes = append(changes, remoteConfigurationChange{prefix + "tagopt", printablePointer(before.TagOpt), fmt.Sprint(*in.TagOpt)})
	}
	if in.FollowRemoteHEAD != nil {
		changes = append(changes, remoteConfigurationChange{prefix + "followRemoteHEAD", printablePointer(before.FollowRemoteHEAD), fmt.Sprint(*in.FollowRemoteHEAD)})
	}
	return changes
}

func stringPointerValue(value *string) string {
	if value == nil {
		return "<unset>"
	}
	return *value
}
func printablePointer[T any](value *T) string {
	if value == nil {
		return "<unset>"
	}
	return fmt.Sprint(*value)
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
