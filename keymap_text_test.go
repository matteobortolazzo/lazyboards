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

// --- delete mode: textBinding + hint-bar derivation (#539 PR 2/2) ---
//
// delete stays one Mode (ModeDelete) shared by both steps of the two-step
// flow: delete.submit/delete.cancel resolve identically regardless of step,
// while the per-step wording ("Continue" vs "Confirm") and dispatch
// (handleDeleteModeKey, mode_handlers.go) live in package main. Unlike
// close_confirm/label_confirm, delete.ConsumesPrintableRunes() is true, so
// textBinding's printable-rune guard is load-bearing here -- proven
// structurally below, and end-to-end through the real handler in
// delete_mode_test.go.

func TestKeymapText_TextBinding_Delete_DefaultEnterEscResolve(t *testing.T) {
	b := newTestBoard(t)

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{arrowMsg(tea.KeyEnter), keymap.CommandDeleteSubmit},
		{arrowMsg(tea.KeyEsc), keymap.CommandDeleteCancel},
	}
	for _, tc := range cases {
		binding, ok := b.textBinding(keymap.ModeDelete, tc.msg)
		if !ok {
			t.Errorf("textBinding(ModeDelete, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("textBinding(ModeDelete, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

// TestKeymapText_TextBinding_Delete_PrintableRuneNeverFound proves
// textBinding's ConsumesPrintableRunes guard for delete mode specifically:
// unlike close_confirm/label_confirm (ConsumesPrintableRunes() == false), a
// printable rune -- even one that happens to be bound as a command
// elsewhere (e.g. "y"/"n" in close_confirm/label_confirm) -- must be
// reported not-found by construction here, not merely because nothing is
// bound to it in ModeDelete's own table. This is the structural half of the
// guard; TestDeleteMode_TypedCommentPassesThroughLiteralRunes_NoDispatch
// (delete_mode_test.go) proves the same guard end-to-end through the real
// handler.
func TestKeymapText_TextBinding_Delete_PrintableRuneNeverFound(t *testing.T) {
	b := newTestBoard(t)
	for _, r := range []string{"y", "n", "d", "x", "g"} {
		if _, ok := b.textBinding(keymap.ModeDelete, keyMsg(r)); ok {
			t.Errorf("textBinding(ModeDelete, %q) resolved, want not found (ConsumesPrintableRunes guard)", r)
		}
	}
}

// TestKeymapText_DeleteCommentHints_DefaultParityMatchesTodaysWording locks
// in today's UI text for the optional-comment step's status-bar hint bar
// (model.go's deleteCommentHints package var, pre-#539), now derived from
// the registry via the new b.deleteCommentHints() Board method.
func TestKeymapText_DeleteCommentHints_DefaultParityMatchesTodaysWording(t *testing.T) {
	b := newTestBoard(t)
	hints := b.deleteCommentHints()

	if got := hintDesc(t, hints, "esc"); got != "Cancel" {
		t.Errorf("deleteCommentHints() esc Desc = %q, want %q", got, "Cancel")
	}
	if got := hintDesc(t, hints, "enter"); got != "Continue" {
		t.Errorf("deleteCommentHints() enter Desc = %q, want %q", got, "Continue")
	}
}

// TestKeymapText_DeleteConfirmHints_DefaultParityMatchesTodaysWording is the
// retype-to-confirm-step sibling of the test above (deleteConfirmHints,
// pre-#539), via the new b.deleteConfirmHints() Board method.
func TestKeymapText_DeleteConfirmHints_DefaultParityMatchesTodaysWording(t *testing.T) {
	b := newTestBoard(t)
	hints := b.deleteConfirmHints()

	if got := hintDesc(t, hints, "esc"); got != "Cancel" {
		t.Errorf("deleteConfirmHints() esc Desc = %q, want %q", got, "Cancel")
	}
	if got := hintDesc(t, hints, "enter"); got != "Confirm" {
		t.Errorf("deleteConfirmHints() enter Desc = %q, want %q", got, "Confirm")
	}
}

// TestKeymapText_DeleteHints_RemapSubmitKey_BothStepsReflectNewKey covers
// item 1/2 of the #539 PR2 plan: remapping delete.submit's key must be
// reflected in BOTH steps' hint bars (the single ModeDelete table backs
// both), and the stale old key must no longer be advertised by either.
func TestKeymapText_DeleteHints_RemapSubmitKey_BothStepsReflectNewKey(t *testing.T) {
	b := newTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"enter": keymap.UnboundBinding(),
			"c":     keymap.CommandBinding(keymap.CommandDeleteSubmit),
		},
	}, nil)

	commentHints := b.deleteCommentHints()
	if got := hintDesc(t, commentHints, "c"); got != "Continue" {
		t.Errorf("deleteCommentHints() after remap: Desc for new key %q = %q, want %q", "c", got, "Continue")
	}
	for _, h := range commentHints {
		if h.Key == "enter" {
			t.Errorf("deleteCommentHints() still advertises the old 'enter' key after remap, got %+v", commentHints)
		}
	}

	confirmHints := b.deleteConfirmHints()
	if got := hintDesc(t, confirmHints, "c"); got != "Confirm" {
		t.Errorf("deleteConfirmHints() after remap: Desc for new key %q = %q, want %q", "c", got, "Confirm")
	}
	for _, h := range confirmHints {
		if h.Key == "enter" {
			t.Errorf("deleteConfirmHints() still advertises the old 'enter' key after remap, got %+v", confirmHints)
		}
	}
}

// TestKeymapText_DeleteCommentHints_UnboundSubmitKey_OmitsHintEntry and its
// sibling below lock in the "never advertise a key that silently no-ops"
// convention (docs/view-state-consistency.md, also documented at
// dispatchModalHints in keymap_panels.go): once a side has no bound key
// left, its Hint entry must be dropped from the slice entirely, not
// rendered with a blank Key.
func TestKeymapText_DeleteCommentHints_UnboundSubmitKey_OmitsHintEntry(t *testing.T) {
	b := newTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"enter": keymap.UnboundBinding()},
	}, nil)

	hints := b.deleteCommentHints()
	for _, h := range hints {
		if h.Desc == "Continue" {
			t.Errorf("deleteCommentHints() still advertises a Continue hint after unbinding enter, got %+v (never advertise a key that silently no-ops)", hints)
		}
	}
	if got := hintDesc(t, hints, "esc"); got != "Cancel" {
		t.Errorf("deleteCommentHints() esc Desc = %q, want %q (only the submit side was unbound)", got, "Cancel")
	}
}

