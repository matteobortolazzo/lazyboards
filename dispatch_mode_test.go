package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newDispatchTestBoard creates a loaded Board with Width/Height set so View()
// can render normal-mode hints for assertion.
func newDispatchTestBoard(t *testing.T) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	return b
}

// newDispatchTestBoardWithExecutor creates a loaded Board wired to the given
// FakeExecutor, so tests can assert on RunShellOutputCalls and control
// RunShellOutput fixtures for dispatch command wiring tests.
func newDispatchTestBoardWithExecutor(t *testing.T, fe *action.FakeExecutor) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, "", "")
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded
}

// --- Mode transition ---

func TestDispatch_PressD_EntersDispatchMode(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))

	if b.mode != dispatchMode {
		t.Errorf("after pressing 'd': mode = %v, want dispatchMode", b.mode)
	}
}

func TestDispatch_PressD_ResetsStateToLoading(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))

	if !b.dispatch.loading {
		t.Error("after pressing 'd': dispatch.loading = false, want true")
	}
	if b.dispatch.err != "" {
		t.Errorf("after pressing 'd': dispatch.err = %q, want empty", b.dispatch.err)
	}
	if b.dispatch.running {
		t.Error("after pressing 'd': dispatch.running = true, want false")
	}
}

func TestDispatch_Escape_ReturnsToNormalMode(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))
	if b.mode != dispatchMode {
		t.Fatalf("expected dispatchMode after 'd', got %v", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Escape in dispatchMode: mode = %v, want normalMode", b.mode)
	}
}

func TestDispatch_Escape_RestoresNormalHints(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	view := b.View()
	if !strings.Contains(view, "Edit") {
		t.Error("after Escape from dispatchMode, normal hint 'Edit' should be visible")
	}
}

// --- Noop while loading/error/running ---

