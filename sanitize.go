package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// sanitizeControlSequences strips terminal control sequences from untrusted
// text (e.g. a GitHub issue/PR body) before it is ever handed to glamour.
//
// It runs in three passes:
//  1. A strings.Map pass drops standalone C1 control bytes (0x80-0x9F, e.g.
//     a lone single-byte CSI introducer 0x9B) up front. ansi.Strip treats a
//     bare C1 byte as the start of an escape sequence and would consume
//     following visible bytes as its "final byte", corrupting text that was
//     never actually part of an escape sequence.
//  2. ansi.Strip removes ANSI/OSC/CSI/DCS escape sequences while keeping the
//     visible text they wrapped (e.g. an SGR color code around "RED" leaves
//     "RED" behind; an OSC-8 hyperlink leaves only its visible label).
//  3. A final strings.Map pass drops any remaining standalone C0 control
//     runes (0x00-0x1F) plus DEL (0x7F), since ansi.Strip alone leaves a
//     lone control byte (e.g. a stray BEL not part of a recognized escape
//     sequence) behind. \n and \t are preserved throughout so markdown
//     structure and code indentation survive.
//
// Ordering contract: callers must sanitize the raw untrusted body BEFORE
// passing it to annotateBodyRefs, so the injected "[a]"-style reference
// labels (added after sanitization) are not themselves stripped.
func sanitizeControlSequences(s string) string {
	withoutC1 := strings.Map(func(r rune) rune {
		if r >= 0x80 && r <= 0x9F {
			return -1
		}
		return r
	}, s)
	stripped := ansi.Strip(withoutC1)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if (r >= 0x00 && r <= 0x1F) || r == 0x7F {
			return -1
		}
		return r
	}, stripped)
}

// flattenToSingleLine collapses \n, \r, and \t into a single space. Unlike
// sanitizeControlSequences (which deliberately preserves \n/\t for
// multi-line card.Body rendering), callers that render untrusted text into a
// single-line field -- e.g. a fixed-cell row grid or a one-line status
// message -- must flatten it first, or an embedded newline/tab can visually
// spoof an extra row or break the field's layout. Apply this AFTER
// sanitizeControlSequences (so ANSI escapes are stripped first) and BEFORE
// any length truncation.
func flattenToSingleLine(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
}

// sanitizeSingleLine sanitizes untrusted text destined for a field that must
// render as exactly one physical terminal line (e.g. a status-bar message).
// It runs in four passes:
//  1. A strings.Map pre-pass converts \r (U+000D) and U+0085 (NEL) to a plain
//     space. This must happen BEFORE sanitizeControlSequences, because that
//     function's C0/C1 strip (pass 2 below) would otherwise delete \r and NEL
//     outright rather than treating them as line separators, silently joining
//     the text on either side with no space between.
//  2. sanitizeControlSequences strips C1 control bytes, ANSI/OSC/CSI/DCS
//     escape sequences, and remaining C0/DEL control bytes, while preserving
//     \n and \t (see its own doc comment). Running this before the
//     whitespace-flattening passes below ensures an escape sequence smuggling
//     a literal newline inside it (e.g. an OSC-8 hyperlink target) is removed
//     as a unit, rather than having its embedded newline flattened into a
//     stray space that leaks part of the escape sequence's payload.
//  3. A strings.Map pass drops -- entirely, with no replacement space -- the
//     bidi-control and zero-width runes: U+061C, U+200B-U+200F, U+202A-U+202E,
//     U+2060, U+2066-U+2069, and U+FEFF. These are invisible and carry no
//     layout meaning for a single-line field, so (unlike the line/paragraph
//     separators in pass 1) they are removed rather than replaced with a
//     space.
//  4. strings.Join(strings.Fields(s), " ") collapses every run of
//     unicode.IsSpace runes -- \n, \t, U+00A0 (NBSP), U+2028 (LINE
//     SEPARATOR), U+2029 (PARAGRAPH SEPARATOR), U+3000 (IDEOGRAPHIC SPACE),
//     U+205F, U+2000-U+200A, and the plain spaces introduced by pass 1 -- to a
//     single space, and trims leading/trailing whitespace.
//
// Ordering contract: callers must sanitize with this function BEFORE any
// length truncation, mirroring flattenToSingleLine's contract, so truncation
// operates on the final flattened text rather than being able to cut in the
// middle of content that sanitization would otherwise have collapsed.
//
// sanitizeSingleLine is idempotent: re-sanitizing an already-sanitized string
// is a no-op, since none of the four passes can produce output that a
// subsequent pass would further change.
//
// A whitespace-only input sanitizes to "" (the empty string, not a lingering
// single space). This is an intended contract, not a bug: callers that use
// "" to mean "no message set" (e.g. StatusBar's sticky/timed message fields)
// can rely on a whitespace-only message being treated the same as an unset
// one.
func sanitizeSingleLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\u0085' {
			return ' '
		}
		return r
	}, s)

	s = sanitizeControlSequences(s)

	s = strings.Map(func(r rune) rune {
		if isBidiOrZeroWidthRune(r) {
			return -1
		}
		return r
	}, s)

	return strings.Join(strings.Fields(s), " ")
}

// isBidiOrZeroWidthRune reports whether r is one of the bidi-control or
// zero-width runes sanitizeSingleLine removes entirely (pass 3): U+061C,
// U+200B-U+200F, U+202A-U+202E, U+2060, U+2066-U+2069, and U+FEFF.
func isBidiOrZeroWidthRune(r rune) bool {
	switch {
	case r == 0x061C:
		return true
	case r >= 0x200B && r <= 0x200F:
		return true
	case r >= 0x202A && r <= 0x202E:
		return true
	case r == 0x2060:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0xFEFF:
		return true
	default:
		return false
	}
}
