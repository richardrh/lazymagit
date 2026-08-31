package git

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type RemoteChangePreflight struct {
	Remote                string
	TrackingRefs          []string
	UsesRemotePushDefault bool
	BranchPushRemotes     []string
	BranchRemotes         []string
	BranchMerges          []string
	RemoteConfig          []string
	TrackingRefOIDs       []string
	TrackingRefSymbols    []string
	RequiresConfirmation  bool
}

func (r *Repository) RemoteChangePreflight(ctx context.Context, remote string) (RemoteChangePreflight, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return RemoteChangePreflight{}, err
	}
	p := RemoteChangePreflight{Remote: remote, RequiresConfirmation: true}
	if err := r.loadRemotePushUses(ctx, &p); err != nil {
		return p, err
	}
	if err := r.loadRemoteBranchUses(ctx, &p); err != nil {
		return p, err
	}
	if err := r.loadRemoteConfigurationSnapshot(ctx, &p); err != nil {
		return p, err
	}
	if err := r.loadRemoteTrackingRefs(ctx, &p); err != nil {
		return p, err
	}
	sortRemotePreflight(&p)
	return p, nil
}

func (r *Repository) loadRemotePushUses(ctx context.Context, p *RemoteChangePreflight) error {
	value, ok, err := r.configValue(ctx, "remote.pushDefault")
	if err != nil {
		return err
	}
	p.UsesRemotePushDefault = ok && value == p.Remote
	keys, err := r.branchPushRemoteKeys(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if value, ok, err := r.configValue(ctx, key); err != nil {
			return err
		} else if ok && value == p.Remote {
			name := strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".pushRemote")
			p.BranchPushRemotes = append(p.BranchPushRemotes, name)
		}
	}
	return nil
}

func (r *Repository) loadRemoteBranchUses(ctx context.Context, p *RemoteChangePreflight) error {
	branchKeys, err := r.configValuesMatching(ctx, `^branch\..*\.(remote|merge)$`)
	if err != nil {
		return err
	}
	for _, entry := range branchKeys {
		key, value, _ := strings.Cut(entry, "\x00")
		if strings.HasSuffix(key, ".remote") && value == p.Remote {
			p.BranchRemotes = append(p.BranchRemotes, strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".remote"))
		}
	}
	p.BranchMerges = linkedBranchMerges(branchKeys, p.BranchRemotes)
	return nil
}

func linkedBranchMerges(entries, linked []string) []string {
	set := make(map[string]bool, len(linked))
	for _, branch := range linked {
		set[branch] = true
	}
	var merges []string
	for _, entry := range entries {
		key, value, _ := strings.Cut(entry, "\x00")
		branch := strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".merge")
		if strings.HasSuffix(key, ".merge") && set[branch] {
			merges = append(merges, branch+"="+value)
		}
	}
	return merges
}

func (r *Repository) loadRemoteConfigurationSnapshot(ctx context.Context, p *RemoteChangePreflight) error {
	values, err := r.configValuesMatching(ctx, `^remote\.`+regexp.QuoteMeta(p.Remote)+`\.`)
	p.RemoteConfig = values
	return err
}

