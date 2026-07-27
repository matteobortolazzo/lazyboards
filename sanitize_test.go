package main

import (
	"strings"
	"testing"
)

// --- Terminal Control Sequence Sanitization (#469) ---
//
// card.Body originates from an untrusted GitHub issue and is rendered
// through glamour. sanitizeControlSequences must strip ANSI/OSC/CSI/DCS
// escape sequences (retaining the visible text they wrapped) and remove
// remaining standalone C0/C1 control runes, preserving only \n and \t so
// markdown structure and code indentation survive.

func TestSanitizeControlSequences_StripsSGREscapeButKeepsVisibleText(t *testing.T) {
	input := "\x1b[31mRED\x1b[0m"

	got := sanitizeControlSequences(input)

	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("sanitizeControlSequences(%q) = %q, want no ESC (0x1b) byte", input, got)
	}
	if !strings.Contains(got, "RED") {
		t.Errorf("sanitizeControlSequences(%q) = %q, want visible text %q retained", input, got, "RED")
	}
}

func TestSanitizeControlSequences_RemovesRawBEL(t *testing.T) {
	input := "before\x07after"

	got := sanitizeControlSequences(input)

	if strings.ContainsRune(got, '\x07') {
		t.Errorf("sanitizeControlSequences(%q) = %q, want no BEL (0x07) byte", input, got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("sanitizeControlSequences(%q) = %q, want surrounding visible text retained", input, got)
	}
}

func TestSanitizeControlSequences_OSC8Hyperlink_FullyRemovedVisibleLabelRetained(t *testing.T) {
	// An OSC-8 hyperlink wraps visible label text between BEL-terminated
	// OSC sequences pointing at an arbitrary (here, malicious-looking) URL.
	input := "\x1b]8;;https://evil\x07label\x1b]8;;\x07"

	got := sanitizeControlSequences(input)

	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("sanitizeControlSequences(%q) = %q, want no ESC (0x1b) byte", input, got)
	}
	if strings.ContainsRune(got, '\x07') {
		t.Errorf("sanitizeControlSequences(%q) = %q, want no BEL (0x07) byte", input, got)
	}
	if strings.Contains(got, "evil") {
		t.Errorf("sanitizeControlSequences(%q) = %q, want the hyperlink target URL fully removed", input, got)
	}
	if !strings.Contains(got, "label") {
		t.Errorf("sanitizeControlSequences(%q) = %q, want visible hyperlink label %q retained", input, got, "label")
	}
}

func TestSanitizeControlSequences_PreservesNewlineAndTab(t *testing.T) {
	input := "line one\n\tindented line two"

	got := sanitizeControlSequences(input)

	if got != input {
		t.Errorf("sanitizeControlSequences(%q) = %q, want unchanged (newline/tab must be preserved)", input, got)
	}
}

