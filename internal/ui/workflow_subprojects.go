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
	submodulePrefix = "o"
	subtreePrefix   = "O"
	sparsePrefix    = ">"
)

// This domain intentionally discovers command IDs from the pinned registry.
// Occurrence-specific IDs remain owned by keymap; this file only associates
// upstream behavior names with typed repository operations.
func init() {
	submoduleOptions := []string{"transient:magit-submodule:--force", "transient:magit-submodule:--recursive", "transient:magit-submodule:--no-fetch", "transient:magit-submodule:--checkout", "transient:magit-submodule:--rebase", "transient:magit-submodule:--merge", "transient:magit-submodule:--remote"}
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-submodule", map[string][]string{
		"magit-submodule-add":         {"transient:magit-submodule:--force"},
		"magit-submodule-populate":    submoduleOptions,
		"magit-submodule-update":      submoduleOptions,
		"magit-submodule-synchronize": {"transient:magit-submodule:--recursive"},
		"magit-submodule-unpopulate":  {"transient:magit-submodule:--force"},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-subtree-import", map[string][]string{
		"magit-subtree-add":        {"magit-subtree:--prefix", "magit-subtree:--message", "transient:magit-subtree-import:--squash"},
		"magit-subtree-add-commit": {"magit-subtree:--prefix", "magit-subtree:--message", "transient:magit-subtree-import:--squash"},
		"magit-subtree-merge":      {"magit-subtree:--prefix", "magit-subtree:--message", "transient:magit-subtree-import:--squash"},
		"magit-subtree-pull":       {"magit-subtree:--prefix", "magit-subtree:--message", "transient:magit-subtree-import:--squash"},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-subtree-export", map[string][]string{
		"magit-subtree-push":  {"magit-subtree:--prefix", "magit-subtree:--branch"},
		"magit-subtree-split": {"magit-subtree:--prefix", "magit-subtree:--branch"},
	})...)
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		byUpstream := map[string]WorkflowHandler{
			"magit-submodule-add": submoduleAddWorkflow, "magit-submodule-register": submoduleInitWorkflow,
			"magit-submodule-populate": submodulePopulateWorkflow, "magit-submodule-update": submoduleUpdateWorkflow,
			"magit-submodule-synchronize": submoduleSyncWorkflow, "magit-submodule-unpopulate": submoduleDeinitWorkflow,
			"magit-submodule-remove": submoduleRemoveWorkflow, "magit-list-submodules": submoduleListWorkflow,
			"magit-fetch-modules": submoduleFetchWorkflow,
			"magit-subtree-add":   subtreeWorkflow("add"), "magit-subtree-add-commit": subtreeWorkflow("add-commit"),
			"magit-subtree-merge": subtreeWorkflow("merge"), "magit-subtree-pull": subtreeWorkflow("pull"),
			"magit-subtree-push": subtreeWorkflow("push"), "magit-subtree-split": subtreeWorkflow("split"),
			"magit-sparse-checkout-enable": sparseEnableWorkflow, "magit-sparse-checkout-disable": sparseDisableWorkflow,
			"magit-sparse-checkout-reapply": sparseReapplyWorkflow, "magit-sparse-checkout-set": sparseSetWorkflow,
			"magit-sparse-checkout-add": sparseAddWorkflow,
		}
		out := make(map[keymap.CommandID]WorkflowHandler)
		for _, binding := range keymap.Registry() {
			if handler := byUpstream[binding.UpstreamCommand]; handler != nil {
				out[binding.Command] = handler
			}
		}
		return out
	})
}

func boolOption(command WorkflowCommand, upstream string) bool {
	for _, binding := range keymap.Registry() {
		if binding.Context == keymap.ContextTransient+command.Prefix && binding.UpstreamCommand == upstream {
			value := command.Options[binding.Command]
			return value.Enabled || value.Value != ""
		}
	}
	return false
}

func stringOption(command WorkflowCommand, upstream string) string {
	for _, binding := range keymap.Registry() {
		if binding.Context == keymap.ContextTransient+command.Prefix && binding.UpstreamCommand == upstream {
			return command.Options[binding.Command].Value
		}
	}
	return ""
}

