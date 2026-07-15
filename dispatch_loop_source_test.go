package main

import (
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
)

// Ticket #403: the dispatch modal's "Loop:" line prefers the live dispatch
// state pushed over the daemon socket (Board.agentSnapshot.Dispatch) and
// falls back to the one-shot `cenci dispatch status --json` result when the
// live source is absent or no longer trusted. dispatchLoopSource is the pure
// render-time selector; this table covers every row of its precedence matrix
// (per the CLAUDE.md full-precedence rule), setting b.agentSnapshot /
// b.cenciWatchConsecutiveErrors directly per the cenciwatch_test.go
// convention. The live and CLI fixtures carry DISTINCT values so a
// preference row cannot pass coincidentally by selecting identical state
// from the wrong source.
func TestDispatchLoopSource_PrecedenceMatrix(t *testing.T) {
	live := &cenciwatch.DispatchState{Enabled: true, DaemonRunning: true, Interval: "7m", LastDispatched: 3, LastSkipped: 2}
	cli := &cenciwatch.DispatchState{Enabled: false}
	const cliGuardErr = "cenci version too old for loop status — upgrade to use this feature"

	tests := []struct {
		name        string
		watcher     cenciwatch.Watcher
		snapshot    *cenciwatch.StateSnapshot
		consecutive int
		cliLoop     *cenciwatch.DispatchState
		cliErr      string
		wantLoop    *cenciwatch.DispatchState
		wantErr     string
	}{
		{
			name:     "watcher disabled (cenci: false) uses CLI",
			watcher:  nil,
			snapshot: nil,
			cliLoop:  cli,
			wantLoop: cli,
		},
		{
			name:     "enabled but never connected (no snapshot yet) uses CLI",
			watcher:  &cenciwatch.FakeWatcher{},
			snapshot: nil,
			cliLoop:  cli,
			wantLoop: cli,
		},
		{
			name:     "connected pre-#219 daemon (snapshot without dispatch data) uses CLI",
			watcher:  &cenciwatch.FakeWatcher{},
			snapshot: &cenciwatch.StateSnapshot{},
			cliLoop:  cli,
			wantLoop: cli,
		},
		{
			name:     "live dispatch state present and trusted uses live",
			watcher:  &cenciwatch.FakeWatcher{},
			snapshot: &cenciwatch.StateSnapshot{Dispatch: live},
			cliLoop:  cli,
			wantLoop: live,
		},
		{
			name:        "one tolerated watcher blip (below threshold, #333) still uses live",
			watcher:     &cenciwatch.FakeWatcher{},
			snapshot:    &cenciwatch.StateSnapshot{Dispatch: live},
			consecutive: cenciWatchClearThreshold - 1,
			cliLoop:     cli,
			wantLoop:    live,
		},
		{
			name:        "disconnected (threshold reached) falls back to CLI, never a silently-stale live value",
			watcher:     &cenciwatch.FakeWatcher{},
			snapshot:    &cenciwatch.StateSnapshot{Dispatch: live},
			consecutive: cenciWatchClearThreshold,
			cliLoop:     cli,
			wantLoop:    cli,
		},
		{
			name:        "disconnected AND pre-#219 snapshot passes the CLI guard message through",
			watcher:     &cenciwatch.FakeWatcher{},
			snapshot:    &cenciwatch.StateSnapshot{},
			consecutive: cenciWatchClearThreshold,
			cliLoop:     nil,
			cliErr:      cliGuardErr,
			wantLoop:    nil,
			wantErr:     cliGuardErr,
		},
		{
			name:     "nothing anywhere (zero-value board) returns nil for renderLoopLine's upgrade guard",
			watcher:  nil,
			snapshot: nil,
			cliLoop:  nil,
			wantLoop: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDispatchTestBoard(t)
			b.cenciWatcher = tt.watcher
			b.agentSnapshot = tt.snapshot
			b.cenciWatchConsecutiveErrors = tt.consecutive
			b.dispatch.loop = tt.cliLoop
			b.dispatch.loopErr = tt.cliErr

			loop, loopErr := b.dispatchLoopSource()

			if loop != tt.wantLoop {
				t.Errorf("dispatchLoopSource() loop = %+v, want %+v", loop, tt.wantLoop)
			}
			if loopErr != tt.wantErr {
				t.Errorf("dispatchLoopSource() loopErr = %q, want %q", loopErr, tt.wantErr)
			}
		})
	}
}
