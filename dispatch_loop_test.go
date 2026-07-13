package main

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/dispatchloop"
)

// newDispatchLoopTestBoard creates a loaded Board wired to fe, with
// dispatchLoopPidPath/dispatchLoopLogPath pointed at a fresh t.TempDir() so
// tests never touch the real $XDG_STATE_HOME/lazyboards directory.
func newDispatchLoopTestBoard(t *testing.T, fe *action.FakeExecutor) Board {
	t.Helper()
	b := newDispatchTestBoardWithExecutor(t, fe)
	dir := t.TempDir()
	b.dispatchLoopPidPath = dispatchloop.PidPath(dir)
	b.dispatchLoopLogPath = dispatchloop.LogPath(dir)
	return b
}

// writeTestPid writes raw pid content to path for test setup.
func writeTestPid(t *testing.T, path string, pid int) {
	t.Helper()
	if err := dispatchloop.WritePid(path, pid); err != nil {
		t.Fatalf("WritePid setup error = %v", err)
	}
}

// --- dispatchLoopStatusCmd ---

func TestDispatchLoopStatusCmd_NoPidfile_ReportsStopped(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchLoopTestBoard(t, fe)

	msgs := collectMsgs(dispatchLoopStatusCmd(fe, b.dispatchLoopPidPath))

	var found *dispatchLoopStatusMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStatusMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStatusMsg, got %#v", msgs)
	}
	if found.err != nil {
		t.Errorf("err = %v, want nil", found.err)
	}
	if found.pid != 0 {
		t.Errorf("pid = %d, want 0 (stopped)", found.pid)
	}
}

func TestDispatchLoopStatusCmd_LivePid_ReportsRunning(t *testing.T) {
	fe := &action.FakeExecutor{ProcessAliveResult: true}
	b := newDispatchLoopTestBoard(t, fe)
	writeTestPid(t, b.dispatchLoopPidPath, 4242)

	msgs := collectMsgs(dispatchLoopStatusCmd(fe, b.dispatchLoopPidPath))

	var found *dispatchLoopStatusMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStatusMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStatusMsg, got %#v", msgs)
	}
	if found.pid != 4242 {
		t.Errorf("pid = %d, want 4242", found.pid)
	}
	if len(fe.ProcessAliveCalls) != 1 || fe.ProcessAliveCalls[0] != 4242 {
		t.Errorf("ProcessAliveCalls = %v, want [4242]", fe.ProcessAliveCalls)
	}
}

func TestDispatchLoopStatusCmd_StalePid_RemovesPidfileAndReportsStopped(t *testing.T) {
	fe := &action.FakeExecutor{ProcessAliveResult: false}
	b := newDispatchLoopTestBoard(t, fe)
	writeTestPid(t, b.dispatchLoopPidPath, 9999)

	msgs := collectMsgs(dispatchLoopStatusCmd(fe, b.dispatchLoopPidPath))

	var found *dispatchLoopStatusMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStatusMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStatusMsg, got %#v", msgs)
	}
	if found.pid != 0 {
		t.Errorf("pid = %d, want 0 (stale pid treated as stopped)", found.pid)
	}
	if found.err != nil {
		t.Errorf("err = %v, want nil", found.err)
	}

	pid, err := dispatchloop.ReadPid(b.dispatchLoopPidPath)
	if err != nil {
		t.Fatalf("ReadPid after stale cleanup error = %v, want nil", err)
	}
	if pid != 0 {
		t.Errorf("pidfile still present after stale cleanup, ReadPid = %d, want 0", pid)
	}
}

func TestDispatchLoopStatusCmd_MalformedPidfile_SurfacesError(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchLoopTestBoard(t, fe)
	// Write malformed content directly (bypassing WritePid's pid-formatting)
	// to simulate a corrupted pidfile.
	if err := os.WriteFile(b.dispatchLoopPidPath, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatalf("setup write of malformed pidfile error = %v", err)
	}

	msgs := collectMsgs(dispatchLoopStatusCmd(fe, b.dispatchLoopPidPath))

	var found *dispatchLoopStatusMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStatusMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStatusMsg, got %#v", msgs)
	}
	if found.err == nil {
		t.Error("expected err to be surfaced for a malformed pidfile, got nil")
	}
}

// --- dispatchLoopStartCmd ---

