package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"github.com/matteobortolazzo/lazyboards/internal/sysstat"
)

// --- #677: host CPU/RAM status-bar segment wiring ---
//
// The reader (internal/sysstat) and the segment formatter (statusbar.go) have
// their own tests; this file covers what joins them to the board: the fetch
// Cmd, the tick chain, the baseline handling in Update, the single chain from
// Init(), and the system_usage toggle through config.Load -> sysReaderFor.

// Two recorded samples taken from a host whose /proc/meminfo reported the
// ticket's example figures (MemTotal 16330000 kB, MemAvailable 9900000 kB).
var (
	sysWiringBase = sysstat.Sample{Total: 1000, Idle: 800, MemTotalKiB: 16330000, MemAvailableKiB: 9900000}
	sysWiringNext = sysstat.Sample{Total: 1400, Idle: 1000, MemTotalKiB: 16330000, MemAvailableKiB: 9900000}
)

// sysWiringMemText is the memory half of the segment for the samples above,
// taken from the ticket's own worked example (AC3: "mem 6.1/15.6G").
const sysWiringMemText = "mem 6.1/15.6G"

// sysWiringCPU computes the expected CPU percent from the AC2 definition:
// round(100 x delta-busy / delta-total), busy = total - idle (idle here already
// includes iowait).
func sysWiringCPU(prev, cur sysstat.Sample) int {
	dTotal := float64(cur.Total) - float64(prev.Total)
	dBusy := dTotal - (float64(cur.Idle) - float64(prev.Idle))
	return int(math.Round(100 * dBusy / dTotal))
}

// sysWiringSegment is the full segment expected for a prev -> cur pair over the
// shared memory figures.
func sysWiringSegment(prev, cur sysstat.Sample) string {
	return fmt.Sprintf("cpu %d%% %s", sysWiringCPU(prev, cur), sysWiringMemText)
}

// sysWiringReader returns its samples in order, repeating the last one, so a
// test can observe two different readings across the real Init -> tick path.
type sysWiringReader struct {
	mu    sync.Mutex
	reads []sysstat.Sample
	next  int
}

func (r *sysWiringReader) Read() (sysstat.Sample, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.next
	if i >= len(r.reads)-1 {
		i = len(r.reads) - 1
	} else {
		r.next++
	}
	return r.reads[i], nil
}

// newSysUsageTestBoard builds a loaded Board wired to reader (nil disables the
// feature) with a 1 ms poll interval so ticks complete inside the Cmd helpers'
// timeout (docs/bubbletea-async-patterns.md).
func newSysUsageTestBoard(t *testing.T, reader sysstat.Reader) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, false)
	b.sysReader = reader
	b.sysPollInterval = time.Millisecond
	return loadFromFakeProvider(t, b, p)
}

// sysWiringSend sends msg through Update and returns the updated Board and Cmd.
func sysWiringSend(t *testing.T, b Board, msg tea.Msg) (Board, tea.Cmd) {
	t.Helper()
	m, cmd := b.Update(msg)
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated, cmd
}

// sysWiringRead sends one host-usage read through Update and returns the
// updated Board. A read never schedules further work (the tick chain is driven
// by sysUsageTickMsg alone), so a non-nil Cmd fails the test.
func sysWiringRead(t *testing.T, b Board, msg sysUsageMsg) Board {
	t.Helper()
	b, cmd := sysWiringSend(t, b, msg)
	if cmd != nil {
		t.Errorf("Update(sysUsageMsg) returned a Cmd, want nil")
	}
	return b
}

// sysWiringCounts reports how many sysUsageMsg and sysUsageTickMsg values msgs
// holds.
func sysWiringCounts(msgs []tea.Msg) (reads, ticks int) {
	for _, msg := range msgs {
		switch msg.(type) {
		case sysUsageMsg:
			reads++
		case sysUsageTickMsg:
			ticks++
		}
	}
	return reads, ticks
}

