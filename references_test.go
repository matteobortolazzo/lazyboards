package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// --- isWordByte ---

func TestIsWordByte(t *testing.T) {
	wordBytes := []byte{'a', 'z', 'A', 'Z', '0', '9', '_'}
	for _, b := range wordBytes {
		if !isWordByte(b) {
			t.Errorf("isWordByte(%q) = false, want true", b)
		}
	}

	nonWordBytes := []byte{' ', ',', '.', '#', '(', ')', '\n', '\t', '-', '/'}
	for _, b := range nonWordBytes {
		if isWordByte(b) {
			t.Errorf("isWordByte(%q) = true, want false", b)
		}
	}

	// Multibyte-safe: any byte >= 0x80 (a UTF-8 continuation or lead byte)
	// counts as a word byte so multibyte characters aren't treated as
	// boundaries.
	multibyteBytes := []byte{0x80, 0xC3, 0xE9, 0xFF}
	for _, b := range multibyteBytes {
		if !isWordByte(b) {
			t.Errorf("isWordByte(0x%X) = false, want true (multibyte byte)", b)
		}
	}
}

// --- parseCardRefs ---

func TestParseCardRefs_OrdersByFirstAppearanceAndDedups(t *testing.T) {
	body := "See #5 and #3 and #5 again"

	refs := parseCardRefs(body)

	if len(refs) != 2 {
		t.Fatalf("parseCardRefs() returned %d refs, want 2 (deduped by number): %+v", len(refs), refs)
	}
	if refs[0].Number != 5 || refs[0].Label != 'a' {
		t.Errorf("refs[0] = %+v, want {Number: 5, Label: 'a'} (first appearance)", refs[0])
	}
	if refs[1].Number != 3 || refs[1].Label != 'b' {
		t.Errorf("refs[1] = %+v, want {Number: 3, Label: 'b'} (second distinct number seen)", refs[1])
	}
}

func TestParseCardRefs_RejectsMissingBoundaries(t *testing.T) {
	// "abc#12" fails the leading-boundary check (preceding char 'c' is a word char).
	// "#12abc" fails the trailing-boundary check (following char 'a' is a word char).
	// "x#12" fails the leading-boundary check (preceding char 'x' is a word char).
	// Only "#99" satisfies both boundaries.
	body := "abc#12 #12abc x#12 valid #99 useful"

	refs := parseCardRefs(body)

	if len(refs) != 1 {
		t.Fatalf("parseCardRefs(%q) returned %d refs, want 1 (only #99 has valid boundaries): %+v", body, len(refs), refs)
	}
	if refs[0].Number != 99 {
		t.Errorf("refs[0].Number = %d, want 99", refs[0].Number)
	}
}

func TestParseCardRefs_AcceptsValidBoundaries(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"parenthesized", "(#12)"},
		{"followed by comma", "#12,"},
		{"start of string", "#12 is the issue"},
		{"end of string", "the issue is #12"},
		{"after newline", "line one\n#12 fix"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refs := parseCardRefs(tc.body)
			if len(refs) != 1 {
				t.Fatalf("parseCardRefs(%q) returned %d refs, want 1: %+v", tc.body, len(refs), refs)
			}
			if refs[0].Number != 12 {
				t.Errorf("refs[0].Number = %d, want 12", refs[0].Number)
			}
		})
	}
}

func TestParseCardRefs_RejectsZeroAndLeadingZero(t *testing.T) {
	body := "#0 #007 #7"

	refs := parseCardRefs(body)

	if len(refs) != 1 {
		t.Fatalf("parseCardRefs(%q) returned %d refs, want 1 (#0 and #007 must be rejected): %+v", body, len(refs), refs)
	}
	if refs[0].Number != 7 {
		t.Errorf("refs[0].Number = %d, want 7", refs[0].Number)
	}
}

