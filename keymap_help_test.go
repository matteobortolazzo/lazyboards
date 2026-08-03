package main

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Help modal generation (#550, phase A: per-mode sections) ---
//
// buildHelpContent's sections move from the hardcoded helpSections table
// (view.go) to a generator (keymap_help.go, not yet created) that derives
// every row from the active *keymap.Keymap: helpModeSections (curated
// title/mode/openedBy table), helpBuiltinRows (catalogue-order walk,
// identical-desc merge, contiguous single-rune run collapse), and
// helpKeysForCommand (help-only all-keys collector, glyph-substituted, no
// alias suppression -- Q1). These tests are RED against today's production
// code: none of the four symbols above exist yet.
//
// Key-ordering assumption pinned by these tests (derived from the plan's Q1
// examples "h/◀/esc", "l/▶", "↓/ctrl+n/↑/ctrl+p"): helpKeysForCommand sorts
// a command's glyph-substituted keys by (rune-length ascending, then string
// ascending) -- the same tie-break commandHintKeys already uses, just
// applied post-substitution and without the arrow-alias suppression step.
// A merged identical-desc row concatenates each constituent command's own
// helpKeysForCommand result in keymap.Commands() declaration order.

// helpSectionBody extracts the body of the section named title from a
// buildHelpContent() rendering: the text between the "\n<title>\n" header
// line and the next blank line, mirroring the idiom help_test.go's
// per-section tests already use inline. Fails the test if the section is
// missing.
func helpSectionBody(t *testing.T, content, title string) string {
	t.Helper()
	idx := strings.Index(content, "\n"+title+"\n")
	if idx == -1 {
		t.Fatalf("buildHelpContent() missing section header %q, got:\n%s", title, content)
	}
	body := content[idx:]
	if next := strings.Index(body[1:], "\n\n"); next != -1 {
		body = body[:next+1]
	}
	return body
}

// mustFindCommand looks up id in the catalogue, failing the test if it
// isn't registered -- so every expected label in these tests is computed
// from the real catalogue rather than copied as a literal (.claude/rules/
// testing.md).
func mustFindCommand(t *testing.T, id keymap.CommandID) keymap.Command {
	t.Helper()
	cmd, ok := keymap.FindCommand(id)
	if !ok {
		t.Fatalf("keymap.FindCommand(%q) returned ok=false", id)
	}
	return cmd
}

// expectedRangeLabel computes the longest common prefix (trimmed) of ids'
// catalogue descs -- the same programmatic computation AC2 requires
// helpBuiltinRows' 1-9 collapse label to use, so these tests never hardcode
// the collapsed label as a literal.
func expectedRangeLabel(t *testing.T, ids ...keymap.CommandID) string {
	t.Helper()
	descs := make([]string, len(ids))
	for i, id := range ids {
		descs[i] = mustFindCommand(t, id).Desc
	}
	prefix := descs[0]
	for _, d := range descs[1:] {
		for !strings.HasPrefix(d, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return strings.TrimSpace(prefix)
}

// columnJumpCommandIDs is the nav.column_1..9 command group, in catalogue
// declaration order.
var columnJumpCommandIDs = []keymap.CommandID{
	keymap.CommandNavColumn1, keymap.CommandNavColumn2, keymap.CommandNavColumn3,
	keymap.CommandNavColumn4, keymap.CommandNavColumn5, keymap.CommandNavColumn6,
	keymap.CommandNavColumn7, keymap.CommandNavColumn8, keymap.CommandNavColumn9,
}

// --- AC1: remap/unbind truthfulness ---

func TestKeymapHelp_RemapCardNew_ShowsNewKeyAndDropsOldKey(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"n":  keymap.UnboundBinding(),
			"f1": keymap.CommandBinding(keymap.CommandCardNew),
		},
	}, nil)

	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Normal Mode")
	desc := mustFindCommand(t, keymap.CommandCardNew).Desc

	newKeyRow := regexp.MustCompile(`(?m)^  f1\s+` + regexp.QuoteMeta(desc) + `$`)
	if !newKeyRow.MatchString(section) {
		t.Errorf("Normal Mode section missing remapped row (key %q, desc %q) after remapping card.new onto f1, got:\n%s", "f1", desc, section)
	}
	oldKeyRow := regexp.MustCompile(`(?m)^  n\s+` + regexp.QuoteMeta(desc) + `$`)
	if oldKeyRow.MatchString(section) {
		t.Errorf("Normal Mode section still shows the old key %q for %q after it was unbound, got:\n%s", "n", desc, section)
	}
}

