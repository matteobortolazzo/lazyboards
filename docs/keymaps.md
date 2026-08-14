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
   `mergeKeymaps` -> `validateSortOrder`/`validateColumns` ->
   `validateKeymapActions` -> `validateCommandIDs` -> `validateModeCapabilities` ->
   `validateSequenceCapability` -> `validatePrintableRuneBindings` ->
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

<!-- keymap-schema-example:start -->
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
    q: app.quit
  columns:
    Refined:
      I:                 # scoped to the "Refined" column, normal/detail only
        name: Implement
        type: shell
        command: "cenci run implement {number} --model sonnet -- {comment}"
```
<!-- keymap-schema-example:end -->

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
  sequence. This resolution-layer rule has a dispatch-layer twin (#589):
  package main's `universalDispatch` (`keymap_dispatch.go`) achieves the
  same "quit works everywhere" property for the user-configurable
  `app.quit` command id, invoked at each of the 19 binding-resolution
  consumer sites (after that mode's own precedence guards) — see the
  Dispatch seams entry above.
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
`altFallbackEligible`) retries a failed lookup with the `alt+` prefix stripped
from the query's **last key only**, so a held-Alt keypress can still resolve to
the inline action it would resolve to unmodified (the comment-first flow) — but
an explicit `alt+key` binding always wins first, and a stripped-fallback result
can only ever be an inline action, never a built-in command.

Two consequences worth stating, since both are load-bearing for the
comment-first flow's user-visible behavior:

- **The retry is last-key-only, but the Alt *flag* is sticky.** `dispatchKey`
  stores `pendingSeqAlt` when a prefix goes pending and `handlePendingSeqKey`
  ORs it with each subsequent `msg.Alt`, so holding Alt on any key of a
  sequence reaches `dispatchActionWithAlt` — provided that keystroke resolved
  at all.
- **On a non-final key it often doesn't resolve.** `altFallbackEligible` adopts
  a stripped `OutcomePending` only when *every* candidate under the prefix is an
  inline action, so a prefix mixing a built-in with an action (`"u p"` →
  `card.open_pr` beside `"u c"` → an action) makes `alt+u` a no-op while
  `u` then `alt+c` works. This is the engine-level half of the guarantee that a
  stripped fallback can never fire a built-in.

## Mode capability matrix

Every resolvable `Mode` (`keymap.Modes()`) restricts what a `keymaps.<mode>`
binding can actually do, per four independent `Mode` predicates/behaviors:
whether the mode's dispatch seam accumulates a pending multi-key sequence
(`DispatchesKeySequences`, `mode.go`), whether it can resolve an inline
`BindingAction` at all (`DispatchesInlineActions`, `mode.go`), whether its
handler swallows every bare printable-rune keypress as literal text input
before any lookup ever runs (`ConsumesPrintableRunes`, `mode.go`), and
whether a `keymaps.columns.<name>` table can overlay the mode's own table at
all (`effectiveTable`, `keymap.go:227-236`). The "Bare printable-rune key"
column below is the INVERSE of the `ConsumesPrintableRunes` predicate:
`ConsumesPrintableRunes() == true` means such a key is *rejected* (the
textinput swallows it first), not allowed. Every mode also dispatches
`app.quit` (`CommandQuit`) regardless of its own table, since it's the one
universal command (`IsUniversalCommand`/`universalCommands`,
`capability.go`) -- hence "Command ids" reads "yes" throughout. The "Column
overlay" column is "yes" only for `normal`/`detail`: `effectiveTable`
precomputes a column overlay for those two modes only, so a
`keymaps.columns.<name>` entry can never reach any other mode's dispatch
seam, no matter what that mode's table itself contains.
`columns.<name>` is a config namespace, not itself a resolvable `Mode`
(`Mode.Resolvable()` is `false` for `ModeColumns`), so it gets its own
one-row table below instead of a row in the main one.

Each capability cell's text must start with one of `yes`/`no`/`rejected`/
`allowed`/`n/a`; trailing prose or a footnote is fine, but the drift test
(`internal/config/docs_capability_drift_test.go`) parses only the leading
token.

<!-- keymap-capability-matrix:start -->
| Mode | Key sequences | Inline actions | Bare printable-rune key | Command ids | Column overlay |
|------|----------------|------------------|----------------------------|--------------|-----------------|
| `normal` | yes | yes | allowed | yes | yes |
| `detail` | yes | yes | allowed | yes | yes |
| `create` | no | no | rejected | yes | no |
| `error` | no | no | allowed | yes | no |
| `config` | no | no | rejected | yes | no |
| `pr_picker` | no | no | allowed | yes | no |
| `search` | no | no | rejected | yes | no |
| `help` | no | no | allowed | yes | no |
| `label_confirm` | no | no | allowed | yes | no |
| `close_confirm` | no | no | allowed | yes | no |
| `comment` | no | no | rejected | yes | no |
| `delete` | no | no | rejected | yes | no |
| `filter` | no | no | allowed | yes | no |
| `assign` | no | no | allowed | yes | no |
| `git_panel` | no | yes | allowed | yes | no |
| `dispatch` | no | no | allowed | yes | no |
| `pr_list` | no | yes (scope: pr only, never inferred) | allowed | yes | no |
| `milestone_list` | no | no | allowed | yes | no |
| `agent_list` | no | no | allowed | yes | no |
<!-- keymap-capability-matrix:end -->

`columns.<name>` overlays `normal`/`detail` (`keymap.go`'s `Resolve`), so it
inherits both of their capabilities in full:

<!-- keymap-capability-matrix-columns:start -->
| Mode | Key sequences | Inline actions | Bare printable-rune key | Command ids | Column overlay |
|------|----------------|------------------|----------------------------|--------------|-----------------|
| `columns` | yes | yes | n/a (overlays normal/detail, neither of which consumes printable runes, but columns is not itself a resolvable dispatch seam) | yes | n/a (this row already represents the overlay itself, not a mode an overlay can be applied to) |
<!-- keymap-capability-matrix-columns:end -->

A binding one of these seams can never reach is rejected at load time, not
silently dropped at runtime: `validateModeCapabilities`
(`keymap_semantic_validate.go`, #577) rejects a `BindingCommand` whose id
isn't in `keymap.DispatchableCommands(mode)`, and a `BindingAction` in a
mode where `DispatchesInlineActions()` is false (plus `keymaps.pr_list`'s
additional scope requirement, below); `validateSequenceCapability`
(#578) rejects a multi-key binding in a mode where
`DispatchesKeySequences()` is false; `validatePrintableRuneBindings`
rejects a bare printable-rune key in a mode where `ConsumesPrintableRunes()`
is true. See [Load-time validation](#load-time-validation) checks 4, 5, and
9 below for the full rules.

`keymaps.pr_list`'s `scope: pr` requirement is never inferred, always
stated: a scope-omitted inline action there can never satisfy it, since
`inferScope` (`config.go`) only ever infers `"card"` or `"board"` from a
template's placeholders, never `"pr"`. Omitting `scope:` is a load-time
config error, not a silent fallback to `"pr"`:

<!-- keymap-schema-pr-list-scope-omitted:start -->
```yaml
keymaps:
  pr_list:
    z:
      name: Missing scope
      type: shell
      command: "echo hi"
