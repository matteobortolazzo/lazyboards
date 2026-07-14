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
