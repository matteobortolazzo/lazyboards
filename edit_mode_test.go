package main

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Frontmatter Compose/Parse ---

func TestComposeFrontmatter_BasicTitleAndBody(t *testing.T) {
	title := "My Title"
	body := "Some body"
	result := composeFrontmatter(title, nil, body)

	if !strings.HasPrefix(result, "---\n") {
		t.Error("composeFrontmatter should start with opening ---")
	}
	if !strings.Contains(result, "title: My Title") {
		t.Errorf("composeFrontmatter should contain title field, got:\n%s", result)
	}
	// The closing --- should separate frontmatter from body.
	parts := strings.SplitN(result, "---", 3)
	if len(parts) < 3 {
		t.Fatalf("composeFrontmatter should have opening ---, content, closing ---, got:\n%s", result)
	}
	bodyPart := strings.TrimSpace(parts[2])
	if bodyPart != body {
		t.Errorf("composeFrontmatter body = %q, want %q", bodyPart, body)
	}
}

func TestComposeFrontmatter_EmptyBody(t *testing.T) {
	title := "My Title"
	result := composeFrontmatter(title, nil, "")

	if !strings.Contains(result, "title: My Title") {
		t.Errorf("composeFrontmatter should contain title field, got:\n%s", result)
	}
	// With empty body, content after closing --- should be empty.
	parts := strings.SplitN(result, "---", 3)
	if len(parts) < 3 {
		t.Fatalf("composeFrontmatter should have opening ---, content, closing ---, got:\n%s", result)
	}
	bodyPart := strings.TrimSpace(parts[2])
	if bodyPart != "" {
		t.Errorf("composeFrontmatter body with empty input = %q, want empty string", bodyPart)
	}
}

func TestComposeFrontmatter_SpecialCharsInTitle(t *testing.T) {
	// Titles with colons and quotes should be preserved in the output.
	title := `Fix: handle "edge" case`
	result := composeFrontmatter(title, nil, "body")

	if !strings.Contains(result, "Fix") {
		t.Errorf("composeFrontmatter should preserve title content, got:\n%s", result)
	}
	if !strings.Contains(result, "edge") {
		t.Errorf("composeFrontmatter should preserve quoted content in title, got:\n%s", result)
	}
}

func TestParseFrontmatter_RoundTrip(t *testing.T) {
	originalTitle := "My Title"
	originalBody := "Some body content"

	composed := composeFrontmatter(originalTitle, nil, originalBody)
	title, _, body, err := parseFrontmatter(composed)

	if err != nil {
		t.Fatalf("parseFrontmatter round-trip error: %v", err)
	}
	if title != originalTitle {
		t.Errorf("parseFrontmatter title = %q, want %q", title, originalTitle)
	}
	if body != originalBody {
		t.Errorf("parseFrontmatter body = %q, want %q", body, originalBody)
	}
}

func TestParseFrontmatter_TitleWithDashes(t *testing.T) {
	// A title containing "---" should round-trip without corruption.
	originalTitle := "My --- Title"
	originalBody := "Some body"

	composed := composeFrontmatter(originalTitle, nil, originalBody)
	title, _, body, err := parseFrontmatter(composed)

	if err != nil {
		t.Fatalf("parseFrontmatter round-trip error with dashes in title: %v", err)
	}
	if title != originalTitle {
		t.Errorf("parseFrontmatter title = %q, want %q", title, originalTitle)
	}
	if body != originalBody {
		t.Errorf("parseFrontmatter body = %q, want %q", body, originalBody)
	}
}

func TestParseFrontmatter_EmptyBody(t *testing.T) {
	composed := composeFrontmatter("Title Only", nil, "")
	title, _, body, err := parseFrontmatter(composed)

	if err != nil {
		t.Fatalf("parseFrontmatter error: %v", err)
	}
	if title != "Title Only" {
		t.Errorf("parseFrontmatter title = %q, want %q", title, "Title Only")
	}
	if body != "" {
		t.Errorf("parseFrontmatter body = %q, want empty string", body)
	}
}

func TestParseFrontmatter_MissingClosingDelimiter(t *testing.T) {
	// Missing closing --- should return an error.
	input := "---\ntitle: Bad\nNo closing delimiter"
	_, _, _, err := parseFrontmatter(input)

	if err == nil {
		t.Error("parseFrontmatter should return an error when closing --- is missing")
	}
}

