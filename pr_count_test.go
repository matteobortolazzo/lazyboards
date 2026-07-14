package main

import (
	"strings"
	"testing"

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