func submoduleChoices(ctx context.Context, m *Model, withAll bool) ([]WorkflowChoice, []gitbackend.Submodule, error) {
	modules, err := m.repo.Submodules(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(modules) == 0 {
		return nil, nil, errors.New("no configured submodules")
	}
	choices := make([]WorkflowChoice, 0, len(modules)+1)
	if withAll {
		choices = append(choices, WorkflowChoice{Value: "", Label: "All configured submodules"})
	}
	for _, module := range modules {
		label := module.Path
		if !module.Initialized {
			label += " (not initialized)"
		}
		choices = append(choices, WorkflowChoice{Value: module.Path, Label: label})
	}
	return choices, modules, nil
}

func selectedSubmodulePaths(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func submoduleAddWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	force := boolOption(command, "transient:magit-submodule:--force")
	d := WorkflowDialog{Title: "Add submodule", Operation: "add submodule", Fields: []WorkflowField{
		{Name: "url", Label: "Repository URL", Kind: WorkflowText, Required: true},
		{Name: "path", Label: "Repository-relative path", Kind: WorkflowText, Required: true},
		{Name: "name", Label: "Optional module name", Kind: WorkflowText},
		{Name: "branch", Label: "Optional branch", Kind: WorkflowText},
	}, Validate: func(v WorkflowValues) error {
		for _, name := range []string{"url", "path", "name", "branch"} {
			if strings.ContainsAny(v[name], "\x00\r\n") {
				return fmt.Errorf("%s must be a single-line value", name)
			}
		}
		return nil
	}}
	options := func(v WorkflowValues) gitbackend.SubmoduleAddOptions {
		return gitbackend.SubmoduleAddOptions{Name: v["name"], Branch: v["branch"], Force: force}
	}
	if !force {
		d.Submit = func(ctx context.Context, v WorkflowValues) error {
			return m.repo.AddSubmodule(ctx, v["url"], v["path"], options(v))
		}
		return m.OpenWorkflow(d)
	}
	type reviewedAdd struct {
		url, path string
		opts      gitbackend.SubmoduleAddOptions
	}
	d.ReviewPreflight = func(_ context.Context, v WorkflowValues) (WorkflowReview, error) {
		plan := reviewedAdd{url: v["url"], path: v["path"], opts: options(v)}
		return WorkflowReview{Plan: []string{"Force-add submodule at: " + plan.path, "Repository: " + plan.url}, Confirmation: "Force-add exactly this reviewed submodule?", Data: plan}, nil
	}
	d.SubmitReview = func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
		plan, ok := review.Data.(reviewedAdd)
		if !ok {
			return errors.New("invalid force-add submodule review")
		}
		return m.repo.AddSubmodule(ctx, plan.url, plan.path, plan.opts)
	}
	return m.OpenWorkflow(d)
}

func submoduleSelectionWorkflow(m *Model, name string, withAll bool, submit func(context.Context, []string) error) tea.Cmd {
	return m.LoadWorkflow(name, func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := submoduleChoices(ctx, m, withAll)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: name, Operation: strings.ToLower(name), Fields: []WorkflowField{{Name: "path", Label: "Scope", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices}}, Submit: func(ctx context.Context, v WorkflowValues) error {
			return submit(ctx, selectedSubmodulePaths(v["path"]))
		}}, nil
	})
}

func submoduleInitWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return submoduleSelectionWorkflow(m, "Initialize submodules", true, m.repo.InitSubmodules)
}

func submodulePopulateWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return submoduleUpdateDialog(m, command, true)
}

func submoduleUpdateWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return submoduleUpdateDialog(m, command, false)
}

type reviewedSubmoduleUpdate struct {
	paths []string
	opts  gitbackend.SubmoduleUpdateOptions
}

func submoduleUpdateDialog(m *Model, command WorkflowCommand, initModules bool) tea.Cmd {
	return m.LoadWorkflow("submodule update", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := submoduleChoices(ctx, m, true)
		if err != nil {
			return WorkflowDialog{}, err
		}
		opts := gitbackend.SubmoduleUpdateOptions{Init: initModules, Recursive: boolOption(command, "transient:magit-submodule:--recursive"), Remote: boolOption(command, "transient:magit-submodule:--remote"), Rebase: boolOption(command, "transient:magit-submodule:--rebase"), Merge: boolOption(command, "transient:magit-submodule:--merge"), Checkout: boolOption(command, "transient:magit-submodule:--checkout"), NoFetch: boolOption(command, "transient:magit-submodule:--no-fetch")}
		force := boolOption(command, "transient:magit-submodule:--force")
		if force {
			opts.Force = gitbackend.Confirmed
		}
		if opts.Rebase && opts.Merge || opts.Rebase && opts.Checkout || opts.Merge && opts.Checkout {
			return WorkflowDialog{}, errors.New("checkout, rebase, and merge update modes are mutually exclusive")
		}
		d := WorkflowDialog{Title: "Update submodules", Operation: "update submodules", Fields: []WorkflowField{{Name: "path", Label: "Scope", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices}}}
		if !force {
			d.Submit = func(ctx context.Context, v WorkflowValues) error {
				return m.repo.UpdateSubmodules(ctx, selectedSubmodulePaths(v["path"]), opts)
			}
			return d, nil
		}
		d.ReviewPreflight = func(_ context.Context, v WorkflowValues) (WorkflowReview, error) {
			paths := append([]string(nil), selectedSubmodulePaths(v["path"])...)
			scope := "all configured submodules"
			if len(paths) == 1 {
				scope = paths[0]
			}
			return WorkflowReview{Plan: []string{"Force update scope: " + scope}, Confirmation: "Force exactly this reviewed scope?", Data: reviewedSubmoduleUpdate{paths: paths, opts: opts}}, nil
		}
		d.SubmitReview = func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			plan, ok := review.Data.(reviewedSubmoduleUpdate)
			if !ok {
				return errors.New("invalid submodule update review")
			}
			return m.repo.UpdateSubmodules(ctx, append([]string(nil), plan.paths...), plan.opts)
		}
		return d, nil
	})
}

func submoduleSyncWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	recursive := boolOption(command, "transient:magit-submodule:--recursive")
	return submoduleSelectionWorkflow(m, "Synchronize submodules", true, func(ctx context.Context, paths []string) error { return m.repo.SyncSubmodules(ctx, paths, recursive) })
}

type reviewedSubmoduleScope struct {
	path  string
	all   bool
	force gitbackend.ConfirmedForce
}

func submoduleDeinitWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("submodule deinitialization", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := submoduleChoices(ctx, m, true)
		if err != nil {
			return WorkflowDialog{}, err
		}
		force := gitbackend.NotConfirmed
		if boolOption(command, "transient:magit-submodule:--force") {
			force = gitbackend.Confirmed
		}
		return WorkflowDialog{Title: "Deinitialize submodules", Operation: "deinitialize submodules", Fields: []WorkflowField{{Name: "path", Label: "Reviewed scope", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices}}, ReviewPreflight: func(_ context.Context, v WorkflowValues) (WorkflowReview, error) {
			path := v["path"]
			scope := path
			if path == "" {
				scope = "ALL configured submodules"
			}
			return WorkflowReview{Plan: []string{"Deinitialize: " + scope}, Confirmation: "Deinitialize exactly this reviewed scope?", Data: reviewedSubmoduleScope{path: path, all: path == "", force: force}}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			plan, ok := review.Data.(reviewedSubmoduleScope)
			if !ok {
				return errors.New("invalid submodule deinitialization review")
			}
			if plan.all {
				return m.repo.DeinitAllSubmodules(ctx, gitbackend.NewConfirmationToken("all-submodules"), plan.force)
			}
			return m.repo.DeinitSubmodules(ctx, []string{plan.path}, plan.force)
		}}, nil
	})
}

func submoduleRemoveWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("submodule removal", func(ctx context.Context) (WorkflowDialog, error) {
		choices, _, err := submoduleChoices(ctx, m, false)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Remove submodule", Operation: "remove submodule", Fields: []WorkflowField{{Name: "path", Label: "Submodule", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices}}, ReviewPreflight: func(_ context.Context, v WorkflowValues) (WorkflowReview, error) {
			path := v["path"]
			return WorkflowReview{Plan: []string{"Deinitialize and remove gitlink: " + path, "Remove private submodule metadata after Git succeeds"}, Confirmation: "Remove exactly this reviewed submodule?", Data: reviewedSubmoduleScope{path: path}}, nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			plan, ok := review.Data.(reviewedSubmoduleScope)
			if !ok || plan.path == "" {
				return errors.New("invalid submodule removal review")
			}
			return m.repo.RemoveSubmodule(ctx, plan.path, gitbackend.Confirmed)
		}}, nil
	})
}

func submoduleListWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("submodules", func(ctx context.Context) (WorkflowDialog, error) {
		_, modules, err := submoduleChoices(ctx, m, false)
		if err != nil {
			return WorkflowDialog{}, err
		}
		lines := make([]string, 0, len(modules))
		for _, module := range modules {
			state := "not initialized"
			if module.Initialized {
				state = "initialized"
			}
			lines = append(lines, module.Path+"  "+module.Commit+"  "+state+"  "+module.URL)
		}
		return WorkflowDialog{Title: "Configured submodules", Plan: lines, Confirmation: "Enter closes this list", Submit: func(context.Context, WorkflowValues) error { return nil }}, nil
	})
}

func submoduleFetchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.StartWorkflowOperation("fetch submodules", func(ctx context.Context) error { return m.repo.FetchModulesWithArgs(ctx, gitbackend.FetchArgs{}) })
}