func sysWiringHasBoardFetched(msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if _, ok := msg.(boardFetchedMsg); ok {
			return true
		}
	}
	return false
}

// sysWiringShowSegment drives b through two successful reads so the segment
// is visible.
func sysWiringShowSegment(t *testing.T, b Board) Board {
	t.Helper()
	b = sysWiringRead(t, b, sysUsageMsg{sample: sysWiringBase})
	b = sysWiringRead(t, b, sysUsageMsg{sample: sysWiringNext})
	if want := sysWiringSegment(sysWiringBase, sysWiringNext); b.statusBar.sysUsage != want {
		t.Fatalf("precondition: sysUsage = %q, want %q after two reads", b.statusBar.sysUsage, want)
	}
	return b
}

// sysWiringInitAndOneTick runs b.Init(), feeds the host-usage and git-status
// results back through Update, then fires one sysUsageTickMsg and feeds its
// read back too -- the real Init -> tick -> render path.
func sysWiringInitAndOneTick(t *testing.T, b Board) Board {
	t.Helper()
	for _, msg := range collectMsgs(b.Init()) {
		switch msg.(type) {
		case sysUsageMsg, gitStatusMsg:
			var cmd tea.Cmd
			b, cmd = sysWiringSend(t, b, msg)
			if cmd != nil {
				t.Fatalf("Update(%T) returned a Cmd, want nil", msg)
			}
		}
	}
	b, cmd := sysWiringSend(t, b, sysUsageTickMsg{})
	for _, msg := range collectMsgs(cmd) {
		if read, ok := msg.(sysUsageMsg); ok {
			b = sysWiringRead(t, b, read)
		}
	}
	return b
}

// --- constants ---

func TestSysUsage_PollIntervalIsTwoSeconds(t *testing.T) {
	if sysUsagePollInterval != 2*time.Second {
		t.Errorf("sysUsagePollInterval = %v, want 2s", sysUsagePollInterval)
	}
	b := NewBoard(provider.NewFakeProvider(), nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, false)
	if b.sysPollInterval != sysUsagePollInterval {
		t.Errorf("NewBoard sysPollInterval = %v, want sysUsagePollInterval %v", b.sysPollInterval, sysUsagePollInterval)
	}
}

// --- fetchSysUsageCmd ---

func TestFetchSysUsageCmd_Success_ReturnsSysUsageMsgWithSample(t *testing.T) {
	cmd := fetchSysUsageCmd(sysstat.FakeReader{Sample: sysWiringNext})
	if cmd == nil {
		t.Fatal("fetchSysUsageCmd() returned a nil Cmd")
	}

	msg, ok := cmd().(sysUsageMsg)
	if !ok {
		t.Fatalf("fetchSysUsageCmd() produced %T, want sysUsageMsg", cmd())
	}
	if msg.err != nil {
		t.Errorf("sysUsageMsg.err = %v, want nil on a successful read", msg.err)
	}
	if msg.sample != sysWiringNext {
		t.Errorf("sysUsageMsg.sample = %+v, want %+v", msg.sample, sysWiringNext)
	}
}

func TestFetchSysUsageCmd_Error_ReturnsSysUsageMsgWithErr(t *testing.T) {
	cmd := fetchSysUsageCmd(sysstat.FakeReader{Err: errors.New("open /proc/stat: permission denied")})
	if cmd == nil {
		t.Fatal("fetchSysUsageCmd() returned a nil Cmd")
	}

	msg, ok := cmd().(sysUsageMsg)
	if !ok {
		t.Fatalf("fetchSysUsageCmd() produced %T, want sysUsageMsg", cmd())
	}
	if msg.err == nil {
		t.Error("sysUsageMsg.err = nil, want the reader's error to be propagated")
	}
}

// --- scheduleSysUsageTick / nil-reader guards ---

