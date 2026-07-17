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
//     draft/blocked checks; never wins over a known status in worstPRStatus)

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

// --- worstPRStatus: worst-wins priority across a card's linked PRs ---
// Priority: Conflicting > Blocked > Draft > Mergeable. UNKNOWN never wins
// over a known status and is the fallback when every linked PR is UNKNOWN.

// prWithStatus builds a LinkedPR whose fields derive the given prStatus
// value, so worstPRStatus tests can be expressed in terms of status names
// instead of duplicating field combinations already pinned by TestPRStatus_*.
func prWithStatus(number int, status string) LinkedPR {
	switch status {
	case "draft":
		return LinkedPR{Number: number, IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT"}
	case "mergeable":
		return LinkedPR{Number: number, IsDraft: false, Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"}
	case "conflicting":
		return LinkedPR{Number: number, IsDraft: false, Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"}
	case "blocked":
		return LinkedPR{Number: number, IsDraft: false, Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED"}
	default:
		return LinkedPR{Number: number, IsDraft: false, Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"}
	}
}

func TestWorstPRStatus_SingleKnownStates(t *testing.T) {
	for _, status := range []string{"draft", "mergeable", "conflicting", "blocked"} {
		got := worstPRStatus([]LinkedPR{prWithStatus(1, status)})
		if got != status {
			t.Errorf("worstPRStatus([%s]) = %q, want %q", status, got, status)
		}
	}
}

func TestWorstPRStatus_PriorityOrdering(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"conflicting beats blocked", []string{"conflicting", "blocked"}, "conflicting"},
		{"blocked beats draft", []string{"blocked", "draft"}, "blocked"},
		{"draft beats mergeable", []string{"draft", "mergeable"}, "draft"},
		{"conflicting beats everything", []string{"mergeable", "draft", "blocked", "conflicting"}, "conflicting"},
		{"order in the slice does not matter", []string{"conflicting", "mergeable", "blocked", "draft"}, "conflicting"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var prs []LinkedPR
			for i, s := range tt.statuses {
				prs = append(prs, prWithStatus(i+1, s))
			}
			if got := worstPRStatus(prs); got != tt.want {
				t.Errorf("worstPRStatus(%v) = %q, want %q", tt.statuses, got, tt.want)
			}
		})
	}
}

// TestWorstPRStatus_UnknownDoesNotMaskKnownBadState asserts UNKNOWN never
// wins the "worst wins" comparison over a known status from another linked
// PR on the same card.
func TestWorstPRStatus_UnknownDoesNotMaskKnownBadState(t *testing.T) {
	prs := []LinkedPR{prWithStatus(1, "unknown"), prWithStatus(2, "conflicting")}
	if got := worstPRStatus(prs); got != "conflicting" {
		t.Errorf("worstPRStatus([unknown, conflicting]) = %q, want %q (unknown must not mask a known bad state)", got, "conflicting")
	}
}

// TestWorstPRStatus_AllUnknown_ReturnsUnknown asserts an all-UNKNOWN slice
// returns "unknown" rather than fabricating a known state.
func TestWorstPRStatus_AllUnknown_ReturnsUnknown(t *testing.T) {
	prs := []LinkedPR{prWithStatus(1, "unknown"), prWithStatus(2, "unknown")}
	if got := worstPRStatus(prs); got != "unknown" {
		t.Errorf("worstPRStatus(all unknown) = %q, want %q (must not fabricate a known state)", got, "unknown")
	}
}
