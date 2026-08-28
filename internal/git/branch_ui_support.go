package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ConfigUpdateAction string

const (
	ConfigKeep  ConfigUpdateAction = "keep"
	ConfigSet   ConfigUpdateAction = "set"
	ConfigUnset ConfigUpdateAction = "unset"
)

type ConfigUpdate struct {
	Action ConfigUpdateAction
	Value  string
}

type BranchConfigSnapshot struct {
	OID                             string
	Configuration                   BranchConfiguration
	UpstreamRemote, UpstreamMerge   ConfiguredValue
	PullRebase, RemotePushDefault   ConfiguredValue
	AutoSetupMerge, AutoSetupRebase ConfiguredValue
}

// BranchConfigUpdate is both the typed request and, after review, the immutable
// execution plan. Before, Plan and Token are populated only by review.
type BranchConfigUpdate struct {
	Branch                                    string
	Description, Upstream, Rebase, PushRemote ConfigUpdate
	PullRebase, RemotePushDefault             ConfigUpdate
	AutoSetupMerge, AutoSetupRebase           ConfigUpdate
	Before                                    BranchConfigSnapshot
	Plan                                      []string
	Token                                     ConfirmationToken
}

func (r *Repository) ReviewBranchConfigUpdate(ctx context.Context, request BranchConfigUpdate) (BranchConfigUpdate, error) {
	before, err := r.branchConfigSnapshot(ctx, request.Branch)
	if err != nil {
		return BranchConfigUpdate{}, err
	}
	request.Before = before
	for name, update := range map[string]ConfigUpdate{"description": request.Description, "upstream": request.Upstream, "rebase": request.Rebase, "pushRemote": request.PushRemote, "pull.rebase": request.PullRebase, "remote.pushDefault": request.RemotePushDefault, "branch.autoSetupMerge": request.AutoSetupMerge, "branch.autoSetupRebase": request.AutoSetupRebase} {
		if update.Action != ConfigKeep && update.Action != ConfigSet && update.Action != ConfigUnset {
			return BranchConfigUpdate{}, fmt.Errorf("invalid %s action", name)
		}
	}
	if request.Rebase.Action == ConfigSet && !validRebaseMode(RebaseMode(request.Rebase.Value)) {
		return BranchConfigUpdate{}, fmt.Errorf("invalid branch rebase mode %q", request.Rebase.Value)
	}
	if request.PullRebase.Action == ConfigSet && !validRebaseMode(RebaseMode(request.PullRebase.Value)) {
		return BranchConfigUpdate{}, fmt.Errorf("invalid pull rebase mode %q", request.PullRebase.Value)
	}
	if request.AutoSetupMerge.Action == ConfigSet && !validAutoSetupMerge(request.AutoSetupMerge.Value) {
		return BranchConfigUpdate{}, fmt.Errorf("invalid branch.autoSetupMerge mode %q", request.AutoSetupMerge.Value)
	}
	if request.AutoSetupRebase.Action == ConfigSet && !validAutoSetupRebase(request.AutoSetupRebase.Value) {
		return BranchConfigUpdate{}, fmt.Errorf("invalid branch.autoSetupRebase mode %q", request.AutoSetupRebase.Value)
	}
	if request.PushRemote.Action == ConfigSet {
		if err := r.validateRemote(ctx, request.PushRemote.Value); err != nil {
			return BranchConfigUpdate{}, err
		}
	}
	if request.RemotePushDefault.Action == ConfigSet {
		if err := r.validateRemote(ctx, request.RemotePushDefault.Value); err != nil {
			return BranchConfigUpdate{}, err
		}
	}
	if request.Upstream.Action == ConfigSet {
		out, err := r.output(ctx, "rev-parse", "--verify", "--symbolic-full-name", "--end-of-options", request.Upstream.Value)
		if err != nil {
			return BranchConfigUpdate{}, fmt.Errorf("resolve upstream %q: %w", request.Upstream.Value, err)
		}
		request.Upstream.Value = trimLine(out)
	}
	request.Plan = branchConfigPlan(request)
	request.Token = NewConfirmationToken(branchConfigIdentity(request))
	return request, nil
}

