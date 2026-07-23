package main

import (
	"fmt"
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
