package config

import "testing"

func TestColumnsByNameLower_MatchesCaseInsensitively(t *testing.T) {
	columns := []ColumnConfig{
		{Name: "Refined"},
		{Name: "New"},
	}

	byName := columnsByNameLower(columns)

	got, ok := byName["refined"]
	if !ok {
		t.Fatalf("columnsByNameLower() missing entry for lowercased %q", "refined")
	}
	if got.Name != "Refined" {
		t.Errorf("columnsByNameLower()[%q].Name = %q, want %q", "refined", got.Name, "Refined")
	}
}

func TestColumnsByNameLower_EmptyInput_ReturnsEmptyMap(t *testing.T) {
	byName := columnsByNameLower(nil)

	if len(byName) != 0 {
		t.Errorf("columnsByNameLower(nil) returned %d entries, want 0", len(byName))
	}
}

func TestDefaultScope_EmptyReturnsCard(t *testing.T) {
	if got := DefaultScope(""); got != "card" {
		t.Errorf("DefaultScope(\"\") = %q, want %q", got, "card")
	}
}

func TestDefaultScope_NonEmptyPassesThrough(t *testing.T) {
	for _, scope := range []string{"board", "pr", "card"} {
		if got := DefaultScope(scope); got != scope {
			t.Errorf("DefaultScope(%q) = %q, want %q", scope, got, scope)
		}
	}
}
