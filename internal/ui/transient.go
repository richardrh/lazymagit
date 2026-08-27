package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

// menuCatalog is the single source of truth for what a transient presents.
// Available entries map to behavior that exists today; unavailable entries are
// documentation, never inert commands disguised as working ones.
type menuCatalog struct {
	Title  string
	Groups []menuGroup
}

type menuGroup struct {
	Title   string
	Entries []menuEntry
}

type menuEntry struct {
	Occurrence      string
	Key             string
	Display         string
	Label           string
	Available       bool
	Command         keymap.CommandID
	UpstreamCommand string
	Reason          string
	Category        menuEntryCategory
	Kind            keymap.EntryKind
	Conditions      []string
	Option          OptionValue
	TakesValue      bool
	Active          bool
	Prefix          bool
}

type menuEntryCategory string

const (
	menuEntryRegistry     menuEntryCategory = "registry"
	menuEntryContext      menuEntryCategory = "context"
	menuEntryMissing      menuEntryCategory = "missing"
	menuEntryInfix        menuEntryCategory = "infix"
	menuEntryPresentation menuEntryCategory = "presentation-only"
)

var prefixCatalogs = buildPrefixCatalogs(keymap.SchemeVim)

func buildPrefixCatalogs(scheme keymap.Scheme) map[string]menuCatalog {
	out := map[string]menuCatalog{}
	for _, top := range keymap.BindingsFor(scheme, keymap.ContextStatus) {
		if top.Handler != keymap.HandlerPrefix {
			continue
		}
		tr, ok := keymap.TransientByName(top.UpstreamCommand)
		if !ok {
			continue
		}
		prefix := strings.Join(top.Sequence, " ")
		catalog := menuCatalog{Title: tr.Title}
		groups := map[string]int{}
		for _, binding := range keymap.BindingsForTransient(scheme, tr.Name) {
			index, ok := groups[binding.Group]
			if !ok {
				index = len(catalog.Groups)
				groups[binding.Group] = index
				catalog.Groups = append(catalog.Groups, menuGroup{Title: binding.Group})
			}
			available, reason := binding.Available(keymap.Context{View: keymap.ViewStatus, Scheme: scheme})
			category := menuEntryRegistry
			if !available {
				reason = binding.Unavailable
				switch binding.UnavailableCategory {
				case keymap.UnavailableInfix:
					category = menuEntryInfix
				case keymap.UnavailableContext:
					category = menuEntryContext
				default:
					category = menuEntryMissing
				}
			}
			key := strings.Join(binding.LocalSequence, " ")
			catalog.Groups[index].Entries = append(catalog.Groups[index].Entries, menuEntry{Occurrence: binding.Occurrence, Key: key, Display: keymapDisplayFor(binding.LocalSequence), Label: binding.Label, Available: available, Command: binding.Command, UpstreamCommand: binding.UpstreamCommand, Reason: reason, Category: category, Kind: binding.Kind, Conditions: append([]string(nil), binding.Conditions...), Active: true, Prefix: binding.Handler == keymap.HandlerPrefix})
		}
		out[prefix] = catalog
	}
	return out
}

