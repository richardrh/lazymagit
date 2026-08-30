package git

import (
	"context"
	"errors"
)

// CommitUIVariant identifies the non-interactive commit operation selected by
// the UI. It is intentionally limited to workflows that can be represented
// without handing control to an external editor.
type CommitUIVariant string

const (
	CommitUICreate  CommitUIVariant = "create"
	CommitUIExtend  CommitUIVariant = "extend"
	CommitUIAmend   CommitUIVariant = "amend"
	CommitUIReword  CommitUIVariant = "reword"
	CommitUIFixup   CommitUIVariant = "fixup"
	CommitUISquash  CommitUIVariant = "squash"
	CommitUIAlter   CommitUIVariant = "alter"
	CommitUIAugment CommitUIVariant = "augment"
	CommitUIRevise  CommitUIVariant = "revise"
)

var ErrCommitSigningConsentRequired = errors.New("commit signing requires explicit consent")

type commitUIInvocation struct {
	args          []string
	editorMessage string
}

// ExecuteCommitUI is the safe UI boundary for commit workflows. In particular,
// an unsigned invocation explicitly disables commit.gpgSign, rather than
// accidentally inheriting it from repository or user configuration.
func (r *Repository) ExecuteCommitUI(ctx context.Context, variant CommitUIVariant, target, message string, options CommitOptions, signingConsent bool) (Commit, error) {
	signingRequested, err := prepareCommitUIOptions(variant, &options, signingConsent)
	if err != nil {
		return Commit{}, err
	}
	invocation, err := r.commitUIInvocation(ctx, variant, target, message, options)
	if err != nil {
		return Commit{}, err
	}
	if !signingRequested {
		// Git's explicit negation overrides commit.gpgSign without changing the
		// command shape consumed by process recording/redaction.
		invocation.args = append(invocation.args, "--no-gpg-sign")
	}
	if invocation.editorMessage != "" {
		return r.runCommitWithEditor(ctx, invocation.args, invocation.editorMessage)
	}
	return r.runCommit(ctx, invocation.args)
}

func prepareCommitUIOptions(variant CommitUIVariant, options *CommitOptions, signingConsent bool) (bool, error) {
	signingRequested := options.Sign || options.SigningKey != ""
	if signingRequested && !signingConsent {
		return false, ErrCommitSigningConsentRequired
	}
	options.AllowInteractiveSigning = signingRequested && signingConsent
	if options.ReeditMessage == "" || allowsCommitUIReedit(variant) {
		return signingRequested, nil
	}
	return false, &CommitOptionError{Message: string(variant) + " commit cannot reedit another commit message"}
}

func allowsCommitUIReedit(variant CommitUIVariant) bool {
	switch variant {
	case CommitUICreate, CommitUIExtend, CommitUIAmend, CommitUIReword:
		return true
	default:
		return false
	}
}

func (r *Repository) commitUIInvocation(ctx context.Context, variant CommitUIVariant, target, message string, options CommitOptions) (commitUIInvocation, error) {
	switch variant {
	case CommitUICreate, CommitUIAmend, CommitUIReword:
		return r.standardCommitUIInvocation(ctx, variant, message, options)
	case CommitUIExtend:
		return r.extendCommitUIInvocation(ctx, message, options)
	case CommitUIFixup, CommitUISquash, CommitUIAlter, CommitUIAugment, CommitUIRevise:
		return r.autosquashCommitUIInvocation(ctx, variant, target, message, options)
	default:
		return commitUIInvocation{}, &CommitOptionError{Message: "unknown commit workflow " + string(variant)}
	}
}

func (r *Repository) standardCommitUIInvocation(ctx context.Context, variant CommitUIVariant, message string, options CommitOptions) (commitUIInvocation, error) {
	initial := []string(nil)
	operation := string(variant)
	if variant == CommitUIAmend {
		initial = []string{"--amend"}
	}
	if variant == CommitUIReword {
		if options.All {
			return commitUIInvocation{}, &CommitOptionError{Message: "reword commit cannot use all while preserving the staged tree"}
		}
		initial = []string{"--amend", "--only", "--allow-empty"}
	}
	args, err := r.commitArgs(ctx, operation, initial, options)
	if err == nil {
		err = requireCommitMessage(operation+" commit", message, options)
	}
	if err != nil {
		return commitUIInvocation{}, err
	}
	if options.ReeditMessage != "" {
		return commitUIInvocation{args: args, editorMessage: message}, nil
	}
	return commitUIInvocation{args: appendMessage(args, message)}, nil
}

