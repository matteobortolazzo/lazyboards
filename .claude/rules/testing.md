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
