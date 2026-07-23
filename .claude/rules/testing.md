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

## When a Test Forces an Implausible Production Shape
If making a test pass requires production code to do something the plan/spec doesn't call for — a duplicated side-effecting call, an unreachable branch, a magic constant — stop and fix the test, don't bend production to it. A "guard against a race" comment justifying two concurrent `tea.Batch` Cmds is a red flag: batched Cmds run concurrently and cannot order a write-then-read.

## Shared Fixtures
- Modifications to `internal/provider/fake.go` or other shared test fixtures require running the full test suite, not just tests that directly import the fixture. Multiple test files depend on specific fixture properties (e.g., "Card #1 must have zero LinkedPRs for PR-gating tests" in `delete_mode_test.go` and "ListOpenPRs must have exactly N entries" in other PR-count tests). Test failures from fixture changes only surface when the complete suite runs, so partial test runs during implementation cannot validate safety.
- Fixture default values that coincidentally match hardcoded "correct answers" can silently mask bugs in unrelated features. When a feature consumes fixture data to reference a special/fixed entity, write a test that deliberately uses a different fixture value to expose misuse — a test like `TestInit_UpdateCheckTargetsLazyboardsRepoNotTrackedRepo` that verifies the feature does *not* mistakenly use the fixture's user-repo when it should always use the app's own repo.

## Provider Layer Fields
- When adding a new field to `provider.Card` in `internal/provider/github.go`'s `FetchBoard`, add a dedicated unit test in `github_fetchboard_test.go` following the established `TestGitHubFetchBoard_<FieldName>` pattern (e.g., `TestGitHubFetchBoard_CardCreatedAtPopulated`, `TestGitHubFetchBoard_AssigneesPopulated`). Do not assume higher-level integration tests (like detail-panel tests that construct `Card{}` literals directly) validate the provider layer's field extraction — the IPC boundary between GitHub GraphQL and `provider.Card` must be tested explicitly at the source.
