package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Frontmatter with Labels ---

func TestComposeFrontmatter_WithLabels(t *testing.T) {
	// composeFrontmatter should include labels field when labels are non-empty.
	result := composeFrontmatter("Title", []string{"bug", "feature"}, "body")
	if !strings.Contains(result, "labels: bug, feature") {
		t.Errorf("expected labels field in frontmatter, got:\n%s", result)
	}
}

func TestComposeFrontmatter_EmptyLabels(t *testing.T) {
	// composeFrontmatter with empty labels should still include the labels: key
	// (with no value after it) so the user can add labels in the editor.
	result := composeFrontmatter("Title", nil, "body")
	if !strings.Contains(result, "labels:") {
		t.Errorf("expected labels: key present even when labels empty, got:\n%s", result)
	}
	// The labels: line should have no value after the colon (bare key).
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "labels:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "labels:"))
			if value != "" {
				t.Errorf("labels: line should have no value when labels are empty, got value %q", value)
			}
		}
	}
}

func TestParseFrontmatter_WithLabels(t *testing.T) {
	// Round-trip: compose with labels, then parse should return same labels.
	composed := composeFrontmatter("Title", []string{"bug", "feature"}, "body text")
	title, labels, body, err := parseFrontmatter(composed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Title" {
		t.Errorf("title = %q, want %q", title, "Title")
	}
	if len(labels) != 2 || labels[0] != "bug" || labels[1] != "feature" {
		t.Errorf("labels = %v, want [bug, feature]", labels)
	}
	if body != "body text" {
		t.Errorf("body = %q, want %q", body, "body text")
	}
}

func TestParseFrontmatter_NoLabelsField(t *testing.T) {
	// Frontmatter without labels field should return empty labels slice.
	input := "---\ntitle: My Title\n---\nBody"
	title, labels, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "My Title" {
		t.Errorf("title = %q, want %q", title, "My Title")
	}
	if len(labels) != 0 {
		t.Errorf("labels = %v, want empty", labels)
	}
	if body != "Body" {
		t.Errorf("body = %q, want %q", body, "Body")
	}
}

func TestParseFrontmatter_LabelsWithWhitespace(t *testing.T) {
	// Labels with extra whitespace should be trimmed.
	input := "---\ntitle: Title\nlabels:  bug ,  feature  \n---\nBody"
	_, labels, _, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 2 || labels[0] != "bug" || labels[1] != "feature" {
		t.Errorf("labels = %v, want [bug, feature]", labels)
	}
}

func TestParseFrontmatter_SingleLabel(t *testing.T) {
	input := "---\ntitle: Title\nlabels: bug\n---\nBody"
	_, labels, _, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 1 || labels[0] != "bug" {
		t.Errorf("labels = %v, want [bug]", labels)
	}
}

func TestParseFrontmatter_EmptyLabelsField(t *testing.T) {
	// "labels:" with empty value should return empty slice.
	input := "---\ntitle: Title\nlabels: \n---\nBody"
	_, labels, _, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("labels = %v, want empty", labels)
	}
}

// --- Known Label Collection ---

func TestCollectKnownLabels(t *testing.T) {
	// Board with cards across columns should collect all unique label names.
	b := newLoadedTestBoard(t)
	known := b.collectKnownLabels()
	// FakeProvider has labels: infra, design, docs, feature, backend, ui, bug, chore
	for _, name := range []string{"infra", "design", "docs", "feature", "backend", "ui", "bug", "chore"} {
		if !known[strings.ToLower(name)] {
			t.Errorf("collectKnownLabels missing label %q", name)
		}
	}
}

func TestCollectKnownLabels_CaseInsensitive(t *testing.T) {
	b := newBoardWithCustomCard(t, "Card", []provider.Label{{Name: "BUG"}}, "body")
	known := b.collectKnownLabels()
	if !known["bug"] {
		t.Error("collectKnownLabels should store lowercased label names")
	}
}

// --- Editor Finished with Label Validation ---

func TestEditMode_AllLabelsKnown_ProceedsToUpdate(t *testing.T) {
	// When all edited labels are already known, handleEditorFinished should
	// return updateCardCmd (non-nil cmd) without entering labelConfirmMode.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Updated Title", []string{"bug"}, "new body")

	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, cmd := b.Update(msg)
	updated := m.(Board)

	if cmd == nil {
		t.Fatal("expected non-nil cmd (updateCardCmd) when all labels known")
	}
	if updated.mode == labelConfirmMode {
		t.Error("should NOT enter labelConfirmMode when all labels are known")
	}
}

