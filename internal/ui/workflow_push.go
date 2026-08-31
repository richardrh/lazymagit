package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

const (
	pushUpstreamID  keymap.CommandID = "push.push-current-to-upstream"
	pushElsewhereID keymap.CommandID = "push.push-current"
	pushOtherID     keymap.CommandID = "push.push-other"
	pushRefspecsID  keymap.CommandID = "push.push-refspecs"
	pushMatchingID  keymap.CommandID = "push.push-matching"
	pushTagID       keymap.CommandID = "push.push-tag"
	pushTagsID      keymap.CommandID = "push.push-tags"
	pushNotesID     keymap.CommandID = "push.push-notes-ref"
	pushConfigureID keymap.CommandID = "push.branch-configure"
)

func init() {
	pushOptions := []string{"transient:magit-push:--force-with-lease", "transient:magit-push:--force", "transient:magit-push:--no-verify", "transient:magit-push:--dry-run", "transient:magit-push:--set-upstream", "transient:magit-push:--tags", "transient:magit-push:--follow-tags", "magit-push:--push-option"}
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-push", map[string][]string{
		"magit-push-current-to-pushremote": pushOptions,
		"magit-push-current-to-upstream":   pushOptions,
		"magit-push-current":               pushOptions,
		"magit-push-other":                 pushOptions,
		"magit-push-refspecs":              pushOptions,
		"magit-push-matching":              pushOptions,
		"magit-push-tag":                   pushOptions,
		"magit-push-tags":                  pushOptions,
		"magit-push-notes-ref":             pushOptions,
	})...)
	RegisterWorkflowDomain(func(_ *Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			keymap.CommandPush: pushWorkflowHandler(pushCurrentRemote),
			pushUpstreamID:     pushWorkflowHandler(pushCurrentUpstream),
			pushElsewhereID:    pushWorkflowHandler(pushCurrentElsewhere),
			pushOtherID:        pushWorkflowHandler(pushAnotherBranch),
			pushRefspecsID:     pushWorkflowHandler(pushExplicitRefspecs),
			pushMatchingID:     pushWorkflowHandler(pushMatchingBranches),
			pushTagID:          pushWorkflowHandler(pushOneTag),
			pushTagsID:         pushWorkflowHandler(pushAllTags),
			pushNotesID:        pushWorkflowHandler(pushOneNotesRef),
			pushConfigureID:    configurePushBranchHandler,
		}
	})
}

type pushWorkflowKind uint8

const (
	pushCurrentRemote pushWorkflowKind = iota
	pushCurrentUpstream
	pushCurrentElsewhere
	pushAnotherBranch
	pushExplicitRefspecs
	pushMatchingBranches
	pushOneTag
	pushAllTags
	pushOneNotesRef
)

func pushWorkflowHandler(kind pushWorkflowKind) WorkflowHandler {
	return func(m *Model, command WorkflowCommand) tea.Cmd {
		// Existing central tests inject the legacy push callbacks into a nil-repo
		// model. Preserve that seam while real repositories use this domain.
		if m.repo == nil && kind == pushCurrentRemote {
			return legacyConfiguredPush(m)
		}
		if m.repo == nil {
			m.setError(errors.New("push workflow requires a repository"))
			return nil
		}
		return m.LoadWorkflow("push", func(ctx context.Context) (WorkflowDialog, error) {
			return loadPushDialog(ctx, m.repo, kind, command.Options)
		})
	}
}

func legacyConfiguredPush(m *Model) tea.Cmd {
	if m.snapshot.summary.Upstream != "" {
		return m.startOperation("push", m.push)
	}
	if m.snapshot.pushRemote != "" {
		remote := m.snapshot.pushRemote
		return m.startOperation("push and set upstream", func(ctx context.Context) error { return m.pushSetUpstream(ctx, remote) })
	}
	if len(m.snapshot.remotes) == 0 {
		m.setError(errors.New("push: no remotes configured"))
		return nil
	}
	m.remoteCursor, m.remotePurpose = 0, remoteConfigureAndPush
	m.setMode(modeRemotes)
	return nil
}

