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
- NEVER assert call counts
- NEVER copy expected values from implementation