// transientCatalog is evaluated for each render/key press. Scheme, repository
// context, installed domain handlers, conditions, and invocation-local options
// therefore cannot go stale.
func (m *Model) transientCatalog(prefix string) (menuCatalog, bool) {
	name := m.resolver.ActiveTransient()
	if name == "" {
		if _, exact := keymap.TransientByName(prefix); exact {
			name = prefix
		}
		for _, top := range keymap.BindingsFor(schemeID(m.scheme), keymap.ContextStatus) {
			if strings.Join(top.Sequence, " ") == prefix && top.Handler == keymap.HandlerPrefix {
				name = top.UpstreamCommand
				break
			}
		}
	}
	tr, ok := keymap.TransientByName(name)
	if !ok {
		return menuCatalog{}, false
	}
	catalog := menuCatalog{Title: tr.Title}
	groups := map[string]int{}
	ctx := m.keyContext()
	for _, binding := range keymap.BindingsForTransient(schemeID(m.scheme), name) {
		index, exists := groups[binding.Group]
		if !exists {
			index = len(catalog.Groups)
			groups[binding.Group] = index
			catalog.Groups = append(catalog.Groups, menuGroup{Title: m.transientGroupTitle(name, binding.Group)})
		}
		active, conditionReason := m.bindingCondition(binding)
		available, reason := binding.Available(ctx)
		category := menuEntryRegistry
		if binding.Kind == keymap.KindInfix {
			consumers := m.optionConsumers(prefix, binding)
			available = active && len(consumers) > 0
			reason = conditionReason
			if active && len(consumers) == 0 {
				reason = "no registered workflow consumes this option"
			}
			category = menuEntryInfix
		} else if binding.Handler == keymap.HandlerPrefix {
			available = active
			reason = conditionReason
		} else if _, registered := m.workflowHandlers[binding.Command]; registered {
			available = active
			reason = conditionReason
		} else if binding.Handler == keymap.HandlerExecute {
			available = available && active
		} else {
			available = false
			if reason == "" {
				reason = "workflow handler is not registered"
			}
			category = menuEntryMissing
		}
		if !active && reason == "" {
			reason = conditionReason
		}
		if !active {
			category = menuEntryContext
		}
		if !available && reason == "" {
			reason = "not available in the current context"
		}
		key := strings.Join(binding.LocalSequence, " ")
		command := binding.Command
		// Keep menuCatalog as an injectable test seam while deriving all runtime
		// availability from current Model state.
		if baseline, found := prefixCatalogs[prefix].occurrence(binding.Occurrence); found {
			command = baseline.Command
		}
		value, set := m.transientOptions[command]
		if !set {
			value = transientDefaultOption(binding)
		}
		if m.transientEdit != nil && m.transientEdit.Command == binding.Command {
			value.Value += "█"
		}
		catalog.Groups[index].Entries = append(catalog.Groups[index].Entries, menuEntry{Occurrence: binding.Occurrence, Key: key, Display: keymapDisplayFor(binding.LocalSequence), Label: binding.Label, Available: available, Command: command, UpstreamCommand: binding.UpstreamCommand, Reason: reason, Category: category, Kind: binding.Kind, Conditions: append([]string(nil), binding.Conditions...), Option: value, TakesValue: binding.TakesValue, Active: active, Prefix: binding.Handler == keymap.HandlerPrefix})
	}
	return catalog, true
}

func (m *Model) transientGroupTitle(transient, title string) string {
	if !strings.HasPrefix(title, "#[") {
		return title
	}
	branch := sanitizeSingleLine(strings.TrimSpace(m.snapshot.summary.Branch))
	if branch == "" || m.snapshot.summary.Detached {
		branch = "HEAD"
	}
	switch transient {
	case "magit-branch", "magit-branch-configure":
		return "Configure " + branch
	case "magit-pull":
		switch {
		case strings.Contains(title, "Pull arguments"):
			return "Pull arguments"
		case strings.Contains(title, "magit-get-upstream-branch"):
			upstream := sanitizeSingleLine(strings.TrimSpace(m.snapshot.summary.Upstream))
			if upstream == "" {
				upstream = branch
			}
			return "Pull into " + upstream + " from"
		default:
			return "Pull into " + branch + " from"
		}
	case "magit-push":
		return "Push " + branch + " to"
	case "magit-rebase":
		return "Rebase " + branch + " onto"
	case "magit-remote-configure":
		return "Configure remote"
	default:
		return "Commands"
	}
}

func transientDefaultOption(binding keymap.Binding) OptionValue {
	if binding.Transient == "magit-shortlog" {
		switch binding.UpstreamCommand {
		case "transient:magit-shortlog:--numbered", "transient:magit-shortlog:--summary":
			return OptionValue{Enabled: true}
		}
	}
	return OptionValue{}
}

func keymapDisplayFor(sequence []string) string {
	parts := append([]string(nil), sequence...)
	for i := range parts {
		if parts[i] == "tab" {
			parts[i] = "Tab"
		}
		if parts[i] == "enter" {
			parts[i] = "Enter"
		}
	}
	return strings.Join(parts, " ")
}

