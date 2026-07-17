# View-State Consistency

When an event handler guards against an invalid state, the view renderer must use the same guard to avoid misleading the user into taking actions that will silently no-op.

## Rules

- When an event handler (key press, mouse click, etc.) checks a condition like `if b.state.repo == ""` to skip an action, the view renderer must also guard that same condition and render an alternative message (e.g., "No repository detected") instead of rendering placeholder fields (e.g., blank "Repo: " line with "Enroll" hint). Otherwise, users see action hints and try actions that silently fail due to the guard, creating the illusion of a bug.
- Example: dispatch modal's `handleDispatchModeKey` guards Enter on `repo == ""` (silent no-op), while `viewDispatchModal` previously rendered the default "ready to enroll" state for zero-value `dispatchState{repo: ""}`, showing blank repo/dir fields and "Enroll" hints. Fix: add explicit case for `repo == ""` rendering "No repository detected" with no action hints.
- In a two-step confirm flow (open-confirm key, then a separate confirm/commit key), re-check the entry guard at the **commit** step — don't assume the state that was valid when the confirm opened is still valid when it's confirmed. Async messages (background status polls, live socket pushes) can flip `loading`/`err`/`running` or null out the state being acted on in the gap between the two keypresses. The open-confirm handler and the commit handler must share the same guard.
- Example: dispatch loop toggle's `handleDispatchModeKey` gates the `l` (open-confirm) key on `!loading && err == "" && !running`; the `y` (commit) branch must repeat that same check — plus its existing `loop == nil` defense — before firing `toggleLoopCmd`, otherwise a `y` pressed after a background poll set `err` would toggle on top of the error.
