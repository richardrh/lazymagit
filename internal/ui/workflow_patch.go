package ui

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const (
	amApplyMaildirID keymap.CommandID = "am.am-apply-maildir"
	amApplyPatchesID keymap.CommandID = "am.am-apply-patches"
	amContinueID     keymap.CommandID = "am.am-continue"
	amSkipID         keymap.CommandID = "am.am-skip"
	amAbortID        keymap.CommandID = "am.am-abort"
	patchApplyID     keymap.CommandID = "patch.patch-apply"
	patchCreateID    keymap.CommandID = "patch.patch-create"
	patchSaveID      keymap.CommandID = "patch.patch-save"

	patchPathLimit       = 4096
	patchInputLimit      = 64 << 10
	patchFileLimit       = 256
	patchRecipientLimit  = 128
	patchHeaderByteLimit = 998
)

func init() {
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-am", map[string][]string{
		"magit-am-apply-maildir": {"transient:magit-am:--3way", "transient:magit-am:--scissors", "magit:--signoff"},
		"magit-am-apply-patches": {"transient:magit-am:--3way", "transient:magit-am:--scissors", "magit:--signoff"},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-patch-apply", map[string][]string{
		"magit-patch-apply": {"transient:magit-patch-apply:--index", "transient:magit-patch-apply:--cached", "transient:magit-patch-apply:--3way"},
	})...)
	RegisterWorkflowCapabilities(capabilitiesForTransient("magit-patch-create", map[string][]string{
		"magit-patch-create": {"magit-format-patch:--in-reply-to", "magit-format-patch:--thread", "magit-format-patch:--from", "magit-format-patch:--to", "magit-format-patch:--cc", "magit-format-patch:--base", "magit-format-patch:--reroll-count", "magit-format-patch:--subject-prefix", "transient:magit-patch-create:--rfc", "transient:magit-patch-create:--cover-letter", "magit-format-patch:--output-directory"},
	})...)
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		handlers := map[keymap.CommandID]WorkflowHandler{
			amApplyMaildirID: amStartWorkflow,
			amApplyPatchesID: amStartWorkflow,
			amContinueID:     amControlWorkflow("continue", (*gitbackend.Repository).AMContinue),
			amSkipID:         amControlWorkflow("skip", (*gitbackend.Repository).AMSkip),
			amAbortID:        amControlWorkflow("abort", (*gitbackend.Repository).AMAbort),
			patchApplyID:     applyPatchWorkflow,
			patchCreateID:    formatPatchWorkflow,
			patchSaveID:      savePatchWorkflow,
		}
		// magit-patch-create is both the W c recursive transient edge and the
		// child's final c suffix. The terminal adaptation opens the same exact,
		// typed format-patch workflow at either occurrence.
		for _, binding := range keymap.Registry() {
			switch binding.UpstreamCommand {
			case "magit-patch-create":
				handlers[binding.Command] = formatPatchWorkflow
			case "magit-patch-apply":
				handlers[binding.Command] = applyPatchWorkflow
			}
		}
		return handlers
	})
}

func amControlWorkflow(name string, operation func(*gitbackend.Repository, context.Context) error) WorkflowHandler {
	return func(m *Model, _ WorkflowCommand) tea.Cmd {
		return m.StartWorkflowOperation("am "+name, func(ctx context.Context) error { return operation(m.repo, ctx) })
	}
}

func amStartWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	options, err := amOptions(command.Options)
	if err != nil {
		m.setError(err)
		return nil
	}
	title := "Apply patch series"
	if command.ID == amApplyMaildirID {
		title = "Apply maildir"
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: title, Operation: "am start", Confirmation: "Apply commits from these paths",
		Fields: []WorkflowField{
			{Name: "paths", Label: "Patch paths (one per line)", Kind: WorkflowText, Required: true},
			{Name: "keep-cr", Label: "Keep CR line endings", Kind: WorkflowBool, Bool: options.KeepCR},
		},
		Validate: func(values WorkflowValues) error {
			_, err := patchPaths(values["paths"])
			return err
		},
		ReviewPreflight: func(_ context.Context, values WorkflowValues) (WorkflowReview, error) {
			paths, err := patchPaths(values["paths"])
			if err != nil {
				return WorkflowReview{}, err
			}
			runOptions := options
			runOptions.KeepCR = values["keep-cr"] == "true"
			reviewed, err := m.repo.ReviewAMStart(paths, runOptions)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := make([]string, 0, len(reviewed.Inputs)+1)
			for _, input := range reviewed.Inputs {
				plan = append(plan, fmt.Sprintf("input: %s (%d bytes, sha256 %s)", input.Path, input.Size, input.Digest))
			}
			plan = append(plan, "Git will create commits from these reviewed inputs")
			return WorkflowReview{Plan: plan, Confirmation: "Apply commits from the reviewed patch inputs", Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			reviewed, ok := review.Data.(gitbackend.ReviewedAMStart)
			if !ok {
				return errors.New("git am review is invalid")
			}
			return m.repo.ExecuteReviewedAMStart(ctx, reviewed)
		},
	})
}

func applyPatchWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	index := optionEnabled(command.Options, "transient:magit-patch-apply:--index")
	cached := optionEnabled(command.Options, "transient:magit-patch-apply:--cached")
	threeWay := optionEnabled(command.Options, "transient:magit-patch-apply:--3way") || optionEnabled(command.Options, "transient:magit-am:--3way")
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Apply patch", Operation: "apply patch", Confirmation: "Apply this file without creating a commit",
		Fields: []WorkflowField{
			{Name: "path", Label: "Patch file", Kind: WorkflowText, Required: true},
			{Name: "index", Label: "Apply to worktree and index", Kind: WorkflowBool, Bool: index},
			{Name: "cached", Label: "Apply to index only", Kind: WorkflowBool, Bool: cached},
			{Name: "three-way", Label: "Use three-way merge", Kind: WorkflowBool, Bool: threeWay},
		},
		Validate: func(values WorkflowValues) error {
			if err := validPatchPath(values["path"]); err != nil {
				return err
			}
			if values["index"] == "true" && values["cached"] == "true" {
				return errors.New("index and cached modes are mutually exclusive")
			}
			return nil
		},
		ReviewPreflight: func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
			options := gitbackend.ApplyPatchOptions{Index: values["index"] == "true", Cached: values["cached"] == "true", ThreeWay: values["three-way"] == "true"}
			reviewed, err := m.repo.ReviewApplyPatch(ctx, values["path"], options)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := []string{fmt.Sprintf("patch: %s (%d bytes, sha256 %s)", reviewed.Filename, reviewed.Size, reviewed.Digest), "Git verified this patch against the current repository"}
			return WorkflowReview{Plan: plan, Confirmation: "Apply the reviewed patch", Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			reviewed, ok := review.Data.(gitbackend.ReviewedApplyPatch)
			if !ok {
				return errors.New("patch application review is invalid")
			}
			return m.repo.ExecuteReviewedApplyPatch(ctx, reviewed)
		},
	})
}

func formatPatchWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	defaults, err := formatPatchOptions(command.Options)
	if err != nil {
		m.setError(err)
		return nil
	}
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Format patches", Operation: "format patches", Confirmation: "Review the exact bounded patch series before publishing it",
		Fields: []WorkflowField{
			{Name: "range", Label: "Revision range", Kind: WorkflowText, Value: "HEAD~1..HEAD", Required: true},
			{Name: "directory", Label: "Output directory", Kind: WorkflowText, Value: defaults.OutputDirectory, Required: true},
			{Name: "numbered", Label: "Number patches", Kind: WorkflowBool, Bool: defaults.Numbered},
			{Name: "cover", Label: "Create cover letter", Kind: WorkflowBool, Bool: defaults.CoverLetter},
			{Name: "cover-message", Label: "Cover letter message (internal editor, max 64 KiB)", Kind: WorkflowMultiline, Value: defaults.CoverLetterBody},
			{Name: "signoff", Label: "Add signoff", Kind: WorkflowBool, Bool: defaults.Signoff},
			{Name: "thread", Label: "Thread messages", Kind: WorkflowBool, Bool: defaults.Thread},
			{Name: "subject", Label: "Subject prefix", Kind: WorkflowText, Value: defaults.SubjectPrefix},
			{Name: "reroll", Label: "Reroll count", Kind: WorkflowText, Value: strconv.Itoa(defaults.RerollCount)},
			{Name: "start", Label: "Start number (0 for Git default)", Kind: WorkflowText, Value: strconv.Itoa(defaults.StartNumber)},
			{Name: "from", Label: "From identity", Kind: WorkflowText, Value: defaults.From},
			{Name: "in-reply-to", Label: "In-Reply-To message ID", Kind: WorkflowText, Value: defaults.InReplyTo},
			{Name: "base", Label: "Base commit (optional)", Kind: WorkflowText, Value: defaults.Base},
			{Name: "to", Label: "To recipients (comma-separated)", Kind: WorkflowText, Value: strings.Join(defaults.To, ", ")},
			{Name: "cc", Label: "Cc recipients (comma-separated)", Kind: WorkflowText, Value: strings.Join(defaults.Cc, ", ")},
		},
		Validate: func(values WorkflowValues) error {
			if err := validBoundedText("revision range", values["range"]); err != nil {
				return err
			}
			if err := validPatchPath(values["directory"]); err != nil {
				return err
			}
			if err := patchHeader("subject prefix", values["subject"], true); err != nil {
				return err
			}
			if err := patchCoverMessage(values["cover-message"]); err != nil {
				return err
			}
			if err := patchMailAddress("From identity", values["from"], true); err != nil {
				return err
			}
			if err := patchMessageID(values["in-reply-to"]); err != nil {
				return err
			}
			if values["base"] != "" {
				if err := validBoundedText("base commit", values["base"]); err != nil {
					return err
				}
			}
			if _, err := patchNonNegative("reroll count", values["reroll"]); err != nil {
				return err
			}
			if _, err := patchNonNegative("start number", values["start"]); err != nil {
				return err
			}
			if _, err := patchAddresses("To recipients", values["to"]); err != nil {
				return err
			}
			_, err := patchAddresses("Cc recipients", values["cc"])
			return err
		},
		ReviewPreflight: func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
			options, err := formatPatchOptionsFromValues(defaults, values)
			if err != nil {
				return WorkflowReview{}, err
			}
			reviewed, err := m.repo.ReviewFormatPatchUI(ctx, values["range"], options)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := []string{fmt.Sprintf("destination: %s", reviewed.Directory), fmt.Sprintf("commits: %d", len(reviewed.Revisions))}
			for _, file := range reviewed.Files {
				plan = append(plan, fmt.Sprintf("create: %s (%d bytes, sha256 %s)", file.Name, file.Size, file.Digest))
			}
			return WorkflowReview{Plan: plan, Confirmation: "Publish exactly these reviewed patch files", Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			reviewed, ok := review.Data.(gitbackend.ReviewedFormatPatch)
			if !ok {
				return errors.New("format-patch review is invalid")
			}
			return m.repo.ExecuteReviewedFormatPatch(ctx, reviewed)
		},
		OnCancel: func() {
			if m.workflow != nil && m.workflow.review != nil {
				if reviewed, ok := m.workflow.review.Data.(gitbackend.ReviewedFormatPatch); ok {
					m.repo.DiscardReviewedFormatPatch(reviewed)
				}
			}
		},
	})
}

func savePatchWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{
		Title: "Save diff as patch", Operation: "save patch",
		Fields: []WorkflowField{
			{Name: "path", Label: "Output file", Kind: WorkflowText, Required: true},
			{Name: "range", Label: "Revision range (optional)", Kind: WorkflowText},
			{Name: "cached", Label: "Save staged changes", Kind: WorkflowBool},
			{Name: "overwrite", Label: "Overwrite existing regular file", Kind: WorkflowBool},
		},
		Validate: func(values WorkflowValues) error {
			if err := validPatchPath(values["path"]); err != nil {
				return err
			}
			if values["range"] != "" {
				return validBoundedText("revision range", values["range"])
			}
			return nil
		},
		ReviewPreflight: func(_ context.Context, values WorkflowValues) (WorkflowReview, error) {
			options := gitbackend.DiffPatchOptions{Cached: values["cached"] == "true", Range: values["range"], Overwrite: values["overwrite"] == "true"}
			reviewed, err := m.repo.ReviewDiffPatch(values["path"], options)
			if err != nil {
				return WorkflowReview{}, err
			}
			plan := []string{"destination: " + reviewed.Filename, "source: working tree diff"}
			confirmation := "Create patch file"
			if reviewed.Exists {
				plan = append(plan, fmt.Sprintf("existing regular file: %d bytes, sha256 %s", reviewed.Size, reviewed.Digest))
				if !options.Overwrite {
					return WorkflowReview{}, errors.New("output exists; enable overwrite to replace it")
				}
				confirmation = "Confirm atomic replacement of the reviewed file"
			}
			return WorkflowReview{Plan: plan, Confirmation: confirmation, Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			reviewed, ok := review.Data.(gitbackend.ReviewedDiffPatch)
			if !ok {
				return errors.New("patch output review is invalid")
			}
			return m.repo.ExecuteReviewedDiffPatch(ctx, reviewed)
		},
	})
}

func amOptions(options map[keymap.CommandID]OptionValue) (gitbackend.AMOptions, error) {
	var out gitbackend.AMOptions
	for id, value := range options {
		if !value.Enabled && value.Value == "" {
			continue
		}
		upstream, belongs := patchOptionUpstream(id)
		if !belongs {
			continue
		}
		if err := applyAMOption(&out, upstream); err != nil {
			return out, err
		}
	}
	return out, nil
}

