package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// maxRefDigits bounds the accepted digit-run length for a #N reference.
// Real GitHub issue/PR numbers never come close to this many digits; the
// bound exists to guarantee strconv.Atoi cannot overflow (see scanRefs).
const maxRefDigits = 10

// cardRef is a distinct #N reference found in a card body, assigned a
// stable single-letter label ('a'-'z') in first-appearance order. Body refs
// leave URL and Repo empty. A blocker-derived ref carries them only when the
// blocker is cross-repo: a non-empty URL means an explicit foreign target
// that resolveReference must open directly, bypassing findCard entirely.
type cardRef struct {
	Number int
	Label  rune
	URL    string
	Repo   string
}

// refMatch is an internal record of a validated #N match: end is the byte
// offset immediately after the match in the original body string, and
// number is the parsed reference number.
type refMatch struct {
	end, number int
}

var refPattern = regexp.MustCompile(`#\d+`)

// isWordByte reports whether b should be treated as part of a "word" for
// reference boundary checks: ASCII letters, digits, underscore, or any
// byte >= 0x80 (so multibyte UTF-8 sequences are never mistaken for word
// boundaries).
func isWordByte(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b == '_':
		return true
	case b >= 0x80:
		return true
	default:
		return false
	}
}

// scanRefs finds all valid #N references in body: the digit run must not
// have a leading zero (rejects #0 and #007), and both the leading and
// trailing bytes adjacent to the match must not be word bytes (rejects
// abc#12 and #12abc). Detection is regex-only and code-block-agnostic.
func scanRefs(body string) []refMatch {
	var matches []refMatch
	for _, loc := range refPattern.FindAllStringIndex(body, -1) {
		start, end := loc[0], loc[1]

		// Reject leading zero / bare zero: digit run starts at start+1.
		if body[start+1] == '0' {
			continue
		}

		if start > 0 && isWordByte(body[start-1]) {
			continue
		}
		if end < len(body) && isWordByte(body[end]) {
			continue
		}

		// Reject pathologically long digit runs (e.g. untrusted body text
		// containing "#99999999999999999999999999999"): the regex only
		// guarantees a non-empty digit run, not a bounded one, and an
		// unbounded run can overflow strconv.Atoi (returning ErrRange plus
		// a silently-clamped MaxInt/MinInt). Capping the digit-run length
		// here, before calling Atoi, guarantees the call below cannot fail.
		if end-(start+1) > maxRefDigits {
			continue
		}

		// The digit-run length is bounded above by the check just above,
		// so this can never overflow or otherwise fail to parse.
		number, _ := strconv.Atoi(body[start+1 : end])

		matches = append(matches, refMatch{end: end, number: number})
	}
	return matches
}

// parseCardRefs returns the ordered, de-duplicated number -> label mapping
// for a card body: each distinct number is assigned the next unused label
// starting at 'a', in first-appearance order, capped at 26 distinct
// numbers ('a' to 'z'). Numbers beyond the 26th distinct one are omitted
// entirely.
func parseCardRefs(body string) []cardRef {
	var refs []cardRef
	seen := make(map[int]bool)
	nextLabel := 'a'

	for _, m := range scanRefs(body) {
		if seen[m.number] {
			continue
		}
		if nextLabel > 'z' {
			break
		}
		seen[m.number] = true
		refs = append(refs, cardRef{Number: m.number, Label: nextLabel})
		nextLabel++
	}

	return refs
}

// annotateBodyRefs inserts a persistent " [x]" label after every
// occurrence of a #N reference whose number has an assigned label,
// leaving references beyond the 26-distinct cap untouched.
func annotateBodyRefs(body string) string {
	refs := parseCardRefs(body)
	labelByNumber := make(map[int]rune, len(refs))
	for _, r := range refs {
		labelByNumber[r.Number] = r.Label
	}

	matches := scanRefs(body)

	// Splice in reverse order so earlier insertions don't shift the byte
	// offsets of matches still to be processed.
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		label, ok := labelByNumber[m.number]
		if !ok {
			continue
		}
		// Brackets are backslash-escaped so the insertion can never combine
		// with immediately adjacent text (e.g. a following "(text)" with no
		// space) into markdown link/image syntax when rendered by glamour.
		// Mirrors escapeMarkdown in view.go.
		insertion := " \\[" + string(label) + "\\]"
		body = body[:m.end] + insertion + body[m.end:]
	}

	return body
}

