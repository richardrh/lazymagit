package ui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestCommandArgvGrammarTreatsShellSyntaxAsData(t *testing.T) {
	argv, err := parseCommandArgv(`printf '%s' "; $(touch nope)" '$HOME' back\ slash`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"printf", "%s", "; $(touch nope)", "$HOME", "back slash"}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}
	for _, malformed := range []string{`"unterminated`, `trailing\`} {
		if _, err := parseCommandArgv(malformed); err == nil {
			t.Fatalf("malformed argv accepted: %q", malformed)
		}
	}
}

func TestCommandArgvParserHelpers(t *testing.T) {
	unquoted := commandArgvParser{}
	unquoted.consumeUnquoted('a')
	unquoted.consumeUnquoted(' ')
	unquoted.consumeUnquoted('\'')
	if !unquoted.started || unquoted.mode != commandArgvSingleQuoted || strings.Join(unquoted.argv, "|") != "a" {
		t.Fatalf("unquoted parser state = %+v", unquoted)
	}

	single := commandArgvParser{mode: commandArgvSingleQuoted}
	single.consumeSingleQuoted('b')
	single.consumeSingleQuoted('\'')
	if single.word.String() != "b" || single.mode != commandArgvUnquoted || !single.started {
		t.Fatalf("single-quoted parser state = %+v", single)
	}

	double := commandArgvParser{mode: commandArgvDoubleQuoted}
	double.consumeDoubleQuoted('c')
	double.consumeDoubleQuoted('\\')
	double.consume('d')
	double.consumeDoubleQuoted('"')
	if double.word.String() != "cd" || double.mode != commandArgvUnquoted || double.escaped || !double.started {
		t.Fatalf("double-quoted parser state = %+v", double)
	}

	quotedRoute := commandArgvParser{mode: commandArgvSingleQuoted}
	quotedRoute.consume('x')
	doubleRoute := commandArgvParser{mode: commandArgvDoubleQuoted}
	doubleRoute.consume('y')
	unquotedRoute := commandArgvParser{}
	unquotedRoute.consume('z')
	if quotedRoute.word.String() != "x" || doubleRoute.word.String() != "y" || unquotedRoute.word.String() != "z" {
		t.Fatalf("consume routing failed: %q %q %q", quotedRoute.word.String(), doubleRoute.word.String(), unquotedRoute.word.String())
	}

	for name, parser := range map[string]commandArgvParser{
		"escape": {escaped: true},
		"quote":  {mode: commandArgvSingleQuoted},
		"empty":  {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parser.finish(); err == nil {
				t.Fatal("invalid parser state finished without an error")
			}
		})
	}
	finished := commandArgvParser{started: true}
	finished.word.WriteString("done")
	argv, err := finished.finish()
	if err != nil || strings.Join(argv, "|") != "done" {
		t.Fatalf("finished argv = %#v, %v", argv, err)
	}
	finished.flush()
	if strings.Join(finished.argv, "|") != "done" {
		t.Fatalf("empty flush changed argv: %#v", finished.argv)
	}
}

func TestRawCommandStatusKeysReachReviewedWorkflows(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.loading, m.scheme = false, schemeMagit
	for _, key := range []string{"Q", ":"} {
		_, _ = m.Update(keyMsg(key))
		if m.mode != modeWorkflow || m.workflow == nil || !strings.Contains(m.workflow.dialog.Confirmation, "UNSAFE") {
			t.Fatalf("%s did not open reviewed Git workflow: mode=%v message=%q", key, m.mode, m.message)
		}
		_, _ = m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	}
	_, _ = m.Update(keyMsg("!"))
	_, _ = m.Update(keyMsg("s"))
	if m.mode != modeWorkflow || m.workflow == nil || m.workflow.dialog.Title != "Run argv directly" {
		t.Fatalf("! s did not open adapted direct-argv workflow: mode=%v message=%q", m.mode, m.message)
	}
}

func typeWorkflowText(t *testing.T, m *Model, text string) {
	t.Helper()
	for _, r := range text {
		_, cmd := m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		if cmd != nil {
			t.Fatalf("typing %q unexpectedly returned a command", r)
		}
	}
}

func reviewTypedRun(t *testing.T, m *Model, input string) {
	t.Helper()
	m.scheme = schemeMagit
	_, _ = m.Update(keyMsg("!"))
	_, _ = m.Update(keyMsg("s"))
	if m.workflow == nil || m.workflow.dialog.Title != "Run argv directly" {
		t.Fatalf("! s did not open direct argv workflow: %q", m.message)
	}
	typeWorkflowText(t, m, input)
	_, _ = m.handleWorkflowKey(keyMsg("tab"))
	_, preflight := m.handleWorkflowKey(keyMsg("enter"))
	if preflight == nil {
		t.Fatal("first Enter did not start unsafe review")
	}
	_, operation := m.Update(preflight())
	if operation != nil || m.workflow == nil || m.workflow.review == nil {
		t.Fatal("first Enter executed instead of displaying review")
	}
}

func TestRunWorkflowKeysKeepSemicolonAndSubstitutionInert(t *testing.T) {
	marker := t.TempDir() + "/substitution-ran"
	m := New(&gitbackend.Repository{})
	m.loading, m.snapshotLoader = false, nil
	input := `/usr/bin/printf '%s' ';$(/usr/bin/touch ` + marker + `)'`
	reviewTypedRun(t, m, input)
	if rendered := m.renderWorkflowOverlay(100, 30); !strings.Contains(rendered, "UNSAFE EXECUTION") || !strings.Contains(rendered, "$(/usr/bin/touch") {
		t.Fatalf("unsafe review is unclear: %q", rendered)
	}
	_, operation := m.handleWorkflowKey(keyMsg("enter"))
	if operation == nil {
		t.Fatal("second Enter did not return an operation")
	}
	_, _ = m.Update(operation())
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command substitution was evaluated: %v", err)
	}
	if transcript := m.processTranscript(); !strings.Contains(transcript, ";$(/usr/bin/touch") {
		t.Fatalf("literal argv was not process-recorded: %q", transcript)
	}
}

func TestRunWorkflowEscapeAfterReviewStartsNoProcessAndReviewRedacts(t *testing.T) {
	marker := t.TempDir() + "/cancelled-ran"
	m := New(&gitbackend.Repository{})
	m.loading, m.snapshotLoader = false, nil
	reviewTypedRun(t, m, `/usr/bin/touch `+marker+` token=do-not-render-7812`)
	if plan := strings.Join(m.workflow.review.Plan, "\n"); strings.Contains(plan, "do-not-render-7812") || !strings.Contains(plan, "[REDACTED]") {
		t.Fatalf("review plan credential redaction failed: %q", plan)
	}
	_, _ = m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("cancelled workflow started a process: %v", err)
	}
	if m.workflow != nil || m.busy || m.processTranscript() != "" {
		t.Fatal("cancel retained or recorded unsafe execution")
	}
}
