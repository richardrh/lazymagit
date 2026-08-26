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
	p.SourceRef = p.Branch
	if in.Source != "" {
		p.SourceRef = in.Source
	} else if in.Tag != "" {
		p.SourceRef = "refs/tags/" + in.Tag
	} else if in.NotesRef != "" {
		p.SourceRef = in.NotesRef
	}
	if p.SourceRef != "" {
		out, resolveErr := r.output(ctx, "rev-parse", "--verify", "--end-of-options", p.SourceRef)
		if resolveErr != nil {
			return p, fmt.Errorf("resolve reviewed push source %q: %w", p.SourceRef, resolveErr)
		}
		p.SourceOID = trimLine(out)
		// Freeze ordinary singular source selectors to the reviewed object while
		// preserving the exact reviewed destination.
		for i, spec := range p.Refspecs {
			if spec == p.SourceRef {
				destination := p.SourceRef
				if !strings.HasPrefix(destination, "refs/") {
					destination = "refs/heads/" + destination
				}
				p.Refspecs[i] = p.SourceOID + ":" + destination
			} else if strings.HasPrefix(spec, p.SourceRef+":") {
				destination := strings.TrimPrefix(spec, p.SourceRef+":")
				if destination != "" && !strings.HasPrefix(destination, "refs/") {
					destination = "refs/heads/" + destination
				}
				p.Refspecs[i] = p.SourceOID + ":" + destination
			}
		}
		for i, arg := range p.Argv {
			if arg == "--" && i+2 <= len(p.Argv) {
				p.Argv = append(append([]string(nil), p.Argv[:i+2]...), p.Refspecs...)
				break
			}
		}
	}
	// Bind every explicit source, not only the dialog's primary selector.
	for _, spec := range p.Refspecs {
		source, _, _ := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if source == "" || source == p.SourceOID || strings.Contains(source, "*") {
			continue
		}
		out, resolveErr := r.output(ctx, "rev-parse", "--verify", "--end-of-options", source)
		if resolveErr != nil {
			continue
		}
		found := false
		for _, existing := range p.Sources {
			found = found || existing.Ref == source
		}
		if !found {
			p.Sources = append(p.Sources, ReviewedPushSource{source, trimLine(out)})
		}
	}
	if p.SourceRef != "" && len(p.Sources) == 0 {
		p.Sources = append(p.Sources, ReviewedPushSource{p.SourceRef, p.SourceOID})
	}
	for _, spec := range p.Refspecs {
		if spec == ":" {
			p.SourceNamespace = "refs/heads"
		}
		if strings.Contains(spec, "*") {
			p.SourceNamespace = strings.Split(spec, "*")[0]
		}
	}
	for _, arg := range p.Argv {
		if arg == "--tags" {
			p.SourceNamespace = "refs/tags"
		}
	}
	if p.SourceNamespace != "" {
		p.Sources, err = r.reviewedSourcesInNamespace(ctx, p.SourceNamespace)
		if err != nil {
			return p, err
		}
	} else {
		for i, spec := range p.Refspecs {
			forced := strings.HasPrefix(spec, "+")
			plain := strings.TrimPrefix(spec, "+")
			source, destination, hasDestination := strings.Cut(plain, ":")
			for _, reviewedSource := range p.Sources {
				if source != reviewedSource.Ref {
					continue
				}
				if !hasDestination {
					destination = source
				}
				if destination != "" && !strings.HasPrefix(destination, "refs/") {
					destination = "refs/heads/" + destination
				}
				prefix := ""
				if forced {
					prefix = "+"
				}
				p.Refspecs[i] = prefix + reviewedSource.OID + ":" + destination
			}
		}
		for i, arg := range p.Argv {
			if arg == "--" && i+1 < len(p.Argv) {
				p.Argv = append(append([]string(nil), p.Argv[:i+2]...), p.Refspecs...)
				break
			}
		}
	}
	if p.RemoteConfig, err = r.remoteIdentityConfig(ctx, p.Remote); err != nil {
		return p, err
	}
	p.Token = NewConfirmationToken(reviewedPushIdentity(p))
	return p, nil
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
		var rollback error
		if p.PushRemote.Set {
			rollback = r.SetBranchPushRemote(ctx, p.Branch, p.PushRemote.Value)
		} else {
			rollback = r.UnsetBranchPushRemote(ctx, p.Branch)
		}
		if rollback != nil {
			return &PartialMutationError{Operation: "persist pushRemote and push", Cause: err, Rollback: rollback, State: []string{"branch." + p.Branch + ".pushRemote=" + remote}}
		}
		return fmt.Errorf("push failed; previous pushRemote restored: %w", err)
	}
	return nil
}

func (r *Repository) validateReviewedPush(ctx context.Context, p ReviewedPush) error {
	if len(p.Argv) == 0 || p.Argv[0] != "push" || p.Remote == "" {
		return errors.New("invalid reviewed push")
	}
	current := p
	var err error
	if current.Branch, err = r.currentBranch(ctx); err != nil {
		return err
	}
	if current.Branch != p.Branch {
		return ErrStalePlan
	}
	if current.Branch != "" {
		current.UpstreamRemote, err = r.workflowConfigValue(ctx, "branch."+current.Branch+".remote")
		if err == nil {
			current.UpstreamMerge, err = r.workflowConfigValue(ctx, "branch."+current.Branch+".merge")
		}
		if err == nil {
			current.PushRemote, err = r.workflowConfigValue(ctx, "branch."+current.Branch+".pushRemote")
		}
		if err != nil {
			return err
		}
	}
	if p.SourceRef != "" {
		out, resolveErr := r.output(ctx, "rev-parse", "--verify", "--end-of-options", p.SourceRef)
		if resolveErr != nil {
			return ErrStalePlan
		}
		current.SourceOID = trimLine(out)
	}
	for _, source := range p.Sources {
		out, resolveErr := r.output(ctx, "rev-parse", "--verify", "--end-of-options", source.Ref)
		if resolveErr != nil || trimLine(out) != source.OID {
			return ErrStalePlan
		}
	}
	if p.SourceNamespace != "" {
		sources, sourceErr := r.reviewedSourcesInNamespace(ctx, p.SourceNamespace)
		if sourceErr != nil {
			return sourceErr
		}
		if len(sources) != len(p.Sources) {
			return ErrStalePlan
		}
		for i := range sources {
			if sources[i] != p.Sources[i] {
				return ErrStalePlan
			}
		}
	}
	if current.RemoteConfig, err = r.remoteIdentityConfig(ctx, p.Remote); err != nil {
		return err
	}
	current.Token = ConfirmationToken{}
	if !p.Token.validFor(reviewedPushIdentity(current)) {
		return ErrStalePlan
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
