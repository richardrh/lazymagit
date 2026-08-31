package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const (
	commandArgvField     = "argv"
	commandExternalField = "external-git-confirmed"
)

var (
	rawGitCommandIDs []keymap.CommandID
	rawRunCommandIDs []keymap.CommandID
)

type commandArgvQuoteMode uint8

const (
	commandArgvUnquoted commandArgvQuoteMode = iota
	commandArgvSingleQuoted
	commandArgvDoubleQuoted
)

type commandArgvParser struct {
	argv             []string
	word             strings.Builder
	mode             commandArgvQuoteMode
	escaped, started bool
}

func init() {
	for _, binding := range keymap.Registry() {
		if binding.Context == keymap.ContextStatus && binding.UpstreamCommand == "magit-git-command" {
			rawGitCommandIDs = appendUniqueCommandID(rawGitCommandIDs, binding.Command)
		}
		if binding.Transient == "magit-run" && binding.Kind == keymap.KindSuffix {
			switch binding.UpstreamCommand {
			case "magit-git-command-topdir", "magit-git-command":
				rawGitCommandIDs = appendUniqueCommandID(rawGitCommandIDs, binding.Command)
			case "magit-shell-command-topdir", "magit-shell-command":
				rawRunCommandIDs = appendUniqueCommandID(rawRunCommandIDs, binding.Command)
			}
		}
	}
	RegisterWorkflowDomain(func(m *Model) map[keymap.CommandID]WorkflowHandler {
		handlers := make(map[keymap.CommandID]WorkflowHandler, len(rawGitCommandIDs)+len(rawRunCommandIDs))
		for _, id := range rawGitCommandIDs {
			handlers[id] = func(_ *Model, _ WorkflowCommand) tea.Cmd { return openRawCommandWorkflow(m, false) }
		}
		for _, id := range rawRunCommandIDs {
			handlers[id] = func(_ *Model, _ WorkflowCommand) tea.Cmd { return openRawCommandWorkflow(m, true) }
		}
		return handlers
	})
}

func appendUniqueCommandID(ids []keymap.CommandID, id keymap.CommandID) []keymap.CommandID {
	if id == keymap.CommandNone {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// parseCommandArgv implements an argv grammar, not shell syntax. Unquoted
// Unicode whitespace separates arguments; single and double quotes group text;
// and backslash quotes exactly the next rune outside quotes and in double
// quotes. Backslash is literal in single quotes. Quotes and backslashes are
// removed. There is intentionally no variable, command, tilde, glob, comment,
// operator, redirect, or semicolon processing.
func parseCommandArgv(input string) ([]string, error) {
	parser := commandArgvParser{}
	for _, r := range input {
		parser.consume(r)
	}
	return parser.finish()
}

func (p *commandArgvParser) consume(r rune) {
	if p.escaped {
		p.word.WriteRune(r)
		p.escaped, p.started = false, true
		return
	}
	switch p.mode {
	case commandArgvSingleQuoted:
		p.consumeSingleQuoted(r)
	case commandArgvDoubleQuoted:
		p.consumeDoubleQuoted(r)
	default:
		p.consumeUnquoted(r)
	}
}

func (p *commandArgvParser) consumeSingleQuoted(r rune) {
	if r == '\'' {
		p.mode = commandArgvUnquoted
	} else {
		p.word.WriteRune(r)
	}
	p.started = true
}

func (p *commandArgvParser) consumeDoubleQuoted(r rune) {
	switch r {
	case '"':
		p.mode = commandArgvUnquoted
	case '\\':
		p.escaped = true
	default:
		p.word.WriteRune(r)
	}
	p.started = true
}

func (p *commandArgvParser) consumeUnquoted(r rune) {
	switch {
	case unicode.IsSpace(r):
		p.flush()
	case r == '\'':
		p.mode, p.started = commandArgvSingleQuoted, true
	case r == '"':
		p.mode, p.started = commandArgvDoubleQuoted, true
	case r == '\\':
		p.escaped, p.started = true, true
	default:
		p.word.WriteRune(r)
		p.started = true
	}
}

func (p *commandArgvParser) finish() ([]string, error) {
	if p.escaped {
		return nil, errors.New("argv ends with an incomplete backslash escape")
	}
	if p.mode != commandArgvUnquoted {
		return nil, errors.New("argv contains an unterminated quote")
	}
	p.flush()
	if len(p.argv) == 0 {
		return nil, errors.New("command argv is required")
	}
	return p.argv, nil
}

func (p *commandArgvParser) flush() {
	if p.started {
		p.argv = append(p.argv, p.word.String())
		p.word.Reset()
		p.started = false
	}
}

func openRawCommandWorkflow(m *Model, run bool) tea.Cmd {
	title, operation := "Raw Git command", "raw Git command"
	label := "Git argv (without git)"
	fields := []WorkflowField{{Name: commandArgvField, Label: label, Kind: WorkflowText, Required: true}, {Name: commandExternalField, Label: "Separately allow external git-<name> helper", Kind: WorkflowBool}}
	if run {
		title, operation, label = "Run argv directly", "direct argv command", "Executable argv"
		fields = []WorkflowField{{Name: commandArgvField, Label: label, Kind: WorkflowText, Required: true}}
	}
	dialog := WorkflowDialog{
		Title: title, Operation: operation, Fields: fields,
		Confirmation: "UNSAFE: review argv. No shell is used. First Enter reviews; a separate second Enter executes.",
		Plan:         []string{"Adapted from Magit: shell syntax, expansion, pipes, redirects, and bisect run are unsupported"},
		ReviewPreflight: func(ctx context.Context, values WorkflowValues) (WorkflowReview, error) {
			argv, err := parseCommandArgv(values[commandArgvField])
			if err != nil {
				return WorkflowReview{}, err
			}
			var reviewed gitbackend.ReviewedCommand
			if run {
				reviewed, err = gitbackend.ReviewRunCommand(argv)
			} else {
				reviewed, err = m.repo.ReviewGitCommand(ctx, argv, values[commandExternalField] == "true")
			}
			if err != nil {
				return WorkflowReview{}, err
			}
			display := reviewed.Args()
			parts := make([]string, len(display))
			for i, arg := range display {
				parts[i] = humanQuote(arg)
			}
			prefix := "git"
			if run {
				prefix = "direct exec"
			}
			plan := []string{prefix + " argv: " + strings.Join(parts, " ")}
			if reviewed.ExternalGit() {
				plan = append(plan, "Separately confirmed external git helper (aliases remain blocked)")
			}
			return WorkflowReview{Plan: plan, Confirmation: "UNSAFE EXECUTION: press Enter again to execute this exact argv; Esc cancels without starting a process", Data: reviewed}, nil
		},
		SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
			plan, ok := review.Data.(gitbackend.ReviewedCommand)
			if !ok {
				return fmt.Errorf("invalid raw command review token")
			}
			return m.repo.ExecuteReviewedCommand(ctx, gitbackend.NewAllowUnsafeExecution(), plan)
		},
	}
	return m.OpenWorkflow(dialog)
}