func TestScheduleSysUsageTick_NilReader_ReturnsNil(t *testing.T) {
	b := newSysUsageTestBoard(t, nil)

	if cmd := scheduleSysUsageTick(b); cmd != nil {
		t.Error("scheduleSysUsageTick() with no reader configured should return nil")
	}
}

func TestScheduleSysUsageTick_WithReader_FiresSysUsageTickMsg(t *testing.T) {
	b := newSysUsageTestBoard(t, sysstat.FakeReader{})

	cmd := scheduleSysUsageTick(b)
	if cmd == nil {
		t.Fatal("scheduleSysUsageTick() with a reader configured should return a non-nil tea.Cmd")
	}
	_, ticks := sysWiringCounts(collectMsgs(cmd))
	if ticks == 0 {
		t.Error("scheduleSysUsageTick() Cmd did not fire a sysUsageTickMsg")
	}
}

func TestUpdate_SysUsageTickMsg_NilReader_NoCmd(t *testing.T) {
	b := newSysUsageTestBoard(t, nil)

	_, cmd := sysWiringSend(t, b, sysUsageTickMsg{})
	if cmd != nil {
		t.Error("sysUsageTickMsg with no reader configured should not schedule any further work")
	}
}

func TestUpdate_SysUsageTickMsg_WithReader_RefetchesAndReschedules(t *testing.T) {
	b := newSysUsageTestBoard(t, sysstat.FakeReader{Sample: sysWiringNext})

	_, cmd := sysWiringSend(t, b, sysUsageTickMsg{})
	if cmd == nil {
		t.Fatal("sysUsageTickMsg with a reader configured should return a non-nil Cmd")
	}

	msgs := collectMsgs(cmd)
	var got *sysUsageMsg
	for _, msg := range msgs {
		if m, ok := msg.(sysUsageMsg); ok {
			got = &m
		}
	}
	if got == nil {
		t.Fatal("sysUsageTickMsg should trigger a re-fetch that produces a sysUsageMsg")
	}
	if got.sample != sysWiringNext {
		t.Errorf("sysUsageMsg.sample = %+v, want the reader's sample %+v", got.sample, sysWiringNext)
	}
	if _, ticks := sysWiringCounts(msgs); ticks == 0 {
		t.Error("sysUsageTickMsg should schedule the next tick (a following sysUsageTickMsg)")
	}
}

func TestView_NilReader_NeverRendersSegment(t *testing.T) {
	b := newSysUsageTestBoard(t, nil)
	b.Width = 250

	if view := ansi.Strip(b.View()); strings.Contains(view, "cpu ") {
		t.Errorf("a board with no reader rendered a system-usage segment:\n%s", view)
	}
}

// --- sysUsageMsg handling ---

func TestUpdate_SysUsageMsg_FirstSample_StaysHidden(t *testing.T) {
	b := newSysUsageTestBoard(t, sysstat.FakeReader{})
	b.Width = 250

	b, cmd := sysWiringSend(t, b, sysUsageMsg{sample: sysWiringBase})

	if cmd != nil {
		t.Error("sysUsageMsg should not schedule further work (the tick chain is driven by sysUsageTickMsg)")
	}
	if b.statusBar.sysUsage != "" {
		t.Errorf("sysUsage = %q after the first sample, want \"\" (no delta yet)", b.statusBar.sysUsage)
	}
	if view := ansi.Strip(b.View()); strings.Contains(view, "cpu") {
		t.Errorf("first sample rendered a segment (never cpu 0%%):\n%s", view)
	}
	if b.sysPrevSample == nil || *b.sysPrevSample != sysWiringBase {
		t.Errorf("sysPrevSample = %v, want the first sample stored as the baseline", b.sysPrevSample)
	}
}

