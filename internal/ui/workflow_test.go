package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func TestWorkflowActionRowMovesBackToEditableField(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.OpenWorkflow(WorkflowDialog{
		Title: "Amend HEAD", ActionLabel: "Review & Submit",
		Fields: []WorkflowField{{Name: "message", Label: "Message", Kind: WorkflowText, Value: "fix: b menu errors", Required: true}},
		Submit: func(context.Context, WorkflowValues) error { return nil },
	})
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, _ = m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.workflow.field != 0 {
		t.Fatalf("Up from action row left field=%d", m.workflow.field)
	}
	_, _ = m.handleWorkflowKey(keyMsg(" edited"))
	if got := m.workflow.dialog.Fields[0].Value; got != "fix: b menu errors edited" {
		t.Fatalf("edited message = %q", got)
	}
	_, _ = m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.workflow.field != len(m.workflow.dialog.Fields) {
		t.Fatalf("Down from message left field=%d", m.workflow.field)
	}
}

func TestWorkflowSearchFiltersSelectsAndAllowsCustomValues(t *testing.T) {
	field := WorkflowField{Kind: WorkflowSearch, Choices: []WorkflowChoice{{Value: "main", Label: "main (current)"}, {Value: "feature/login", Label: "feature/login"}, {Value: "release", Label: "release"}}, AllowCustom: true}
	field.Search = "log"
	updateWorkflowSearch(&field, 0)
	if field.Value != "feature/login" || len(workflowSearchChoices(field)) != 1 {
		t.Fatalf("filtered search field = %+v", field)
	}
	field.Search = "HEAD~2"
	updateWorkflowSearch(&field, 0)
	if field.Value != "HEAD~2" || len(workflowSearchChoices(field)) != 0 {
		t.Fatalf("custom search field = %+v", field)
	}
}

func TestWorkflowSearchKeyboardAndStyledActions(t *testing.T) {
	m := New(nil)
	m.width, m.height = 100, 24
	m.loading = false
	m.OpenWorkflow(WorkflowDialog{Title: "Checkout", ActionLabel: "Checkout", Fields: []WorkflowField{{Name: "revision", Label: "Search", Kind: WorkflowSearch, Choices: []WorkflowChoice{{Value: "main", Label: "main"}, {Value: "feature", Label: "feature"}}, Required: true}}, Run: func(WorkflowValues) tea.Cmd { return nil }})
	_, _ = m.Update(keyMsg("f"))
	if got := m.workflow.dialog.Fields[0].Value; got != "feature" {
		t.Fatalf("typed search selected %q", got)
	}
	rendered := m.renderWorkflowOverlay(100, 20)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "feature") || !strings.Contains(plain, "Checkout") || !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("search/action overlay = %q", plain)
	}
}

func TestUIHandlerAndInfixInvariants(t *testing.T) {
	m := New(&gitbackend.Repository{})
	if err := m.ValidateUIHandlers(); err != nil {
		t.Fatal(err)
	}
	catalog, ok := m.transientCatalog("c")
	if !ok {
		t.Fatal("commit transient missing")
	}
	entry, ok := catalog.entry("-a")
	if !ok || !entry.Available || entry.Kind != keymap.KindInfix {
		t.Fatalf("enabled infix is not actionable: %+v", entry)
	}
	m.editTransientOption(entry)
	if !m.transientOptions[entry.Command].Enabled {
		t.Fatal("boolean infix did not update invocation state")
	}
	// Branch's conditional configuration infix and checkout-remote suffix both
	// use r. The active suffix must win; dispatching the hidden infix is unsafe.
	branch, _ := m.transientCatalog("b")
	overlap, ok := branch.entry("r")
	if !ok || overlap.Kind != keymap.KindSuffix || overlap.UpstreamCommand != "magit-checkout-remote-ref" {
		t.Fatalf("conditional overlap resolved to wrong occurrence: %+v", overlap)
	}
}

func TestMutationSerializationRejectsSecondKeyAction(t *testing.T) {
	m := New(nil)
	m.loading = false
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	m.push = func(context.Context) error { once.Do(func() { close(started) }); <-release; return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
	first := m.StartWorkflowOperation("first push", m.push)
	request := m.operationRequest
	done := make(chan tea.Msg, 1)
	go func() { done <- first() }()
	<-started
	m.scheme = schemeMagit
	_, _ = m.Update(keyMsg("P"))
	_, second := m.Update(keyMsg("p"))
	if second != nil || m.operationRequest != request || !strings.Contains(m.message, "already in progress") {
		t.Fatalf("second mutation was not rejected: cmd=%v request=%d/%d message=%q", second != nil, m.operationRequest, request, m.message)
	}
	close(release)
	_, _ = m.Update(<-done)
	if m.busy {
		t.Fatal("first result was discarded")
	}
}

func TestWorkflowLoaderEscapeCancelsChildAndIgnoresLateMessage(t *testing.T) {
	m := New(&gitbackend.Repository{})
	started, cancelled := make(chan struct{}), make(chan struct{})
	cmd := m.LoadWorkflow("blocked", func(ctx context.Context) (WorkflowDialog, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return WorkflowDialog{}, ctx.Err()
	})
	late := make(chan tea.Msg, 1)
	go func() { late <- cmd() }()
	<-started
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("loader context was not cancelled")
	}
	_, _ = m.Update(<-late)
	if m.workflow != nil || m.workflowLoading || m.isError {
		t.Fatalf("late loader result was installed: workflow=%v message=%q", m.workflow != nil, m.message)
	}
}