func (m *Model) bindingCondition(binding keymap.Binding) (bool, string) {
	if binding.Transient == "magit-status-jump" {
		if target := statusJumpSections[binding.UpstreamCommand]; target != "" && m.tree.Section(target) == nil {
			return false, "status section is not present"
		}
	}
	for _, condition := range binding.Conditions {
		if matched, active, reason := m.operationBindingCondition(condition); matched {
			if !active {
				return false, reason
			}
			continue
		}
		if matched, active, reason := m.sparseCheckoutBindingCondition(condition); matched {
			if !active {
				return false, reason
			}
			continue
		}
		switch {
		case binding.Kind == keymap.KindInfix && strings.Contains(condition, "direct-configure"):
			return false, "configured in the corresponding Configure dialog"
		case strings.Contains(condition, "inapt-if-not") && strings.Contains(condition, "magit-get-some-remote") && len(m.snapshot.remotes) == 0:
			return false, "requires a configured remote"
		case strings.Contains(condition, "magit-get-some-remote") && len(m.snapshot.remotes) == 0:
			return false, "requires a configured remote"
		case strings.Contains(condition, "inapt-if-not") && strings.Contains(condition, "magit-get-current-branch") && (m.snapshot.summary.Branch == "" || m.snapshot.summary.Detached):
			return false, "requires a current local branch"
		case strings.Contains(condition, "inapt-if-not") && strings.Contains(condition, "magit-get-current-remote") && m.snapshot.summary.Upstream == "":
			return false, "requires a configured upstream"
		}
	}
	return true, ""
}

func (m *Model) sparseCheckoutBindingCondition(condition string) (matched, active bool, reason string) {
	if !strings.Contains(condition, "magit-sparse-checkout-enabled-p") {
		return false, false, ""
	}
	if m.repo == nil {
		return true, true, ""
	}
	state := m.snapshot.sparse
	negated := strings.Contains(condition, "if-not:")
	if negated == state.Enabled {
		if negated {
			return true, false, "requires sparse checkout to be disabled"
		}
		return true, false, "requires sparse checkout to be enabled"
	}
	return true, true, ""
}

func (m *Model) operationBindingCondition(condition string) (matched, active bool, reason string) {
	predicates := map[string]gitbackend.OperationKind{
		"magit-am-in-progress-p":        gitbackend.OperationApplyMailbox,
		"magit-bisect-in-progress-p":    gitbackend.OperationBisect,
		"magit-merge-in-progress-p":     gitbackend.OperationMerge,
		"magit-notes-merging-p":         gitbackend.OperationNotesMerge,
		"magit-rebase-in-progress-p":    gitbackend.OperationRebase,
		"magit-sequencer-in-progress-p": 0,
	}
	var predicate string
	var kind gitbackend.OperationKind
	for candidate, operationKind := range predicates {
		if strings.Contains(condition, candidate) {
			predicate, kind = candidate, operationKind
			break
		}
	}
	if predicate == "" {
		return false, false, ""
	}
	if m.repo == nil {
		return true, true, ""
	}
	state := m.snapshot.operations
	inProgress := false
	for _, operation := range state.Items {
		if predicate == "magit-sequencer-in-progress-p" {
			if operation.Kind == gitbackend.OperationCherryPick || operation.Kind == gitbackend.OperationRevert {
				inProgress = true
				break
			}
		} else if operation.Kind == kind {
			inProgress = true
			break
		}
	}
	negated := strings.Contains(condition, "if-not:")
	if negated == inProgress {
		name := strings.TrimSuffix(strings.TrimPrefix(predicate, "magit-"), "-p")
		if negated {
			return true, false, "requires no " + name
		}
		return true, false, "requires " + name
	}
	return true, true, ""
}

func (m *Model) optionConsumers(prefix string, option keymap.Binding) map[keymap.CommandID]bool {
	wanted := keymap.OptionConsumerCommands(prefix, option.UpstreamCommand)
	out := make(map[keymap.CommandID]bool, len(wanted))
	for _, upstream := range wanted {
		name := option.Transient
		for _, binding := range keymap.BindingsForTransient(schemeID(m.scheme), name) {
			if binding.Kind == keymap.KindSuffix && binding.UpstreamCommand == upstream {
				if _, registered := m.workflowHandlers[binding.Command]; registered || builtinUICommands[binding.Command] {
					out[binding.Command] = true
				}
			}
		}
	}
	for id, capability := range m.workflowCapabilities {
		if capability.Transient != option.Transient {
			continue
		}
		for _, consumed := range capability.Consumes {
			if consumed == option.UpstreamCommand {
				if _, installed := m.workflowHandlers[id]; installed {
					out[id] = true
				}
			}
		}
	}
	return out
}

func (catalog menuCatalog) occurrence(id string) (menuEntry, bool) {
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			if entry.Occurrence == id {
				return entry, true
			}
		}
	}
	return menuEntry{}, false
}

