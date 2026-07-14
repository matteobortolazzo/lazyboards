package agentwatch

// StateSnapshot is the top-level message the agentwatch daemon broadcasts
// over its unix socket, one NDJSON line per snapshot. The shape mirrors the
// daemon's public, stdlib-only wire contract (agent-stack's
// agentwatch/pkg/watch package): fields are additive-only and unknown fields
// are ignored on decode, so this local copy keeps decoding correctly across
// daemon upgrades without importing the agent-stack module.
type StateSnapshot struct {
	// Timestamp is the RFC 3339 time at which the daemon built this snapshot.
	Timestamp string `json:"timestamp"`
	// Windows is the current set of tracked windows, one entry per window.
	Windows []WindowState `json:"windows"`
	// Summary holds aggregate counts across Windows.
	Summary StatusSummary `json:"summary"`
	// Dispatch describes the fleet-wide dispatch loop's live state, if the
	// daemon reports it (nil on pre-#219 daemons that predate the loop).
	Dispatch *DispatchState `json:"dispatch,omitempty"`
}

// DispatchState describes the fleet-wide dispatch loop's live state, as
// broadcast by the agentwatch daemon. Field names and JSON tags are pinned
// to the authoritative producer source (agent-stack's
// agentwatch/pkg/watch/snapshot.go), not guessed or copied from a doc
// comment (see #316/#317 lesson).
type DispatchState struct {
	// Enabled reports whether the dispatch loop is turned on.
	Enabled bool `json:"enabled"`
	// DaemonRunning reports whether the agentwatch daemon process is running.
	DaemonRunning bool `json:"daemon_running"`
	// Interval is the configured dispatch interval, e.g. "5m".
	Interval string `json:"interval,omitempty"`
	// PassRunning reports whether a dispatch pass is currently executing.
	PassRunning bool `json:"pass_running"`
	// LastRunAt is the RFC 3339 time of the last completed dispatch pass.
	LastRunAt string `json:"last_run_at,omitempty"`
	// LastDispatched is the number of cards dispatched in the last pass.
	LastDispatched int `json:"last_dispatched"`
	// LastSkipped is the number of cards skipped in the last pass.
	LastSkipped int `json:"last_skipped"`
	// LastError is the error message from the last failed dispatch pass, if
	// any.
	LastError string `json:"last_error,omitempty"`
}

// WindowState describes a single tracked window.
type WindowState struct {
	// Session is the tmux session name the window belongs to.
	Session string `json:"session"`
	// WindowIndex is the window's index within its session, as a string.
	WindowIndex string `json:"window_index"`
	// WindowName is the "<number>-<slug>" window name. It is the stable join
	// key lazyboards uses to associate a card with its live agent window.
	WindowName string `json:"window_name"`
	// TaskName is the human-readable task extracted for the window, if any.
	TaskName string `json:"task_name"`
	// Status is the window's current status (for example "idle", "running",
	// "done", "stopped", "need-input", or "failed").
	Status string `json:"status"`
	// Agent identifies the coding agent detected in the window, if known.
	Agent string `json:"agent,omitempty"`
	// ManuallyNamed reports whether the window name was set by the user
	// rather than derived by agentwatch.
	ManuallyNamed bool `json:"manually_named"`
}

// StatusSummary counts tracked windows by status.
type StatusSummary struct {
	Total     int `json:"total"`
	Idle      int `json:"idle"`
	Running   int `json:"running"`
	Done      int `json:"done"`
	Stopped   int `json:"stopped"`
	NeedInput int `json:"need_input"`
	Failed    int `json:"failed"`
}
