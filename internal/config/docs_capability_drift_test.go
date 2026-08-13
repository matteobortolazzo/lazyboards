package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #586: machine-checkable drift guard between README.md/docs/keymaps.md
// and the shipped keymap.Modes()/capability predicates/ResolveKeymap
// pipeline ---
//
// This is the RED-phase file: none of the HTML-comment fences these tests
// extract exist yet in README.md or docs/keymaps.md, so every test below
// fails at the extractFencedBlock call with a "missing fence" message, not a
// compile error or an unrelated panic. Phase 4 adds the fences (and fixes
// the doc content they wrap) to turn these green.
//
// Fences this file requires, once added:
//
//   README.md:
//     - "keymap-bindable-modes"   wraps the "Bindable modes: `a`, `b`, ..."
//       sentence (around line 206) -- a flat, backtick-quoted mode-name list.
//     - "keymap-schema-example"   wraps the existing ```yaml keymaps: ...```
//       config-schema block (around lines 193-202).
//
//   docs/keymaps.md:
//     - "keymap-capability-matrix"          wraps a markdown table, one row
//       per keymap.Modes() entry (never the "columns" row -- see below),
//       columns in this exact order: Mode | Key sequences | Inline actions |
//       Bare printable-rune key | Command ids. Mode cells are backtick-
//       quoted (e.g. `` `normal` ``); every other cell's text must START
//       with one of "yes"/"no"/"rejected"/"allowed"/"n/a" (a trailing
//       parenthetical/footnote is fine, the token prefix is what's checked).
//     - "keymap-capability-matrix-columns"  wraps the same table shape, but
//       exactly one row: the "columns" overlay row, kept in its own fence
//       precisely because ModeColumns is not itself a resolvable mode
//       (Mode.Resolvable() == false) and so does not belong in the main
//       per-Modes() table.
//     - "keymap-schema-example"             wraps the existing ```yaml
//       keymaps: ...``` config-schema block (around lines 93-107) -- must be
//       a config that actually loads (the placeholder "detail:\n    ...\n"
//       shape currently in that block does not parse as a keymap table).
//     - "keymap-schema-pr-list-scope-omitted" wraps a small ```yaml
//       keymaps: pr_list: ...``` snippet whose action entry omits `scope:`
//       entirely, demonstrating that pr_list's scope requirement is never
//       inferred, always stated: config.Load must REJECT this snippet with
//       the pr_list capability error, since inferScope (config.go) only
//       ever infers "card" or "board" from a template's placeholders, never
//       "pr" -- see TestDocsCapability_KeymapsDocPRListScopeOmittedExampleLoadsCleanly's
//       own doc comment below for why this diverges from its RED-phase name.
//
// Known coverage gap: README.md's legacy top-level `actions:`/
// `columns[].actions:` examples (around :252-261, :269-286, and :327-336)
// are NOT extracted or parse-checked by any test in this file -- only the
// two `keymaps:` schema examples above ("keymap-schema-example",
// README.md and docs/keymaps.md) are. The legacy examples are hand-verified
// only. A future edit to one of those legacy snippets is not covered by
// this drift test and must not be assumed to be.

const (
	readmeBindableModesMarker     = "keymap-bindable-modes"
	keymapSchemaExampleMarker     = "keymap-schema-example"
	keymapCapabilityMatrixMarker  = "keymap-capability-matrix"
	keymapCapabilityColumnsMarker = "keymap-capability-matrix-columns"
	keymapSchemaPRListMarker      = "keymap-schema-pr-list-scope-omitted"
)

// capabilityMatrixColumns names the five capability columns, in the fixed
// order every capability-matrix row must use, for error messages.
var capabilityMatrixColumns = []string{"Key sequences", "Inline actions", "Bare printable-rune key", "Command ids", "Column overlay"}

// validCapabilityTokens is the fixed set of tokens a capability cell's text
// must start with (point 4 of the ticket's table-parsing-robustness
// requirement).
var validCapabilityTokens = []string{"yes", "no", "rejected", "allowed", "n/a"}

// --- shared fenced-block extraction ---