func lastDisplay(display string) string {
	parts := strings.Fields(display)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func keymapDisplay(display string) string {
	parts := strings.Fields(display)
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[1:], " ")
}

// dispatcherSection preserves Magit's source columns instead of regrouping
// commands by lazymagit's implementation status.
type dispatcherSection struct {
	Title   string
	Columns [][]menuEntry
}

func dispatcherCatalog(scheme keyScheme, contexts ...keymap.Context) []dispatcherSection {
	ctx := keymap.Context{View: keymap.ViewStatus, Scheme: schemeID(scheme)}
	if len(contexts) > 0 {
		ctx = contexts[0]
		ctx.Scheme = schemeID(scheme)
	}
	entry := func(key, label string, command keymap.CommandID) menuEntry {
		available, reason := false, "presentation-only dispatcher adaptation"
		category := menuEntryPresentation
		if b, ok := keymap.Find(schemeID(scheme), keymap.ContextStatus, key); ok {
			available, reason = b.Available(ctx)
			command = b.Command
			category = menuEntryRegistry
			if !available {
				category = menuEntryContext
			}
		}
		return menuEntry{Key: key, Display: key, Label: label, Available: available, Command: command, Reason: reason, Category: category, Kind: keymap.KindBinding}
	}
	discard := entry("x", "Discard", keymap.CommandDiscard)
	if scheme == schemeMagit {
		discard = entry("k", "Discard", keymap.CommandDiscard)
	}
	sections := []dispatcherSection{
		{Title: "Transient and dwim commands", Columns: [][]menuEntry{
			{{Key: "A", Label: "Apply"}, entry("b", "Branch", keymap.CommandNone), {Key: "B", Label: "Bisect"}, entry("c", "Commit", keymap.CommandNone), {Key: "C", Label: "Clone"}, {Key: "d", Label: "Diff"}, {Key: "D", Label: "Diff (change)"}, {Key: "e", Label: "Ediff (dwim)"}, {Key: "E", Label: "Ediff"}, entry("f", "Fetch", keymap.CommandNone), {Key: "F", Label: "Pull"}, {Key: "h", Label: "Help"}, {Key: "H", Label: "Section info"}},
			{{Key: "i", Label: "Ignore"}, {Key: "I", Label: "Init"}, {Key: "j", Label: "Jump to section"}, {Key: "J", Label: "Display buffer"}, {Key: "l", Label: "Log"}, {Key: "L", Label: "Log (change)"}, {Key: "m", Label: "Merge"}, entry("M", "Remote", keymap.CommandNone), {Key: "o", Label: "Submodule"}, {Key: "O", Label: "Subtree"}, entry("P", "Push", keymap.CommandNone), {Key: "Q", Label: "Command"}},
			{{Key: "r", Label: "Rebase"}, {Key: "t", Label: "Tag"}, {Key: "T", Label: "Note"}, {Key: "V", Label: "Revert"}, {Key: "w", Label: "Apply patches"}, {Key: "W", Label: "Format patches"}, {Key: "X", Label: "Reset"}, {Key: "y", Label: "Show Refs"}, {Key: "Y", Label: "Cherries"}, {Key: "z", Label: "Stash"}, {Key: "Z", Label: "Worktree"}, {Key: "!", Label: "Run"}},
		}},
		{Title: "Applying changes", Columns: [][]menuEntry{
			{{Key: "a", Label: "Apply"}, {Key: "v", Label: "Reverse"}, discard},
			{entry("s", "Stage", keymap.CommandStage), entry("u", "Unstage", keymap.CommandUnstage)},
			{entry("S", "Stage all tracked changes", keymap.CommandStageAll), entry("U", "Unstage all", keymap.CommandUnstageAll)},
		}},
		{Title: "Essential commands", Columns: [][]menuEntry{
			{entry("g", "Refresh current buffer", keymap.CommandRefresh), entry("q", "Close dispatcher", keymap.CommandQuit), {Key: "tab", Display: "Tab", Label: "Toggle section", Available: true, Command: keymap.CommandToggleSection}, {Key: "enter", Display: "Enter", Label: "Visit thing"}},
			{entry("$", "Git processes", keymap.CommandShowProcesses), {Key: "ctrl+x m", Display: "C-x m", Label: "Show all key bindings"}, {Key: "ctrl+x i", Display: "C-x i", Label: "Show Info manual"}},
		}},
	}
	for sectionIndex := range sections {
		for columnIndex := range sections[sectionIndex].Columns {
			for entryIndex := range sections[sectionIndex].Columns[columnIndex] {
				item := &sections[sectionIndex].Columns[columnIndex][entryIndex]
				if binding, ok := keymap.Find(schemeID(scheme), keymap.ContextStatus, item.Key); ok && binding.UpstreamCommand != "" {
					item.Command = binding.Command
					item.Available, item.Reason = binding.Available(ctx)
					item.Category, item.Kind = menuEntryRegistry, binding.Kind
					if !item.Available && binding.UpstreamCommand != "" {
						item.Reason = binding.UpstreamCommand + " (" + string(binding.Parity) + ")"
						item.Category = menuEntryMissing
					}
				}
				// Rows without a status-map identity are intentional Magit dispatcher
				// presentation adaptations, never implicitly grey zero values.
				if !item.Available && item.Category == "" {
					item.Category = menuEntryPresentation
					item.Reason = "presentation-only dispatcher adaptation"
					item.Kind = keymap.KindBinding
				}
			}
		}
	}
	return sections
}