func TestEditMode_UnknownLabel_EntersLabelConfirmMode(t *testing.T) {
	// When an unknown label is found, board should enter labelConfirmMode.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	updated := m.(Board)

	if updated.mode != labelConfirmMode {
		t.Errorf("mode = %d, want labelConfirmMode", updated.mode)
	}
}

// --- Label Confirm Mode Keys ---

func TestLabelConfirm_YKey_CreatesLabel(t *testing.T) {
	// Pressing 'y' should return a non-nil cmd (createLabelCmd).
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Now press 'y' to confirm label creation.
	m, cmd := b.Update(keyMsg("y"))
	b = m.(Board)

	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'y' key (createLabelCmd)")
	}
}

func TestLabelConfirm_NKey_CancelsEdit(t *testing.T) {
	// Pressing 'n' should cancel edit and return to normalMode.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Press 'n' to decline.
	m, _ = b.Update(keyMsg("n"))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after 'n', want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") {
		t.Errorf("View() after 'n' should contain cancel message, got:\n%s", view)
	}
}

func TestLabelConfirm_EscKey_CancelsEdit(t *testing.T) {
	// Pressing 'esc' should cancel edit and return to normalMode.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Press 'esc' to cancel.
	m, _ = b.Update(arrowMsg(tea.KeyEsc))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after 'esc', want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") {
		t.Errorf("View() after 'esc' should contain cancel message, got:\n%s", view)
	}
}

// --- Label Created / Error Messages ---

func TestLabelConfirm_LabelCreated_LastLabel_ProceedsToUpdate(t *testing.T) {
	// After last unknown label is created, should proceed with updateCardCmd.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	// Enter labelConfirmMode.
	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Press 'y' to create label.
	m, cmd := b.Update(keyMsg("y"))
	b = m.(Board)
	execCmds(cmd)

	// Send labelCreatedMsg (label was created successfully).
	m, cmd = b.Update(labelCreatedMsg{})
	b = m.(Board)

	if cmd == nil {
		t.Fatal("expected non-nil cmd (updateCardCmd) after last label created")
	}
}

func TestLabelConfirm_MultipleUnknownLabels_SequentialPrompts(t *testing.T) {
	// With multiple unknown labels, after creating first, should prompt for next.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel1", "newlabel2"}, "body")

	// Enter labelConfirmMode.
	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Press 'y' for first unknown label.
	m, cmd := b.Update(keyMsg("y"))
	b = m.(Board)
	execCmds(cmd)

	// Send labelCreatedMsg for first label.
	m, _ = b.Update(labelCreatedMsg{})
	b = m.(Board)

	// Should still be in labelConfirmMode for the second unknown label.
	if b.mode != labelConfirmMode {
		t.Errorf("mode = %d after first label created, want labelConfirmMode for next unknown", b.mode)
	}
}

func TestLabelConfirm_LabelCreateError_CancelsEdit(t *testing.T) {
	// labelCreateErrorMsg should cancel the edit and show error.
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	// Enter labelConfirmMode.
	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	// Press 'y' to attempt creation.
	m, _ = b.Update(keyMsg("y"))
	b = m.(Board)

	// Send error.
	m, _ = b.Update(labelCreateErrorMsg{err: errSentinel("failed to create label")})
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after label create error, want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Errorf("View() after label create error should contain error message")
	}
}

// --- View Rendering ---

func TestLabelConfirm_ViewShowsPrompt(t *testing.T) {
	b := newBoardWithCustomCard(t, "Card One", []provider.Label{{Name: "bug"}}, "body")
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter("Card One", []string{"bug", "newlabel"}, "body")

	msg := editorFinishedMsg{
		editedContent:   editedContent,
		originalContent: originalContent,
		card:            card,
	}
	m, _ := b.Update(msg)
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "newlabel") {
		t.Errorf("View() in labelConfirmMode should show unknown label name, got:\n%s", view)
	}
	if !strings.Contains(strings.ToLower(view), "create") || !strings.Contains(view, "y/n") {
		t.Errorf("View() in labelConfirmMode should show create prompt with y/n, got:\n%s", view)
	}
}