func TestKeymapText_DeleteConfirmHints_UnboundCancelKey_OmitsHintEntry(t *testing.T) {
	b := newTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"esc": keymap.UnboundBinding()},
	}, nil)

	hints := b.deleteConfirmHints()
	for _, h := range hints {
		if h.Desc == "Cancel" {
			t.Errorf("deleteConfirmHints() still advertises a Cancel hint after unbinding esc, got %+v (never advertise a key that silently no-ops)", hints)
		}
	}
	if got := hintDesc(t, hints, "enter"); got != "Confirm" {
		t.Errorf("deleteConfirmHints() enter Desc = %q, want %q (only the cancel side was unbound)", got, "Confirm")
	}
}

// --- create/config eligibility predicates (#540 PR 1/2) ---
//
// createCommandActive/configCommandActive are the fifth eligibility
// condition handleCreateModeKey/handleConfigModeKey layer onto the
// recognized-command predicate borrowed from handleDeleteModeKey
// (mode_handlers.go): a resolved command id belonging to this mode's own
// set must ALSO be currently eligible given b.create.focus/b.config.focus
// (and, for create's assignee cycle, len(b.create.assigneeOptions) > 0), or
// it falls through to the focused textinput exactly like an unbound key
// would. The full focus x command matrix is only reachable through this
// unit-level predicate -- integration tests can't drive focus == 2 with
// zero/one assigneeOptions, since Tab only advances into that focus when
// len(assigneeOptions) > 1 (mode_handlers.go, Test Strategy table in the
// #540 plan).

func TestKeymapText_CreateCommandActive_AssigneeCycleOnlyAtFocus2(t *testing.T) {
	b := newTestBoard(t)
	b.create.assigneeOptions = []string{noneAssignee, "alice", "bob"}

	cycleIDs := []keymap.CommandID{keymap.CommandCreateAssigneePrev, keymap.CommandCreateAssigneeNext}

	b.create.focus = 2
	for _, id := range cycleIDs {
		if !b.createCommandActive(id) {
			t.Errorf("createCommandActive(%v) at focus 2 with options = false, want true", id)
		}
	}

	for _, focus := range []int{0, 1} {
		b.create.focus = focus
		for _, id := range cycleIDs {
			if b.createCommandActive(id) {
				t.Errorf("createCommandActive(%v) at focus %d = true, want false (assignee cycle is gated on focus == 2)", id, focus)
			}
		}
	}
}