func rejectOptionLike(values ...string) error {
	for _, value := range values {
		if value != "" && (strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n")) {
			return fmt.Errorf("subtree value %q is option-like or contains a control character", value)
		}
	}
	return nil
}

func subtreeWorkflow(action string) WorkflowHandler {
	return func(m *Model, command WorkflowCommand) tea.Cmd {
		unsupported := []string{"magit-subtree:--annotate", "magit-subtree:--onto", "transient:magit-subtree-export:--ignore-joins", "transient:magit-subtree-export:--rejoin"}
		for _, option := range unsupported {
			if boolOption(command, option) || stringOption(command, option) != "" {
				m.setError(fmt.Errorf("%s is unsupported by the typed subtree API", option))
				return nil
			}
		}
		prefix := stringOption(command, "magit-subtree:--prefix")
		message := stringOption(command, "magit-subtree:--message")
		squash := boolOption(command, "transient:magit-subtree-import:--squash")
		fields := []WorkflowField{{Name: "prefix", Label: "Subtree prefix", Kind: WorkflowText, Value: prefix, Required: true}}
		switch action {
		case "add":
			fields = append(fields, WorkflowField{Name: "repository", Label: "Repository", Kind: WorkflowText, Required: true}, WorkflowField{Name: "ref", Label: "Ref", Kind: WorkflowText, Required: true})
		case "add-commit", "merge":
			fields = append(fields, WorkflowField{Name: "ref", Label: "Commit", Kind: WorkflowText, Required: true})
		case "pull", "push":
			fields = append(fields, WorkflowField{Name: "repository", Label: "Repository", Kind: WorkflowText, Required: true}, WorkflowField{Name: "ref", Label: "Ref", Kind: WorkflowText, Required: true})
		case "split":
			fields = append(fields, WorkflowField{Name: "branch", Label: "Optional output branch", Kind: WorkflowText, Value: stringOption(command, "magit-subtree:--branch")})
		}
		return m.OpenWorkflow(WorkflowDialog{Title: "Subtree " + action, Operation: "subtree " + action, Fields: fields, Validate: func(v WorkflowValues) error {
			return rejectOptionLike(v["prefix"], v["repository"], v["ref"], v["branch"])
		}, Submit: func(ctx context.Context, v WorkflowValues) error {
			opts := gitbackend.SubtreeOptions{Prefix: v["prefix"], Repository: v["repository"], Ref: v["ref"], Branch: v["branch"], Message: message, Squash: squash}
			switch action {
			case "add", "add-commit":
				return m.repo.AddSubtree(ctx, opts)
			case "merge":
				return m.repo.MergeSubtree(ctx, opts)
			case "pull":
				return m.repo.PullSubtree(ctx, opts)
			case "push":
				return m.repo.PushSubtree(ctx, opts)
			case "split":
				return m.repo.SplitSubtree(ctx, opts)
			default:
				return errors.New("unsupported subtree operation")
			}
		}})
	}
}

type reviewedSparseChange struct {
	action   string
	patterns []string
	init     gitbackend.SparseCheckoutInitOptions
}

func sparseModeLabel(cone, sparseIndex bool) string {
	mode := "non-cone patterns"
	if cone {
		mode = "cone directories"
	}
	if sparseIndex {
		mode += ", sparse index"
	} else {
		mode += ", full index"
	}
	return mode
}

func sparseReview(state gitbackend.SparseCheckoutState, action string, patterns []string, init gitbackend.SparseCheckoutInitOptions) WorkflowReview {
	mode := "disabled"
	if state.Enabled {
		mode = "enabled (" + sparseModeLabel(state.Cone, state.SparseIndex) + ")"
	}
	plan := []string{"Current sparse checkout: " + mode, "Action: " + action}
	if action == "enable" {
		plan = append(plan, "New mode: "+sparseModeLabel(init.Cone, init.SparseIndex))
	}
	if len(state.Patterns) > 0 {
		plan = append(plan, "Current bounded pattern set: "+strings.Join(state.Patterns, ", "))
	}
	if len(patterns) > 0 {
		plan = append(plan, "Patterns: "+strings.Join(patterns, ", "))
	}
	confirmation := "Apply exactly this reviewed sparse-checkout change?"
	if action == "disable" {
		confirmation = "Disable sparse checkout and restore all tracked files?"
	} else if action == "set" {
		confirmation = "Replace the current sparse selection with exactly these reviewed paths?"
	}
	return WorkflowReview{Plan: plan, Confirmation: confirmation, Data: reviewedSparseChange{action: action, patterns: append([]string(nil), patterns...), init: init}}
}