func (r *Repository) loadRemoteTrackingRefs(ctx context.Context, p *RemoteChangePreflight) error {
	out, err := r.output(ctx, "for-each-ref", "--format=%(refname)", "refs/remotes/"+p.Remote)
	if err != nil {
		return err
	}
	for _, ref := range strings.Split(trimLine(out), "\n") {
		if ref == "" {
			continue
		}
		if err := r.loadRemoteTrackingRef(ctx, p, ref); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) loadRemoteTrackingRef(ctx context.Context, p *RemoteChangePreflight, ref string) error {
	p.TrackingRefs = append(p.TrackingRefs, ref)
	oid, err := r.output(ctx, "rev-parse", "--verify", ref)
	if err != nil {
		return err
	}
	p.TrackingRefOIDs = append(p.TrackingRefOIDs, ref+"="+trimLine(oid))
	target, err := r.output(ctx, "symbolic-ref", "-q", ref)
	if err == nil {
		p.TrackingRefSymbols = append(p.TrackingRefSymbols, ref+"="+trimLine(target))
		return nil
	}
	if commandExitCode(err) != 1 {
		return err
	}
	return nil
}

func sortRemotePreflight(p *RemoteChangePreflight) {
	sort.Strings(p.BranchPushRemotes)
	sort.Strings(p.BranchRemotes)
	sort.Strings(p.BranchMerges)
	sort.Strings(p.RemoteConfig)
	sort.Strings(p.TrackingRefOIDs)
	sort.Strings(p.TrackingRefSymbols)
}

func (r *Repository) configValuesMatching(ctx context.Context, pattern string) ([]string, error) {
	out, err := r.output(ctx, "config", "--get-regexp", pattern)
	if err != nil {
		if commandExitCode(err) == 1 {
			return nil, nil
		}
		return nil, err
	}
	var values []string
	for _, line := range strings.Split(trimLine(out), "\n") {
		if key, value, ok := strings.Cut(line, " "); ok {
			values = append(values, key+"\x00"+value)
		}
	}
	sort.Strings(values)
	return values, nil
}

type RenameRemoteArgs struct{ Old, New string }

func (r *Repository) RenameRemote(ctx context.Context, in RenameRemoteArgs) (RemoteChangePreflight, error) {
	p, err := r.RemoteChangePreflight(ctx, in.Old)
	if err != nil {
		return p, err
	}
	if err := r.validateNewRemoteName(ctx, in.New); err != nil {
		return p, err
	}
	configPath, configBefore, configExisted, err := r.snapshotConfigFile(ctx)
	if err != nil {
		return p, err
	}
	rollback := func(cause error) error {
		return r.rollbackRemoteChange(ctx, cause, configPath, configBefore, configExisted, p, in.New)
	}
	if err := r.run(ctx, "remote", "rename", "--", in.Old, in.New); err != nil {
		return p, rollback(err)
	}
	// `remote rename` updates fetch refspecs and branch.*.remote, but Git does
	// not consistently migrate push-only selection across versions.
	if p.UsesRemotePushDefault {
		if err := r.run(ctx, "config", "remote.pushDefault", in.New); err != nil {
			return p, rollback(err)
		}
	}
	for _, branch := range p.BranchPushRemotes {
		if err := r.run(ctx, "config", "branch."+branch+".pushRemote", in.New); err != nil {
			return p, rollback(err)
		}
	}
	return p, nil
}

type RemoveRemoteArgs struct {
	Remote  string
	Confirm bool
}

func (r *Repository) RemoveRemoteWithArgs(ctx context.Context, in RemoveRemoteArgs) (RemoteChangePreflight, error) {
	p, err := r.RemoteChangePreflight(ctx, in.Remote)
	if err != nil {
		return p, err
	}
	if !in.Confirm {
		return p, errors.New("remote removal requires confirmation")
	}
	configPath, configBefore, configExisted, err := r.snapshotConfigFile(ctx)
	if err != nil {
		return p, err
	}
	rollback := func(cause error) error {
		return r.rollbackRemoteChange(ctx, cause, configPath, configBefore, configExisted, p, "")
	}
	if err := r.run(ctx, "remote", "remove", "--", in.Remote); err != nil {
		return p, rollback(err)
	}
	if err := r.removeResidualPushRemoteConfig(ctx, p); err != nil {
		return p, rollback(err)
	}
	return p, nil
}

func (r *Repository) removeResidualPushRemoteConfig(ctx context.Context, p RemoteChangePreflight) error {
	var keys []string
	if p.UsesRemotePushDefault {
		keys = append(keys, "remote.pushDefault")
	}
	for _, branch := range p.BranchPushRemotes {
		keys = append(keys, "branch."+branch+".pushRemote")
	}
	for _, key := range keys {
		_, set, err := r.configValue(ctx, key)
		if err != nil {
			return err
		}
		if set {
			if err := r.run(ctx, "config", "--unset-all", key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) rollbackRemoteChange(ctx context.Context, cause error, configPath string, configBefore []byte, configExisted bool, before RemoteChangePreflight, newName string) error {
	rollbackErr := restoreConfigFile(configPath, configBefore, configExisted)
	rollbackErr = firstRollbackError(rollbackErr, r.removeRemoteTrackingRefs(ctx, before.Remote))
	rollbackErr = firstRollbackError(rollbackErr, r.removeRemoteTrackingRefs(ctx, newName))
	rollbackErr = firstRollbackError(rollbackErr, r.restoreDirectTrackingRefs(ctx, before))
	rollbackErr = firstRollbackError(rollbackErr, r.restoreSymbolicTrackingRefs(ctx, before.TrackingRefSymbols))
	if rollbackErr != nil {
		return &PartialMutationError{Operation: "remote change", Cause: cause, Rollback: rollbackErr, State: []string{"remote/config/ref rollback incomplete"}}
	}
	return cause
}

func firstRollbackError(current, candidate error) error {
	if current != nil {
		return current
	}
	return candidate
}

func (r *Repository) removeRemoteTrackingRefs(ctx context.Context, remote string) error {
	if remote == "" {
		return nil
	}
	out, err := r.output(ctx, "for-each-ref", "--format=%(refname)", "refs/remotes/"+remote)
	if err != nil {
		return err
	}
	var rollbackErr error
	for _, ref := range strings.Split(trimLine(out), "\n") {
		if ref != "" {
			rollbackErr = firstRollbackError(rollbackErr, r.run(ctx, "update-ref", "-d", ref))
		}
	}
	return rollbackErr
}

func (r *Repository) restoreDirectTrackingRefs(ctx context.Context, before RemoteChangePreflight) error {
	symbols := trackingRefSymbols(before.TrackingRefSymbols)
	var rollbackErr error
	for _, pair := range before.TrackingRefOIDs {
		ref, oid, ok := strings.Cut(pair, "=")
		if ok && !symbols[ref] {
			rollbackErr = firstRollbackError(rollbackErr, r.run(ctx, "update-ref", ref, oid))
		}
	}
	return rollbackErr
}

func trackingRefSymbols(pairs []string) map[string]bool {
	symbols := make(map[string]bool, len(pairs))
	for _, pair := range pairs {
		ref, _, _ := strings.Cut(pair, "=")
		symbols[ref] = true
	}
	return symbols
}

func (r *Repository) restoreSymbolicTrackingRefs(ctx context.Context, pairs []string) error {
	var rollbackErr error
	for _, pair := range pairs {
		ref, target, ok := strings.Cut(pair, "=")
		if ok {
			rollbackErr = firstRollbackError(rollbackErr, r.run(ctx, "symbolic-ref", ref, target))
		}
	}
	return rollbackErr
}

func (r *Repository) branchPushRemoteKeys(ctx context.Context) ([]string, error) {
	out, err := r.output(ctx, "config", "--name-only", "--get-regexp", `^branch\..*\.pushRemote$`)
	if err != nil {
		if commandExitCode(err) == 1 {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, key := range strings.Split(trimLine(out), "\n") {
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (r *Repository) validateNewRemoteName(ctx context.Context, name string) error {
	if err := validateToken("remote", name); err != nil {
		return err
	}
	if strings.ContainsAny(name, " \t~^:?*[\\") || strings.Contains(name, "..") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("invalid remote %q", name)
	}
	out, err := r.output(ctx, "remote")
	if err != nil {
		return err
	}
	for _, existing := range strings.Split(trimLine(out), "\n") {
		if existing == name {
			return fmt.Errorf("remote %q already exists", name)
		}
	}
	return nil
}

type RemoteTagOpt uint8

const (
	RemoteTagsDefault RemoteTagOpt = iota
	RemoteTagsAll
	RemoteTagsNone
)

// RemoteFollowRemoteHEAD controls whether fetch updates the remote HEAD
// symbolic ref. The default leaves Git's configuration unset.
type RemoteFollowRemoteHEAD uint8

const (
	RemoteFollowRemoteHEADDefault RemoteFollowRemoteHEAD = iota
	RemoteFollowRemoteHEADNever
	RemoteFollowRemoteHEADCreate
	RemoteFollowRemoteHEADWarn
	RemoteFollowRemoteHEADAlways
)

func (o RemoteFollowRemoteHEAD) String() string {
	switch o {
	case RemoteFollowRemoteHEADDefault:
		return "default"
	case RemoteFollowRemoteHEADNever:
		return "never"
	case RemoteFollowRemoteHEADCreate:
		return "create"
	case RemoteFollowRemoteHEADWarn:
		return "warn"
	case RemoteFollowRemoteHEADAlways:
		return "always"
	default:
		return "invalid"
	}
}

// RemoteConfigArgs replaces the selected settings. Nil slices leave a setting
// unchanged; an empty non-nil slice removes all values. FetchURL and PushURL
// use pointers for the same reason.
type RemoteConfigArgs struct {
	Remote           string
	FetchURL         *string
	PushURL          *string
	FetchRefspecs    []string
	PushRefspecs     []string
	TagOpt           *RemoteTagOpt
	FollowRemoteHEAD *RemoteFollowRemoteHEAD
}

func (r *Repository) ConfigureRemote(ctx context.Context, in RemoteConfigArgs) error {
	if err := r.validateRemoteConfigArgs(ctx, in); err != nil {
		return err
	}
	configPath, before, configExisted, err := r.snapshotConfigFile(ctx)
	if err != nil {
		return fmt.Errorf("snapshot repository config: %w", err)
	}
	rollback := func(cause error) error {
		if err := restoreConfigFile(configPath, before, configExisted); err != nil {
			return fmt.Errorf("%w (config rollback failed: %v)", cause, err)
		}
		return cause
	}
	return r.applyRemoteConfig(ctx, in, rollback)
}

func (r *Repository) validateRemoteConfigArgs(ctx context.Context, in RemoteConfigArgs) error {
	if err := r.validateTransferRemote(ctx, in.Remote); err != nil {
		return err
	}
	for name, value := range map[string]*string{"fetch": in.FetchURL, "push": in.PushURL} {
		if value != nil && (*value == "" || strings.ContainsAny(*value, "\x00\r\n")) {
			return fmt.Errorf("%s URL is empty or contains a control character", name)
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

func (r *Repository) applyRemoteConfig(ctx context.Context, in RemoteConfigArgs, rollback func(error) error) error {
	if in.FetchURL != nil {
		if err := r.run(ctx, "remote", "set-url", "--", in.Remote, *in.FetchURL); err != nil {
			return rollback(err)
		}
	}
	if in.PushURL != nil {
		if err := r.run(ctx, "remote", "set-url", "--push", "--", in.Remote, *in.PushURL); err != nil {
			return rollback(err)
		}
	}
	if err := r.replaceRemoteConfigValues(ctx, "remote."+in.Remote+".fetch", in.FetchRefspecs); err != nil {
		return rollback(err)
	}
	if err := r.replaceRemoteConfigValues(ctx, "remote."+in.Remote+".push", in.PushRefspecs); err != nil {
		return rollback(err)
	}
	if err := r.configureRemoteTagOpt(ctx, in.Remote, in.TagOpt); err != nil {
		return rollback(err)
	}
	return r.configureRemoteFollowRemoteHEAD(ctx, in.Remote, in.FollowRemoteHEAD, rollback)
}

func (r *Repository) replaceRemoteConfigValues(ctx context.Context, key string, values []string) error {
	if values == nil {
		return nil
	}
	// A regexp matching every value avoids treating a refspec as a regexp.
	if err := r.run(ctx, "config", "--unset-all", key, `.*`); err != nil && commandExitCode(err) != 5 {
		return err
	}
	for _, value := range values {
		if err := r.run(ctx, "config", "--add", key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) configureRemoteTagOpt(ctx context.Context, remote string, option *RemoteTagOpt) error {
	if option == nil {
		return nil
	}
	key := "remote." + remote + ".tagOpt"
	switch *option {
	case RemoteTagsDefault:
		if err := r.run(ctx, "config", "--unset-all", key); err != nil && commandExitCode(err) != 5 {
			return err
		}
		return nil
	case RemoteTagsAll:
		return r.run(ctx, "config", key, "--tags")
	case RemoteTagsNone:
		return r.run(ctx, "config", key, "--no-tags")
	default:
		return errors.New("invalid remote tag option")
	}
}

func (r *Repository) configureRemoteFollowRemoteHEAD(ctx context.Context, remote string, follow *RemoteFollowRemoteHEAD, rollback func(error) error) error {
	if follow == nil {
		return nil
	}
	key := "remote." + remote + ".followRemoteHEAD"
	if *follow == RemoteFollowRemoteHEADDefault {
		if err := r.run(ctx, "config", "--unset-all", key); err != nil && commandExitCode(err) != 5 {
			return rollback(err)
		}
		return nil
	}
	values := map[RemoteFollowRemoteHEAD]string{RemoteFollowRemoteHEADNever: "never", RemoteFollowRemoteHEADCreate: "create", RemoteFollowRemoteHEADWarn: "warn", RemoteFollowRemoteHEADAlways: "always"}
	if err := r.run(ctx, "config", key, values[*follow]); err != nil {
		return rollback(err)
	}
	return nil
}

type RemotePrunePlan struct {
	Remote               string
	StaleTrackingRefs    []string
	RequiresConfirmation bool
	Token                ConfirmationToken
}

// RemotePruneComparison delegates planning to Git so configured fetch
// refspecs, negative refspecs, and custom destinations are interpreted exactly
// as they will be by execution.
func (r *Repository) RemotePruneComparison(ctx context.Context, remote string) (RemotePrunePlan, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return RemotePrunePlan{}, err
	}
	out, err := r.output(ctx, "remote", "prune", "--dry-run", "--", remote)
	if err != nil {
		return RemotePrunePlan{}, err
	}
	plan := RemotePrunePlan{Remote: remote}
	for _, line := range strings.Split(string(out), "\n") {
		if at := strings.Index(line, "[would prune]"); at >= 0 {
			name := strings.TrimSpace(line[at+len("[would prune]"):])
			if name != "" {
				plan.StaleTrackingRefs = append(plan.StaleTrackingRefs, name)
			}
		}
	}
	sort.Strings(plan.StaleTrackingRefs)
	plan.RequiresConfirmation = len(plan.StaleTrackingRefs) != 0
	plan.Token = NewConfirmationToken(remote + "\x00" + strings.Join(plan.StaleTrackingRefs, "\x00"))
	return plan, nil
}

func (r *Repository) PruneRemote(ctx context.Context, reviewed RemotePrunePlan, token ConfirmationToken) (RemotePrunePlan, error) {
	plan, err := r.RemotePruneComparison(ctx, reviewed.Remote)
	if err != nil {
		return plan, err
	}
	identity := plan.Remote + "\x00" + strings.Join(plan.StaleTrackingRefs, "\x00")
	if !token.validFor(identity) || !sameStrings(plan.StaleTrackingRefs, reviewed.StaleTrackingRefs) {
		return plan, ErrStalePlan
	}
	if len(plan.StaleTrackingRefs) == 0 {
		return plan, nil
	}
	return plan, r.run(ctx, "remote", "prune", "--", plan.Remote)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