func TestParseCardRefs_CapsAt26Distinct(t *testing.T) {
	var parts []string
	for i := 1; i <= 27; i++ {
		parts = append(parts, fmt.Sprintf("#%d", i))
	}
	body := strings.Join(parts, " ")

	refs := parseCardRefs(body)

	if len(refs) != 26 {
		t.Fatalf("parseCardRefs() returned %d refs for 27 distinct numbers, want 26 (capped)", len(refs))
	}
	if refs[0].Number != 1 || refs[0].Label != 'a' {
		t.Errorf("refs[0] = %+v, want {Number: 1, Label: 'a'}", refs[0])
	}
	if refs[25].Number != 26 || refs[25].Label != 'z' {
		t.Errorf("refs[25] = %+v, want {Number: 26, Label: 'z'}", refs[25])
	}
	for _, r := range refs {
		if r.Number == 27 {
			t.Errorf("parseCardRefs() included #27 (the 27th distinct number), want it left out entirely (not truncated to 'z')")
		}
	}
}

func TestParseCardRefs_RejectsPathologicallyLongDigitRun(t *testing.T) {
	// A digit run this long cannot be a real GitHub issue/PR number. Before
	// the length bound, strconv.Atoi would silently overflow (ErrRange,
	// discarded) and return a clamped MaxInt as if it were a valid
	// reference number -- it must instead be rejected outright, exactly
	// like any other invalid match.
	longDigitRun := strings.Repeat("9", 30)
	body := "See #" + longDigitRun + " and #7"

	refs := parseCardRefs(body)

	if len(refs) != 1 {
		t.Fatalf("parseCardRefs(%q) returned %d refs, want 1 (only #7 is valid; the long digit run must be rejected): %+v", body, len(refs), refs)
	}
	if refs[0].Number != 7 {
		t.Errorf("refs[0].Number = %d, want 7", refs[0].Number)
	}

	annotated := annotateBodyRefs(body)
	if !strings.Contains(annotated, "#"+longDigitRun) {
		t.Errorf("annotateBodyRefs() should leave the long digit run present in the body (just unlabeled), got:\n%s", annotated)
	}
	if strings.Contains(annotated, longDigitRun+" \\[") {
		t.Errorf("annotateBodyRefs() should not label the pathologically long digit run, got:\n%s", annotated)
	}
}

func TestParseCardRefs_AdjacentRefsSeparatedBySpace(t *testing.T) {
	body := "#1 #2"

	refs := parseCardRefs(body)

	if len(refs) != 2 {
		t.Fatalf("parseCardRefs(%q) returned %d refs, want 2: %+v", body, len(refs), refs)
	}
	if refs[0].Number != 1 || refs[0].Label != 'a' {
		t.Errorf("refs[0] = %+v, want {Number: 1, Label: 'a'}", refs[0])
	}
	if refs[1].Number != 2 || refs[1].Label != 'b' {
		t.Errorf("refs[1] = %+v, want {Number: 2, Label: 'b'}", refs[1])
	}
}

// --- annotateBodyRefs ---

func TestAnnotateBodyRefs_LabelsAllOccurrencesOfSharedNumber(t *testing.T) {
	body := "See #5 now and #5 later"

	annotated := annotateBodyRefs(body)

	count := strings.Count(annotated, "#5 \\[a\\]")
	if count != 2 {
		t.Errorf("annotateBodyRefs(%q) = %q, want both occurrences of #5 labeled \"#5 \\[a\\]\" (escaped brackets) (got %d matches)", body, annotated, count)
	}
}

func TestAnnotateBodyRefs_OverflowLeftUnlabeled(t *testing.T) {
	var parts []string
	for i := 1; i <= 27; i++ {
		parts = append(parts, fmt.Sprintf("#%d", i))
	}
	body := strings.Join(parts, " ")

	annotated := annotateBodyRefs(body)

	if !strings.Contains(annotated, "#26 \\[z\\]") {
		t.Errorf("annotateBodyRefs() should label the 26th distinct reference \"#26 \\[z\\]\" (escaped brackets), got:\n%s", annotated)
	}
	if strings.Contains(annotated, "#27 \\[") {
		t.Errorf("annotateBodyRefs() should leave the 27th distinct reference (#27) unlabeled, got:\n%s", annotated)
	}
	if !strings.Contains(annotated, "#27") {
		t.Errorf("annotateBodyRefs() should leave #27 present in the body (just unlabeled), got:\n%s", annotated)
	}
}

