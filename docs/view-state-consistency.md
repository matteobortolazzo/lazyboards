# View-State Consistency

When an event handler guards against an invalid state, the view renderer must use the same guard to avoid misleading the user into taking actions that will silently no-op.

## Rules

- When an event handler (key press, mouse click, etc.) checks a condition like `if b.state.repo == ""` to skip an action, the view renderer must also guard that same condition and render an alternative message (e.g., "No repository detected") instead of rendering placeholder fields (e.g., blank "Repo: " line with "Enroll" hint). Otherwise, users see action hints and try actions that silently fail due to the guard, creating the illusion of a bug.
- Example: dispatch modal's `handleDispatchModeKey` guards Enter on `repo == ""` (silent no-op), while `viewDispatchModal` previously rendered the default "ready to enroll" state for zero-value `dispatchState{repo: ""}`, showing blank repo/dir fields and "Enroll" hints. Fix: add explicit case for `repo == ""` rendering "No repository detected" with no action hints.
