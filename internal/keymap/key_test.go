package keymap

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestParseKey_AcceptsACNotations pins the AC1 notation surface: single
// runes and the named multi-key notations bubbletea's Key.String() emits.
func TestParseKey_AcceptsACNotations(t *testing.T) {
	for _, s := range []string{"n", "ctrl+a", "shift+tab", "alt+j", "esc"} {
		k, err := ParseKey(s)
		if err != nil {
			t.Errorf("ParseKey(%q) returned unexpected error: %v", s, err)
			continue
		}
		if string(k) != s {
			t.Errorf("ParseKey(%q) = %q, want %q", s, k, s)
		}
	}
}

// TestParseKey_AcceptsSingleRunes covers the single-rune branch across
// letters (upper/lower), digits, and punctuation -- none of these are
// named entries in bubbletea's KeyType enumeration.
func TestParseKey_AcceptsSingleRunes(t *testing.T) {
	for _, s := range []string{"n", "J", "/", "1", "?", "."} {
		k, err := ParseKey(s)
		if err != nil {
			t.Errorf("ParseKey(%q) returned unexpected error: %v", s, err)
			continue
		}
		if string(k) != s {
			t.Errorf("ParseKey(%q) = %q, want %q", s, k, s)
		}
	}
}

// TestParseKey_RejectsRunesSentinel pins that bubbletea's internal
// KeyRunes.String() value ("runes") is not itself a valid key notation --
// it's a KeyType label, never a value Key.String() actually emits for a
// real keypress.
func TestParseKey_RejectsRunesSentinel(t *testing.T) {
	assertParseKeyRejects(t, "runes")
}

// TestParseKey_RejectsUppercaseCtrlNotation pins that name matching is
// case-sensitive: bubbletea always prints lowercase notations, and
// accepting "Ctrl+A" would reintroduce the translation layer #484 rejected.
func TestParseKey_RejectsUppercaseCtrlNotation(t *testing.T) {
	assertParseKeyRejects(t, "Ctrl+A")
}

// TestParseKey_RejectsUnknownName pins that an arbitrary non-notation
// string is rejected.
func TestParseKey_RejectsUnknownName(t *testing.T) {
	assertParseKeyRejects(t, "foo")
}

// TestParseKey_RejectsEmptyString pins the explicit empty-input error.
func TestParseKey_RejectsEmptyString(t *testing.T) {
	assertParseKeyRejects(t, "")
}

// TestParseKey_RejectsLoneSpace pins the documented space-key trap: a lone
// space collides with the sequence separator (Sequence.String() joins on
// " "), so ParseKey must reject it explicitly rather than accept a key
// ParseSequence could never round-trip.
func TestParseKey_RejectsLoneSpace(t *testing.T) {
	assertParseKeyRejects(t, " ")
}

// assertParseKeyRejects asserts ParseKey(s) returns a non-nil error whose
// message names the offending key, per the plan's error-message contract.
func assertParseKeyRejects(t *testing.T, s string) {
	t.Helper()
	_, err := ParseKey(s)
	if err == nil {
		t.Fatalf("ParseKey(%q) returned nil error, want an error", s)
	}
	if !strings.Contains(err.Error(), s) {
		t.Errorf("ParseKey(%q) error = %q, want it to name the offending key %q", s, err.Error(), s)
	}
}

// TestParseKey_EveryBubbleteaKeyTypeNameParses is the generative test the
// plan calls for: every non-empty name bubbletea's own KeyType.String()
// yields (except the "runes" sentinel, rejected above) must parse, so the
// accepted-name set is derived from bubbletea's enumeration rather than a
// hand-copied table that could silently drift. Iterating [-256, 255]
// covers KeyF20 (bubbletea's most negative named KeyType, -53) with a wide
// safety margin, per the plan's stated risk.
func TestParseKey_EveryBubbleteaKeyTypeNameParses(t *testing.T) {
	names := bubbleteaKeyTypeNames(t)

	if len(names) < 50 {
		t.Fatalf("derived only %d bubbletea key names, want > 50 -- the derivation range may have shrunk", len(names))
	}

	for _, name := range names {
		if strings.Contains(name, " ") && name != " " {
			t.Errorf("derived key name %q unexpectedly contains a space", name)
		}
		if name == " " {
			// The space key itself is asserted un-parseable elsewhere
			// (TestParseKey_RejectsLoneSpace); skip it here.
			continue
		}
		if _, err := ParseKey(name); err != nil {
			t.Errorf("ParseKey(%q) returned unexpected error: %v (name derived from tea.KeyType.String())", name, err)
		}
	}
}

