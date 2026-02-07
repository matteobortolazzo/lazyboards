# Git Workflow

## Worktrees
All feature work happens in worktrees under `.worktrees/`.
Main worktree stays on `main` — read-only for implementation.

```bash
# Create
git worktree add .worktrees/<id>-<desc> -b feature/<id>-<desc>

# Create stacked
git worktree add .worktrees/<id>-<desc> -b feature/<id>-<desc> feature/<prev>

# Remove after merge
git worktree remove .worktrees/<id>-<desc>
```

## Branch Naming
Use the pattern from `.claude/config.json`:
- `feature/<id>-<short-description>`

## Commit Format
```
<type>(<scope>): <description>

<body>

#<issue-number>
```

Types: feat, fix, refactor, test, docs, chore
Ticket ref: `#123` (GitHub Issues)

## PR Size
Max ~300 lines. If larger, split into stacked PRs.

## Stacked PRs
1. First PR targets `main`
2. Subsequent PRs target previous feature branch
3. Note in description: "Stack: 2/3 — depends on #<prev>"
4. After merge, rebase subsequent branches onto `main`