func TestParseFrontmatter_BlankTitle(t *testing.T) {
	// A blank title should return an error.
	input := "---\ntitle: \n---\nSome body"
	_, _, _, err := parseFrontmatter(input)

	if err == nil {
		t.Error("parseFrontmatter should return an error when title is blank")
	}
}

func TestParseFrontmatter_ExtraWhitespace(t *testing.T) {
	// Title with leading/trailing whitespace should be trimmed.
	input := "---\ntitle:   Spaced Title   \n---\nBody here"
	title, _, body, err := parseFrontmatter(input)

	if err != nil {
		t.Fatalf("parseFrontmatter error: %v", err)
	}
	if title != "Spaced Title" {
		t.Errorf("parseFrontmatter title = %q, want %q (should be trimmed)", title, "Spaced Title")
	}
	if body != "Body here" {
		t.Errorf("parseFrontmatter body = %q, want %q", body, "Body here")
	}
}

// --- Editor Resolution ---

func TestResolveEditor_VisualSet(t *testing.T) {
	t.Setenv("VISUAL", "code")
	t.Setenv("EDITOR", "vim")

	editor := resolveEditor()
	if editor != "code" {
		t.Errorf("resolveEditor() = %q, want %q (should prefer $VISUAL)", editor, "code")
	}
}

func TestResolveEditor_VisualEmptyEditorSet(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nano")

	editor := resolveEditor()
	if editor != "nano" {
		t.Errorf("resolveEditor() = %q, want %q (should fall back to $EDITOR)", editor, "nano")
	}
}

func TestResolveEditor_BothEmpty(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	editor := resolveEditor()
	if editor != "vi" {
		t.Errorf("resolveEditor() = %q, want %q (should fall back to vi)", editor, "vi")
	}
}

// --- Edit Mode Key Behavior ---

func TestEditMode_EKeyNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Pressing 'e' in normal mode with cards should return a non-nil cmd
	// (the editor command via tea.ExecProcess).
	_, cmd := b.Update(keyMsg("e"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'e' key in normal mode (should open editor)")
	}
}

func TestEditMode_EKeyDetailFocused(t *testing.T) {
	b := newBoardWithBody(t, "Some body text", "Other body")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	// Pressing 'e' in detail-focused mode should also return a non-nil cmd.
	_, cmd := b.Update(keyMsg("e"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'e' key in detail focus (should open editor)")
	}
}

func TestEditMode_EKeyNoCards(t *testing.T) {
	// Load a board with an empty column.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty", Cards: nil},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Pressing 'e' with no cards should do nothing (nil cmd).
	_, cmd := b.Update(keyMsg("e"))
	if cmd != nil {
		t.Error("expected nil cmd from 'e' key when no cards exist")
	}
}

func TestEditMode_EKeyNonNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode.
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Fatalf("precondition: mode = %d, want %d (createMode)", b.mode, createMode)
	}

	// Pressing 'e' in createMode should stay in createMode (not open editor).
	b = sendKey(t, b, keyMsg("e"))
	if b.mode != createMode {
		t.Errorf("mode = %d after 'e' in createMode, want %d (should stay in createMode)", b.mode, createMode)
	}
}

// --- Editor Finished Messages ---

func TestEditMode_EditorFinishedNoChanges(t *testing.T) {
	b := newBoardWithBody(t, "Original body", "Other body")

	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	content := composeFrontmatter(card.Title, labelNames, card.Body)

	// Send editorFinishedMsg with unchanged content.
	msg := editorFinishedMsg{
		originalContent: content,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Status bar should show a "cancelled" or "no changes" message.
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") && !strings.Contains(strings.ToLower(view), "no change") {
		t.Errorf("View() after no-change edit should contain 'cancel' or 'no change' message, got:\n%s", view)
	}
}

func TestEditMode_EditorFinishedBlankTitle(t *testing.T) {
	b := newBoardWithBody(t, "Original body", "Other body")

	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	originalContent := composeFrontmatter(card.Title, labelNames, card.Body)

	// Send editorFinishedMsg with edited content that has a blank title.
	blankTitleContent := "---\ntitle: \n---\nnew body"
	msg := editorFinishedMsg{
		editedContent:   blankTitleContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Status bar should show an error message about the blank title.
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Errorf("View() after blank title edit should contain 'error' message, got:\n%s", view)
	}
}