func TestDispatch_EnterNoop_WhileLoading(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = true

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != dispatchMode {
		t.Errorf("Enter while dispatch.loading=true: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

func TestDispatch_ONoop_WhileLoading(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = true

	b = sendKey(t, b, keyMsg("o"))

	if b.mode != dispatchMode {
		t.Errorf("'o' while dispatch.loading=true: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

func TestDispatch_EnterNoop_WhileError(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = false
	b.dispatch.err = "x"

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != dispatchMode {
		t.Errorf("Enter while dispatch.err set: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

func TestDispatch_EnterNoop_WhileRunning(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = false
	b.dispatch.running = true

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != dispatchMode {
		t.Errorf("Enter while dispatch.running=true: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

// TestDispatch_EnterAndO_NoopWhenClean exercises the fall-through no-op path:
// with dispatch fully zeroed, enter is a no-op because repo == "" and "o" is a
// no-op because enrolled == false (the guards #283 added), so both stay in
// dispatchMode without firing a Cmd.
func TestDispatch_EnterAndO_NoopWhenClean(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch = dispatchState{} // loading=false, err="", running=false

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != dispatchMode {
		t.Errorf("Enter with clean dispatch state: mode = %v, want dispatchMode (no-op)", b.mode)
	}

	b = sendKey(t, b, keyMsg("o"))
	if b.mode != dispatchMode {
		t.Errorf("'o' with clean dispatch state: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

// --- dispatchModeHints gains enter/o entries (#283) ---

func TestDispatchModeHints_IncludesEnterAndO(t *testing.T) {
	keys := make(map[string]bool)
	for _, h := range dispatchModeHints {
		keys[h.Key] = true
	}
	if !keys["enter"] {
		t.Errorf("dispatchModeHints missing 'enter' entry: %+v", dispatchModeHints)
	}
	if !keys["o"] {
		t.Errorf("dispatchModeHints missing 'o' entry: %+v", dispatchModeHints)
	}
}

// --- Pressing 'd' fires an async status query (#283) ---

func TestDispatch_PressD_FiresStatusQueryCmd(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{}, // agentwatch version probe succeeds
			{Stdout: `{"repo":"owner/repo","dir":"/tmp/x","enrolled":false}`},
		},
	}
	b := newDispatchTestBoardWithExecutor(t, fe)

	m, cmd := b.Update(keyMsg("d"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != dispatchMode {
		t.Errorf("after pressing 'd': mode = %v, want dispatchMode", b2.mode)
	}
	if cmd == nil {
		t.Fatal("expected pressing 'd' to fire a Cmd querying dispatch status, got nil")
	}

	msgs := collectMsgs(cmd)
	var found *dispatchStatusMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchStatusMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchStatusMsg among executed commands, got %#v", msgs)
	}
	if found.repo != "owner/repo" {
		t.Errorf("dispatchStatusMsg.repo = %q, want %q", found.repo, "owner/repo")
	}

	if len(fe.RunShellOutputCalls) != 2 {
		t.Fatalf("expected 2 RunShellOutput calls (version probe + dispatch status), got %d: %v", len(fe.RunShellOutputCalls), fe.RunShellOutputCalls)
	}
	if !strings.Contains(fe.RunShellOutputCalls[1], "dispatch status --json --dir") {
		t.Errorf("RunShellOutput called with %q, want substring %q", fe.RunShellOutputCalls[1], "dispatch status --json --dir")
	}
}

// --- Enter/'o' guards: no-op on missing repo/not-enrolled, fire Cmd when idle (#283) ---

func TestDispatch_Enter_NoopWhenRepoEmpty(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{} // idle, repo empty

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != dispatchMode {
		t.Errorf("Enter with empty dispatch.repo: mode = %v, want dispatchMode (no-op)", b2.mode)
	}
	if cmd != nil {
		t.Error("expected Enter to be a no-op (nil Cmd) when dispatch.repo is empty")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("expected no RunShellOutput calls when dispatch.repo is empty, got %v", fe.RunShellOutputCalls)
	}
}

func TestDispatch_Enter_FiresToggleEnrollWhenIdleAndRepoSet(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.dispatch.loading {
		t.Error("expected dispatch.loading=true after Enter fires the enroll toggle")
	}
	if cmd == nil {
		t.Fatal("expected Enter to fire a Cmd when idle and dispatch.repo is set")
	}

	msgs := collectMsgs(cmd)
	found := false
	for _, msg := range msgs {
		if _, ok := msg.(dispatchEnrollMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a dispatchEnrollMsg among executed commands, got %#v", msgs)
	}

	if len(fe.RunShellOutputCalls) == 0 {
		t.Fatal("expected RunShellOutput to be called")
	}
	cmdStr := fe.RunShellOutputCalls[0]
	if !strings.Contains(cmdStr, "dispatch enroll --dir") {
		t.Errorf("expected enroll command, got %q", cmdStr)
	}
	if strings.Contains(cmdStr, "unenroll") {
		t.Errorf("expected enroll (not unenroll) when dispatch.enrolled=false, got %q", cmdStr)
	}
}

func TestDispatch_O_NoopWhenNotEnrolled(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	m, cmd := b.Update(keyMsg("o"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != dispatchMode {
		t.Errorf("'o' while not enrolled: mode = %v, want dispatchMode (no-op)", b2.mode)
	}
	if cmd != nil {
		t.Error("expected 'o' to be a no-op (nil Cmd) when not enrolled")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("expected no RunShellOutput calls when not enrolled, got %v", fe.RunShellOutputCalls)
	}
}

func TestDispatch_O_FiresDispatchOnceWhenEnrolledAndIdle(t *testing.T) {
	fe := &action.FakeExecutor{RunShellOutputStdout: "owner/repo#1 dispatch session-a"}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true}

	m, cmd := b.Update(keyMsg("o"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.dispatch.running {
		t.Error("expected dispatch.running=true after 'o' fires dispatch-once")
	}
	if cmd == nil {
		t.Fatal("expected 'o' to fire a Cmd when enrolled and idle")
	}

	msgs := collectMsgs(cmd)
	found := false
	for _, msg := range msgs {
		if _, ok := msg.(dispatchRunMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a dispatchRunMsg among executed commands, got %#v", msgs)
	}

	if len(fe.RunShellOutputCalls) == 0 {
		t.Fatal("expected RunShellOutput to be called")
	}
	cmdStr := fe.RunShellOutputCalls[0]
	if strings.Contains(cmdStr, "--dir") {
		t.Errorf("dispatch-once must be fleet-wide (no --dir filter), got %q", cmdStr)
	}
}

// --- Update() handlers for dispatchStatusMsg/dispatchEnrollMsg/dispatchRunMsg (#283) ---

func TestDispatch_HandleStatusMsg_Success(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loading: true}

	msg := dispatchStatusMsg{repo: "owner/repo", dir: "/tmp/x", enrolled: true}
	m, _ := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loading {
		t.Error("expected dispatch.loading=false after dispatchStatusMsg")
	}
	if b2.dispatch.repo != "owner/repo" {
		t.Errorf("dispatch.repo = %q, want %q", b2.dispatch.repo, "owner/repo")
	}
	if b2.dispatch.dir != "/tmp/x" {
		t.Errorf("dispatch.dir = %q, want %q", b2.dispatch.dir, "/tmp/x")
	}
	if !b2.dispatch.enrolled {
		t.Error("expected dispatch.enrolled=true")
	}
	if b2.dispatch.err != "" {
		t.Errorf("dispatch.err = %q, want empty", b2.dispatch.err)
	}
}

func TestDispatch_HandleStatusMsg_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loading: true}

	msg := dispatchStatusMsg{err: "agentwatch not found on PATH — install it to use dispatch"}
	m, _ := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loading {
		t.Error("expected dispatch.loading=false after dispatchStatusMsg with err")
	}
	if b2.dispatch.err != msg.err {
		t.Errorf("dispatch.err = %q, want %q", b2.dispatch.err, msg.err)
	}
}

func TestDispatch_HandleEnrollMsg_SuccessRequeriesStatus(t *testing.T) {
	fe := &action.FakeExecutor{RunShellOutputStdout: `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true}`}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loading: true}

	msg := dispatchEnrollMsg{} // success (empty err)
	m, cmd := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.dispatch.loading {
		t.Error("expected dispatch.loading to remain true after a successful enroll (awaiting requery)")
	}
	if cmd == nil {
		t.Fatal("expected a requery Cmd after a successful dispatchEnrollMsg")
	}

	msgs := collectMsgs(cmd)
	var found *dispatchStatusMsg
	for _, m2 := range msgs {
		if sm, ok := m2.(dispatchStatusMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected requery to produce a dispatchStatusMsg, got %#v", msgs)
	}
	if found.repo != "owner/repo" {
		t.Errorf("requeried dispatchStatusMsg.repo = %q, want %q", found.repo, "owner/repo")
	}

	// The enroll RunShellOutput call is simulated by the dispatchEnrollMsg here
	// (out of scope), so the handler's success path must issue exactly one
	// status re-query -- never a duplicate. That re-query is now 2 calls (the
	// version probe + dispatch status), not 1, since queryDispatchStatusCmd
	// itself probes first (#299); this is a genuine behavior change, not test
	// appeasement -- see lessons-learned.md on justified count assertions.
	if len(fe.RunShellOutputCalls) != 2 {
		t.Fatalf("expected exactly 2 RunShellOutput calls (single status requery: probe + status), got %d: %v", len(fe.RunShellOutputCalls), fe.RunShellOutputCalls)
	}
	if !strings.Contains(fe.RunShellOutputCalls[1], "dispatch status") {
		t.Errorf("expected requery call to invoke dispatch status, got %q", fe.RunShellOutputCalls[1])
	}
}

func TestDispatch_HandleEnrollMsg_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loading: true}

	msg := dispatchEnrollMsg{err: "boom"}
	m, _ := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loading {
		t.Error("expected dispatch.loading=false after dispatchEnrollMsg with err")
	}
	if b2.dispatch.err != "boom" {
		t.Errorf("dispatch.err = %q, want %q", b2.dispatch.err, "boom")
	}
}

func TestDispatch_HandleRunMsg_Success(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{running: true}

	msg := dispatchRunMsg{result: "2 dispatched, 1 skipped (all enrolled repos)"}
	m, _ := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.running {
		t.Error("expected dispatch.running=false after dispatchRunMsg")
	}
	if b2.dispatch.lastResult != msg.result {
		t.Errorf("dispatch.lastResult = %q, want %q", b2.dispatch.lastResult, msg.result)
	}
	if b2.dispatch.err != "" {
		t.Errorf("dispatch.err = %q, want empty", b2.dispatch.err)
	}
}

func TestDispatch_HandleRunMsg_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{running: true}

	msg := dispatchRunMsg{err: "boom"}
	m, _ := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.running {
		t.Error("expected dispatch.running=false after dispatchRunMsg with err")
	}
	if b2.dispatch.err != "boom" {
		t.Errorf("dispatch.err = %q, want %q", b2.dispatch.err, "boom")
	}
}

func TestDispatch_HandleRunMsg_StoresLines(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{running: true}

	wantLines := []string{
		"owner/repo1#1 dispatch session-a",
		"owner/repo2#2 skip: already running",
	}
	msg := dispatchRunMsg{
		result: "1 dispatched, 1 skipped (all enrolled repos)",
		lines:  wantLines,
	}
	m, _ := b.Update(msg)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if len(b2.dispatch.lastLines) != len(wantLines) {
		t.Fatalf("dispatch.lastLines = %#v, want %#v", b2.dispatch.lastLines, wantLines)
	}
	for i, want := range wantLines {
		if b2.dispatch.lastLines[i] != want {
			t.Errorf("dispatch.lastLines[%d] = %q, want %q", i, b2.dispatch.lastLines[i], want)
		}
	}
}

// --- classifyAgentwatchError (#283) ---

func TestClassifyAgentwatchError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		stderr string
		assert func(t *testing.T, got string)
	}{
		{
			name:   "exit code 127 in error text -> not found",
			err:    errors.New("exit status 127"),
			stderr: "",
			assert: func(t *testing.T, got string) {
				if !strings.Contains(strings.ToLower(got), "not found") {
					t.Errorf("classifyAgentwatchError() = %q, want a message indicating agentwatch is not found", got)
				}
			},
		},
		{
			name:   "stderr command not found -> not found",
			err:    errors.New("exit status 1"),
			stderr: "sh: agentwatch: command not found",
			assert: func(t *testing.T, got string) {
				if !strings.Contains(strings.ToLower(got), "not found") {
					t.Errorf("classifyAgentwatchError() = %q, want a message indicating agentwatch is not found", got)
				}
			},
		},
		{
			name:   "git repo stderr 'is not a git repository' -> repo not resolvable",
			err:    errors.New("exit status 1"),
			stderr: "fatal: not a git repository (or any of the parent directories)",
			assert: func(t *testing.T, got string) {
				lower := strings.ToLower(got)
				if !strings.Contains(lower, "git") && !strings.Contains(lower, "repo") {
					t.Errorf("classifyAgentwatchError() = %q, want a message mentioning the repo could not be resolved", got)
				}
			},
		},
		{
			name:   "git repo stderr 'getting origin remote url' -> repo not resolvable",
			err:    errors.New("exit status 1"),
			stderr: "error getting origin remote url: no such remote",
			assert: func(t *testing.T, got string) {
				lower := strings.ToLower(got)
				if !strings.Contains(lower, "git") && !strings.Contains(lower, "repo") {
					t.Errorf("classifyAgentwatchError() = %q, want a message mentioning the repo could not be resolved", got)
				}
			},
		},
		{
			name:   "generic fallback prefixes stderr",
			err:    errors.New("exit status 2"),
			stderr: "some unexpected failure",
			assert: func(t *testing.T, got string) {
				want := "agentwatch: some unexpected failure"
				if got != want {
					t.Errorf("classifyAgentwatchError() = %q, want %q", got, want)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAgentwatchError(tc.err, tc.stderr)
			tc.assert(t, got)
		})
	}
}

// --- queryDispatchStatusCmd (#283) ---

func TestQueryDispatchStatusCmd_Success(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{}, // agentwatch version probe succeeds (empty stdout, no error)
			{Stdout: `{"repo":"owner/repo","dir":"/some/dir","enrolled":true}`},
		},
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	if statusMsg.err != "" {
		t.Errorf("dispatchStatusMsg.err = %q, want empty", statusMsg.err)
	}
	if statusMsg.repo != "owner/repo" {
		t.Errorf("dispatchStatusMsg.repo = %q, want %q", statusMsg.repo, "owner/repo")
	}
	if statusMsg.dir != "/some/dir" {
		t.Errorf("dispatchStatusMsg.dir = %q, want %q", statusMsg.dir, "/some/dir")
	}
	if !statusMsg.enrolled {
		t.Error("expected dispatchStatusMsg.enrolled=true")
	}

	if len(fe.RunShellOutputCalls) != 2 {
		t.Fatalf("expected 2 RunShellOutput calls (version probe + dispatch status), got %d: %v", len(fe.RunShellOutputCalls), fe.RunShellOutputCalls)
	}
	if !strings.Contains(fe.RunShellOutputCalls[0], "agentwatch version") {
		t.Errorf("RunShellOutputCalls[0] = %q, want the side-effect-free version probe", fe.RunShellOutputCalls[0])
	}
	cmdStr := fe.RunShellOutputCalls[1]
	if !strings.Contains(cmdStr, "dispatch status --json --dir") {
		t.Errorf("RunShellOutput called with %q, want substring %q", cmdStr, "dispatch status --json --dir")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}
	escapedCwd := action.ShellEscape(cwd)
	if !strings.Contains(cmdStr, escapedCwd) {
		t.Errorf("RunShellOutput called with %q, want it to contain shell-escaped cwd %q", cmdStr, escapedCwd)
	}
}

func TestQueryDispatchStatusCmd_ExecError(t *testing.T) {
	// The canned (unscripted) FakeExecutor fields apply to every RunShellOutput
	// call, so this "command not found" error is what the version probe (the
	// first RunShellOutput call) sees -- dispatch status must never run.
	fe := &action.FakeExecutor{
		RunShellOutputErr:    errors.New("exit status 127"),
		RunShellOutputStderr: "agentwatch: command not found",
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	if statusMsg.err == "" {
		t.Fatal("expected dispatchStatusMsg.err to be non-empty on exec error")
	}
	if !strings.Contains(strings.ToLower(statusMsg.err), "not found") {
		t.Errorf("dispatchStatusMsg.err = %q, want a message indicating agentwatch is not found", statusMsg.err)
	}
}

// TestQueryDispatchStatusCmd_OldBinaryNonJSON is the defensive case: the
// version probe succeeds (so the binary resolved and isn't plain "not
// found"), but dispatch status still returns non-JSON garbage -- e.g. a
// binary new enough for a version probe someone hand-rolled, but still too
// old for the JSON `dispatch status` contract. Must still classify as
// too-old, with a path suffix appended.
func TestQueryDispatchStatusCmd_OldBinaryNonJSON(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{},                   // agentwatch version probe succeeds
			{Stdout: "not json"}, // dispatch status returns garbage
		},
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	if statusMsg.err == "" {
		t.Fatal("expected dispatchStatusMsg.err to be non-empty when stdout is not valid JSON")
	}
	lower := strings.ToLower(statusMsg.err)
	if !strings.Contains(lower, "upgrade") && !strings.Contains(lower, "old") {
		t.Errorf("dispatchStatusMsg.err = %q, want a message indicating the agentwatch binary is too old", statusMsg.err)
	}
	if len(fe.RunShellOutputCalls) < 2 || !strings.Contains(fe.RunShellOutputCalls[1], "dispatch status") {
		t.Errorf("expected dispatch status to run after a successful probe, calls = %v", fe.RunShellOutputCalls)
	}
}

func TestQueryDispatchStatusCmd_EmptyOutput(t *testing.T) {
	fe := &action.FakeExecutor{RunShellOutputStdout: ""}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	if statusMsg.err == "" {
		t.Fatal("expected dispatchStatusMsg.err to be non-empty when stdout is empty")
	}
	lower := strings.ToLower(statusMsg.err)
	if strings.Contains(lower, "old") || strings.Contains(lower, "upgrade") {
		t.Errorf("dispatchStatusMsg.err = %q, want a message about missing output, not a version mismatch", statusMsg.err)
	}
	if !strings.Contains(lower, "no output") && !strings.Contains(lower, "path") {
		t.Errorf("dispatchStatusMsg.err = %q, want a message indicating agentwatch produced no output", statusMsg.err)
	}
}

// TestQueryDispatchStatusCmd_VersionProbeFails_TooOldWithoutRunningStatus is
// the core regression test (#299): an old agentwatch binary that predates
// the `dispatch status` verb does not fail cleanly on an unknown subcommand
// -- Go's flag parsing stops at the first positional argument and silently
// discards the rest of argv, so `dispatch status --json --dir <cwd>`
// degrades, on such a binary, to a REAL `dispatch` pass with real
// side effects (dispatching tickets, creating tmux windows). Probing
// `agentwatch version` first, and refusing to run `dispatch status` when
// that probe fails for any non-"not found" reason, is what prevents a status
// poll from ever accidentally dispatching.
func TestQueryDispatchStatusCmd_VersionProbeFails_TooOldWithoutRunningStatus(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{Err: errors.New("exit status 2"), Stderr: "flag provided but not defined: -json"},
		},
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	lower := strings.ToLower(statusMsg.err)
	if !strings.Contains(lower, "old") && !strings.Contains(lower, "upgrade") {
		t.Errorf("dispatchStatusMsg.err = %q, want a message indicating the agentwatch binary is too old", statusMsg.err)
	}

	// Justified negative-call assertion (see lessons-learned.md): this guards
	// the actual bug being fixed -- a status poll must never invoke a real
	// dispatch pass on a binary that failed the version probe.
	for _, call := range fe.RunShellOutputCalls {
		if strings.Contains(call, "dispatch status") {
			t.Fatalf("expected dispatch status to never run after a failed version probe, but found call %q among %v", call, fe.RunShellOutputCalls)
		}
	}
}

func TestQueryDispatchStatusCmd_VersionProbeSucceeds_StatusRunsAsBefore(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{}, // agentwatch version probe succeeds
			{Stdout: `{"repo":"owner/repo","dir":"/some/dir","enrolled":false}`},
		},
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	if statusMsg.err != "" {
		t.Errorf("dispatchStatusMsg.err = %q, want empty", statusMsg.err)
	}
	if statusMsg.repo != "owner/repo" {
		t.Errorf("dispatchStatusMsg.repo = %q, want %q", statusMsg.repo, "owner/repo")
	}
	if len(fe.RunShellOutputCalls) != 2 {
		t.Fatalf("expected 2 RunShellOutput calls (version probe + dispatch status), got %d: %v", len(fe.RunShellOutputCalls), fe.RunShellOutputCalls)
	}
}

func TestQueryDispatchStatusCmd_TooOldError_IncludesBinaryPath(t *testing.T) {
	resolvedPath := "/home/user/go/bin/agentwatch"
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{Err: errors.New("exit status 2"), Stderr: "flag provided but not defined: -json"}, // version probe fails, not "not found"
			{Stdout: resolvedPath + "\n"}, // command -v agentwatch resolves a path
		},
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	want := "(using " + resolvedPath + ")"
	if !strings.Contains(statusMsg.err, want) {
		t.Errorf("dispatchStatusMsg.err = %q, want it to contain %q so PATH shadowing is visible", statusMsg.err, want)
	}
}

func TestQueryDispatchStatusCmd_NotFoundError_HasNoBinaryPathSuffix(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{Err: errors.New("exit status 127"), Stderr: "sh: agentwatch: command not found"},
		},
	}

	cmd := queryDispatchStatusCmd(fe)
	msg := cmd()

	statusMsg, ok := msg.(dispatchStatusMsg)
	if !ok {
		t.Fatalf("queryDispatchStatusCmd() returned %T, want dispatchStatusMsg", msg)
	}
	if !strings.Contains(strings.ToLower(statusMsg.err), "not found") {
		t.Errorf("dispatchStatusMsg.err = %q, want a message indicating agentwatch is not found", statusMsg.err)
	}
	if strings.Contains(statusMsg.err, "(using") {
		t.Errorf("dispatchStatusMsg.err = %q, want no binary path suffix when the binary itself cannot be found (a path would be misleading)", statusMsg.err)
	}

	// A not-found binary must not trigger a second `command -v` lookup --
	// there is nothing useful to resolve, and it would just be a second
	// guaranteed-failing exec.
	if len(fe.RunShellOutputCalls) != 1 {
		t.Errorf("expected exactly 1 RunShellOutput call (the failed probe, no path lookup), got %d: %v", len(fe.RunShellOutputCalls), fe.RunShellOutputCalls)
	}
}

// --- toggleEnrollCmd (#283) ---

func TestToggleEnrollCmd_EnrollSuccess(t *testing.T) {
	fe := &action.FakeExecutor{}

	cmd := toggleEnrollCmd(fe, false) // not yet enrolled -> enroll
	msg := cmd()

	enrollMsg, ok := msg.(dispatchEnrollMsg)
	if !ok {
		t.Fatalf("toggleEnrollCmd() returned %T, want dispatchEnrollMsg", msg)
	}
	if enrollMsg.err != "" {
		t.Errorf("dispatchEnrollMsg.err = %q, want empty", enrollMsg.err)
	}

	if len(fe.RunShellOutputCalls) != 1 {
		t.Fatalf("expected 1 RunShellOutput call, got %d", len(fe.RunShellOutputCalls))
	}
	cmdStr := fe.RunShellOutputCalls[0]
	if !strings.Contains(cmdStr, "dispatch enroll --dir") {
		t.Errorf("RunShellOutput called with %q, want substring %q", cmdStr, "dispatch enroll --dir")
	}
	if strings.Contains(cmdStr, "unenroll") {
		t.Errorf("expected enroll (not unenroll) command, got %q", cmdStr)
	}
}

func TestToggleEnrollCmd_UnenrollSuccess(t *testing.T) {
	fe := &action.FakeExecutor{}

	cmd := toggleEnrollCmd(fe, true) // currently enrolled -> unenroll
	msg := cmd()

	enrollMsg, ok := msg.(dispatchEnrollMsg)
	if !ok {
		t.Fatalf("toggleEnrollCmd() returned %T, want dispatchEnrollMsg", msg)
	}
	if enrollMsg.err != "" {
		t.Errorf("dispatchEnrollMsg.err = %q, want empty", enrollMsg.err)
	}

	if len(fe.RunShellOutputCalls) != 1 {
		t.Fatalf("expected 1 RunShellOutput call, got %d", len(fe.RunShellOutputCalls))
	}
	cmdStr := fe.RunShellOutputCalls[0]
	if !strings.Contains(cmdStr, "dispatch unenroll --dir") {
		t.Errorf("RunShellOutput called with %q, want substring %q", cmdStr, "dispatch unenroll --dir")
	}
}

func TestToggleEnrollCmd_Error(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputErr:    errors.New("exit status 1"),
		RunShellOutputStderr: "fatal: not a git repository (or any of the parent directories)",
	}

	cmd := toggleEnrollCmd(fe, false)
	msg := cmd()

	enrollMsg, ok := msg.(dispatchEnrollMsg)
	if !ok {
		t.Fatalf("toggleEnrollCmd() returned %T, want dispatchEnrollMsg", msg)
	}
	if enrollMsg.err == "" {
		t.Error("expected dispatchEnrollMsg.err to be non-empty on exec error")
	}
}

func TestToggleEnrollCmd_IgnoresStdout(t *testing.T) {
	// enroll/unenroll must check exit code ONLY -- never parse stdout. A
	// non-JSON, garbage stdout with a nil error must still be a success.
	fe := &action.FakeExecutor{RunShellOutputStdout: "{not valid json at all"}

	cmd := toggleEnrollCmd(fe, false)
	msg := cmd()

	enrollMsg, ok := msg.(dispatchEnrollMsg)
	if !ok {
		t.Fatalf("toggleEnrollCmd() returned %T, want dispatchEnrollMsg", msg)
	}
	if enrollMsg.err != "" {
		t.Errorf("dispatchEnrollMsg.err = %q, want empty (enroll must ignore stdout content and check exit code only)", enrollMsg.err)
	}
}

// --- dispatchOnceCmd (#283) ---

func TestDispatchOnceCmd_ParsesCounts(t *testing.T) {
	stdout := strings.Join([]string{
		"owner/repo1#1 dispatch session-a",
		"owner/repo2#2 dispatch session-b",
		"owner/repo3#3 skip: already running",
		"some unrelated log line that matches neither pattern",
	}, "\n")
	fe := &action.FakeExecutor{RunShellOutputStdout: stdout}

	cmd := dispatchOnceCmd(fe)
	msg := cmd()

	runMsg, ok := msg.(dispatchRunMsg)
	if !ok {
		t.Fatalf("dispatchOnceCmd() returned %T, want dispatchRunMsg", msg)
	}
	if runMsg.err != "" {
		t.Fatalf("dispatchRunMsg.err = %q, want empty", runMsg.err)
	}
	if !strings.Contains(runMsg.result, "2") {
		t.Errorf("dispatchRunMsg.result = %q, want it to contain the dispatched count %q", runMsg.result, "2")
	}
	if !strings.Contains(runMsg.result, "1") {
		t.Errorf("dispatchRunMsg.result = %q, want it to contain the skipped count %q", runMsg.result, "1")
	}
	lower := strings.ToLower(runMsg.result)
	if !strings.Contains(lower, "dispatch") {
		t.Errorf("dispatchRunMsg.result = %q, want it to mention dispatched cards", runMsg.result)
	}
	if !strings.Contains(lower, "skip") {
		t.Errorf("dispatchRunMsg.result = %q, want it to mention skipped cards", runMsg.result)
	}
	if !strings.Contains(lower, "all enrolled repos") {
		t.Errorf("dispatchRunMsg.result = %q, want it to explicitly signal the fleet-wide scope (all enrolled repos)", runMsg.result)
	}

	if len(fe.RunShellOutputCalls) != 1 {
		t.Fatalf("expected 1 RunShellOutput call, got %d", len(fe.RunShellOutputCalls))
	}
	cmdStr := fe.RunShellOutputCalls[0]
	if !strings.Contains(cmdStr, "dispatch --once") {
		t.Errorf("RunShellOutput called with %q, want substring %q", cmdStr, "dispatch --once")
	}
	if strings.Contains(cmdStr, "--dir") || strings.Contains(cmdStr, "--repo") {
		t.Errorf("dispatchOnceCmd must be fleet-wide (no --dir/--repo filter), got %q", cmdStr)
	}

	wantLines := []string{
		"owner/repo1#1 dispatch session-a",
		"owner/repo2#2 dispatch session-b",
		"owner/repo3#3 skip: already running",
	}
	if len(runMsg.lines) != len(wantLines) {
		t.Fatalf("dispatchRunMsg.lines = %#v, want %d retained lines %#v", runMsg.lines, len(wantLines), wantLines)
	}
	for i, want := range wantLines {
		if runMsg.lines[i] != want {
			t.Errorf("dispatchRunMsg.lines[%d] = %q, want %q", i, runMsg.lines[i], want)
		}
	}
	for _, line := range runMsg.lines {
		if strings.Contains(line, "unrelated log line") {
			t.Errorf("dispatchRunMsg.lines = %#v, want the unrelated log line dropped", runMsg.lines)
		}
	}
}

// TestDispatchOnceCmd_RetainsBothIssueRefFormats is a regression test for the
// agentwatch output format transitioning from "#N …" to "owner/repo#N …".
// The line-matching in dispatchOnceCmd is deliberately prefix-agnostic
// (strings.Contains on " dispatch "/" skip:", not anchored on a leading "#"),
// so both the old bare "#N" form and the new "owner/repo#N" form must survive
// into dispatchRunMsg.lines without either one being dropped.
func TestDispatchOnceCmd_RetainsBothIssueRefFormats(t *testing.T) {
	stdout := strings.Join([]string{
		"#12 skip: already running",
		"owner/repo#34 skip: rate limited",
	}, "\n")
	fe := &action.FakeExecutor{RunShellOutputStdout: stdout}

	cmd := dispatchOnceCmd(fe)
	msg := cmd()

	runMsg, ok := msg.(dispatchRunMsg)
	if !ok {
		t.Fatalf("dispatchOnceCmd() returned %T, want dispatchRunMsg", msg)
	}

	wantLines := []string{
		"#12 skip: already running",
		"owner/repo#34 skip: rate limited",
	}
	if len(runMsg.lines) != len(wantLines) {
		t.Fatalf("dispatchRunMsg.lines = %#v, want %d retained lines %#v", runMsg.lines, len(wantLines), wantLines)
	}
	for i, want := range wantLines {
		if runMsg.lines[i] != want {
			t.Errorf("dispatchRunMsg.lines[%d] = %q, want %q", i, runMsg.lines[i], want)
		}
	}
}

func TestDispatchOnceCmd_Error(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputErr:    errors.New("exit status 127"),
		RunShellOutputStderr: "agentwatch: command not found",
	}

	cmd := dispatchOnceCmd(fe)
	msg := cmd()

	runMsg, ok := msg.(dispatchRunMsg)
	if !ok {
		t.Fatalf("dispatchOnceCmd() returned %T, want dispatchRunMsg", msg)
	}
	if runMsg.err == "" {
		t.Error("expected dispatchRunMsg.err to be non-empty on exec error")
	}
}

// --- viewDispatchModal (#284) ---
//
// State precedence exercised through b.View(): loading -> running -> err ->
// ready (not-enrolled vs enrolled, with an optional "Last run" summary line).

func TestDispatchView_Loading(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loading: true}

	view := b.View()

	if !strings.Contains(view, "Checking dispatch status...") {
		t.Errorf("dispatch view while loading should contain %q, got:\n%s", "Checking dispatch status...", view)
	}
}

func TestDispatchView_Running(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{running: true, repo: "owner/repo"}

	view := b.View()

	if !strings.Contains(view, "Running dispatch...") {
		t.Errorf("dispatch view while running should contain %q, got:\n%s", "Running dispatch...", view)
	}
}

func TestDispatchView_ErrorWithRepoAndDir(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	errMsg := "agentwatch not found on PATH — install it to use dispatch"
	b.dispatch = dispatchState{err: errMsg, repo: "owner/repo", dir: "/tmp/some-dir"}

	view := b.View()

	if !strings.Contains(view, errMsg) {
		t.Errorf("dispatch view with err set should contain the error message %q, got:\n%s", errMsg, view)
	}
	if !strings.Contains(view, "owner/repo") {
		t.Errorf("dispatch view with err set and repo populated should still show repo %q, got:\n%s", "owner/repo", view)
	}
	if !strings.Contains(view, "/tmp/some-dir") {
		t.Errorf("dispatch view with err set and dir populated should still show dir %q, got:\n%s", "/tmp/some-dir", view)
	}
}

func TestDispatchView_ErrorWithoutRepo(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	errMsg := "fatal: not a git repository"
	b.dispatch = dispatchState{err: errMsg, repo: "", dir: ""}

	view := b.View()

	if !strings.Contains(view, errMsg) {
		t.Errorf("dispatch view with err set (no repo) should contain the error message %q, got:\n%s", errMsg, view)
	}
	if strings.Contains(view, "\n\n\n\n") {
		t.Errorf("dispatch view with empty repo should not leave a stray blank line, got:\n%s", view)
	}
}

func TestDispatchView_ReadyNotEnrolled(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	view := b.View()

	if !strings.Contains(view, "owner/repo") {
		t.Errorf("ready dispatch view should contain repo %q, got:\n%s", "owner/repo", view)
	}
	if !strings.Contains(view, "Enroll") {
		t.Errorf("ready dispatch view for a not-enrolled repo should offer an Enroll hint, got:\n%s", view)
	}
	if strings.Contains(view, "Dispatch once") {
		t.Errorf("ready dispatch view for a not-enrolled repo should NOT offer 'Dispatch once', got:\n%s", view)
	}
}

func TestDispatchView_ReadyWithEmptyRepoDoesNotShowEnrolledFields(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{}

	view := b.View()

	if strings.Contains(view, "Enrolled:") {
		t.Errorf("dispatch view with no repo should not render enrolled/ready fields as if valid, got:\n%s", view)
	}
	if strings.Contains(view, "Dispatch once") || strings.Contains(view, "Enroll") {
		t.Errorf("dispatch view with no repo should not offer enroll/dispatch hints, got:\n%s", view)
	}
}

func TestDispatchView_ReadyEnrolled(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true}

	view := b.View()

	if !strings.Contains(view, "Unenroll") {
		t.Errorf("ready dispatch view for an enrolled repo should offer an Unenroll hint, got:\n%s", view)
	}
	if !strings.Contains(view, "Dispatch once") {
		t.Errorf("ready dispatch view for an enrolled repo should offer 'Dispatch once', got:\n%s", view)
	}
}

func TestDispatchView_ShowsLastResult(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	result := "2 dispatched, 1 skipped (all enrolled repos)"
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, lastResult: result}

	view := b.View()

	if !strings.Contains(view, result) {
		t.Errorf("ready dispatch view with a lastResult should surface the summary %q, got:\n%s", result, view)
	}
	if !strings.Contains(view, "all enrolled repos") {
		t.Errorf("dispatch view result summary should include the fleet-wide scope marker, got:\n%s", view)
	}
}

func TestDispatchView_ShowsPerIssueLines(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	result := "1 dispatched, 1 skipped (all enrolled repos)"
	lastLines := []string{
		"owner/repo1#1 dispatch session-a",
		"owner/repo2#2 skip: already running",
	}
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, lastResult: result, lastLines: lastLines}

	view := b.View()

	for _, line := range lastLines {
		if !strings.Contains(view, line) {
			t.Errorf("dispatch view with lastLines should contain %q, got:\n%s", line, view)
		}
	}
	if strings.Contains(view, "more") {
		t.Errorf("dispatch view with only 2 lastLines should not show a truncation notice, got:\n%s", view)
	}
}

