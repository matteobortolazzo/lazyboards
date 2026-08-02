# Keymap and Config Layer Conventions

Rules for `internal/keymap`, `internal/config`, and related config-handling code.

See also [`keymaps.md`](keymaps.md) for the registry architecture and config
schema reference (layers, resolution/precedence, load-time validation, how to
add a command id) — this file is the append-only lessons log for the same
subsystem.

## Rules

- When emitting a deprecation notice for a legacy config block, gate it on the block's *presence* in the loaded config, not on whether migration actually inserted any entries. If every legacy-derived key collides with an existing `keymaps:` entry and nothing is inserted, the notice must still fire — the user's stale config block needs to be nudged for removal. Spec AC wording like "loading a legacy block emits a deprecation notice" is presence-based; don't conflate "did the translation succeed" with "was the block present."

- Column name matching in config (e.g., merging `keymaps.columns` entries, translating legacy `columns[].actions`) must use case-insensitive lookup to align with `internal/keymap`'s established convention (`mergeKeymaps`'s `globalColumnsByLower`, `lowercaseColumnNames`). Use `strings.ToLower()` when storing or looking up column names, and mirror collision-detection logic (rejecting raw-name pairs that lowercase to the same value). This prevents silent duplicate-column errors when legacy and keymaps: entries mix different case variants of the same column name.

- When translating legacy config entries into the registry-based dispatch system (e.g., `translateLegacyActions` inserting `scope: pr` entries into `keymaps.pr_list`), apply the *original dispatch code's input gates* to the translator, not just the insertion mechanism. The old `pr_list` dispatch only accepted uppercase single letters (`!msg.Alt && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z'`); a translator that inserts every matching legacy action without this case restriction violates behavior parity. Preserve all constraints from the old dispatch's key-matching logic: case restrictions, modifier checks, character-class filters, etc.

- When building a modal's hint-bar from the registry, only advertise bindings that the dispatcher can actually resolve. If a modal has no pending-sequence/which-key machinery to resolve multi-key sequences, exclude multi-key sequences from its hints—a hint for an unresolvable key-combo creates a hint/dispatch mismatch where users see an advertised key that silently no-ops if pressed.

- When testing a remap of a command in a mode where `Mode.ConsumesPrintableRunes() == true` (delete, close_confirm, label_confirm, and potentially others per #539/#491 migration), the test must remap to a **named key** (`"tab"`, `"enter"`, `"esc"`, etc.), never a **bare printable rune** (`"c"`, `"y"`, etc.). Config validation (keymap_semantic_validate.go's `validatePrintableRuneBindings`) forbids bare printable-rune bindings in such modes at load time — a mode's textinput handler swallows every literal rune before any lookup could see it. A test using `boardWithOverrideKeymap` (or other raw-keymap construction) bypasses this validation, creating an unreachable production state. A GREEN-phase implementer who then "fixes" code to satisfy such a test will silently weaken the defense-in-depth guard in `textBinding` (keymap_text.go) that protects even hypothetically-unvalidated keymaps.

- When a new hint-derivation helper layers on an existing filtering helper, ensure the new selection logic doesn't depend on information the upstream filter discards. For example, `glyphHintKey` (arrow-glyph preference) composes with `commandHintKeys` (which suppresses arrow aliases when non-alias keys exist), creating silent data-loss when a keymap has both keys bound to the same command. Test composition against all existing consumers' real keymap data, not just the new caller — the interaction only surfaces with specific binding combinations.

- When computing a label or display value derived from registry data (e.g., longest common `Desc` prefix for a command family's range collapse in help generation), guard against empty or falsy results — a missing check causes silent rendering failures where the UI displays a blank label instead of falling back to the safe default. For numeric-suffix family collapse, if `commonPrefixLabel` returns `""`, decline the collapse and render each member via the per-command fallback instead of a range row with a blank label. Write a test with synthetic catalogue entries that share no common prefix to pin this edge case.