func TestUpdate_SysUsageMsg_SecondSample_ShowsSegment(t *testing.T) {
	b := newSysUsageTestBoard(t, sysstat.FakeReader{})
	b.Width = 250

	b = sysWiringShowSegment(t, b)

	if view := ansi.Strip(b.View()); !strings.Contains(view, sysWiringSegment(sysWiringBase, sysWiringNext)) {
		t.Errorf("rendered view lacks the segment %q:\n%s", sysWiringSegment(sysWiringBase, sysWiringNext), view)
	}
	if b.sysPrevSample == nil || *b.sysPrevSample != sysWiringNext {
		t.Errorf("sysPrevSample = %v, want the latest sample as the new baseline", b.sysPrevSample)
	}
}

func TestUpdate_SysUsageMsg_NonAdvancingTotal_KeepsSegmentAndRebases(t *testing.T) {
	cases := []struct {
		name  string
		reset sysstat.Sample
	}{
		{"counter went backwards", sysstat.Sample{Total: 50, Idle: 20, MemTotalKiB: 16330000, MemAvailableKiB: 9900000}},
		{"total did not advance", sysWiringNext},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := sysWiringShowSegment(t, newSysUsageTestBoard(t, sysstat.FakeReader{}))
			shown := b.statusBar.sysUsage

			b = sysWiringRead(t, b, sysUsageMsg{sample: tc.reset})

			if b.statusBar.sysUsage != shown {
				t.Errorf("sysUsage = %q, want the previously rendered %q unchanged", b.statusBar.sysUsage, shown)
			}
			if b.sysPrevSample == nil || *b.sysPrevSample != tc.reset {
				t.Errorf("sysPrevSample = %v, want the new sample %+v as the baseline", b.sysPrevSample, tc.reset)
			}

			// The next read is measured against the new baseline.
			after := sysstat.Sample{
				Total: tc.reset.Total + 400, Idle: tc.reset.Idle + 100,
				MemTotalKiB: 16330000, MemAvailableKiB: 9900000,
			}
			b = sysWiringRead(t, b, sysUsageMsg{sample: after})
			if want := sysWiringSegment(tc.reset, after); b.statusBar.sysUsage != want {
				t.Errorf("sysUsage = %q, want %q computed against the rebased sample", b.statusBar.sysUsage, want)
			}
		})
	}
}

func TestUpdate_SysUsageMsg_Error_ClearsSilentlyAndRecovers(t *testing.T) {
	b := sysWiringShowSegment(t, newSysUsageTestBoard(t, sysstat.FakeReader{}))
	messageBefore := b.statusBar.message

	b, cmd := sysWiringSend(t, b, sysUsageMsg{err: errors.New("open /proc/meminfo: no such file")})

	if b.statusBar.sysUsage != "" {
		t.Errorf("sysUsage = %q after a read error, want \"\"", b.statusBar.sysUsage)
	}
	if b.statusBar.message != messageBefore {
		t.Errorf("status message = %q after a read error, want unchanged %q (no error spam)", b.statusBar.message, messageBefore)
	}
	if cmd != nil {
		t.Error("a read error should return no Cmd; the next tick is already scheduled")
	}
	if b.sysPrevSample == nil || *b.sysPrevSample != sysWiringNext {
		t.Errorf("sysPrevSample = %v, want the last good sample kept as the baseline", b.sysPrevSample)
	}

	// A later successful read shows the segment again straight away, measured
	// against the baseline kept across the error.
	recovered := sysstat.Sample{Total: 1800, Idle: 1300, MemTotalKiB: 16330000, MemAvailableKiB: 9900000}
	b = sysWiringRead(t, b, sysUsageMsg{sample: recovered})
	if want := sysWiringSegment(sysWiringNext, recovered); b.statusBar.sysUsage != want {
		t.Errorf("sysUsage = %q after recovery, want %q", b.statusBar.sysUsage, want)
	}
}

// --- Init(): a single poll chain ---

