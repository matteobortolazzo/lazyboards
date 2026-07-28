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
Use for a short chain (2–3 PRs) whose intermediate states are safe to ship.
1. First PR targets `main`
2. Subsequent PRs target previous feature branch
3. Note in description: "Stack: 2/3 — depends on #<prev>"
4. After merge, rebase subsequent branches onto `main`

## Epic Integration Branches
Use when an epic has **4+ children**, or when any intermediate state would leave the
app half-migrated. `main` must never sit in a half-migrated state.

1. Branch the integration branch off `main`, named after the epic ticket:
   ```bash
   git branch feature/<epic-id>-<desc> origin/main
   git push -u origin feature/<epic-id>-<desc>
   ```
2. Every child branches from it and PRs into it — **no child PR targets `main`**:
   ```bash
   git worktree add .worktrees/<id>-<desc> -b feature/<id>-<desc> feature/<epic-id>-<desc>
   gh pr create --base feature/<epic-id>-<desc>
   ```
3. If a child's dependency has not merged into the integration branch yet, branch from
   the dependency's branch and re-target the PR base once the dependency lands.
4. Rebase the integration branch onto `main` as unrelated work merges.
5. When the last child lands, open one integration branch → `main` PR. The per-child
   ~300-line cap still applies; the final PR is already-reviewed work.

Record the integration branch and the per-child base in the epic ticket and in every
child ticket (a `### Branch` section), so an implementing agent never has to infer it.

**Worktree base must match plan's `### Branch`**: When a child ticket's plan names an
epic integration branch as its base, verify the worktree was created off that branch
(not `main`). The `cenci pipeline worktree` CLI defaults to `main` and will not respect
the plan's `### Branch` section — manually verify or recreate the worktree with the
correct base (e.g., `git worktree remove .worktrees/<id>-<desc>; git worktree add .worktrees/<id>-<desc> -b feature/<id>-<desc> origin/feature/<epic-id>-<desc>`). A mismatched base causes `cenci pipeline plan-check` to diff against the wrong branch, potentially showing the plan as "stale and safe" when the actual target branch has already landed conflicting structural changes.
