# Trust Model

`.lazyboards.yml` is repo-local and typically checked into the repository, so
it is attacker-controlled the moment you clone someone else's repo. The trust
store (epic #533) closes the resulting shell-execution gap: a content-hash
allowlist gates every local-origin construct that would otherwise let a
`.lazyboards.yml` you didn't write execute a shell command on your machine the
moment lazyboards loads it. An untrusted local file is not rejected — it still
loads, and its non-executing settings (columns, labels, keymap remaps onto
built-in commands, etc.) still apply — but its shell-executing sinks are
silently stripped before they ever reach the merge/dispatch pipeline. This is
fail-closed: a missing, unreadable, or malformed trust store trusts nothing,
never everything.

Global config (`~/.config/lazyboards/config.yml`) is never subject to any of
this — you wrote it yourself, so it's trusted by construction. Only
`.lazyboards.yml` (`config.DefaultLocalPath`) is gated.

## What counts as a sink

A "sink" is any local-origin construct that shells out. Two kinds exist,
tallied and stripped independently by `internal/config/trust_strip.go`'s
`stripLocalShellSinks`:

| Sink kind | Source | Stripped when untrusted |
|---|---|---|
| Keymap shell bindings | `keymaps.<mode>.<key>` / `keymaps.columns.<name>.<key>` entries whose value is an inline `type: shell` action | Compared by value against the matching global mode/column table entry (ignoring the derived `Order` field -- see "Stripping is decided by..." below). A value-equivalent binding is left alone and not counted. A genuinely differing or local-only binding is deleted from the table and counted once; if something was actually stripped and that empties the mode/column table entirely, the whole table entry is removed so the mode/column falls back to inheriting the global table. An explicitly-empty local table that stripped nothing (e.g. `keymaps: {normal: {}}`) is never deleted this way -- it stays explicit-and-empty, matching what a trusted load of the same bytes would resolve to |
| Cleanup fields | `cleanup:` / `columns[].cleanup` | Reset to the matching global value (or unset if none), never to an empty string |

`type: url` actions, and keymap bindings to a catalogued built-in command id,
are never candidates for stripping — only `type: shell` is an executing
construct. This is why `terminal: true` (#623) and `window:`/`cwd:`/`focus:`
(#624) are *modifiers* on `type: shell` rather than types of their own: all
of them still execute a shell command — a terminal action a more dangerous
one, since it also owns the terminal's stdin — so they must be stripped by
the same gate, and they are, with no change to `stripShellBindings`. Any future "run a command" construct must either
keep `Type == "shell"` or widen that gate in the same change; a new type
value that shells out and isn't added here is a silent trust bypass. That claim is sound only because `internal/action.OpenURL` is
itself a non-shell sink on every platform (see `docs/shell-and-url-safety.md`;
#576) — if it ever regressed to shelling out, an untrusted `type: url`
binding would reopen the exact command-injection gap this trust model exists
to close. Stripping is decided by comparing the local value against a
snapshot of the *global* document taken before the local file was merged in,
via `sameShellAction`/`sameShellBinding` (`internal/config/trust_strip.go`):
each side's derived `Order` field is zeroed out first, then the remainder is
compared by **whole-struct equality** (`a == b`) -- not a hand-picked list of
"execution-relevant" fields. A local binding that is `Order`-blind-equal to
its global counterpart is genuinely global (inherited, not locally declared)
and is left alone, whatever the local file's trust state; anything that
differs on any other field -- a real override or a local-only declaration --
is stripped unconditionally and counted once. `Order` alone is excluded --
document-position metadata used only for hint-bar/help ordering, never
consumed at execution time -- because a YAML alias (`keymaps: *anchor`)
leaves every aliased entry's `Order` at its zero value rather than stamping
it from document position; an `Order`-inclusive comparison would
misclassify an aliased-but-genuinely-global entry as locally differing
purely because of where it appears in the document. Comparing the *whole*
remaining struct, rather than naming individual fields, is deliberate and
fail-safe: `Terminal`, `Window`, `Cwd`, and `Focus` (#623/#624) all
participate in the comparison automatically, and so will any field `Action`
gains in the future, with no matching update needed here -- a hand-maintained
field list would otherwise silently stop covering a new field the moment
someone adds one, exactly the class of gap `AGENTS.md`'s "any new config
construct that executes a command must be reachable by the trust model's
gate" rule exists to prevent. This fails safe (over-strip, never
under-strip) and closes a YAML merge-key/alias bypass a raw "was this key
literally in the document" walk would have missed.

An explicit local `cleanup: ""` is left alone even when untrusted — it's a
disable directive, not a command, and can never reach a shell.

Each `Load()` call that strips anything appends **at most one** entry to the
returned `Config.Notices`, naming every stripped kind together with its count
(e.g. `"untrusted .lazyboards.yml: stripped 2 keymap shell binding(s), 1
cleanup field(s) -- run `lazyboards trust` to allow this file's shell
commands"`).

## Hash identity

Trust is keyed on the local config file's raw content hash, computed by
`internal/config.hashConfigBytes` as `"sha256:" + hex(sha256(data))` over the
exact bytes read from `.lazyboards.yml` — not a normalized/re-marshaled form,
so a single whitespace or comment change produces a different hash and drops
back to untrusted. `HashLocalConfig` is the file-reading wrapper `Load()` and
the CLI verbs use; `Config.LocalHash` carries the hash `Load()` computed
alongside the rest of the resolved config (empty if no local file was read).

## Repo identity (`Path`) is not part of the trust decision

`TrustEntry` also carries a `Path` field (`internal/config/trust.go`), the
stable per-repo identity `resolveTrustIdentity` (`trust_identity.go`)
resolves: the git common directory (absolute) when resolvable — so every
worktree of a repo converges on one identity — else the absolute local
config path, else `""`. It is easy to misread `Path` as a second trust key
("trusted at this hash *for this repo*"); it is not. **The trust decision
stays content-hash only** (`Trust.Trusts`, unchanged by this field): an
entry whose `Path` matches the repo you're in but whose `Hash` doesn't match
the file's current content is not trusted, full stop — `Path` is never
consulted by `Trusts`.

`Path`'s only job is deciding whether a *re-approval prompt* is worth
showing: `Trust.StaleTrust(hash, path)` reports whether `hash` is currently
untrusted **but** `path` was trusted before under a different hash — i.e.
"you've reviewed this repo's config before, just not this exact edit of it."
A genuinely first-ever untrusted load (no prior entry recorded for this
`Path`) never counts as stale, so it never short-circuits the normal
strip-and-notify flow into a prompt. An empty `Path`, on either side of any
comparison, never matches — mirroring `Trusts`'s existing empty-hash guard —
so every entry written before this field existed degrades to "no identity
recorded" rather than false-matching against each other or against a
freshly-resolved identity that happens to also be `""`.

## In-app re-approval prompt (`trustConfirmMode`, #640/#644)

`main()` resolves the current repo identity via `resolveTrustIdentity(".git",
config.DefaultLocalPath)` and feeds it, alongside the already-loaded `cfg`
and `trust` values (no extra I/O — both are the same values every other
`config.Load` call in `main()` reuses), into `trustConfirmEntry`. That
function's gate is deliberately narrower than "something got stripped": it
checks `cfg.LocalHash != "" && !trust.Trusts(cfg.LocalHash)` (the exact
semantics `Trust.StaleTrust` encapsulates) and then `trust.StaleTrust`'s own
identity match — **never** `len(cfg.Notices) > 0`. `Notices` is populated
only when a sink was actually stripped, so an untrusted `.lazyboards.yml`
that happens to declare no shell bindings or `cleanup:` at all would produce
an empty `Notices` and, if gated on that instead, never prompt — even though
it is exactly the "content changed, please re-review" case this feature
exists for.

On a hit, `main()` starts the board in `trustConfirmMode` instead of the
normal `loadingMode`, with `Board.trustConfirm` populated (`hash`, `identity`,
and the stale entry's `note`, carried forward for display and for the eventual
accept-write). `Board.Init()` returns `nil` for this mode — the same
early-return shape it already uses for `firstLaunch` — so the initial board
fetch and every other startup watcher (cenci-watch, git status polling, the
update check) are deferred rather than racing the prompt; both the mode's
`skip` and `trust` outcomes resume startup via the extracted
`Board.startupCmds()` once the user has decided.

The prompt itself (`t`/`s`/`esc`, `keymap.ModeTrustConfirm`,
`handleTrustConfirmModeKey`/`runTrustConfirmCommand`, `mode_handlers.go`)
offers exactly two outcomes:

- **Skip** (`s`/`esc`, `trust_confirm.skip`) clears `Board.trustConfirm` and
  transitions straight to `loadingMode`, continuing startup — byte-identical
  to today's silent-strip behavior, including `Board.startupWarning` (seeded
  before the mode was ever entered) still surfacing as a timed status-bar
  warning once the first fetch lands.
- **Trust** (`t`, `trust_confirm.trust`) runs `acceptTrustCmd`
  asynchronously: it writes a `TrustEntry{Hash, Path, Note}` for the new
  content via `config.UpsertTrustEntry`/`config.SaveTrust` (replacing the
  stale entry for this identity, carrying its `Note` forward), then reloads
  `config.Load` → `config.ResolveKeymap` against the now-trusted store and
  applies the result via `Board.withKeymap` plus `Board.columnConfigs =
  cfg.Columns` — mirroring `main()`'s own startup sequence (the only other
  place that `Load` → `ResolveKeymap` → `withKeymap` chain exists), **not**
  `handleConfigSaved`, which never re-resolves the keymap because
  `config.Save` only ever changes provider/repo. The board's
  `repoOwner`/`repoName`/`providerName`/`provider`/`defaultActions` are left
  untouched: accepting a trust re-approval is not a repo retarget. Every step
  fails closed — a malformed store is never rewritten, and a failed reload
  never applies a half-updated board; the board stays in `trustConfirmMode`
  with a visible error, and the user can still retry `t` or fall back to
  `s`/`esc`.

`Board.trustConfirm.note` is untrusted-ish free-form text (a hand-edited or
malformed `trust.yml` could carry control bytes, ANSI escapes, or a bidi
override) and is rendered through the same `fitQuotedTitle`/
`sanitizeSingleLine` bounding every other inlined-untrusted-string prompt in
this codebase uses — never raw.

## Store location and format

The trust store lives at `~/.config/lazyboards/trust.yml`
(`config.DefaultTrustPath`), a YAML document shaped as:

```yaml
trusted:
  - hash: "sha256:<hex>"
    note: "owner/repo"
    path: "/home/user/repos/owner-repo/.git"
```

`note` is a free-form label (the CLI populates it with the cwd) kept purely
for the user's own reference — it plays no role in the trust decision, which
is hash-only. `path` (`omitempty` — absent entirely on a legacy entry) is
the load-bearing repo identity described above: unlike `note`, it *is* read
back by the app (`PriorEntryForPath`/`StaleTrust`), but only to decide
whether to offer a re-approval prompt, never to decide trust itself.

`SaveTrust` writes it defensively: the parent directory is created and
explicitly `chmod`'d to `0700` (tightened even if it pre-existed looser,
since this store gates command execution — stricter than the `0700`
create-time-only mode `SaveState` uses for non-security runtime state), and
the file itself is written to a `0600` temp file in the same directory and
atomically renamed into place, so the store's content is never briefly
reachable at a looser mode than its final one.

`LoadTrust` treats a missing file as "nothing trusted yet" (not an error), but
unlike the runtime state file, a malformed or wrong-shape trust document is
**never** silently downgraded to an empty result — it's a load error, since a
parse failure here must be visible rather than quietly trusting nothing (which
happens to be safe) or quietly trusting everything (which wouldn't be, if a
different malformed-parse path existed). Callers that can't distinguish
"nothing trusted" from "couldn't read the store" fail closed to a zero-value
`Trust{}` (trusts nothing) rather than aborting startup.

## `lazyboards trust` / `lazyboards untrust`

Two argument-free CLI verbs, dispatched the same way `lazyboards --version`
is: a bare `trust` or `untrust` as the sole argument (`cli_trust.go`'s
`trustVerb`/`runTrustVerb`). No flags are supported — `trust --force` doesn't
match either verb and falls through to the normal board-launch flow.

- **`lazyboards trust`** hashes the local config at the resolved local path,
  resolves the repo identity (`resolveTrustIdentity`), and grants a
  `TrustEntry` for that hash (with a `note` identifying the cwd, and `path`
  set to the resolved identity) via `config.UpsertTrustEntry`. This is the
  *bootstrap* path for the identity feature described above: it's the only
  code path that ever writes a `Path`-bearing entry in the first place, so
  running it is what makes a repo eligible for the in-app re-approval prompt
  at all. `UpsertTrustEntry` drops any existing entry that shares the same
  `Path` before appending the new one, so re-running `trust` after the
  file's content changed **replaces** the stale entry for that repo instead
  of accumulating a second one — this also self-heals a pre-existing
  duplicated or stale entry on the very next grant. Running it twice against
  unchanged content is still idempotent (the replacement is a same-hash
  no-op).
- **`lazyboards untrust`** removes every entry matching the local config's
  current hash. Idempotent: running it when nothing is trusted (or after it
  already removed the entry) is a no-op, not an error.

Both verbs read and write **only** the local config file (read) and the trust
store file (read/write) — the user's global `~/.config/lazyboards/config.yml`
is never touched by either.

Exit codes: `0` on success (including the idempotent no-op cases above).
Non-zero, with an explanatory message on the given writer, when: no local
config file exists at the resolved path ("nothing to trust/untrust"), the
local config can't be read, or the trust store itself is malformed/unreadable.
The malformed-store case fails closed and leaves the store's bytes completely
unchanged — a broken store is reported, never rewritten out from under the
user.

## `Save`'s carry-forward

`config.Save(path, provider, repo, trustPath)` (used by the in-app config
modal, `c`) never *grants* trust on its own — it only carries an
already-trusted file's trust forward across its own rewrite, so saving through
the app doesn't silently revoke trust the user explicitly granted via
`lazyboards trust`.

Concretely: `Save` hashes the file's content immediately before writing
(`preHash`) and immediately after (`postHash`). If `preHash` was trusted, the
matching trust-store entry is updated to `postHash` (deduplicated against any
entry that already has that hash) via `carryTrustForward`; if `preHash` was
*not* trusted — including because the store is empty, missing, or malformed —
nothing is added, and the post-write file remains untrusted like before. An
empty `trustPath` disables the whole carry-forward step (no store I/O at
all). Any error loading or saving the store during carry-forward is swallowed:
the config write itself has already succeeded by that point and must never be
failed by a broken trust store.

## Surfacing strip notices

A run that stripped anything surfaces it twice, once per audience:

- **stderr**, via `main.go`'s `printNotices`. It runs once per process,
  before BubbleTea takes the terminal over (so it's visible in a plain
  shell), printing each group's lines in order, one sanitized
  (`sanitizeSingleLine`) line per entry.
- **in-app status bar**, via `Board.startupWarning`: `main.go` seeds it from
  `cfg.Notices` (joined with `"; "`) when non-empty, and it's applied as a
  timed warning message (mirroring the existing `cleanupBreakerWarning`
  hand-off) on the first successful board fetch, then cleared — a one-shot
  notice for the user who never sees the pre-altscreen stderr output at all.

## Residual accepted risk

Trust only ever gates `type: shell` constructs. A local config can still
rebind a destructive **built-in** command onto an innocuous-looking key —
e.g. binding `card.delete` onto `j` — without needing trust at all, since that
binding is a catalogued command id, not a shell action. This is deliberately
out of scope for the trust store: every destructive built-in already sits
behind its own confirm step (delete's two-step confirm, close's
close-confirm, etc.), so the worst case is an unpleasant surprise requiring a
`y`/`n` decision, not unattended code execution. Reviewing a repo's
`.lazyboards.yml` before running lazyboards inside it remains good practice
for this reason (see the README's [Keymaps](../README.md#keymaps) security
note), but it is not a gap the trust store is meant to close.

Trust is also keyed on the local config file's **content only** ("Hash
identity" above), never on `(path, content)` or `(repo, content)`. A
byte-identical `.lazyboards.yml` in a different repo inherits the exact same
trust grant — e.g. an org-wide templated config trusted once in one repo
covers every other repo checking out that same template unchanged. The
impact is limited (identical bytes can only ever produce identical commands,
so this never grants anything beyond what was already reviewed and trusted),
but the property is non-obvious: trusting a file does not scope that trust to
"this repo," only to "this exact content," wherever it's found.
