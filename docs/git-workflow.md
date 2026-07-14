# Git Workflow

## Worktrees
All feature work happens in worktrees under `.worktrees/`.
Main worktree stays on `main` — read-only for implementation.

```bash
# Create
git worktree add .worktrees/<id>-<desc> -b feature/<id>-<desc>
```

## Branch Naming
Use the pattern from `.claude/config.json`:
- GitHub: `feature/<id>-<short-description>`

## Commit Format
```
<type>(<scope>): <description>

<body>

<ticket-ref>
```

Types: feat, fix, refactor, test, docs, chore
Ticket ref: `#123` (GitHub) — per config.json

## PR Workflow
No hard PR size limit. 1 ticket = 1 PR targeting `main`.
Multiple commits within a PR are fine — use them to organize logical steps.

## Stacked PRs
When a feature exceeds ~300 lines (per CLAUDE.md), split into stacked PRs:

- When scaffolding an unused constant/function/type in Part A for Part B to consume, `go build` and `go vet` will pass locally (they only flag unused imports and unused local variables, never unused package-level declarations). However, `golangci-lint` (backing GitHub Actions CI) includes the `unused` linter (staticcheck U1000 check) which DOES flag unused package-level symbols. Add `//nolint:unused` with a doc comment explaining the symbol is intentionally unused and will be consumed in the stacked follow-up PR. Always verify the full CI suite passes (not just local `go build`/`go test`) before considering a stacked PR complete.
