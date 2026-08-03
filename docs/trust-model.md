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

A "sink" is any local-origin construct that shells out. Three kinds exist,
tallied and stripped independently by `internal/config/trust_strip.go`'s
`stripLocalShellSinks`:

| Sink kind | Source | Stripped when untrusted |
|---|---|---|
| Keymap shell bindings | `keymaps.<mode>.<key>` / `keymaps.columns.<name>.<key>` entries whose value is an inline `type: shell` action | The binding is deleted from the table; if that empties a mode/column table entirely, the whole table entry is removed so the mode/column falls back to inheriting the global table, rather than resolving to an explicit-but-empty one |
| Legacy shell actions | Top-level `actions:` / `columns[].actions:` entries with `type: shell` (pre-#510 config shape) | The action is deleted; a matching global entry (byte-identical) is restored in its place |
| Cleanup fields | `cleanup:` / `columns[].cleanup` | Reset to the matching global value (or unset if none), never to an empty string |

`type: url` actions, and keymap bindings to a catalogued built-in command id,
are never candidates for stripping — only `type: shell` is an executing
construct. Stripping is decided by comparing the local value against a
snapshot of the *global* document taken before the local file was merged in:
anything byte-identical to its global counterpart is genuinely global
(inherited, not locally declared) and is left alone, whatever the local file's
trust state; anything else is stripped unconditionally. This fails safe
(over-strip, never under-strip) and closes a YAML merge-key/alias bypass a
raw "was this key literally in the document" walk would have missed.

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

## Store location and format

The trust store lives at `~/.config/lazyboards/trust.yml`
(`config.DefaultTrustPath`), a YAML document shaped as:

```yaml
trusted:
  - hash: "sha256:<hex>"
    note: "owner/repo"
```

`note` is a free-form label (the CLI populates it with the cwd) kept purely
for the user's own reference — it plays no role in the trust decision, which
is hash-only.

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
  and adds a `TrustEntry` for that hash to the trust store (with a `note`
  identifying the cwd) unless an entry for that exact hash is already present.
  Idempotent: running it twice against unchanged content never appends a
  duplicate entry.
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

- **stderr**, via `main.go`'s `printNotices`, generalized from the older
  legacy-config deprecation printer to also cover `Config.Notices`. It runs
  once per process, before BubbleTea takes the terminal over (so it's visible
  in a plain shell), printing each group's lines in order, one sanitized
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