func TestInit_Normal_StartsOnePollChain(t *testing.T) {
	b := newSysUsageTestBoard(t, sysstat.FakeReader{Sample: sysWiringBase})

	msgs := collectMsgs(b.Init())

	reads, ticks := sysWiringCounts(msgs)
	if reads == 0 {
		t.Error("Init() did not start an initial system-usage read")
	}
	// Exactly one tick: two would mean two chains, refreshing the segment
	// twice per interval.
	if ticks != 1 {
		t.Errorf("Init() scheduled %d system-usage ticks, want exactly 1", ticks)
	}
	if !sysWiringHasBoardFetched(msgs) {
		t.Error("Init() lost the startup board fetch when batching the poll")
	}
}

func TestInit_Normal_NilReader_NoPollNoSegment(t *testing.T) {
	b := newSysUsageTestBoard(t, nil)

	msgs := collectMsgs(b.Init())

	if reads, ticks := sysWiringCounts(msgs); reads != 0 || ticks != 0 {
		t.Errorf("Init() with no reader produced %d reads and %d ticks, want none", reads, ticks)
	}
	if !sysWiringHasBoardFetched(msgs) {
		t.Error("Init() with no reader should still start the board fetch")
	}
}

func TestInit_FirstLaunch_StartsPollWithoutBoardFetch(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, false)
	b.sysReader = sysstat.FakeReader{Sample: sysWiringBase}
	b.sysPollInterval = time.Millisecond

	msgs := collectMsgs(b.Init())

	reads, ticks := sysWiringCounts(msgs)
	if reads == 0 || ticks != 1 {
		t.Errorf("firstLaunch Init() produced %d reads and %d ticks, want a read and exactly 1 tick", reads, ticks)
	}
	if sysWiringHasBoardFetched(msgs) {
		t.Error("firstLaunch Init() must not start a board fetch while the config modal is up")
	}
}

func TestInit_FirstLaunch_NilReader_ReturnsNil(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, false)

	if cmd := b.Init(); cmd != nil {
		t.Error("firstLaunch Init() with no reader should return a nil Cmd")
	}
}

func TestInit_TrustConfirmMode_StartsPollWithoutBoardFetch(t *testing.T) {
	identity := "sysusage-init-trust"
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
	b := newTrustConfirmBoard(t, fx, identity)
	b.sysReader = sysstat.FakeReader{Sample: sysWiringBase}
	b.sysPollInterval = time.Millisecond

	msgs := collectMsgs(b.Init())

	reads, ticks := sysWiringCounts(msgs)
	if reads == 0 || ticks != 1 {
		t.Errorf("trustConfirmMode Init() produced %d reads and %d ticks, want a read and exactly 1 tick", reads, ticks)
	}
	if sysWiringHasBoardFetched(msgs) {
		t.Error("trustConfirmMode Init() must not start a board fetch while the prompt is up")
	}
}

func TestInit_TrustConfirmMode_NilReader_ReturnsNil(t *testing.T) {
	identity := "sysusage-init-trust-nil"
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
	b := newTrustConfirmBoard(t, fx, identity)

	if cmd := b.Init(); cmd != nil {
		t.Error("trustConfirmMode Init() with no reader should return a nil Cmd")
	}
}

// After the trust prompt is resolved, startupCmds() runs again. It must not
// start a second tick chain: the poll lives in Init() only.
func TestTrustConfirmMode_Skip_DoesNotStartSecondPollChain(t *testing.T) {
	identity := "sysusage-trust-skip"
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
	b := newTrustConfirmBoard(t, fx, identity)
	b.sysReader = sysstat.FakeReader{Sample: sysWiringBase}
	b.sysPollInterval = time.Millisecond

	_, cmd := sysWiringSend(t, b, keyMsg("s"))

	msgs := runCmd(cmd)
	if !sysWiringHasBoardFetched(msgs) {
		t.Fatalf("precondition: skip's Cmd produced no boardFetchedMsg, so the collected messages prove nothing; got %#v", msgs)
	}
	if reads, ticks := sysWiringCounts(msgs); reads != 0 || ticks != 0 {
		t.Errorf("skip's Cmd produced %d system-usage reads and %d ticks, want none (the chain Init() started continues)", reads, ticks)
	}
}