func TestKeymapHelp_UnbindCardDelete_NoRowCarriesItsDesc(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"d": keymap.UnboundBinding()},
	}, nil)

	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Normal Mode")
	desc := mustFindCommand(t, keymap.CommandCardDelete).Desc

	if strings.Contains(section, desc) {
		t.Errorf("Normal Mode section still contains %q after unbinding card.delete's only key (d, #502 remap: was t), got:\n%s", desc, section)
	}
}

// --- AC3: helpModeSections drift guard over keymap.Modes() ---

func TestHelpModeSections_OneEntryPerModeNoDuplicatesNoUnknownModes(t *testing.T) {
	seen := make(map[keymap.Mode]int)
	for _, spec := range helpModeSections {
		if spec.title == "" {
			t.Errorf("helpModeSections entry for mode %q has an empty title", spec.mode)
		}
		if !spec.mode.Resolvable() {
			t.Errorf("helpModeSections entry %q names mode %q, which is not in keymap.Modes()", spec.title, spec.mode)
		}
		seen[spec.mode]++
	}
	for mode, count := range seen {
		if count > 1 {
			t.Errorf("helpModeSections has %d entries for mode %q, want exactly 1", count, mode)
		}
	}
	for _, mode := range keymap.Modes() {
		if seen[mode] == 0 {
			t.Errorf("keymap.Modes() includes %q but helpModeSections has no entry for it -- a mode added to Modes() without a display section", mode)
		}
	}
}

func TestHelpModeSections_OpenedByAlwaysResolvesViaFindCommand(t *testing.T) {
	for _, spec := range helpModeSections {
		if spec.openedBy == "" {
			continue
		}
		if _, ok := keymap.FindCommand(spec.openedBy); !ok {
			t.Errorf("helpModeSections entry %q (mode %q) names openedBy command %q, which keymap.FindCommand cannot resolve", spec.title, spec.mode, spec.openedBy)
		}
	}
}

// --- AC3: the new Help section ---

func TestKeymapHelp_HelpSection_RendersCoreKeys(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Help")

	// Row keys are rendered as "  %-12s %s\n" (view.go's original format,
	// preserved verbatim by buildHelpContent), so a key at the start of its
	// row is preceded by the row's own two-space indent ("^  "), not a bare
	// "^" -- matching the same "^  <key>" convention the sibling
	// TestKeymapHelp_RemapCardNew_.../TestKeymapHelp_NormalSection_
	// ColumnJumpsCollapseToRange row-anchored regexes already use elsewhere
	// in this file. A key later in a "/"-joined merged row is still matched
	// via the "/" alternative.
	for _, key := range []string{"esc", "?", "j", "k"} {
		re := regexp.MustCompile(`(?m)(^  |/)` + regexp.QuoteMeta(key) + `(/|\s)`)
		if !re.MatchString(section) {
			t.Errorf("Help section missing a row for key %q, got:\n%s", key, section)
		}
	}
}

// --- AC2: catalogue-desc equivalence (never a hardcoded copy) ---

func TestHelpBuiltinRows_LabelEqualsCatalogueDesc(t *testing.T) {
	cases := []struct {
		id  keymap.CommandID
		key string
	}{
		{keymap.CommandCardNew, "n"},
		{keymap.CommandCardEdit, "e"},
		{keymap.CommandCardDelete, "d"},
		{keymap.CommandBoardRefresh, "r"},
	}
	for _, tc := range cases {
		entries := []keymap.Entry{{Sequence: tc.key, Binding: keymap.CommandBinding(tc.id)}}
		rows := helpBuiltinRows(entries)
		if len(rows) != 1 {
			t.Fatalf("helpBuiltinRows() for %q = %d rows, want 1: %v", tc.id, len(rows), rows)
		}
		want := mustFindCommand(t, tc.id).Desc
		if rows[0][1] != want {
			t.Errorf("row desc for %q = %q, want catalogue desc %q (never a hardcoded copy)", tc.id, rows[0][1], want)
		}
	}
}

// --- Q1: all-keys labels, no alias suppression ---

