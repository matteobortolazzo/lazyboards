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

// --- sortFold / sortFoldStrings (#477) ---
//
// These are unit tests (not integration) because the stable-tie property for
// case-only duplicates is not reliably observable through the deduped filter
// picker surface (collectFilterItems removes case-only duplicates before
// sorting), so it must be verified directly against the helper.

// sortFoldItem is a small struct-keyed fixture standing in for the assign
// modal's assignItem, which is the one call site that sorts a struct slice
// rather than a plain []string.
type sortFoldItem struct {
	name string
}

func TestSortFold_MixedCaseAlphabeticalOrder(t *testing.T) {
	in := []sortFoldItem{{"banana"}, {"Apple"}, {"cherry"}}

	sortFold(in, func(it sortFoldItem) string { return it.name })

	want := []string{"Apple", "banana", "cherry"}
	for i, it := range in {
		if it.name != want[i] {
			t.Errorf("sortFold() = %v, want %v", in, want)
			break
		}
	}
}

func TestSortFold_StableTieBreak_CaseOnlyDuplicatesKeepEncounterOrder(t *testing.T) {
	// "Beta" and "beta" are equal under case-insensitive fold, so their
	// original relative order (Beta before beta) must survive the sort.
	in := []sortFoldItem{{"Beta"}, {"beta"}, {"Alpha"}}

	sortFold(in, func(it sortFoldItem) string { return it.name })

	want := []string{"Alpha", "Beta", "beta"}
	for i, it := range in {
		if it.name != want[i] {
			t.Errorf("sortFold() stable tie-break = %v, want %v", in, want)
			break
		}
	}
}

func TestSortFoldStrings_MixedCaseAlphabeticalOrder(t *testing.T) {
	in := []string{"cherry", "Apple", "banana"}

	sortFoldStrings(in)

	want := []string{"Apple", "banana", "cherry"}
	for i, s := range in {
		if s != want[i] {
			t.Errorf("sortFoldStrings() = %v, want %v", in, want)
			break
		}
	}
}

func TestSortFoldStrings_StableTieBreak_CaseOnlyDuplicatesKeepEncounterOrder(t *testing.T) {
	// Case-only duplicates ("Beta"/"beta") must keep their original relative
	// order since they compare equal under the fold.
	in := []string{"Beta", "beta", "Alpha"}

	sortFoldStrings(in)

	want := []string{"Alpha", "Beta", "beta"}
	for i, s := range in {
		if s != want[i] {
			t.Errorf("sortFoldStrings() stable tie-break = %v, want %v", in, want)
			break
		}
	}
}
