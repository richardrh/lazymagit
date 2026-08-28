// Package keymap owns the canonical command and binding contract shared by the
// resolver, menus, footer, parity ledger, and validation tests.
package keymap

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// CommandID is a stable, scalable application command identity.
type CommandID string

const (
	CommandNone           CommandID = ""
	CommandMoveDown       CommandID = "ui.move-down"
	CommandMoveUp         CommandID = "ui.move-up"
	CommandFirst          CommandID = "ui.first"
	CommandLast           CommandID = "ui.last"
	CommandRefresh        CommandID = "status.refresh"
	CommandToggleSection  CommandID = "section.toggle"
	CommandStage          CommandID = "change.stage"
	CommandUnstage        CommandID = "change.unstage"
	CommandStageAll       CommandID = "change.stage-all"
	CommandUnstageAll     CommandID = "change.unstage-all"
	CommandDiscard        CommandID = "change.discard"
	CommandCommit         CommandID = "commit.create"
	CommandSwitchBranch   CommandID = "branch.switch"
	CommandPush           CommandID = "push.configured"
	CommandFetchUpstream  CommandID = "fetch.upstream"
	CommandFetchPush      CommandID = "fetch.push-remote"
	CommandFetchElsewhere CommandID = "fetch.elsewhere"
	CommandFetchAll       CommandID = "fetch.all"
	CommandAddRemote      CommandID = "remote.add"
	CommandShowProcesses  CommandID = "process.toggle"
	CommandOpenDispatcher CommandID = "transient.dispatch"
	CommandQuit           CommandID = "ui.quit"
	// CommandDepth1 through CommandDepth3 are retained for compatibility with
	// earlier callers. New section-depth bindings distinguish local and global
	// scope, as Magit does.
	CommandDepth1             CommandID = "section.depth-1"
	CommandDepth2             CommandID = "section.depth-2"
	CommandDepth3             CommandID = "section.depth-3"
	CommandScrollDown         CommandID = "detail.page-down"
	CommandScrollUp           CommandID = "detail.page-up"
	CommandSectionCycle       CommandID = "section.cycle"
	CommandSectionCycleGlobal CommandID = "section.cycle-global"
	CommandSectionParent      CommandID = "section.parent"
	CommandSiblingPrevious    CommandID = "section.sibling-previous"
	CommandSiblingNext        CommandID = "section.sibling-next"
	CommandLocalDepth1        CommandID = "section.local-depth-1"
	CommandLocalDepth2        CommandID = "section.local-depth-2"
	CommandLocalDepth3        CommandID = "section.local-depth-3"
	CommandLocalDepth4        CommandID = "section.local-depth-4"
	CommandGlobalDepth1       CommandID = "section.global-depth-1"
	CommandGlobalDepth2       CommandID = "section.global-depth-2"
	CommandGlobalDepth3       CommandID = "section.global-depth-3"
	CommandGlobalDepth4       CommandID = "section.global-depth-4"
	CommandVisitThing         CommandID = "status.visit-thing"
	CommandCycleDiffs         CommandID = "detail.cycle"
	CommandDetailBackward     CommandID = "detail.page-backward"
	CommandDiffMoreContext    CommandID = "detail.more-context"
	CommandDiffLessContext    CommandID = "detail.less-context"
	CommandDiffDefaultContext CommandID = "detail.default-context"
	CommandDescribeSection    CommandID = "section.describe"
	CommandStatusJump         CommandID = "status.jump"
	CommandDisplayRepository  CommandID = "status.display-repository"
	CommandCopyThing          CommandID = "status.copy-thing"
	CommandCopySectionValue   CommandID = "section.copy-value"
	CommandCopyBufferRevision CommandID = "status.copy-revision"
)

type View uint8

const (
	ViewNone View = iota
	ViewStatus
)

type Section uint8

const (
	SectionNone Section = iota
	SectionUnstaged
	SectionStaged
)

// Scheme makes intentional Vim/Magit collisions data, not resolver branches.
type Scheme string

const (
	SchemeVim   Scheme = "vim"
	SchemeMagit Scheme = "magit"
)

type Context struct {
	View    View
	Section Section
	Scheme  Scheme
}

type Parity string

const (
	ParityExactSlice    Parity = "exact-slice"
	ParityPartial       Parity = "partial"
	ParityAdapted       Parity = "adapted"
	ParityNotApplicable Parity = "not-applicable"
	ParityMissing       Parity = "missing"
)

type Handler string

const (
	HandlerExecute     Handler = "execute"
	HandlerPrefix      Handler = "prefix"
	HandlerInfix       Handler = "infix"
	HandlerUnsupported Handler = "unsupported"
)