func TestHelpKeysForCommand_SearchNextResult_ReturnsAllKeysNoAliasSuppression(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "down", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
		{Sequence: "ctrl+n", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
	}

	// Regression guard: commandHintKeys (the hint-bar collector) suppresses
	// the arrow-alias "down" whenever a non-alias key is also bound to the
	// same command. Q1 explicitly forbids reusing it for help -- if a future
	// edit made helpKeysForCommand delegate to it, this would catch the
	// alias silently disappearing from the help row.
	if suppressed := commandHintKeys(entries, keymap.CommandSearchNextResult); len(suppressed) != 1 || suppressed[0] != "ctrl+n" {
		t.Fatalf("commandHintKeys() = %v, want exactly [\"ctrl+n\"] (documents the alias-suppression help must NOT inherit)", suppressed)
	}

	got := helpKeysForCommand(entries, keymap.CommandSearchNextResult)
	want := []string{"↓", "ctrl+n"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("helpKeysForCommand() = %v, want %v (arrow glyph substituted, alias kept)", got, want)
	}
}

func TestHelpKeysForCommand_SearchPrevResult_ReturnsAllKeys(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "up", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "ctrl+p", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
	}
	got := helpKeysForCommand(entries, keymap.CommandSearchPrevResult)
	want := []string{"↑", "ctrl+p"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("helpKeysForCommand() = %v, want %v", got, want)
	}
}

func TestHelpKeysForCommand_DetailBlur_ReturnsAllThreeKeys(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "h", Binding: keymap.CommandBinding(keymap.CommandDetailBlur)},
		{Sequence: "left", Binding: keymap.CommandBinding(keymap.CommandDetailBlur)},
		{Sequence: "esc", Binding: keymap.CommandBinding(keymap.CommandDetailBlur)},
	}
	got := helpKeysForCommand(entries, keymap.CommandDetailBlur)
	want := []string{"h", "◀", "esc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("helpKeysForCommand() = %v, want %v", got, want)
	}
}

func TestHelpBuiltinRows_SearchNavigateRow_MergesBothCommandsFullKeySet(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "down", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
		{Sequence: "ctrl+n", Binding: keymap.CommandBinding(keymap.CommandSearchNextResult)},
		{Sequence: "up", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
		{Sequence: "ctrl+p", Binding: keymap.CommandBinding(keymap.CommandSearchPrevResult)},
	}
	nextDesc := mustFindCommand(t, keymap.CommandSearchNextResult).Desc
	prevDesc := mustFindCommand(t, keymap.CommandSearchPrevResult).Desc
	if nextDesc != prevDesc {
		t.Fatalf("precondition: CommandSearchNextResult/PrevResult descs differ (%q vs %q); the merge-on-identical-desc assumption doesn't hold", nextDesc, prevDesc)
	}

	rows := helpBuiltinRows(entries)
	if len(rows) != 1 {
		t.Fatalf("helpBuiltinRows() = %d rows, want 1 merged Navigate row: %v", len(rows), rows)
	}
	if rows[0][0] != "↓/ctrl+n/↑/ctrl+p" {
		t.Errorf("merged row key = %q, want %q", rows[0][0], "↓/ctrl+n/↑/ctrl+p")
	}
	if rows[0][1] != nextDesc {
		t.Errorf("merged row desc = %q, want catalogue desc %q", rows[0][1], nextDesc)
	}
}

func TestKeymapHelp_SearchSection_NavigateRowShowsFullKeySet(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Search")

	if !strings.Contains(section, "↓/ctrl+n/↑/ctrl+p") {
		t.Errorf("Search section missing full-key-set Navigate row %q, got:\n%s", "↓/ctrl+n/↑/ctrl+p", section)
	}
}

func TestKeymapHelp_DetailSection_BackRowShowsFullKeySet(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Detail Panel")

	if !strings.Contains(section, "h/◀/esc") {
		t.Errorf("Detail Panel section missing full-key-set back row %q, got:\n%s", "h/◀/esc", section)
	}
}

func TestKeymapHelp_NormalSection_DetailFocusRowShowsFullKeySet(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Normal Mode")

	if !strings.Contains(section, "l/▶") {
		t.Errorf("Normal Mode section missing full-key-set detail-focus row %q, got:\n%s", "l/▶", section)
	}
}

// --- Q2/AC2: the 1-9 collapse and its non-contiguous fallback ---

