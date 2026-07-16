package main

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- Help Mode: Open/Close ---

func TestHelpMode_QuestionMark_OpensHelp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))

	if b.mode != helpMode {
		t.Errorf("after '?': mode = %d, want %d (helpMode)", b.mode, helpMode)
	}
}

func TestHelpMode_QuestionMark_OpensFromDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter detail focus first.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	b = sendKey(t, b, keyMsg("?"))

	if b.mode != helpMode {
		t.Errorf("after '?' from detail focus: mode = %d, want %d (helpMode)", b.mode, helpMode)
	}
	if !b.helpFromDetailFocused {
		t.Error("helpFromDetailFocused should be true when opened from detail focus")
	}
}

func TestHelpMode_Escape_ClosesToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatal("precondition: mode should be helpMode")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Esc in helpMode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestHelpMode_QuestionMark_TogglesClose(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatal("precondition: mode should be helpMode")
	}

	// Press ? again to close.
	b = sendKey(t, b, keyMsg("?"))

	if b.mode != normalMode {
		t.Errorf("after second '?': mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestHelpMode_Escape_RestoresDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Open from detail focus.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatal("precondition: mode should be helpMode")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Esc: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if !b.detailFocused {
		t.Error("after Esc: detailFocused should be restored to true")
	}
}

func TestHelpMode_QuestionMark_RestoresDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Open from detail focus.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("?"))

	// Close with ?.
	b = sendKey(t, b, keyMsg("?"))

	if !b.detailFocused {
		t.Error("after '?' close: detailFocused should be restored to true")
	}
}

// --- Help Mode: Ignored in Other Modes ---

func TestHelpMode_IgnoredInOtherModes(t *testing.T) {
	tests := []struct {
		name string
		mode boardMode
	}{
		{"createMode", createMode},
		{"creatingMode", creatingMode},
		{"configMode", configMode},
		{"loadingMode", loadingMode},
		{"errorMode", errorMode},
		{"prPickerMode", prPickerMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			b.Width = 120
			b.Height = 40
			b.mode = tt.mode

			b = sendKey(t, b, keyMsg("?"))

			if b.mode == helpMode {
				t.Errorf("pressing '?' in %s should not open helpMode", tt.name)
			}
		})
	}
}

// --- Help Mode: Scroll ---

func TestHelpMode_JKey_ScrollsDown(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("j"))

	if b.helpScrollOffset < 1 {
		t.Errorf("helpScrollOffset = %d after 'j', want >= 1", b.helpScrollOffset)
	}
}

func TestHelpMode_KKey_ScrollsUp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	offsetAfterDown := b.helpScrollOffset

	b = sendKey(t, b, keyMsg("k"))

	if b.helpScrollOffset >= offsetAfterDown {
		t.Errorf("helpScrollOffset = %d after 'k', want less than %d", b.helpScrollOffset, offsetAfterDown)
	}
}

func TestHelpMode_KKey_ClampsAtZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("k"))

	if b.helpScrollOffset != 0 {
		t.Errorf("helpScrollOffset = %d after 'k' at offset 0, want 0", b.helpScrollOffset)
	}
}

func TestHelpMode_JKey_ClampsAtMaxOffset(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 20 // Small height so help content exceeds visible area.

	b = sendKey(t, b, keyMsg("?"))

	// Scroll down many times — should clamp at max offset.
	for i := 0; i < 200; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	maxOffset := b.helpMaxScrollOffset()
	if b.helpScrollOffset != maxOffset {
		t.Errorf("helpScrollOffset = %d after excessive scrolling, want %d (maxOffset)", b.helpScrollOffset, maxOffset)
	}
	// Pressing k once from max offset should immediately respond.
	b = sendKey(t, b, keyMsg("k"))
	if b.helpScrollOffset != maxOffset-1 {
		t.Errorf("helpScrollOffset = %d after single 'k' from max, want %d", b.helpScrollOffset, maxOffset-1)
	}
}

// --- Help Mode: Quit ---

func TestHelpMode_QKey_Quits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))

	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("pressing 'q' in helpMode should return a non-nil cmd (tea.Quit)")
	}
}

func TestHelpMode_CtrlC_Quits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("pressing ctrl+c in helpMode should return a non-nil cmd (tea.Quit)")
	}
}

// --- Help Mode: Blocks Navigation ---

func TestHelpMode_BlocksNavigation(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	initialTab := b.ActiveTab
	initialCursor := b.Columns[b.ActiveTab].Cursor

	b = sendKey(t, b, keyMsg("?"))

	// Try various navigation keys.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, keyMsg("r"))

	if b.mode != helpMode {
		t.Errorf("navigation keys should not change mode in helpMode, got %d", b.mode)
	}
	if b.ActiveTab != initialTab {
		t.Errorf("ActiveTab changed from %d to %d in helpMode", initialTab, b.ActiveTab)
	}
	if b.Columns[b.ActiveTab].Cursor != initialCursor {
		t.Errorf("Cursor changed from %d to %d in helpMode", initialCursor, b.Columns[b.ActiveTab].Cursor)
	}
}

