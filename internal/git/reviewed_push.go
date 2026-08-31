package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ReviewedPush is an immutable repository-bound capability. Argv is the exact
// git argument vector, including "push", used by execution.
type ReviewedPush struct {
	Argv, Refspecs, RemoteConfig              []string
	Remote, SourceRef, SourceOID, Branch      string
	Sources                                   []ReviewedPushSource
	SourceNamespace                           string
	UpstreamRemote, UpstreamMerge, PushRemote ConfiguredValue
	Token                                     ConfirmationToken
}

type ReviewedPushSource struct{ Ref, OID string }

func (r *Repository) ReviewPush(ctx context.Context, in PushUIArgs) (ReviewedPush, error) {
	argv, err := r.pushUICommand(ctx, in)
	if err != nil {
		return ReviewedPush{}, err
	}
	p := ReviewedPush{Argv: append([]string(nil), argv...)}
	for i, arg := range argv {
		if arg == "--" && i+1 < len(argv) {
			p.Remote = argv[i+1]
			p.Refspecs = append([]string(nil), argv[i+2:]...)
			break
		}
	}
	if p.Branch, err = r.currentBranch(ctx); err != nil {
		return p, err
	}
	if p.Branch != "" {
		p.UpstreamRemote, err = r.workflowConfigValue(ctx, "branch."+p.Branch+".remote")
		if err == nil {
			p.UpstreamMerge, err = r.workflowConfigValue(ctx, "branch."+p.Branch+".merge")
		}
		if err == nil {
			p.PushRemote, err = r.workflowConfigValue(ctx, "branch."+p.Branch+".pushRemote")
		}
		if err != nil {
			return p, err
		}
	}
	p.SourceRef = reviewedPushSourceRef(p.Branch, in)
	if err := r.freezeReviewedPrimarySource(ctx, &p); err != nil {
		return p, err
	}
	if err := r.bindReviewedPushSources(ctx, &p); err != nil {
		return p, err
	}
	p.SourceNamespace = reviewedPushSourceNamespace(p)
	if p.SourceNamespace != "" {
		p.Sources, err = r.reviewedSourcesInNamespace(ctx, p.SourceNamespace)
		if err != nil {
			return p, err
		}
	} else {
		freezeReviewedPushRefspecs(&p)
	}
	if p.RemoteConfig, err = r.remoteIdentityConfig(ctx, p.Remote); err != nil {
		return p, err
	}
	p.Token = NewConfirmationToken(reviewedPushIdentity(p))
	return p, nil
}

func reviewedPushSourceRef(branch string, in PushUIArgs) string {
	if in.Source != "" {
		return in.Source
	}
	if in.Tag != "" {
		return "refs/tags/" + in.Tag
	}
	if in.NotesRef != "" {
		return in.NotesRef
	}
	return branch
}

func (r *Repository) freezeReviewedPrimarySource(ctx context.Context, p *ReviewedPush) error {
	if p.SourceRef == "" {
		return nil
	}
	out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", p.SourceRef)
	if err != nil {
		return fmt.Errorf("resolve reviewed push source %q: %w", p.SourceRef, err)
	}
	p.SourceOID = trimLine(out)
	for i, spec := range p.Refspecs {
		if spec == p.SourceRef {
			p.Refspecs[i] = p.SourceOID + ":" + reviewedPushDestination(p.SourceRef)
		} else if strings.HasPrefix(spec, p.SourceRef+":") {
			p.Refspecs[i] = p.SourceOID + ":" + reviewedPushDestination(strings.TrimPrefix(spec, p.SourceRef+":"))
		}
	}
	replaceReviewedPushArgv(p)
	return nil
}

func reviewedPushDestination(value string) string {
	if value != "" && !strings.HasPrefix(value, "refs/") {
		return "refs/heads/" + value
	}
	return value
}