func TestHelpBuiltinRows_CollapsesContiguousColumnJumpsToRange(t *testing.T) {
	var entries []keymap.Entry
	for i, id := range columnJumpCommandIDs {
		entries = append(entries, keymap.Entry{Sequence: fmt.Sprintf("%d", i+1), Binding: keymap.CommandBinding(id)})
	}

	rows := helpBuiltinRows(entries)
	if len(rows) != 1 {
		t.Fatalf("helpBuiltinRows() = %d rows for a full contiguous 1-9 run, want 1 collapsed row; got %v", len(rows), rows)
	}
	if rows[0][0] != "1-9" {
		t.Errorf("collapsed row key = %q, want %q", rows[0][0], "1-9")
	}
	wantDesc := expectedRangeLabel(t, columnJumpCommandIDs...)
	if rows[0][1] != wantDesc {
		t.Errorf("collapsed row desc = %q, want %q (longest common prefix of the 9 catalogue descs, computed programmatically)", rows[0][1], wantDesc)
	}
}

func TestHelpBuiltinRows_NonContiguousColumnJumps_FallsBackToSlashJoinedKeys(t *testing.T) {
	// Simulates a remap that moves nav.column_5 off "5" onto "0", breaking
	// the ascending 1-9 rune run while every command stays bound.
	keys := []string{"1", "2", "3", "4", "0", "6", "7", "8", "9"}
	var entries []keymap.Entry
	for i, id := range columnJumpCommandIDs {
		entries = append(entries, keymap.Entry{Sequence: keys[i], Binding: keymap.CommandBinding(id)})
	}

	rows := helpBuiltinRows(entries)
	for _, row := range rows {
		if row[0] == "1-9" {
			t.Fatalf("helpBuiltinRows() still collapsed to a 1-9 range after a remap broke contiguity: %v", rows)
		}
	}

	wantKey := strings.Join(keys, "/")
	wantDesc := expectedRangeLabel(t, columnJumpCommandIDs...)
	var found bool
	for _, row := range rows {
		if row[0] == wantKey {
			found = true
			if row[1] != wantDesc {
				t.Errorf("fallback row desc = %q, want %q", row[1], wantDesc)
			}
		}
	}
	if !found {
		t.Errorf("helpBuiltinRows() = %v, want a /-joined fallback row %q after contiguity breaks", rows, wantKey)
	}
}

// TestTryNumericRun_NoCommonPrefixDesc_FallsBackToPerCommandRows guards the
// silent-failure case flagged in #550 review: a numeric-suffix command
// family (structurally detected by commandIDNumericSuffix, independent of
// wording) is currently always nav.column_1..9, whose catalogue Desc
// strings happen to share a common prefix. If a future family's Desc
// strings share none, commonPrefixLabel converges to "" and the row must
// not collapse into a range with a blank label. This synthesizes such a
// family (not in the real catalogue, since internal/keymap and its desc
// strings are off-limits here) and asserts tryNumericRun declines the
// collapse, then mirrors
// TestHelpBuiltinRows_NonContiguousColumnJumps_FallsBackToSlashJoinedKeys's
// shape by checking the resulting rows are the existing per-command
// fallback (one row per command, verbatim Desc) rather than a new shape.
func TestTryNumericRun_NoCommonPrefixDesc_FallsBackToPerCommandRows(t *testing.T) {
	present := []keymap.Command{
		{ID: "test.family_1", Desc: "Alpha option"},
		{ID: "test.family_2", Desc: "Beta option"},
		{ID: "test.family_3", Desc: "Gamma option"},
	}
	entries := make([]keymap.Entry, len(present))
	for i, cmd := range present {
		entries[i] = keymap.Entry{Sequence: fmt.Sprintf("%d", i+1), Binding: keymap.CommandBinding(cmd.ID)}
	}

	if next, desc, keyLabel, ok := tryNumericRun(present, 0, entries); ok {
		t.Fatalf("tryNumericRun() = (next=%d, desc=%q, keyLabel=%q, ok=true) for a family with no common Desc prefix, want ok=false (no collapse)", next, desc, keyLabel)
	}

	rows := builtinRowsFromCommands(present, entries)
	for _, row := range rows {
		if strings.TrimSpace(row[1]) == "" {
			t.Fatalf("builtinRowsFromCommands() produced a row with a blank label: %v", rows)
		}
	}
	for i, cmd := range present {
		wantKey := entries[i].Sequence
		var found bool
		for _, row := range rows {
			if row[0] == wantKey && row[1] == cmd.Desc {
				found = true
			}
		}
		if !found {
			t.Errorf("builtinRowsFromCommands() = %v, want a per-command fallback row (key %q, desc %q)", rows, wantKey, cmd.Desc)
		}
	}
}

