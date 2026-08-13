package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestPrintNotices_TwoNoticesOnePerLineInOrder verifies that each notice
// in the slice is written to w on its own line, preserving input order.
func TestPrintNotices_TwoNoticesOnePerLineInOrder(t *testing.T) {
	var buf bytes.Buffer
	notices := []string{"first notice", "second notice"}

	printNotices(&buf, notices)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(notices) {
		t.Fatalf("expected %d lines, got %d: %q", len(notices), len(lines), buf.String())
	}
	for i, notice := range notices {
		if !strings.Contains(lines[i], notice) {
			t.Errorf("line %d = %q, expected to contain %q (order preserved)", i, lines[i], notice)
		}
	}
}

// TestPrintNotices_NilSliceWritesNothing verifies that a nil notices
// slice results in zero bytes written to w.
func TestPrintNotices_NilSliceWritesNothing(t *testing.T) {
	var buf bytes.Buffer

	printNotices(&buf, nil)

	if buf.Len() != 0 {
		t.Errorf("expected zero bytes written for nil slice, got %d bytes: %q", buf.Len(), buf.String())
	}
}

// TestPrintNotices_EmptySliceWritesNothing verifies that an empty (but
// non-nil) notices slice results in zero bytes written to w.
func TestPrintNotices_EmptySliceWritesNothing(t *testing.T) {
	var buf bytes.Buffer

	printNotices(&buf, []string{})

	if buf.Len() != 0 {
		t.Errorf("expected zero bytes written for empty slice, got %d bytes: %q", buf.Len(), buf.String())
	}
}

// TestPrintNotices_ControlBytesFlattenToOneLine verifies that a notice
// containing embedded \n, \r, and an ESC control byte is flattened to exactly
// one physical output line, with none of those control bytes surviving in
// the output. This asserts structural invariants (line count, absence of
// control bytes) per .claude/rules/testing.md, not a byte-for-byte constant
// copied from sanitizeSingleLine's own implementation or test table.
func TestPrintNotices_ControlBytesFlattenToOneLine(t *testing.T) {
	var buf bytes.Buffer
	notice := "hostile notice \n mid-notice\r tail\x1b[31m red"

	printNotices(&buf, []string{notice})

	out := buf.String()
	trimmed := strings.TrimRight(out, "\n")
	if strings.Count(trimmed, "\n") != 0 {
		t.Errorf("expected exactly one physical output line, got %d embedded newlines in %q", strings.Count(trimmed, "\n"), out)
	}
	for _, b := range []byte(out) {
		if b == '\n' {
			continue // the single trailing Fprintln newline is expected
		}
		if b < 0x20 || b == 0x7f {
			t.Errorf("output contains control byte 0x%02x, expected none to survive: %q", b, out)
		}
	}
}

// TestPrintNotices_MultipleGroupsOrderedSkippingEmpty verifies printNotices'
// multi-group contract (#568, commit 2): every group is printed in the order
// given, one sanitized line per entry, and nil/empty groups in between
// contribute zero lines without disrupting the ordering of the surrounding
// groups.
func TestPrintNotices_MultipleGroupsOrderedSkippingEmpty(t *testing.T) {
	var buf bytes.Buffer
	first := []string{"first group line one", "first group line two"}
	notices := []string{"untrusted .lazyboards.yml: stripped 1 keymap shell binding(s)"}

	printNotices(&buf, first, nil, notices, []string{})

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	want := append(append([]string{}, first...), notices...)
	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %q", len(want), len(lines), buf.String())
	}
	for i, expected := range want {
		if !strings.Contains(lines[i], expected) {
			t.Errorf("line %d = %q, expected to contain %q (group order preserved, nil/empty groups skipped)", i, lines[i], expected)
		}
	}
}