// dispatcherCatalog overlays package-init domain declarations on the static
// manifest sheet. Static "missing" classifications therefore never mask an
// installed exact workflow capability.
func (m *Model) dispatcherCatalog() []dispatcherSection {
	sections := dispatcherCatalog(m.scheme, m.keyContext())
	for si := range sections {
		for ci := range sections[si].Columns {
			for ei := range sections[si].Columns[ci] {
				entry := &sections[si].Columns[ci][ei]
				binding, ok := keymap.Find(schemeID(m.scheme), keymap.ContextStatus, entry.Key)
				if !ok {
					continue
				}
				if _, installed := m.workflowHandlers[binding.Command]; installed {
					entry.Command, entry.Available, entry.Reason, entry.Category = binding.Command, true, "", menuEntryRegistry
					continue
				}
				for id, capability := range m.workflowCapabilities {
					if capability.UpstreamCommand == binding.UpstreamCommand {
						if _, installed := m.workflowHandlers[id]; installed {
							entry.Command, entry.Available, entry.Reason, entry.Category = id, true, "", menuEntryRegistry
						}
					}
				}
			}
		}
	}
	return sections
}

func dispatcherEntry(sections []dispatcherSection, key string) (menuEntry, bool) {
	for _, section := range sections {
		for _, column := range section.Columns {
			for _, entry := range column {
				if entry.Key == key {
					return entry, true
				}
			}
		}
	}
	return menuEntry{}, false
}

func (catalog menuCatalog) entry(key string) (menuEntry, bool) {
	if len(key) > 1 && !strings.ContainsRune(key, ' ') && strings.ContainsRune("-+=", rune(key[0])) {
		key = strings.Join(strings.Split(key, ""), " ")
	}
	var fallback *menuEntry
	var activeFallback *menuEntry
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			if entry.Key == key {
				copy := entry
				if entry.Active && entry.Available {
					return entry, true
				}
				if entry.Active && activeFallback == nil {
					activeFallback = &copy
				}
				if fallback == nil {
					fallback = &copy
				}
			}
		}
	}
	if activeFallback != nil {
		return *activeFallback, true
	}
	if fallback != nil {
		return *fallback, true
	}
	return menuEntry{}, false
}

func (catalog menuCatalog) hasDescendant(key string) bool {
	prefix := key + " "
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			if strings.HasPrefix(entry.Key, prefix) {
				return true
			}
		}
	}
	return false
}

// renderDispatcher renders Magit's dispatch sheet on its own borderless
// canvas. Its paging math intentionally does not share transient chrome.
func renderDispatcher(sections []dispatcherSection, width, height, offset int) string {
	if width <= 0 || height <= 0 {
		return fitBlock("", max(0, width), max(0, height))
	}
	canvas := dispatcherCanvas(sections, width)
	viewport := dispatcherViewportHeight(width, height, len(canvas))
	maximum := max(0, len(canvas)-viewport)
	offset = min(max(0, offset), maximum)
	end := min(len(canvas), offset+viewport)
	visible := append([]string(nil), canvas[offset:end]...)
	first := 0
	if len(canvas) > 0 {
		first = offset + 1
	}
	if height == 1 {
		marker := dispatcherRangeHint(width, first, end, len(canvas))
		if len(visible) == 0 {
			visible = []string{marker}
		} else {
			available := max(0, width-ansi.StringWidth(marker)-1)
			visible[0] = truncate(visible[0], available) + " " + marker
		}
		return fitBlock(strings.Join(visible, "\n"), width, height)
	}
	hints := dispatcherHintLines(width, first, end, len(canvas))
	for _, hint := range hints[:min(len(hints), height-viewport)] {
		visible = append(visible, lipgloss.NewStyle().Foreground(colorMuted).Render(hint))
	}
	return fitBlock(strings.Join(visible, "\n"), width, height)
}

