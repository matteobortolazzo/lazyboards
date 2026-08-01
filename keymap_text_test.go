package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Route close_confirm and label_confirm through the registry (#539 PR 1/2) ---
//
// handleCloseConfirmModeKey/handleLabelConfirmModeKey cut over from a
// hardcoded literal `msg.Type == tea.KeyEsc` pre-check + switch msg.String()
// to keymap.Keymap.Lookup against ModeCloseConfirm/ModeLabelConfirm,
// mirroring the git-panel/dispatch-modal cutover (#511's keymap_panels.go).
// This file targets a seam that does not exist yet:
//   - textBinding: the single-key exact-match dispatch primitive these two
//     modes (and delete, PR 2/2) share -- mirroring panelBinding
//     (keymap_panels.go).
//   - textHintKey: picks the single best display key bound to a command id
//     in a mode's entries, preferring the *shortest* key (matching
//     commandHintKeys' tie-break, NOT panelHintKey's named-preferred
//     tie-break -- today's "(y/n)" text picks the single-rune "n" over the
//     also-bound "esc", and that default-parity text must not regress).
//   - promptKeySuffix: builds the registry-derived "(y/n)"-style prompt tail
//     view.go's close/label confirm helpBar lines render, from a
//     (confirm-command-id, cancel-command-id) pair -- dropping either side
//     that has no bound key left, and returning "" (no parenthetical at
//     all) when neither side is bound, per docs/view-state-consistency.md's
//     "never advertise a key that silently no-ops" rule.
//
// This whole file (and thus the package) is expected to fail to compile
// until keymap_text.go lands (a separate, later delegation).

// findLineContaining returns the first line of view containing substr, or
// fails the test if no line matches.
func findLineContaining(t *testing.T, view, substr string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("view has no line containing %q, got:\n%s", substr, view)
	return ""
}

// --- textBinding: single-key exact-match dispatch primitive ---

func TestKeymapText_TextBinding_CloseConfirm_DefaultYNEscResolve(t *testing.T) {
	b := newTestBoard(t)

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{keyMsg("y"), keymap.CommandCloseConfirmConfirm},
		{keyMsg("n"), keymap.CommandCloseConfirmCancel},
		{arrowMsg(tea.KeyEsc), keymap.CommandCloseConfirmCancel},
	}
	for _, tc := range cases {
		binding, ok := b.textBinding(keymap.ModeCloseConfirm, tc.msg)
		if !ok {
			t.Errorf("textBinding(ModeCloseConfirm, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("textBinding(ModeCloseConfirm, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

func TestKeymapText_TextBinding_LabelConfirm_DefaultYNEscResolve(t *testing.T) {
	b := newTestBoard(t)

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{keyMsg("y"), keymap.CommandLabelConfirmCreate},
		{keyMsg("n"), keymap.CommandLabelConfirmCancel},
		{arrowMsg(tea.KeyEsc), keymap.CommandLabelConfirmCancel},
	}
	for _, tc := range cases {
		binding, ok := b.textBinding(keymap.ModeLabelConfirm, tc.msg)
		if !ok {
			t.Errorf("textBinding(ModeLabelConfirm, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("textBinding(ModeLabelConfirm, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

func TestKeymapText_TextBinding_UnrecognizedKeyNotFound(t *testing.T) {
	b := newTestBoard(t)

	if _, ok := b.textBinding(keymap.ModeCloseConfirm, keyMsg("z")); ok {
		t.Error(`textBinding(ModeCloseConfirm, "z") resolved, want not found for a genuinely unbound key`)
	}
	if _, ok := b.textBinding(keymap.ModeLabelConfirm, keyMsg("z")); ok {
		t.Error(`textBinding(ModeLabelConfirm, "z") resolved, want not found for a genuinely unbound key`)
	}
}

// --- promptKeySuffix: pure prompt-tail derivation from the registry ---

func TestKeymapText_PromptKeySuffix_BothBoundJoinsWithSlash_PrefersShortestCancelKey(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "y", Binding: keymap.CommandBinding(keymap.CommandCloseConfirmConfirm)},
		{Sequence: "n", Binding: keymap.CommandBinding(keymap.CommandCloseConfirmCancel)},
		{Sequence: "esc", Binding: keymap.CommandBinding(keymap.CommandCloseConfirmCancel)},
	}
	got := promptKeySuffix(entries, keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel)
	if got != "y/n" {
		t.Errorf("promptKeySuffix() = %q, want %q (default-parity: shortest cancel key %q preferred over %q, matching today's UI text)", got, "y/n", "n", "esc")
	}
}

func TestKeymapText_PromptKeySuffix_CancelUnbound_DropsTrailingSlash(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "y", Binding: keymap.CommandBinding(keymap.CommandCloseConfirmConfirm)},
	}
	got := promptKeySuffix(entries, keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel)
	if got != "y" {
		t.Errorf("promptKeySuffix() = %q, want %q (no trailing slash when the cancel side is unbound)", got, "y")
	}
}

func TestKeymapText_PromptKeySuffix_ConfirmUnbound_DropsLeadingSlash(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "n", Binding: keymap.CommandBinding(keymap.CommandCloseConfirmCancel)},
	}
	got := promptKeySuffix(entries, keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel)
	if got != "n" {
		t.Errorf("promptKeySuffix() = %q, want %q (no leading slash when the confirm side is unbound)", got, "n")
	}
}

func TestKeymapText_PromptKeySuffix_BothUnbound_ReturnsEmpty(t *testing.T) {
	var entries []keymap.Entry
	got := promptKeySuffix(entries, keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel)
	if got != "" {
		t.Errorf("promptKeySuffix() = %q, want empty string when neither side is bound (never advertise a key that silently no-ops)", got)
	}
}

func TestKeymapText_PromptKeySuffix_DefaultTablesMatchTodaysYNText(t *testing.T) {
	b := newTestBoard(t)

	closeEntries := b.keys.Entries(keymap.ModeCloseConfirm, "")
	if got := promptKeySuffix(closeEntries, keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel); got != "y/n" {
		t.Errorf("promptKeySuffix(close_confirm defaults) = %q, want %q", got, "y/n")
	}

	labelEntries := b.keys.Entries(keymap.ModeLabelConfirm, "")
	if got := promptKeySuffix(labelEntries, keymap.CommandLabelConfirmCreate, keymap.CommandLabelConfirmCancel); got != "y/n" {
		t.Errorf("promptKeySuffix(label_confirm defaults) = %q, want %q", got, "y/n")
	}
}

// --- 1. Default-parity dispatch: y/n behave exactly as today ---

func TestKeymapText_CloseConfirm_DefaultParity_YConfirmsNCancels(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	m, cmd := b.Update(keyMsg("y"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (closeCardCmd) from 'y'")
	}
	if b2.mode != normalMode {
		t.Errorf("mode after 'y' = %v, want normalMode", b2.mode)
	}

	m, cmd = b.Update(keyMsg("n"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after 'n' = %v, want normalMode", b3.mode)
	}
	execCmds(cmd)
}

func TestKeymapText_LabelConfirm_DefaultParity_YConfirmsNCancels(t *testing.T) {
	b := newBoardWithUnknownLabelConfirm(t)

	m, cmd := b.Update(keyMsg("y"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (createLabelCmd) from 'y'")
	}
	if b2.mode != labelConfirmMode {
		t.Errorf("mode after 'y' = %v, want labelConfirmMode unchanged (creation completes async on labelCreatedMsg)", b2.mode)
	}
	execCmds(cmd)

	b = newBoardWithUnknownLabelConfirm(t)
	m, cmd = b.Update(keyMsg("n"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after 'n' = %v, want normalMode", b3.mode)
	}
	execCmds(cmd)
}

// --- 2. esc cancels in both modes THROUGH THE REGISTRY, not a literal pre-check ---

func TestKeymapText_CloseConfirm_DefaultParity_EscCancels(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != normalMode {
		t.Errorf("mode after esc = %v, want normalMode", b2.mode)
	}
}

func TestKeymapText_LabelConfirm_DefaultParity_EscCancels(t *testing.T) {
	b := newBoardWithUnknownLabelConfirm(t)

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != normalMode {
		t.Errorf("mode after esc = %v, want normalMode", b2.mode)
	}
}

// TestKeymapText_CloseConfirm_EscUnbound_NoLongerCancels is the regression
// guard for the literal `msg.Type == tea.KeyEsc` pre-check the old handler
// used (mode_handlers.go): once dispatch goes exclusively through the
// registry, unbinding just the "esc" key entry (leaving "n" bound) must make
// Escape a genuine no-op -- a leftover literal pre-check would incorrectly
// keep cancelling regardless of this override.
func TestKeymapText_CloseConfirm_EscUnbound_NoLongerCancels(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCloseConfirm: {"esc": keymap.UnboundBinding()},
	}, nil)
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	before := b.mode
	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unbound esc = %v, want unchanged (%v) -- a literal tea.KeyEsc pre-check would incorrectly still cancel here", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound esc should not fire a cmd")
	}

	// 'n' still cancels (only esc was unbound).
	m, cmd = b2.Update(keyMsg("n"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after 'n' (only esc unbound) = %v, want normalMode", b3.mode)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (status message) from 'n' cancel")
	}
	execCmds(cmd)
}

func TestKeymapText_LabelConfirm_EscUnbound_NoLongerCancels(t *testing.T) {
	b := newBoardWithUnknownLabelConfirmCustomKeymap(t, map[keymap.Mode]keymap.Table{
		keymap.ModeLabelConfirm: {"esc": keymap.UnboundBinding()},
	})

	before := b.mode
	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unbound esc = %v, want unchanged (%v) -- a literal tea.KeyEsc pre-check would incorrectly still cancel here", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound esc should not fire a cmd")
	}

	m, cmd = b2.Update(keyMsg("n"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after 'n' (only esc unbound) = %v, want normalMode", b3.mode)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (status message) from 'n' cancel")
	}
	execCmds(cmd)
}

// --- 3. Remap: rebinding the confirm key changes dispatch AND the rendered prompt ---

func TestKeymapText_CloseConfirm_RemapConfirmKey_DispatchAndPromptStayInSync(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCloseConfirm: {
			"y": keymap.UnboundBinding(),
			"c": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	promptLine := findLineContaining(t, b.View(), "Close #")
	if strings.Contains(promptLine, "(y/n)") {
		t.Errorf("prompt line = %q, still advertises the old 'y' key after remap", promptLine)
	}
	if !strings.Contains(promptLine, "(c/n)") {
		t.Errorf("prompt line = %q, want it to advertise the remapped confirm key as (c/n)", promptLine)
	}

	// Old 'y' must now no-op.
	before := b.mode
	m, cmd := b.Update(keyMsg("y"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after old (now unbound) 'y' = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound 'y' should not fire a cmd")
	}

	// New 'c' key confirms.
	m, cmd = b.Update(keyMsg("c"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("remapped 'c' should fire closeCardCmd")
	}
	if b3.mode != normalMode {
		t.Errorf("mode after remapped 'c' = %v, want normalMode", b3.mode)
	}
}

func TestKeymapText_LabelConfirm_RemapConfirmKey_DispatchAndPromptStayInSync(t *testing.T) {
	b := newBoardWithUnknownLabelConfirmCustomKeymap(t, map[keymap.Mode]keymap.Table{
		keymap.ModeLabelConfirm: {
			"y": keymap.UnboundBinding(),
			"c": keymap.CommandBinding(keymap.CommandLabelConfirmCreate),
		},
	})

	promptLine := findLineContaining(t, b.View(), "doesn't exist")
	if strings.Contains(promptLine, "(y/n)") {
		t.Errorf("prompt line = %q, still advertises the old 'y' key after remap", promptLine)
	}
	if !strings.Contains(promptLine, "(c/n)") {
		t.Errorf("prompt line = %q, want it to advertise the remapped confirm key as (c/n)", promptLine)
	}

	before := b.mode
	m, cmd := b.Update(keyMsg("y"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after old (now unbound) 'y' = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound 'y' should not fire a cmd")
	}

	m, cmd = b.Update(keyMsg("c"))
	if _, ok := m.(Board); !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("remapped 'c' should fire createLabelCmd")
	}
	execCmds(cmd)
}

// --- 4. Unbind one side (e.g. cancel) -> prompt drops that side ---

func TestKeymapText_CloseConfirm_UnbindCancelSide_PromptDropsIt(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCloseConfirm: {
			"n":   keymap.UnboundBinding(),
			"esc": keymap.UnboundBinding(),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	promptLine := findLineContaining(t, b.View(), "Close #")
	if !strings.Contains(promptLine, "(y)") {
		t.Errorf("prompt line = %q, want the cancel side dropped, leaving only (y)", promptLine)
	}
	if strings.Contains(promptLine, "/") {
		t.Errorf("prompt line = %q, want no trailing slash when only the confirm side is bound", promptLine)
	}

	before := b.mode
	m, cmd := b.Update(keyMsg("n"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unbound 'n' = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound 'n' should not fire a cmd")
	}
}

func TestKeymapText_LabelConfirm_UnbindCancelSide_PromptDropsIt(t *testing.T) {
	b := newBoardWithUnknownLabelConfirmCustomKeymap(t, map[keymap.Mode]keymap.Table{
		keymap.ModeLabelConfirm: {
			"n":   keymap.UnboundBinding(),
			"esc": keymap.UnboundBinding(),
		},
	})

	promptLine := findLineContaining(t, b.View(), "doesn't exist")
	if !strings.Contains(promptLine, "(y)") {
		t.Errorf("prompt line = %q, want the cancel side dropped, leaving only (y)", promptLine)
	}
	if strings.Contains(promptLine, "/") {
		t.Errorf("prompt line = %q, want no trailing slash when only the confirm side is bound", promptLine)
	}

	before := b.mode
	m, cmd := b.Update(keyMsg("n"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unbound 'n' = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound 'n' should not fire a cmd")
	}
}

// --- 5. Unbind both sides -> prompt omits the whole parenthetical ---

func TestKeymapText_CloseConfirm_UnbindBothSides_PromptOmitsParenthetical(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCloseConfirm: {
			"y":   keymap.UnboundBinding(),
			"n":   keymap.UnboundBinding(),
			"esc": keymap.UnboundBinding(),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	promptLine := findLineContaining(t, b.View(), "Close #")
	if strings.ContainsAny(promptLine, "()") {
		t.Errorf("prompt line = %q, want no parenthetical at all when neither side is bound (never advertise a key that silently no-ops)", promptLine)
	}
}

func TestKeymapText_LabelConfirm_UnbindBothSides_PromptOmitsParenthetical(t *testing.T) {
	b := newBoardWithUnknownLabelConfirmCustomKeymap(t, map[keymap.Mode]keymap.Table{
		keymap.ModeLabelConfirm: {
			"y":   keymap.UnboundBinding(),
			"n":   keymap.UnboundBinding(),
			"esc": keymap.UnboundBinding(),
		},
	})

	promptLine := findLineContaining(t, b.View(), "doesn't exist")
	if strings.ContainsAny(promptLine, "()") {
		t.Errorf("prompt line = %q, want no parenthetical at all when neither side is bound (never advertise a key that silently no-ops)", promptLine)
	}
}

// --- 6. ctrl+c always quits, regardless of user keymap config ---

func TestKeymapText_CloseConfirm_CtrlCQuits_EvenWithOverriddenKeymap(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	// Attempt to steal ctrl+c for something else -- update.go's global
	// ctrl+c-always-quits check runs before any mode dispatch, so this must
	// have no effect.
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCloseConfirm: {"ctrl+c": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm)},
	}, nil)
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in closeConfirmMode should return a non-nil Cmd (tea.Quit), even with an overridden keymap")
	}
}

func TestKeymapText_LabelConfirm_CtrlCQuits_EvenWithOverriddenKeymap(t *testing.T) {
	b := newBoardWithUnknownLabelConfirmCustomKeymap(t, map[keymap.Mode]keymap.Table{
		keymap.ModeLabelConfirm: {"ctrl+c": keymap.CommandBinding(keymap.CommandLabelConfirmCreate)},
	})

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in labelConfirmMode should return a non-nil Cmd (tea.Quit), even with an overridden keymap")
	}
}

// --- 7. Unrecognized key is a no-op in both modes ---

func TestKeymapText_CloseConfirm_UnrecognizedKey_NoOp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	before := b.mode
	m, cmd := b.Update(keyMsg("z"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unrecognized 'z' = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unrecognized key should not fire a cmd")
	}
}

func TestKeymapText_LabelConfirm_UnrecognizedKey_NoOp(t *testing.T) {
	b := newBoardWithUnknownLabelConfirm(t)

	before := b.mode
	m, cmd := b.Update(keyMsg("z"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unrecognized 'z' = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unrecognized key should not fire a cmd")
	}
}
