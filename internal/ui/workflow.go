package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/richard/lazymagit/internal/keymap"
)

// WorkflowCommand is the lossless hand-off from a transient to a domain.
// Occurrence identifies the exact upstream row; Options is a defensive copy of
// the values edited during this invocation of the transient.
type WorkflowCommand struct {
	ID         keymap.CommandID
	Occurrence string
	Prefix     string
	Options    map[keymap.CommandID]OptionValue
}

type OptionValue struct {
	Enabled bool
	Value   string
}

// WorkflowHandler is installed by a domain file and is called on Bubble Tea's
// update goroutine. A handler normally calls OpenWorkflow or StartWorkflowOperation.
type WorkflowHandler func(*Model, WorkflowCommand) tea.Cmd

// WorkflowRegistration is an immutable per-Model set of domain handlers.
type WorkflowRegistration func(*Model) map[keymap.CommandID]WorkflowHandler

// WorkflowCapability declares the manifest identity implemented by a handler.
// Consumes lists exact upstream infix command identities; undeclared options
// are never made actionable or silently forwarded.
type WorkflowCapability struct {
	ID              keymap.CommandID
	Transient       string
	UpstreamCommand string
	Consumes        []string
}

var workflowRegistrations struct {
	sync.RWMutex
	providers    []WorkflowRegistration
	capabilities []WorkflowCapability
}

// RegisterWorkflowCapabilities is the data-only extension point for domains.
// It lets new domains connect status bindings, suffixes, options, validation,
// and generated documentation without editing the central dispatcher.
func RegisterWorkflowCapabilities(capabilities ...WorkflowCapability) {
	workflowRegistrations.Lock()
	defer workflowRegistrations.Unlock()
	for _, capability := range capabilities {
		if capability.ID == keymap.CommandNone || capability.UpstreamCommand == "" {
			panic("ui: incomplete workflow capability")
		}
		capability.Consumes = append([]string(nil), capability.Consumes...)
		workflowRegistrations.capabilities = append(workflowRegistrations.capabilities, capability)
	}
}

// RegisterWorkflowDomain is the only package-level extension point. A domain
// file named workflow_<domain>.go should call it from init. Registration is
// synchronized; New snapshots providers and handlers, so running Models never
// observe later mutation. Duplicate command IDs fail loudly rather than route
// to an arbitrary workflow.
func RegisterWorkflowDomain(provider WorkflowRegistration) {
	if provider == nil {
		panic("ui: nil workflow domain registration")
	}
	workflowRegistrations.Lock()
	workflowRegistrations.providers = append(workflowRegistrations.providers, provider)
	workflowRegistrations.Unlock()
}

func workflowHandlersFor(m *Model) map[keymap.CommandID]WorkflowHandler {
	workflowRegistrations.RLock()
	providers := append([]WorkflowRegistration(nil), workflowRegistrations.providers...)
	workflowRegistrations.RUnlock()
	handlers := make(map[keymap.CommandID]WorkflowHandler)
	for _, provider := range providers {
		for id, handler := range provider(m) {
			if id == keymap.CommandNone || handler == nil {
				panic("ui: workflow domain registered an empty command or nil handler")
			}
			if _, exists := handlers[id]; exists {
				panic("ui: duplicate workflow handler for " + string(id))
			}
			commandID, domainHandler := id, handler
			handlers[id] = func(model *Model, command WorkflowCommand) tea.Cmd {
				if model.repo == nil && !model.hasInjectedBackend() {
					model.setError(fmt.Errorf("internal error: %s requires a repository", commandID))
					return nil
				}
				return domainHandler(model, command)
			}
		}
	}
	return handlers
}

func capabilitiesForTransient(transient string, consumers map[string][]string) []WorkflowCapability {
	seen := map[keymap.CommandID]bool{}
	var out []WorkflowCapability
	for _, binding := range keymap.Registry() {
		consumes, ok := consumers[binding.UpstreamCommand]
		if !ok || binding.Scheme != keymap.SchemeMagit || binding.Transient != transient || binding.Kind != keymap.KindSuffix || seen[binding.Command] {
			continue
		}
		seen[binding.Command] = true
		out = append(out, WorkflowCapability{ID: binding.Command, Transient: transient, UpstreamCommand: binding.UpstreamCommand, Consumes: append([]string(nil), consumes...)})
	}
	return out
}

