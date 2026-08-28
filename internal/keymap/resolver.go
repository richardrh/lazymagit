package keymap

import "strings"

type Result struct {
	Command CommandID
	Binding *Binding
	Handled bool
	Pending bool
	Prefix  string
	Reason  string
}

type trieNode struct {
	children map[string]*trieNode
	binding  *Binding
}

// Resolver is a generic sequence trie. It has no command-specific prefix state
// and therefore supports arbitrary-length and nested canonical token sequences.
type Resolver struct {
	root, node    *trieNode
	context       Context
	tokens        []string
	completed     *Binding
	transient     string
	transientBase int
}

func NewResolver() *Resolver              { return &Resolver{} }
func (r *Resolver) PendingPrefix() string { return strings.Join(r.tokens, " ") }
func (r *Resolver) Reset() {
	r.root, r.node, r.tokens, r.completed, r.transient, r.transientBase = nil, nil, nil, nil, "", 0
}
func (r *Resolver) ActiveTransient() string { return r.transient }
func (r *Resolver) PendingSuffix() string {
	if r.transientBase >= len(r.tokens) {
		return ""
	}
	return strings.Join(r.tokens[r.transientBase:], " ")
}

// ContinueTransient discards a partially consumed suffix while retaining the
// active transient invocation. UIs use this after handling an infix through
// their typed option editor so the next suffix starts at the transient root.
func (r *Resolver) ContinueTransient() bool {
	if r.transient == "" || r.transientBase > len(r.tokens) {
		return false
	}
	r.tokens = r.tokens[:r.transientBase]
	r.buildTransient(r.context)
	return true
}

func (r *Resolver) Feed(context Context, key string) Result {
	if len(r.tokens) > 0 {
		if key == "esc" {
			r.Reset()
			return Result{Handled: true}
		}
		if context.View != r.context.View || context.Scheme != r.context.Scheme {
			r.Reset()
			return r.Feed(context, key)
		}
		return r.advance(context, key)
	}
	r.build(context)
	return r.advance(context, key)
}

func (r *Resolver) Flush(context Context) Result {
	if len(r.tokens) == 0 {
		return Result{}
	}
	b := r.completed
	r.Reset()
	if b == nil {
		return Result{Handled: true}
	}
	return resultFor(*b, context)
}

func (r *Resolver) advance(context Context, key string) Result {
	next := r.node.children[key]
	if next == nil {
		r.Reset()
		return Result{}
	}
	r.node = next
	r.tokens = append(r.tokens, key)
	r.completed = next.binding
	if len(next.children) > 0 {
		return Result{Handled: true, Pending: true, Prefix: r.PendingPrefix(), Binding: next.binding}
	}
	b := next.binding
	if b == nil {
		r.Reset()
		return Result{}
	}
	if b.Handler == HandlerPrefix {
		r.transient = b.ChildTransient
		if r.transient == "" {
			r.transient = b.UpstreamCommand
		}
		r.transientBase = len(r.tokens)
		r.buildTransient(context)
		return Result{Handled: true, Pending: true, Prefix: r.PendingPrefix(), Binding: b}
	}
	r.Reset()
	return resultFor(*b, context)
}

func resultFor(b Binding, context Context) Result {
	ok, reason := b.Available(context)
	result := Result{Handled: true, Binding: &b, Reason: reason}
	if b.Handler == HandlerPrefix {
		result.Pending = true
		result.Prefix = strings.Join(b.Sequence, " ")
		return result
	}
	if b.Handler == HandlerExecute && ok {
		result.Command = b.Command
	}
	return result
}

func (r *Resolver) build(context Context) {
	r.root = &trieNode{children: map[string]*trieNode{}}
	r.node = r.root
	r.context = context
	for i := range registry {
		b := &registry[i]
		if b.Scheme != context.Scheme || b.Context != ContextStatus {
			continue
		}
		n := r.root
		for _, token := range b.Sequence {
			if n.children[token] == nil {
				n.children[token] = &trieNode{children: map[string]*trieNode{}}
			}
			n = n.children[token]
		}
		n.binding = b
	}
}

func (r *Resolver) buildTransient(context Context) {
	r.root = &trieNode{children: map[string]*trieNode{}}
	r.node = r.root
	r.completed = nil
	for i := range registry {
		b := &registry[i]
		if b.Scheme != context.Scheme || b.Transient != r.transient {
			continue
		}
		n := r.root
		for _, token := range b.LocalSequence {
			if n.children[token] == nil {
				n.children[token] = &trieNode{children: map[string]*trieNode{}}
			}
			n = n.children[token]
		}
		// Duplicate local sequences can pair an infix with a conditional suffix
		// (for example notes' c). Prefer the executable suffix so an available
		// action is not shadowed by an infix with no command; the UI still chooses
		// the active occurrence when rendering conditions.
		if n.binding == nil || n.binding.Handler == HandlerInfix && b.Handler != HandlerInfix {
			n.binding = b
		}
	}
}

// Find returns a canonical registry binding for menu/footer integration.
func Find(scheme Scheme, context string, sequence ...string) (Binding, bool) {
	var infix *Binding
	for _, b := range registry {
		if b.Scheme == scheme && b.Context == context && strings.Join(b.Sequence, "\x00") == strings.Join(sequence, "\x00") {
			if b.Handler != HandlerInfix {
				return b, true
			}
			copy := b
			infix = &copy
		}
	}
	if infix != nil {
		return *infix, true
	}
	return Binding{}, false
}

func BindingsFor(scheme Scheme, context string) []Binding {
	var out []Binding
	for _, b := range registry {
		if b.Scheme == scheme && b.Context == context {
			out = append(out, b)
		}
	}
	return out
}

func PrimaryBindings(scheme Scheme) []Binding {
	order := []CommandID{"transient.commit", "transient.fetch", "transient.push", "transient.branch"}
	var out []Binding
	for _, command := range order {
		for _, b := range registry {
			if b.Scheme == scheme && b.Context == ContextStatus && b.Command == command {
				out = append(out, b)
				break
			}
		}
	}
	return out
}