// TestParseKey_OnlySpaceNameContainsASpace cross-checks the generative
// enumeration against the plan's stated invariant directly (rather than
// only skipping it inline above), so a regression that introduces a second
// space-containing name is caught explicitly.
func TestParseKey_OnlySpaceNameContainsASpace(t *testing.T) {
	names := bubbleteaKeyTypeNames(t)
	for _, name := range names {
		if name == " " {
			continue
		}
		if strings.Contains(name, " ") {
			t.Errorf("derived key name %q contains a space but is not the literal space key", name)
		}
	}
}

// bubbleteaKeyTypeNames derives the full set of non-empty names
// tea.KeyType(i).String() yields over i in [-256, 255], dropping
// "runes" (KeyRunes.String(), which is a KeyType label, not a real
// notation). This is the same derivation ParseKey's production
// implementation must use, asserted here against bubbletea's own output
// rather than a hand-copied table (never copy expected values across a
// package boundary).
func bubbleteaKeyTypeNames(t *testing.T) []string {
	t.Helper()
	var names []string
	seen := make(map[string]bool)
	for i := -256; i <= 255; i++ {
		s := tea.KeyType(i).String()
		if s == "" || s == "runes" {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		names = append(names, s)
	}
	return names
}

// TestParseKey_StripsAtMostOneLeadingAltPrefix pins step 2 of the parsing
// rule: "alt+j" strips to a valid single-rune remainder "j". A malformed
// double-alt string is rejected rather than silently stripped twice.
func TestParseKey_StripsAtMostOneLeadingAltPrefix(t *testing.T) {
	k, err := ParseKey("alt+j")
	if err != nil {
		t.Fatalf("ParseKey(\"alt+j\") returned unexpected error: %v", err)
	}
	if string(k) != "alt+j" {
		t.Errorf("ParseKey(\"alt+j\") = %q, want %q", k, "alt+j")
	}

	assertParseKeyRejects(t, "alt+alt+j")
}

// TestParseKey_RejectsAltSpace pins that the lone-space guard applies to the
// post-strip remainder, not just the raw input: "alt+ " strips to a lone
// space, which would otherwise pass the single-rune rule but can never
// round-trip through Sequence.String()/ParseSequence (it collides with the
// space separator), same as the bare " " case.
func TestParseKey_RejectsAltSpace(t *testing.T) {
	assertParseKeyRejects(t, "alt+ ")
}

// TestParseKey_RejectsZeroWidthSpace pins that a Unicode "Cf" (format)
// category rune -- here U+200B ZERO WIDTH SPACE -- is rejected in the
// single-rune branch even though it would otherwise pass the rune-count
// check. These runes can never correspond to a real msg.String() keypress
// and risk bidi-spoofing/control-sequence injection into rendered help/
// which-key labels.
//
// Unlike assertParseKeyRejects's raw strings.Contains check, this asserts
// against the error's %q-escaped rendering of the offending key: %q
// deliberately escapes non-printable/format runes rather than embedding
// them raw, which is itself part of the fix -- the error text must never
// echo a raw invisible/bidi-control character.
func TestParseKey_RejectsZeroWidthSpace(t *testing.T) {
	assertParseKeyRejectsFormatRune(t, "\u200b")
}

// TestParseKey_RejectsBidiOverride pins that a bidi-control rune -- here
// U+202E RIGHT-TO-LEFT OVERRIDE, also in the Cf category -- is rejected for
// the same reason as TestParseKey_RejectsZeroWidthSpace.
func TestParseKey_RejectsBidiOverride(t *testing.T) {
	assertParseKeyRejectsFormatRune(t, "\u202e")
}

// assertParseKeyRejectsFormatRune asserts ParseKey(s) returns a non-nil
// error whose message names the offending key via its %q-escaped form (used
// for non-printable/format runes, where the error must not embed the raw
// character itself).
func assertParseKeyRejectsFormatRune(t *testing.T, s string) {
	t.Helper()
	_, err := ParseKey(s)
	if err == nil {
		t.Fatalf("ParseKey(%q) returned nil error, want an error", s)
	}
	want := fmt.Sprintf("%q", s)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("ParseKey(%q) error = %q, want it to name the offending key as %s", s, err.Error(), want)
	}
}