func TestTrustConfirmMode_Accept_DoesNotStartSecondPollChain(t *testing.T) {
	identity := "sysusage-trust-accept"
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
	b := newTrustConfirmBoard(t, fx, identity)
	b.sysReader = sysstat.FakeReader{Sample: sysWiringBase}
	b.sysPollInterval = time.Millisecond

	b, cmd := sysWiringSend(t, b, keyMsg("t"))
	var accepted trustAcceptedMsg
	found := false
	for _, msg := range runCmd(cmd) {
		if a, ok := msg.(trustAcceptedMsg); ok {
			accepted = a
			found = true
		}
	}
	if !found {
		t.Fatal("acceptTrustCmd did not produce trustAcceptedMsg")
	}

	b, cmd = sysWiringSend(t, b, accepted)

	msgs := runCmd(cmd)
	if !sysWiringHasBoardFetched(msgs) {
		t.Fatalf("precondition: accept's Cmd produced no boardFetchedMsg, so the collected messages prove nothing; got %#v", msgs)
	}
	if reads, ticks := sysWiringCounts(msgs); reads != 0 || ticks != 0 {
		t.Errorf("accept's Cmd produced %d system-usage reads and %d ticks, want none", reads, ticks)
	}
	if b.sysReader == nil {
		t.Error("sysReader was dropped by the trust accept reload, want the startup reader kept")
	}
}

// --- repo switch ---

func TestConfigSaved_RepoSwitch_KeepsSysUsageSegment(t *testing.T) {
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	b.sysReader = sysstat.FakeReader{}
	b = sysWiringShowSegment(t, b)
	shown := b.statusBar.sysUsage

	b, cmd := sysWiringSend(t, b, configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	if cmd == nil {
		t.Fatal("a repo switch should return a Cmd that refetches the new board")
	}

	if b.repoOwner != "new-owner" || b.repoName != "new-repo" {
		t.Fatalf("precondition: board did not retarget (repoOwner/repoName = %q/%q)", b.repoOwner, b.repoName)
	}
	if b.statusBar.sysUsage != shown {
		t.Errorf("sysUsage = %q after a repo switch, want %q kept (host usage is not repo-scoped)", b.statusBar.sysUsage, shown)
	}
}

// --- system_usage config -> sysReaderFor -> Init (AC5) ---

// loadSysUsageConfig writes the given global and local YAML (local "" means no
// local file) and loads them through the real config.Load.
func loadSysUsageConfig(t *testing.T, globalYAML, localYAML string) config.Config {
	t.Helper()
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0600); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}
	localPath := filepath.Join(dir, "local.yml")
	trust := config.Trust{}
	if localYAML != "" {
		if err := os.WriteFile(localPath, []byte(localYAML), 0600); err != nil {
			t.Fatalf("failed to write local config: %v", err)
		}
		trust = trustingLocal(t, localPath)
	}
	cfg, err := config.Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}
	return cfg
}