func loadPushDialog(ctx context.Context, repo *gitbackend.Repository, kind pushWorkflowKind, options map[keymap.CommandID]OptionValue) (WorkflowDialog, error) {
	d := WorkflowDialog{Title: "Push", Operation: "push"}
	base, err := pushArgsFromOptions(options)
	if err != nil {
		return d, err
	}
	configureRemote, fields, err := loadPushFields(ctx, repo, kind)
	if err != nil {
		return d, err
	}
	d.Fields = fields

	d.Validate = func(values WorkflowValues) error { return validatePushFields(d.Fields, values) }
	d.ReviewPreflight = func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
		args := base
		switch kind {
		case pushCurrentRemote:
			args.Target = gitbackend.PushToPushRemote
			if configureRemote {
				args.Target, args.Remote = gitbackend.PushElsewhere, values["remote"]
			}
		case pushCurrentUpstream:
			args.Target = gitbackend.PushToUpstream
		case pushCurrentElsewhere:
			args.Target, args.Remote = gitbackend.PushElsewhere, values["remote"]
		case pushAnotherBranch:
			args.Target, args.Remote, args.Source, args.Destination = gitbackend.PushElsewhere, values["remote"], values["source"], strings.TrimSpace(values["destination"])
		case pushExplicitRefspecs:
			args.Target, args.Remote, args.Refspecs = gitbackend.PushElsewhere, values["remote"], strings.Fields(values["refspecs"])
		case pushMatchingBranches:
			args.Target, args.Remote, args.Matching = gitbackend.PushElsewhere, values["remote"], true
		case pushOneTag:
			args.Target, args.Remote, args.Tag = gitbackend.PushElsewhere, values["remote"], values["tag"]
		case pushAllTags:
			args.Target, args.Remote, args.AllTags = gitbackend.PushElsewhere, values["remote"], true
		case pushOneNotesRef:
			args.Target, args.Remote, args.NotesRef = gitbackend.PushElsewhere, values["remote"], values["notes"]
		}
		reviewed, err := repo.ReviewPush(ctx, args)
		if err != nil {
			return WorkflowReview{}, fmt.Errorf("validate push: %w", err)
		}
		configured := ""
		plan := []string{"remote: " + reviewed.Remote, "source: " + reviewed.SourceRef + " @ " + reviewed.SourceOID, "exact argv: " + formatArgv(reviewed.Argv)}
		for _, refspec := range reviewed.Refspecs {
			plan = append(plan, "refspec: "+refspec)
		}
		switch args.Force {
		case gitbackend.PushForceWithLease:
			plan = append(plan, "Force with lease (remote changes remain protected)")
		case gitbackend.PushForceUnconditionally:
			plan = append(plan, "UNCONDITIONAL FORCE (may overwrite remote changes)")
		}
		confirmation := "Review the exact push plan, then execute"
		if configureRemote {
			configured = values["remote"]
			previous := "<unset>"
			if reviewed.PushRemote.Set {
				previous = reviewed.PushRemote.Value
			}
			plan = append([]string{"Persist branch pushRemote as " + configured, "Persist branch." + reviewed.Branch + ".pushRemote: " + previous + " -> " + configured}, plan...)
			confirmation = "Confirm persistent push-remote setting and push"
		}
		if args.Force == gitbackend.PushForceUnconditionally {
			confirmation = "DANGER: confirm unconditional force push"
		}
		return WorkflowReview{Plan: plan, Confirmation: confirmation, Data: struct {
			Plan      gitbackend.ReviewedPush
			Configure string
		}{reviewed, configured}}, nil
	}
	d.SubmitReview = func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
		approved, ok := review.Data.(struct {
			Plan      gitbackend.ReviewedPush
			Configure string
		})
		if !ok {
			return errors.New("push review is invalid")
		}
		if approved.Configure != "" {
			return repo.ExecuteReviewedPushWithPushRemote(ctx, approved.Plan, approved.Configure)
		}
		return repo.ExecuteReviewedPush(ctx, approved.Plan)
	}
	return d, nil
}

func loadPushFields(ctx context.Context, repo *gitbackend.Repository, kind pushWorkflowKind) (bool, []WorkflowField, error) {
	needRemote, configure, err := pushRemoteRequirement(ctx, repo, kind)
	if err != nil {
		return false, nil, err
	}
	var fields []WorkflowField
	if needRemote {
		field, err := pushRemoteField(ctx, repo)
		if err != nil {
			return false, nil, err
		}
		fields = append(fields, field)
	}
	extra, err := pushKindFields(ctx, repo, kind)
	return configure, append(fields, extra...), err
}

func pushRemoteRequirement(ctx context.Context, repo *gitbackend.Repository, kind pushWorkflowKind) (bool, bool, error) {
	if kind != pushCurrentRemote {
		return kind != pushCurrentUpstream, false, nil
	}
	if _, err := repo.PushRemote(ctx); err != nil {
		if errors.Is(err, gitbackend.ErrNoFetchRemote) {
			return true, true, nil
		}
		return false, false, fmt.Errorf("resolve push remote: %w", err)
	}
	return false, false, nil
}

func pushRemoteField(ctx context.Context, repo *gitbackend.Repository) (WorkflowField, error) {
	remotes, err := repo.Remotes(ctx)
	if err != nil {
		return WorkflowField{}, fmt.Errorf("load remotes: %w", err)
	}
	choices := make([]WorkflowChoice, 0, len(remotes))
	for _, remote := range remotes {
		choices = append(choices, WorkflowChoice{Value: remote.Name, Label: remote.Name})
	}
	if len(choices) == 0 {
		return WorkflowField{}, errors.New("push: no remotes configured")
	}
	return selectField("remote", "Remote", choices), nil
}

func pushKindFields(ctx context.Context, repo *gitbackend.Repository, kind pushWorkflowKind) ([]WorkflowField, error) {
	switch kind {
	case pushAnotherBranch:
		return pushBranchFields(ctx, repo)
	case pushExplicitRefspecs:
		return []WorkflowField{{Name: "refspecs", Label: "Refspecs (space separated)", Kind: WorkflowText, Required: true}}, nil
	case pushOneTag:
		return pushTagFields(ctx, repo)
	case pushOneNotesRef:
		return pushNotesFields(ctx, repo)
	default:
		return nil, nil
	}
}