func TestEditMode_EditorFinishedWithChanges(t *testing.T) {
	b := newBoardWithBody(t, "Original body", "Other body")

	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	originalContent := composeFrontmatter(card.Title, labelNames, card.Body)

	// Create a modified version of the content.
	modifiedContent := composeFrontmatter("Updated Title", labelNames, "Updated body")

	// When edited content differs from original, the handler should return
	// a non-nil cmd (updateCardCmd) to persist the changes.
	msg := editorFinishedMsg{
		editedContent:   modifiedContent,
		originalContent: originalContent,
		card:            card,
	}
	_, cmd := b.Update(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd (updateCardCmd) when content changed")
	}
}

func TestEditMode_EditorError(t *testing.T) {
	b := newBoardWithBody(t, "Original body", "Other body")

	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]

	// Send editorFinishedMsg with an error (e.g., editor failed to open).
	msg := editorFinishedMsg{
		card: card,
		err:  errEditorFailed,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Status bar should show an error message.
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Errorf("View() after editor error should contain 'error' message, got:\n%s", view)
	}
}

// --- Card Updated / Update Error Messages ---

func TestEditMode_CardUpdatedMsg(t *testing.T) {
	b := newBoardWithBody(t, "Original body", "Other body")

	// Get the current card.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]

	// Send cardUpdatedMsg with updated title and body.
	updatedCard := provider.Card{
		Number: selectedCard.Number,
		Title:  "Updated Title",
		Body:   "Updated body",
		Labels: []provider.Label{{Name: "bug"}},
	}
	m, _ := b.Update(cardUpdatedMsg{card: updatedCard})
	b = m.(Board)

	// The local card data should reflect the update.
	localCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if localCard.Title != "Updated Title" {
		t.Errorf("card Title = %q after cardUpdatedMsg, want %q", localCard.Title, "Updated Title")
	}
	if localCard.Body != "Updated body" {
		t.Errorf("card Body = %q after cardUpdatedMsg, want %q", localCard.Body, "Updated body")
	}

	// Status bar should show a success message.
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "updated") {
		t.Errorf("View() after card update should contain 'updated' message, got:\n%s", view)
	}
}

func TestEditMode_CardUpdateErrorMsg(t *testing.T) {
	b := newBoardWithBody(t, "Original body", "Other body")

	// Send cardUpdateErrorMsg.
	msg := cardUpdateErrorMsg{err: errUpdateFailed}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Status bar should show an error message.
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "error") || !strings.Contains(strings.ToLower(view), "update") {
		t.Errorf("View() after card update error should contain error message, got:\n%s", view)
	}
}

// --- Hints ---

func TestEditMode_HintShown(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	view := b.View()
	if !strings.Contains(view, "Edit") {
		t.Errorf("View() in normal mode should contain hint desc %q for edit key, got:\n%s", "Edit", view)
	}
}

func TestEditMode_DetailHintShown(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	view := b.View()
	if !strings.Contains(view, "Edit") {
		t.Errorf("View() in detail focus should contain hint desc %q for edit key, got:\n%s", "Edit", view)
	}
}

// --- Frontmatter Empty Labels (#217) ---

func TestComposeFrontmatter_EmptyLabelsRoundTrip(t *testing.T) {
	// Composing with nil labels and parsing back should produce an empty
	// labels slice and no error.
	composed := composeFrontmatter("Title", nil, "body")
	title, labels, body, err := parseFrontmatter(composed)

	if err != nil {
		t.Fatalf("parseFrontmatter round-trip error: %v", err)
	}
	if title != "Title" {
		t.Errorf("title = %q, want %q", title, "Title")
	}
	if len(labels) != 0 {
		t.Errorf("labels = %v, want empty slice", labels)
	}
	if body != "body" {
		t.Errorf("body = %q, want %q", body, "body")
	}
}

func TestParseFrontmatter_EmptyLabelsValue(t *testing.T) {
	// A labels: key with no value should parse to an empty labels slice.
	input := "---\ntitle: My Title\nlabels:\n---\nBody"
	title, labels, body, err := parseFrontmatter(input)

	if err != nil {
		t.Fatalf("parseFrontmatter error: %v", err)
	}
	if title != "My Title" {
		t.Errorf("title = %q, want %q", title, "My Title")
	}
	if len(labels) != 0 {
		t.Errorf("labels = %v, want empty slice", labels)
	}
	if body != "Body" {
		t.Errorf("body = %q, want %q", body, "Body")
	}
}

// errEditorFailed is a sentinel error for testing editor failures.
var errEditorFailed = errSentinel("editor failed to open")

// errUpdateFailed is a sentinel error for testing card update failures.
var errUpdateFailed = errSentinel("update failed")

// errSentinel is a simple error type for test sentinels.
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