```
<!-- keymap-schema-pr-list-scope-omitted:end -->

The snippet above fails `config.Load` with a `pr_list only dispatches
inline actions with scope "pr"` error; fix it by adding `scope: pr`
explicitly.

## Load-time validation

Eleven checks run before a resolved `*keymap.Keymap` is ever handed to the
runtime, in `Load`'s order:

1. **Unknown mode name** — `Keymaps.UnmarshalYAML` (`keymaps.go:143`), via
   `keymap.ParseMode`.
2. **Duplicate key within one table** — `parseKeymapTable` (`keymaps.go:101`),
   since walking `node.Content` by hand bypasses yaml.v3's own map-decode
   duplicate detection.
3. **Unknown command id** — `validateCommandIDs` (`keymap_semantic_validate.go:54`),
   checked against `keymap.FindCommand`.
4. **Mode capability** — `validateModeCapabilities` (`keymap_semantic_validate.go`,
   #577): a `BindingCommand` entry's command id must be in
   `keymap.DispatchableCommands(mode)` (checked via
   `keymap.CommandDispatchable`) — the mode's dispatch seam has to be able to
   actually reach it, not merely recognize it as a catalogued id (check 3
   above). A `BindingAction` (inline action) entry requires
   `mode.DispatchesInlineActions()`, and, for `keymaps.pr_list` specifically,
   additionally requires the action's already-inferred effective scope
   (written back by check 8's `validateKeymapActions`-driven inference,
   which runs before this check) to be exactly `"pr"`. An explicit unbind is
   always skipped. `keymaps.columns.<name>` tables are checked against
   `keymap.ModeColumns`'s own entry in the capability index.
5. **Sequence capability** — `validateSequenceCapability`
   (`keymap_semantic_validate.go`, #578): a binding's key must parse (via
   `keymap.ParseSequence`) to a single element unless the mode is
   `Mode.DispatchesKeySequences()` (`normal`, `detail`, and
   `keymaps.columns.<name>`) — every other mode's dispatch seam
   (`panelBinding`, `textBinding`, or a modal's single-key `Lookup`) can only
   ever resolve a single key by exact match, so a bound sequence there could
   never dispatch. Rejected regardless of binding kind, including an
   explicit unbind (unlike check 4, which skips unbinds) — an unbind that
   could never do anything is still an error, mirroring check 6's (below)
   own `ctrl+c` reasoning. The error names the config path, the raw key, its
   canonical `Sequence.String()` form, and the offending mode.
6. **`ctrl+c` anywhere** — `validateNoCtrlC` (`keymap_validate.go:99`): a
   `ctrl+c` token anywhere in a space-separated sequence is rejected
   regardless of binding kind, including an explicit unbind — `Lookup`'s
   short-circuit doesn't consult the table, so an unbind could never do
   anything anyway.
7. **Prefix conflict** — `validateModePrefixes` (`keymap_validate.go:135`):
   over the fully *resolved* namespace (defaults + user together, via
   `ResolveKeymap`), a bound key that is a strict, whitespace-boundary prefix
   of another bound key in the same mode/column is rejected, mirroring
   `Lookup`'s own pending-match boundary check.
8. **`alt+` shadowing a `{comment}` overload** — `validateModeAltCommentShadow`
   (`keymap_validate.go:155`): an `alt+` modifier on *any* token of a bound
   sequence is rejected when that sequence's fully alt-free base (via
   `altFreeBaseSequence`, `keymap_semantic_validate.go`) is itself bound to a
   `{comment}`-bearing action — since holding Alt anywhere in the pending
   sequence already means "enter comment mode first" for that base.
9. **Bare printable rune in a text-input mode** — `validatePrintableRuneBindings`
   (`keymap_semantic_validate.go:92`): a bare printable-rune key bound in any
   mode where `Mode.ConsumesPrintableRunes()` is true is rejected — the
   mode's textinput swallows the keystroke before any lookup could see it.
   An `alt+<rune>` form is exempt.
10. **Card/PR scope conflict** — `validateScopeConflicts` (`config.go`):
    action keys are grouped by their canonical `keymap.ParseSequence(...).String()`
    form (not raw YAML spelling), so whitespace/spelling variants of the same
    physical sequence share one bucket; the same canonical sequence cannot be
    `"card"`-scope in one inline action and `"pr"`-scope in another, across
    `keymaps.normal`, `keymaps.detail`, and every `keymaps.columns.<name>`
    table. An action key that fails to parse is a contextual load error
    (naming the owning table and the raw key), not a silently skipped entry.
11. **Run-mode fields on a non-shell action** — `validateActionValue`
    (`config.go`, #623/#624): `terminal:` (hand the command lazyboards' own
    terminal, via `tea.ExecProcess` instead of the buffered `runShellCmd`),
    `window:`/`cwd:`/`focus:` (run it in a named tmux window, in a given
    directory, optionally switching to it) only mean anything for
    `type: shell`. Declaring any of them on a `type: url` action is a
    load-time error naming the offending key, rather than a silently ignored
    field. The same check rejects the two contradictory combinations —
    `window:` together with `terminal:` (one detaches the command, the other
    hands it this terminal) and `focus:` without a `window:` — and relaxes
    the "command is required" rule for a `window:` action, where opening a
    window on a directory with no command is a complete action. All four are
    deliberately modifiers on the existing `shell` type and not types of
    their own, so every `Type == "shell"` gate — above all
    `trust_strip.go`'s local-shell-sink stripping (see
    `docs/trust-model.md`) — keeps covering them unchanged.

    `window:` and `cwd:` carry template variables, so they are part of
    `Action.Template()` (`config.go`, mirrored on `keymap.Action`) — the
    single accessor every "does this action reference variable X?" site
    reads: scope inference and the board/card variable restrictions here,
    the `{comment}` Alt-overload scan and the on-demand `{pr_worktree}`
    lookup in `action_dispatch.go`, and check 8's alt-comment shadow scan.
    A template-bearing field added to `Action` but missed by one of those
    sites would let a board-scope action smuggle `{number}` in through the
    missed field, or leave `{pr_worktree}` unresolved at dispatch.

Checks 1-2 run during YAML unmarshal itself; 3, 4, 5, 9, 10, and 11 run over
`cfg.Keymaps` directly; 6, 7, and 8 run inside `validateKeymap` (`keymap_validate.go:45`),
which calls `ResolveKeymap` itself so the prefix/alt-shadow checks see
built-ins and user config together, exactly what a real `Lookup` would see.

**Universal-command exception (#589):** `app.quit` is valid and dispatchable
in all 19 resolvable modes plus every `keymaps.columns.<name>` overlay, not
just the four modes (`normal`, `detail`, `help`, `error`) that bind it by
default. This exception is not derived from `keymap.Defaults()` — a
per-mode allowed-set built from `Defaults()` would only see the four modes
that happen to bind `app.quit` today and would wrongly reject it everywhere
else. It is instead the `IsUniversalCommand` predicate over the
`universalCommands` set in `internal/keymap/capability.go`, which check 4's
`validateModeCapabilities` consults directly (via the `commandModeIndex` it
feeds `keymap.CommandDispatchable`/`keymap.DispatchableCommands`) for
`app.quit`'s allowed-mode set, rather than deriving it from `Defaults()` or
a hand-written per-mode list.

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
   binding on the same key. This step applies to a **mode-scoped** id only.
   A **universal** id — currently just `app.quit`/`CommandQuit`, tracked in
   `internal/keymap/capability.go`'s `universalCommands` set — skips it
   entirely: it gets no per-mode `run*Command` case at all, and instead
   dispatches once from the shared `universalDispatch` seam
   (`keymap_dispatch.go`), invoked at every binding-resolution consumer site
   (after that mode's own precedence guards) so one source of truth covers
   all 19 modes uniformly.
4. Add (or extend) a `hintSpec` so the mode's `*Hints()` builder surfaces it
   in the status bar / help modal — the hint and the dispatch case must
   agree on the id, or the hint will advertise a key that silently no-ops.
5. Add the command's row to the README's per-mode Keybindings table.
6. Add tests: default-parity dispatch, remap/unbind override, and the
   hint<->dispatch invariant (see `keymap_dispatch_test.go`/
   `keymap_panels_test.go`/`keymap_modals_test.go`/`keymap_text_test.go` for
   the established shape per seam).

**Single-key-only caveat (#589):** universal dispatch is single-key only
outside `normal`/`detail`. Those two modes' `dispatchBinding`/
`handlePendingSeqKey` seam is the only one with pending-sequence
("which-key") machinery; the other 17 seams (`panelBinding`, `textBinding`,
`lookupModalBinding`) resolve a single key by exact match only, treating a
multi-key match as `OutcomePending` and no-oping on it. So a multi-key
binding to a universal command (e.g. `keymaps.filter."g q": app.quit`) is a
documented no-op in those 17 modes, not a bug.

## See also

- [`keymap-conventions.md`](keymap-conventions.md) — lessons and gotchas for
  this subsystem (case-insensitive column matching, hint/dispatch composition
  pitfalls).
- README [`### Keymaps`](../README.md#keymaps) — the user-facing schema
  reference, and [`## Keybindings`](../README.md#keybindings) for the actual
  per-mode command-id tables.
