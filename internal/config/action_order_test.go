package config

import "testing"

func TestLoad_ActionsGetSequentialOrderFromYAMLPosition(t *testing.T) {
	localYAML := `actions:
  Z:
    name: Zebra
    type: shell
    command: "echo z"
  A:
    name: Apple
    type: shell
    command: "echo a"
  M:
    name: Mango
    type: shell
    command: "echo m"
`
	result := mustLoadConfig(t, "", localYAML)

	if len(result.Actions) != 3 {
		t.Fatalf("Actions count = %d, want 3", len(result.Actions))
	}
	if result.Actions["Z"].Order >= result.Actions["A"].Order {
		t.Errorf("Actions[Z].Order = %d, want < Actions[A].Order = %d", result.Actions["Z"].Order, result.Actions["A"].Order)
	}
	if result.Actions["A"].Order >= result.Actions["M"].Order {
		t.Errorf("Actions[A].Order = %d, want < Actions[M].Order = %d", result.Actions["A"].Order, result.Actions["M"].Order)
	}
}

func TestLoad_GlobalOnlyActions_OrderReflectsGlobalFilePosition(t *testing.T) {
	globalYAML := `provider: github
actions:
  Z:
    name: Zebra
    type: shell
    command: "echo z"
  A:
    name: Apple
    type: shell
    command: "echo a"
`
	result := mustLoadConfig(t, globalYAML, "")

	if result.Actions["Z"].Order >= result.Actions["A"].Order {
		t.Errorf("Actions[Z].Order = %d, want < Actions[A].Order = %d", result.Actions["Z"].Order, result.Actions["A"].Order)
	}
}

func TestLoad_GlobalAndLocalActionsMerge_LocalKeysOrderedBeforeGlobalOnlyKeys(t *testing.T) {
	globalYAML := `provider: github
actions:
  Y:
    name: Global only 1
    type: shell
    command: "echo y"
  Z:
    name: Global only 2
    type: shell
    command: "echo z"
`
	localYAML := `actions:
  B:
    name: Local 1
    type: shell
    command: "echo b"
  A:
    name: Local 2
    type: shell
    command: "echo a"
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	if len(result.Actions) != 4 {
		t.Fatalf("Actions count = %d, want 4", len(result.Actions))
	}

	localKeys := []string{"B", "A"}
	globalOnlyKeys := []string{"Y", "Z"}

	for _, lk := range localKeys {
		for _, gk := range globalOnlyKeys {
			if result.Actions[lk].Order >= result.Actions[gk].Order {
				t.Errorf("Actions[%s].Order = %d, want < Actions[%s].Order = %d (local keys must precede global-only keys)",
					lk, result.Actions[lk].Order, gk, result.Actions[gk].Order)
			}
		}
	}

	// Relative order within each group is preserved.
	if result.Actions["B"].Order >= result.Actions["A"].Order {
		t.Errorf("Actions[B].Order = %d, want < Actions[A].Order = %d", result.Actions["B"].Order, result.Actions["A"].Order)
	}
	if result.Actions["Y"].Order >= result.Actions["Z"].Order {
		t.Errorf("Actions[Y].Order = %d, want < Actions[Z].Order = %d", result.Actions["Y"].Order, result.Actions["Z"].Order)
	}
}

func TestLoad_LocalOverridesGlobalKey_OrderReflectsLocalPosition(t *testing.T) {
	globalYAML := `provider: github
actions:
  A:
    name: Global open
    type: url
    url: "https://global.example.com"
  B:
    name: Global other
    type: url
    url: "https://global-other.example.com"
`
	localYAML := `actions:
  B:
    name: Local other
    type: url
    url: "https://local-other.example.com"
  A:
    name: Local open
    type: url
    url: "https://local.example.com"
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	if result.Actions["A"].Name != "Local open" {
		t.Fatalf("Actions[A].Name = %q, want %q (local should win value)", result.Actions["A"].Name, "Local open")
	}
	// Local file order: B (position 1), A (position 2). So B's Order < A's Order.
	if result.Actions["B"].Order >= result.Actions["A"].Order {
		t.Errorf("Actions[B].Order = %d, want < Actions[A].Order = %d (order should come from local position)",
			result.Actions["B"].Order, result.Actions["A"].Order)
	}
}