// trailingNumberPattern matches the trailing digit run of an issue/PR URL
// (e.g. the "463" in ".../issues/463"), used by refIssueURL to construct a
// same-repo URL for a referenced number.
var trailingNumberPattern = regexp.MustCompile(`\d+$`)

// refIssueURL derives a same-repo issue/PR URL for number by replacing the
// trailing digit run of cardURL (the selected card's own URL) with number.
// This is provider-agnostic (no hardcoded host, no reuse of the board's
// configured repoOwner/repoName): whatever host and path style the card's
// own URL uses is preserved. GitHub redirects /issues/N to /pull/N when N is
// actually a PR, so referenced PRs still resolve correctly.
//
// ok is false when cardURL is empty or has no trailing digit run to replace
// (ReplaceAllString would be a no-op, silently returning the wrong URL
// unchanged) -- callers must treat that as "URL not available" rather than
// opening the returned string.
func refIssueURL(cardURL string, number int) (string, bool) {
	if cardURL == "" || !trailingNumberPattern.MatchString(cardURL) {
		return "", false
	}
	return trailingNumberPattern.ReplaceAllString(cardURL, strconv.Itoa(number)), true
}

// cardReferences returns card's full set of g r reference-navigation
// targets: the #N references parsed from its body, followed by refs derived
// from its open blockers (see appendBlockerRefs). This is the single source
// handleReferenceNavKey consumes; annotateBodyRefs deliberately keeps
// calling parseCardRefs directly so blocker labels never leak into rendered
// body text.
func (b Board) cardReferences(card Card) []cardRef {
	refs := parseCardRefs(card.Body)
	ownRepo := ""
	if b.repoOwner != "" && b.repoName != "" {
		ownRepo = b.repoOwner + "/" + b.repoName
	}
	return appendBlockerRefs(refs, card.Blockers, ownRepo)
}

// appendBlockerRefs appends card's open blockers to refs (already-parsed
// body refs) as additional reference-navigation targets: closed blockers are
// excluded, a same-repo blocker already present in refs (by number) is
// deduped, foreign blockers dedup among themselves by URL (not number, since
// the same number in two different repos is a distinct target), and a
// foreign blocker with no URL is omitted entirely (unreachable target).
// Labels continue from the last existing ref's label + 1, and the 26-label
// cap is enforced by omitting any ref beyond it entirely -- never truncating
// onto 'z' (parseCardRefs' contract).
func appendBlockerRefs(refs []cardRef, blockers []Blocker, ownRepo string) []cardRef {
	seenNumbers := make(map[int]bool, len(refs))
	for _, r := range refs {
		seenNumbers[r.Number] = true
	}
	seenForeignURLs := make(map[string]bool)

	nextLabel := rune('a')
	if len(refs) > 0 {
		nextLabel = refs[len(refs)-1].Label + 1
	}

	for _, bl := range blockers {
		if !strings.EqualFold(bl.State, "OPEN") {
			continue
		}

		url, repo := blockerRefTarget(bl, ownRepo)
		if repo == "" {
			// Same-repo target: dedup by number against body refs and
			// already-appended same-repo blockers.
			if seenNumbers[bl.Number] {
				continue
			}
		} else {
			// Foreign target: an empty URL is unreachable and must be
			// omitted entirely; otherwise dedup among foreign blockers by
			// URL (the same number in a different repo is a distinct
			// target).
			if url == "" {
				continue
			}
			if seenForeignURLs[url] {
				continue
			}
		}

		if nextLabel > 'z' {
			break
		}

		if repo == "" {
			seenNumbers[bl.Number] = true
		} else {
			seenForeignURLs[url] = true
		}

		refs = append(refs, cardRef{Number: bl.Number, Label: nextLabel, URL: url, Repo: repo})
		nextLabel++
	}

	return refs
}