func (r *Repository) bindReviewedPushSources(ctx context.Context, p *ReviewedPush) error {
	for _, spec := range p.Refspecs {
		source, _, _ := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if source == "" || source == p.SourceOID || strings.Contains(source, "*") || reviewedPushHasSource(p.Sources, source) {
			continue
		}
		out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", source)
		if err == nil {
			p.Sources = append(p.Sources, ReviewedPushSource{source, trimLine(out)})
		}
	}
	if p.SourceRef != "" && len(p.Sources) == 0 {
		p.Sources = append(p.Sources, ReviewedPushSource{p.SourceRef, p.SourceOID})
	}
	return nil
}

func reviewedPushHasSource(sources []ReviewedPushSource, source string) bool {
	for _, existing := range sources {
		if existing.Ref == source {
			return true
		}
	}
	return false
}

func reviewedPushSourceNamespace(p ReviewedPush) string {
	namespace := ""
	for _, spec := range p.Refspecs {
		if spec == ":" {
			namespace = "refs/heads"
		}
		if strings.Contains(spec, "*") {
			namespace = strings.Split(spec, "*")[0]
		}
	}
	for _, arg := range p.Argv {
		if arg == "--tags" {
			return "refs/tags"
		}
	}
	return namespace
}

func freezeReviewedPushRefspecs(p *ReviewedPush) {
	for i, spec := range p.Refspecs {
		forced := strings.HasPrefix(spec, "+")
		source, destination, hasDestination := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		for _, reviewedSource := range p.Sources {
			if source == reviewedSource.Ref {
				if !hasDestination {
					destination = source
				}
				prefix := ""
				if forced {
					prefix = "+"
				}
				p.Refspecs[i] = prefix + reviewedSource.OID + ":" + reviewedPushDestination(destination)
			}
		}
	}
	replaceReviewedPushArgv(p)
}

func replaceReviewedPushArgv(p *ReviewedPush) {
	for i, arg := range p.Argv {
		if arg == "--" && i+1 < len(p.Argv) {
			p.Argv = append(append([]string(nil), p.Argv[:i+2]...), p.Refspecs...)
			return
		}
	}
}

func (r *Repository) ExecuteReviewedPush(ctx context.Context, p ReviewedPush) error {
	if err := r.validateReviewedPush(ctx, p); err != nil {
		return err
	}
	return r.run(ctx, append([]string(nil), p.Argv...)...)
}

func (r *Repository) ExecuteReviewedPushWithPushRemote(ctx context.Context, p ReviewedPush, remote string) error {
	if remote == "" || remote != p.Remote {
		return errors.New("reviewed push remote does not match persisted pushRemote")
	}
	if err := r.validateReviewedPush(ctx, p); err != nil {
		return err
	}
	if p.Branch == "" {
		return errors.New("cannot configure pushRemote from detached HEAD")
	}
	if err := r.SetBranchPushRemote(ctx, p.Branch, remote); err != nil {
		return fmt.Errorf("configure push remote: %w", err)
	}
	if err := r.run(ctx, append([]string(nil), p.Argv...)...); err != nil {
		return r.rollbackReviewedPushRemote(ctx, p, remote, err)
	}
	return nil
}

func (r *Repository) rollbackReviewedPushRemote(ctx context.Context, p ReviewedPush, remote string, cause error) error {
	rollback := r.UnsetBranchPushRemote(ctx, p.Branch)
	if p.PushRemote.Set {
		rollback = r.SetBranchPushRemote(ctx, p.Branch, p.PushRemote.Value)
	}
	if rollback != nil {
		return &PartialMutationError{Operation: "persist pushRemote and push", Cause: cause, Rollback: rollback, State: []string{"branch." + p.Branch + ".pushRemote=" + remote}}
	}
	return fmt.Errorf("push failed; previous pushRemote restored: %w", cause)
}

