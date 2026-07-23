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
