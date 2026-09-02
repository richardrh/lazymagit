package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ReviewedRemoteBranchPush binds a publish operation to the exact local commit
// and observed destination shown in the UI.
type ReviewedRemoteBranchPush struct {
	LocalBranch  string
	LocalOID     string
	Remote       string
	RemoteBranch string
	RemoteOID    string
	SetUpstream  bool
	Token        ConfirmationToken
}

func (r *Repository) ReviewRemoteBranchPush(ctx context.Context, localBranch, remote, remoteBranch string, setUpstream bool) (ReviewedRemoteBranchPush, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return ReviewedRemoteBranchPush{}, err
	}
	if err := r.validateBranchName(ctx, localBranch); err != nil {
		return ReviewedRemoteBranchPush{}, err
	}
	if err := r.validateBranchName(ctx, remoteBranch); err != nil {
		return ReviewedRemoteBranchPush{}, err
	}
	oid, err := r.localBranchOID(ctx, localBranch)
	if err != nil {
		return ReviewedRemoteBranchPush{}, err
	}
	remoteOID, err := r.remoteBranchOID(ctx, remote, remoteBranch)
	if err != nil {
		return ReviewedRemoteBranchPush{}, err
	}
	plan := ReviewedRemoteBranchPush{LocalBranch: localBranch, LocalOID: oid, Remote: remote, RemoteBranch: remoteBranch, RemoteOID: remoteOID, SetUpstream: setUpstream}
	plan.Token = NewConfirmationToken(remoteBranchPushIdentity(plan))
	return plan, nil
}

func (r *Repository) PushRemoteBranchReviewed(ctx context.Context, reviewed ReviewedRemoteBranchPush) error {
	current, err := r.ReviewRemoteBranchPush(ctx, reviewed.LocalBranch, reviewed.Remote, reviewed.RemoteBranch, reviewed.SetUpstream)
	if err != nil {
		return err
	}
	if !reviewed.Token.validFor(remoteBranchPushIdentity(current)) || current.LocalOID != reviewed.LocalOID || current.RemoteOID != reviewed.RemoteOID {
		return ErrStalePlan
	}
	args := []string{"push"}
	if reviewed.SetUpstream {
		args = append(args, "--set-upstream")
	}
	args = append(args, "--", reviewed.Remote, "refs/heads/"+reviewed.LocalBranch+":refs/heads/"+reviewed.RemoteBranch)
	return r.run(ctx, args...)
}

func RemoteBranchPushPlanLines(plan ReviewedRemoteBranchPush) []string {
	lines := []string{
		"local branch: " + plan.LocalBranch,
		"reviewed commit: " + plan.LocalOID,
		"destination: " + plan.Remote + "/" + plan.RemoteBranch,
	}
	if plan.RemoteOID == "" {
		lines = append(lines, "create remote branch")
	} else {
		lines = append(lines, "replace remote commit: "+plan.RemoteOID)
	}
	if plan.SetUpstream {
		lines = append(lines, "set upstream: "+plan.LocalBranch+" -> "+plan.Remote+"/"+plan.RemoteBranch)
	}
	return lines
}

// ReviewedRemoteBranchDelete binds deletion to the exact advertised remote OID.
type ReviewedRemoteBranchDelete struct {
	Remote, Branch, OID string
	Token               ConfirmationToken
}

func (r *Repository) ReviewRemoteBranchDelete(ctx context.Context, remote, branch string) (ReviewedRemoteBranchDelete, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return ReviewedRemoteBranchDelete{}, err
	}
	if err := r.validateBranchName(ctx, branch); err != nil {
		return ReviewedRemoteBranchDelete{}, err
	}
	oid, err := r.remoteBranchOID(ctx, remote, branch)
	if err != nil {
		return ReviewedRemoteBranchDelete{}, err
	}
	if oid == "" {
		return ReviewedRemoteBranchDelete{}, fmt.Errorf("remote branch %s/%s does not exist", remote, branch)
	}
	plan := ReviewedRemoteBranchDelete{Remote: remote, Branch: branch, OID: oid}
	plan.Token = NewConfirmationToken(remoteBranchDeleteIdentity(plan))
	return plan, nil
}

func (r *Repository) DeleteRemoteBranchReviewed(ctx context.Context, reviewed ReviewedRemoteBranchDelete) error {
	current, err := r.ReviewRemoteBranchDelete(ctx, reviewed.Remote, reviewed.Branch)
	if err != nil {
		return err
	}
	if !reviewed.Token.validFor(remoteBranchDeleteIdentity(current)) || current.OID != reviewed.OID {
		return ErrStalePlan
	}
	return r.DeleteRemoteBranch(ctx, reviewed.Remote, reviewed.Branch)
}

func RemoteBranchDeletePlanLines(plan ReviewedRemoteBranchDelete) []string {
	return []string{"delete remote branch: " + plan.Remote + "/" + plan.Branch, "reviewed remote commit: " + plan.OID, "the remote server will remove refs/heads/" + plan.Branch}
}

func (r *Repository) remoteBranchOID(ctx context.Context, remote, branch string) (string, error) {
	out, err := r.output(ctx, "ls-remote", "--heads", "--", remote, "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	line := trimLine(out)
	if line == "" {
		return "", nil
	}
	fields := strings.Fields(line)
	if len(fields) != 2 || fields[1] != "refs/heads/"+branch {
		return "", errors.New("malformed remote branch advertisement")
	}
	return fields[0], nil
}

func remoteBranchPushIdentity(plan ReviewedRemoteBranchPush) string {
	b, _ := json.Marshal(struct {
		Local, OID, Remote, Branch, RemoteOID string
		Upstream                              bool
	}{plan.LocalBranch, plan.LocalOID, plan.Remote, plan.RemoteBranch, plan.RemoteOID, plan.SetUpstream})
	return string(b)
}

func remoteBranchDeleteIdentity(plan ReviewedRemoteBranchDelete) string {
	return plan.Remote + "\x00" + plan.Branch + "\x00" + plan.OID
}

// CheckoutRemoteTrackingBranch creates and checks out a local branch whose
// upstream is the exact selected remote-tracking ref.
func (r *Repository) CheckoutRemoteTrackingBranch(ctx context.Context, remoteRef, localBranch string, opts CheckoutOptions) error {
	if err := r.validateBranchName(ctx, localBranch); err != nil {
		return err
	}
	full, err := r.output(ctx, "rev-parse", "--symbolic-full-name", "--verify", remoteRef)
	if err != nil {
		return err
	}
	fullRef := trimLine(full)
	if !strings.HasPrefix(fullRef, "refs/remotes/") || strings.TrimPrefix(fullRef, "refs/remotes/") != remoteRef {
		return errors.New("selected ref is not a remote-tracking branch")
	}
	args := []string{"switch"}
	if opts.RecurseSubmodules {
		args = append(args, "--recurse-submodules")
	}
	args = append(args, "--track", "-c", localBranch, remoteRef)
	return r.run(ctx, args...)
}