func workflowCapabilitiesFor() map[keymap.CommandID]WorkflowCapability {
	workflowRegistrations.RLock()
	defer workflowRegistrations.RUnlock()
	out := make(map[keymap.CommandID]WorkflowCapability, len(workflowRegistrations.capabilities))
	for _, capability := range workflowRegistrations.capabilities {
		if _, exists := out[capability.ID]; exists {
			panic("ui: duplicate workflow capability for " + string(capability.ID))
		}
		capability.Consumes = append([]string(nil), capability.Consumes...)
		out[capability.ID] = capability
	}
	return out
}

func (m *Model) hasInjectedBackend() bool {
	return m.showCommit != nil || m.addRemote != nil || m.fetch != nil || m.fetchUpstream != nil || m.fetchPush != nil || m.pushRemote != nil || m.push != nil || m.pushSetUpstream != nil || m.setPushRemote != nil || m.fetchAll != nil || m.stageAll != nil || m.unstageAll != nil || m.snapshotLoader != nil
}

// ValidateUIHandlers checks the registry/UI integration boundary for a Model.
// It is exported for domain tests and is also exercised by central tests.
func (m *Model) ValidateUIHandlers() error {
	for _, binding := range keymap.Registry() {
		if binding.Handler == keymap.HandlerExecute && !builtinUICommands[binding.Command] {
			if _, ok := m.workflowHandlers[binding.Command]; !ok {
				return fmt.Errorf("%s has no UI handler", binding.Command)
			}
		}
		if binding.Handler == keymap.HandlerInfix && binding.Kind != keymap.KindInfix {
			return fmt.Errorf("%s has invalid infix handler", binding.Command)
		}
		if binding.Availability == keymap.AvailabilityNever && (binding.Unavailable == "" || binding.UnavailableCategory == keymap.UnavailableNone) {
			return fmt.Errorf("%s has untyped unavailability", binding.Command)
		}
	}
	for id, capability := range m.workflowCapabilities {
		if _, ok := m.workflowHandlers[id]; !ok {
			return fmt.Errorf("%s declares a capability without a handler", id)
		}
		matched := false
		for _, binding := range keymap.Registry() {
			if binding.UpstreamCommand == capability.UpstreamCommand && (capability.Transient == "" || binding.Transient == capability.Transient) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s capability does not match the manifest", id)
		}
	}
	return nil
}

var builtinUICommands = map[keymap.CommandID]bool{
	keymap.CommandMoveDown: true, keymap.CommandMoveUp: true, keymap.CommandFirst: true, keymap.CommandLast: true,
	keymap.CommandRefresh: true, keymap.CommandToggleSection: true, keymap.CommandStage: true, keymap.CommandUnstage: true,
	keymap.CommandStageAll: true, keymap.CommandUnstageAll: true, keymap.CommandDiscard: true, keymap.CommandCommit: true,
	keymap.CommandSwitchBranch: true, keymap.CommandPush: true, keymap.CommandFetchUpstream: true, keymap.CommandFetchPush: true,
	keymap.CommandFetchElsewhere: true, keymap.CommandFetchAll: true, keymap.CommandAddRemote: true, keymap.CommandShowProcesses: true,
	keymap.CommandOpenDispatcher: true, keymap.CommandQuit: true, keymap.CommandDepth1: true, keymap.CommandDepth2: true,
	keymap.CommandDepth3: true, keymap.CommandScrollDown: true, keymap.CommandScrollUp: true,
	commandSectionCycle: true, commandCopyThing: true, commandCopySectionValue: true, commandCopyBufferRevision: true,
}