func applyAMOption(out *gitbackend.AMOptions, upstream string) error {
	flags := map[string]*bool{
		"transient:magit-am:--3way":     &out.ThreeWay,
		"transient:magit-am:--scissors": &out.Scissors,
		"magit:--signoff":               &out.Signoff,
	}
	flag, ok := flags[upstream]
	if !ok {
		return fmt.Errorf("%s is unavailable: the patch backend does not safely support this option", upstream)
	}
	*flag = true
	return nil
}

type formatPatchOptionSetter func(*gitbackend.FormatPatchOptions, OptionValue) error

var formatPatchOptionSetters = map[string]formatPatchOptionSetter{
	"magit-format-patch:--in-reply-to":            setFormatPatchInReplyTo,
	"magit-format-patch:--thread":                 setFormatPatchThread,
	"magit-format-patch:--from":                   setFormatPatchFrom,
	"magit-format-patch:--to":                     setFormatPatchTo,
	"magit-format-patch:--cc":                     setFormatPatchCc,
	"magit-format-patch:--base":                   setFormatPatchBase,
	"magit-format-patch:--reroll-count":           setFormatPatchRerollCount,
	"magit-format-patch:--subject-prefix":         setFormatPatchSubjectPrefix,
	"transient:magit-patch-create:--rfc":          setFormatPatchRFC,
	"transient:magit-patch-create:--cover-letter": setFormatPatchCoverLetter,
	"magit-format-patch:--output-directory":       setFormatPatchOutputDirectory,
}

func formatPatchOptions(values map[keymap.CommandID]OptionValue) (gitbackend.FormatPatchOptions, error) {
	out := gitbackend.FormatPatchOptions{OutputDirectory: "."}
	for id, value := range values {
		if !value.Enabled && value.Value == "" {
			continue
		}
		upstream, belongs := patchOptionUpstream(id)
		if !belongs {
			continue
		}
		setter, ok := formatPatchOptionSetters[upstream]
		if !ok {
			return out, fmt.Errorf("%s is unavailable: the format-patch backend does not safely support this option", upstream)
		}
		if err := setter(&out, value); err != nil {
			return out, err
		}
	}
	return out, nil
}

func setFormatPatchInReplyTo(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	if err := patchMessageID(value.Value); err != nil {
		return err
	}
	out.InReplyTo = value.Value
	return nil
}

func setFormatPatchThread(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	style, err := patchThreadStyle(value.Value)
	if err != nil {
		return err
	}
	out.Thread, out.ThreadStyle = true, style
	return nil
}

func setFormatPatchFrom(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	if err := patchMailAddress("From identity", value.Value, false); err != nil {
		return err
	}
	out.From = value.Value
	return nil
}

func setFormatPatchTo(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	addresses, err := patchAddresses("To recipients", value.Value)
	if err != nil {
		return err
	}
	out.To = addresses
	return nil
}

func setFormatPatchCc(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	addresses, err := patchAddresses("Cc recipients", value.Value)
	if err != nil {
		return err
	}
	out.Cc = addresses
	return nil
}

func setFormatPatchBase(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	if err := validBoundedText("base commit", value.Value); err != nil {
		return err
	}
	out.Base = value.Value
	return nil
}

func setFormatPatchRerollCount(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	count, err := patchNonNegative("reroll count", value.Value)
	if err != nil {
		return err
	}
	out.RerollCount = count
	return nil
}

func setFormatPatchSubjectPrefix(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	if err := patchHeader("subject prefix", value.Value, false); err != nil {
		return err
	}
	out.SubjectPrefix = value.Value
	return nil
}

func setFormatPatchRFC(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	out.RFC = value.Enabled
	return nil
}

func setFormatPatchCoverLetter(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	out.CoverLetter = value.Enabled
	return nil
}

func setFormatPatchOutputDirectory(out *gitbackend.FormatPatchOptions, value OptionValue) error {
	if err := validPatchPath(value.Value); err != nil {
		return err
	}
	out.OutputDirectory = value.Value
	return nil
}

