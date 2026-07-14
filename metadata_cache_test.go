package main

import (
	"testing"
	"time"

	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- metadataDue() unit tests ---

func TestMetadataDue_NeverFetched_ReturnsTrue(t *testing.T) {
	b := newTestBoard(t)
	b.metadataTTL = 30 * time.Minute
	// lastMetadataFetch is left at its zero value (never fetched).

	if !b.metadataDue() {
		t.Error("metadataDue() should be true when lastMetadataFetch is zero (never fetched)")
	}
}

func TestMetadataDue_WithinTTL_ReturnsFalse(t *testing.T) {
	b := newTestBoard(t)
	b.metadataTTL = 30 * time.Minute
	b.lastMetadataFetch = time.Now()

	if b.metadataDue() {
		t.Error("metadataDue() should be false when the TTL has not elapsed since the last fetch")
	}
}

func TestMetadataDue_TTLElapsed_ReturnsTrue(t *testing.T) {
	b := newTestBoard(t)
	b.metadataTTL = 30 * time.Minute
	b.lastMetadataFetch = time.Now().Add(-31 * time.Minute)

	if !b.metadataDue() {
		t.Error("metadataDue() should be true once the TTL has elapsed since the last fetch")
	}
}

// --- Integration: refresh tick skips metadata calls when not due ---

// TestMetadataCache_RefreshTick_SkipsMetadataCallsWhenNotDue exercises a full
// refresh cycle (refreshTickMsg -> fetchBoardCmd -> boardFetchedMsg) right
// after an initial metadata fetch, when the metadata TTL has not yet
// elapsed. It asserts that (a) previously-known collaborators/labels/user
// survive the cycle untouched, and (b) the provider's metadata-fetching
// methods were not invoked during this cycle.
func TestMetadataCache_RefreshTick_SkipsMetadataCallsWhenNotDue(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 5*time.Minute, 0, "Working", false, false, nil, nil)
	b.Width = 120
	b.Height = 40

	// Initial full fetch (mirrors Init(), which always includes metadata) to
	// populate collaborators/labels/user and to set lastMetadataFetch.
	initMsgs := collectMsgs(fetchBoardCmd(p, true))
	var initial boardFetchedMsg
	found := false
	for _, msg := range initMsgs {
		if fm, ok := msg.(boardFetchedMsg); ok {
			initial = fm
			found = true
		}
	}
	if !found {
		t.Fatal("expected initial fetchBoardCmd to produce a boardFetchedMsg")
	}

	m, _ := b.Update(initial)
	b = m.(Board)

	if len(b.collaborators) == 0 || b.authenticatedUser == "" || len(b.repoLabels) == 0 {
		t.Fatal("precondition: initial fetch should populate collaborators/user/labels")
	}
	knownCollaboratorCount := len(b.collaborators)
	knownLabelCount := len(b.repoLabels)
	knownUser := b.authenticatedUser

	collabCallsBefore := p.FetchCollaboratorsCalls
	userCallsBefore := p.GetAuthenticatedUserCalls
	labelCallsBefore := p.ListLabelsCalls

	// Trigger a refresh tick immediately -- the metadata TTL has not elapsed,
	// so this cycle must NOT re-fetch collaborators/labels/user.
	m, cmd := b.Update(refreshTickMsg{})
	b = m.(Board)
	if !b.refreshing {
		t.Fatal("precondition: refreshing should be true after refreshTickMsg")
	}

	tickMsgs := collectMsgs(cmd)
	var refreshed boardFetchedMsg
	found = false
	for _, msg := range tickMsgs {
		if fm, ok := msg.(boardFetchedMsg); ok {
			refreshed = fm
			found = true
		}
	}
	if !found {
		t.Fatal("expected refresh tick's fetchBoardCmd to produce a boardFetchedMsg")
	}

	m, _ = b.Update(refreshed)
	b = m.(Board)

	// (a) previously-known metadata must survive an untouched fetch cycle.
	if len(b.collaborators) != knownCollaboratorCount {
		t.Errorf("collaborators wiped after metadata-skipped refresh: got %d, want %d", len(b.collaborators), knownCollaboratorCount)
	}
	if b.authenticatedUser != knownUser {
		t.Errorf("authenticatedUser wiped after metadata-skipped refresh: got %q, want %q", b.authenticatedUser, knownUser)
	}
	if len(b.repoLabels) != knownLabelCount {
		t.Errorf("repoLabels wiped after metadata-skipped refresh: got %d, want %d", len(b.repoLabels), knownLabelCount)
	}

	// (b) the provider's metadata-fetching methods must not have been called
	// during this cycle. This call-count assertion is a legitimate behavior
	// guard, not an implementation-detail check (see testing.md's "NEVER
	// assert call counts" -- this is the documented exception): it verifies
	// the TTL gate actually prevented the three redundant metadata network
	// calls, which is the entire point of this feature.
	if p.FetchCollaboratorsCalls != collabCallsBefore {
		t.Errorf("FetchCollaborators called during metadata-skipped refresh cycle: %d calls, want 0", p.FetchCollaboratorsCalls-collabCallsBefore)
	}
	if p.GetAuthenticatedUserCalls != userCallsBefore {
		t.Errorf("GetAuthenticatedUser called during metadata-skipped refresh cycle: %d calls, want 0", p.GetAuthenticatedUserCalls-userCallsBefore)
	}
	if p.ListLabelsCalls != labelCallsBefore {
		t.Errorf("ListLabels called during metadata-skipped refresh cycle: %d calls, want 0", p.ListLabelsCalls-labelCallsBefore)
	}
}