// extractFencedBlock reads relPath (relative to the repo root, e.g.
// "README.md" or "docs/keymaps.md") relative to this package
// (internal/config) and returns the content between the HTML comment fences
// `<!-- marker:start -->` / `<!-- marker:end -->`. If that content itself
// contains a ```yaml / ``` code fence, only the code fence's body is
// returned (trimmed); otherwise the raw fenced content is returned trimmed
// as-is (e.g. a markdown table or a plain list). It fails the test with a
// clear message -- naming the file and the missing/misordered/empty fence --
// if extraction cannot succeed, so an accidental doc edit can't silently
// pass with an empty or wrong snippet.
func extractFencedBlock(t *testing.T, relPath, marker string) string {
	t.Helper()

	startFence := "<!-- " + marker + ":start -->"
	endFence := "<!-- " + marker + ":end -->"

	fullPath := filepath.Join("..", "..", relPath)
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", fullPath, err)
	}
	content := string(raw)

	startIdx := strings.Index(content, startFence)
	if startIdx == -1 {
		t.Fatalf("%s is missing the %q fence", relPath, startFence)
	}
	endIdx := strings.Index(content, endFence)
	if endIdx == -1 {
		t.Fatalf("%s is missing the %q fence", relPath, endFence)
	}
	if endIdx < startIdx {
		t.Fatalf("%s has %q before %q -- the %q fences are out of order", relPath, endFence, startFence, marker)
	}

	between := content[startIdx+len(startFence) : endIdx]

	if fenceOpenIdx := strings.Index(between, "```yaml"); fenceOpenIdx != -1 {
		yamlStart := fenceOpenIdx + len("```yaml")
		fenceCloseIdx := strings.Index(between[yamlStart:], "```")
		if fenceCloseIdx == -1 {
			t.Fatalf("%s: %q block is missing its closing ``` code fence", relPath, marker)
		}
		body := strings.TrimSpace(between[yamlStart : yamlStart+fenceCloseIdx])
		if body == "" {
			t.Fatalf("%s: %q block's ```yaml fence is empty", relPath, marker)
		}
		return body
	}

	body := strings.TrimSpace(between)
	if body == "" {
		t.Fatalf("%s: %q block is empty", relPath, marker)
	}
	return body
}

// --- (a) README's bindable-mode list vs. keymap.Modes() ---

var backtickModeTokenPattern = regexp.MustCompile("`([a-z_]+)`")

// TestDocsCapability_ReadmeBindableModesMatchesKeymapModes asserts the
// README's fenced bindable-mode list is exactly keymap.Modes(), naming both
// any mode present in code but missing from the doc and any mode named in
// the doc that is not a real keymap.Modes() entry.
func TestDocsCapability_ReadmeBindableModesMatchesKeymapModes(t *testing.T) {
	content := extractFencedBlock(t, "README.md", readmeBindableModesMarker)

	matches := backtickModeTokenPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		t.Fatalf("README.md's %q block contains no backtick-quoted mode names: %q", readmeBindableModesMarker, content)
	}
	docModes := make(map[string]bool, len(matches))
	for _, m := range matches {
		docModes[m[1]] = true
	}

	codeModes := make(map[string]bool)
	for _, m := range keymap.Modes() {
		codeModes[string(m)] = true
	}

	for _, mode := range sortedKeys(codeModes) {
		if !docModes[mode] {
			t.Errorf("README.md's %q list is missing mode %q, which is a keymap.Modes() entry", readmeBindableModesMarker, mode)
		}
	}
	for _, mode := range sortedKeys(docModes) {
		if !codeModes[mode] {
			t.Errorf("README.md's %q list names %q, which is not a keymap.Modes() entry", readmeBindableModesMarker, mode)
		}
	}
}

// --- (b) docs/keymaps.md capability matrix rows vs. keymap.Modes() + columns ---

// capabilityMatrixRow is one parsed data row: mode is the backtick-stripped
// first cell; cells holds the remaining capabilityMatrixColumns-ordered
// cells verbatim (untrimmed of their leading token, just whitespace-trimmed).
type capabilityMatrixRow struct {
	mode  string
	cells []string
}