func TestSanitizeControlSequences_RemovesC1ControlByte(t *testing.T) {
	// 0x9b (CSI as a single C1 byte) is in the C1 control range (0x80-0x9F).
	input := "before\x9bafter"

	got := sanitizeControlSequences(input)

	if strings.ContainsRune(got, '\x9b') {
		t.Errorf("sanitizeControlSequences(%q) = %q, want no C1 control byte 0x9b", input, got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("sanitizeControlSequences(%q) = %q, want surrounding visible text retained", input, got)
	}
}

func TestSanitizeControlSequences_CleanTextUnchanged(t *testing.T) {
	input := "This is a normal card body.\n\nWith a second paragraph, and *markdown* too."

	got := sanitizeControlSequences(input)

	if got != input {
		t.Errorf("sanitizeControlSequences(%q) = %q, want unchanged for clean text", input, got)
	}
}

// --- Single-line sanitization (#499) ---
//
// sanitizeSingleLine is the sink-side counterpart to sanitizeControlSequences
// for fields that must always render on exactly one physical line (status-bar
// messages). It strips ANSI/OSC/CSI/DCS escapes first (so an escape sequence
// containing a literal newline cannot smuggle a line break through), then
// flattens every whitespace-like line/paragraph separator to a single space,
// removes bidi-control and zero-width runes entirely (no residual space),
// collapses runs of whitespace, and trims the result. It is idempotent:
// re-sanitizing an already-sanitized string is a no-op.

// TestSanitizeSingleLine_WhitespaceRunesFlattenToSpace covers every rune the
// ticket calls out as a line/paragraph/whitespace separator: \n, \r, \t,
// U+0085 (NEL), U+00A0 (NBSP), U+2028 (LINE SEPARATOR), U+2029 (PARAGRAPH
// SEPARATOR), and U+3000 (IDEOGRAPHIC SPACE). Each must flatten to a single
// space between its neighbors, and the result must contain no embedded '\n'.
func TestSanitizeSingleLine_WhitespaceRunesFlattenToSpace(t *testing.T) {
	whitespaceRunes := map[string]rune{
		"LF (U+000A)":                  '\n',
		"CR (U+000D)":                  '\r',
		"TAB (U+0009)":                 '\t',
		"NEL (U+0085)":                 '\u0085',
		"NBSP (U+00A0)":                '\u00A0',
		"LINE SEPARATOR (U+2028)":      ' ',
		"PARAGRAPH SEPARATOR (U+2029)": ' ',
		"IDEOGRAPHIC SPACE (U+3000)":   '　',
	}
	for name, r := range whitespaceRunes {
		t.Run(name, func(t *testing.T) {
			input := "a" + string(r) + "b"
			got := sanitizeSingleLine(input)
			if got != "a b" {
				t.Errorf("sanitizeSingleLine(%q) = %q, want %q", input, got, "a b")
			}
			if strings.ContainsRune(got, '\n') {
				t.Errorf("sanitizeSingleLine(%q) = %q, want no embedded newline", input, got)
			}
		})
	}
}

// TestSanitizeSingleLine_BidiAndZeroWidthRunesRemoved covers every
// bidi-control/zero-width rune the ticket calls out for removal: U+061C,
// U+200B-U+200F, U+202A-U+202E, U+2060, U+2066-U+2069, and U+FEFF. These must
// be removed entirely -- unlike the whitespace runes above, they leave no
// residual space in their place.
func TestSanitizeSingleLine_BidiAndZeroWidthRunesRemoved(t *testing.T) {
	removedRunes := map[string]rune{
		"ARABIC LETTER MARK (U+061C)":              '؜',
		"ZERO WIDTH SPACE (U+200B)":                '​',
		"ZERO WIDTH NON-JOINER (U+200C)":           '‌',
		"ZERO WIDTH JOINER (U+200D)":               '‍',
		"LEFT-TO-RIGHT MARK (U+200E)":              '‎',
		"RIGHT-TO-LEFT MARK (U+200F)":              '‏',
		"LEFT-TO-RIGHT EMBEDDING (U+202A)":         '‪',
		"RIGHT-TO-LEFT EMBEDDING (U+202B)":         '‫',
		"POP DIRECTIONAL FORMATTING (U+202C)":      '‬',
		"LEFT-TO-RIGHT OVERRIDE (U+202D)":          '‭',
		"RIGHT-TO-LEFT OVERRIDE (U+202E)":          '‮',
		"WORD JOINER (U+2060)":                     '⁠',
		"LEFT-TO-RIGHT ISOLATE (U+2066)":           '⁦',
		"RIGHT-TO-LEFT ISOLATE (U+2067)":           '⁧',
		"FIRST STRONG ISOLATE (U+2068)":            '⁨',
		"POP DIRECTIONAL ISOLATE (U+2069)":         '⁩',
		"ZERO WIDTH NO-BREAK SPACE / BOM (U+FEFF)": '\uFEFF',
	}
	for name, r := range removedRunes {
		t.Run(name, func(t *testing.T) {
			input := "a" + string(r) + "b"
			got := sanitizeSingleLine(input)
			if got != "ab" {
				t.Errorf("sanitizeSingleLine(%q) = %q, want %q (removed with no residual space)", input, got, "ab")
			}
		})
	}
}

// TestSanitizeSingleLine_CollapsesWhitespaceRunsAndTrims verifies runs of
// whitespace (including multiple embedded newlines) collapse to a single
// space, and leading/trailing whitespace is trimmed.
func TestSanitizeSingleLine_CollapsesWhitespaceRunsAndTrims(t *testing.T) {
	input := "  a\n\n\nb  "
	got := sanitizeSingleLine(input)
	if got != "a b" {
		t.Errorf("sanitizeSingleLine(%q) = %q, want %q", input, got, "a b")
	}
}

// TestSanitizeSingleLine_Idempotent verifies sanitizing an already-sanitized
// string is a no-op, using a fixture combining ANSI, whitespace-separator,
// bidi/zero-width, and run-collapsing cases together.
func TestSanitizeSingleLine_Idempotent(t *testing.T) {
	input := "  \u200ba\n\n\tb\u2028c\u00a0\u202ed  \x1b[31mRED\x1b[0m  "
	once := sanitizeSingleLine(input)
	twice := sanitizeSingleLine(once)
	if twice != once {
		t.Errorf("sanitizeSingleLine(sanitizeSingleLine(%q)) = %q, want it to equal sanitizeSingleLine(%q) = %q", input, twice, input, once)
	}
}

// TestSanitizeSingleLine_ANSIStrippedBeforeFlatten verifies ANSI stripping
// happens BEFORE whitespace-flattening: an OSC-8 hyperlink escape sequence
// containing a literal embedded newline must not leak that newline through
// once the escape is stripped -- ansi.Strip consumes the whole OSC sequence
// (including the newline inside it) along with the hyperlink target, leaving
// only the visible label.
func TestSanitizeSingleLine_ANSIStrippedBeforeFlatten(t *testing.T) {
	input := "\x1b]8;;https://ev\nil\x07label\x1b]8;;\x07"
	got := sanitizeSingleLine(input)
	if got != "label" {
		t.Errorf("sanitizeSingleLine(%q) = %q, want %q", input, got, "label")
	}
}

// TestSanitizeSingleLine_WhitespaceOnlyYieldsEmptyString verifies a string
// made entirely of whitespace/separator runes sanitizes to the empty string
// (not a lingering single space), so callers can treat it as "no message".
func TestSanitizeSingleLine_WhitespaceOnlyYieldsEmptyString(t *testing.T) {
	input := "  \n\t   "
	got := sanitizeSingleLine(input)
	if got != "" {
		t.Errorf("sanitizeSingleLine(%q) = %q, want empty string", input, got)
	}
}

// TestSanitizeSingleLine_CleanTextUnchanged verifies clean, single-line text
// with no control/whitespace-separator/bidi runes passes through unchanged.
func TestSanitizeSingleLine_CleanTextUnchanged(t *testing.T) {
	input := "This is a normal single-line message."
	got := sanitizeSingleLine(input)
	if got != input {
		t.Errorf("sanitizeSingleLine(%q) = %q, want unchanged for clean text", input, got)
	}
}