func TestKeymapHelp_NormalSection_ColumnJumpsCollapseToRange(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Normal Mode")

	wantDesc := expectedRangeLabel(t, columnJumpCommandIDs...)
	re := regexp.MustCompile(`(?m)^  1-9\s+` + regexp.QuoteMeta(wantDesc) + `$`)
	if !re.MatchString(section) {
		t.Errorf("Normal Mode section missing collapsed column-jump row (key %q, desc %q), got:\n%s", "1-9", wantDesc, section)
	}
}

// --- Identical-desc merge (general mechanism, e.g. j/k Navigate cards) ---

func TestHelpBuiltinRows_MergesAdjacentIdenticalDescRows(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "j", Binding: keymap.CommandBinding(keymap.CommandNavCursorDown)},
		{Sequence: "down", Binding: keymap.CommandBinding(keymap.CommandNavCursorDown)},
		{Sequence: "k", Binding: keymap.CommandBinding(keymap.CommandNavCursorUp)},
		{Sequence: "up", Binding: keymap.CommandBinding(keymap.CommandNavCursorUp)},
	}

	downDesc := mustFindCommand(t, keymap.CommandNavCursorDown).Desc
	upDesc := mustFindCommand(t, keymap.CommandNavCursorUp).Desc
	if downDesc != upDesc {
		t.Fatalf("precondition: CommandNavCursorDown/Up descs differ (%q vs %q); the merge-on-identical-desc assumption doesn't hold", downDesc, upDesc)
	}

	rows := helpBuiltinRows(entries)
	if len(rows) != 1 {
		t.Fatalf("helpBuiltinRows() = %d rows for cursor up/down, want 1 merged row (identical desc %q): %v", len(rows), downDesc, rows)
	}
	if rows[0][1] != downDesc {
		t.Errorf("merged row desc = %q, want %q", rows[0][1], downDesc)
	}
	if rows[0][0] != "j/↓/k/↑" {
		t.Errorf("merged row key = %q, want %q", rows[0][0], "j/↓/k/↑")
	}
}

// --- Empty-key / unbound / uncatalogued skip ---

func TestHelpBuiltinRows_SkipsUnboundAndActionEntries(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "n", Binding: keymap.CommandBinding(keymap.CommandCardNew)},
		{Sequence: "x", Binding: keymap.UnboundBinding()},
		{Sequence: "z", Binding: keymap.ActionBinding(keymap.Action{Name: "Zap", Type: "shell"})},
	}

	rows := helpBuiltinRows(entries)
	if len(rows) != 1 {
		t.Fatalf("helpBuiltinRows() = %d rows, want exactly 1 (unbound and inline-action entries must be skipped -- actions render separately): %v", len(rows), rows)
	}
	wantDesc := mustFindCommand(t, keymap.CommandCardNew).Desc
	if rows[0][0] != "n" || rows[0][1] != wantDesc {
		t.Errorf("rows[0] = %v, want {%q, %q}", rows[0], "n", wantDesc)
	}
}

// --- Q3: openedBy rows derived from the normal-mode table ---

func TestKeymapHelp_GitMenuSection_ShowsOpenedByRow(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Git Menu")

	wantDesc := mustFindCommand(t, keymap.CommandViewGitPanel).Desc
	re := regexp.MustCompile(`(?m)^  G\s+` + regexp.QuoteMeta(wantDesc) + `$`)
	if !re.MatchString(section) {
		t.Errorf("Git Menu section missing openedBy row (key %q from the normal-mode table, desc %q), got:\n%s", "G", wantDesc, section)
	}
}

// TestKeymapHelp_GitMenuSection_OpenedByRowDisappearsWhenNormalCommandUnbound
// covers the plan's named Risk: "openedBy row disappears when its
// normal-mode command is unbound" -- a hint must never advertise a key that
// silently no-ops (docs/view-state-consistency.md). #502 remapped
// view.git_panel's normal-mode key from "g" to "G".
func TestKeymapHelp_GitMenuSection_OpenedByRowDisappearsWhenNormalCommandUnbound(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"G": keymap.UnboundBinding()},
	}, nil)

	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Git Menu")

	wantDesc := mustFindCommand(t, keymap.CommandViewGitPanel).Desc
	if strings.Contains(section, wantDesc) {
		t.Errorf("Git Menu section still shows the openedBy row %q after unbinding view.git_panel's normal-mode key, got:\n%s", wantDesc, section)
	}
}