// parseCapabilityMatrixRows parses a markdown table (header row + separator
// row + data rows) into its data rows, requiring exactly
// 1+len(capabilityMatrixColumns) cells per data row (mode + 5 capability
// columns). A row with the wrong cell count is reported via t.Errorf naming
// the offending row text and cell count, and excluded from the returned
// slice.
func parseCapabilityMatrixRows(t *testing.T, content string) []capabilityMatrixRow {
	t.Helper()

	wantCells := 1 + len(capabilityMatrixColumns)
	var rows []capabilityMatrixRow
	dataLineIdx := 0
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || !strings.HasPrefix(line, "|") {
			continue
		}
		if isMarkdownTableSeparatorLine(line) {
			continue
		}
		dataLineIdx++
		if dataLineIdx == 1 {
			// Header row -- skip.
			continue
		}
		cells := splitMarkdownTableRow(line)
		if len(cells) != wantCells {
			t.Errorf("capability matrix row %q has %d columns, want %d (mode + %v)", line, len(cells), wantCells, capabilityMatrixColumns)
			continue
		}
		rows = append(rows, capabilityMatrixRow{
			mode:  strings.Trim(cells[0], "`"),
			cells: cells[1:],
		})
	}
	return rows
}

// isMarkdownTableSeparatorLine reports whether line is a markdown table's
// header/body separator row (e.g. "|---|---|---|---|---|"): once stripped of
// leading/trailing "|", every rune is '-', ':', ' ', or '|', and it contains
// at least one '-'.
func isMarkdownTableSeparatorLine(line string) bool {
	inner := strings.Trim(line, "|")
	if !strings.ContainsRune(inner, '-') {
		return false
	}
	for _, r := range inner {
		if r != '-' && r != ':' && r != ' ' && r != '|' {
			return false
		}
	}
	return true
}

// splitMarkdownTableRow splits one "| a | b | c |"-shaped line into its
// trimmed cell texts.
func splitMarkdownTableRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// capabilityCellToken asserts cell starts with one of validCapabilityTokens
// and returns the matched token, or reports an error (naming mode, column,
// and the offending cell text) and returns "" if it starts with none of
// them.
func capabilityCellToken(t *testing.T, mode, column, cell string) string {
	t.Helper()
	for _, tok := range validCapabilityTokens {
		if strings.HasPrefix(cell, tok) {
			return tok
		}
	}
	t.Errorf("capability matrix cell for mode %q column %q = %q, want it to start with one of %v", mode, column, cell, validCapabilityTokens)
	return ""
}

// TestDocsCapability_KeymapsMatrixRowsMatchModes asserts the main capability
// matrix's row set is exactly keymap.Modes() (no duplicates, nothing
// missing, nothing foreign, and never the "columns" row, which must live in
// its own fence), and that the separately-fenced columns block has exactly
// one row naming keymap.ModeColumns.
func TestDocsCapability_KeymapsMatrixRowsMatchModes(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityMatrixMarker)
	rows := parseCapabilityMatrixRows(t, content)

	seen := make(map[string]int, len(rows))
	for _, row := range rows {
		seen[row.mode]++
	}
	for _, mode := range sortedKeys(seen) {
		if seen[mode] > 1 {
			t.Errorf("docs/keymaps.md capability matrix has %d rows for mode %q, want exactly 1", seen[mode], mode)
		}
	}
	for _, m := range keymap.Modes() {
		if seen[string(m)] == 0 {
			t.Errorf("docs/keymaps.md capability matrix is missing a row for mode %q", m)
		}
	}
	codeModes := make(map[string]bool, len(keymap.Modes()))
	for _, m := range keymap.Modes() {
		codeModes[string(m)] = true
	}
	for _, mode := range sortedKeys(seen) {
		if mode == string(keymap.ModeColumns) {
			t.Errorf("docs/keymaps.md capability matrix's main table includes the %q row -- it must be fenced separately under %q", mode, keymapCapabilityColumnsMarker)
			continue
		}
		if !codeModes[mode] {
			t.Errorf("docs/keymaps.md capability matrix names mode %q, which is not a keymap.Modes() entry", mode)
		}
	}

	columnsContent := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityColumnsMarker)
	columnsRows := parseCapabilityMatrixRows(t, columnsContent)
	if len(columnsRows) != 1 {
		t.Fatalf("docs/keymaps.md's %q block has %d rows, want exactly 1 (the columns overlay row)", keymapCapabilityColumnsMarker, len(columnsRows))
	}
	if columnsRows[0].mode != string(keymap.ModeColumns) {
		t.Errorf("docs/keymaps.md's %q block's row names mode %q, want %q", keymapCapabilityColumnsMarker, columnsRows[0].mode, keymap.ModeColumns)
	}
}

// --- (c) per-row capability cells vs. Mode predicates ---