func (r *Repository) validateReviewedPush(ctx context.Context, p ReviewedPush) error {
	if len(p.Argv) == 0 || p.Argv[0] != "push" || p.Remote == "" {
		return errors.New("invalid reviewed push")
	}
	current := p
	if err := r.loadReviewedPushBranchState(ctx, &current, p.Branch); err != nil {
		return err
	}
	if err := r.validateReviewedPushSources(ctx, &current, p); err != nil {
		return err
	}
	var err error
	if current.RemoteConfig, err = r.remoteIdentityConfig(ctx, p.Remote); err != nil {
		return err
	}
	current.Token = ConfirmationToken{}
	if !p.Token.validFor(reviewedPushIdentity(current)) {
		return ErrStalePlan
	}
	return nil
}

func (r *Repository) loadReviewedPushBranchState(ctx context.Context, current *ReviewedPush, reviewedBranch string) error {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	current.Branch = branch
	if branch != reviewedBranch {
		return ErrStalePlan
	}
	if branch == "" {
		return nil
	}
	current.UpstreamRemote, err = r.workflowConfigValue(ctx, "branch."+branch+".remote")
	if err == nil {
		current.UpstreamMerge, err = r.workflowConfigValue(ctx, "branch."+branch+".merge")
	}
	if err == nil {
		current.PushRemote, err = r.workflowConfigValue(ctx, "branch."+branch+".pushRemote")
	}
	return err
}

func (r *Repository) validateReviewedPushSources(ctx context.Context, current *ReviewedPush, reviewed ReviewedPush) error {
	if reviewed.SourceRef != "" {
		out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", reviewed.SourceRef)
		if err != nil {
			return ErrStalePlan
		}
		current.SourceOID = trimLine(out)
	}
	for _, source := range reviewed.Sources {
		out, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", source.Ref)
		if err != nil || trimLine(out) != source.OID {
			return ErrStalePlan
		}
	}
	if reviewed.SourceNamespace == "" {
		return nil
	}
	sources, err := r.reviewedSourcesInNamespace(ctx, reviewed.SourceNamespace)
	if err != nil {
		return err
	}
	if len(sources) != len(reviewed.Sources) {
		return ErrStalePlan
	}
	for i := range sources {
		if sources[i] != reviewed.Sources[i] {
			return ErrStalePlan
		}
	}
	return nil
}

func (r *Repository) remoteIdentityConfig(ctx context.Context, remote string) ([]string, error) {
	out, err := r.output(ctx, "config", "--get-regexp", "^remote\\."+regexp.QuoteMeta(remote)+"\\.")
	if err != nil {
		if commandExitCode(err) == 1 {
			return nil, nil
		}
		return nil, err
	}
	values := strings.Split(trimLine(out), "\n")
	sort.Strings(values)
	return values, nil
}

func reviewedPushIdentity(p ReviewedPush) string {
	b, _ := json.Marshal(struct {
		Argv, Refspecs, RemoteConfig              []string
		Remote, SourceRef, SourceOID, Branch      string
		Sources                                   []ReviewedPushSource
		SourceNamespace                           string
		UpstreamRemote, UpstreamMerge, PushRemote ConfiguredValue
	}{p.Argv, p.Refspecs, p.RemoteConfig, p.Remote, p.SourceRef, p.SourceOID, p.Branch, p.Sources, p.SourceNamespace, p.UpstreamRemote, p.UpstreamMerge, p.PushRemote})
	return string(b)
}

func (r *Repository) reviewedSourcesInNamespace(ctx context.Context, namespace string) ([]ReviewedPushSource, error) {
	out, err := r.output(ctx, "for-each-ref", "--sort=refname", "--format=%(refname) %(objectname)", namespace)
	if err != nil {
		return nil, err
	}
	var sources []ReviewedPushSource
	for _, line := range strings.Split(trimLine(out), "\n") {
		if ref, oid, ok := strings.Cut(line, " "); ok {
			sources = append(sources, ReviewedPushSource{ref, oid})
		}
	}
	return sources, nil
}
