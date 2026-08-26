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

// ExecuteCommitUI is the safe UI boundary for commit workflows. In particular,
// an unsigned invocation explicitly disables commit.gpgSign, rather than
// accidentally inheriting it from repository or user configuration.
func (r *Repository) ExecuteCommitUI(ctx context.Context, variant CommitUIVariant, target, message string, options CommitOptions, signingConsent bool) (Commit, error) {
	signingRequested := options.Sign || options.SigningKey != ""
	if signingRequested && !signingConsent {
		return Commit{}, ErrCommitSigningConsentRequired
	}
	options.AllowInteractiveSigning = signingRequested && signingConsent

	var args []string
	var editorMessage string
	var err error
	switch variant {
	case CommitUICreate:
		args, err = r.commitArgs(ctx, "create", nil, options)
		if err == nil {
			err = requireCommitMessage("create commit", message, options)
		}
		args = appendMessage(args, message)
	case CommitUIExtend:
		if options.ReuseMessage != "" {
			err = &CommitOptionError{Message: "extend commit cannot reuse another message"}
			break
		}
		args, err = r.commitArgs(ctx, "extend", []string{"--amend", "--no-edit"}, options)
	case CommitUIAmend:
		args, err = r.commitArgs(ctx, "amend", []string{"--amend"}, options)
		if err == nil {
			err = requireCommitMessage("amend commit", message, options)
		}
		args = appendMessage(args, message)
	case CommitUIReword:
		if options.All {
			err = &CommitOptionError{Message: "reword commit cannot use all while preserving the staged tree"}
			break
		}
		args, err = r.commitArgs(ctx, "reword", []string{"--amend", "--only", "--allow-empty"}, options)
		if err == nil {
			err = requireCommitMessage("reword commit", message, options)
		}
		args = appendMessage(args, message)
	case CommitUIFixup, CommitUISquash, CommitUIAlter, CommitUIAugment, CommitUIRevise:
		var oid string
		oid, err = r.resolveCommitForCommitWorkflow(ctx, target)
		if err != nil {
			break
		}
		switch variant {
		case CommitUIFixup:
			if options.ReuseMessage != "" {
				err = &CommitOptionError{Message: "fixup commit cannot reuse another message"}
				break
			}
			args, err = r.commitArgs(ctx, "fixup", []string{"--fixup=" + oid}, options)
		case CommitUISquash, CommitUIAugment:
			if options.ReuseMessage != "" {
				err = &CommitOptionError{Message: "squash commit cannot reuse another message"}
				break
			}
			if variant == CommitUIAugment && message == "" {
				err = &EditorRequiredError{Operation: "augment commit"}
				break
			}
			args, err = r.commitArgs(ctx, "squash", []string{"--squash=" + oid}, options)
			args = appendMessage(args, message)
		case CommitUIAlter, CommitUIRevise:
			if variant == CommitUIRevise && options.All {
				err = &CommitOptionError{Message: "revise commit cannot use all while preserving the staged tree"}
				break
			}
			if message == "" {
				err = &EditorRequiredError{Operation: string(variant) + " commit"}
				break
			}
			if options.ReuseMessage != "" || options.ReeditMessage != "" {
				err = &CommitOptionError{Message: string(variant) + " commit cannot combine an autosquash message and reuse-message"}
				break
			}
			if err = r.requireStructuredFixup(ctx, string(variant)); err != nil {
				break
			}
			var subject []byte
			subject, err = r.output(ctx, "show", "-s", "--format=%s", oid)
			if err != nil {
				break
			}
			mode := "amend"
			if variant == CommitUIRevise {
				mode = "reword"
			}
			args, err = r.commitArgs(ctx, string(variant), []string{"--fixup=" + mode + ":" + oid}, options)
			editorMessage = "amend! " + trimLine(subject) + "\n\n" + message + "\n"
		}
	default:
		err = &CommitOptionError{Message: "unknown commit workflow " + string(variant)}
	}
	if err != nil {
		return Commit{}, err
	}
	if !signingRequested {
		// Git's explicit negation overrides commit.gpgSign without changing the
		// command shape consumed by process recording/redaction.
		args = append(args, "--no-gpg-sign")
	}
	if editorMessage != "" {
		return r.runCommitWithEditor(ctx, args, editorMessage)
	}
	return r.runCommit(ctx, args)
}