// --- Help Mode: Scroll Reset on Reopen ---

func TestHelpMode_ScrollResetOnReopen(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Open help and scroll down.
	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.helpScrollOffset == 0 {
		t.Fatal("precondition: helpScrollOffset should be > 0 after scrolling")
	}

	// Close help.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Reopen help — scroll offset should be reset to 0.
	b = sendKey(t, b, keyMsg("?"))
	if b.helpScrollOffset != 0 {
		t.Errorf("helpScrollOffset = %d after reopening help, want 0", b.helpScrollOffset)
	}
}

// --- Help Mode: View ---

func TestHelpMode_ViewShowsKeybindings(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 80

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	expectedTexts := []string{
		"Quit",
		"New card",
		"Help",
		"Normal Mode",
		"Detail Panel",
		"Create Card",
		"Configuration",
		"Comment",
		"Search",
		"Assign",
	}
	for _, text := range expectedTexts {
		if !strings.Contains(view, text) {
			t.Errorf("View() in helpMode should contain %q", text)
		}
	}
}

func TestHelpContent_AllSectionsPresent(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	sections := []string{
		"Normal Mode",
		"Detail Panel",
		"Create Card",
		"Configuration",
		"PR Picker",
		"Comment",
		"Filter",
		"Search",
		"Assign",
		"Error",
		"Usage",
	}
	for _, section := range sections {
		if !strings.Contains(content, section) {
			t.Errorf("buildHelpContent() should contain section %q", section)
		}
	}
}

func TestHelpContent_ContainsSearchSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	searchIdx := strings.Index(content, "\nSearch\n")
	if searchIdx == -1 {
		t.Fatal("buildHelpContent() should contain 'Search' section header")
	}

	// Find the section content (between Search header and next section).
	sectionContent := content[searchIdx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	if !strings.Contains(sectionContent, "esc") {
		t.Error("Search section should contain 'esc' key")
	}
	if !strings.Contains(sectionContent, "Clear") {
		t.Error("Search section should contain 'Clear' description")
	}
}

func TestHelpContent_ContainsAssignSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	assignIdx := strings.Index(content, "\nAssign\n")
	if assignIdx == -1 {
		t.Fatal("buildHelpContent() should contain 'Assign' section header")
	}

	sectionContent := content[assignIdx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	expectedKeys := []string{"Navigate", "Toggle", "Cancel"}
	for _, key := range expectedKeys {
		if !strings.Contains(sectionContent, key) {
			t.Errorf("Assign section should contain %q", key)
		}
	}
}

func TestHelpContent_NormalModeIncludesSearchAndAssign(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	// Extract Normal Mode section (between "Normal Mode" and "Detail Panel").
	normalStart := strings.Index(content, "Normal Mode\n")
	detailStart := strings.Index(content, "\nDetail Panel\n")
	if normalStart == -1 || detailStart == -1 {
		t.Fatal("buildHelpContent() should contain Normal Mode and Detail Panel sections")
	}
	normalSection := content[normalStart:detailStart]

	if !strings.Contains(normalSection, "/") {
		t.Error("Normal Mode section should contain '/' for Search")
	}
	if !strings.Contains(normalSection, "Assign") {
		t.Error("Normal Mode section should contain 'Assign'")
	}
	if !strings.Contains(normalSection, "Filter") {
		t.Error("Normal Mode section should contain 'Filter'")
	}
}

func TestHelpMode_ViewShowsCustomActions(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Deploy App", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Height = 170 // Tall enough that full help content (incl. the Delete section) renders without scrolling.

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Custom Actions") {
		t.Error("View() in helpMode should contain 'Custom Actions' section header")
	}
	if !strings.Contains(view, "Deploy App") {
		t.Error("View() in helpMode should contain custom action name 'Deploy App'")
	}
}

func TestHelpMode_ViewShowsColumnActions(t *testing.T) {
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"D": {Name: "Deploy Column", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)
	b.Height = 200 // Tall enough that full help content (incl. column-specific actions) renders without scrolling.

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Deploy Column") {
		t.Error("View() in helpMode should contain column action name 'Deploy Column'")
	}
}