type EntryKind string

const (
	KindBinding EntryKind = "binding"
	KindSuffix  EntryKind = "suffix"
	KindInfix   EntryKind = "infix"
)

type UnavailableCategory string

const (
	UnavailableNone          UnavailableCategory = ""
	UnavailableMissing       UnavailableCategory = "missing"
	UnavailableNotApplicable UnavailableCategory = "not-applicable"
	UnavailableInfix         UnavailableCategory = "infix"
	UnavailableContext       UnavailableCategory = "context"
	UnavailableUnsupported   UnavailableCategory = "backend-unsupported"
)

type Availability string

const (
	AvailabilityAlways   Availability = "always"
	AvailabilityUnstaged Availability = "unstaged-file"
	AvailabilityStaged   Availability = "staged-file"
	AvailabilityChange   Availability = "changed-file"
	AvailabilityNever    Availability = "never"
)

const (
	ContextStatus    = "status"
	ContextTransient = "transient:"
)

type Source struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Definition string `json:"definition"`
}

// Binding is the canonical representation. Sequence is input, while Display
// is presentation text; they must never be conflated (notably tab/Tab).
type Binding struct {
	// Occurrence identifies this exact manifest row. Command identifies behavior;
	// conditional duplicate rows may intentionally share a Command.
	Occurrence          string
	Sequence            []string
	Display             string
	Command             CommandID
	Label               string
	Scheme              Scheme
	Context             string
	Parity              Parity
	UpstreamCommand     string
	UpstreamKey         string
	Kind                EntryKind
	InfixClass          string
	Argument            string
	TakesValue          bool
	Domain              string
	Layer               string
	Source              Source
	Handler             Handler
	Availability        Availability
	Unavailable         string
	UnavailableCategory UnavailableCategory
	Group               string
	Conditions          []string
	Primary             bool
	EffectiveTop        bool
	// Transient is the upstream transient that owns this occurrence. Sequence
	// includes one canonical route for documentation compatibility; LocalSequence
	// is the suffix as declared by that transient and is used for runtime routing.
	Transient      string
	ChildTransient string
	LocalSequence  []string
}

// Transient describes one exact manifest transient and its canonical route.
type Transient struct {
	Name, Title string
	Sequence    []string
	Source      Source
}

func (b Binding) Available(ctx Context) (bool, string) {
	switch b.Availability {
	case AvailabilityNever:
		return false, b.Unavailable
	case AvailabilityUnstaged:
		if ctx.Section != SectionUnstaged {
			return false, "select an unstaged or untracked file"
		}
	case AvailabilityStaged:
		if ctx.Section != SectionStaged {
			return false, "select a staged file"
		}
	case AvailabilityChange:
		if ctx.Section != SectionUnstaged && ctx.Section != SectionStaged {
			return false, "select a changed file"
		}
	}
	return true, ""
}

//go:embed testdata/magit-v4.7.0-status-bindings.json
var upstreamManifest []byte

type manifest struct {
	SchemaVersion string `json:"schema_version"`
	Subject       string `json:"subject"`
	Upstream      struct {
		Version, Commit, Tag string
		CheckoutClean        bool `json:"checkout_clean"`
	} `json:"upstream"`
	Scope struct {
		Mode          string
		MapChain      []string `json:"map_chain"`
		TransientRule string   `json:"transient_rule"`
	} `json:"scope"`
	Top []struct {
		Key, Command, Kind, Domain, Layer string
		Effective                         bool
		Source                            Source
	} `json:"top_level_bindings"`
	Transients []struct {
		Name    string
		Source  Source
		Entries []struct {
			Key, Command, Kind, Class string
			Description               json.RawMessage
			Argument                  *string
			Groups                    []string
			Conditions                []struct{ Type, Expression string }
			Domain                    string
		} `json:"entries"`
	} `json:"transients"`
}

var registry = buildRegistry()

// Registry returns a defensive copy of the canonical binding registry.
func Registry() []Binding {
	out := make([]Binding, len(registry))
	copy(out, registry)
	for i := range out {
		out[i].Sequence = append([]string(nil), out[i].Sequence...)
		out[i].LocalSequence = append([]string(nil), out[i].LocalSequence...)
		out[i].Conditions = append([]string(nil), out[i].Conditions...)
	}
	return out
}