func TestDispatchLoopStartCmd_Stopped_SpawnsAndWritesPidfile(t *testing.T) {
	fe := &action.FakeExecutor{StartDetachedPid: 555}
	b := newDispatchLoopTestBoard(t, fe)

	msgs := collectMsgs(dispatchLoopStartCmd(fe, b.dispatchLoopPidPath, b.dispatchLoopLogPath))

	var found *dispatchLoopStartedMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStartedMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStartedMsg, got %#v", msgs)
	}
	if found.err != nil {
		t.Errorf("err = %v, want nil", found.err)
	}
	if found.pid != 555 {
		t.Errorf("pid = %d, want 555", found.pid)
	}
	if len(fe.StartDetachedCalls) != 1 {
		t.Fatalf("expected exactly 1 StartDetached call, got %d: %v", len(fe.StartDetachedCalls), fe.StartDetachedCalls)
	}
	if fe.StartDetachedCalls[0].Command != dispatchLoopCommand {
		t.Errorf("StartDetached command = %q, want %q", fe.StartDetachedCalls[0].Command, dispatchLoopCommand)
	}
	if fe.StartDetachedCalls[0].LogPath != b.dispatchLoopLogPath {
		t.Errorf("StartDetached logPath = %q, want %q", fe.StartDetachedCalls[0].LogPath, b.dispatchLoopLogPath)
	}

	pid, err := dispatchloop.ReadPid(b.dispatchLoopPidPath)
	if err != nil {
		t.Fatalf("ReadPid after start error = %v, want nil", err)
	}
	if pid != 555 {
		t.Errorf("pidfile pid = %d, want 555", pid)
	}
}

// TestDispatchLoopStartCmd_AlreadyAlive_SkipsDuplicateSpawn exercises the
// re-check-before-spawn guard: another lazyboards instance may have started
// the loop between the last status poll and this "s" keypress, so
// dispatchLoopStartCmd must re-read the pidfile and, if alive, return the
// existing pid without a second StartDetached call.
func TestDispatchLoopStartCmd_AlreadyAlive_SkipsDuplicateSpawn(t *testing.T) {
	fe := &action.FakeExecutor{ProcessAliveResult: true}
	b := newDispatchLoopTestBoard(t, fe)
	writeTestPid(t, b.dispatchLoopPidPath, 777)

	msgs := collectMsgs(dispatchLoopStartCmd(fe, b.dispatchLoopPidPath, b.dispatchLoopLogPath))

	var found *dispatchLoopStartedMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStartedMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStartedMsg, got %#v", msgs)
	}
	if found.pid != 777 {
		t.Errorf("pid = %d, want existing 777", found.pid)
	}
	if len(fe.StartDetachedCalls) != 0 {
		t.Errorf("expected no StartDetached calls when already alive, got %v", fe.StartDetachedCalls)
	}
}

// --- dispatchLoopStopCmd ---

func TestDispatchLoopStopCmd_Success_SignalsAndRemovesPidfile(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchLoopTestBoard(t, fe)
	writeTestPid(t, b.dispatchLoopPidPath, 333)

	msgs := collectMsgs(dispatchLoopStopCmd(fe, b.dispatchLoopPidPath, 333))

	var found *dispatchLoopStoppedMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStoppedMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStoppedMsg, got %#v", msgs)
	}
	if found.err != nil {
		t.Errorf("err = %v, want nil", found.err)
	}
	if len(fe.SignalProcessCalls) != 1 || fe.SignalProcessCalls[0] != 333 {
		t.Errorf("SignalProcessCalls = %v, want [333]", fe.SignalProcessCalls)
	}

	pid, err := dispatchloop.ReadPid(b.dispatchLoopPidPath)
	if err != nil {
		t.Fatalf("ReadPid after stop error = %v, want nil", err)
	}
	if pid != 0 {
		t.Errorf("pidfile still present after successful stop, ReadPid = %d, want 0", pid)
	}
}

func TestDispatchLoopStopCmd_SignalFails_LeavesPidfileInPlace(t *testing.T) {
	fe := &action.FakeExecutor{SignalProcessErr: errors.New("no such process")}
	b := newDispatchLoopTestBoard(t, fe)
	writeTestPid(t, b.dispatchLoopPidPath, 333)

	msgs := collectMsgs(dispatchLoopStopCmd(fe, b.dispatchLoopPidPath, 333))

	var found *dispatchLoopStoppedMsg
	for _, msg := range msgs {
		if sm, ok := msg.(dispatchLoopStoppedMsg); ok {
			found = &sm
		}
	}
	if found == nil {
		t.Fatalf("expected a dispatchLoopStoppedMsg, got %#v", msgs)
	}
	if found.err == nil {
		t.Error("expected err to be surfaced when SignalProcess fails")
	}

	pid, err := dispatchloop.ReadPid(b.dispatchLoopPidPath)
	if err != nil {
		t.Fatalf("ReadPid error = %v, want nil", err)
	}
	if pid != 333 {
		t.Errorf("pidfile removed despite failed signal; ReadPid = %d, want 333 (untouched)", pid)
	}
}

