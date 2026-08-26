package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if value, ok, err := r.configValue(ctx, "remote.pushDefault"); err != nil {
		return p, err
	} else {
		p.UsesRemotePushDefault = ok && value == remote
	}
	keys, err := r.branchPushRemoteKeys(ctx)
	if err != nil {
		return p, err
	}
	for _, key := range keys {
		if value, ok, err := r.configValue(ctx, key); err != nil {
			return p, err
		} else if ok && value == remote {
			name := strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".pushRemote")
			p.BranchPushRemotes = append(p.BranchPushRemotes, name)
		}
	}
	branchKeys, err := r.configValuesMatching(ctx, `^branch\..*\.(remote|merge)$`)
	if err != nil {
		return p, err
	}
	for _, entry := range branchKeys {
		key, value, _ := strings.Cut(entry, "\x00")
		if strings.HasSuffix(key, ".remote") && value == remote {
			p.BranchRemotes = append(p.BranchRemotes, strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".remote"))
		}
	}
	for _, entry := range branchKeys {
		key, value, _ := strings.Cut(entry, "\x00")
		if !strings.HasSuffix(key, ".merge") {
			continue
		}
		branch := strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".merge")
		for _, linked := range p.BranchRemotes {
			if linked == branch {
				p.BranchMerges = append(p.BranchMerges, branch+"="+value)
			}
		}
	}
	p.RemoteConfig, err = r.configValuesMatching(ctx, `^remote\.`+regexp.QuoteMeta(remote)+`\.`)
	if err != nil {
		return p, err
	}
	out, err := r.output(ctx, "for-each-ref", "--format=%(refname)", "refs/remotes/"+remote)
	if err != nil {
		return p, err
	}
	for _, ref := range strings.Split(trimLine(out), "\n") {
		if ref != "" {
			p.TrackingRefs = append(p.TrackingRefs, ref)
			oid, resolveErr := r.output(ctx, "rev-parse", "--verify", ref)
			if resolveErr != nil {
				return p, resolveErr
			}
			p.TrackingRefOIDs = append(p.TrackingRefOIDs, ref+"="+trimLine(oid))
			if target, symbolErr := r.output(ctx, "symbolic-ref", "-q", ref); symbolErr == nil {
				p.TrackingRefSymbols = append(p.TrackingRefSymbols, ref+"="+trimLine(target))
			} else if commandExitCode(symbolErr) != 1 {
				return p, symbolErr
			}
		}
	}
	sort.Strings(p.BranchPushRemotes)
	sort.Strings(p.BranchRemotes)
	sort.Strings(p.BranchMerges)
	sort.Strings(p.RemoteConfig)
	sort.Strings(p.TrackingRefOIDs)
	sort.Strings(p.TrackingRefSymbols)
	return p, nil
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
	if p.UsesRemotePushDefault {
		_, stillSet, err := r.configValue(ctx, "remote.pushDefault")
		if err != nil {
			return p, rollback(err)
		}
		if stillSet {
			if err := r.run(ctx, "config", "--unset-all", "remote.pushDefault"); err != nil {
				return p, rollback(err)
			}
		}
	}
	for _, branch := range p.BranchPushRemotes {
		key := "branch." + branch + ".pushRemote"
		_, stillSet, err := r.configValue(ctx, key)
		if err != nil {
			return p, rollback(err)
		}
		if stillSet {
			if err := r.run(ctx, "config", "--unset-all", key); err != nil {
				return p, rollback(err)
			}
		}
	}
	return p, nil
}