func buildRegistry() []Binding {
	var m manifest
	if err := json.Unmarshal(upstreamManifest, &m); err != nil {
		panic("embedded Magit manifest: " + err.Error())
	}
	transientNames := make(map[string]bool, len(m.Transients))
	for _, tr := range m.Transients {
		transientNames[tr.Name] = true
	}
	var out []Binding
	for _, top := range m.Top {
		if !top.Effective {
			continue
		}
		seq := canonicalSequence(top.Key)
		if top.Domain == "emacs" || strings.Contains(top.Key, "<remap>") || strings.Contains(top.Key, "<left-fringe>") {
			seq = []string{"emacs:" + top.Key}
		}
		b := Binding{Sequence: seq, Display: displaySequence(seq), Command: CommandID("missing/" + top.Command), Label: friendlyLabel(top.Command), Scheme: SchemeMagit, Context: ContextStatus, Parity: ParityMissing, UpstreamCommand: top.Command, UpstreamKey: top.Key, Kind: EntryKind(top.Kind), Domain: top.Domain, Layer: top.Layer, Source: top.Source, Handler: HandlerUnsupported, Availability: AvailabilityNever, Unavailable: "not implemented", UnavailableCategory: UnavailableMissing, EffectiveTop: true}
		classifyTop(&b, transientNames)
		out = append(out, b)
		if keyAvailableInVim(strings.Join(b.Sequence, " ")) {
			copy := b
			copy.Scheme = SchemeVim
			if copy.Handler != HandlerUnsupported {
				copy.Parity = ParityAdapted
			}
			copy.EffectiveTop = false
			out = append(out, copy)
		}
	}
	// Vim adaptations are explicit collision metadata and are intentionally not
	// part of the 98-entry upstream ledger.
	out = append(out,
		vim("j", CommandMoveDown, "Next row", HandlerExecute),
		vim("k", CommandMoveUp, "Previous row", HandlerExecute),
		vim("g", CommandRefresh, "Refresh", HandlerExecute),
		vim("g g", CommandFirst, "First row", HandlerExecute),
		vim("G", CommandLast, "Last row", HandlerExecute),
		vim("x", CommandDiscard, "Discard", HandlerExecute),
	)
	out = append(out, transientBindings(m)...)
	return out
}

func keyAvailableInVim(key string) bool {
	switch key {
	case "j", "k", "g", "G", "x", "n", "p":
		return false
	}
	return true
}

