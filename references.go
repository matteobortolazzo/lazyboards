package main

import (
	"regexp"
	"strconv"
)

// maxRefDigits bounds the accepted digit-run length for a #N reference.
// Real GitHub issue/PR numbers never come close to this many digits; the
// bound exists to guarantee strconv.Atoi cannot overflow (see scanRefs).
const maxRefDigits = 10

// cardRef is a distinct #N reference found in a card body, assigned a
// stable single-letter label ('a'-'z') in first-appearance order.
type cardRef struct {
	Number int
	Label  rune
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
