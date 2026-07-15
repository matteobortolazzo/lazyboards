package main

import "testing"

func TestMapSlice_TransformsEachElement(t *testing.T) {
	in := []int{1, 2, 3}

	got := mapSlice(in, func(n int) string {
		if n == 1 {
			return "one"
		}
		return "many"
	})

	if len(got) != len(in) {
		t.Fatalf("mapSlice() returned %d elements, want %d", len(got), len(in))
	}
	if got[0] != "one" || got[1] != "many" || got[2] != "many" {
		t.Errorf("mapSlice() = %v, want transformed elements", got)
	}
}

func TestMapSlice_NilInput_ReturnsNil(t *testing.T) {
	got := mapSlice[int, string](nil, func(n int) string { return "x" })

	if got != nil {
		t.Errorf("mapSlice(nil) = %v, want nil", got)
	}
}

func TestMapSlice_EmptyInput_ReturnsNil(t *testing.T) {
	got := mapSlice([]int{}, func(n int) string { return "x" })

	if got != nil {
		t.Errorf("mapSlice([]int{}) = %v, want nil (not an empty non-nil slice)", got)
	}
}
