package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestBoard_PRCounts_SumsAcrossColumnsAndCards(t *testing.T) {
	cols := []provider.Column{
		{Title: "New", Cards: []provider.Card{
			{Number: 1, Title: "No PRs"},
			{Number: 2, Title: "One PR", LinkedPRs: []provider.LinkedPR{
				{Number: 10, URL: "https://x/10"},
			}},
		}},
		{Title: "Done", Cards: []provider.Card{
			{Number: 3, Title: "Two PRs", LinkedPRs: []provider.LinkedPR{
				{Number: 20, URL: "https://x/20"},
				{Number: 21, URL: "https://x/21"},
			}},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, cols)

	if got := b.prCounts(); got != 3 {
		t.Errorf("prCounts() = %d, want 3 (summed across both columns and all cards)", got)
	}
}

func TestBoard_PRCounts_ZeroWhenNoLinkedPRs(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "No PRs"},
		{Number: 2, Title: "Still none"},
	}, 120, 40)

	if got := b.prCounts(); got != 0 {
		t.Errorf("prCounts() = %d, want 0", got)
	}
}

func TestBoard_View_ShowsPRCountIndicator(t *testing.T) {
	// The shared fixture has 3 linked PRs across its cards.
	b := newBoardWithPRs(t)

	// Assert on the glyph-immediately-followed-by-count token: the per-card PR
	// indicator also renders the bare glyph, but only the status-bar count
	// renders it contiguously followed by the aggregated number.
	view := b.View()
	if !strings.Contains(view, linkedPRGlyph+"3") {
		t.Errorf("View() should show the aggregated PR count next to the glyph (%q3)", linkedPRGlyph)
	}
}

func TestBoard_View_OmitsPRCountWhenZero(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "No PRs"},
	}, 120, 40)

	if strings.Contains(b.View(), linkedPRGlyph) {
		t.Errorf("View() should not contain the PR count glyph when no card has a linked PR")
	}
}

// --- Repo-wide open-PR indicator (#v modal and status bar must agree: the
// count covers every open PR, not only card-linked ones) ---

// TestBoard_PRIndicatorCount_FallsBackToLinkedSumBeforeRepoWideFetch verifies
// the indicator mirrors prListState's fallback precedence: until a repo-wide
// open-PR listing has succeeded, the card-linked sum is shown.
func TestBoard_PRIndicatorCount_FallsBackToLinkedSumBeforeRepoWideFetch(t *testing.T) {
	// newBoardWithPRs loads via a boardFetchedMsg carrying no repo-wide
	// open-PR result; its fixture has 3 card-linked PRs.
	b := newBoardWithPRs(t)

	if got := b.prIndicatorCount(); got != 3 {
		t.Errorf("prIndicatorCount() = %d, want 3 (card-linked fallback before any repo-wide fetch)", got)
	}
}

// TestBoard_PRIndicatorCount_UsesRepoWideTotalFromBoardFetch verifies a board
// fetch carrying a successful repo-wide open-PR listing switches the
// indicator to that total, even when it exceeds the card-linked sum, and that
// the status bar renders it.
func TestBoard_PRIndicatorCount_UsesRepoWideTotalFromBoardFetch(t *testing.T) {
	b := newBoardWithPRs(t) // 3 card-linked PRs

	openPRs := []provider.LinkedPR{
		{Number: 20, Title: "feat: linked", URL: "https://x/20"},
		{Number: 30, Title: "docs: linked", URL: "https://x/30"},
		{Number: 31, Title: "docs: linked too", URL: "https://x/31"},
		{Number: 40, Title: "chore: unlinked", URL: "https://x/40"},
		{Number: 41, Title: "chore: unlinked too", URL: "https://x/41"},
	}
	b = sendKey(t, b, boardFetchedMsg{
		board:          provider.Board{Columns: prFixtureColumns()},
		openPRs:        openPRs,
		openPRsFetched: true,
	})

	if got := b.prIndicatorCount(); got != len(openPRs) {
		t.Errorf("prIndicatorCount() = %d, want %d (repo-wide open-PR total)", got, len(openPRs))
	}
	wantToken := linkedPRGlyph + strconv.Itoa(len(openPRs))
	if !strings.Contains(b.View(), wantToken) {
		t.Errorf("View() should show the repo-wide open-PR total in the status bar (%q)", wantToken)
	}
}

// TestBoard_PRIndicatorCount_ZeroOpenPRsHidesIndicator verifies a successful
// repo-wide listing with no open PRs zeroes the indicator even while cards
// still carry linked PRs — the v modal and the indicator must agree.
func TestBoard_PRIndicatorCount_ZeroOpenPRsHidesIndicator(t *testing.T) {
	b := newBoardWithPRs(t)
	b = sendKey(t, b, boardFetchedMsg{
		board:          provider.Board{Columns: prFixtureColumns()},
		openPRsFetched: true,
	})

	if got := b.prIndicatorCount(); got != 0 {
		t.Errorf("prIndicatorCount() = %d, want 0 (successful listing found no open PRs)", got)
	}
	view := b.View()
	if strings.Contains(view, linkedPRGlyph+"0") || strings.Contains(view, linkedPRGlyph+"3") {
		t.Error("View() should omit the status-bar PR token entirely when the repo has no open PRs")
	}
}