func classifyTop(b *Binding, transientNames map[string]bool) {
	key := strings.Join(b.Sequence, " ")
	navigation := map[string]CommandID{
		"magit-section-cycle":             CommandSectionCycle,
		"magit-section-cycle-global":      CommandSectionCycleGlobal,
		"magit-section-up":                CommandSectionParent,
		"magit-section-backward-sibling":  CommandSiblingPrevious,
		"magit-section-forward-sibling":   CommandSiblingNext,
		"magit-section-show-level-1":      CommandLocalDepth1,
		"magit-section-show-level-2":      CommandLocalDepth2,
		"magit-section-show-level-3":      CommandLocalDepth3,
		"magit-section-show-level-4":      CommandLocalDepth4,
		"magit-section-show-level-1-all":  CommandGlobalDepth1,
		"magit-section-show-level-2-all":  CommandGlobalDepth2,
		"magit-section-show-level-3-all":  CommandGlobalDepth3,
		"magit-section-show-level-4-all":  CommandGlobalDepth4,
		"magit-visit-thing":               CommandVisitThing,
		"magit-section-cycle-diffs":       CommandCycleDiffs,
		"magit-diff-show-or-scroll-down":  CommandDetailBackward,
		"magit-diff-more-context":         CommandDiffMoreContext,
		"magit-diff-less-context":         CommandDiffLessContext,
		"magit-diff-default-context":      CommandDiffDefaultContext,
		"magit-describe-section":          CommandDescribeSection,
		"magit-display-repository-buffer": CommandDisplayRepository,
		"magit-copy-thing":                CommandCopyThing,
		"magit-copy-section-value":        CommandCopySectionValue,
		"magit-copy-buffer-revision":      CommandCopyBufferRevision,
	}
	if command, ok := navigation[b.UpstreamCommand]; ok {
		b.Command, b.Handler, b.Availability, b.Parity = command, HandlerExecute, AvailabilityAlways, ParityPartial
		if command == CommandCycleDiffs {
			// A terminal has no inline diff sections. The UI safely cycles the
			// selected row's detail pane instead.
			b.Parity = ParityAdapted
		}
		return
	}
	implemented := map[string]CommandID{
		"tab": CommandToggleSection, "n": CommandMoveDown, "p": CommandMoveUp,
		"1": CommandDepth1, "2": CommandDepth2, "3": CommandDepth3,
		"space": CommandScrollDown, "shift+space": CommandScrollUp,
		"g": CommandRefresh, "G": CommandRefresh, "h": CommandOpenDispatcher,
		"?": CommandOpenDispatcher, "k": CommandDiscard, "s": CommandStage,
		"S": CommandStageAll, "u": CommandUnstage, "U": CommandUnstageAll,
		"$": CommandShowProcesses, "q": CommandQuit,
		"b": "transient.branch", "c": "transient.commit", "f": "transient.fetch",
		"P": "transient.push", "M": "transient.remote",
	}
	if transientNames[b.UpstreamCommand] {
		b.Command = transientCommandID(b.UpstreamCommand)
		b.Handler, b.Availability, b.Parity, b.Primary = HandlerPrefix, AvailabilityAlways, ParityPartial, true
		return
	}
	if command, ok := implemented[key]; ok {
		b.Command, b.Handler, b.Availability, b.Parity = command, HandlerExecute, AvailabilityAlways, ParityPartial
		if strings.HasPrefix(string(command), "transient.") {
			b.Handler = HandlerPrefix
			b.Primary = true
		}
		switch command {
		case CommandStage:
			b.Availability, b.UnavailableCategory = AvailabilityUnstaged, UnavailableContext
		case CommandUnstage:
			b.Availability, b.UnavailableCategory = AvailabilityStaged, UnavailableContext
		case CommandDiscard:
			b.Availability, b.UnavailableCategory = AvailabilityChange, UnavailableContext
		}
		return
	}
	if b.UpstreamCommand == "magit-ediff-dwim" {
		// Ediff is adapted by the UI to a read-only unified terminal comparison;
		// the external split-window editor is never launched.
		b.Command, b.Handler, b.Availability, b.Parity = CommandID("inspection.ediff-dwim"), HandlerExecute, AvailabilityAlways, ParityAdapted
		return
	}
	if b.UpstreamCommand == "magit-dispatch" {
		b.Command, b.Handler, b.Availability, b.Parity = CommandOpenDispatcher, HandlerExecute, AvailabilityAlways, ParityPartial
		return
	}
	if b.UpstreamCommand == "magit-dired-jump" || strings.Contains(b.UpstreamKey, "<left-fringe>") || strings.Contains(b.UpstreamKey, "<remap>") {
		b.Parity, b.Unavailable, b.UnavailableCategory = ParityNotApplicable, "Emacs-only input or integration", UnavailableNotApplicable
	}
}

func vim(sequence string, command CommandID, label string, handler Handler) Binding {
	seq := strings.Split(sequence, " ")
	b := Binding{Sequence: seq, Display: displaySequence(seq), Command: command, Label: label, Scheme: SchemeVim, Context: ContextStatus, Parity: ParityAdapted, Handler: handler, Availability: AvailabilityAlways}
	if command == CommandDiscard {
		b.Availability, b.UnavailableCategory = AvailabilityUnstaged, UnavailableContext
	}
	return b
}

