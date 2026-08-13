package keymap

import (
	"reflect"
	"testing"
)

// TestBinding_ZeroValueIsInvalid pins that a zero-value Binding (as would
// appear in an uninitialized map entry) is distinguishable from every
// explicit constructor's output, so Resolve/Lookup (PR 2) can treat it as
// absent without a nil-check special case.
func TestBinding_ZeroValueIsInvalid(t *testing.T) {
	var b Binding
	if b.Kind != BindingInvalid {
		t.Errorf("zero Binding.Kind = %v, want BindingInvalid", b.Kind)
	}
}

// TestCommandBinding_HasCommandKind asserts CommandBinding produces a
// Binding carrying the given CommandID under BindingCommand.
func TestCommandBinding_HasCommandKind(t *testing.T) {
	b := CommandBinding(CommandQuit)
	if b.Kind != BindingCommand {
		t.Errorf("Kind = %v, want BindingCommand", b.Kind)
	}
	if b.Command != CommandQuit {
		t.Errorf("Command = %q, want %q", b.Command, CommandQuit)
	}
}

// TestActionBinding_HasActionKind asserts ActionBinding produces a Binding
// carrying the given Action under BindingAction.
func TestActionBinding_HasActionKind(t *testing.T) {
	action := Action{Name: "Open PR", Type: "url", URL: "https://example.com"}
	b := ActionBinding(action)
	if b.Kind != BindingAction {
		t.Errorf("Kind = %v, want BindingAction", b.Kind)
	}
	if b.Action != action {
		t.Errorf("Action = %+v, want %+v", b.Action, action)
	}
}

// TestUnboundBinding_HasUnboundKind asserts UnboundBinding produces a
// Binding representing a `~`/null unbind, distinct from BindingInvalid (the
// zero value / "never specified") so Resolve can tell "user unbound this
// key" apart from "user never mentioned this key" (PR 2's merge rule).
func TestUnboundBinding_HasUnboundKind(t *testing.T) {
	b := UnboundBinding()
	if b.Kind != BindingUnbound {
		t.Errorf("Kind = %v, want BindingUnbound", b.Kind)
	}
}

// TestBindingKind_ValuesAreDistinct guards against a copy-paste that gives
// two Kind constants the same underlying value, which would compile fine
// but silently merge two semantically different binding kinds.
func TestBindingKind_ValuesAreDistinct(t *testing.T) {
	kinds := []BindingKind{BindingInvalid, BindingCommand, BindingAction, BindingUnbound}
	seen := make(map[BindingKind]bool)
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("BindingKind value %v is reused by more than one constant", k)
		}
		seen[k] = true
	}
}

// TestBindingKind_InvalidIsZeroValue pins that BindingInvalid is the zero
// value of BindingKind (int 0), which is what makes an unset map entry
// distinguishable without an explicit "ok" check everywhere it's read.
func TestBindingKind_InvalidIsZeroValue(t *testing.T) {
	if BindingInvalid != 0 {
		t.Errorf("BindingInvalid = %d, want 0", int(BindingInvalid))
	}
}

// TestAction_FieldsAreTagCompatibleWithConfigAction pins the Q&A decision
// that keymap.Action is a standalone struct (internal/keymap must not
// import internal/config) but stays field/tag-compatible with
// config.Action so #509's conversion is a straight field copy. This test
// can't import internal/config (that would violate the one-directional
// edge), so it pins the yaml tags directly against the known config.Action
// shape (internal/config/config.go).
func TestAction_FieldsAreTagCompatibleWithConfigAction(t *testing.T) {
	a := Action{
		Name:    "Open PR",
		Type:    "url",
		URL:     "https://example.com",
		Command: "gh pr view",
		Scope:   "pr",
		Order:   3,
	}
	if a.Name != "Open PR" || a.Type != "url" || a.URL != "https://example.com" ||
		a.Command != "gh pr view" || a.Scope != "pr" || a.Order != 3 {
		t.Fatalf("Action fields did not round-trip: %+v", a)
	}

	// config.Action's known yaml tags (internal/config/config.go); this
	// test cannot import that package (the edge must stay one-directional),
	// so the tag strings are pinned here as the cross-package contract
	// #509 relies on to convert config.Action <-> keymap.Action field-for-field.
	wantTags := map[string]string{
		"Name":    "name",
		"Type":    "type",
		"URL":     "url",
		"Command": "command",
		"Scope":   "scope",
		"Order":   "-",
	}
	typ := reflect.TypeOf(Action{})
	if typ.NumField() != len(wantTags) {
		t.Fatalf("Action has %d fields, want %d (must stay field-compatible with config.Action)", typ.NumField(), len(wantTags))
	}
	for fieldName, wantTag := range wantTags {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Errorf("Action is missing field %q", fieldName)
			continue
		}
		if got := field.Tag.Get("yaml"); got != wantTag {
			t.Errorf("Action.%s yaml tag = %q, want %q", fieldName, got, wantTag)
		}
	}
}