func TestKeymapText_CreateCommandActive_AssigneeCycle_FalseWhenNoOptions(t *testing.T) {
	b := newTestBoard(t)
	b.create.focus = 2
	b.create.assigneeOptions = nil

	if b.createCommandActive(keymap.CommandCreateAssigneePrev) {
		t.Error("createCommandActive(assignee_prev) at focus 2 with zero assigneeOptions = true, want false")
	}
	if b.createCommandActive(keymap.CommandCreateAssigneeNext) {
		t.Error("createCommandActive(assignee_next) at focus 2 with zero assigneeOptions = true, want false")
	}
}

func TestKeymapText_CreateCommandActive_NonCycleCommandsAlwaysActive(t *testing.T) {
	b := newTestBoard(t)
	ids := []keymap.CommandID{keymap.CommandCreateSubmit, keymap.CommandCreateCancel, keymap.CommandCreateNextField}
	for _, focus := range []int{0, 1, 2} {
		b.create.focus = focus
		for _, id := range ids {
			if !b.createCommandActive(id) {
				t.Errorf("createCommandActive(%v) at focus %d = false, want true (not focus-gated)", id, focus)
			}
		}
	}
}

func TestKeymapText_ConfigCommandActive_ProviderCycleOnlyAtFocus0(t *testing.T) {
	b := newTestBoard(t)
	cycleIDs := []keymap.CommandID{keymap.CommandConfigProviderPrev, keymap.CommandConfigProviderNext}

	b.config.focus = 0
	for _, id := range cycleIDs {
		if !b.configCommandActive(id) {
			t.Errorf("configCommandActive(%v) at focus 0 = false, want true", id)
		}
	}

	b.config.focus = 1
	for _, id := range cycleIDs {
		if b.configCommandActive(id) {
			t.Errorf("configCommandActive(%v) at focus 1 = true, want false (provider cycle is gated on focus == 0)", id)
		}
	}
}

func TestKeymapText_ConfigCommandActive_NonCycleCommandsAlwaysActive(t *testing.T) {
	b := newTestBoard(t)
	ids := []keymap.CommandID{keymap.CommandConfigSave, keymap.CommandConfigCancel, keymap.CommandConfigNextField}
	for _, focus := range []int{0, 1} {
		b.config.focus = focus
		for _, id := range ids {
			if !b.configCommandActive(id) {
				t.Errorf("configCommandActive(%v) at focus %d = false, want true (not focus-gated)", id, focus)
			}
		}
	}
}

// --- glyphHintKey: arrow-preferred glyph substitution (#540 PR 2/2) ---
//
// create's Cycle hint (PR 1/2) only ever exercises glyphHintKey against a
// fixture where an id's *only* bound key is an arrow alias (left/right),
// which commandHintKeys' own alias-suppression never has to fight against.
// search's default table is the first caller to bind an arrow key (up/down)
// *and* a non-alias alias (ctrl+n/ctrl+p) to the very same command id
// simultaneously -- the "both-aliases-bound" fixture the #540 plan's Risks
// section names as glyphHintKey's named mitigation. This is a Calculation
// (State Coverage for Composite Hints, .claude/rules/testing.md): the
// preferArrow tie-break can only be fully exercised at the unit level
// against a hand-built []keymap.Entry fixture, mirroring
// TestKeymapText_PromptKeySuffix_* above.

// TestKeymapText_GlyphHintKey_PreferArrow_BothArrowAndCtrlAliasBound_PicksArrowGlyph
// reproduces searchDefaults verbatim (internal/keymap/defaults_text.go):
// search.prev_result is bound to both "up" and "ctrl+p", search.next_result
// to both "down" and "ctrl+n". Byte-identity with today's "↑/↓" literal
// (searchModeHints, pre-#540 model.go) requires glyphHintKey to prefer the
// arrow key here, not commandHintKeys' own alias-suppressed pick (which
// would surface "ctrl+p"/"ctrl+n" instead, since commandHintKeys treats
// up/down as suppressible aliases whenever a non-alias key is also bound).
func TestKeymapText_GlyphHintKey_PreferArrow_BothArrowAndCtrlAliasBound_PicksArrowGlyph(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "up", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "ctrl+p", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "down", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
		{Sequence: "ctrl+n", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
	}

	got := glyphHintKey(entries, true, keymap.CommandSearchPrevResult, keymap.CommandSearchNextResult)
	want := "↑/↓"
	if got != want {
		t.Errorf("glyphHintKey(preferArrow=true, both up/ctrl+p and down/ctrl+n bound) = %q, want %q (today's byte-identical search Navigate hint)", got, want)
	}
}