func pushBranchFields(ctx context.Context, repo *gitbackend.Repository) ([]WorkflowField, error) {
	branches, err := repo.Branches(ctx)
	if err != nil {
		return nil, fmt.Errorf("load branches: %w", err)
	}
	var choices []WorkflowChoice
	for _, branch := range branches {
		if !branch.Remote {
			choices = append(choices, WorkflowChoice{Value: branch.Name, Label: branch.Name})
		}
	}
	if len(choices) == 0 {
		return nil, errors.New("push: no local branches")
	}
	return []WorkflowField{selectField("source", "Local branch", choices), {Name: "destination", Label: "Destination branch", Kind: WorkflowText, Value: choices[0].Value, Required: true}}, nil
}

func pushTagFields(ctx context.Context, repo *gitbackend.Repository) ([]WorkflowField, error) {
	tags, err := repo.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("load tags: %w", err)
	}
	choices := make([]WorkflowChoice, 0, len(tags))
	for _, tag := range tags {
		choices = append(choices, WorkflowChoice{Value: tag.Name, Label: tag.Name})
	}
	if len(choices) == 0 {
		return nil, errors.New("push: no tags")
	}
	return []WorkflowField{selectField("tag", "Tag", choices)}, nil
}

func pushNotesFields(ctx context.Context, repo *gitbackend.Repository) ([]WorkflowField, error) {
	refs, err := repo.ListNotesRefs(ctx)
	if err != nil {
		return nil, fmt.Errorf("load notes refs: %w", err)
	}
	choices := make([]WorkflowChoice, 0, len(refs))
	for _, ref := range refs {
		choices = append(choices, WorkflowChoice{Value: ref, Label: ref})
	}
	if len(choices) == 0 {
		return nil, errors.New("push: no notes refs")
	}
	return []WorkflowField{selectField("notes", "Notes ref", choices)}, nil
}

func formatArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		parts[i] = fmt.Sprintf("%q", arg)
	}
	return "git " + strings.Join(parts, " ")
}

func pushArgsFromOptions(options map[keymap.CommandID]OptionValue) (gitbackend.PushUIArgs, error) {
	var out gitbackend.PushUIArgs
	enabled := func(suffix string) bool {
		for id, value := range options {
			if strings.HasSuffix(string(id), suffix) && value.Enabled {
				return true
			}
		}
		return false
	}
	lease, force := enabled("--force-with-lease"), enabled("--force")
	if lease && force {
		return out, errors.New("force-with-lease and unconditional force are mutually exclusive")
	}
	if lease {
		out.Force = gitbackend.PushForceWithLease
	} else if force {
		out.Force = gitbackend.PushForceUnconditionally
	}
	out.NoVerify = enabled("--no-verify")
	out.DryRun = enabled("--dry-run")
	out.SetUpstream = enabled("--set-upstream")
	out.Tags = enabled("--tags")
	out.FollowTags = enabled("--follow-tags")
	for id, value := range options {
		if strings.HasSuffix(string(id), "--push-option") && (value.Enabled || value.Value != "") {
			for _, option := range strings.Split(value.Value, "\n") {
				option = strings.TrimSuffix(option, "\r")
				if option == "" {
					return out, errors.New("push option is empty")
				}
				out.PushOptions = append(out.PushOptions, option)
			}
		}
	}
	return out, nil
}

func pushPlan(args gitbackend.PushUIArgs) []string {
	target := args.Remote
	if args.Target == gitbackend.PushToPushRemote {
		target = "configured pushRemote"
	} else if args.Target == gitbackend.PushToUpstream {
		target = "upstream"
	}
	plan := []string{"Push to " + target}
	switch args.Force {
	case gitbackend.PushForceWithLease:
		plan = append(plan, "Force with lease (remote changes remain protected)")
	case gitbackend.PushForceUnconditionally:
		plan = append(plan, "UNCONDITIONAL FORCE (may overwrite remote changes)")
	}
	if args.DryRun {
		plan = append(plan, "Dry run only")
	}
	return plan
}

func selectField(name, label string, choices []WorkflowChoice) WorkflowField {
	return WorkflowField{Name: name, Label: label, Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices, Required: true}
}

func validatePushFields(fields []WorkflowField, values WorkflowValues) error {
	for _, field := range fields {
		value := values[field.Name]
		if field.Required && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field.Label)
		}
		if field.Kind == WorkflowSelect {
			valid := false
			for _, choice := range field.Choices {
				valid = valid || value == choice.Value
			}
			if !valid {
				return fmt.Errorf("invalid %s selection", field.Label)
			}
		}
	}
	return nil
}

func configurePushBranchHandler(m *Model, _ WorkflowCommand) tea.Cmd {
	return branchConfigure(m, WorkflowCommand{})
}