// AC1's ParseKey rejection threat set has six cases: control byte, ANSI CSI
// escape sequence, bidi override (U+202E), zero-width space (U+200B),
// invalid UTF-8, and a lone space (including the alt+-prefixed variant).
// Bidi override and zero-width space are covered above by
// TestParseKey_RejectsBidiOverride and TestParseKey_RejectsZeroWidthSpace;
// invalid UTF-8 is covered below by TestParseKey_RejectsInvalidUTF8LeadByte
// and TestParseKey_RejectsInvalidUTF8ContinuationByte; lone space is covered
// by TestParseKey_RejectsLoneSpace and TestParseKey_RejectsAltSpace. This
// ticket adds only the two remaining cases: control byte
// (TestParseKey_RejectsControlByte) and ANSI CSI sequence
// (TestParseKey_RejectsANSICSISequence).

// TestParseKey_RejectsControlByte pins that a raw control byte -- here
// \x1b (ESC), the introducer for both C0 escape sequences and standalone
// C1-range bytes -- is rejected the same way as the other invisible/format
// runes above, so it can never be canonicalized into a Key later rendered
// in help/which-key labels.
//
// Uses assertParseKeyRejectsFormatRune (the %q-escaping variant), not
// assertParseKeyRejects: the plain helper's raw strings.Contains check
// against the unescaped "\x1b" byte would fail here, since the error
// message embeds the key via %q, which escapes ESC rather than including
// it literally.
func TestParseKey_RejectsControlByte(t *testing.T) {
	assertParseKeyRejectsFormatRune(t, "\x1b")
}

// TestParseKey_RejectsANSICSISequence pins that a full ANSI CSI escape
// sequence -- here "\x1b[31m", an SGR color-setting sequence -- is rejected
// as a whole, not just its leading ESC byte: ParseKey's single-rune branch
// never matches a multi-byte string in the first place, so the multi-rune
// rejection path covers this case the same as any other multi-character
// non-notation string.
func TestParseKey_RejectsANSICSISequence(t *testing.T) {
	assertParseKeyRejectsFormatRune(t, "\x1b[31m")
}

// TestParseKey_AcceptsPrintableRune pins that an ordinary printable,
// non-format rune still passes the single-rune branch after the
// printability/Cf-category guard is added.
func TestParseKey_AcceptsPrintableRune(t *testing.T) {
	k, err := ParseKey("n")
	if err != nil {
		t.Fatalf("ParseKey(\"n\") returned unexpected error: %v", err)
	}
	if string(k) != "n" {
		t.Errorf("ParseKey(\"n\") = %q, want %q", k, "n")
	}
}

// TestParseKey_RejectsInvalidUTF8LeadByte pins that a malformed single byte
// that utf8.DecodeRuneInString cannot decode -- here 0xFF, never a valid
// UTF-8 lead byte -- is rejected explicitly. Without the fix,
// DecodeRuneInString returns (utf8.RuneError, 1) for it, and because
// unicode.IsPrint(utf8.RuneError) is true (U+FFFD is category So), the
// single-rune branch would otherwise accept the raw invalid byte verbatim as
// a Key, later rendered in help/which-key labels.
func TestParseKey_RejectsInvalidUTF8LeadByte(t *testing.T) {
	assertParseKeyRejectsFormatRune(t, "\xff")
}

// TestParseKey_RejectsInvalidUTF8ContinuationByte pins the same guard for
// 0x80, a UTF-8 continuation byte that can never start a valid encoding on
// its own.
func TestParseKey_RejectsInvalidUTF8ContinuationByte(t *testing.T) {
	assertParseKeyRejectsFormatRune(t, "\x80")
}

