package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// sendWizardKey sends a message through the wizard's Update and returns the updated ConfigWizard.
func sendWizardKey(t *testing.T, w ConfigWizard, msg tea.Msg) ConfigWizard {
	t.Helper()
	m, _ := w.Update(msg)
	return m.(ConfigWizard)
}

// --- Step Navigation Tests ---

func TestWizard_StartsAtProviderStep(t *testing.T) {
	w := NewConfigWizard("", "")
	if w.step != providerStep {
		t.Errorf("step = %d, want %d (providerStep)", w.step, providerStep)
	}
}

func TestWizard_EnterOnProviderStep_AdvancesToRepoStep(t *testing.T) {
	w := NewConfigWizard("", "")
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.step != repoStep {
		t.Errorf("step = %d, want %d (repoStep)", w.step, repoStep)
	}
}

func TestWizard_EnterOnRepoStep_WithValidRepo_AdvancesToDoneStep(t *testing.T) {
	w := NewConfigWizard("", "owner/repo")
	// Advance to repoStep first.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Press Enter with valid repo already filled.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.step != doneStep {
		t.Errorf("step = %d, want %d (doneStep)", w.step, doneStep)
	}
}

func TestWizard_EnterOnRepoStep_WithInvalidRepo_StaysOnRepoStep(t *testing.T) {
	w := NewConfigWizard("", "")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Press Enter with empty repo (invalid).
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.step != repoStep {
		t.Errorf("step = %d, want %d (repoStep)", w.step, repoStep)
	}
	if w.repoErr == "" {
		t.Error("repoErr should be set when repo format is invalid")
	}
}

// --- Provider Selection Tests ---

func TestWizard_ProviderStep_JMovesDown(t *testing.T) {
	w := NewConfigWizard("", "")
	// With only 1 provider, j should clamp at index 0.
	w = sendWizardKey(t, w, keyMsg("j"))
	if w.providerIdx != 0 {
		t.Errorf("providerIdx = %d after j with single provider, want 0 (clamped)", w.providerIdx)
	}
}

func TestWizard_ProviderStep_DefaultsToGithub(t *testing.T) {
	w := NewConfigWizard("", "")
	if len(w.providers) == 0 {
		t.Fatal("providers list is empty, want at least one")
	}
	if w.providers[w.providerIdx] != "github" {
		t.Errorf("selected provider = %q, want %q", w.providers[w.providerIdx], "github")
	}
}

// --- Repo Validation Tests ---

func TestWizard_RepoValidation_AcceptsValidFormat(t *testing.T) {
	w := NewConfigWizard("", "owner/repo")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Submit.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.repoErr != "" {
		t.Errorf("repoErr = %q for valid repo, want empty string", w.repoErr)
	}
}

func TestWizard_RepoValidation_RejectsEmptyString(t *testing.T) {
	w := NewConfigWizard("", "")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Submit with empty input.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.repoErr == "" {
		t.Error("repoErr should be set for empty repo string")
	}
}

func TestWizard_RepoValidation_RejectsNoSlash(t *testing.T) {
	w := NewConfigWizard("", "")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Type "noslash" into the repo input.
	for _, ch := range "noslash" {
		w = sendWizardKey(t, w, keyMsg(string(ch)))
	}
	// Submit.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.repoErr == "" {
		t.Error("repoErr should be set for repo without slash")
	}
}

func TestWizard_RepoValidation_RejectsEmptyOwner(t *testing.T) {
	w := NewConfigWizard("", "")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Type "/repo".
	for _, ch := range "/repo" {
		w = sendWizardKey(t, w, keyMsg(string(ch)))
	}
	// Submit.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.repoErr == "" {
		t.Error("repoErr should be set for empty owner")
	}
}

func TestWizard_RepoValidation_RejectsEmptyRepo(t *testing.T) {
	w := NewConfigWizard("", "")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Type "owner/".
	for _, ch := range "owner/" {
		w = sendWizardKey(t, w, keyMsg(string(ch)))
	}
	// Submit.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.repoErr == "" {
		t.Error("repoErr should be set for empty repo name")
	}
}

// --- Quit Tests ---

func TestWizard_Esc_ReturnsQuitCmd(t *testing.T) {
	w := NewConfigWizard("", "")
	_, cmd := w.Update(arrowMsg(tea.KeyEsc))
	if cmd == nil {
		t.Error("Esc should return a non-nil cmd (wizardQuitMsg)")
	}
}

func TestWizard_CtrlC_ReturnsQuitCmd(t *testing.T) {
	w := NewConfigWizard("", "")
	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C should return a non-nil cmd (wizardQuitMsg)")
	}
}

// --- Pre-fill Tests ---

func TestWizard_PreFillProvider_SelectsCorrectProvider(t *testing.T) {
	w := NewConfigWizard("github", "")
	if w.providers[w.providerIdx] != "github" {
		t.Errorf("selected provider = %q, want %q", w.providers[w.providerIdx], "github")
	}
}

func TestWizard_PreFillRepo_PopulatesRepoInput(t *testing.T) {
	w := NewConfigWizard("", "owner/repo")
	if w.repoInput.Value() != "owner/repo" {
		t.Errorf("repoInput.Value() = %q, want %q", w.repoInput.Value(), "owner/repo")
	}
}

// --- View Tests ---

func TestWizard_View_ProviderStep_ShowsProviderList(t *testing.T) {
	w := NewConfigWizard("", "")
	w.width = 80
	w.height = 24
	view := w.View()
	if !strings.Contains(view, "Provider") {
		t.Error("View() at providerStep should contain 'Provider'")
	}
	if !strings.Contains(view, "github") {
		t.Error("View() at providerStep should contain 'github'")
	}
}

func TestWizard_View_RepoStep_ShowsRepoInput(t *testing.T) {
	w := NewConfigWizard("", "")
	w.width = 80
	w.height = 24
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	view := w.View()
	if !strings.Contains(view, "Repository") {
		t.Error("View() at repoStep should contain 'Repository'")
	}
}

func TestWizard_View_ShowsHelpBar(t *testing.T) {
	w := NewConfigWizard("", "")
	w.width = 80
	w.height = 24
	view := w.View()
	if !strings.Contains(view, "esc") {
		t.Error("View() should contain 'esc' in help bar")
	}
}

// --- Error Clearing Test ---

func TestWizard_RepoStep_TypingClearsError(t *testing.T) {
	w := NewConfigWizard("", "")
	// Advance to repoStep.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	// Submit to trigger error.
	w = sendWizardKey(t, w, arrowMsg(tea.KeyEnter))
	if w.repoErr == "" {
		t.Fatal("expected repoErr to be set after empty submit")
	}
	// Type a character to clear error.
	w = sendWizardKey(t, w, keyMsg("a"))
	if w.repoErr != "" {
		t.Errorf("repoErr = %q after typing, want empty string (error should clear)", w.repoErr)
	}
}