func (r *Repository) ExecuteBranchConfigUpdate(ctx context.Context, reviewed BranchConfigUpdate) error {
	current, err := r.branchConfigSnapshot(ctx, reviewed.Branch)
	if err != nil {
		return err
	}
	check := reviewed
	check.Before = current
	check.Token = ConfirmationToken{}
	if !reviewed.Token.validFor(branchConfigIdentity(check)) {
		return ErrStalePlan
	}
	configPath, before, existed, err := r.snapshotConfigFile(ctx)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		if err := restoreConfigFile(configPath, before, existed); err != nil {
			return &PartialMutationError{Operation: "configure branch", Cause: cause, Rollback: err, State: []string{"repository config may contain a subset of reviewed changes"}}
		}
		return cause
	}
	apply := func(update ConfigUpdate, set func(string) error, unset func() error) error {
		switch update.Action {
		case ConfigKeep:
			return nil
		case ConfigSet:
			return set(update.Value)
		case ConfigUnset:
			return unset()
		default:
			return errors.New("invalid config update")
		}
	}
	if err = apply(reviewed.Description, func(v string) error { return r.SetBranchDescription(ctx, reviewed.Branch, v) }, func() error { return r.UnsetBranchDescription(ctx, reviewed.Branch) }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.Upstream, func(v string) error { return r.SetBranchUpstream(ctx, reviewed.Branch, v) }, func() error { return r.UnsetBranchUpstream(ctx, reviewed.Branch) }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.Rebase, func(v string) error { return r.SetBranchRebase(ctx, reviewed.Branch, RebaseMode(v)) }, func() error { return r.UnsetBranchRebase(ctx, reviewed.Branch) }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.PushRemote, func(v string) error { return r.SetBranchPushRemote(ctx, reviewed.Branch, v) }, func() error { return r.UnsetBranchPushRemote(ctx, reviewed.Branch) }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.PullRebase, func(v string) error { return r.SetPullRebase(ctx, RebaseMode(v)) }, func() error { return r.UnsetPullRebase(ctx) }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.RemotePushDefault, func(v string) error { return r.SetRemotePushDefault(ctx, v) }, func() error { return r.UnsetRemotePushDefault(ctx) }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.AutoSetupMerge, func(v string) error { return r.run(ctx, "config", "branch.autoSetupMerge", v) }, func() error { return r.unsetConfig(ctx, "branch.autoSetupMerge") }); err != nil {
		return rollback(err)
	}
	if err = apply(reviewed.AutoSetupRebase, func(v string) error { return r.run(ctx, "config", "branch.autoSetupRebase", v) }, func() error { return r.unsetConfig(ctx, "branch.autoSetupRebase") }); err != nil {
		return rollback(err)
	}
	return nil
}

func (r *Repository) branchConfigSnapshot(ctx context.Context, branch string) (BranchConfigSnapshot, error) {
	var s BranchConfigSnapshot
	var err error
	if s.OID, err = r.localBranchOID(ctx, branch); err != nil {
		return s, err
	}
	if s.Configuration, err = r.BranchConfiguration(ctx, branch); err != nil {
		return s, err
	}
	if s.UpstreamRemote, err = r.workflowConfigValue(ctx, "branch."+branch+".remote"); err != nil {
		return s, err
	}
	if s.UpstreamMerge, err = r.workflowConfigValue(ctx, "branch."+branch+".merge"); err != nil {
		return s, err
	}
	if s.PullRebase, err = r.PullRebase(ctx); err != nil {
		return s, err
	}
	if s.RemotePushDefault, err = r.RemotePushDefault(ctx); err != nil {
		return s, err
	}
	if s.AutoSetupMerge, err = r.workflowConfigValue(ctx, "branch.autoSetupMerge"); err != nil {
		return s, err
	}
	s.AutoSetupRebase, err = r.workflowConfigValue(ctx, "branch.autoSetupRebase")
	return s, err
}