// --- AC4 (inline-action rendering only, no column diffing yet): git panel ---
//
// Extends TestBuildHelpContent_ListsGitMenuKeys (git_actions_test.go:83),
// which only asserts the action names are present, with the "(shell)" type
// annotation every inline action row must carry.

func TestKeymapHelp_GitMenuSection_ListsDefaultActionsWithShellType(t *testing.T) {
	b, _ := gitDefaultsBoard(t, nil)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Git Menu")

	for _, name := range []string{"Push", "Pull (rebase)", "Fetch", "Mergetool", "Stash push", "Stash pop"} {
		want := name + " (shell)"
		if !strings.Contains(section, want) {
			t.Errorf("Git Menu section missing action row %q, got:\n%s", want, section)
		}
	}
}

// --- Empty-section omission ---

// TestKeymapHelp_LabelConfirmSection_OmittedWhenAllBindingsUnbound covers
// the plan's named Risk: "empty section omitted when every binding in a
// mode is unbound".
func TestKeymapHelp_LabelConfirmSection_OmittedWhenAllBindingsUnbound(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeLabelConfirm: {
			"y":   keymap.UnboundBinding(),
			"n":   keymap.UnboundBinding(),
			"esc": keymap.UnboundBinding(),
		},
	}, nil)

	content := b.buildHelpContent()
	if strings.Contains(content, "\nLabel Confirm\n") {
		t.Errorf("expected the Label Confirm section to be omitted once every one of its bindings is unbound, got:\n%s", content)
	}
}

// --- Help modal generation (#550, phase B: registry-derived Custom Actions) ---
//
// Q4: the Custom Actions section becomes column-only -- a config with only
// global actions:/keymaps.normal inline actions renders NO "Custom Actions"
// header (those rows now live in the Normal/Detail sections, already proven
// by phase A's helpActionRows composition); every CONFIGURED column
// (b.columnConfigs) with column-specific bindings that differ from the
// global table is listed under Custom Actions, grouped by column, including
// the currently active column (whose overrides also render inline in
// Normal/Detail). helpColumnActionRows (not yet implemented) is the
// generator these tests pin.

// --- Column diff: identical-to-global exclusion / override inclusion ---

// Both tests below configure "Refined" -- a real column in the loaded
// board's fixture (expectedColumnTitles), but NOT the active column
// (ActiveTab defaults to 0, "New") -- deliberately, so a false pass can't
// hide behind Normal Mode's own active-column inline rendering (already
// covered by phase A): only the dedicated Custom Actions column-diff
// generator can make these assertions true.

func TestHelpColumnActionRows_ColumnEntryIdenticalToGlobal_Excluded(t *testing.T) {
	b, _ := newColumnActionTestBoard(t, nil, []config.ColumnConfig{{Name: "Refined"}})
	if got := b.activeColumnTitle(); got == "Refined" {
		t.Fatalf("precondition: %q must not be the active column for this test to isolate the Custom Actions section", "Refined")
	}
	shared := keymap.ActionBinding(keymap.Action{Name: "Shared Zap", Type: "shell", Command: "echo shared"})
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"Z": shared},
	}, map[string]keymap.Table{
		"refined": {"Z": shared},
	})

	content := b.buildHelpContent()
	if strings.Contains(content, "Custom Actions") {
		t.Errorf("expected no Custom Actions header when the column's only action is byte-identical to the global one bound to the same key, got:\n%s", content)
	}
}

func TestHelpColumnActionRows_ColumnOverrideOfGlobalKey_Included(t *testing.T) {
	b, _ := newColumnActionTestBoard(t, nil, []config.ColumnConfig{{Name: "Refined"}})
	if got := b.activeColumnTitle(); got == "Refined" {
		t.Fatalf("precondition: %q must not be the active column for this test to isolate the Custom Actions section", "Refined")
	}
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"Z": keymap.ActionBinding(keymap.Action{Name: "Global Zap", Type: "shell", Command: "echo global"})},
	}, map[string]keymap.Table{
		"refined": {"Z": keymap.ActionBinding(keymap.Action{Name: "Column Zap", Type: "shell", Command: "echo column"})},
	})

	content := b.buildHelpContent()
	if !strings.Contains(content, "Custom Actions") {
		t.Fatalf("expected a Custom Actions header once a configured column overrides a global key with a different action, got:\n%s", content)
	}
	if !strings.Contains(content, "Column Zap") {
		t.Errorf("expected the column's override action name %q under Custom Actions, got:\n%s", "Column Zap", content)
	}
}