func transientBindings(m manifest) []Binding {
	routes := transientRoutes(m)
	names := make(map[string]bool, len(m.Transients))
	for _, tr := range m.Transients {
		names[tr.Name] = true
	}
	orphans := orphanTransientNames(m, names)
	implemented := map[string]CommandID{
		"magit-branch\x00magit-checkout": CommandSwitchBranch, "magit-commit\x00magit-commit-create": CommandCommit,
		"magit-fetch\x00magit-fetch-from-pushremote": CommandFetchPush, "magit-fetch\x00magit-fetch-from-upstream": CommandFetchUpstream,
		"magit-fetch\x00magit-fetch-other": CommandFetchElsewhere, "magit-fetch\x00magit-fetch-all": CommandFetchAll,
		"magit-push\x00magit-push-current-to-pushremote": CommandPush, "magit-remote\x00magit-remote-add": CommandAddRemote,
	}
	var out []Binding
	for _, tr := range m.Transients {
		prefix := strings.Join(routes[tr.Name], " ")
		for occurrence, entry := range tr.Entries {
			kind := EntryKind(entry.Kind)
			childTransient := ""
			// Every suffix has a stable domain behavior identity even before its UI
			// workflow is installed. This lets a domain file make it executable by
			// registration without modifying the central registry.
			command := domainCommandID(tr.Name, entry.Command)
			handler, availability, parity := HandlerUnsupported, AvailabilityNever, ParityMissing
			category, reason := UnavailableMissing, "suffix: not implemented"
			if kind == KindInfix {
				handler, category, reason = HandlerInfix, UnavailableInfix, "infix: argument editing is not implemented"
			} else if child := transientChild(tr.Name, entry.Command, names, orphans); child != "" && child != tr.Name {
				childTransient = child
				command, handler, availability, parity, category, reason = transientCommandID(child), HandlerPrefix, AvailabilityAlways, ParityPartial, UnavailableNone, ""
			} else if id, ok := implemented[tr.Name+"\x00"+entry.Command]; ok {
				command, handler, availability, parity, category, reason = id, HandlerExecute, AvailabilityAlways, ParityPartial, UnavailableNone, ""
			} else if transientCapability[tr.Name][entry.Command] {
				handler, availability, parity, category, reason = HandlerExecute, AvailabilityAlways, ParityPartial, UnavailableNone, ""
			} else if entry.Command == "magit-branch-spinoff" || entry.Command == "magit-branch-spinout" {
				category, reason = UnavailableUnsupported, "backend explicitly reports this branch workflow as unsupported"
			}
			local := canonicalSequence(entry.Key)
			seq := append(append([]string(nil), routes[tr.Name]...), local...)
			group := ""
			if len(entry.Groups) > 0 {
				group = entry.Groups[0]
			}
			conditions := make([]string, len(entry.Conditions))
			directConfigure := false
			for i, c := range entry.Conditions {
				conditions[i] = c.Type + ": " + c.Expression
				directConfigure = directConfigure || strings.Contains(c.Expression, "direct-configure")
			}
			if kind == KindInfix && (len(OptionConsumerCommands(prefix, entry.Command)) > 0 || directConfigure) {
				parity = ParityPartial
				if directConfigure {
					reason = "available in the corresponding Configure dialog"
				} else {
					reason = "availability is resolved from installed TUI consumers"
				}
			}
			b := Binding{Occurrence: fmt.Sprintf("%s:%02d", tr.Name, occurrence), Sequence: seq, LocalSequence: local, Transient: tr.Name, ChildTransient: childTransient, Display: displaySequence(seq), Command: command, Label: transientLabel(entry.Description, entry.Command), Scheme: SchemeMagit, Context: ContextTransient + prefix, Parity: parity, UpstreamCommand: entry.Command, UpstreamKey: entry.Key, Kind: kind, InfixClass: entry.Class, Domain: entry.Domain, Source: tr.Source, Handler: handler, Availability: availability, Unavailable: reason, UnavailableCategory: category, Group: group, Conditions: conditions}
			if entry.Argument != nil {
				b.Argument = *entry.Argument
			}
			b.TakesValue = kind == KindInfix && entry.Class != "transient-switch"
			out = append(out, b)
			b.Scheme = SchemeVim
			if tr.Name == "magit-status-jump" && b.Handler == HandlerExecute {
				// Vim's j is navigation, so these Magit-only descendants cannot
				// be reached in this scheme. Give them a distinct unsupported ID:
				// shared Magit handlers must not accidentally promote the rows.
				b.Command = CommandID("missing/" + strings.TrimPrefix(b.UpstreamCommand, "magit-"))
				b.Handler, b.Availability, b.Parity = HandlerUnsupported, AvailabilityNever, ParityMissing
				b.Unavailable, b.UnavailableCategory = "status jump uses Magit keys", UnavailableContext
			}
			out = append(out, b)
		}
	}
	return out
}

func transientCommandID(name string) CommandID {
	return CommandID("transient." + strings.TrimPrefix(name, "magit-"))
}

func transientChild(owner, command string, names, orphans map[string]bool) string {
	if names[command] {
		return command
	}
	best := ""
	for name := range orphans {
		if name != owner && strings.HasPrefix(command, name+"-") && len(name) > len(best) {
			best = name
		}
	}
	return best
}

func orphanTransientNames(m manifest, names map[string]bool) map[string]bool {
	incoming := map[string]bool{}
	for _, top := range m.Top {
		if top.Effective && names[top.Command] {
			incoming[top.Command] = true
		}
	}
	for _, tr := range m.Transients {
		for _, entry := range tr.Entries {
			if names[entry.Command] && entry.Command != tr.Name {
				incoming[entry.Command] = true
			}
		}
	}
	out := map[string]bool{}
	for name := range names {
		if !incoming[name] {
			out[name] = true
		}
	}
	return out
}