func TestHelpContent_GlobalActionsAreSortedByKey(t *testing.T) {
	actions := map[string]config.Action{
		"Z": {Name: "Zebra Deploy", Type: "url", URL: "https://example.com/{number}"},
		"A": {Name: "Alpha Deploy", Type: "url", URL: "https://example.com/{number}"},
		"M": {Name: "Mid Deploy", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)
	content := b.buildHelpContent()

	aIdx := strings.Index(content, "Alpha Deploy")
	mIdx := strings.Index(content, "Mid Deploy")
	zIdx := strings.Index(content, "Zebra Deploy")
	if aIdx == -1 || mIdx == -1 || zIdx == -1 {
		t.Fatal("buildHelpContent() should list all global custom actions")
	}
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("global custom actions should be sorted by key (A, M, Z), got order at indices A=%d M=%d Z=%d", aIdx, mIdx, zIdx)
	}
}

func TestHelpContent_ColumnActionsAreSortedByKey(t *testing.T) {
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"Z": {Name: "Zebra Column", Type: "url", URL: "https://deploy.com/{number}"},
				"A": {Name: "Alpha Column", Type: "url", URL: "https://deploy.com/{number}"},
				"M": {Name: "Mid Column", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)
	content := b.buildHelpContent()

	aIdx := strings.Index(content, "Alpha Column")
	mIdx := strings.Index(content, "Mid Column")
	zIdx := strings.Index(content, "Zebra Column")
	if aIdx == -1 || mIdx == -1 || zIdx == -1 {
		t.Fatal("buildHelpContent() should list all column custom actions")
	}
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("column custom actions should be sorted by key (A, M, Z), got order at indices A=%d M=%d Z=%d", aIdx, mIdx, zIdx)
	}
}

func TestHelpMode_ViewShowsUsageSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 170 // Tall enough that full help content (incl. Status Bar section) renders without scrolling.

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Usage") {
		t.Error("View() in helpMode should contain 'Usage' section")
	}
	if !strings.Contains(view, ".lazyboards.yml") {
		t.Error("View() in helpMode should mention config file name")
	}
}

func TestHelpMode_ViewShowsCommentModeSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 80

	b = sendKey(t, b, keyMsg("?"))
	content := b.buildHelpContent()

	if !strings.Contains(content, "Comment") {
		t.Error("buildHelpContent() should contain 'Comment' section header")
	}
	if !strings.Contains(content, "Submit") {
		t.Error("buildHelpContent() Comment section should contain 'Submit' key")
	}
	if !strings.Contains(content, "Cancel") {
		t.Error("buildHelpContent() Comment section should contain 'Cancel' key")
	}
}

func TestHelpMode_ViewShowsAltKeyInNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 80

	b = sendKey(t, b, keyMsg("?"))
	content := b.buildHelpContent()

	if !strings.Contains(content, "alt+shift+key") {
		t.Error("buildHelpContent() Normal Mode should contain 'alt+shift+key' entry")
	}
}

// --- Help Mode: cenci-dependent features labeled ---
//
// Agents (w/s keys, "Agents" modal section) and Dispatch (d key, "Dispatch"
// modal section) both require cenci integration to be configured. They are
// labeled with a static "(cenci)" annotation in the help popup so users know
// these features won't do anything useful without cenci set up. See #417.

func TestHelpContent_NormalModeRowsLabeledCenci(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	normalStart := strings.Index(content, "Normal Mode\n")
	detailStart := strings.Index(content, "\nDetail Panel\n")
	if normalStart == -1 || detailStart == -1 {
		t.Fatal("buildHelpContent() should contain Normal Mode and Detail Panel sections")
	}
	normalSection := content[normalStart:detailStart]

	tests := []struct {
		name string
		key  string
	}{
		{"AgentsRow_w", "w"},
		{"CardAgentsRow_s", "s"},
		{"DispatchRow_d", "d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(`(?m)^  ` + tt.key + `\s+.*$`)
			line := re.FindString(normalSection)
			if line == "" {
				t.Fatalf("Normal Mode section should contain a row for key %q", tt.key)
			}
			if !strings.Contains(line, "(cenci)") {
				t.Errorf("Normal Mode row for key %q should be labeled with '(cenci)', got line %q", tt.key, line)
			}
		})
	}
}

// findSectionHeader returns the first unindented line in content that
// contains want. Section header lines start at column 0; keybinding row
// lines always start with two spaces (see buildHelpContent's "  %-12s %s\n"
// format), so this distinguishes a section title (e.g. "Agents") from a row
// whose description merely mentions the same word (e.g. Normal Mode's "w"
// row, or the Status Bar section's "Agents running" line).
func findSectionHeader(content, want string) string {
	for _, line := range strings.Split(content, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		if strings.Contains(line, want) {
			return line
		}
	}
	return ""
}

func TestHelpContent_AgentsSectionHeaderLabeledCenci(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	header := findSectionHeader(content, "Agents")
	if header == "" {
		t.Fatal("buildHelpContent() should contain an 'Agents' section header")
	}
	if !strings.Contains(header, "(cenci)") {
		t.Errorf("Agents section header should be labeled with '(cenci)', got %q", header)
	}
}