// --- Integration: manual refresh always bypasses the TTL ---

// TestMetadataCache_ManualRefresh_BypassesTTL exercises a full refresh cycle
// triggered by the manual 'r' key (handleNormalModeKey) right after an
// initial metadata fetch, while the metadata TTL has NOT yet elapsed (i.e.
// metadataDue() would return false). It asserts that the manual refresh
// path unconditionally requests metadata rather than deferring to
// metadataDue(), so collaborators/authenticated user/repo labels are all
// re-fetched even though a periodic refresh at the same moment would have
// skipped them (see TestMetadataCache_RefreshTick_SkipsMetadataCallsWhenNotDue
// above).
func TestMetadataCache_ManualRefresh_BypassesTTL(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 5*time.Minute, 0, "Working", false, false, nil, nil)
	b.Width = 120
	b.Height = 40

	// Initial full fetch (mirrors Init(), which always includes metadata) to
	// populate collaborators/labels/user and to set lastMetadataFetch.
	initMsgs := collectMsgs(fetchBoardCmd(p, true))
	var initial boardFetchedMsg
	found := false
	for _, msg := range initMsgs {
		if fm, ok := msg.(boardFetchedMsg); ok {
			initial = fm
			found = true
		}
	}
	if !found {
		t.Fatal("expected initial fetchBoardCmd to produce a boardFetchedMsg")
	}

	m, _ := b.Update(initial)
	b = m.(Board)

	if len(b.collaborators) == 0 || b.authenticatedUser == "" || len(b.repoLabels) == 0 {
		t.Fatal("precondition: initial fetch should populate collaborators/user/labels")
	}
	// Precondition: the TTL has not elapsed immediately after the fetch above,
	// so a periodic (non-manual) refresh right now would skip metadata.
	if b.metadataDue() {
		t.Fatal("precondition: metadataDue() should be false immediately after an initial fetch")
	}

	collabCallsBefore := p.FetchCollaboratorsCalls
	userCallsBefore := p.GetAuthenticatedUserCalls
	labelCallsBefore := p.ListLabelsCalls

	// Trigger a manual refresh via the 'r' key. Unlike the periodic tick path,
	// manual refresh must always request metadata, ignoring metadataDue().
	m, cmd := b.Update(keyMsg("r"))
	b = m.(Board)
	if !b.refreshing {
		t.Fatal("precondition: refreshing should be true after 'r'")
	}

	rMsgs := collectMsgs(cmd)
	var refreshed boardFetchedMsg
	found = false
	for _, msg := range rMsgs {
		if fm, ok := msg.(boardFetchedMsg); ok {
			refreshed = fm
			found = true
		}
	}
	if !found {
		t.Fatal("expected manual refresh's fetchBoardCmd to produce a boardFetchedMsg")
	}

	m, _ = b.Update(refreshed)
	b = m.(Board)

	// The provider's metadata-fetching methods must have been called exactly
	// once each during this cycle, despite metadataDue() being false. This
	// call-count assertion is a legitimate behavior guard, not an
	// implementation-detail check (see testing.md's "NEVER assert call
	// counts" -- this is the documented exception): it verifies manual
	// refresh forces includeMetadata=true rather than routing through
	// b.metadataDue() like the periodic tick path does.
	if p.FetchCollaboratorsCalls != collabCallsBefore+1 {
		t.Errorf("FetchCollaborators calls during manual refresh = %d, want %d (manual refresh must bypass the TTL)", p.FetchCollaboratorsCalls-collabCallsBefore, 1)
	}
	if p.GetAuthenticatedUserCalls != userCallsBefore+1 {
		t.Errorf("GetAuthenticatedUser calls during manual refresh = %d, want %d (manual refresh must bypass the TTL)", p.GetAuthenticatedUserCalls-userCallsBefore, 1)
	}
	if p.ListLabelsCalls != labelCallsBefore+1 {
		t.Errorf("ListLabels calls during manual refresh = %d, want %d (manual refresh must bypass the TTL)", p.ListLabelsCalls-labelCallsBefore, 1)
	}
}