func TestEveryRegisteredHandlerRejectsNilRepository(t *testing.T) {
	m := New(nil)
	for id, handler := range m.workflowHandlers {
		m.message, m.isError = "", false
		if cmd := handler(m, WorkflowCommand{ID: id}); cmd != nil || !m.isError || !strings.Contains(m.message, "requires a repository") {
			t.Fatalf("%s did not return a clear internal repository error: cmd=%v message=%q", id, cmd != nil, m.message)
		}
	}
}

func TestLoadWorkflowAsyncSuccessAndError(t *testing.T) {
	m := New(&gitbackend.Repository{})
	cmd := m.LoadWorkflow("branches", func(context.Context) (WorkflowDialog, error) {
		return WorkflowDialog{
			Title:  "Choose branch",
			Fields: []WorkflowField{{Name: "branch", Label: "Branch", Kind: WorkflowSelect, Choices: []WorkflowChoice{{Value: "main", Label: "main"}}}},
			Submit: func(context.Context, WorkflowValues) error { return nil },
		}, nil
	})
	if cmd == nil || m.mode != modeStatus {
		t.Fatal("async loader blocked or opened the dialog synchronously")
	}
	_, _ = m.Update(cmd())
	if m.mode != modeWorkflow || len(m.workflow.dialog.Fields[0].Choices) != 1 || m.workflow.dialog.Fields[0].Choices[0].Value != "main" {
		t.Fatalf("loaded choices were not installed: %+v", m.workflow)
	}

	m.setMode(modeStatus)
	m.workflow = nil
	cmd = m.LoadWorkflow("remotes", func(context.Context) (WorkflowDialog, error) {
		return WorkflowDialog{}, errors.New("repository query failed")
	})
	_, _ = m.Update(cmd())
	if !m.isError || m.mode != modeStatus || !strings.Contains(m.message, "repository query failed") {
		t.Fatalf("loader error was not safely surfaced: mode=%v message=%q", m.mode, m.message)
	}
}

func reviewedTestDialog(plan []string, submit func(context.Context, WorkflowValues, WorkflowReview) error) WorkflowDialog {
	return WorkflowDialog{
		Title: "Delete branch", Operation: "delete branch",
		Fields: []WorkflowField{{Name: "branch", Label: "Branch", Kind: WorkflowText, Value: "topic", Required: true}},
		ReviewPreflight: func(context.Context, WorkflowValues) (WorkflowReview, error) {
			return WorkflowReview{Plan: plan, Confirmation: "Review deletion", Data: "token-v1"}, nil
		},
		SubmitReview: submit,
	}
}

func obtainReview(t *testing.T, m *Model) {
	t.Helper()
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd := m.handleWorkflowKey(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("first Enter did not start review preflight")
	}
	_, submit := m.Update(cmd())
	if submit != nil || m.mode != modeWorkflow || m.workflow.review == nil {
		t.Fatal("first Enter submitted instead of displaying review")
	}
}

func TestReviewedWorkflowRequiresSecondEnterAndCopiesPlan(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height = 80, 24
	plan := []string{"delete topic"}
	submits := 0
	m.OpenWorkflow(reviewedTestDialog(plan, func(_ context.Context, _ WorkflowValues, review WorkflowReview) error {
		submits++
		if review.Data != "token-v1" {
			t.Errorf("review token = %#v", review.Data)
		}
		return nil
	}))
	obtainReview(t, m)
	plan[0] = "mutated after message"
	rendered := m.renderWorkflowOverlay(m.width, m.height-4)
	if !strings.Contains(rendered, "delete topic") || !strings.Contains(rendered, "Review deletion") || !strings.Contains(rendered, "Execute") || strings.Contains(rendered, "mutated after message") {
		t.Fatalf("review was not defensively rendered: %q", rendered)
	}
	if submits != 0 {
		t.Fatal("submit ran during review phase")
	}
	_, operation := m.handleWorkflowKey(keyMsg("enter"))
	if operation == nil || submits != 0 || m.mode != modeStatus {
		t.Fatal("second Enter did not cross the operation boundary")
	}
	_ = operation()
	if submits != 1 {
		t.Fatalf("submit count = %d", submits)
	}
}