func dispatcherRangeHint(_ int, first, end, total int) string {
	arrows := ""
	if first > 1 {
		arrows += "↑"
	}
	if end < total {
		arrows += "↓"
	}
	return arrows + strconv.Itoa(first) + "-" + strconv.Itoa(end) + "/" + strconv.Itoa(total)
}

func dispatcherHintLines(width, first, end, total int) []string {
	components := []string{"q/Esc close", "↑/↓ PageUp/PageDown", dispatcherRangeHint(width, first, end, total), "× unavailable"}
	var lines []string
	for _, component := range components {
		component = truncate(component, width)
		if len(lines) == 0 || ansi.StringWidth(lines[len(lines)-1])+2+ansi.StringWidth(component) > width {
			lines = append(lines, component)
		} else {
			lines[len(lines)-1] += "  " + component
		}
	}
	return lines
}

func dispatcherViewportHeight(width, height, total int) int {
	if height <= 1 {
		return 1
	}
	hintRows := len(dispatcherHintLines(width, total, total, total))
	return max(1, height-min(height-1, hintRows))
}

func dispatcherMaximumOffset(sections []dispatcherSection, width, height int) int {
	total := len(dispatcherCanvas(sections, width))
	return max(0, total-dispatcherViewportHeight(width, height, total))
}

func dispatcherCanvas(sections []dispatcherSection, width int) []string {
	var canvas []string
	for _, section := range sections {
		canvas = append(canvas, lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(
			truncate(sanitizeSingleLine(section.Title), width)))
		columnsAtOnce := dispatcherColumnCount(len(section.Columns), width)
		for first := 0; first < len(section.Columns); first += columnsAtOnce {
			last := min(len(section.Columns), first+columnsAtOnce)
			count := last - first
			gap := 2
			cellWidth := max(1, (width-gap*(count-1))/count)
			rowCount := 0
			for _, column := range section.Columns[first:last] {
				rowCount = max(rowCount, len(column))
			}
			for row := 0; row < rowCount; row++ {
				cells := make([]string, 0, count)
				for _, column := range section.Columns[first:last] {
					cell := ""
					if row < len(column) {
						cell = renderDispatcherEntry(column[row], cellWidth)
					}
					cells = append(cells, padANSI(cell, cellWidth))
				}
				canvas = append(canvas, strings.Join(cells, strings.Repeat(" ", gap)))
			}
		}
	}
	return canvas
}

func dispatcherColumnCount(sourceColumns, width int) int {
	switch {
	case sourceColumns >= 3 && width >= 66:
		return 3
	case sourceColumns >= 2 && width >= 44:
		return 2
	default:
		return 1
	}
}

func renderDispatcherEntry(entry menuEntry, width int) string {
	key := sanitizeSingleLine(entry.Display)
	if key == "" {
		key = sanitizeSingleLine(entry.Key)
	}
	label := sanitizeSingleLine(entry.Label)
	if !entry.Available {
		return truncate(lipgloss.NewStyle().Foreground(colorMuted).Faint(true).Render(key+" × "+label), width)
	}
	return truncate(lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render(key)+" "+label, width)
}

func unavailableMarker(entry menuEntry) string {
	if entry.Category == menuEntryContext {
		return " ! "
	}
	if entry.Kind == keymap.KindInfix {
		return " [infix] "
	}
	return " × "
}

func visibleTransientCatalog(catalog menuCatalog) menuCatalog {
	visible := menuCatalog{Title: catalog.Title}
	for _, group := range catalog.Groups {
		filtered := menuGroup{Title: group.Title}
		for _, entry := range group.Entries {
			if !entry.Active && hasDirectConfigureCondition(entry.Conditions) {
				continue
			}
			filtered.Entries = append(filtered.Entries, entry)
		}
		if len(filtered.Entries) > 0 {
			visible.Groups = append(visible.Groups, filtered)
		}
	}
	return visible
}

