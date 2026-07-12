package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newGitStatusTestBoard creates a loaded Board wired to the given git reader
// (nil disables the git status feature, mirroring the "no repo/remote" gate).
func newGitStatusTestBoard(t *testing.T, reader gitdetect.Reader) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, reader)
	return loadFromFakeProvider(t, b, p)
}

// --- fetchGitStatusCmd ---

func TestFetchGitStatusCmd_Success_ReturnsGitStatusMsgWithStatus(t *testing.T) {
	want := gitdetect.Status{Branch: "main", Staged: 2, Unstaged: 1, Ahead: 3, Behind: 0, HasUpstream: true}
	reader := gitdetect.FakeReader{Status: want}

	cmd := fetchGitStatusCmd(reader, ".")
	msg := cmd()

	gsMsg, ok := msg.(gitStatusMsg)
	if !ok {
		t.Fatalf("fetchGitStatusCmd() returned %T, want gitStatusMsg", msg)
	}
	if gsMsg.err != nil {
		t.Errorf("gitStatusMsg.err = %v, want nil on a successful read", gsMsg.err)
	}
	if gsMsg.status != want {
		t.Errorf("gitStatusMsg.status = %+v, want %+v", gsMsg.status, want)
	}
}

func TestFetchGitStatusCmd_Error_ReturnsGitStatusMsgWithErr(t *testing.T) {
	wantErr := errors.New("not a git repository")
	reader := gitdetect.FakeReader{Err: wantErr}

	cmd := fetchGitStatusCmd(reader, ".")
	msg := cmd()

	gsMsg, ok := msg.(gitStatusMsg)
	if !ok {
		t.Fatalf("fetchGitStatusCmd() returned %T, want gitStatusMsg", msg)
	}
	if gsMsg.err == nil {
		t.Error("gitStatusMsg.err = nil, want the reader's error to be propagated")
	}
}

// --- scheduleGitStatusTick ---

func TestScheduleGitStatusTick_NilReader_ReturnsNil(t *testing.T) {
	b := newGitStatusTestBoard(t, nil)

	if cmd := scheduleGitStatusTick(b); cmd != nil {
		t.Error("scheduleGitStatusTick() with no git reader configured should return nil")
	}
}

func TestScheduleGitStatusTick_WithReader_ReturnsNonNilCmd(t *testing.T) {
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{})

	if cmd := scheduleGitStatusTick(b); cmd == nil {
		t.Error("scheduleGitStatusTick() with a git reader configured should return a non-nil tea.Cmd")
	}
}

// --- update.go: gitStatusMsg / gitStatusTickMsg handling ---

func TestUpdate_GitStatusMsg_Success_SetsGitStatusSegment(t *testing.T) {
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{})
	status := gitdetect.Status{Branch: "main", Staged: 1, Unstaged: 0, HasUpstream: false}

	m, _ := b.Update(gitStatusMsg{status: status})
	updated := m.(Board)

	if updated.statusBar.gitStatus == "" {
		t.Fatal("gitStatusMsg with a successful read should set a non-empty git status segment")
	}
	if !strings.Contains(updated.statusBar.gitStatus, "main") {
		t.Errorf("gitStatus = %q, want it to contain the branch name %q", updated.statusBar.gitStatus, "main")
	}
}

func TestUpdate_GitStatusMsg_Error_ClearsGitStatusSegment(t *testing.T) {
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{})
	b.statusBar.SetGitStatus("main +1~0")

	m, _ := b.Update(gitStatusMsg{err: errors.New("not a git repository")})
	updated := m.(Board)

	if updated.statusBar.gitStatus != "" {
		t.Errorf("gitStatus = %q, want empty after a failed read (segment hidden, no error spam)", updated.statusBar.gitStatus)
	}
}

func TestUpdate_GitStatusTickMsg_NilReader_NoCmd(t *testing.T) {
	b := newGitStatusTestBoard(t, nil)

	_, cmd := b.Update(gitStatusTickMsg{})
	if cmd != nil {
		t.Error("gitStatusTickMsg with no git reader configured should not schedule any further work")
	}
}

func TestUpdate_GitStatusTickMsg_WithReader_RefetchesAndReschedules(t *testing.T) {
	want := gitdetect.Status{Branch: "feature", HasUpstream: false}
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{Status: want})

	_, cmd := b.Update(gitStatusTickMsg{})
	if cmd == nil {
		t.Fatal("gitStatusTickMsg with a git reader configured should return a non-nil cmd")
	}

	msgs := collectMsgs(cmd)
	found := false
	for _, msg := range msgs {
		if gsMsg, ok := msg.(gitStatusMsg); ok {
			found = true
			if gsMsg.status != want {
				t.Errorf("gitStatusMsg.status = %+v, want %+v", gsMsg.status, want)
			}
		}
	}
	if !found {
		t.Error("gitStatusTickMsg should trigger a re-fetch that produces a gitStatusMsg")
	}
}

// --- update.go: broad refresh hooks (actionResultMsg, handleBoardFetched) ---

func TestUpdate_ActionResultMsg_Success_RefreshesGitStatus(t *testing.T) {
	want := gitdetect.Status{Branch: "main", HasUpstream: false}
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{Status: want})

	_, cmd := b.Update(actionResultMsg{success: true, message: "Pushed"})
	if cmd == nil {
		t.Fatal("actionResultMsg{success:true} should return a non-nil cmd")
	}

	msgs := collectMsgs(cmd)
	found := false
	for _, msg := range msgs {
		if _, ok := msg.(gitStatusMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("a successful action should trigger a git status refresh (broad refresh, every action, per plan Q2)")
	}
}

func TestUpdate_ActionResultMsg_Failure_DoesNotRefreshGitStatus(t *testing.T) {
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{})

	_, cmd := b.Update(actionResultMsg{success: false, message: "Failed"})
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(gitStatusMsg); ok {
			t.Error("a failed action should NOT trigger a git status refresh")
		}
	}
}

func TestHandleBoardFetched_Refreshing_TriggersGitStatusRefresh(t *testing.T) {
	b := newGitStatusTestBoard(t, gitdetect.FakeReader{Status: gitdetect.Status{Branch: "main"}})
	b.refreshing = true
	b.Width = 120
	b.Height = 40

	board, err := provider.NewFakeProvider().FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	_, cmd := b.Update(boardFetchedMsg{board: board})

	found := false
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(gitStatusMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("board refresh completion (refreshing=true path) should trigger a git status refresh")
	}
}

func TestHandleBoardFetched_InitialLoad_TriggersGitStatusRefresh(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, gitdetect.FakeReader{Status: gitdetect.Status{Branch: "main"}})

	board, err := p.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	_, cmd := b.Update(boardFetchedMsg{board: board})

	found := false
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(gitStatusMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("initial board load (non-refreshing path) should trigger a git status refresh")
	}
}