func (m *Model) performWorkflow(command WorkflowCommand) (tea.Cmd, bool) {
	handler, handled := m.workflowHandlers[command.ID]
	if !handled {
		return nil, false
	}
	if !m.canOperate() {
		return nil, true
	}
	command.Options = cloneOptions(command.Options)
	cmd := handler(m, command)
	// A handler may intentionally complete synchronously, but it may not vanish.
	if cmd == nil && m.mode == modeStatus && m.message == "" {
		m.setError(fmt.Errorf("internal error: %s workflow produced no action", command.ID))
	}
	return cmd, true
}

func cloneOptions(in map[keymap.CommandID]OptionValue) map[keymap.CommandID]OptionValue {
	out := make(map[keymap.CommandID]OptionValue, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type WorkflowFieldKind string

const (
	WorkflowText    WorkflowFieldKind = "text"
	WorkflowBool    WorkflowFieldKind = "bool"
	WorkflowEnum    WorkflowFieldKind = "enum"
	WorkflowSelect  WorkflowFieldKind = "select"
	WorkflowSearch  WorkflowFieldKind = "search"
	WorkflowConfirm WorkflowFieldKind = "confirm"
)

type WorkflowChoice struct{ Value, Label string }

type WorkflowField struct {
	Name, Label string
	Kind        WorkflowFieldKind
	Value       string
	Search      string
	Bool        bool
	Choices     []WorkflowChoice
	Choice      int
	AllowCustom bool
	Required    bool
}

type WorkflowValues map[string]string

// WorkflowReview is the immutable result of an asynchronous reviewed
// preflight. Data is an opaque backend token/snapshot and must be treated as
// immutable by callers. The UI only transports it through Bubble Tea messages
// and back to SubmitReview; it never inspects or mutates Data.
type WorkflowReview struct {
	Plan         []string
	Confirmation string
	Data         any
}

// WorkflowLoader performs asynchronous dialog setup, including repository
// queries used to populate select choices. Use Model.LoadWorkflow from a domain
// handler; no domain-specific tea.Msg or blocking Update implementation is
// needed.
type WorkflowLoader func(context.Context) (WorkflowDialog, error)

// WorkflowDialog is a generic, safely-rendered workflow contract. Preflight is
// asynchronous and runs before Submit; Submit is executed through the normal
// operation recorder/refresh path. Validation errors keep the dialog open.
type WorkflowDialog struct {
	Title, Confirmation string
	ActionLabel         string
	Plan                []string
	Fields              []WorkflowField
	Validate            func(WorkflowValues) error
	Preflight           func(context.Context, WorkflowValues) error
	Submit              func(context.Context, WorkflowValues) error
	// Run dispatches a UI-thread command after validation and closes the dialog.
	// It is mutually exclusive with mutation and preflight callbacks.
	Run func(WorkflowValues) tea.Cmd
	// ReviewPreflight and SubmitReview opt into reviewed two-phase mutation.
	// The first activation asynchronously obtains a review; only a distinct
	// second activation executes it. Legacy Preflight/Submit remain supported.
	ReviewPreflight func(context.Context, WorkflowValues) (WorkflowReview, error)
	SubmitReview    func(context.Context, WorkflowValues, WorkflowReview) error
	Operation       string
	OnCancel        func()
}

type workflowState struct {
	dialog  WorkflowDialog
	field   int
	error   string
	busy    bool
	review  *WorkflowReview
	request uint64
	cancel  context.CancelFunc
}

type workflowPreflightMsg struct {
	request uint64
	values  WorkflowValues
	err     error
}

type workflowReviewMsg struct {
	request uint64
	values  WorkflowValues
	review  WorkflowReview
	err     error
}

type workflowLoadMsg struct {
	request uint64
	state   uint64
	dialog  WorkflowDialog
	err     error
}

// OpenWorkflow opens a fresh dialog. Dialog and choice slices are copied.
func (m *Model) OpenWorkflow(dialog WorkflowDialog) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	dialog = cloneWorkflowDialog(dialog)
	m.workflow = &workflowState{dialog: dialog}
	m.setMode(modeWorkflow)
	return nil
}

// LoadWorkflow asynchronously builds and opens a dialog. Results are ignored
// after a newer load, state transition, or application shutdown.
func (m *Model) LoadWorkflow(name string, loader WorkflowLoader) tea.Cmd {
	if !m.canOperate() {
		return nil
	}
	if loader == nil {
		m.setError(errors.New("workflow loader is not configured"))
		return nil
	}
	m.cancelWorkflowLoad()
	m.workflowRequest++
	ctx, cancel := context.WithCancel(m.appCtx)
	m.workflowLoadCancel = cancel
	m.workflowLoading = true
	request, state := m.workflowRequest, m.stateGeneration
	if strings.TrimSpace(name) == "" {
		name = "workflow"
	}
	m.setMessage("Loading " + sanitizeSingleLine(name) + "…")
	return func() tea.Msg {
		dialog, err := loader(ctx)
		return workflowLoadMsg{request: request, state: state, dialog: cloneWorkflowDialog(dialog), err: err}
	}
}

func (m *Model) cancelWorkflowLoad() {
	if m.workflowLoadCancel != nil {
		m.workflowLoadCancel()
		m.workflowLoadCancel = nil
	}
	if m.workflowLoading {
		m.workflowLoading = false
		m.workflowRequest++
	}
}

func cloneWorkflowDialog(dialog WorkflowDialog) WorkflowDialog {
	dialog.Fields = append([]WorkflowField(nil), dialog.Fields...)
	dialog.Plan = append([]string(nil), dialog.Plan...)
	for i := range dialog.Fields {
		dialog.Fields[i].Choices = append([]WorkflowChoice(nil), dialog.Fields[i].Choices...)
	}
	return dialog
}

func cloneWorkflowValues(values WorkflowValues) WorkflowValues {
	out := make(WorkflowValues, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneWorkflowReview(review WorkflowReview) WorkflowReview {
	review.Plan = append([]string(nil), review.Plan...)
	return review
}

// StartWorkflowOperation exposes the standard safe async operation boundary to
// domain handlers that do not need a dialog.
func (m *Model) StartWorkflowOperation(name string, operation func(context.Context) error) tea.Cmd {
	return m.startOperation(name, operation)
}

func workflowSearchChoices(field WorkflowField) []WorkflowChoice {
	query := strings.ToLower(strings.TrimSpace(field.Search))
	if query == "" {
		return append([]WorkflowChoice(nil), field.Choices...)
	}
	var matches []WorkflowChoice
	for _, choice := range field.Choices {
		if strings.Contains(strings.ToLower(choice.Label), query) || strings.Contains(strings.ToLower(choice.Value), query) {
			matches = append(matches, choice)
		}
	}
	return matches
}

func updateWorkflowSearch(field *WorkflowField, delta int) {
	matches := workflowSearchChoices(*field)
	if len(matches) == 0 {
		field.Choice = 0
		if field.AllowCustom {
			field.Value = strings.TrimSpace(field.Search)
		} else {
			field.Value = ""
		}
		return
	}
	field.Choice = min(max(0, field.Choice+delta), len(matches)-1)
	field.Value = matches[field.Choice].Value
}

func (m *Model) workflowValues() WorkflowValues {
	values := make(WorkflowValues, len(m.workflow.dialog.Fields))
	for _, f := range m.workflow.dialog.Fields {
		if f.Kind == WorkflowBool {
			values[f.Name] = fmt.Sprint(f.Bool)
		} else {
			values[f.Name] = f.Value
		}
	}
	return values
}

func validateWorkflow(d WorkflowDialog, values WorkflowValues) error {
	for _, field := range d.Fields {
		if field.Required && strings.TrimSpace(values[field.Name]) == "" {
			return fmt.Errorf("%s is required", field.Label)
		}
	}
	if d.Run != nil {
		if d.Submit != nil || d.Preflight != nil || d.ReviewPreflight != nil || d.SubmitReview != nil {
			return errors.New("workflow run callback is mutually exclusive with submit and preflight callbacks")
		}
	} else if d.ReviewPreflight != nil || d.SubmitReview != nil {
		if d.ReviewPreflight == nil || d.SubmitReview == nil {
			return errors.New("reviewed workflow requires both preflight and submit callbacks")
		}
	} else if d.Submit == nil {
		return errors.New("workflow submit callback is not configured")
	}
	if d.Validate != nil {
		return d.Validate(values)
	}
	return nil
}

func (m *Model) handleWorkflowKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	w := m.workflow
	if w == nil {
		m.setMode(modeStatus)
		return m, nil
	}
	key := msg.String()
	if key == "esc" || key == "q" && len(w.dialog.Fields) == 0 {
		return m, m.cancelWorkflow()
	}
	if w.busy {
		return m, nil
	}
	if w.review != nil && key != "enter" {
		w.error = "Review is locked; press Enter to execute or Esc to cancel"
		return m, nil
	}
	if handleWorkflowNavigation(w, key) {
		return m, nil
	}
	if key == "enter" && w.field >= len(w.dialog.Fields) {
		return m, m.activateWorkflow()
	}
	if w.field >= len(w.dialog.Fields) {
		return m, nil
	}
	if editWorkflowField(&w.dialog.Fields[w.field], msg) {
		w.review = nil
		w.request++
	}
	w.error = ""
	return m, nil
}

func (m *Model) cancelWorkflow() tea.Cmd {
	w := m.workflow
	if w.cancel != nil {
		w.cancel()
	}
	if w.dialog.OnCancel != nil {
		w.dialog.OnCancel()
	}
	m.workflow = nil
	m.workflowRequest++
	m.setMode(modeStatus)
	m.setMessage("Workflow cancelled")
	return m.loadDetailCmd()
}

func handleWorkflowNavigation(w *workflowState, key string) bool {
	switch key {
	case "tab":
		w.field = min(w.field+1, len(w.dialog.Fields))
		return true
	case "shift+tab":
		w.field = max(0, w.field-1)
		return true
	}
	if w.field >= len(w.dialog.Fields) {
		if key == "up" {
			w.field = max(0, len(w.dialog.Fields)-1)
			return true
		}
		return false
	}
	field := &w.dialog.Fields[w.field]
	if field.Kind == WorkflowSearch {
		switch key {
		case "down":
			updateWorkflowSearch(field, 1)
			return true
		case "up":
			updateWorkflowSearch(field, -1)
			return true
		case "enter":
			w.field = len(w.dialog.Fields)
			return false
		}
		return false
	}
	if key == "down" {
		w.field = min(w.field+1, len(w.dialog.Fields))
		return true
	}
	if key == "up" {
		w.field = max(0, w.field-1)
		return true
	}
	return false
}

func (m *Model) activateWorkflow() tea.Cmd {
	w := m.workflow
	values := m.workflowValues()
	if err := validateWorkflow(w.dialog, values); err != nil {
		w.error = sanitizeSingleLine(err.Error())
		return nil
	}
	if w.review != nil {
		return m.submitReviewedWorkflow(values, *w.review)
	}
	if w.dialog.ReviewPreflight != nil {
		return m.startWorkflowReview(values)
	}
	if w.dialog.Preflight != nil {
		return m.startWorkflowPreflight(values)
	}
	return m.submitWorkflow(values)
}

func (m *Model) startWorkflowReview(values WorkflowValues) tea.Cmd {
	w := m.workflow
	w.busy = true
	w.request++
	ctx, cancel := context.WithCancel(m.appCtx)
	w.cancel = cancel
	request, preflight := w.request, w.dialog.ReviewPreflight
	values = cloneWorkflowValues(values)
	return func() tea.Msg {
		review, err := preflight(ctx, cloneWorkflowValues(values))
		return workflowReviewMsg{request: request, values: values, review: cloneWorkflowReview(review), err: err}
	}
}

func (m *Model) startWorkflowPreflight(values WorkflowValues) tea.Cmd {
	w := m.workflow
	w.busy = true
	w.request++
	ctx, cancel := context.WithCancel(m.appCtx)
	w.cancel = cancel
	request, preflight := w.request, w.dialog.Preflight
	values = cloneWorkflowValues(values)
	return func() tea.Msg {
		return workflowPreflightMsg{request: request, values: values, err: preflight(ctx, cloneWorkflowValues(values))}
	}
}

func editWorkflowField(field *WorkflowField, msg tea.KeyPressMsg) bool {
	key := msg.String()
	switch field.Kind {
	case WorkflowBool:
		return editWorkflowBool(field, key)
	case WorkflowEnum, WorkflowSelect:
		return editWorkflowChoice(field, key)
	case WorkflowSearch:
		return editWorkflowSearch(field, key, msg.Key().Text)
	case WorkflowText, WorkflowConfirm:
		return editWorkflowText(field, key, msg.Key().Text)
	default:
		return false
	}
}

func editWorkflowBool(field *WorkflowField, key string) bool {
	if key != "enter" && key != "space" {
		return false
	}
	field.Bool = !field.Bool
	return true
}

func editWorkflowChoice(field *WorkflowField, key string) bool {
	if len(field.Choices) == 0 || key != "enter" && key != "space" && key != "right" {
		return false
	}
	index := 0
	for i, choice := range field.Choices {
		if choice.Value == field.Value {
			index = i
		}
	}
	field.Value = field.Choices[(index+1)%len(field.Choices)].Value
	return true
}

func editWorkflowSearch(field *WorkflowField, key, text string) bool {
	if key == "backspace" || key == "ctrl+h" {
		if field.Search != "" {
			_, size := utf8.DecodeLastRuneInString(field.Search)
			field.Search = field.Search[:len(field.Search)-size]
			field.Choice = 0
			updateWorkflowSearch(field, 0)
			return true
		}
		if field.Value != "" {
			field.Value = ""
			return true
		}
		return false
	}
	if text == "" {
		return false
	}
	field.Search += text
	field.Choice = 0
	updateWorkflowSearch(field, 0)
	return true
}

func editWorkflowText(field *WorkflowField, key, text string) bool {
	if key == "backspace" || key == "ctrl+h" {
		if field.Value == "" {
			return false
		}
		_, size := utf8.DecodeLastRuneInString(field.Value)
		field.Value = field.Value[:len(field.Value)-size]
		return true
	}
	if text == "" {
		return false
	}
	field.Value += text
	return true
}

func (m *Model) submitWorkflow(values WorkflowValues) tea.Cmd {
	w := m.workflow
	if w.dialog.Run != nil {
		run := w.dialog.Run
		m.workflow = nil
		m.setMode(modeStatus)
		return run(cloneWorkflowValues(values))
	}
	name, submit := strings.TrimSpace(w.dialog.Operation), w.dialog.Submit
	if name == "" {
		name = strings.TrimSpace(w.dialog.Title)
	}
	if name == "" {
		name = "workflow"
	}
	m.workflow = nil
	m.setMode(modeStatus)
	return m.startOperation(name, func(ctx context.Context) error { return submit(ctx, values) })
}

func (m *Model) submitReviewedWorkflow(values WorkflowValues, review WorkflowReview) tea.Cmd {
	w := m.workflow
	name, submit := strings.TrimSpace(w.dialog.Operation), w.dialog.SubmitReview
	if submit == nil {
		w.error = "workflow reviewed submit callback is not configured"
		return nil
	}
	if name == "" {
		name = strings.TrimSpace(w.dialog.Title)
	}
	if name == "" {
		name = "workflow"
	}
	values, review = cloneWorkflowValues(values), cloneWorkflowReview(review)
	m.workflow = nil
	m.setMode(modeStatus)
	return m.startOperation(name, func(ctx context.Context) error {
		return submit(ctx, cloneWorkflowValues(values), cloneWorkflowReview(review))
	})
}

func optionSummary(options map[keymap.CommandID]OptionValue) string {
	keys := make([]string, 0, len(options))
	for id, value := range options {
		if value.Enabled || value.Value != "" {
			keys = append(keys, string(id))
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
