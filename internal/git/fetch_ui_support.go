package git

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const fetchChoicesByteLimit = 1 << 20

// FetchRemoteBranch is a configured remote-tracking branch suitable for a
// bounded UI chooser. Branch is the name understood by git fetch (without the
// remote prefix).
type FetchRemoteBranch struct {
	Remote string
	Branch string
}

type FetchUIChoices struct {
	Remotes        []string
	RemoteBranches []FetchRemoteBranch
}

// FetchChoices returns only configured remotes and existing remote-tracking
// branches. The caller supplies a positive bound so repository-controlled ref
// counts cannot create an unbounded dialog.
func (r *Repository) FetchChoices(ctx context.Context, limit int) (FetchUIChoices, error) {
	if limit <= 0 {
		return FetchUIChoices{}, errors.New("fetch choice limit must be positive")
	}
	remoteOutput, truncated, err := r.outputLimited(ctx, fetchChoicesByteLimit, "remote")
	if err != nil {
		return FetchUIChoices{}, err
	}
	if truncated {
		return FetchUIChoices{}, errors.New("configured remote list exceeds fetch chooser limit")
	}
	result, configured := fetchRemoteChoices(remoteOutput, limit)
	branchOutput, truncated, err := r.outputLimited(ctx, fetchChoicesByteLimit,
		"for-each-ref", "--count="+strconv.Itoa(limit+1), "--sort=refname", "--format=%(refname)%09%(symref)", "refs/remotes")
	if err != nil {
		return FetchUIChoices{}, err
	}
	if truncated {
		return FetchUIChoices{}, errors.New("remote branch list exceeds fetch chooser limit")
	}
	result.RemoteBranches = fetchRemoteBranchChoices(branchOutput, configured, limit)
	sort.Slice(result.RemoteBranches, func(i, j int) bool {
		left, right := result.RemoteBranches[i], result.RemoteBranches[j]
		return left.Remote+"/"+left.Branch < right.Remote+"/"+right.Branch
	})
	return result, nil
}

func fetchRemoteChoices(output []byte, limit int) (FetchUIChoices, map[string]bool) {
	result := FetchUIChoices{Remotes: make([]string, 0, limit)}
	configured := make(map[string]bool, limit)
	for _, remote := range strings.Split(trimLine(output), "\n") {
		if remote == "" || len(result.Remotes) >= limit {
			continue
		}
		result.Remotes = append(result.Remotes, remote)
		configured[remote] = true
	}
	return result, configured
}

func fetchRemoteBranchChoices(output []byte, configured map[string]bool, limit int) []FetchRemoteBranch {
	result := make([]FetchRemoteBranch, 0, limit)
	for _, line := range strings.Split(trimLine(output), "\n") {
		if line == "" || len(result) >= limit {
			continue
		}
		if branch, ok := fetchRemoteBranchChoice(line, configured); ok {
			result = append(result, branch)
		}
	}
	return result
}

func fetchRemoteBranchChoice(line string, configured map[string]bool) (FetchRemoteBranch, bool) {
	full, symref, found := strings.Cut(line, "\t")
	if !found || symref != "" || !strings.HasPrefix(full, "refs/remotes/") {
		return FetchRemoteBranch{}, false
	}
	name := strings.TrimPrefix(full, "refs/remotes/")
	remote := longestFetchRemotePrefix(name, configured)
	if remote == "" {
		return FetchRemoteBranch{}, false
	}
	return FetchRemoteBranch{Remote: remote, Branch: strings.TrimPrefix(name, remote+"/")}, true
}

func longestFetchRemotePrefix(name string, configured map[string]bool) string {
	best := ""
	for remote := range configured {
		if strings.HasPrefix(name, remote+"/") && len(remote) > len(best) {
			best = remote
		}
	}
	return best
}

