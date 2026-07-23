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