func TestAnnotateBodyRefs_CodeBlockAgnostic(t *testing.T) {
	// Regex-only detection: a #N reference inside a fenced code block IS
	// labeled, since no goldmark AST parsing / code-fence exclusion is done.
	body := "```\nSee #5 in code\n```"

	annotated := annotateBodyRefs(body)

	if !strings.Contains(annotated, "#5 \\[a\\]") {
		t.Errorf("annotateBodyRefs() should label #5 even inside a fenced code block, got:\n%s", annotated)
	}
}

// --- Integration: renderDetailLines / composeDetailMarkdown wiring ---

func TestIntegration_RenderDetailLines_PersistentRefLabelSurvivesRender(t *testing.T) {
	// This exercises the real render stack (composeDetailMarkdown -> glamour ->
	// ansi.Hardwrap) that both viewCardDetail and scrollDetailDown route
	// through via renderDetailLines (docs/view-state-consistency.md).
	// The wiring that injects the label into the body via annotateBodyRefs
	// lands in composeDetailMarkdown in Phase 4 -- until then this must fail.
	card := Card{
		Number: 1,
		Title:  "Investigate crash",
		Body:   "Related to #5 for context.",
	}
	contentWidth := 60

	lines := renderDetailLines(card, contentWidth)
	rendered := strings.Join(lines, "\n")

	if !strings.Contains(rendered, "[a]") {
		t.Errorf("renderDetailLines() output does not contain persistent ref label \"[a]\" for #5, got:\n%s", rendered)
	}

	// The transform must not corrupt layout: no rendered line may exceed
	// contentWidth (per docs/terminal-rendering.md, measured with
	// lipgloss.Width, not len/rune-count).
	for i, line := range lines {
		if w := lipgloss.Width(line); w > contentWidth {
			t.Errorf("renderDetailLines() line %d has width %d, want <= %d (contentWidth); line: %q", i, w, contentWidth, line)
		}
	}

	// Line-count consistency: both viewCardDetail and scrollDetailDown derive
	// their line count from this same renderDetailLines call, so a single
	// call's result is definitionally what both consumers see. Assert the
	// result is non-empty and deterministic for a fixed input.
	linesAgain := renderDetailLines(card, contentWidth)
	if len(linesAgain) != len(lines) {
		t.Errorf("renderDetailLines() called twice with identical input returned different line counts (%d vs %d) -- scroll math and rendering would disagree", len(linesAgain), len(lines))
	}
}

func TestIntegration_RenderDetailLines_AdjacentParenthesisDoesNotFormLink(t *testing.T) {
	// Regression test: a #N reference immediately followed by "(text)" with
	// no separating space must not let the inserted " [a]" label combine
	// with the following "(...)" into markdown link syntax ("[a](text)"),
	// which glamour would parse as a real hyperlink -- swallowing/styling
	// the visible "(duplicate)" text instead of leaving it as plain text.
	card := Card{
		Number: 1,
		Title:  "Investigate crash",
		Body:   "Related to #5(duplicate) for context.",
	}
	contentWidth := 60

	rendered := strings.Join(renderDetailLines(card, contentWidth), "\n")

	t.Logf("rendered output: %q", rendered)

	if !strings.Contains(rendered, "[a]") {
		t.Errorf("renderDetailLines() output does not contain persistent ref label \"[a]\" for #5, got:\n%s", rendered)
	}

	// The literal "(duplicate)" text must survive as plain text -- if the
	// label and the following parenthesized run combined into markdown
	// link syntax, glamour would drop the parentheses and/or the label
	// text would be swallowed into link styling instead of staying visible
	// as ordinary text.
	if !strings.Contains(rendered, "(duplicate)") {
		t.Errorf("renderDetailLines() output does not preserve literal \"(duplicate)\" as plain text -- looks like it was swallowed into markdown link syntax, got:\n%s", rendered)
	}
}

