// Package keymap resolves key strings into UI-independent actions.
package keymap

// View identifies the active application view.
type View uint8

const (
	ViewNone View = iota
	ViewStatus
	ViewLog
	ViewBranches
)

// Section identifies the semantic status area under the cursor.
type Section uint8

const (
	SectionNone Section = iota
	SectionUnstaged
	SectionStaged
)

// Context is the state relevant to conditional key bindings.
type Context struct {
	View    View
	Section Section
}

// Action is a command understood by the application update loop.
type Action uint8

const (
	ActionNone Action = iota
	ActionMoveDown
	ActionMoveUp
	ActionFirst
	ActionLast
	ActionRefresh
	ActionStage
	ActionUnstage
	ActionDiscard
	ActionCommit
	ActionSwitchBranch
	ActionFetch
	ActionPush
)

// Result describes how a key was resolved. Prefix is populated while Pending
// so a status line can display the keys awaiting completion.
type Result struct {
	Action  Action
	Handled bool
	Pending bool
	Prefix  string
}

type prefixState uint8

const (
	prefixNone prefixState = iota
	prefixG
	prefixCommit
	prefixBranch
	prefixFetch
	prefixPush
)

// Resolver is an explicit prefix state machine. A Resolver should be retained
// by the update loop rather than recreated for every key.
type Resolver struct {
	state prefixState
	view  View
}

// NewResolver returns an idle resolver.
func NewResolver() *Resolver { return &Resolver{} }

// PendingPrefix returns text suitable for a status-line prefix indicator.
func (r *Resolver) PendingPrefix() string { return prefixText(r.state) }

// Reset cancels a pending sequence.
func (r *Resolver) Reset() {
	r.state = prefixNone
	r.view = ViewNone
}

// Feed consumes one Bubble Tea-style key string.
func (r *Resolver) Feed(context Context, key string) Result {
	if r.state != prefixNone {
		if key == "esc" {
			r.Reset()
			return Result{Handled: true}
		}
		// Prefixes belong to the view in which they started. If focus changes,
		// cancel and interpret this key normally in the new context.
		if context.View != r.view {
			r.Reset()
			return r.feedIdle(context, key)
		}
		return r.feedPending(key)
	}
	return r.feedIdle(context, key)
}

// Flush resolves an ambiguous complete prefix (currently a single g), and
// cancels command prefixes that still require another key.
func (r *Resolver) Flush(context Context) Result {
	if r.state == prefixNone {
		return Result{}
	}
	state, view := r.state, r.view
	r.Reset()
	if state == prefixG && context.View == view && view == ViewStatus {
		return actionResult(ActionRefresh)
	}
	return Result{Handled: true}
}

func (r *Resolver) feedIdle(context Context, key string) Result {
	if context.View != ViewStatus {
		return Result{}
	}
	switch key {
	case "j":
		return actionResult(ActionMoveDown)
	case "k":
		return actionResult(ActionMoveUp)
	case "G":
		return actionResult(ActionLast)
	case "s":
		if context.Section == SectionUnstaged {
			return actionResult(ActionStage)
		}
	case "u":
		if context.Section == SectionStaged {
			return actionResult(ActionUnstage)
		}
	case "x":
		if context.Section == SectionUnstaged {
			return actionResult(ActionDiscard)
		}
	case "g":
		return r.start(prefixG, context.View)
	case "c":
		return r.start(prefixCommit, context.View)
	case "b":
		return r.start(prefixBranch, context.View)
	case "f":
		return r.start(prefixFetch, context.View)
	case "P":
		return r.start(prefixPush, context.View)
	}
	return Result{}
}

func (r *Resolver) feedPending(key string) Result {
	state := r.state
	r.Reset()
	switch {
	case state == prefixG && key == "g":
		return actionResult(ActionFirst)
	case state == prefixCommit && key == "c":
		return actionResult(ActionCommit)
	case state == prefixBranch && key == "b":
		return actionResult(ActionSwitchBranch)
	case state == prefixFetch && key == "f":
		return actionResult(ActionFetch)
	case state == prefixPush && key == "p":
		return actionResult(ActionPush)
	default:
		// The unmatched key is not claimed, allowing the caller to handle it.
		return Result{}
	}
}

func (r *Resolver) start(state prefixState, view View) Result {
	r.state = state
	r.view = view
	return Result{Handled: true, Pending: true, Prefix: prefixText(state)}
}

func actionResult(action Action) Result { return Result{Action: action, Handled: true} }

func prefixText(state prefixState) string {
	switch state {
	case prefixG:
		return "g"
	case prefixCommit:
		return "c"
	case prefixBranch:
		return "b"
	case prefixFetch:
		return "f"
	case prefixPush:
		return "P"
	default:
		return ""
	}
}