func formatPatchOptionsFromValues(defaults gitbackend.FormatPatchOptions, values WorkflowValues) (gitbackend.FormatPatchOptions, error) {
	out := defaults
	var err error
	out.OutputDirectory, out.Numbered, out.CoverLetter, out.Signoff, out.Thread, out.SubjectPrefix = values["directory"], values["numbered"] == "true", values["cover"] == "true", values["signoff"] == "true", values["thread"] == "true", values["subject"]
	out.CoverLetterBody, out.From, out.InReplyTo, out.Base = values["cover-message"], values["from"], values["in-reply-to"], values["base"]
	if !out.Thread {
		out.ThreadStyle = ""
	}
	if out.CoverLetterBody != "" {
		out.CoverLetter = true
	}
	if out.RerollCount, err = patchNonNegative("reroll count", values["reroll"]); err != nil {
		return out, err
	}
	if out.StartNumber, err = patchNonNegative("start number", values["start"]); err != nil {
		return out, err
	}
	if out.To, err = patchAddresses("To recipients", values["to"]); err != nil {
		return out, err
	}
	out.Cc, err = patchAddresses("Cc recipients", values["cc"])
	if err != nil {
		return out, err
	}
	if err := patchHeader("subject prefix", out.SubjectPrefix, true); err != nil {
		return out, err
	}
	if err := patchCoverMessage(out.CoverLetterBody); err != nil {
		return out, err
	}
	if err := patchMailAddress("From identity", out.From, true); err != nil {
		return out, err
	}
	if err := patchMessageID(out.InReplyTo); err != nil {
		return out, err
	}
	if out.Base != "" {
		if err := validBoundedText("base commit", out.Base); err != nil {
			return out, err
		}
	}
	return out, nil
}

func patchNonNegative(name, value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is empty", name)
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("%s must be a non-negative whole number", name)
	}
	return number, nil
}

func patchAddresses(name, value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	if err := patchHeader(name, value, false); err != nil {
		return nil, err
	}
	parsed, err := mail.ParseAddressList(value)
	if err != nil || len(parsed) == 0 || len(parsed) > patchRecipientLimit {
		return nil, fmt.Errorf("%s must contain at most %d valid recipients", name, patchRecipientLimit)
	}
	addresses := make([]string, 0, len(parsed))
	for _, address := range parsed {
		if address.Name == "" {
			addresses = append(addresses, address.Address)
		} else {
			addresses = append(addresses, address.String())
		}
	}
	return addresses, nil
}

func patchMailAddress(name, value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if err := patchHeader(name, value, false); err != nil {
		return err
	}
	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("%s is not a valid recipient", name)
	}
	return nil
}

func patchMessageID(value string) error {
	if value == "" {
		return nil
	}
	if err := patchHeader("In-Reply-To", value, false); err != nil {
		return err
	}
	if !strings.HasPrefix(value, "<") || !strings.HasSuffix(value, ">") || strings.Count(value, "@") != 1 || strings.ContainsAny(value[1:len(value)-1], " <>\t") {
		return errors.New("In-Reply-To must be a message ID such as <id@example.test>")
	}
	return nil
}

func patchThreadStyle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "shallow" || value == "deep" {
		return value, nil
	}
	return "", errors.New("thread style must be shallow or deep")
}

func patchCoverMessage(value string) error {
	if len(value) > patchInputLimit || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || strings.ContainsRune(value, '\r') {
		return errors.New("cover letter message is invalid or exceeds 64 KiB")
	}
	for _, r := range value {
		if r < 32 && r != '\n' || r == 127 {
			return errors.New("cover letter message contains a control character")
		}
	}
	return nil
}

func patchHeader(name, value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if value == "" || len(value) > patchHeaderByteLimit || !utf8.ValidString(value) {
		return fmt.Errorf("%s is empty, invalid, or too long", name)
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return fmt.Errorf("%s contains a control character", name)
		}
	}
	return nil
}

func optionEnabled(options map[keymap.CommandID]OptionValue, upstream string) bool {
	for id, value := range options {
		if candidate, ok := patchOptionUpstream(id); ok && candidate == upstream && value.Enabled {
			return true
		}
	}
	return false
}

func patchOptionUpstream(id keymap.CommandID) (string, bool) {
	for _, binding := range keymap.Registry() {
		if binding.Command == id && binding.Kind == keymap.KindInfix {
			return binding.UpstreamCommand, true
		}
	}
	return "", false
}

func patchPaths(value string) ([]string, error) {
	if len(value) > patchInputLimit {
		return nil, errors.New("patch path input exceeds 64 KiB")
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if err := validPatchPath(path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
		if len(paths) > patchFileLimit {
			return nil, fmt.Errorf("patch series exceeds %d input paths", patchFileLimit)
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("at least one patch path is required")
	}
	return paths, nil
}

func validPatchPath(value string) error {
	if err := validBoundedText("path", value); err != nil {
		return err
	}
	if len(value) > patchPathLimit {
		return fmt.Errorf("path exceeds %d bytes", patchPathLimit)
	}
	return nil
}

func validBoundedText(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if len(value) > patchInputLimit || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s is invalid or too long", name)
	}
	return nil
}