// --- cardReferences / appendBlockerRefs / blockerRefTarget / refLabelText (#632) ---
//
// #632 extends the g r reference source from "body #N refs" to "body refs +
// the card's open blockers": appendBlockerRefs merges a card's Blockers into
// an already-parsed []cardRef (open-state filter, same-repo/foreign dedup,
// label continuation, 26-cap), blockerRefTarget classifies a single blocker
// as same-repo or foreign, refLabelText renders a cardRef's which-key hint
// label, and cardReferences is the Board method wiring body refs + blockers
// together using the board's own configured repo slug.

func TestAppendBlockerRefs_ContinuesLabelSequenceAfterBodyRefs(t *testing.T) {
	bodyRefs := []cardRef{{Number: 5, Label: 'a'}, {Number: 3, Label: 'b'}}
	blockers := []Blocker{
		{Number: 10, State: "OPEN", RepoNameWithOwner: "owner/repo"},
		{Number: 11, State: "OPEN", RepoNameWithOwner: "owner/repo"},
	}

	got := appendBlockerRefs(bodyRefs, blockers, "owner/repo")

	want := []cardRef{
		{Number: 5, Label: 'a'},
		{Number: 3, Label: 'b'},
		{Number: 10, Label: 'c'},
		{Number: 11, Label: 'd'},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appendBlockerRefs() = %+v, want %+v (blocker labels continue after the last body label)", got, want)
	}
}

func TestAppendBlockerRefs_ExcludesClosedBlockers(t *testing.T) {
	blockers := []Blocker{
		{Number: 10, State: "OPEN", RepoNameWithOwner: "owner/repo"},
		{Number: 11, State: "CLOSED", RepoNameWithOwner: "owner/repo"},
	}

	got := appendBlockerRefs(nil, blockers, "owner/repo")

	want := []cardRef{{Number: 10, Label: 'a'}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appendBlockerRefs() = %+v, want %+v (a closed blocker must not be offered as a reference target)", got, want)
	}
}

func TestAppendBlockerRefs_SameRepoBlockerAlreadyInBodyDeduped(t *testing.T) {
	bodyRefs := []cardRef{{Number: 5, Label: 'a'}}
	blockers := []Blocker{{Number: 5, State: "OPEN", RepoNameWithOwner: "owner/repo"}}

	got := appendBlockerRefs(bodyRefs, blockers, "owner/repo")

	want := []cardRef{{Number: 5, Label: 'a'}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appendBlockerRefs() = %+v, want %+v (a same-repo blocker already mentioned in the body must not be duplicated)", got, want)
	}
}

func TestAppendBlockerRefs_DuplicateBlockerNumbersDeduped(t *testing.T) {
	blockers := []Blocker{
		{Number: 7, State: "OPEN", RepoNameWithOwner: "owner/repo"},
		{Number: 7, State: "OPEN", RepoNameWithOwner: "owner/repo"},
	}

	got := appendBlockerRefs(nil, blockers, "owner/repo")

	want := []cardRef{{Number: 7, Label: 'a'}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appendBlockerRefs() = %+v, want %+v (two same-repo blockers sharing a number must dedup to one ref)", got, want)
	}
}

func TestAppendBlockerRefs_ForeignBlockersDedupedByURLNotNumber(t *testing.T) {
	// #632 Q&A decision 5: cross-repo blockers dedup among themselves by URL,
	// not by number -- the same issue number in two different repos is two
	// distinct targets.
	blockers := []Blocker{
		{Number: 30, State: "OPEN", RepoNameWithOwner: "other/repo", URL: "https://github.com/other/repo/issues/30"},
		{Number: 30, State: "OPEN", RepoNameWithOwner: "other/repo", URL: "https://github.com/other/repo/issues/30"},
		{Number: 30, State: "OPEN", RepoNameWithOwner: "third/repo", URL: "https://github.com/third/repo/issues/30"},
	}

	got := appendBlockerRefs(nil, blockers, "owner/repo")

	if len(got) != 2 {
		t.Fatalf("appendBlockerRefs() = %+v, want 2 refs (the identical-URL duplicate dedups, but the same number in a different foreign repo is a distinct target)", got)
	}
}