// --- Active column: override appears both inline and under Custom Actions ---

func TestHelpColumnActionRows_ActiveColumnOverride_AlsoAppearsInlineInNormalMode(t *testing.T) {
	b, _ := newColumnActionTestBoard(t, nil, []config.ColumnConfig{{Name: "New"}})
	if got := b.activeColumnTitle(); got != "New" {
		t.Fatalf("precondition: active column should be %q, got %q", "New", got)
	}
	b = boardWithOverrideKeymap(t, b, nil, map[string]keymap.Table{
		"new": {"Z": keymap.ActionBinding(keymap.Action{Name: "Column Zap", Type: "shell", Command: "echo column"})},
	})

	content := b.buildHelpContent()
	normalSection := helpSectionBody(t, content, "Normal Mode")
	if !strings.Contains(normalSection, "Column Zap") {
		t.Errorf("the active column's own override should also appear inline in Normal Mode, got:\n%s", normalSection)
	}
	if !strings.Contains(content, "Custom Actions") || !strings.Contains(content, "Column Zap") {
		t.Errorf("the active column should still be listed under Custom Actions too (Q4: every configured column is listed, including the active one), got:\n%s", content)
	}
}

// --- Ordering ---

func TestHelpColumnActionRows_OrderedByActionOrder(t *testing.T) {
	b, _ := newColumnActionTestBoard(t, nil, []config.ColumnConfig{{Name: "Refined"}})
	if got := b.activeColumnTitle(); got == "Refined" {
		t.Fatalf("precondition: %q must not be the active column for this test to isolate the Custom Actions section", "Refined")
	}
	b = boardWithOverrideKeymap(t, b, nil, map[string]keymap.Table{
		"refined": {
			"Z": keymap.ActionBinding(keymap.Action{Name: "Zebra", Type: "shell", Order: 2}),
			"A": keymap.ActionBinding(keymap.Action{Name: "Alpha", Type: "shell", Order: 1}),
		},
	})

	content := b.buildHelpContent()
	aIdx := strings.Index(content, "Alpha")
	zIdx := strings.Index(content, "Zebra")
	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("expected both column actions present under Custom Actions, got:\n%s", content)
	}
	if aIdx >= zIdx {
		t.Errorf("column actions should be ordered by Action.Order (Alpha=1 before Zebra=2), got indices Alpha=%d Zebra=%d", aIdx, zIdx)
	}
}

// --- Sanitization ---

func TestHelpActionRows_SanitizesUntrustedNameAndType(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"Z": keymap.ActionBinding(keymap.Action{Name: "Evil\nRow", Type: "sh\nell"})},
	}, nil)

	content := b.buildHelpContent()
	if strings.Contains(content, "Evil\nRow") {
		t.Errorf("buildHelpContent() should sanitize a newline out of an untrusted action Name before rendering, got:\n%q", content)
	}
	if !strings.Contains(content, "Evil Row (sh ell)") {
		t.Errorf("buildHelpContent() should render the sanitized, single-line action Name/Type (sanitizeSingleLine collapses embedded newlines to spaces), got:\n%s", content)
	}
}

func TestHelpColumnActionRows_SanitizesUntrustedNameAndType(t *testing.T) {
	b, _ := newColumnActionTestBoard(t, nil, []config.ColumnConfig{{Name: "Refined"}})
	if got := b.activeColumnTitle(); got == "Refined" {
		t.Fatalf("precondition: %q must not be the active column for this test to isolate the Custom Actions section", "Refined")
	}
	b = boardWithOverrideKeymap(t, b, nil, map[string]keymap.Table{
		"refined": {"Z": keymap.ActionBinding(keymap.Action{Name: "Evil\nColumn", Type: "shell"})},
	})

	content := b.buildHelpContent()
	if strings.Contains(content, "Evil\nColumn") {
		t.Errorf("buildHelpContent() should sanitize a newline out of an untrusted column action Name before rendering, got:\n%q", content)
	}
	if !strings.Contains(content, "Evil Column (shell)") {
		t.Errorf("buildHelpContent() should render the sanitized, single-line column action Name/Type, got:\n%s", content)
	}
}