// TestDocsCapability_KeymapsMatrixKeySequencesCellMatchesPredicate asserts
// every row's "Key sequences" cell agrees with Mode.DispatchesKeySequences().
func TestDocsCapability_KeymapsMatrixKeySequencesCellMatchesPredicate(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityMatrixMarker)
	rows := parseCapabilityMatrixRows(t, content)
	if len(rows) == 0 {
		t.Fatalf("parsed zero capability matrix rows -- table may be empty or malformed")
	}
	for _, row := range rows {
		mode, err := keymap.ParseMode(row.mode)
		if err != nil {
			t.Errorf("capability matrix row names unparseable mode %q: %v", row.mode, err)
			continue
		}
		want := "no"
		if mode.DispatchesKeySequences() {
			want = "yes"
		}
		got := capabilityCellToken(t, row.mode, "Key sequences", row.cells[0])
		if got != "" && got != want {
			t.Errorf("mode %q's %q cell = %q, want %q (DispatchesKeySequences() = %v)", row.mode, "Key sequences", got, want, mode.DispatchesKeySequences())
		}
	}
}

// TestDocsCapability_KeymapsMatrixInlineActionsCellMatchesPredicate asserts
// every row's "Inline actions" cell agrees with Mode.DispatchesInlineActions().
func TestDocsCapability_KeymapsMatrixInlineActionsCellMatchesPredicate(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityMatrixMarker)
	rows := parseCapabilityMatrixRows(t, content)
	if len(rows) == 0 {
		t.Fatalf("parsed zero capability matrix rows -- table may be empty or malformed")
	}
	for _, row := range rows {
		mode, err := keymap.ParseMode(row.mode)
		if err != nil {
			t.Errorf("capability matrix row names unparseable mode %q: %v", row.mode, err)
			continue
		}
		want := "no"
		if mode.DispatchesInlineActions() {
			want = "yes"
		}
		got := capabilityCellToken(t, row.mode, "Inline actions", row.cells[1])
		if got != "" && got != want {
			t.Errorf("mode %q's %q cell = %q, want %q (DispatchesInlineActions() = %v)", row.mode, "Inline actions", got, want, mode.DispatchesInlineActions())
		}
	}
}

// TestDocsCapability_KeymapsMatrixPrintableRuneCellMatchesInversePredicate
// asserts every row's "Bare printable-rune key" cell agrees with the INVERSE
// of Mode.ConsumesPrintableRunes(). This gets its own named test case
// (rather than folding into the two tests above) because the polarity is
// backwards from every other column: ConsumesPrintableRunes() == true means
// the mode's textinput swallows a bare printable rune before any keymap
// lookup ever sees it, so such a key binding is REJECTED, not allowed;
// ConsumesPrintableRunes() == false means a bare printable rune key is
// ALLOWED to resolve as a normal binding. A doc author copy-pasting the same
// "yes/no maps straight to true/false" pattern used by the other two
// columns would get this column backwards -- exactly the drift this test
// exists to catch.
func TestDocsCapability_KeymapsMatrixPrintableRuneCellMatchesInversePredicate(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityMatrixMarker)
	rows := parseCapabilityMatrixRows(t, content)
	if len(rows) == 0 {
		t.Fatalf("parsed zero capability matrix rows -- table may be empty or malformed")
	}
	for _, row := range rows {
		mode, err := keymap.ParseMode(row.mode)
		if err != nil {
			t.Errorf("capability matrix row names unparseable mode %q: %v", row.mode, err)
			continue
		}
		// INVERSE polarity: ConsumesPrintableRunes() == true -> "rejected";
		// ConsumesPrintableRunes() == false -> "allowed".
		want := "allowed"
		if mode.ConsumesPrintableRunes() {
			want = "rejected"
		}
		got := capabilityCellToken(t, row.mode, "Bare printable-rune key", row.cells[2])
		if got != "" && got != want {
			t.Errorf("mode %q's %q cell = %q, want %q (ConsumesPrintableRunes() = %v, and this column is the INVERSE of that predicate)", row.mode, "Bare printable-rune key", got, want, mode.ConsumesPrintableRunes())
		}
	}
}

