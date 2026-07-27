package config

import (
	"strings"
	"testing"
)

func TestSortNewestFirstValue_UnsetDefaultsToOldestFirst(t *testing.T) {
	cfg := mustLoadConfig(t, "", "")

	if cfg.SortNewestFirstValue() {
		t.Error("SortNewestFirstValue() = true with sort_order unset, want false (oldest-created-first is the default order, #503)")
	}
}

func TestSortNewestFirstValue_Newest(t *testing.T) {
	cfg := mustLoadConfig(t, "sort_order: newest\n", "")

	if !cfg.SortNewestFirstValue() {
		t.Error("SortNewestFirstValue() = false with sort_order: newest, want true")
	}
}

func TestSortNewestFirstValue_Oldest(t *testing.T) {
	cfg := mustLoadConfig(t, "sort_order: oldest\n", "")

	if cfg.SortNewestFirstValue() {
		t.Error("SortNewestFirstValue() = true with sort_order: oldest, want false")
	}
}

// Local config overrides global for scalar fields, like every other scalar.
func TestSortOrder_LocalOverridesGlobal(t *testing.T) {
	cfg := mustLoadConfig(t, "sort_order: newest\n", "sort_order: oldest\n")

	if cfg.SortNewestFirstValue() {
		t.Error("SortNewestFirstValue() = true, want false (local sort_order must override global)")
	}
}

func TestLoad_InvalidSortOrder_ReturnsError(t *testing.T) {
	_, err := loadConfigFromStrings(t, "sort_order: sideways\n", "")

	if err == nil {
		t.Fatal("Load() returned no error for sort_order: sideways, want a validation error")
	}
	if !strings.Contains(err.Error(), "sort_order") {
		t.Errorf("error = %q, want it to name the sort_order field", err.Error())
	}
	if !strings.Contains(err.Error(), "oldest") || !strings.Contains(err.Error(), "newest") {
		t.Errorf("error = %q, want it to list the accepted values", err.Error())
	}
}

// An empty value (`sort_order:` with nothing after it) is a typo, not a
// request for the default — reject it rather than silently falling back.
func TestLoad_EmptySortOrder_ReturnsError(t *testing.T) {
	_, err := loadConfigFromStrings(t, "sort_order: \"\"\n", "")

	if err == nil {
		t.Fatal("Load() returned no error for an empty sort_order, want a validation error")
	}
}