func (r *Repository) extendCommitUIInvocation(ctx context.Context, message string, options CommitOptions) (commitUIInvocation, error) {
	if options.ReuseMessage != "" {
		return commitUIInvocation{}, &CommitOptionError{Message: "extend commit cannot reuse another message"}
	}
	if options.ReeditMessage != "" && message == "" {
		return commitUIInvocation{}, &EditorRequiredError{Operation: "extend commit with reedit-message"}
	}
	initial := []string{"--amend", "--no-edit"}
	invocation := commitUIInvocation{}
	if options.ReeditMessage != "" {
		initial = []string{"--amend"}
		invocation.editorMessage = message
	}
	args, err := r.commitArgs(ctx, "extend", initial, options)
	invocation.args = args
	return invocation, err
}

func (r *Repository) autosquashCommitUIInvocation(ctx context.Context, variant CommitUIVariant, target, message string, options CommitOptions) (commitUIInvocation, error) {
	oid, err := r.resolveCommitForCommitWorkflow(ctx, target)
	if err != nil {
		return commitUIInvocation{}, err
	}
	switch variant {
	case CommitUIFixup:
		return r.fixupCommitUIInvocation(ctx, oid, options)
	case CommitUISquash, CommitUIAugment:
		return r.squashCommitUIInvocation(ctx, variant, oid, message, options)
	case CommitUIAlter, CommitUIRevise:
		return r.structuredFixupCommitUIInvocation(ctx, variant, oid, message, options)
	default:
		return commitUIInvocation{}, &CommitOptionError{Message: "unknown commit workflow " + string(variant)}
	}
}

func (r *Repository) fixupCommitUIInvocation(ctx context.Context, oid string, options CommitOptions) (commitUIInvocation, error) {
	if options.ReuseMessage != "" {
		return commitUIInvocation{}, &CommitOptionError{Message: "fixup commit cannot reuse another message"}
	}
	args, err := r.commitArgs(ctx, "fixup", []string{"--fixup=" + oid}, options)
	return commitUIInvocation{args: args}, err
}

func (r *Repository) squashCommitUIInvocation(ctx context.Context, variant CommitUIVariant, oid, message string, options CommitOptions) (commitUIInvocation, error) {
	if options.ReuseMessage != "" {
		return commitUIInvocation{}, &CommitOptionError{Message: "squash commit cannot reuse another message"}
	}
	if variant == CommitUIAugment && message == "" {
		return commitUIInvocation{}, &EditorRequiredError{Operation: "augment commit"}
	}
	args, err := r.commitArgs(ctx, "squash", []string{"--squash=" + oid}, options)
	return commitUIInvocation{args: appendMessage(args, message)}, err
}

func (r *Repository) structuredFixupCommitUIInvocation(ctx context.Context, variant CommitUIVariant, oid, message string, options CommitOptions) (commitUIInvocation, error) {
	if err := validateStructuredFixupInvocation(variant, message, options); err != nil {
		return commitUIInvocation{}, err
	}
	if err := r.requireStructuredFixup(ctx, string(variant)); err != nil {
		return commitUIInvocation{}, err
	}
	subject, err := r.output(ctx, "show", "-s", "--format=%s", oid)
	if err != nil {
		return commitUIInvocation{}, err
	}
	mode := "amend"
	if variant == CommitUIRevise {
		mode = "reword"
	}
	args, err := r.commitArgs(ctx, string(variant), []string{"--fixup=" + mode + ":" + oid}, options)
	return commitUIInvocation{args: args, editorMessage: "amend! " + trimLine(subject) + "\n\n" + message + "\n"}, err
}

func validateStructuredFixupInvocation(variant CommitUIVariant, message string, options CommitOptions) error {
	if variant == CommitUIRevise && options.All {
		return &CommitOptionError{Message: "revise commit cannot use all while preserving the staged tree"}
	}
	if message == "" {
		return &EditorRequiredError{Operation: string(variant) + " commit"}
	}
	if options.ReuseMessage != "" || options.ReeditMessage != "" {
		return &CommitOptionError{Message: string(variant) + " commit cannot combine an autosquash message and reuse-message"}
	}
	return nil
}
