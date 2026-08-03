# Testing Rules

## Test-First (TDD)
1. Write failing test (red)
2. Implement to make it pass (green)
3. Refactor (keep green)

## Integration Tests (Preferred)
Test real flows end-to-end through the application stack.

## Unit Tests (Complex Logic Only)
- State machines
- Calculations
- Validation rules
- Parsing

## Assertions
- Assert behavior: status codes, response shapes, business state
- Assert business rules: data presence, state transitions
- NEVER hardcode magic values
- NEVER assert call counts (exception: a minimal `== 1` guarding an observable no-duplicate-side-effect invariant, with a comment explaining why)
- NEVER copy expected values from implementation — for values that cross a service/process boundary (socket, API, IPC), assert against a real observed sample of the producer's output, not a value you also hardcode in the fixture. Otherwise producer and consumer can share the same wrong constant and both stay green.
- NEVER discard a BubbleTea `Update()` call's return values with `_` — a discarded `model`/`cmd` makes the test a no-op that passes regardless of implementation. Always capture and assert on both, and set every message field the handler depends on (don't rely on zero values).

## Explicit Risk Coverage
When a plan's `### Risks` section names a specific mitigation (e.g., "empty-URL guard mirroring `handleTicketOpenKey`'s 'URL not available' path"), write a test case that exercises it before marking implementation done. Happy-path coverage alone will not expose missing edge-case guards. Without explicit coverage, implementation will pass Phase 4 verification and only surface during external review, costing a fix cycle.

## State Coverage for Composite Hints
When a hint's displayed key is composited from multiple command IDs via a variadic join helper (e.g. `panelHintKey(entries, idA, idB)`), **and the composite hint appears only in certain UI states** (state-gated rendering), the hint→dispatch invariant test's state matrix must include every such state — not just a representative sample. A composite hint silently assumes all its constituent command IDs are properly dispatched in that state; if the dispatcher's `switch` case only handles one of the composite IDs, the hint will advertise a key that silently no-ops. This assumption is exactly what can go stale when states change. Without full state coverage, the invariant test passes even though a missing dispatcher case exists.

## When a Test Forces an Implausible Production Shape
If making a test pass requires production code to do something the plan/spec doesn't call for — a duplicated side-effecting call, an unreachable branch, a magic constant — stop and fix the test, don't bend production to it. A "guard against a race" comment justifying two concurrent `tea.Batch` Cmds is a red flag: batched Cmds run concurrently and cannot order a write-then-read.

## Parity Tests During Refactoring
When writing a parity/regression test during a refactor that will delete a legacy dispatch/lookup function, assert against the canonical upstream data source (config defaults, fixtures) directly, never against the legacy function being deleted — a test that calls the soon-to-be-dead path creates the exact "kept alive for tests" trap the dead-code rule prohibits. Dispatch through the new code path and assert behavior against the canonical source, mirroring what the legacy function was orchestrating.

## Shared Fixtures
- Modifications to `internal/provider/fake.go` or other shared test fixtures require running the full test suite, not just tests that directly import the fixture. Multiple test files depend on specific fixture properties (e.g., "Card #1 must have zero LinkedPRs for PR-gating tests" in `delete_mode_test.go` and "ListOpenPRs must have exactly N entries" in other PR-count tests). Test failures from fixture changes only surface when the complete suite runs, so partial test runs during implementation cannot validate safety.
- Fixture default values that coincidentally match hardcoded "correct answers" can silently mask bugs in unrelated features. When a feature consumes fixture data to reference a special/fixed entity, write a test that deliberately uses a different fixture value to expose misuse — a test like `TestInit_UpdateCheckTargetsLazyboardsRepoNotTrackedRepo` that verifies the feature does *not* mistakenly use the fixture's user-repo when it should always use the app's own repo.