// --- 'd' handler batches loop status alongside enroll status ---

func TestDispatch_PressD_ResetsLoopCheckingAndFiresLoopStatusCmd(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{}, // agentwatch version probe succeeds
			{Stdout: `{"repo":"owner/repo","dir":"/tmp/x","enrolled":false}`},
		},
	}
	b := newDispatchLoopTestBoard(t, fe)

	m, cmd := b.Update(keyMsg("d"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.dispatch.loopChecking {
		t.Error("after pressing 'd': dispatch.loopChecking = false, want true")
	}
	if cmd == nil {
		t.Fatal("expected pressing 'd' to fire a Cmd batch, got nil")
	}

	msgs := collectMsgs(cmd)
	var foundStatus, foundLoop bool
	for _, msg := range msgs {
		switch msg.(type) {
		case dispatchStatusMsg:
			foundStatus = true
		case dispatchLoopStatusMsg:
			foundLoop = true
		}
	}
	if !foundStatus {
		t.Errorf("expected a dispatchStatusMsg among executed commands, got %#v", msgs)
	}
	if !foundLoop {
		t.Errorf("expected a dispatchLoopStatusMsg among executed commands, got %#v", msgs)
	}
}

// --- 's' key handler ---

func TestDispatch_S_FiresStartWhenStopped(t *testing.T) {
	fe := &action.FakeExecutor{StartDetachedPid: 111}
	b := newDispatchLoopTestBoard(t, fe)
	b.mode = dispatchMode
	b.dispatch.loopPid = 0

	m, cmd := b.Update(keyMsg("s"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.dispatch.loopBusy {
		t.Error("after pressing 's' while stopped: dispatch.loopBusy = false, want true")
	}
	if cmd == nil {
		t.Fatal("expected 's' to fire a Cmd when stopped, got nil")
	}

	msgs := collectMsgs(cmd)
	var found bool
	for _, msg := range msgs {
		if _, ok := msg.(dispatchLoopStartedMsg); ok {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a dispatchLoopStartedMsg among executed commands, got %#v", msgs)
	}
}

func TestDispatch_S_FiresStopWhenRunning(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchLoopTestBoard(t, fe)
	b.mode = dispatchMode
	b.dispatch.loopPid = 42

	m, cmd := b.Update(keyMsg("s"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.dispatch.loopBusy {
		t.Error("after pressing 's' while running: dispatch.loopBusy = false, want true")
	}
	if cmd == nil {
		t.Fatal("expected 's' to fire a Cmd when running, got nil")
	}

	msgs := collectMsgs(cmd)
	var found bool
	for _, msg := range msgs {
		if _, ok := msg.(dispatchLoopStoppedMsg); ok {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a dispatchLoopStoppedMsg among executed commands, got %#v", msgs)
	}
	if len(fe.SignalProcessCalls) != 1 || fe.SignalProcessCalls[0] != 42 {
		t.Errorf("SignalProcessCalls = %v, want [42]", fe.SignalProcessCalls)
	}
}

func TestDispatch_S_NoopWhileChecking(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopChecking: true}

	m, cmd := b.Update(keyMsg("s"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("expected 's' to be a no-op (nil Cmd) while loopChecking")
	}
	if b2.dispatch.loopBusy {
		t.Error("expected loopBusy to remain false when 's' is a no-op")
	}
}

func TestDispatch_S_NoopWhileBusy(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopBusy: true}

	_, cmd := b.Update(keyMsg("s"))
	if cmd != nil {
		t.Error("expected 's' to be a no-op (nil Cmd) while loopBusy")
	}
}

func TestDispatch_S_NoopWhileLoopErrSet(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopErr: "boom"}

	_, cmd := b.Update(keyMsg("s"))
	if cmd != nil {
		t.Error("expected 's' to be a no-op (nil Cmd) while loopErr is set")
	}
}

// --- Message handlers update dispatch.loop* state ---

func TestDispatch_HandleLoopStatusMsg_Success(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopChecking: true}

	m, _ := b.Update(dispatchLoopStatusMsg{pid: 99})
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loopChecking {
		t.Error("expected loopChecking=false after dispatchLoopStatusMsg")
	}
	if b2.dispatch.loopPid != 99 {
		t.Errorf("loopPid = %d, want 99", b2.dispatch.loopPid)
	}
	if b2.dispatch.loopErr != "" {
		t.Errorf("loopErr = %q, want empty", b2.dispatch.loopErr)
	}
}

func TestDispatch_HandleLoopStatusMsg_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopChecking: true}

	m, _ := b.Update(dispatchLoopStatusMsg{err: errors.New("boom")})
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loopChecking {
		t.Error("expected loopChecking=false after dispatchLoopStatusMsg with err")
	}
	if b2.dispatch.loopErr != "boom" {
		t.Errorf("loopErr = %q, want %q", b2.dispatch.loopErr, "boom")
	}
}

func TestDispatch_HandleLoopStartedMsg_Success(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopBusy: true}

	m, _ := b.Update(dispatchLoopStartedMsg{pid: 321})
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loopBusy {
		t.Error("expected loopBusy=false after dispatchLoopStartedMsg")
	}
	if b2.dispatch.loopPid != 321 {
		t.Errorf("loopPid = %d, want 321", b2.dispatch.loopPid)
	}
}

func TestDispatch_HandleLoopStartedMsg_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopBusy: true}

	m, _ := b.Update(dispatchLoopStartedMsg{err: errors.New("boom")})
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loopBusy {
		t.Error("expected loopBusy=false after dispatchLoopStartedMsg with err")
	}
	if b2.dispatch.loopErr != "boom" {
		t.Errorf("loopErr = %q, want %q", b2.dispatch.loopErr, "boom")
	}
}