func TestHelpContent_DispatchSectionHeaderLabeledCenci(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	header := findSectionHeader(content, "Dispatch")
	if header == "" {
		t.Fatal("buildHelpContent() should contain a 'Dispatch' section header")
	}
	if !strings.Contains(header, "(cenci)") {
		t.Errorf("Dispatch section header should be labeled with '(cenci)', got %q", header)
	}
}

func TestHelpMode_StatusBarShowsHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Close") {
		t.Errorf("View() in helpMode should contain hint desc %q", "Close")
	}
	if !strings.Contains(view, "Scroll") {
		t.Errorf("View() in helpMode should contain hint desc %q", "Scroll")
	}
}

// --- Help Content: Accuracy & Consistency ---
//
// handleAgentJumpKey (mode_handlers.go) documents that "s" jumps directly to
// the single matching agent window (no modal) when there's exactly one, and
// only opens the Agents modal when there are several. "Card agents" implied
// a list is always shown, which isn't true in the single-window case.

func TestHelpContent_AgentJumpKeyIsNotMislabeledAsCardAgents(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	if strings.Contains(content, "Card agents") {
		t.Error("buildHelpContent() should not describe 's' as 'Card agents' — it jumps directly to the single matching window, only opening a list when there are several")
	}
}

func TestHelpContent_NormalModeAgentJumpKeyDescribesJump(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	normalStart := strings.Index(content, "Normal Mode\n")
	detailStart := strings.Index(content, "\nDetail Panel\n")
	if normalStart == -1 || detailStart == -1 {
		t.Fatal("buildHelpContent() should contain Normal Mode and Detail Panel sections")
	}
	normalSection := content[normalStart:detailStart]

	if !strings.Contains(normalSection, "Go to agent") {
		t.Error("Normal Mode section should describe 's' as 'Go to agent'")
	}
}

func TestHelpContent_ContainsCloseConfirmSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	idx := strings.Index(content, "\nClose Confirm\n")
	if idx == -1 {
		t.Fatal("buildHelpContent() should contain 'Close Confirm' section header — closeConfirmMode (triggered by 'x') has no documented keys")
	}
	sectionContent := content[idx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	for _, want := range []string{"x", "y", "Cancel"} {
		if !strings.Contains(sectionContent, want) {
			t.Errorf("Close Confirm section should contain %q, got:\n%s", want, sectionContent)
		}
	}
}

func TestHelpContent_ContainsLabelConfirmSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	idx := strings.Index(content, "\nLabel Confirm\n")
	if idx == -1 {
		t.Fatal("buildHelpContent() should contain 'Label Confirm' section header — labelConfirmMode (entered after editing a card with unknown labels) has no documented keys")
	}
	sectionContent := content[idx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	for _, want := range []string{"y", "n", "Cancel"} {
		if !strings.Contains(sectionContent, want) {
			t.Errorf("Label Confirm section should contain %q, got:\n%s", want, sectionContent)
		}
	}
}

func TestHelpContent_SearchSectionDocumentsColumnSwitch(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	idx := strings.Index(content, "\nSearch\n")
	if idx == -1 {
		t.Fatal("buildHelpContent() should contain 'Search' section header")
	}
	sectionContent := content[idx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	if !strings.Contains(sectionContent, "tab/s-tab") {
		t.Error("Search section should document tab/shift-tab column switching (handleSearchModeKey handles both)")
	}
}

func TestHelpContent_CreateCardDocumentsAssigneeCycle(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	idx := strings.Index(content, "\nCreate Card\n")
	if idx == -1 {
		t.Fatal("buildHelpContent() should contain 'Create Card' section header")
	}
	sectionContent := content[idx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	if !strings.Contains(sectionContent, "Cycle assignee") {
		t.Error("Create Card section should document left/right cycling the assignee field (handleCreateModeKey)")
	}
}

func TestHelpContent_GitMenuDocumentsOpenTrigger(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	idx := strings.Index(content, "\nGit Menu\n")
	if idx == -1 {
		t.Fatal("buildHelpContent() should contain 'Git Menu' section header")
	}
	sectionContent := content[idx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	if !strings.Contains(sectionContent, "g") || !strings.Contains(sectionContent, "Open") {
		t.Errorf("Git Menu section should document 'g' as the open trigger (consistent with Delete/Filter/Dispatch), got:\n%s", sectionContent)
	}
}

func TestHelpContent_DispatchOpenTriggerMatchesDeleteConvention(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()

	idx := strings.Index(content, "\nDispatch (cenci)\n")
	if idx == -1 {
		t.Fatal("buildHelpContent() should contain 'Dispatch' section header")
	}
	sectionContent := content[idx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}

	if !strings.Contains(sectionContent, "(from Normal Mode)") {
		t.Error("Dispatch section's open trigger should note '(from Normal Mode)', consistent with the Delete section")
	}
}