func TestDispatchView_TruncatesPerIssueLinesPast8(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	result := "10 dispatched, 0 skipped (all enrolled repos)"
	var lastLines []string
	for i := 1; i <= 10; i++ {
		lastLines = append(lastLines, fmt.Sprintf("owner/repo%d#%d dispatch session-%d", i, i, i))
	}
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, lastResult: result, lastLines: lastLines}

	view := b.View()

	for _, line := range lastLines[:8] {
		if !strings.Contains(view, line) {
			t.Errorf("dispatch view should contain retained line %q, got:\n%s", line, view)
		}
	}
	for _, line := range lastLines[8:] {
		if strings.Contains(view, line) {
			t.Errorf("dispatch view should NOT contain line past the 8-line cap %q, got:\n%s", line, view)
		}
	}
	if !strings.Contains(view, "and 2 more") {
		t.Errorf("dispatch view with 10 lastLines should show a '… and 2 more' truncation notice, got:\n%s", view)
	}
}

func TestDispatchView_NoLastLinesShowsNoExtraSection(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	result := "0 dispatched, 0 skipped (all enrolled repos)"
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, lastResult: result}

	view := b.View()

	if !strings.Contains(view, result) {
		t.Errorf("dispatch view should still show the aggregate result %q, got:\n%s", result, view)
	}
	if strings.Contains(view, "more") {
		t.Errorf("dispatch view with no lastLines should not render a truncation notice, got:\n%s", view)
	}
}