// TestKeymapText_GlyphHintKey_PreferArrowFalse_BothArrowAndCtrlAliasBound_PicksCtrlAlias
// is the preferArrow=false control case for the fixture above: without the
// arrow preference, glyphHintKey must fall back to commandHintKeys' own
// pick, which suppresses the arrow alias and surfaces the ctrl+ form
// instead -- proving preferArrow, not some other code path, is what
// recovers the arrow glyph.
func TestKeymapText_GlyphHintKey_PreferArrowFalse_BothArrowAndCtrlAliasBound_PicksCtrlAlias(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "up", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "ctrl+p", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "down", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
		{Sequence: "ctrl+n", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
	}

	got := glyphHintKey(entries, false, keymap.CommandSearchPrevResult, keymap.CommandSearchNextResult)
	want := "ctrl+p/ctrl+n"
	if got != want {
		t.Errorf("glyphHintKey(preferArrow=false, both up/ctrl+p and down/ctrl+n bound) = %q, want %q (no arrow preference: falls back to commandHintKeys' alias-suppressed pick)", got, want)
	}
}

// TestKeymapText_GlyphHintKey_RemappedOffArrows_FallsBackToRawKeyText is
// glyphHintKey's remap sibling: once neither id has an arrow key bound at
// all (both unbound from up/down, rebound onto ctrl+k/ctrl+j), preferArrow
// has nothing to prefer and the result must be the raw key text, not a
// stale/misleading glyph -- mirroring arrowGlyphs' documented fallback
// (keymap_dispatch.go) and create's
// TestCreateModalHints_RemapAssigneeCycleKeys_ReflectsNewKeysNoGlyph.
func TestKeymapText_GlyphHintKey_RemappedOffArrows_FallsBackToRawKeyText(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "ctrl+k", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "ctrl+j", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
	}

	got := glyphHintKey(entries, true, keymap.CommandSearchPrevResult, keymap.CommandSearchNextResult)
	want := "ctrl+k/ctrl+j"
	if got != want {
		t.Errorf("glyphHintKey(preferArrow=true, no arrow key bound to either id) = %q, want %q (raw key text, no glyph)", got, want)
	}
}

// TestKeymapText_GlyphHintKey_OnlyArrowAliasBound_PicksArrowGlyph is
// glyphHintKey's simplest preferArrow case -- an id whose *only* bound key
// is the arrow alias (create's assignee_prev/assignee_next default
// left/right, with nothing else bound) -- guarding the branch
// createModalHints already exercises indirectly via
// TestCreateModalHints_CycleHintPresentOnlyAtFocus2, at the unit level.
func TestKeymapText_GlyphHintKey_OnlyArrowAliasBound_PicksArrowGlyph(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "left", Binding: keymap.CommandBinding(keymap.CommandCreateAssigneePrev)},
		{Sequence: "right", Binding: keymap.CommandBinding(keymap.CommandCreateAssigneeNext)},
	}

	got := glyphHintKey(entries, true, keymap.CommandCreateAssigneePrev, keymap.CommandCreateAssigneeNext)
	want := "◀/▶"
	if got != want {
		t.Errorf("glyphHintKey(preferArrow=true, only left/right bound) = %q, want %q", got, want)
	}
}

// TestKeymapText_GlyphHintKey_UnboundID_OmittedFromJoin guards the
// no-key-bound skip: an id with nothing bound in entries must be dropped
// from the joined result entirely, not render as an empty segment (e.g.
// "◀/" or "/▶") -- matching textHintKey's documented "" skip.
func TestKeymapText_GlyphHintKey_UnboundID_OmittedFromJoin(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "up", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
	}

	got := glyphHintKey(entries, true, keymap.CommandSearchPrevResult, keymap.CommandSearchNextResult)
	want := "↑"
	if got != want {
		t.Errorf("glyphHintKey with search.next_result unbound = %q, want %q (unbound id dropped, not an empty segment)", got, want)
	}
}