// transientRoutes derives one deterministic shortest route from effective
// status bindings and transient-to-transient suffixes. Runtime navigation does
// not depend on this canonical choice and therefore supports every parent and
// recursive/self edge without expanding manifest occurrences.
func transientRoutes(m manifest) map[string][]string {
	names := make(map[string]bool, len(m.Transients))
	for _, tr := range m.Transients {
		names[tr.Name] = true
	}
	orphans := orphanTransientNames(m, names)
	routes := map[string][]string{}
	queue := []string{}
	for _, top := range m.Top {
		if !top.Effective || !names[top.Command] {
			continue
		}
		seq := canonicalSequence(top.Key)
		if prior, ok := routes[top.Command]; !ok || len(seq) < len(prior) {
			routes[top.Command] = seq
			queue = append(queue, top.Command)
		}
	}
	byName := map[string][]struct{ key, child string }{}
	for _, tr := range m.Transients {
		for _, entry := range tr.Entries {
			if child := transientChild(tr.Name, entry.Command, names, orphans); child != "" && child != tr.Name {
				byName[tr.Name] = append(byName[tr.Name], struct{ key, child string }{entry.Key, child})
			}
		}
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, edge := range byName[parent] {
			candidate := append(append([]string(nil), routes[parent]...), canonicalSequence(edge.key)...)
			if prior, ok := routes[edge.child]; !ok || len(candidate) < len(prior) {
				routes[edge.child] = candidate
				queue = append(queue, edge.child)
			}
		}
	}
	for name := range names {
		if len(routes[name]) == 0 {
			panic("embedded Magit manifest: unreachable transient " + name)
		}
	}
	return routes
}

// Transients returns all exact recursively reachable manifest transients.
func Transients() []Transient {
	var m manifest
	if err := json.Unmarshal(upstreamManifest, &m); err != nil {
		panic(err)
	}
	routes := transientRoutes(m)
	out := make([]Transient, 0, len(m.Transients))
	for _, tr := range m.Transients {
		name := strings.TrimPrefix(tr.Name, "magit-")
		out = append(out, Transient{Name: tr.Name, Title: friendlyLabel("magit-" + name), Sequence: append([]string(nil), routes[tr.Name]...), Source: tr.Source})
	}
	return out
}

func BindingsForTransient(scheme Scheme, name string) []Binding {
	var out []Binding
	for _, b := range registry {
		if b.Scheme == scheme && b.Transient == name {
			out = append(out, b)
		}
	}
	return out
}

func TransientByName(name string) (Transient, bool) {
	for _, tr := range Transients() {
		if tr.Name == name {
			return tr, true
		}
	}
	return Transient{}, false
}

// transientCapability is the static half of the keymap/UI integration
// contract. UI startup validates every executable row against the handlers
// installed by workflow domains, so the registry and generated ledger never
// call a connected workflow "missing" merely because keymap cannot import ui.
var transientCapability = map[string]map[string]bool{
	"magit-branch":        setOf("magit-checkout", "magit-branch-checkout", "magit-branch-orphan", "magit-branch-and-checkout", "magit-worktree-checkout", "magit-branch-create", "magit-worktree-branch", "magit-branch-configure", "magit-branch-rename", "magit-branch-reset", "magit-branch-delete"),
	"magit-commit":        setOf("magit-commit-create", "magit-commit-extend", "magit-commit-amend", "magit-commit-reword", "magit-commit-fixup", "magit-commit-squash", "magit-commit-alter", "magit-commit-augment", "magit-commit-revise"),
	"magit-diff":          setOf("magit-diff-dwim", "magit-diff-range", "magit-diff-paths", "magit-diff-unstaged", "magit-diff-staged", "magit-diff-working-tree", "magit-show-commit", "magit-stash-show"),
	"magit-fetch":         setOf("magit-fetch-from-pushremote", "magit-fetch-from-upstream", "magit-fetch-other", "magit-fetch-all", "magit-fetch-branch", "magit-fetch-refspec", "magit-fetch-modules", "magit-branch-configure"),
	"magit-fetch-modules": setOf("magit-fetch-modules"),
	"magit-diff-refresh":  setOf("magit-diff-refresh"),
	"magit-ediff":         setOf("magit-ediff-dwim", "magit-ediff-show-unstaged", "magit-ediff-show-staged", "magit-ediff-show-working-tree", "magit-ediff-show-commit", "magit-ediff-compare", "magit-ediff-show-stash"),
	"magit-git-mergetool": setOf("magit-git-mergetool"),
	"magit-log":           setOf("magit-log-current", "magit-log-other", "magit-log-head", "magit-log-related", "magit-log-branches", "magit-log-all-branches", "magit-log-all", "magit-log-reflog", "magit-log-matching-branches", "magit-log-matching-tags", "magit-reflog-current", "magit-reflog-other", "magit-reflog-head"),
	"magit-log-refresh":   setOf("magit-log-refresh"),
	"magit-shortlog":      setOf("magit-shortlog-since", "magit-shortlog-range"),
	"magit-show-refs":     setOf("magit-show-refs-head", "magit-show-refs-current", "magit-show-refs-other"),
	"magit-patch-apply":   setOf("magit-patch-apply"),
	"magit-patch-create":  setOf("magit-patch-create"),
	"magit-push":          setOf("magit-push-current-to-pushremote", "magit-push-current-to-upstream", "magit-push-current", "magit-push-other", "magit-push-refspecs", "magit-push-matching", "magit-push-tag", "magit-push-tags", "magit-push-notes-ref", "magit-branch-configure"),
	"magit-remote":        setOf("magit-remote-add", "magit-remote-rename", "magit-remote-remove", "magit-remote-configure", "magit-remote-prune", "magit-remote-unshallow", "magit-update-default-branch"),
	"magit-tag":           setOf("magit-tag-create", "magit-tag-release"),
	"magit-notes":         setOf("magit-notes-edit", "magit-notes-remove", "magit-notes-merge", "magit-notes-prune"),
	"magit-stash":         setOf("magit-stash-both", "magit-stash-keep-index", "magit-stash-apply", "magit-stash-list", "magit-stash-show", "magit-stash-branch", "magit-stash-format-patch"),
	"magit-stash-push":    setOf("magit-stash-push"),
	"magit-status-jump":   setOf("magit-jump-to-stashes", "magit-jump-to-untracked", "magit-jump-to-unstaged", "magit-jump-to-staged", "magit-jump-to-unpulled-from-upstream", "magit-jump-to-unpushed-to-upstream"),
}