func TestAppendBlockerRefs_CapAt26_OneBlockerFitsSecondOmittedNotTruncated(t *testing.T) {
	var bodyRefs []cardRef
	label := 'a'
	for i := 1; i <= 25; i++ {
		bodyRefs = append(bodyRefs, cardRef{Number: i, Label: label})
		label++
	}
	if label != 'z' {
		t.Fatalf("test setup error: expected the 25 body refs to leave 'z' as the next label, got %q", label)
	}
	blockers := []Blocker{
		{Number: 100, State: "OPEN", RepoNameWithOwner: "owner/repo"},
		{Number: 101, State: "OPEN", RepoNameWithOwner: "owner/repo"},
	}

	got := appendBlockerRefs(bodyRefs, blockers, "owner/repo")

	if len(got) != 26 {
		t.Fatalf("appendBlockerRefs() returned %d refs, want 26 (25 body refs + 1 blocker that fits the cap)", len(got))
	}
	last := got[len(got)-1]
	if last.Number != 100 || last.Label != 'z' {
		t.Errorf("last ref = %+v, want {Number: 100, Label: 'z'} (the first blocker fills the 26th slot)", last)
	}
	for _, r := range got {
		if r.Number == 101 {
			t.Errorf("appendBlockerRefs() included the 27th distinct target (#101), want it omitted entirely -- never truncated onto 'z'")
		}
	}
}

func TestAppendBlockerRefs_ForeignBlockerCarriesURLAndRepoSameRepoCarriesNeither(t *testing.T) {
	blockers := []Blocker{
		{Number: 10, State: "OPEN", RepoNameWithOwner: "owner/repo", URL: "https://github.com/owner/repo/issues/10"},
		{Number: 20, State: "OPEN", RepoNameWithOwner: "other/repo", URL: "https://github.com/other/repo/issues/20"},
	}

	got := appendBlockerRefs(nil, blockers, "owner/repo")

	if len(got) != 2 {
		t.Fatalf("appendBlockerRefs() = %+v, want 2 refs", got)
	}
	sameRepo := got[0]
	if sameRepo.Number != 10 || sameRepo.URL != "" || sameRepo.Repo != "" {
		t.Errorf("same-repo ref = %+v, want URL and Repo both empty (so resolveReference's findCard shortcut still fires)", sameRepo)
	}
	foreign := got[1]
	if foreign.Number != 20 || foreign.URL != "https://github.com/other/repo/issues/20" || foreign.Repo != "other/repo" {
		t.Errorf("foreign ref = %+v, want URL/Repo populated from the blocker", foreign)
	}
}

func TestAppendBlockerRefs_ForeignBlockerWithEmptyURLOmitted(t *testing.T) {
	blockers := []Blocker{
		{Number: 20, State: "OPEN", RepoNameWithOwner: "other/repo", URL: ""},
		{Number: 21, State: "OPEN", RepoNameWithOwner: "owner/repo", URL: "https://github.com/owner/repo/issues/21"},
	}

	got := appendBlockerRefs(nil, blockers, "owner/repo")

	want := []cardRef{{Number: 21, Label: 'a'}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appendBlockerRefs() = %+v, want %+v (a foreign blocker with an empty URL is an unreachable target and must be omitted entirely)", got, want)
	}
}

func TestBlockerRefTarget_EqualFoldRepoComparisonTreatsCaseVariantAsSameRepo(t *testing.T) {
	bl := Blocker{Number: 5, RepoNameWithOwner: "Owner/Repo", URL: "https://github.com/Owner/Repo/issues/5"}

	url, repo := blockerRefTarget(bl, "owner/repo")

	if url != "" || repo != "" {
		t.Errorf("blockerRefTarget() = (%q, %q), want (\"\", \"\") for a case-only repo variant (EqualFold match)", url, repo)
	}
}