// TestParseKey_AcceptsValidReplacementCharacterRune pins that a genuine,
// validly-3-byte-encoded U+FFFD REPLACEMENT CHARACTER rune is still accepted
// -- only the invalid-encoding sentinel (RuneError paired with size == 1)
// is rejected, not the rune value itself when it arrives via a well-formed
// encoding.
func TestParseKey_AcceptsValidReplacementCharacterRune(t *testing.T) {
	s := "�"
	k, err := ParseKey(s)
	if err != nil {
		t.Fatalf("ParseKey(%q) returned unexpected error: %v", s, err)
	}
	if string(k) != s {
		t.Errorf("ParseKey(%q) = %q, want %q", s, k, s)
	}
}

// TestSequence_String pins the canonical space-joined form used as the
// Table map key.
func TestSequence_String(t *testing.T) {
	seq := Sequence{Key("g"), Key("d")}
	if got := seq.String(); got != "g d" {
		t.Errorf("Sequence.String() = %q, want %q", got, "g d")
	}
}

// TestParseSequence_MultiKey pins "g d" -> a 2-key ordered sequence.
func TestParseSequence_MultiKey(t *testing.T) {
	seq, err := ParseSequence("g d")
	if err != nil {
		t.Fatalf("ParseSequence(\"g d\") returned unexpected error: %v", err)
	}
	want := Sequence{Key("g"), Key("d")}
	if len(seq) != len(want) {
		t.Fatalf("ParseSequence(\"g d\") = %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Errorf("ParseSequence(\"g d\")[%d] = %q, want %q", i, seq[i], want[i])
		}
	}
}

// TestParseSequence_SingleKey pins "n" -> a one-element sequence.
func TestParseSequence_SingleKey(t *testing.T) {
	seq, err := ParseSequence("n")
	if err != nil {
		t.Fatalf("ParseSequence(\"n\") returned unexpected error: %v", err)
	}
	if len(seq) != 1 || seq[0] != Key("n") {
		t.Errorf("ParseSequence(\"n\") = %v, want [n]", seq)
	}
}

// TestParseSequence_NormalizesStrayWhitespace pins that ParseSequence
// splits on strings.Fields, so stray/duplicate whitespace normalizes away
// and the canonical form matches the tidy two-key input.
func TestParseSequence_NormalizesStrayWhitespace(t *testing.T) {
	seq, err := ParseSequence("  g   d ")
	if err != nil {
		t.Fatalf("ParseSequence(\"  g   d \") returned unexpected error: %v", err)
	}
	want := Sequence{Key("g"), Key("d")}
	if len(seq) != len(want) {
		t.Fatalf("ParseSequence(\"  g   d \") = %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Errorf("ParseSequence(\"  g   d \")[%d] = %q, want %q", i, seq[i], want[i])
		}
	}
	if got := seq.String(); got != "g d" {
		t.Errorf("normalized Sequence.String() = %q, want %q", got, "g d")
	}
}

// TestParseSequence_ErrorNamesOffendingKeyAndWholeSequence pins the error
// contract: a bad key inside a multi-key sequence is named specifically,
// alongside the whole offending sequence string, so a user misconfiguring
// "g foo" gets a diagnosable message.
func TestParseSequence_ErrorNamesOffendingKeyAndWholeSequence(t *testing.T) {
	_, err := ParseSequence("g foo")
	if err == nil {
		t.Fatal("ParseSequence(\"g foo\") returned nil error, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "foo") {
		t.Errorf("ParseSequence(\"g foo\") error = %q, want it to name the offending key %q", msg, "foo")
	}
	if !strings.Contains(msg, "g foo") {
		t.Errorf("ParseSequence(\"g foo\") error = %q, want it to name the whole sequence %q", msg, "g foo")
	}
}

// TestParseSequence_RejectsEmptyString pins that an empty sequence string
// (no keys at all) is an error, not a valid zero-length Sequence -- an
// empty Sequence would be an unreachable/meaningless Lookup input.
func TestParseSequence_RejectsEmptyString(t *testing.T) {
	if _, err := ParseSequence(""); err == nil {
		t.Fatal("ParseSequence(\"\") returned nil error, want an error")
	}
}