// blockerRefTarget classifies a single blocker as same-repo (returns empty
// url/repo, so resolveReference's findCard shortcut still fires) or foreign
// (returns bl.URL/bl.RepoNameWithOwner). The comparison is EqualFold against
// ownRepo. When ownRepo is empty (the board's own repo slug is unknown), any
// blocker with a non-empty RepoNameWithOwner fails safe to foreign --
// opening its own URL is never wrong, only less convenient, while a mistaken
// jump could silently land on the wrong ticket. A blocker with an empty
// RepoNameWithOwner AND an empty URL (a non-GitHub provider genuinely
// carrying neither) is always treated as same-repo, even when ownRepo is
// unknown, since there is no repo/URL to fail safe toward. A blocker with an
// empty RepoNameWithOwner but a non-empty URL (e.g. a GraphQL response whose
// repository field came back null due to a permission/field-level error
// while url still resolved) fails safe to that explicit URL with repo left
// empty -- resolveReference's ref.URL != "" check still bypasses
// findCard/refIssueURL and opens the correct URL directly, while
// refLabelText renders a bare "#N" since the repo slug is genuinely
// unknown.
func blockerRefTarget(bl Blocker, ownRepo string) (url, repo string) {
	if bl.RepoNameWithOwner == "" {
		return bl.URL, ""
	}
	if ownRepo != "" && strings.EqualFold(bl.RepoNameWithOwner, ownRepo) {
		return "", ""
	}
	return bl.URL, bl.RepoNameWithOwner
}

// refLabelText renders the which-key hint label / success-message text for
// a cardRef: a bare "#N" for a same-repo (body or on-board blocker) ref, or
// "owner/repo#N" for a foreign blocker ref. This is the single sanitization
// choke point for Repo, an untrusted API-sourced value -- both refHints and
// the "Opened ..." success message route through it.
func refLabelText(r cardRef) string {
	if r.Repo == "" {
		return fmt.Sprintf("#%d", r.Number)
	}
	return sanitizeSingleLine(r.Repo) + fmt.Sprintf("#%d", r.Number)
}

// refHints builds the which-key hint bar for a pending reference-navigation
// prompt: one hint per reference label, plus a trailing "esc: cancel" hint.
// The trailing "esc" hint is an intentional hard-wired exception (#583
// audit) -- handlePendingRefKey branches on tea.KeyEsc directly with no
// catalogued command behind it, the same class of hard-wire as Lookup's
// ctrl+c short-circuit (internal/keymap/lookup.go).
func refHints(refs []cardRef) []Hint {
	hints := make([]Hint, 0, len(refs)+1)
	for _, r := range refs {
		hints = append(hints, Hint{Key: string(r.Label), Desc: refLabelText(r)})
	}
	hints = append(hints, Hint{Key: "esc", Desc: "cancel"})
	return hints
}

// clearPendingRefs resets the pending reference-navigation state.
func (b *Board) clearPendingRefs() {
	b.pendingRefs = nil
}