## Provider Layer Fields
- When adding a new field to `provider.Card` in `internal/provider/github.go`'s `FetchBoard`, add a dedicated unit test in `github_fetchboard_test.go` following the established `TestGitHubFetchBoard_<FieldName>` pattern (e.g., `TestGitHubFetchBoard_CardCreatedAtPopulated`, `TestGitHubFetchBoard_AssigneesPopulated`). Do not assume higher-level integration tests (like detail-panel tests that construct `Card{}` literals directly) validate the provider layer's field extraction — the IPC boundary between GitHub GraphQL and `provider.Card` must be tested explicitly at the source.

## Test File Cleanup Before Submission
Remove TDD-process narration (RED-phase descriptions, "expected state" comments, TODO-for-implementation notes) from test file headers before marking work done. These are internal working notes that describe *how* tests were discovered, not *what* they specify or cover. They should not appear in permanent code.

## Verifying Claimed Code Path Coverage
When a test fixture or helper includes a comment claiming to exercise a specific code path — especially one the plan's `### Risks` section identifies as high-risk — verify the claim via mutation testing before submission. Temporarily remove, break, or invert that code path and confirm the test fails. Passing tests don't prove they exercise the intended path; unverified claims create false confidence that code review must later catch. Without explicit verification, a test's comment and its actual behavior can silently drift.

## Collision Guards for Parallel Merges
When a package merges multiple data groups into single keyed structures via parallel operations (e.g., appending multiple slices into one, or assigning multiple maps into one with different key types), write a dedicated collision-guard test for each merge operation — don't assume that a guard on one (e.g., slice-uniqueness) covers the other (e.g., map-key collisions). A package's `init()` that concatenates `catalog = append(catalog, group1...); append(catalog, group2...)` needs `TestCommands_IDsAreUnique`, and if it also merges `defaultTables = map[Mode]Table{...}; for mode, table := range modalGroup { defaultTables[mode] = table }...`, it needs a separate `TestDefaultModeTableGroups_NoModeCollisions` guard. Silent overwrites in map assignments leave no trace — the second group's entry will silently replace the first with no panic or error.

## Keymap Remap Tests for Text-Input Modes
When writing remap/unbind regression tests for modes routed through `keymap_text.go`'s `textBinding` seam (any mode where `Mode.ConsumesPrintableRunes()` is true: create, config, search, comment, delete, close_confirm, label_confirm), **never remap a command onto a printable rune key** (`"q"`, `"h"`, `"l"`, etc.). The `textBinding` guard (line 31 of `keymap_text.go`) unconditionally refuses to resolve bare printable runes for such modes — even if the config rebinds a command there — because the keystroke must reach the mode's `textinput` component instead of being intercepted as a registry command. A test that rebinds onto `"q"` and then dispatches via `keyMsg("q")` will silently fail (or worse, silently pass for the wrong reason if the mode has a fallback handler that accepts the keystroke) because the binding will never be reached: `textBinding` returns `Binding{}, false` before the lookup even runs. Instead, remap onto non-printable keys like `"f1"` or `"f2"` and dispatch via `arrowMsg(tea.KeyF1)`/`arrowMsg(tea.KeyF2)`. This pattern is already established in `delete_mode_test.go` and is reproduced in `create_mode_test.go` and `config_mode_test.go` (#540 PR 1/2).

## Test Function Naming Consistency
When refactoring changes constants that appear in test function names (keybindings, mode names, feature keys, etc.), use a search/grep pass to rename all test functions whose names embed those constants. A function body that correctly exercises the new key but still has the old key in its name creates a consistency debt that survives code review — the test passes (the body is correct), but the name no longer describes what the test does, making it harder to navigate and audit. External reviewers and future maintainers may spot the inconsistency only if they read the full function; automated searches by key name will be wrong. When a large refactor (like remapping 9 default keybindings) requires mechanical test-name renaming across many files, verify the pass is complete by grepping for any remnants of the old constant in test function names (`TestNormalMode_<OldKey>_*`, `TestOpenPRList_*` if the old key opened PR list, etc.) — a single miss that an earlier pass caught becomes a finding worth capturing, since it indicates the renaming task is not automatically exhaustive.