func hasDirectConfigureCondition(conditions []string) bool {
	for _, condition := range conditions {
		if strings.Contains(condition, "direct-configure") {
			return true
		}
	}
	return false
}

// renderTransient renders a bordered, vertically scrollable grid of groups.
// Very short viewports use a borderless command summary so the choices never
// disappear behind border and title chrome.
func renderTransient(catalog menuCatalog, width, height, offset int) string {
	catalog = visibleTransientCatalog(catalog)
	if width <= 0 || height <= 0 {
		return fitBlock("", max(0, width), max(0, height))
	}
	if width < 4 || height < 5 {
		return renderCompactTransient(catalog, width, height, offset)
	}
	innerW, innerH := width-4, height-2
	canvas := transientCanvas(catalog, innerW)
	viewportH := transientViewportHeight(catalog, width, height)
	if viewportH < 1 {
		return renderCompactTransient(catalog, width, height, offset)
	}
	maximum := max(0, len(canvas)-viewportH)
	offset = min(max(0, offset), maximum)
	end := min(len(canvas), offset+viewportH)
	visible := canvas[offset:end]
	title := lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(" " + sanitizeSingleLine(catalog.Title) + " ")
	first := 0
	if len(canvas) > 0 {
		first = offset + 1
	}
	hints := transientHintLines(innerW, len(canvas) > viewportH, first, end, len(canvas))
	hint := lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Join(hints, "\n"))
	content := title + "\n" + strings.Join(visible, "\n") + "\n" + hint
	text := fitBlock(content, innerW, innerH)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).
		Border(lipgloss.DoubleBorder()).BorderForeground(colorPurple).Render(text)
}

func transientViewportHeight(catalog menuCatalog, width, height int) int {
	if width < 4 || height < 5 {
		return 0
	}
	innerW, innerH := width-4, height-2
	canvasRows := len(transientCanvas(catalog, innerW))
	baseHintRows := len(transientHintLines(innerW, false, 0, 0, canvasRows))
	viewport := innerH - 1 - baseHintRows
	if canvasRows > viewport {
		hintRows := len(transientHintLines(innerW, true, canvasRows, canvasRows, canvasRows))
		viewport = innerH - 1 - hintRows
	}
	return max(0, viewport)
}

func transientHintLines(width int, clipped bool, first, end, total int) []string {
	components := []string{"Esc/q close", "× unavailable"}
	if clipped {
		components = append(components, "↑/↓ PgUp/PgDn", strconv.Itoa(first)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(total))
	}
	var lines []string
	for _, component := range components {
		component = truncate(component, width)
		if len(lines) == 0 || ansi.StringWidth(lines[len(lines)-1])+2+ansi.StringWidth(component) > width {
			lines = append(lines, component)
		} else {
			lines[len(lines)-1] += "  " + component
		}
	}
	return lines
}

func renderCompactTransient(catalog menuCatalog, width, height, offset int) string {
	var entries []string
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			if entry.Available {
				key := entry.Display
				if key == "" {
					key = entry.Key
				}
				entries = append(entries, sanitizeSingleLine(key)+" "+sanitizeSingleLine(entry.Label))
			}
		}
	}
	if len(entries) == 0 {
		entries = append(entries, "× unavailable")
	}
	maximum := max(0, len(entries)-height)
	offset = min(max(0, offset), maximum)
	end := min(len(entries), offset+height)
	visible := entries[offset:end]
	prefix := sanitizeSingleLine(catalog.Title) + ": "
	lines := make([]string, 0, height)
	for _, entry := range visible {
		linePrefix := ""
		if maximum == 0 && len(lines) == 0 && ansi.StringWidth(prefix)+ansi.StringWidth(entry) <= width {
			linePrefix = prefix
		}
		lines = append(lines, truncate(linePrefix+entry, width))
	}
	if maximum > 0 && len(lines) > 0 {
		marker := ""
		if offset > 0 {
			marker += "↑"
		}
		if end < len(entries) {
			marker += "↓"
		}
		marker += strconv.Itoa(offset+1) + "-" + strconv.Itoa(end) + "/" + strconv.Itoa(len(entries))
		available := max(0, width-ansi.StringWidth(marker)-1)
		lines[len(lines)-1] = truncate(lines[len(lines)-1], available) + " " + marker
	}
	return fitBlock(strings.Join(lines, "\n"), width, height)
}

