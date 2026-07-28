package keymap

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// Key is a single key notation exactly as BubbleTea's Key.String() emits
// it -- there is no separate translation layer (no `<C-a>` notation, no
// `<leader>`). At dispatch time (#489+) no parsing is needed: msg.String()
// is already canonical, so callers build Sequence{Key(msg.String())}
// directly.
type Key string

// Sequence is an ordered list of Keys, matched in order against a Table.
type Sequence []Key

// String returns the canonical, space-joined form of the sequence -- the
// form used as a Table map key.
func (s Sequence) String() string {
	parts := make([]string, len(s))
	for i, k := range s {
		parts[i] = string(k)
	}
	return strings.Join(parts, " ")
}

// validKeyNames is the set of multi-character key notations BubbleTea's own
// KeyType enumeration can emit, derived at init time rather than hand-
// copied (a hand-copied table would silently drift as bubbletea adds key
// types). It excludes "" (unknown/unused KeyType values) and "runes"
// (KeyRunes.String(), a KeyType label, never a value Key.String() actually
// emits for a keypress).
var validKeyNames = func() map[string]bool {
	names := make(map[string]bool)
	for i := -256; i <= 255; i++ {
		s := tea.KeyType(i).String()
		if s == "" || s == tea.KeyRunes.String() {
			continue
		}
		names[s] = true
	}
	return names
}()

// ParseKey parses a single BubbleTea key notation. Rules:
//  1. empty string -> error;
//  2. strip at most one leading "alt+" (BubbleTea's Key.String() prepends
//     exactly one, never two);
//  3. the remainder is valid if it is a single printable, non-format rune
//     ("n", "J", "/", "1") or an exact, case-sensitive member of
//     validKeyNames ("esc", "enter", "tab", "shift+tab", "ctrl+a", "pgup",
//     "f1", ...);
//  4. otherwise -> error naming the offending key.
//
// A lone space is rejected explicitly even though it would otherwise pass
// the single-rune rule: it collides with Sequence.String()'s separator, so
// ParseSequence could never round-trip it. The check applies to the
// post-strip remainder too, so "alt+ " is rejected the same way.
//
// The single-rune branch also rejects non-printable runes and Unicode "Cf"
// (format) category runes -- zero-width joiners/spaces, bidi overrides and
// isolates, etc. None of these can correspond to a real msg.String()
// keypress, and admitting them would risk control-sequence/bidi-spoofing
// injection into rendered help/which-key labels downstream.
func ParseKey(s string) (Key, error) {
	if s == "" {
		return "", fmt.Errorf("keymap: empty key")
	}
	if s == " " {
		return "", fmt.Errorf("keymap: key %q is not bindable (space collides with the sequence separator)", s)
	}

	remainder := strings.TrimPrefix(s, "alt+")
	if remainder == " " {
		return "", fmt.Errorf("keymap: key %q is not bindable (space collides with the sequence separator)", s)
	}

	if remainder != "" {
		if r, size := utf8.DecodeRuneInString(remainder); size == len(remainder) {
			if r == utf8.RuneError && size == 1 {
				// DecodeRuneInString's standard sentinel for a byte
				// sequence it cannot decode at all (e.g. a lone 0xFF lead
				// byte or a stray 0x80 continuation byte). Must be checked
				// before/alongside IsPrint: unicode.IsPrint(utf8.RuneError)
				// is true (U+FFFD is category So), so without this an
				// invalid byte would otherwise be accepted verbatim as a
				// Key. A genuine, validly-encoded U+FFFD rune (size > 1)
				// still passes through to the checks below.
				return "", fmt.Errorf("keymap: invalid key %q", s)
			}
			// unicode.Is(unicode.Cf, r) is redundant with IsPrint above --
			// IsPrint already excludes the entire Cf category (verified
			// against U+200B and U+202E, the format runes this guards
			// against). Kept as explicit defense-in-depth in case Go's
			// Unicode category tables are ever reclassified; don't drop
			// IsPrint to "simplify" this instead.
			if !unicode.IsPrint(r) || unicode.Is(unicode.Cf, r) {
				return "", fmt.Errorf("keymap: invalid key %q", s)
			}
			return Key(s), nil
		}
	}
	if validKeyNames[remainder] {
		return Key(s), nil
	}
	return "", fmt.Errorf("keymap: invalid key %q", s)
}

// ParseSequence parses a space-separated key sequence ("g d" -> a 2-key
// ordered sequence, "n" -> a one-element sequence). It splits on
// strings.Fields, so stray/duplicate whitespace normalizes away. An empty
// (or all-whitespace) string is an error, not a zero-length Sequence --
// Lookup would never receive one. A bad key inside the sequence is named
// specifically, alongside the whole offending sequence string.
func ParseSequence(s string) (Sequence, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("keymap: empty key sequence %q", s)
	}

	seq := make(Sequence, len(fields))
	for i, f := range fields {
		k, err := ParseKey(f)
		if err != nil {
			return nil, fmt.Errorf("keymap: invalid key %q in sequence %q: %w", f, s, err)
		}
		seq[i] = k
	}
	return seq, nil
}