// --- The keymaps:-only config path (AC4) ---
//
// newConfigLoadedActionTestBoard only threads cfg.Actions/cfg.Columns into
// NewBoard's legacy params, so a keymaps:-only YAML (no legacy actions:/
// columns[].actions: block at all) would resolve to the built-in defaults
// only and could never prove AC4's "a keymaps:-only config's custom actions
// appear". newKeymapConfigLoadedTestBoard (helpers_test.go) mirrors main.go's
// real wiring (config.Load -> NewBoard -> config.ResolveKeymap -> withKeymap)
// instead.

func TestHelpContent_KeymapsOnlyConfig_GlobalActionAppearsInNormalMode(t *testing.T) {
	b := newKeymapConfigLoadedTestBoard(t, `keymaps:
  normal:
    Z:
      name: Zap
      type: shell
      command: "echo zap"
`)

	content := b.buildHelpContent()
	normalSection := helpSectionBody(t, content, "Normal Mode")
	if !strings.Contains(normalSection, "Zap (shell)") {
		t.Errorf("a keymaps:-only config's global inline action should render inline in Normal Mode, got:\n%s", normalSection)
	}
	if strings.Contains(content, "Custom Actions") {
		t.Errorf("a keymaps:-only config with no column-specific overrides should render no Custom Actions header (Q4), got:\n%s", content)
	}
}

func TestHelpContent_KeymapsOnlyConfig_ColumnOverrideAppearsUnderCustomActions(t *testing.T) {
	// "Refined" (not "New", the board's active column by default) isolates
	// this assertion to the Custom Actions section -- an active-column
	// override would also render inline in Normal Mode (already covered by
	// TestHelpColumnActionRows_ActiveColumnOverride_AlsoAppearsInlineInNormalMode).
	b := newKeymapConfigLoadedTestBoard(t, `columns:
  - name: Refined
keymaps:
  columns:
    Refined:
      Z:
        name: Column Zap
        type: shell
        command: "echo column"
`)
	if got := b.activeColumnTitle(); got == "Refined" {
		t.Fatalf("precondition: %q must not be the active column for this test to isolate the Custom Actions section", "Refined")
	}

	content := b.buildHelpContent()
	if !strings.Contains(content, "Custom Actions") {
		t.Fatalf("expected a Custom Actions header for a native keymaps.columns override, got:\n%s", content)
	}
	if !strings.Contains(content, "Column Zap (shell)") {
		t.Errorf("expected the native keymaps.columns override action to appear under Custom Actions, got:\n%s", content)
	}
}

// --- No stale A-Z custom-action namespace rows (#484) ---
//
// Before the registry, custom actions were confined to the uppercase A-Z
// namespace and the help modal advertised that convention with two curated
// static rows. #484 deleted the case split: any key of either case binds
// either a built-in or a custom action, and the shipped defaults claim
// uppercase keys (view.pr_list, view.agent_list, view.dispatch,
// view.git_panel). Advertising an "A-Z" custom-action space is now false --
// and the actually-configured actions already render as their own rows via
// helpActionRows. The alt+shift+key comment overload is a real implicit
// overload with no CommandID behind it, so it stays.

func TestHelpContent_NoUppercaseCustomActionNamespaceRows(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	if strings.Contains(content, "A-Z") {
		t.Errorf("buildHelpContent() must not advertise the deleted A-Z custom-action namespace, got:\n%s", content)
	}
}

// The uppercase keys the redesigned defaults claim must render in Normal
// Mode as their built-in command's catalogue Desc -- the concrete reason
// the "A-Z is fully yours" rows above are wrong.
func TestHelpContent_UppercaseDefaultsRenderAsBuiltins(t *testing.T) {
	b := newLoadedTestBoard(t)
	body := helpSectionBody(t, b.buildHelpContent(), "Normal Mode")

	for _, id := range []keymap.CommandID{
		keymap.CommandViewPRList,
		keymap.CommandViewAgentList,
		keymap.CommandViewDispatch,
		keymap.CommandViewGitPanel,
	} {
		cmd := mustFindCommand(t, id)
		if !strings.Contains(body, cmd.Desc) {
			t.Errorf("Normal Mode section missing built-in %q (desc %q), got:\n%s", id, cmd.Desc, body)
		}
	}
}
