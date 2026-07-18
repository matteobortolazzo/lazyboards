package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// PR status glyphs (#431) mirror the codebase's existing single-glyph +
// solid-color badge convention already established for cenci agent status
// badges (agentStatusSymbol/agentBadgeStyle in view.go). This file pins the
// pure derivation/styling functions in isolation; view_test.go,
// pr_list_test.go, and pr_picker_test.go cover the three rendered
// consumers.
//
// Four known states, derived from GitHub GraphQL fields isDraft/mergeable/
// mergeStateStatus:
//   - "draft"       <- IsDraft == true
//   - "mergeable"    <- Mergeable == "MERGEABLE"
//   - "conflicting"  <- Mergeable == "CONFLICTING" or MergeStateStatus == "DIRTY"
//   - "blocked"      <- MergeStateStatus in BLOCKED/BEHIND/UNSTABLE
//   - "unknown"      <- Mergeable == "UNKNOWN" (short-circuits before the
//     draft/blocked checks)

// --- prStatus: single-PR status derivation ---

func TestPRStatus_Draft(t *testing.T) {
	pr := LinkedPR{IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT"}
	if got := prStatus(pr); got != "draft" {
		t.Errorf("prStatus(draft PR) = %q, want %q", got, "draft")
	}
}

func TestPRStatus_Mergeable(t *testing.T) {
	pr := LinkedPR{IsDraft: false, Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"}
	if got := prStatus(pr); got != "mergeable" {
		t.Errorf("prStatus(clean PR) = %q, want %q", got, "mergeable")
	}
}

func TestPRStatus_Conflicting(t *testing.T) {
	pr := LinkedPR{IsDraft: false, Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"}
	if got := prStatus(pr); got != "conflicting" {
		t.Errorf("prStatus(conflicting PR) = %q, want %q", got, "conflicting")
	}
}

func TestPRStatus_Blocked(t *testing.T) {
	tests := []struct {
		name             string
		mergeStateStatus string
	}{
		{"blocked", "BLOCKED"},
		{"behind", "BEHIND"},
		{"unstable", "UNSTABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := LinkedPR{IsDraft: false, Mergeable: "MERGEABLE", MergeStateStatus: tt.mergeStateStatus}
			if got := prStatus(pr); got != "blocked" {
				t.Errorf("prStatus(mergeStateStatus=%s) = %q, want %q", tt.mergeStateStatus, got, "blocked")
			}
		})
	}
}

// TestPRStatus_MergeStateStatusDirty_ConflictingEvenIfMergeableStale asserts
// MergeStateStatus == "DIRTY" alone drives the "conflicting" result -- this
// pins the divergent case where Mergeable is stale ("MERGEABLE") but
// MergeStateStatus has already moved to DIRTY, so conflict detection must
// not depend solely on Mergeable == "CONFLICTING" agreeing.
func TestPRStatus_MergeStateStatusDirty_ConflictingEvenIfMergeableStale(t *testing.T) {
	pr := LinkedPR{IsDraft: false, Mergeable: "MERGEABLE", MergeStateStatus: "DIRTY"}
	if got := prStatus(pr); got != "conflicting" {
		t.Errorf("prStatus(mergeStateStatus=DIRTY, mergeable=MERGEABLE) = %q, want %q (DIRTY must not fall through to mergeable)", got, "conflicting")
	}
}

// TestPRStatus_MergeStateStatusHasHooks_FallsThroughToMergeable pins the
// intentional fallthrough: HAS_HOOKS ("mergeable with passing status and
// pre-receive hooks", per the githubv4 enum doc) is not a blocked-family
// state, so a PR with mergeable == MERGEABLE still reports "mergeable".
func TestPRStatus_MergeStateStatusHasHooks_FallsThroughToMergeable(t *testing.T) {
	pr := LinkedPR{IsDraft: false, Mergeable: "MERGEABLE", MergeStateStatus: "HAS_HOOKS"}
	if got := prStatus(pr); got != "mergeable" {
		t.Errorf("prStatus(mergeStateStatus=HAS_HOOKS, mergeable=MERGEABLE) = %q, want %q", got, "mergeable")
	}
}

func TestPRStatus_Unknown(t *testing.T) {
	pr := LinkedPR{IsDraft: false, Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"}
	if got := prStatus(pr); got != "unknown" {
		t.Errorf("prStatus(unresolved mergeability) = %q, want %q", got, "unknown")
	}
}

// TestPRStatus_UnknownMergeable_ShortCircuitsBeforeDraftCheck asserts
// mergeable == "UNKNOWN" wins over IsDraft: an unresolved mergeability
// calculation must not be misreported as "draft" just because the PR
// happens to also be a draft.
func TestPRStatus_UnknownMergeable_ShortCircuitsBeforeDraftCheck(t *testing.T) {
	pr := LinkedPR{IsDraft: true, Mergeable: "UNKNOWN", MergeStateStatus: "DRAFT"}
	if got := prStatus(pr); got != "unknown" {
		t.Errorf("prStatus(draft PR with UNKNOWN mergeable) = %q, want %q (UNKNOWN short-circuits before the draft check)", got, "unknown")
	}
}

// TestPRStatus_DraftWinsOverMergeStateStatusDraft asserts draft derivation
// comes from IsDraft, not from checking mergeStateStatus == "DRAFT" -- the
// ticket's stated reason for using IsDraft is to avoid overlap/conflict
// with a redundant mergeStateStatus signal.
func TestPRStatus_DraftWinsOverMergeStateStatusDraft(t *testing.T) {
	pr := LinkedPR{IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT"}
	if got := prStatus(pr); got != "draft" {
		t.Errorf("prStatus(isDraft=true, mergeStateStatus=DRAFT) = %q, want %q", got, "draft")
	}
}

// TestPRStatus_DraftWinsOverBlockedMergeStateStatus asserts isDraft is
// checked before mergeStateStatus's blocked states: a draft PR whose
// mergeStateStatus happens to also be BLOCKED still reports as draft
// regardless of mergeStateStatus's value.
func TestPRStatus_DraftWinsOverBlockedMergeStateStatus(t *testing.T) {
	pr := LinkedPR{IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED"}
	if got := prStatus(pr); got != "draft" {
		t.Errorf("prStatus(isDraft=true, mergeStateStatus=BLOCKED) = %q, want %q (draft checked before mergeStateStatus)", got, "draft")
	}
}

// --- prStatusSymbol: glyph per state ---

func TestPRStatusSymbol_AllKnownStates(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"draft", "●"},       // ●
		{"mergeable", "✓"},   // ✓
		{"conflicting", "✗"}, // ✗
		{"blocked", "!"},
	}
	for _, tt := range tests {
		if got := prStatusSymbol(tt.status); got != tt.want {
			t.Errorf("prStatusSymbol(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// TestPRStatusSymbol_Unknown_ReturnsEmptyString mirrors
// agentStatusSymbol's "blank for idle/unknown" convention.
func TestPRStatusSymbol_Unknown_ReturnsEmptyString(t *testing.T) {
	if got := prStatusSymbol("unknown"); got != "" {
		t.Errorf("prStatusSymbol(unknown) = %q, want empty string (no glyph)", got)
	}
}

// --- prStatusStyle: solid color per state ---

func TestPRStatusStyle_MapsEachKnownStatusToItsNamedStyle(t *testing.T) {
	tests := []struct {
		status string
		want   lipgloss.Style
	}{
		{"draft", prDraftStyle},
		{"mergeable", prMergeableStyle},
		{"conflicting", prConflictingStyle},
		{"blocked", prBlockedStyle},
	}
	for _, tt := range tests {
		got := prStatusStyle(tt.status).Render("x")
		want := tt.want.Render("x")
		if got != want {
			t.Errorf("prStatusStyle(%q).Render(x) = %q, want %q (its dedicated style)", tt.status, got, want)
		}
	}
}
