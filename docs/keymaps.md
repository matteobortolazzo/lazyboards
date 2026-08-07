# Keymaps: Registry Architecture and Config Schema

Architecture/schema reference for the `keymaps:` registry: how a key press
becomes a dispatched command or action, and how `internal/config` parses,
validates, and merges user config into it. This documents **mechanism**, not
the shipped default key values — the README's [Keybindings](../README.md#keybindings)
tables (including its [Keybinding migration](../README.md#keybinding-migration)
subsection) are the single home for actual key values. For gotchas
and process rules discovered while working in this subsystem, see
[`keymap-conventions.md`](keymap-conventions.md) — that's the lessons doc;
this one is the stable architecture reference and the two cross-link rather
than merge, so a stable reference and an append-only lessons log don't get
mixed.

#502 remapped nine default bindings to neovim-style mnemonics (the old key,
where still bound, keeps its pre-#502 command; the ones below just moved):

| Command | Old key | New key |
|---|---|---|
| `card.delete` | `t` | `d` |
| `view.dispatch` | `d` | `D` |
| `nav.reference` | `m` | `g r` |
| `view.milestone_list` | `i` | `m` |
| `nav.agent` | `s` | `g a` |
| `board.sort_order` | `u` | `s` |
| `view.pr_list` | `v` | `P` |
| `view.agent_list` | `w` | `A` |
| `view.git_panel` | `g` | `G` |

Every other default key kept its pre-#502 binding — the remap only touched
these nine command ids; nothing else in the default tables moved.

## Layers

Three layers, each with a narrow responsibility:

1. **Engine** (`internal/keymap`) — pure, config-agnostic key resolution.
   `Table`/`Tables`/`Keymap` (`keymap.go`) hold canonical `Mode -> Table` and
   `column -> Table` maps; `Resolve` merges a defaults `Tables` layer with a
   user `Tables` layer into an immutable `*Keymap`; `Lookup`/`Entries`
   (`lookup.go`) are the only ways a caller queries it. `Key`/`Sequence`
   (`key.go`) own key notation and canonicalization. `Mode` (`mode.go`) lists
   every resolvable key surface. `Binding`/`BindingKind` (`binding.go`) are
   the resolved right-hand side. The command catalogue and default bindings
   live in `command_*.go`/`defaults_*.go`, one pair of files per surface
   group (`command_board.go`/`defaults_board.go` for normal/detail,
   `command_modal.go`/`defaults_modal.go` for the list-style modals,
   `command_panel.go`/`defaults_panel.go` for the git panel/dispatch
   modal/help, `command_system.go`/`defaults_system.go`, and
   `command_text.go`/`defaults_text.go` for the seven text/confirm modes) —
   `catalog.go`'s `init()` merges every group's `Command`/default-`Table` var
   into the package-level `catalog`/`defaultModeTables`, so `internal/keymap`
   must never import `internal/config` (`keymap.Action` is kept
   field-/tag-compatible with `config.Action` by hand instead).

2. **Parsing, validation, and merge** (`internal/config`) — turns YAML into
   the engine's input. `Keymaps`/`KeymapTable`/`KeymapBinding` (`keymaps.go`)
   are the parsed shape of one `keymaps:` block, with custom
   `UnmarshalYAML`/`MarshalYAML` for deterministic round-tripping;
   `mergeKeymaps` (`keymaps.go:299`) combines a global and local `*Keymaps`
   per key. `Load` (`config.go:210`) is the single, linear pipeline: unmarshal
   global -> snapshot and nil out `cfg.Keymaps` -> unmarshal local ->
   `mergeKeymaps` -> `validateSortOrder`/`validateColumns`/`validateActions`
   -> `translateLegacyActions` -> `validateKeymapActions` ->
   `validateCommandIDs` -> `validatePrintableRuneBindings` ->
   `validateScopeConflicts` -> `validateKeymap`. `ResolveKeymap`
   (`keymap_validate.go:17`) is the *only* path that combines
   `keymap.Defaults()` with `cfg.Keymaps.Tables()` — every validator and the
   runtime share it, so nothing can validate against one resolution and run
   against another.

3. **Dispatch seams** (package `main`) — where a resolved `Binding` actually
   runs. `keymap_dispatch.go` is the seam for normal mode and the detail
   panel (`dispatchKey`, `handlePendingSeqKey`, `withKeymap`,
   `registryHints`). `keymap_panels.go` is the seam for single-key
   command-panel modes (`panelBinding`, shared by the git panel, plus the
   dispatch-modal- and help-modal-specific `runDispatchCommand`/
   `runHelpCommand`). `keymap_modals.go` is the seam for the six list-style
   modals (`lookupModalBinding`, shared by `filter`/`assign`/`pr_picker`/
   `pr_list`/`milestone_list`/`agent_list`). `keymap_text.go` is the seam for
   the seven text/confirm modes (`textBinding`, shared by `close_confirm`/
   `label_confirm`/`delete`/`create`/`config`/`search`/`comment`), which
   additionally guards `Mode.ConsumesPrintableRunes()` so literal keystrokes
   reach the mode's `textinput` instead of being intercepted as commands.
   None of these seams contains a hardcoded `switch msg.String()` — every one
   resolves through `*keymap.Keymap.Lookup` against `b.keys`.

## Config schema

One `keymaps:` block, one table per bindable mode plus a `columns` overlay
namespace:

```yaml
keymaps:
  normal:
    q: app.quit        # BindingCommand: a catalogued command id string
    O:                 # BindingAction: an inline action mapping
      name: Open issue
      type: url
      url: "https://github.com/{repo_owner}/{repo_name}/issues/{number}"
    n: ~                # BindingUnbound: `~`, `null`, or an empty value
  detail:
    ...
  columns:
    Refined:
      I: implement.dispatch   # scoped to the "Refined" column, normal/detail only
```

**Security note:** `.lazyboards.yml` is repo-local and is typically checked
into the repository, so any key — including built-in lowercase keys like
`j`/`k`/`q` — can be rebound to an inline action by whoever controls that
repo, and a user binding always wins over a default. A rebind onto a
catalogued built-in command id, or an inline `type: url` action, applies
immediately regardless of trust. An inline `type: shell` action is inert
until you explicitly run `lazyboards trust` for that exact file content —
see [`docs/trust-model.md`](trust-model.md) for the full mechanism (what
counts as a shell sink, hash identity, store format) and the README's
[`### Trust Model`](../README.md#trust-model) for the user-facing summary.

`KeymapBinding` (`keymaps.go:19-27`) is the parsed right-hand side, and its
`Kind` mirrors `keymap.BindingKind` (`binding.go:25-40`) one-for-one:
`BindingCommand` (Command field holds the catalogued id), `BindingAction`
(Action field holds an inline action, field-/tag-compatible with
`keymap.Action`), `BindingUnbound` (explicit `~`/`null`, deliberately
distinct from `BindingInvalid` so `Resolve`'s merge can tell an explicit
unbind from "never specified" — a zero-value map entry can never be mistaken
for a real unbind). `Keymaps.UnmarshalYAML` (`keymaps.go:129`) parses the
whole block; `parseKeymapTable` (`keymaps.go:89`) parses one mode/column
table, walking `node.Content` by hand (not decoding into a Go map) so a
repeated key is caught instead of silently last-write-winning, and checking
`node.Tag == "!!null"` ahead of `Decode` (yaml.v3 short-circuits null nodes
before ever calling a custom `UnmarshalYAML` — see `yaml-parsing.md`).

## Resolution & precedence

Every raw key or sequence is canonicalized via `ParseSequence`/
`Sequence.String()` (`key.go`) before it's ever compared: `Resolve`
(`keymap.go:75`) re-canonicalizes every key in every table it's given and
errors if two distinct raw keys in the same table normalize to the same
canonical form.

Precedence is layered, not just "user wins":

- **Per mode**: `defaults < user` — `Resolve` copies the defaults table and
  overwrites it per canonical key with the user table's entry (including an
  explicit `BindingUnbound`), leaving every default key the user table
  doesn't mention untouched.
- **Per column**: `default-mode < default-column < user-mode < user-column`
  (`keymap.go:117-127`) — `Resolve` precomputes this full chain once per
  column so `Lookup` never allocates. Column overlays are precomputed for
  `ModeNormal`/`ModeDetail` only (`effectiveTable`, `keymap.go:227-236`); no
  other mode can be overlaid per column. Column names are matched
  case-insensitively (`lowercaseColumnNames`, `keymap.go:191`); two raw names
  that lowercase to the same value are a hard error, never a silent
  last-write-win.

`Lookup` (`lookup.go:58`) resolves a `Sequence` against `(mode, column)`'s
effective table:

- If the sequence's **last** key is `ctrl+c`, `Lookup` short-circuits to
  `OutcomeMatch(CommandBinding(CommandQuit))` before consulting any table —
  unconditionally, regardless of table contents or earlier keys in the
  sequence.
- Next, if any individual key in the sequence contains a whitespace rune
  (checked per-key, never against the sequence's space-joined canonical
  string), `Lookup` returns `OutcomeNoMatch` without consulting the table.
  This guard sits after the `ctrl+c` short-circuit so a whitespace-bearing
  earlier key can never strand a user without a way to quit, and it is
  behavior-neutral for every real table binding — canonical keys reach the
  table through a whitespace-based split, so none can ever contain a
  whitespace rune. It only rejects sequences built directly from
  unvalidated runtime input.
- Otherwise: an exact canonical match with a resolved binding (not
  `BindingUnbound`/`BindingInvalid`) is `OutcomeMatch`. Failing that, every
  table entry whose canonical key extends the query by further
  whitespace-boundary keys (`query + " "` as a prefix, not a raw substring
  test) and is itself resolved becomes an `OutcomePending` candidate,
  sorted by canonical sequence for deterministic which-key rendering. With no
  such entry, the result is `OutcomeNoMatch`.

Dispatch (`dispatchKey`/`handlePendingSeqKey`, `keymap_dispatch.go`) applies
one more layer ahead of this: the **Alt-strip-and-retry fallback**
(`lookupWithAltFallback`, `keymap_dispatch.go:304`, and its eligibility gate
`altFallbackEligible`) retries a failed lookup with a leading `alt+` stripped
from the query, so a held-Alt keypress can still resolve to the inline
action it would resolve to unmodified (the comment-first flow) — but an
explicit `alt+key` binding always wins first, and a stripped-fallback result
can only ever be an inline action, never a built-in command.

## Load-time validation

Eight checks run before a resolved `*keymap.Keymap` is ever handed to the
runtime, in `Load`'s order:

1. **Unknown mode name** — `Keymaps.UnmarshalYAML` (`keymaps.go:143`), via
   `keymap.ParseMode`.
2. **Duplicate key within one table** — `parseKeymapTable` (`keymaps.go:101`),
   since walking `node.Content` by hand bypasses yaml.v3's own map-decode
   duplicate detection.
3. **Unknown command id** — `validateCommandIDs` (`keymap_semantic_validate.go:54`),
   checked against `keymap.FindCommand`.
4. **`ctrl+c` anywhere** — `validateNoCtrlC` (`keymap_validate.go:99`): a
   `ctrl+c` token anywhere in a space-separated sequence is rejected
   regardless of binding kind, including an explicit unbind — `Lookup`'s
   short-circuit doesn't consult the table, so an unbind could never do
   anything anyway.
5. **Prefix conflict** — `validateModePrefixes` (`keymap_validate.go:135`):
   over the fully *resolved* namespace (defaults + user together, via
   `ResolveKeymap`), a bound key that is a strict, whitespace-boundary prefix
   of another bound key in the same mode/column is rejected, mirroring
   `Lookup`'s own pending-match boundary check.
6. **`alt+` shadowing a `{comment}` overload** — `validateModeAltCommentShadow`
   (`keymap_validate.go:155`): an explicit `alt+<key>` binding that shadows
   the implicit Alt-overload of a `{comment}`-bearing action bound to the
   same base key is rejected, since holding Alt on that base key already
   means "enter comment mode first."
7. **Bare printable rune in a text-input mode** — `validatePrintableRuneBindings`
   (`keymap_semantic_validate.go:92`): a bare printable-rune key bound in any
   mode where `Mode.ConsumesPrintableRunes()` is true is rejected — the
   mode's textinput swallows the keystroke before any lookup could see it.
   An `alt+<rune>` form is exempt.
8. **Card/PR scope conflict** — `validateScopeConflicts` (`config.go:803`):
   the same canonical key sequence cannot be `"card"`-scope in one inline
   action and `"pr"`-scope in another, across `keymaps.normal`,
   `keymaps.detail`, and every `keymaps.columns.<name>` table.

Checks 1-2 run during YAML unmarshal itself; 3, 7, and 8 run over
`cfg.Keymaps` directly (after `translateLegacyActions` has folded legacy
entries in); 4, 5, and 6 run inside `validateKeymap` (`keymap_validate.go:45`),
which calls `ResolveKeymap` itself so the prefix/alt-shadow checks see
built-ins and user config together, exactly what a real `Lookup` would see.

## Adding a command id

1. Add a `const CommandID` and a `Command{ID, Desc}` catalogue entry to the
   right `command_<group>.go` (board/modal/panel/system/text) — `Desc` is
   the one human-readable string shared by both status-bar hints and the `?`
   help modal; there is no second, help-only wording to keep in sync.
2. Add its default binding to the matching `defaults_<group>.go` table.
3. Add a dispatch `case` for the id in that mode's `run*Command` (e.g.
   `runNormalCommand`, `runGitPanelCommand`, `runCreateCommand`). A new
   `normalDefaults` id needs no separate `case` in `runDetailCommand`:
   its `default:` branch delegates every command id it doesn't explicitly
   case to `runNormalCommand` (#588), so a new `ModeNormal` command becomes
   dispatchable from the detail panel automatically, without blurring it.
   This applies to `keymaps.columns.<name>` overrides too: `Resolve`
   overlays a column table onto both `ModeNormal` and `ModeDetail`
   (`keymap.go`), so a column-scoped binding now dispatches from the detail
   panel the same way a `keymaps.detail` binding does, and — because the
   column layer is applied last — it wins over any `keymaps.detail`
   binding on the same key.
4. Add (or extend) a `hintSpec` so the mode's `*Hints()` builder surfaces it
   in the status bar / help modal — the hint and the dispatch case must
   agree on the id, or the hint will advertise a key that silently no-ops.
5. Add the command's row to the README's per-mode Keybindings table.
6. Add tests: default-parity dispatch, remap/unbind override, and the
   hint<->dispatch invariant (see `keymap_dispatch_test.go`/
   `keymap_panels_test.go`/`keymap_modals_test.go`/`keymap_text_test.go` for
   the established shape per seam).

## Legacy translation

`translateLegacyActions` (`legacy_actions.go:73`) folds the deprecated
top-level `actions:` and per-column `columns[].actions:` blocks onto the
`keymaps:` namespace, additively and non-destructively: `cfg.Actions`/
`cfg.Columns[i].Actions` are left populated for any caller that still reads
them, and an existing `keymaps:`-declared entry for the same canonical
sequence is never overwritten by a legacy-derived one.

- Legacy action keys are rune-concatenated with no separator (`"Rf"`, matching
  how `handlePendingSeqKey` builds up a pending sequence one keystroke at a
  time); `legacySequence` (`legacy_actions.go:38`) splits them into the
  canonical space-separated form (`"R f"`) `ParseSequence`/`Sequence.String()`
  expect.
- Top-level `actions:` entries are mirrored into **both**
  `keymaps.normal` and `keymaps.detail` (`legacy_actions.go:83-85`) — the
  legacy dispatch let these actions fire from either surface, so translating
  into `normal` alone would silently kill them while the detail panel is
  focused.
- A legacy `scope: pr` action bound to a single uppercase letter (`A`-`Z`,
  no Alt) is additionally mirrored into `keymaps.pr_list`
  (`legacy_actions.go:87-107`), gated by `isUppercaseLetterKey` — this
  reproduces the old raw PR-list key scan's exact input filter; any other
  kind of key (lowercase, multi-key, digit, ...) stays the no-op it always
  was inside the PR list.
- The deprecation notice (`legacyDeprecationNotice`, `legacy_actions.go:14`)
  is **presence-based**: it's appended to `cfg.Deprecations` whenever a
  legacy block was present in the loaded config at all, even if every
  derived key was shadowed by an existing `keymaps:` entry and nothing was
  actually inserted. `main.go:349-354` prints each notice to stderr once, ahead
  of BubbleTea taking over the terminal.

## See also

- [`keymap-conventions.md`](keymap-conventions.md) — lessons and gotchas for
  this subsystem (presence-based deprecation, case-insensitive column
  matching, legacy-input-gate parity, hint/dispatch composition pitfalls).
- README [`### Keymaps`](../README.md#keymaps) — the user-facing schema
  reference, and [`## Keybindings`](../README.md#keybindings) for the actual
  per-mode command-id tables.