// FetchUpstreamWithArgs preserves fetch options while resolving the same
// context-aware upstream/primary destination as FetchUpstream.
func (r *Repository) FetchUpstreamWithArgs(ctx context.Context, in FetchArgs) error {
	if in.Remote != "" {
		return errors.New("upstream fetch does not accept an explicit remote")
	}
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	in.Remote, err = r.upstreamOrPrimaryRemote(ctx, branch)
	if err != nil {
		return err
	}
	return r.FetchWithArgs(ctx, in)
}

// FetchPushWithArgs preserves fetch options while requiring a configured push
// remote; unlike upstream fetching it intentionally has no fallback.
func (r *Repository) FetchPushWithArgs(ctx context.Context, in FetchArgs) error {
	if in.Remote != "" {
		return errors.New("push-remote fetch does not accept an explicit remote")
	}
	remote, err := r.PushRemote(ctx)
	if err != nil {
		return err
	}
	in.Remote = remote
	return r.FetchWithArgs(ctx, in)
}

func appendFetchUIOptions(args []string, in FetchArgs) ([]string, error) {
	if in.Prune {
		args = append(args, "--prune")
	}
	args, err := appendFetchTags(args, in.Tags)
	if err != nil {
		return nil, err
	}
	for _, option := range []struct {
		set  bool
		flag string
	}{{in.Unshallow, "--unshallow"}, {in.Force, "--force"}} {
		if option.set {
			args = append(args, option.flag)
		}
	}
	return appendFetchSubmodules(args, in.RecurseSubmodules)
}

func appendFetchTags(args []string, tags FetchTags) ([]string, error) {
	switch tags {
	case FetchTagsDefault:
		return args, nil
	case FetchAllTags:
		return append(args, "--tags"), nil
	case FetchNoTags:
		return append(args, "--no-tags"), nil
	default:
		return nil, errors.New("invalid fetch tags mode")
	}
}

func appendFetchSubmodules(args []string, recurse SubmoduleRecursion) ([]string, error) {
	switch recurse {
	case SubmodulesDefault:
		return args, nil
	case SubmodulesOnDemand:
		return append(args, "--recurse-submodules=on-demand"), nil
	case SubmodulesYes:
		return append(args, "--recurse-submodules=yes"), nil
	case SubmodulesNo:
		return append(args, "--recurse-submodules=no"), nil
	default:
		return nil, errors.New("invalid submodule recursion mode")
	}
}

// FetchAllWithArgs applies options to Git's configured --all operation.
func (r *Repository) FetchAllWithArgs(ctx context.Context, in FetchArgs) error {
	if in.Remote != "" || in.Branch != "" || in.Refspec != "" {
		return errors.New("fetch all does not accept a remote, branch, or refspec")
	}
	args, err := appendFetchUIOptions([]string{"fetch"}, in)
	if err != nil {
		return err
	}
	return r.run(ctx, append(args, "--all")...)
}

// FetchModulesWithArgs adapts Magit's module fetch to Git's non-shell
// recursive fetch. This updates initialized submodules using their configured
// remotes and never invokes `submodule foreach`.
func (r *Repository) FetchModulesWithArgs(ctx context.Context, in FetchArgs) error {
	if in.Remote != "" || in.Branch != "" || in.Refspec != "" {
		return errors.New("module fetch does not accept a remote, branch, or refspec")
	}
	in.RecurseSubmodules = SubmodulesYes
	return r.FetchWithArgs(ctx, in)
}

// ConfigureCurrentFetchBranch backs the fetch transient's C suffix with Git's
// normal branch upstream configuration, including branch/ref validation before
// the config mutation.
func (r *Repository) ConfigureCurrentFetchBranch(ctx context.Context, remote, branch string) error {
	current, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	if current == "" {
		return errors.New("cannot configure fetch branch from detached HEAD")
	}
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return err
	}
	if err := r.validateBranch(ctx, branch); err != nil {
		return fmt.Errorf("configure fetch branch: %w", err)
	}
	return r.SetBranchUpstream(ctx, current, remote+"/"+branch)
}
