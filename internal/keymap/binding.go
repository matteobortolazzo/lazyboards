package keymap

// Action is a keymap-owned, inline action a key can be bound to. It stays
// field- and yaml-tag-compatible with config.Action
// (internal/config/config.go) so #509 can convert between the two with a
// straight field copy, but internal/keymap must not import internal/config
// (that would invert the config -> keymap edge #509/#510 need). Keep this
// struct's fields and tags in sync with config.Action by hand; #509 pins
// the cross-package contract with a conversion test (it can import both
// packages -- this one cannot).
type Action struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	URL     string `yaml:"url"`
	Command string `yaml:"command"`
	Scope   string `yaml:"scope"`
	// Order is derived metadata, never read from or written to YAML; see
	// config.Action.Order for why.
	Order int `yaml:"-"`
}

// BindingKind identifies what a Binding's right-hand side is.
type BindingKind int

const (
	// BindingInvalid is the zero value of BindingKind, so a zero-value
	// Binding (an uninitialized map entry) is distinguishable from every
	// explicit constructor's output without a separate "ok" check.
	BindingInvalid BindingKind = iota
	// BindingCommand marks a Binding whose Command field is the resolved
	// built-in command id.
	BindingCommand
	// BindingAction marks a Binding whose Action field is the resolved
	// inline action.
	BindingAction
	// BindingUnbound marks a key explicitly unbound via `~`/null in user
	// config, distinct from BindingInvalid ("never specified") so Resolve's
	// merge can tell the two apart (see keymap.go, PR 2).
	BindingUnbound
)

// Binding is the resolved right-hand side of a key or key sequence: either
// a built-in command, an inline action, or an explicit unbind.
type Binding struct {
	Kind    BindingKind
	Command CommandID
	Action  Action
}

// CommandBinding returns a Binding resolving to the given built-in command.
func CommandBinding(id CommandID) Binding {
	return Binding{Kind: BindingCommand, Command: id}
}

// ActionBinding returns a Binding resolving to the given inline action.
func ActionBinding(a Action) Binding {
	return Binding{Kind: BindingAction, Action: a}
}

// UnboundBinding returns a Binding representing an explicit `~`/null
// unbind.
func UnboundBinding() Binding {
	return Binding{Kind: BindingUnbound}
}