// TestBoard_PRIndicatorCount_KeepsLastKnownTotalWhenFetchFails verifies a
// later board fetch whose open-PR listing failed (openPRsFetched=false) keeps
// the previous repo-wide total instead of clobbering it or falling back to
// the linked sum — same non-fatal treatment as collaborators/labels.
func TestBoard_PRIndicatorCount_KeepsLastKnownTotalWhenFetchFails(t *testing.T) {
	b := newBoardWithPRs(t)
	b = sendKey(t, b, boardFetchedMsg{
		board:          provider.Board{Columns: prFixtureColumns()},
		openPRs:        []provider.LinkedPR{{Number: 40, Title: "chore: unlinked", URL: "https://x/40"}},
		openPRsFetched: true,
	})

	b = sendKey(t, b, boardFetchedMsg{board: provider.Board{Columns: prFixtureColumns()}})

	if got := b.prIndicatorCount(); got != 1 {
		t.Errorf("prIndicatorCount() = %d, want 1 (last known repo-wide total kept)", got)
	}
}

// TestFetchBoardCmd_IncludesRepoWideOpenPRs verifies both fetch modes (with
// and without metadata) carry the provider's open-PR listing in the resulting
// boardFetchedMsg, so the indicator refreshes on every board fetch cycle. The
// expected rows come from the same provider call the message embeds — a real
// observed sample of the producer's output, not a value duplicated here.
func TestFetchBoardCmd_IncludesRepoWideOpenPRs(t *testing.T) {
	for _, includeMetadata := range []bool{true, false} {
		p := provider.NewFakeProvider()
		want, err := p.ListOpenPRs(context.Background())
		if err != nil {
			t.Fatalf("ListOpenPRs: %v", err)
		}
		if len(want) == 0 {
			t.Fatal("test setup: FakeProvider should list at least one open PR")
		}

		msg := fetchBoardCmd(p, includeMetadata)()
		bf, ok := msg.(boardFetchedMsg)
		if !ok {
			t.Fatalf("includeMetadata=%v: fetchBoardCmd returned %T, want boardFetchedMsg", includeMetadata, msg)
		}
		if !bf.openPRsFetched {
			t.Errorf("includeMetadata=%v: openPRsFetched = false, want true on a successful listing", includeMetadata)
		}
		if len(bf.openPRs) != len(want) {
			t.Errorf("includeMetadata=%v: openPRs has %d rows, want %d (provider's full open-PR list)", includeMetadata, len(bf.openPRs), len(want))
		}
	}
}

// TestBoard_PRIndicatorCount_RefreshedByPRListFetch verifies the repo-wide
// listing fetched for the v modal also refreshes the indicator — including a
// result landing after the modal closed, as long as it belongs to the current
// (latest) request generation — while a stale-generation result or an error
// never changes the count.
func TestBoard_PRIndicatorCount_RefreshedByPRListFetch(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t) // 3 card-linked PRs
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: unlinked", URL: "https://x/40"},
	}})

	if got := b.prIndicatorCount(); got != 1 {
		t.Errorf("prIndicatorCount() = %d, want 1 (modal fetch result adopted)", got)
	}

	// A result landing after Esc (same, still-latest generation) still
	// refreshes the count: it's fresh repo-wide data regardless of the modal.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	b = sendKey(t, b, keyMsg("v"))
	gen := b.prList.generation
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	b = sendKey(t, b, openPRsMsg{generation: gen, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: unlinked", URL: "https://x/40"},
		{Number: 41, Title: "chore: another", URL: "https://x/41"},
	}})
	if got := b.prIndicatorCount(); got != 2 {
		t.Errorf("prIndicatorCount() = %d, want 2 (late same-generation result adopted)", got)
	}

	// A stale-generation result (superseded by a newer request) is dropped.
	b = sendKey(t, b, keyMsg("v")) // bumps the generation past gen
	b = sendKey(t, b, openPRsMsg{generation: gen, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: unlinked", URL: "https://x/40"},
		{Number: 41, Title: "chore: another", URL: "https://x/41"},
		{Number: 42, Title: "chore: third", URL: "https://x/42"},
	}})
	if got := b.prIndicatorCount(); got != 2 {
		t.Errorf("prIndicatorCount() = %d, want 2 (stale-generation result must be dropped)", got)
	}

	// An errored fetch never changes the count.
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, err: errors.New("boom")})
	if got := b.prIndicatorCount(); got != 2 {
		t.Errorf("prIndicatorCount() = %d, want 2 (errored fetch must not change the count)", got)
	}
}