func (r *Repository) rollbackRemoteChange(ctx context.Context, cause error, configPath string, configBefore []byte, configExisted bool, before RemoteChangePreflight, newName string) error {
	var rollbackErr error
	if err := restoreConfigFile(configPath, configBefore, configExisted); err != nil {
		rollbackErr = err
	}
	for _, remote := range []string{before.Remote, newName} {
		if remote == "" {
			continue
		}
		out, err := r.output(ctx, "for-each-ref", "--format=%(refname)", "refs/remotes/"+remote)
		if err != nil && rollbackErr == nil {
			rollbackErr = err
			continue
		}
		for _, ref := range strings.Split(trimLine(out), "\n") {
			if ref != "" {
				if err := r.run(ctx, "update-ref", "-d", ref); err != nil && rollbackErr == nil {
					rollbackErr = err
				}
			}
		}
	}
	symbols := make(map[string]bool)
	for _, pair := range before.TrackingRefSymbols {
		ref, _, _ := strings.Cut(pair, "=")
		symbols[ref] = true
	}
	for _, pair := range before.TrackingRefOIDs {
		ref, oid, ok := strings.Cut(pair, "=")
		if !ok || symbols[ref] {
			continue
		}
		if err := r.run(ctx, "update-ref", ref, oid); err != nil && rollbackErr == nil {
			rollbackErr = err
		}
	}
	for _, pair := range before.TrackingRefSymbols {
		ref, target, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if err := r.run(ctx, "symbolic-ref", ref, target); err != nil && rollbackErr == nil {
			rollbackErr = err
		}
	}
	if rollbackErr != nil {
		return &PartialMutationError{Operation: "remote change", Cause: cause, Rollback: rollbackErr, State: []string{"remote/config/ref rollback incomplete"}}
	}
	return cause
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

// RemoteConfigArgs replaces the selected settings. Nil slices leave a setting
// unchanged; an empty non-nil slice removes all values. FetchURL and PushURL
// use pointers for the same reason.
type RemoteConfigArgs struct {
	Remote        string
	FetchURL      *string
	PushURL       *string
	FetchRefspecs []string
	PushRefspecs  []string
	TagOpt        *RemoteTagOpt
}

func (r *Repository) ConfigureRemote(ctx context.Context, in RemoteConfigArgs) error {
	if err := r.validateTransferRemote(ctx, in.Remote); err != nil {
		return err
	}
	// Validate the complete request before the first config write.
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
	commonOut, err := r.output(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	commonDir := trimLine(commonOut)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(r.commandDir, commonDir)
	}
	configPath := filepath.Join(filepath.Clean(commonDir), "config")
	before, readErr := os.ReadFile(configPath)
	configExisted := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("snapshot repository config: %w", readErr)
	}
	rollback := func(cause error) error {
		if configExisted {
			tmp, err := os.CreateTemp(filepath.Dir(configPath), ".lazymagit-config-rollback-")
			if err != nil {
				return fmt.Errorf("%w (config rollback failed: %v)", cause, err)
			}
			name := tmp.Name()
			if _, err = tmp.Write(before); err == nil {
				err = tmp.Sync()
			}
			if closeErr := tmp.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(name, configPath)
			}
			_ = os.Remove(name)
			if err != nil {
				return fmt.Errorf("%w (config rollback failed: %v)", cause, err)
			}
		} else if err := os.Remove(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w (config rollback failed: %v)", cause, err)
		}
		return cause
	}
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
	for _, pair := range []struct {
		key    string
		values []string
	}{
		{"remote." + in.Remote + ".fetch", in.FetchRefspecs}, {"remote." + in.Remote + ".push", in.PushRefspecs},
	} {
		if pair.values == nil {
			continue
		}
		// A regexp matching every value avoids treating a refspec as a regexp.
		if err := r.run(ctx, "config", "--unset-all", pair.key, `.*`); err != nil && commandExitCode(err) != 5 {
			return rollback(err)
		}
		for _, spec := range pair.values {
			if err := r.run(ctx, "config", "--add", pair.key, spec); err != nil {
				return rollback(err)
			}
		}
	}
	if in.TagOpt != nil {
		key := "remote." + in.Remote + ".tagOpt"
		switch *in.TagOpt {
		case RemoteTagsDefault:
			if err := r.run(ctx, "config", "--unset-all", key); err != nil && commandExitCode(err) != 5 {
				return rollback(err)
			}
		case RemoteTagsAll:
			if err := r.run(ctx, "config", key, "--tags"); err != nil {
				return rollback(err)
			}
		case RemoteTagsNone:
			if err := r.run(ctx, "config", key, "--no-tags"); err != nil {
				return rollback(err)
			}
		default:
			return errors.New("invalid remote tag option")
		}
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