func submitSparseReview(ctx context.Context, repo *gitbackend.Repository, review WorkflowReview) error {
	plan, ok := review.Data.(reviewedSparseChange)
	if !ok {
		return errors.New("invalid sparse-checkout review")
	}
	switch plan.action {
	case "enable":
		return repo.EnableSparseCheckout(ctx, plan.init)
	case "disable":
		return repo.DisableSparseCheckout(ctx)
	case "reapply":
		return repo.ReapplySparseCheckout(ctx)
	case "set":
		return repo.SetSparseCheckout(ctx, append([]string(nil), plan.patterns...))
	case "add":
		return repo.AddSparseCheckout(ctx, append([]string(nil), plan.patterns...))
	default:
		return errors.New("unsupported sparse-checkout operation")
	}
}

func sparseSimpleWorkflow(m *Model, command WorkflowCommand, action string) tea.Cmd {
	return m.LoadWorkflow("sparse-checkout state", func(ctx context.Context) (WorkflowDialog, error) {
		state, err := m.repo.SparseCheckoutState(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Sparse checkout: " + action, Operation: "sparse checkout " + action, ReviewPreflight: func(context.Context, WorkflowValues) (WorkflowReview, error) {
			return sparseReview(state, action, nil, gitbackend.SparseCheckoutInitOptions{}), nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			return submitSparseReview(ctx, m.repo, review)
		}}, nil
	})
}

func sparseEnableWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	sparseIndex := boolOption(command, "transient:magit-sparse-checkout:--sparse-index")
	return m.LoadWorkflow("sparse-checkout state", func(ctx context.Context) (WorkflowDialog, error) {
		state, err := m.repo.SparseCheckoutState(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Enable sparse checkout", Operation: "enable sparse checkout", Plan: []string{"Cone mode selects repository-relative directories", "Non-cone mode accepts Git ignore-style patterns", "Sparse index can improve performance in large repositories"}, Fields: []WorkflowField{{Name: "mode", Label: "Pattern mode", Kind: WorkflowSelect, Value: "cone", Choices: []WorkflowChoice{{Value: "cone", Label: "Cone directories (recommended)"}, {Value: "non-cone", Label: "Non-cone Git patterns (advanced)"}}}}, ReviewPreflight: func(_ context.Context, values WorkflowValues) (WorkflowReview, error) {
			opts := gitbackend.SparseCheckoutInitOptions{Cone: values["mode"] != "non-cone", SparseIndex: sparseIndex}
			return sparseReview(state, "enable", nil, opts), nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			return submitSparseReview(ctx, m.repo, review)
		}}, nil
	})
}
func sparseDisableWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return sparseSimpleWorkflow(m, command, "disable")
}
func sparseReapplyWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return sparseSimpleWorkflow(m, command, "reapply")
}

func parseSparsePatterns(value string) ([]string, error) {
	if len(value) > 4<<20 {
		return nil, errors.New("sparse patterns exceed 4 MiB")
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	patterns := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.ContainsRune(line, '\x00') {
			return nil, errors.New("sparse patterns contain NUL")
		}
		patterns = append(patterns, line)
		if len(patterns) > 100000 {
			return nil, errors.New("sparse pattern count exceeds 100000")
		}
	}
	if len(patterns) == 0 {
		return nil, errors.New("at least one sparse path is required")
	}
	return patterns, nil
}

func sparsePatternWorkflow(m *Model, command WorkflowCommand, action string) tea.Cmd {
	_ = command
	return m.LoadWorkflow("sparse-checkout patterns", func(ctx context.Context) (WorkflowDialog, error) {
		state, err := m.repo.SparseCheckoutState(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Sparse checkout: " + action, Operation: "sparse checkout " + action, Plan: []string{"Enter one repository-relative path per line"}, Fields: []WorkflowField{{Name: "patterns", Label: "Paths", Kind: WorkflowText, Required: true}}, Validate: func(v WorkflowValues) error { _, err := parseSparsePatterns(v["patterns"]); return err }, ReviewPreflight: func(_ context.Context, v WorkflowValues) (WorkflowReview, error) {
			patterns, err := parseSparsePatterns(v["patterns"])
			if err != nil {
				return WorkflowReview{}, err
			}
			return sparseReview(state, action, patterns, gitbackend.SparseCheckoutInitOptions{}), nil
		}, SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			return submitSparseReview(ctx, m.repo, review)
		}}, nil
	})
}

func sparseSetWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return sparsePatternWorkflow(m, command, "set")
}
func sparseAddWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return sparsePatternWorkflow(m, command, "add")
}
