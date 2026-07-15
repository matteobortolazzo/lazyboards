# BubbleTea Cmd & Async Testing Patterns

Conventions for propagating `tea.Cmd` through the component hierarchy and for testing async behavior (goroutines, timers, subscriptions) without hanging.

## Rules

- Never discard a `tea.Cmd` returned from a sub-model's `Update()` (textinput, textarea, etc.) — even with `_`. It carries async work like animations and timers (e.g. textinput's cursor-blink schedule). Always capture and propagate it: `var cmd tea.Cmd; model, cmd = model.Update(msg); return model, cmd`.
- When a helper/command builder computes a `tea.Cmd` (e.g. a cleanup command), verify it is applied via `tea.Batch()` on *every* conditional return path in the caller, not just the first one written. A Cmd computed early and only batched into one branch is silently dropped on the others — the bug surfaces only when a test exercises the un-batched path.
- Resetting a `bubbles/textarea` viewport scroll position after a resize requires calling `View()` first (populates `viewport.lines` via `SetContent()`) and then `Update(nil)` (triggers `repositionView()`). `SetValue(Value())` or `Reset()` alone does not move the scroll position — `repositionView()` is unexported and only runs during `Update()`, and it depends on `viewport.lines`, which is only populated inside `View()`.
- Test helpers that execute `tea.Cmd` values directly (`execCmds`, `collectMsgs`) must never call `cmd()` synchronously — `tea.Tick` blocks on `time.After(d)` and will hang the test for the tick's full duration. Wrap execution in a goroutine with a short `select`/timeout (100ms); a command that doesn't return within that window is treated as not-yet-fired. For tick-based features under test, use a 1ms interval so the tick completes inside the timeout.
- `sendKey`-style test helpers must capture and execute the `tea.Cmd` returned from `Update()`, not discard it — commands that trigger async work (e.g. a shell action returning `actionResultMsg` via a background goroutine) are otherwise invisible to the test.