func transientCanvas(catalog menuCatalog, innerW int) []string {
	columns := transientColumnCount(catalog, innerW)
	gap := 2
	widths := transientColumnWidths(catalog, columns)
	preferredTotal := gap * (columns - 1)
	for _, width := range widths {
		preferredTotal += width
	}
	for extra, slot := max(0, innerW-preferredTotal), 0; extra > 0; extra, slot = extra-1, (slot+1)%columns {
		widths[slot]++
	}

	var canvas []string
	for first := 0; first < len(catalog.Groups); first += columns {
		last := min(len(catalog.Groups), first+columns)
		blocks := make([][]string, 0, last-first)
		rowH := 0
		for slot, group := range catalog.Groups[first:last] {
			block := renderMenuGroup(group, widths[slot])
			blocks = append(blocks, block)
			rowH = max(rowH, len(block))
		}
		for line := 0; line < rowH; line++ {
			var cells []string
			for slot, block := range blocks {
				cell := ""
				if line < len(block) {
					cell = block[line]
				}
				cells = append(cells, padANSI(cell, widths[slot]))
			}
			canvas = append(canvas, strings.Join(cells, strings.Repeat(" ", gap)))
		}
		if last < len(catalog.Groups) {
			canvas = append(canvas, "")
		}
	}

	return canvas
}

func transientColumnCount(catalog menuCatalog, innerW int) int {
	limit := min(3, max(1, len(catalog.Groups)))
	for columns := limit; columns >= 1; columns-- {
		widths := transientColumnWidths(catalog, columns)
		total := 2 * (columns - 1)
		for _, width := range widths {
			total += width
		}
		if total <= innerW || columns == 1 {
			return columns
		}
	}
	return 1
}

func transientColumnWidths(catalog menuCatalog, columns int) []int {
	widths := make([]int, columns)
	for i, group := range catalog.Groups {
		slot := i % columns
		widths[slot] = max(widths[slot], preferredMenuGroupWidth(group))
	}
	for i := range widths {
		widths[i] = max(1, widths[i])
	}
	return widths
}

func preferredMenuGroupWidth(group menuGroup) int {
	width := ansi.StringWidth(sanitizeSingleLine(group.Title))
	for _, entry := range group.Entries {
		marker := " "
		if !entry.Available {
			marker = unavailableMarker(entry)
		}
		key := entry.Display
		if key == "" {
			key = entry.Key
		}
		width = max(width, ansi.StringWidth(sanitizeSingleLine(key)+marker+sanitizeSingleLine(entry.Label)))
	}
	return width
}

func transientMaximumOffset(catalog menuCatalog, width, height int) int {
	catalog = visibleTransientCatalog(catalog)
	if width < 4 || height < 5 {
		available := 0
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				if entry.Available {
					available++
				}
			}
		}
		if available == 0 {
			available = 1
		}
		return max(0, available-max(1, height))
	}
	return max(0, len(transientCanvas(catalog, width-4))-transientViewportHeight(catalog, width, height))
}

func renderMenuGroup(group menuGroup, width int) []string {
	var lines []string
	if title := sanitizeSingleLine(strings.TrimSpace(group.Title)); title != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(truncate(title, width)))
	}
	keyW := 0
	for _, entry := range group.Entries {
		key := entry.Display
		if key == "" {
			key = entry.Key
		}
		keyW = max(keyW, ansi.StringWidth(sanitizeSingleLine(key)))
	}
	for _, entry := range group.Entries {
		display := entry.Display
		if display == "" {
			display = entry.Key
		}
		key := padANSI(sanitizeSingleLine(display), keyW)
		label := sanitizeSingleLine(entry.Label)
		var line string
		if entry.Available {
			value := ""
			if entry.Kind == keymap.KindInfix {
				if entry.Option.Value != "" {
					value = " =" + sanitizeSingleLine(entry.Option.Value)
				} else if entry.Option.Enabled {
					value = " [on]"
				} else {
					value = " [off]"
				}
			}
			line = lipgloss.NewStyle().Foreground(colorGold).Bold(true).Render(key) + " " + label + value
		} else {
			line = key + unavailableMarker(entry) + label
			if entry.Category == menuEntryContext {
				line = lipgloss.NewStyle().Foreground(colorCyan).Render(line)
			} else {
				line = lipgloss.NewStyle().Foreground(colorMuted).Faint(true).Render(line)
			}
		}
		lines = append(lines, truncate(line, width))
	}
	return lines
}

func padANSI(value string, width int) string {
	value = truncate(value, width)
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}