func setOf(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

// OptionConsumerCommands returns upstream suffix identities that consume an
// infix value. An empty result means the option must not be editable in the
// status transient. Direct-configure arguments live in their typed dialogs.
func OptionConsumerCommands(prefix, option string) []string {
	all := func(transient string, excluded ...string) []string {
		skip := setOf(excluded...)
		var out []string
		for command := range transientCapability[transient] {
			if !skip[command] {
				out = append(out, command)
			}
		}
		return out
	}
	switch prefix {
	case "c":
		if option == "transient:magit-commit:--verbose" {
			return nil
		}
		return all("magit-commit")
	case "f":
		return all("magit-fetch", "magit-branch-configure")
	case "P":
		return all("magit-push", "magit-branch-configure")
	case "M":
		if option == "transient:magit-remote:-f" {
			return []string{"magit-remote-add"}
		}
	case "t":
		switch option {
		case "transient:magit-tag:--force", "transient:magit-tag:--annotate", "transient:magit-tag:--sign", "magit-tag:--local-user":
			return all("magit-tag")
		}
	case "T":
		switch option {
		case "magit-notes:--ref":
			return all("magit-notes")
		case "transient:magit-notes:--dry-run":
			return []string{"magit-notes-prune"}
		case "magit-notes:--strategy":
			return []string{"magit-notes-merge"}
		}
	case "z":
		return []string{"magit-stash-both", "magit-stash-keep-index"}
	case "z P":
		return []string{"magit-stash-push"}
	}
	return nil
}

func domainCommandID(transient, command string) CommandID {
	domain := strings.TrimPrefix(transient, "magit-")
	behavior := strings.TrimPrefix(command, "magit-")
	behavior = strings.TrimPrefix(behavior, "transient:"+transient+":")
	behavior = strings.NewReplacer("<", "", ">", "", "/", ".", ":", ".").Replace(behavior)
	return CommandID(domain + "." + behavior)
}

func transientLabel(raw json.RawMessage, command string) string {
	var label string
	if len(raw) > 0 && string(raw) != "null" && json.Unmarshal(raw, &label) == nil && label != "" && !strings.HasPrefix(label, "#[") && !strings.HasPrefix(label, "magit-") {
		return label
	}
	if label, ok := map[string]string{
		"magit-fetch-from-pushremote":      "push remote",
		"magit-fetch-from-upstream":        "upstream",
		"magit-push-current-to-pushremote": "configured destination",
	}[command]; ok {
		return label
	}
	return friendlyLabel(command)
}

func canonicalSequence(value string) []string {
	parts := strings.Fields(value)
	// Transient's compact argument keys (for example -n, --, +s and =s) and
	// status-jump pairs are Emacs key sequences, not one terminal key event.
	if len(parts) == 1 && len(parts[0]) > 1 && (strings.ContainsRune("-+=", rune(parts[0][0])) || compactStatusJumpKey(parts[0])) {
		parts = strings.Split(parts[0], "")
	}
	for i, part := range parts {
		parts[i] = canonicalToken(part)
	}
	return parts
}

func compactStatusJumpKey(value string) bool {
	switch value {
	case "fp", "fu", "pp", "pu":
		return true
	default:
		return false
	}
}

func canonicalToken(token string) string {
	switch token {
	case "TAB":
		return "tab"
	case "RET":
		return "enter"
	case "SPC":
		return "space"
	case "S-SPC":
		return "shift+space"
	case "DEL":
		return "backspace"
	case "<backtab>":
		// Bubble Tea consistently reports this portable terminal sequence as
		// shift+tab rather than Emacs' symbolic <backtab> spelling.
		return "shift+tab"
	}
	if strings.HasPrefix(token, "C-") {
		return "ctrl+" + modifiedToken(strings.Trim(strings.TrimPrefix(token, "C-"), "<>"))
	}
	if strings.HasPrefix(token, "M-") {
		return "alt+" + modifiedToken(strings.Trim(strings.TrimPrefix(token, "M-"), "<>"))
	}
	return token
}

func modifiedToken(token string) string {
	switch strings.ToUpper(token) {
	case "TAB":
		return "tab"
	case "RET", "RETURN":
		return "enter"
	case "SPC", "SPACE":
		return "space"
	}
	return token
}

func displaySequence(sequence []string) string {
	parts := append([]string(nil), sequence...)
	for i, token := range parts {
		if token == "tab" {
			parts[i] = "Tab"
		}
		if token == "enter" {
			parts[i] = "Enter"
		}
	}
	return strings.Join(parts, " ")
}

func friendlyLabel(command string) string {
	label := strings.TrimPrefix(command, "magit-")
	label = strings.ReplaceAll(label, "-", " ")
	if label == "section toggle" {
		return "Toggle section"
	}
	if label == "process buffer" {
		return "Git processes"
	}
	if label == "mode bury buffer" {
		return "Close status"
	}
	if label == "dispatch" {
		return "Commands"
	}
	if label == "refresh" || label == "refresh all" {
		return "Refresh current buffer"
	}
	if label == "stage files" {
		return "Stage"
	}
	if label == "unstage files" {
		return "Unstage"
	}
	if label == "stage modified" {
		return "Stage all tracked changes"
	}
	if label == "unstage all" {
		return "Unstage all"
	}
	if label == "delete thing" {
		return "Discard"
	}
	return strings.ToUpper(label[:1]) + label[1:]
}

// ValidateRegistry enforces integration invariants used by production and tests.
func ValidateRegistry(bindings []Binding) error {
	seen := map[string]CommandID{}
	occurrences := map[string]bool{}
	for _, b := range bindings {
		if len(b.Sequence) == 0 || b.Command == CommandNone {
			return fmt.Errorf("incomplete binding for %q", b.Display)
		}
		for _, token := range b.Sequence {
			if token == "Tab" || token == "Enter" || token == "TAB" || token == "RET" || strings.Contains(token, "+TAB") || strings.Contains(token, "+RET") {
				return fmt.Errorf("noncanonical token %q", token)
			}
		}
		if b.Handler == "" {
			return fmt.Errorf("%s has no handler classification", b.Command)
		}
		if b.Availability == AvailabilityNever && (b.UnavailableCategory == UnavailableNone || b.Unavailable == "") {
			return fmt.Errorf("%s has untyped unavailability", b.Command)
		}
		if b.UpstreamCommand != "" && b.Source.File == "" {
			return fmt.Errorf("%s has no upstream source identity", b.Command)
		}
		if strings.HasPrefix(b.Context, ContextTransient) {
			if b.Occurrence == "" {
				return fmt.Errorf("%s has no occurrence identity", b.Command)
			}
			occurrenceKey := string(b.Scheme) + "\x00" + b.Occurrence
			if occurrences[occurrenceKey] {
				return fmt.Errorf("duplicate occurrence identity %s", b.Occurrence)
			}
			occurrences[occurrenceKey] = true
		}
		key := string(b.Scheme) + "\x00" + b.Context + "\x00" + strings.Join(b.Sequence, "\x00")
		if prior, ok := seen[key]; ok && b.Handler != HandlerInfix && !strings.HasPrefix(b.Context, ContextTransient) {
			return fmt.Errorf("duplicate sequence %q in %s/%s (%s, %s)", b.Display, b.Context, b.Scheme, prior, b.Command)
		}
		if b.Handler != HandlerInfix {
			seen[key] = b.Command
		}
	}
	return nil
}

func init() {
	if err := ValidateRegistry(registry); err != nil {
		panic(err)
	}
}