func branchConfigIdentity(update BranchConfigUpdate) string {
	b, _ := json.Marshal(struct {
		Branch                                                                                                    string
		Description, Upstream, Rebase, PushRemote, PullRebase, RemotePushDefault, AutoSetupMerge, AutoSetupRebase ConfigUpdate
		Before                                                                                                    BranchConfigSnapshot
	}{update.Branch, update.Description, update.Upstream, update.Rebase, update.PushRemote, update.PullRebase, update.RemotePushDefault, update.AutoSetupMerge, update.AutoSetupRebase, update.Before})
	return string(b)
}

func branchConfigPlan(update BranchConfigUpdate) []string {
	var out []string
	add := func(key string, old ConfiguredValue, next ConfigUpdate) {
		if next.Action == ConfigKeep {
			return
		}
		value := "<unset>"
		if old.Set {
			value = old.Value
		}
		target := "<unset>"
		if next.Action == ConfigSet {
			target = next.Value
		}
		out = append(out, key+": "+value+" -> "+target)
	}
	add("branch."+update.Branch+".description", update.Before.Configuration.Description, update.Description)
	oldUpstream := ConfiguredValue{}
	if update.Before.UpstreamRemote.Set && update.Before.UpstreamMerge.Set {
		oldUpstream = ConfiguredValue{Set: true, Value: update.Before.UpstreamRemote.Value + ":" + update.Before.UpstreamMerge.Value}
	}
	add("branch."+update.Branch+" upstream", oldUpstream, update.Upstream)
	add("branch."+update.Branch+".rebase", update.Before.Configuration.Rebase, update.Rebase)
	add("branch."+update.Branch+".pushRemote", update.Before.Configuration.PushRemote, update.PushRemote)
	add("pull.rebase", update.Before.PullRebase, update.PullRebase)
	add("remote.pushDefault", update.Before.RemotePushDefault, update.RemotePushDefault)
	add("branch.autoSetupMerge", update.Before.AutoSetupMerge, update.AutoSetupMerge)
	add("branch.autoSetupRebase", update.Before.AutoSetupRebase, update.AutoSetupRebase)
	if len(out) == 0 {
		out = append(out, "No configuration changes")
	}
	return out
}

func validAutoSetupMerge(value string) bool {
	switch value {
	case "false", "true", "always", "simple", "inherit":
		return true
	default:
		return false
	}
}

func validAutoSetupRebase(value string) bool {
	switch value {
	case "never", "local", "remote", "always":
		return true
	default:
		return false
	}
}

func (r *Repository) snapshotConfigFile(ctx context.Context) (string, []byte, bool, error) {
	out, err := r.output(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", nil, false, err
	}
	dir := trimLine(out)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.commandDir, dir)
	}
	path := filepath.Join(filepath.Clean(dir), "config")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return path, b, true, nil
}

func restoreConfigFile(path string, before []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lazymagit-config-rollback-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(before); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}

// AddWorktreeWithNewBranch is the safe UI contract for Magit's branch W
// occurrence. It validates the new branch and resolves the start point before
// delegating destination validation and no-force worktree creation to the
// repository-management contract.
func (r *Repository) AddWorktreeWithNewBranch(ctx context.Context, path, branch, startPoint string) error {
	if err := r.validateBranchName(ctx, branch); err != nil {
		return err
	}
	if startPoint == "" {
		startPoint = "HEAD"
	}
	oid, err := r.resolveBranchCommit(ctx, startPoint)
	if err != nil {
		return err
	}
	return r.AddWorktreeWithBranch(ctx, path, branch, oid, NotConfirmed)
}