// TestDocsCapability_KeymapsMatrixCommandIdsCellPinsUniversalQuit asserts
// every row's "Command ids" cell claims "yes", and pins the code-level truth
// that claim rests on: app.quit dispatches in every mode (including the
// "columns" overlay row) and every mode has at least one dispatchable
// command id.
func TestDocsCapability_KeymapsMatrixCommandIdsCellPinsUniversalQuit(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityMatrixMarker)
	rows := parseCapabilityMatrixRows(t, content)
	columnsContent := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityColumnsMarker)
	rows = append(rows, parseCapabilityMatrixRows(t, columnsContent)...)
	if len(rows) == 0 {
		t.Fatalf("parsed zero capability matrix rows -- table may be empty or malformed")
	}

	for _, row := range rows {
		got := capabilityCellToken(t, row.mode, "Command ids", row.cells[3])
		if got != "" && got != "yes" {
			t.Errorf("mode %q's %q cell = %q, want \"yes\"", row.mode, "Command ids", got)
		}

		mode := keymap.Mode(row.mode)
		if !keymap.CommandDispatchable(mode, keymap.CommandQuit) {
			t.Errorf("keymap.CommandDispatchable(%q, keymap.CommandQuit) = false, want true -- the %q column's universal-command claim requires app.quit to dispatch in every mode", mode, "Command ids")
		}
		if n := len(keymap.DispatchableCommands(mode)); n == 0 {
			t.Errorf("len(keymap.DispatchableCommands(%q)) = 0, want > 0", mode)
		}
	}
}

// TestDocsCapability_KeymapsMatrixColumnOverlayCellMatchesBehavioralProbe
// asserts every main-table row's "Column overlay" cell agrees with a real
// keymap.Resolve + Keymap.Lookup probe, mirroring the technique
// TestDocsCapability_ColumnsRowBehavioralProbe already uses for the
// "columns" pseudo-mode row: a single-key inline action bound ONLY in a
// column overlay table (absent from the mode's own table, and from the
// mode's own defaults, both of which are empty here) must dispatch through
// Lookup(mode, "<col>", key) if and only if the row's cell claims "yes". Per
// effectiveTable (keymap.go:227-236), a column overlay is precomputed for
// ModeNormal/ModeDetail only, so those are the only two rows that should
// ever claim "yes" here -- every other mode's dispatch seam can never reach
// a keymaps.columns.<name> entry, no matter what its own table contains.
func TestDocsCapability_KeymapsMatrixColumnOverlayCellMatchesBehavioralProbe(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityMatrixMarker)
	rows := parseCapabilityMatrixRows(t, content)
	if len(rows) == 0 {
		t.Fatalf("parsed zero capability matrix rows -- table may be empty or malformed")
	}

	probeAction := keymap.Action{Name: "Column overlay probe", Type: "shell", Command: "echo hi", Scope: "board"}

	for _, row := range rows {
		mode, err := keymap.ParseMode(row.mode)
		if err != nil {
			t.Errorf("capability matrix row names unparseable mode %q: %v", row.mode, err)
			continue
		}

		want := "no"
		if mode == keymap.ModeNormal || mode == keymap.ModeDetail {
			want = "yes"
		}
		got := capabilityCellToken(t, row.mode, "Column overlay", row.cells[4])
		if got != "" && got != want {
			t.Errorf("mode %q's %q cell = %q, want %q", row.mode, "Column overlay", got, want)
		}

		user := keymap.Tables{
			Modes: map[keymap.Mode]keymap.Table{},
			Columns: map[string]keymap.Table{
				"probe-column": {"z": keymap.ActionBinding(probeAction)},
			},
		}
		km, err := keymap.Resolve(keymap.Tables{}, user)
		if err != nil {
			t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
		}

		result := km.Lookup(mode, "probe-column", keymap.Sequence{keymap.Key("z")})
		gotMatch := result.Outcome == keymap.OutcomeMatch && result.Binding.Kind == keymap.BindingAction
		wantMatch := want == "yes"
		if gotMatch != wantMatch {
			t.Errorf("Lookup(%q, %q, \"z\") matched = %v, want %v -- mode %q's %q cell claims %q", mode, "probe-column", gotMatch, wantMatch, mode, "Column overlay", want)
		}
	}
}

// --- columns row: behavioral probe via keymap.Resolve + Keymap.Lookup ---