// handleReferenceNavKey is the shared nav.reference (default "g r") trigger
// for normal mode and detail-focused mode: it collects the selected card's
// references (body #N refs plus its open blockers, see cardReferences) and,
// if any exist, enters the pending reference-navigation state with one
// which-key hint per reference. A card with no references is a no-op with a
// status message.
func (b Board) handleReferenceNavKey() (tea.Model, tea.Cmd) {
	if len(b.Columns) == 0 || len(b.visibleCards()) == 0 {
		return b, nil
	}
	card := b.selectedCard()
	refs := b.cardReferences(card)
	if len(refs) == 0 {
		cmd := b.statusBar.SetTimedMessage("No references", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	b.pendingRefs = refs
	b.statusBar.SetActionHints(refHints(refs))
	return b, nil
}

// handlePendingRefKey consumes the next key of a pending reference-navigation
// prompt: esc cancels (keeping detail focus as-is), a key matching a
// reference's label resolves it, and any other key cancels with a warning
// message rather than falling through to built-in key handling (mirrors
// handlePendingSeqKey's unmatched-continuation behavior).
func (b Board) handlePendingRefKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		b.clearPendingRefs()
		b.restoreFocusHints()
		return b, nil
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		b.clearPendingRefs()
		b.restoreFocusHints()
		cmd := b.statusBar.SetTimedMessage("Reference selection cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}

	label := msg.Runes[0]
	var matched cardRef
	found := false
	for _, r := range b.pendingRefs {
		if r.Label == label {
			matched = r
			found = true
			break
		}
	}
	b.clearPendingRefs()
	if !found {
		b.restoreFocusHints()
		cmd := b.statusBar.SetTimedMessage("No reference bound to "+string(label), StatusWarning, statusMessageDuration)
		return b, cmd
	}
	return b.resolveReference(matched)
}

// resolveReference resolves a selected reference: a ref carrying an
// explicit foreign URL (a cross-repo blocker) is opened directly, bypassing
// findCard/refIssueURL entirely -- refIssueURL's trailing-digit substitution
// derives a same-repo URL and would silently resolve a cross-repo blocker to
// the wrong issue if reached. Otherwise, it jumps to the referenced card if
// it's on the board (findCard), or opens its constructed same-repo URL.
func (b Board) resolveReference(ref cardRef) (tea.Model, tea.Cmd) {
	if ref.URL != "" {
		return b.openReferenceTarget(ref, ref.URL)
	}
	if colIdx, cardIdx, ok := b.findCard(ref.Number); ok {
		return b.jumpToReferencedCard(colIdx, cardIdx)
	}
	return b.openReferenceURL(ref)
}

// jumpToReferencedCard switches to the referenced card's column and
// positions the cursor on it. If the card is hidden by the active
// filter/search, both are cleared first (with a "Filter cleared" message,
// reused verbatim for the search-hidden case) so the cursor can index into
// the now-unfiltered visibleCards() -- clear, then position the cursor, per
// docs/list-cursor-invariants.md. Always drops detail focus, mirroring every
// other column-switching action available from detail focus.
func (b Board) jumpToReferencedCard(colIdx, cardIdx int) (tea.Model, tea.Cmd) {
	b.ActiveTab = colIdx
	target := b.Columns[colIdx].Cards[cardIdx]

	hidden := false
	if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
		hidden = true
		for _, c := range b.visibleCards() {
			if c.Number == target.Number {
				hidden = false
				break
			}
		}
	}

	var cmd tea.Cmd
	if hidden {
		b.clearFilter()
		b.clearSearch()
		cmd = b.statusBar.SetTimedMessage("Filter cleared", StatusSuccess, statusMessageDuration)
	}

	cursor := cardIdx
	if !hidden {
		for i, c := range b.visibleCards() {
			if c.Number == target.Number {
				cursor = i
				break
			}
		}
	}

	b.Columns[colIdx].Cursor = cursor
	b.detailFocused = false
	b.onCursorMoved()
	return b, cmd
}

// openReferenceURL opens the constructed same-repo issue/PR URL for a
// referenced ref that isn't on the board, mirroring handleTicketOpenKey's
// error surfacing (including its "URL not available" guard when no valid
// URL can be constructed).
func (b Board) openReferenceURL(ref cardRef) (tea.Model, tea.Cmd) {
	card := b.selectedCard()
	url, ok := refIssueURL(card.URL, ref.Number)
	if !ok {
		cmd := b.statusBar.SetTimedMessage("URL not available", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	return b.openReferenceTarget(ref, url)
}

// openReferenceTarget opens url (either ref's own explicit foreign URL, or
// a same-repo URL constructed by openReferenceURL) via the executor,
// surfacing an executor error verbatim (mirroring handleTicketOpenKey) or a
// success message naming ref via refLabelText on success.
func (b Board) openReferenceTarget(ref cardRef, url string) (tea.Model, tea.Cmd) {
	if err := b.executor.OpenURL(url); err != nil {
		cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
		return b, cmd
	}

	cmd := b.statusBar.SetTimedMessage("Opened "+refLabelText(ref), StatusSuccess, statusMessageDuration)
	return b, cmd
}