func TestSystemUsageConfig_ThroughSysReaderForAndInit(t *testing.T) {
	cases := []struct {
		name       string
		globalYAML string
		localYAML  string
		wantOn     bool
	}{
		{"unset defaults on", "provider: github\n", "", true},
		{"global false", "provider: github\nsystem_usage: false\n", "", false},
		{"local false overrides global true", "provider: github\nsystem_usage: true\n", "system_usage: false\n", false},
		{"local true overrides global false", "provider: github\nsystem_usage: false\n", "system_usage: true\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := loadSysUsageConfig(t, tc.globalYAML, tc.localYAML)
			factoryCalled := false
			newReader := func() sysstat.Reader {
				factoryCalled = true
				return &sysWiringReader{reads: []sysstat.Sample{sysWiringBase, sysWiringNext}}
			}

			// Two readers: the sequenced reader is stateful, so the Init() Cmd
			// census and the Init-plus-one-tick render each need their own.
			reader := sysReaderFor(cfg, newReader)
			reads, ticks := sysWiringCounts(collectMsgs(newSysUsageTestBoard(t, reader).Init()))

			b := newSysUsageTestBoard(t, sysReaderFor(cfg, newReader))
			b.Width = 250
			b = sysWiringInitAndOneTick(t, b)
			view := ansi.Strip(b.View())

			if tc.wantOn {
				if reader == nil || !factoryCalled {
					t.Fatal("system_usage enabled: sysReaderFor should return the factory's reader")
				}
				if reads == 0 || ticks != 1 {
					t.Errorf("Init() produced %d reads and %d ticks, want a read and exactly 1 tick", reads, ticks)
				}
				if !strings.Contains(view, "cpu ") {
					t.Errorf("system_usage enabled: rendered bar lacks the segment after two reads:\n%s", view)
				}
				return
			}
			if reader != nil {
				t.Error("system_usage disabled: sysReaderFor returned a non-nil reader")
			}
			if factoryCalled {
				t.Error("system_usage disabled: sysReaderFor should not construct a reader")
			}
			if reads != 0 || ticks != 0 {
				t.Errorf("system_usage disabled: Init() produced %d reads and %d ticks, want none", reads, ticks)
			}
			if strings.Contains(view, "cpu ") {
				t.Errorf("system_usage disabled: rendered bar contains the segment:\n%s", view)
			}
		})
	}
}

// On a platform with no reader (sysstat.NewReader returns a nil interface),
// the enabled toggle still yields no reader -- not a typed-nil that looks set.
func TestSysReaderFor_FactoryReturnsNil_YieldsNilReader(t *testing.T) {
	cfg := loadSysUsageConfig(t, "provider: github\n", "")

	reader := sysReaderFor(cfg, func() sysstat.Reader { return nil })

	if reader != nil {
		t.Errorf("sysReaderFor() = %#v, want a nil interface when the platform has no reader", reader)
	}
	b := newSysUsageTestBoard(t, reader)
	if cmd := scheduleSysUsageTick(b); cmd != nil {
		t.Error("a board with no platform reader should schedule no tick")
	}
}

// --- end to end ---

// Init + one tick through a sequenced reader (two different samples) renders
// the segment as the rightmost token, after the git segment.
func TestSysUsage_EndToEnd_InitAndOneTick_RendersRightmostAfterGit(t *testing.T) {
	status := gitdetect.Status{Branch: "feature-x", HasUpstream: false}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, gitdetect.FakeReader{Status: status}, false)
	b.sysReader = &sysWiringReader{reads: []sysstat.Sample{sysWiringBase, sysWiringNext}}
	b.sysPollInterval = time.Millisecond
	b = loadFromFakeProvider(t, b, p)
	b.Width = 250

	b = sysWiringInitAndOneTick(t, b)

	segment := sysWiringSegment(sysWiringBase, sysWiringNext)
	gitSegment := formatGitSegment(status)
	var line string
	for _, l := range strings.Split(ansi.Strip(b.View()), "\n") {
		if strings.Contains(l, segment) {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no rendered line contains the segment %q:\n%s", segment, ansi.Strip(b.View()))
	}
	gitAt := strings.Index(line, gitSegment)
	sysAt := strings.Index(line, segment)
	if gitAt < 0 {
		t.Fatalf("status bar line lacks the git segment %q: %q", gitSegment, line)
	}
	if gitAt >= sysAt {
		t.Errorf("git segment at %d, system-usage segment at %d: want the system-usage segment right of git in %q", gitAt, sysAt, line)
	}
	if rest := strings.Trim(line[sysAt+len(segment):], " │"); rest != "" {
		t.Errorf("text %q follows the system-usage segment, want it to be the rightmost token in %q", rest, line)
	}
}