// TestDocsCapability_ColumnsRowBehavioralProbe checks the columns overlay
// row's "Key sequences"/"Inline actions" cells against Mode predicates (as
// the other rows are), the "Bare printable-rune key" cell against the fixed
// "n/a" token (columns overlays only ModeNormal/ModeDetail, neither of which
// consumes printable runes, but ModeColumns is never itself a dispatch seam
// -- Mode.Resolvable() is false for it -- so reusing its
// ConsumesPrintableRunes() value here would be misleading), and additionally
// runs a real keymap.Resolve + Keymap.Lookup probe: a sequence-bound command
// and a single-key inline action, both present ONLY in a column overlay
// table (absent from the mode's own table), must actually dispatch through
// Lookup(ModeNormal, "<col>", ...) -- proving the row's "yes"/"yes" claims
// are backed by real Resolve/Lookup behavior, not just a Mode predicate that
// happens to be defined for the (non-resolvable) ModeColumns constant.
func TestDocsCapability_ColumnsRowBehavioralProbe(t *testing.T) {
	content := extractFencedBlock(t, "docs/keymaps.md", keymapCapabilityColumnsMarker)
	rows := parseCapabilityMatrixRows(t, content)
	if len(rows) != 1 {
		t.Fatalf("docs/keymaps.md's %q block has %d rows, want exactly 1", keymapCapabilityColumnsMarker, len(rows))
	}
	row := rows[0]

	wantSeq := "no"
	if keymap.ModeColumns.DispatchesKeySequences() {
		wantSeq = "yes"
	}
	if got := capabilityCellToken(t, row.mode, "Key sequences", row.cells[0]); got != "" && got != wantSeq {
		t.Errorf("columns row's %q cell = %q, want %q", "Key sequences", got, wantSeq)
	}

	wantAction := "no"
	if keymap.ModeColumns.DispatchesInlineActions() {
		wantAction = "yes"
	}
	if got := capabilityCellToken(t, row.mode, "Inline actions", row.cells[1]); got != "" && got != wantAction {
		t.Errorf("columns row's %q cell = %q, want %q", "Inline actions", got, wantAction)
	}

	if got := capabilityCellToken(t, row.mode, "Bare printable-rune key", row.cells[2]); got != "" && got != "n/a" {
		t.Errorf("columns row's %q cell = %q, want \"n/a\" (columns overlays normal/detail only, neither of which consumes printable runes, but ModeColumns is not itself a resolvable dispatch seam)", "Bare printable-rune key", got)
	}

	seqAction := keymap.Action{Name: "Column sequence action", Type: "shell", Command: "echo hi", Scope: "board"}
	singleAction := keymap.Action{Name: "Column single action", Type: "shell", Command: "echo hi", Scope: "board"}

	user := keymap.Tables{
		Modes: map[keymap.Mode]keymap.Table{},
		Columns: map[string]keymap.Table{
			"probe-column": {
				"g p": keymap.ActionBinding(seqAction),
				"z":   keymap.ActionBinding(singleAction),
			},
		},
	}
	km, err := keymap.Resolve(keymap.Tables{}, user)
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}

	seqResult := km.Lookup(keymap.ModeNormal, "probe-column", keymap.Sequence{keymap.Key("g"), keymap.Key("p")})
	if seqResult.Outcome != keymap.OutcomeMatch || seqResult.Binding.Kind != keymap.BindingAction {
		t.Errorf("Lookup(ModeNormal, %q, \"g p\") = %+v, want OutcomeMatch BindingAction -- a column-overlay-only sequence binding must dispatch, matching the columns row's %q = %q claim", "probe-column", seqResult, "Key sequences", wantSeq)
	}

	singleResult := km.Lookup(keymap.ModeNormal, "probe-column", keymap.Sequence{keymap.Key("z")})
	if singleResult.Outcome != keymap.OutcomeMatch || singleResult.Binding.Kind != keymap.BindingAction {
		t.Errorf("Lookup(ModeNormal, %q, \"z\") = %+v, want OutcomeMatch BindingAction -- a column-overlay-only inline-action binding must dispatch, matching the columns row's %q = %q claim", "probe-column", singleResult, "Inline actions", wantAction)
	}
}

// --- (e) fenced YAML schema examples must actually load ---

// loadKeymapSchemaExample writes a bare "keymaps:\n  ..." YAML body (as
// extracted from a doc's fenced ```yaml block) to a temp local config file,
// prefixed with the minimal provider/repo Load needs, and loads it through
// the real config.Load with a self-trusting Trust (mirroring
// extractLegacyRestoreSnippet's sibling test).
func loadKeymapSchemaExample(t *testing.T, yamlBody string) Config {
	t.Helper()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	fullYAML := "provider: github\nrepo: owner/repo\n" + yamlBody
	if err := os.WriteFile(localPath, []byte(fullYAML), 0644); err != nil {
		t.Fatalf("failed to write extracted schema example to a temp config file: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")

	trust := Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}}
	cfg, err := Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() of the extracted schema example returned unexpected error: %v\nYAML:\n%s", err, fullYAML)
	}
	return cfg
}

