# cenciwatch Integration

Conventions for `internal/cenciwatch` — reading the `cenci-watch` daemon's socket snapshots and driving the reconnect/backoff `tea.Cmd`.

## Rules

- `subscribeCenciWatchCmd`'s blocking read (`Watcher.ReadNext() (*Snapshot, error)`) has three outcomes, not two: data, error, and a legitimate `(nil, nil)` (clean socket close, or `FakeWatcher` exhausting its scripted results). Treat `(nil, nil)` as an error — route it to `cenciWatchErrorMsg` — rather than as a successful read. The self-chaining Cmd's success branch has no rate limit and immediately re-subscribes; silently treating a nil snapshot as success resets the reconnect backoff and spins the resubscribe loop unthrottled.
- Status/enum strings that cross the daemon socket boundary must be pinned to the daemon's actual serialized value (`detect.StatusNeedInput.String()` → `"need-input"`, hyphen), never to a doc comment describing it — `cenci-watch`'s own `snapshot.go` doc comment said `"need_input"` (underscore) and was stale relative to the real wire value. Prefer a test that `json.Unmarshal`s a real wire-format NDJSON sample over constructing the struct by hand: hand-built fixtures can copy the same wrong constant as production and both stay green, hiding the mismatch (see `.claude/rules/testing.md` — never copy expected values from implementation).