func TestBlockerRefTarget_MismatchedRepoIsForeign(t *testing.T) {
	bl := Blocker{Number: 5, RepoNameWithOwner: "other/repo", URL: "https://github.com/other/repo/issues/5"}

	url, repo := blockerRefTarget(bl, "owner/repo")

	if url != bl.URL || repo != bl.RepoNameWithOwner {
		t.Errorf("blockerRefTarget() = (%q, %q), want (%q, %q) for a mismatched repo", url, repo, bl.URL, bl.RepoNameWithOwner)
	}
}

func TestBlockerRefTarget_UnknownOwnRepoFailsSafeToForeign(t *testing.T) {
	// #632 Q&A decision 3: when the board's own repo slug is unknown
	// (unconfigured board), any blocker with a non-empty RepoNameWithOwner
	// must be treated as foreign -- opening its own URL is never wrong, only
	// less convenient, but a mistaken jump would silently land on the wrong
	// ticket.
	bl := Blocker{Number: 5, RepoNameWithOwner: "owner/repo", URL: "https://github.com/owner/repo/issues/5"}

	url, repo := blockerRefTarget(bl, "")

	if url != bl.URL || repo != bl.RepoNameWithOwner {
		t.Errorf("blockerRefTarget() with unknown own repo = (%q, %q), want fail-safe foreign (%q, %q)", url, repo, bl.URL, bl.RepoNameWithOwner)
	}
}

func TestBlockerRefTarget_EmptyRepoNameWithOwnerIsSameRepoEvenWithUnknownOwnRepo(t *testing.T) {
	// A non-GitHub provider's blocker carries no RepoNameWithOwner. Even when
	// the board's own repo slug is unknown, this must not be misclassified
	// as foreign -- there is no repo/URL to fail safe toward, and today's
	// behavior (same-repo, findCard-eligible) must be preserved.
	bl := Blocker{Number: 5, RepoNameWithOwner: "", URL: ""}

	url, repo := blockerRefTarget(bl, "")

	if url != "" || repo != "" {
		t.Errorf("blockerRefTarget() = (%q, %q), want (\"\", \"\") for an empty RepoNameWithOwner (non-GitHub provider)", url, repo)
	}
}

func TestBlockerRefTarget_EmptyRepoNameWithOwnerButPresentURLFailsSafeToExplicitURL(t *testing.T) {
	// A GraphQL partial response can null out Repository (e.g. a
	// permission/field-level error) while URL still resolved. Treating this
	// as same-repo (the old behavior) would discard a perfectly good foreign
	// URL and route the blocker into the number-based findCard/refIssueURL
	// path -- exactly the silent wrong-card-jump risk #632 exists to
	// prevent. It must instead fail safe to the explicit URL: Repo stays
	// empty (the slug is genuinely unknown, so refLabelText renders a bare
	// "#N"), but resolveReference's ref.URL != "" check still bypasses
	// findCard/refIssueURL and opens the correct URL directly.
	bl := Blocker{Number: 5, RepoNameWithOwner: "", URL: "https://github.com/other/repo/issues/5"}

	url, repo := blockerRefTarget(bl, "")

	if url != bl.URL || repo != "" {
		t.Errorf("blockerRefTarget() = (%q, %q), want (%q, \"\") (fail safe to the explicit URL, repo left empty)", url, repo, bl.URL)
	}
}