// TestDocsCapability_ReadmeSchemaExampleLoadsCleanly extracts README.md's
// fenced keymaps: schema example and loads it through the real
// config.Load -> config.ResolveKeymap pipeline.
func TestDocsCapability_ReadmeSchemaExampleLoadsCleanly(t *testing.T) {
	yamlBody := extractFencedBlock(t, "README.md", keymapSchemaExampleMarker)
	cfg := loadKeymapSchemaExample(t, yamlBody)
	if _, err := ResolveKeymap(&cfg); err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error for README's %q example: %v", keymapSchemaExampleMarker, err)
	}
}

// TestDocsCapability_KeymapsDocSchemaExampleLoadsCleanly extracts
// docs/keymaps.md's fenced keymaps: schema example and loads it through the
// real config.Load -> config.ResolveKeymap pipeline. As shipped today this
// example's "detail:\n    ...\n" placeholder is not valid YAML for a keymap
// table -- Phase 4 must replace the placeholder with real content (or omit
// the detail: entry) for this to load.
func TestDocsCapability_KeymapsDocSchemaExampleLoadsCleanly(t *testing.T) {
	yamlBody := extractFencedBlock(t, "docs/keymaps.md", keymapSchemaExampleMarker)
	cfg := loadKeymapSchemaExample(t, yamlBody)
	if _, err := ResolveKeymap(&cfg); err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error for docs/keymaps.md's %q example: %v", keymapSchemaExampleMarker, err)
	}
}

// TestDocsCapability_KeymapsDocPRListScopeOmittedExampleLoadsCleanly
// extracts docs/keymaps.md's dedicated pr_list scope-omitted example,
// asserts it genuinely omits `scope:`, and confirms config.Load REJECTS it
// with the pr_list capability error.
//
// Bug-fix note (#586, phase 4): this test's RED-phase version asserted the
// opposite -- that a scope-omitted example loads cleanly, "demonstrating
// scope inference to pr". That premise is impossible under this codebase's
// already-established, already-tested inferScope semantics: inferScope
// (config.go) only ever infers "card" or "board" from a template's
// placeholders, never "pr" -- see
// TestLoad_KeymapCapability_InlineAction_PRList_ScopeOmitted_Rejected
// (keymap_capability_validation_test.go, landed pre-#586 in #605/#606),
// whose own doc comment states this explicitly: "a scope-omitted pr_list
// action can never pass pr_list's scope=="pr" gate and must be rejected".
// Bending inferScope to match the RED-phase premise would be a production
// change outside this docs-and-test-only ticket's scope and would
// contradict that already-landed, deliberately-tested behavior (and the
// README's own Action Scope section, which documents scope: "pr" as always
// explicit, never inferred). Fixed here to assert the actual, correct
// behavior instead.
func TestDocsCapability_KeymapsDocPRListScopeOmittedExampleLoadsCleanly(t *testing.T) {
	yamlBody := extractFencedBlock(t, "docs/keymaps.md", keymapSchemaPRListMarker)
	if strings.Contains(yamlBody, "scope:") {
		t.Fatalf("docs/keymaps.md's %q example declares an explicit scope: -- it must omit scope so the example demonstrates the omission being rejected, not a stated value:\n%s", keymapSchemaPRListMarker, yamlBody)
	}

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	fullYAML := "provider: github\nrepo: owner/repo\n" + yamlBody
	if err := os.WriteFile(localPath, []byte(fullYAML), 0644); err != nil {
		t.Fatalf("failed to write extracted schema example to a temp config file: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	trust := Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}}

	_, err := Load(globalPath, localPath, trust)
	if err == nil {
		t.Fatalf("config.Load() of docs/keymaps.md's %q example returned nil error, want the pr_list capability error -- a scope-omitted keymaps.pr_list action can never load, since inferScope never yields \"pr\"", keymapSchemaPRListMarker)
	}
	assertCapabilityError(t, err, "keymaps.pr_list", `"pr"`)
}