func TestReviewedWorkflowLocksFieldsAndCancelDropsIt(t *testing.T) {
	m := New(&gitbackend.Repository{})
	preflights := 0
	dialog := reviewedTestDialog([]string{"delete topic"}, func(context.Context, WorkflowValues, WorkflowReview) error { return nil })
	dialog.ReviewPreflight = func(context.Context, WorkflowValues) (WorkflowReview, error) {
		preflights++
		return WorkflowReview{Plan: []string{"delete topic"}, Data: preflights}, nil
	}
	m.OpenWorkflow(dialog)
	obtainReview(t, m)
	m.workflow.field = 0
	_, _ = m.handleWorkflowKey(keyMsg("x"))
	if m.workflow.review == nil || m.workflow.dialog.Fields[0].Value != "topic" || !strings.Contains(m.workflow.error, "locked") {
		t.Fatal("reviewed fields were mutable")
	}
	_, _ = m.handleWorkflowKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.workflow != nil || m.mode != modeStatus {
		t.Fatal("cancel retained reviewed state")
	}
}

func TestReviewedWorkflowStaleBackendErrorUsesOperationReporting(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.loading = false
	m.OpenWorkflow(reviewedTestDialog([]string{"delete topic"}, func(context.Context, WorkflowValues, WorkflowReview) error {
		return errors.New("stale review token")
	}))
	obtainReview(t, m)
	_, operation := m.handleWorkflowKey(keyMsg("enter"))
	if operation == nil {
		t.Fatal("execute produced no operation")
	}
	_, _ = m.Update(operation())
	if !m.isError || !strings.Contains(m.message, "stale review token") || m.mode != modeProcess {
		t.Fatalf("stale backend error was not reported: mode=%v message=%q", m.mode, m.message)
	}
}

func TestWorkflowRoutingCopiesOptionsAndDialogPreflight(t *testing.T) {
	m := New(&gitbackend.Repository{})
	id := keymap.CommandID("test.workflow")
	var received WorkflowCommand
	m.workflowHandlers[id] = func(model *Model, command WorkflowCommand) tea.Cmd {
		received = command
		return model.OpenWorkflow(WorkflowDialog{
			Title: "Safe test", Operation: "safe test",
			Fields:    []WorkflowField{{Name: "name", Label: "Name", Kind: WorkflowText, Value: "ok", Required: true}},
			Preflight: func(context.Context, WorkflowValues) error { return nil },
			Submit:    func(context.Context, WorkflowValues) error { return nil },
		})
	}
	options := map[keymap.CommandID]OptionValue{"test.option": {Enabled: true}}
	if cmd, handled := m.performWorkflow(WorkflowCommand{ID: id, Occurrence: "test:00", Options: options}); !handled || cmd != nil || m.mode != modeWorkflow {
		t.Fatalf("workflow was not routed: handled=%v mode=%v", handled, m.mode)
	}
	options["test.option"] = OptionValue{}
	if !received.Options["test.option"].Enabled {
		t.Fatal("workflow option state aliases caller mutation")
	}
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, preflight := m.handleWorkflowKey(keyMsg("enter"))
	if preflight == nil {
		t.Fatal("async preflight command was not produced")
	}
	msg := preflight()
	_, submit := m.Update(msg)
	if submit == nil || m.mode != modeStatus || !m.busy {
		t.Fatal("successful preflight did not start safe submit operation")
	}
	if rendered := m.render(); strings.Contains(rendered, "\x1b]52") {
		t.Fatal("workflow rendering emitted unsafe control data")
	}
}

func TestWorkflowRunCallbackExclusivity(t *testing.T) {
	run := func(WorkflowValues) tea.Cmd { return nil }
	for name, dialog := range map[string]WorkflowDialog{
		"submit":    {Run: run, Submit: func(context.Context, WorkflowValues) error { return nil }},
		"preflight": {Run: run, Preflight: func(context.Context, WorkflowValues) error { return nil }},
		"review": {Run: run,
			ReviewPreflight: func(context.Context, WorkflowValues) (WorkflowReview, error) { return WorkflowReview{}, nil },
			SubmitReview:    func(context.Context, WorkflowValues, WorkflowReview) error { return nil }},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateWorkflow(dialog, WorkflowValues{}); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("Run conflict validation = %v", err)
			}
		})
	}
	if err := validateWorkflow(WorkflowDialog{Run: run}, WorkflowValues{}); err != nil {
		t.Fatalf("standalone Run callback rejected: %v", err)
	}
}

func TestWorkflowRunInvokesOnSubmissionAndClosesDialog(t *testing.T) {
	type runMsg struct{ value string }
	m := New(&gitbackend.Repository{})
	m.loading = false
	invoked := false
	m.OpenWorkflow(WorkflowDialog{Fields: []WorkflowField{{Name: "value", Kind: WorkflowText, Value: "selected"}}, Run: func(values WorkflowValues) tea.Cmd {
		invoked = true
		return func() tea.Msg { return runMsg{value: values["value"]} }
	}})
	m.workflow.field = len(m.workflow.dialog.Fields)
	_, cmd := m.handleWorkflowKey(keyMsg("enter"))
	if !invoked || cmd == nil || m.workflow != nil || m.mode != modeStatus {
		t.Fatalf("Run dispatch state: invoked=%v cmd=%v workflow=%#v mode=%v", invoked, cmd != nil, m.workflow, m.mode)
	}
	if msg, ok := cmd().(runMsg); !ok || msg.value != "selected" {
		t.Fatalf("Run command message = %#v", msg)
	}
}