func TestLoad_ColumnActionsOrder_MatchesYAMLPosition(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Z:
        name: Zebra
        type: shell
        command: "echo z"
      A:
        name: Apple
        type: shell
        command: "echo a"
`
	result := mustLoadConfig(t, "", localYAML)

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}
	col := result.Columns[0]
	if col.Actions["Z"].Order >= col.Actions["A"].Order {
		t.Errorf("col.Actions[Z].Order = %d, want < col.Actions[A].Order = %d", col.Actions["Z"].Order, col.Actions["A"].Order)
	}
}

func TestLoad_ColumnActionsMerge_GlobalOnlyKeysOrderedAfterLocal(t *testing.T) {
	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      D:
        name: Delete branch
        type: shell
        command: "git branch -d {title}"
`
	localYAML := `columns:
  - name: Implementing
    actions:
      B:
        name: Local branch
        type: shell
        command: "git checkout -b {title}"
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}
	col := result.Columns[0]
	if len(col.Actions) != 2 {
		t.Fatalf("Implementing actions count = %d, want 2", len(col.Actions))
	}
	if col.Actions["B"].Order >= col.Actions["D"].Order {
		t.Errorf("col.Actions[B].Order = %d, want < col.Actions[D].Order = %d (local key must precede global-only key)",
			col.Actions["B"].Order, col.Actions["D"].Order)
	}
}

func TestLoad_ColumnActionsMerge_NilActionsInheritsGlobalOrder(t *testing.T) {
	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      B:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
      D:
        name: Delete branch
        type: shell
        command: "git branch -d {title}"
`
	// Local column "Implementing" omits the actions field entirely (nil).
	localYAML := `columns:
  - name: Implementing
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}
	col := result.Columns[0]
	if len(col.Actions) != 2 {
		t.Fatalf("Implementing actions count = %d, want 2", len(col.Actions))
	}
	if col.Actions["B"].Order >= col.Actions["D"].Order {
		t.Errorf("col.Actions[B].Order = %d, want < col.Actions[D].Order = %d (inherited order should pass through unchanged)",
			col.Actions["B"].Order, col.Actions["D"].Order)
	}
}

func TestValidateActions_ScopeDefaulting_PreservesOrder(t *testing.T) {
	actions := map[string]Action{
		"A": {Name: "First", Type: "shell", Command: "echo a", Order: 5},
		"B": {Name: "Second", Type: "shell", Command: "echo b", Order: 10},
	}

	if err := validateActions(actions); err != nil {
		t.Fatalf("validateActions() returned unexpected error: %v", err)
	}

	if actions["A"].Order != 5 {
		t.Errorf("Actions[A].Order = %d, want 5 (scope-defaulting round-trip must preserve Order)", actions["A"].Order)
	}
	if actions["A"].Scope != "board" {
		t.Errorf("Actions[A].Scope = %q, want %q (scope inferred from template with no ticket-specific placeholders)", actions["A"].Scope, "board")
	}
	if actions["B"].Order != 10 {
		t.Errorf("Actions[B].Order = %d, want 10", actions["B"].Order)
	}
}

func TestAssignActionOrder_NoTopLevelActionsKey_IsNoOp(t *testing.T) {
	globalYAML := `provider: github
actions:
  B:
    name: Open
    type: url
    url: "https://example.com"
`
	// Local file declares no actions: key at all, so cfg.Actions after the
	// local unmarshal is still the same map object populated from global.
	localYAML := `provider: github
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	// Should not panic and should retain the global-assigned order.
	if result.Actions["B"].Order != 1 {
		t.Errorf("Actions[B].Order = %d, want 1 (from global position, unaffected by absent local actions: key)", result.Actions["B"].Order)
	}
}