func TestAppendBlockerRefs_EmptyRepoNameWithOwnerButPresentURLCarriesURLThroughWithEmptyRepo(t *testing.T) {
	// Confirms appendBlockerRefs does not assume "non-empty URL implies
	// non-empty Repo": the resulting cardRef must carry the URL (so
	// resolveReference opens it directly) even though Repo stays empty (so
	// refLabelText renders a bare "#N", not a "owner/repo#N" label built
	// from an unknown slug).
	blockers := []Blocker{
		{Number: 30, State: "OPEN", RepoNameWithOwner: "", URL: "https://github.com/other/repo/issues/30"},
	}

	got := appendBlockerRefs(nil, blockers, "owner/repo")

	want := []cardRef{{Number: 30, Label: 'a', URL: "https://github.com/other/repo/issues/30", Repo: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("appendBlockerRefs() = %+v, want %+v (URL carried through, Repo left empty)", got, want)
	}
}

func TestRefLabelText_BodyRefRendersBareHash(t *testing.T) {
	got := refLabelText(cardRef{Number: 5, Label: 'a'})

	if got != "#5" {
		t.Errorf("refLabelText() = %q, want %q", got, "#5")
	}
}

func TestRefLabelText_ForeignRefPrefixesRepo(t *testing.T) {
	ref := cardRef{Number: 200, Label: 'b', Repo: "other/repo", URL: "https://github.com/other/repo/issues/200"}

	got := refLabelText(ref)

	if got != "other/repo#200" {
		t.Errorf("refLabelText() = %q, want %q", got, "other/repo#200")
	}
}

func TestRefLabelText_SanitizesHostileRepo(t *testing.T) {
	hostile := "evil/repo\n\x1b[31mRED\x1b[0m" + string(rune(0x202E)) + "hack"
	ref := cardRef{Number: 9, Label: 'c', Repo: hostile, URL: "https://github.com/evil/repo/issues/9"}

	got := refLabelText(ref)

	if strings.Contains(got, "\n") {
		t.Errorf("refLabelText() = %q, contains a raw newline -- a hostile Repo must be flattened via sanitizeSingleLine", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("refLabelText() = %q, contains a raw ANSI escape byte", got)
	}
	if strings.ContainsRune(got, rune(0x202E)) {
		t.Errorf("refLabelText() = %q, contains a raw bidi-override rune", got)
	}
	if !strings.HasSuffix(got, "#9") {
		t.Errorf("refLabelText() = %q, want it to end with %q", got, "#9")
	}
}

func TestCardReferences_MergesBodyRefsAndOpenBlockers(t *testing.T) {
	b := Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"}
	card := Card{
		Number: 1,
		Body:   "See #5",
		Blockers: []Blocker{
			{Number: 10, State: "OPEN", RepoNameWithOwner: "matteobortolazzo/lazyboards", URL: "https://github.com/matteobortolazzo/lazyboards/issues/10"},
			{Number: 11, State: "CLOSED", RepoNameWithOwner: "matteobortolazzo/lazyboards"},
			{Number: 20, State: "OPEN", RepoNameWithOwner: "other/repo", URL: "https://github.com/other/repo/issues/20"},
		},
	}

	got := b.cardReferences(card)

	want := []cardRef{
		{Number: 5, Label: 'a'},
		{Number: 10, Label: 'b'},
		{Number: 20, Label: 'c', URL: "https://github.com/other/repo/issues/20", Repo: "other/repo"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cardReferences() = %+v, want %+v", got, want)
	}
}

func TestCardReferences_UnknownOwnRepoTreatsNonEmptyRepoBlockersAsForeign(t *testing.T) {
	b := Board{repoOwner: "", repoName: ""}
	card := Card{
		Blockers: []Blocker{
			{Number: 10, State: "OPEN", RepoNameWithOwner: "owner/repo", URL: "https://github.com/owner/repo/issues/10"},
		},
	}

	got := b.cardReferences(card)

	if len(got) != 1 || got[0].URL == "" || got[0].Repo == "" {
		t.Errorf("cardReferences() with unknown own repo = %+v, want a foreign ref (URL and Repo populated)", got)
	}
}

func TestCardReferences_OnlyBlockerRefsLeaveBodyAnnotationUnchanged(t *testing.T) {
	body := "Nothing to see here"
	card := Card{
		Body: body,
		Blockers: []Blocker{
			{Number: 5, State: "OPEN", RepoNameWithOwner: "matteobortolazzo/lazyboards", URL: "https://github.com/matteobortolazzo/lazyboards/issues/5"},
		},
	}
	b := Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"}

	refs := b.cardReferences(card)
	if len(refs) != 1 {
		t.Fatalf("cardReferences() = %+v, want 1 blocker-derived ref", refs)
	}

	annotated := annotateBodyRefs(body)
	if annotated != body {
		t.Errorf("annotateBodyRefs(%q) = %q, want unchanged -- blocker-derived refs must never leak into rendered body annotation", body, annotated)
	}
}