func TestDispatch_HandleLoopStoppedMsg_Success(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopBusy: true, loopPid: 42}

	m, _ := b.Update(dispatchLoopStoppedMsg{})
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loopBusy {
		t.Error("expected loopBusy=false after dispatchLoopStoppedMsg")
	}
	if b2.dispatch.loopPid != 0 {
		t.Errorf("loopPid = %d, want 0", b2.dispatch.loopPid)
	}
}

func TestDispatch_HandleLoopStoppedMsg_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{loopBusy: true, loopPid: 42}

	m, _ := b.Update(dispatchLoopStoppedMsg{err: errors.New("boom")})
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.loopBusy {
		t.Error("expected loopBusy=false after dispatchLoopStoppedMsg with err")
	}
	if b2.dispatch.loopErr != "boom" {
		t.Errorf("loopErr = %q, want %q", b2.dispatch.loopErr, "boom")
	}
	if b2.dispatch.loopPid != 42 {
		t.Errorf("loopPid = %d, want unchanged 42 (stop failed)", b2.dispatch.loopPid)
	}
}

// --- viewDispatchModal loop line variants ---

func TestDispatchView_LoopChecking(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", loopChecking: true}

	view := b.View()

	if !strings.Contains(view, "Loop: checking...") {
		t.Errorf("dispatch view while loopChecking should contain %q, got:\n%s", "Loop: checking...", view)
	}
}

func TestDispatchView_LoopStopped(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", loopPid: 0}

	view := b.View()

	if !strings.Contains(view, "Loop: stopped") {
		t.Errorf("dispatch view with loopPid=0 should contain %q, got:\n%s", "Loop: stopped", view)
	}
	if !strings.Contains(view, "Start loop") {
		t.Errorf("dispatch view with loop stopped should offer 'Start loop' hint, got:\n%s", view)
	}
}

func TestDispatchView_LoopRunning(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", loopPid: 4242}

	view := b.View()

	want := "Loop: running (pid " + strconv.Itoa(4242) + ")"
	if !strings.Contains(view, want) {
		t.Errorf("dispatch view with loopPid=4242 should contain %q, got:\n%s", want, view)
	}
	if !strings.Contains(view, "Stop loop") {
		t.Errorf("dispatch view with loop running should offer 'Stop loop' hint, got:\n%s", view)
	}
}

func TestDispatchView_LoopError(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", loopErr: "signal pid 42: no such process"}

	view := b.View()

	if !strings.Contains(view, "Loop: signal pid 42: no such process") {
		t.Errorf("dispatch view with loopErr set should surface it, got:\n%s", view)
	}
}

// --- dispatchModeHints gains 's' (this ticket) ---

func TestDispatchModeHints_IncludesS(t *testing.T) {
	keys := make(map[string]bool)
	for _, h := range dispatchModeHints {
		keys[h.Key] = true
	}
	if !keys["s"] {
		t.Errorf("dispatchModeHints missing 's' entry: %+v", dispatchModeHints)
	}
}
