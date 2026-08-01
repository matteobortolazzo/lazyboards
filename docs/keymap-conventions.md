# Keymap and Config Layer Conventions

Rules for `internal/keymap`, `internal/config`, and related config-handling code.

## Rules

- When emitting a deprecation notice for a legacy config block, gate it on the block's *presence* in the loaded config, not on whether migration actually inserted any entries. If every legacy-derived key collides with an existing `keymaps:` entry and nothing is inserted, the notice must still fire — the user's stale config block needs to be nudged for removal. Spec AC wording like "loading a legacy block emits a deprecation notice" is presence-based; don't conflate "did the translation succeed" with "was the block present."

- Column name matching in config (e.g., merging `keymaps.columns` entries, translating legacy `columns[].actions`) must use case-insensitive lookup to align with `internal/keymap`'s established convention (`mergeKeymaps`'s `globalColumnsByLower`, `lowercaseColumnNames`). Use `strings.ToLower()` when storing or looking up column names, and mirror collision-detection logic (rejecting raw-name pairs that lowercase to the same value). This prevents silent duplicate-column errors when legacy and keymaps: entries mix different case variants of the same column name.

- When translating legacy config entries into the registry-based dispatch system (e.g., `translateLegacyActions` inserting `scope: pr` entries into `keymaps.pr_list`), apply the *original dispatch code's input gates* to the translator, not just the insertion mechanism. The old `pr_list` dispatch only accepted uppercase single letters (`!msg.Alt && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z'`); a translator that inserts every matching legacy action without this case restriction violates behavior parity. Preserve all constraints from the old dispatch's key-matching logic: case restrictions, modifier checks, character-class filters, etc.

- When building a modal's hint-bar from the registry, only advertise bindings that the dispatcher can actually resolve. If a modal has no pending-sequence/which-key machinery to resolve multi-key sequences, exclude multi-key sequences from its hints—a hint for an unresolvable key-combo creates a hint/dispatch mismatch where users see an advertised key that silently no-ops if pressed.
