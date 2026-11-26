package sysstat

import (
	"os"
	"path/filepath"
	"testing"
)

// Recorded from a real Linux host: the aggregate `cpu ` line carries ten
// fields (user nice system idle iowait irq softirq steal guest guest_nice) and
// is followed by per-core lines and unrelated counters.
const recordedStat = `cpu  10132153 290696 3084719 46828483 16683 0 25195 0 175628 0
cpu0 1393280 32966 572056 13343292 6130 0 17875 0 23933 0
cpu1 1380064 31504 551812 13321094 4281 0 3517 0 20847 0
intr 1462898
ctxt 3138457
btime 1700000000
processes 44128
procs_running 2
procs_blocked 0
`

// Values of the first cpu line of recordedStat, in kernel field order.
const (
	recUser    uint64 = 10132153
	recNice    uint64 = 290696
	recSystem  uint64 = 3084719
	recIdle    uint64 = 46828483
	recIowait  uint64 = 16683
	recIrq     uint64 = 0
	recSoftirq uint64 = 25195
	recSteal   uint64 = 0
)

const recordedMeminfo = `MemTotal:       16330000 kB
MemFree:         1234567 kB
MemAvailable:    9900000 kB
Buffers:          345678 kB
Cached:          4567890 kB
SwapCached:            0 kB
`

const (
	recMemTotalKiB     uint64 = 16330000
	recMemAvailableKiB uint64 = 9900000
)

// writeProcRoot writes a /proc-shaped directory under t.TempDir(). An empty
// content string leaves the corresponding file absent.
func writeProcRoot(t *testing.T, stat, meminfo string) string {
	t.Helper()
	root := t.TempDir()
	if stat != "" {
		if err := os.WriteFile(filepath.Join(root, "stat"), []byte(stat), 0o600); err != nil {
			t.Fatalf("write stat fixture: %v", err)
		}
	}
	if meminfo != "" {
		if err := os.WriteFile(filepath.Join(root, "meminfo"), []byte(meminfo), 0o600); err != nil {
			t.Fatalf("write meminfo fixture: %v", err)
		}
	}
	return root
}

func TestProcReader_ReadsRecordedProcFiles(t *testing.T) {
	root := writeProcRoot(t, recordedStat, recordedMeminfo)

	got, err := procReader{root: root}.Read()
	if err != nil {
		t.Fatalf("Read error = %v, want nil", err)
	}

	// AC2: total is the sum of the first eight cpu fields; the trailing guest
	// and guest_nice fields are already counted inside user/nice and must not
	// be added again.
	wantTotal := recUser + recNice + recSystem + recIdle + recIowait + recIrq + recSoftirq + recSteal
	wantIdle := recIdle + recIowait
	if got.Total != wantTotal {
		t.Errorf("Total = %d, want %d", got.Total, wantTotal)
	}
	if got.Idle != wantIdle {
		t.Errorf("Idle = %d, want %d (idle + iowait)", got.Idle, wantIdle)
	}
	if got.MemTotalKiB != recMemTotalKiB {
		t.Errorf("MemTotalKiB = %d, want %d", got.MemTotalKiB, recMemTotalKiB)
	}
	if got.MemAvailableKiB != recMemAvailableKiB {
		t.Errorf("MemAvailableKiB = %d, want %d", got.MemAvailableKiB, recMemAvailableKiB)
	}
}

func TestProcReader_ExactlyEightCPUFieldsIsEnough(t *testing.T) {
	// Older kernels report only the first eight fields.
	stat := "cpu  100 20 30 400 50 6 7 8\ncpu0 100 20 30 400 50 6 7 8\n"
	root := writeProcRoot(t, stat, recordedMeminfo)

	got, err := procReader{root: root}.Read()
	if err != nil {
		t.Fatalf("Read error = %v, want nil for an eight-field cpu line", err)
	}

	const user, nice, system, idle, iowait, irq, softirq, steal uint64 = 100, 20, 30, 400, 50, 6, 7, 8
	if want := user + nice + system + idle + iowait + irq + softirq + steal; got.Total != want {
		t.Errorf("Total = %d, want %d", got.Total, want)
	}
	if want := idle + iowait; got.Idle != want {
		t.Errorf("Idle = %d, want %d", got.Idle, want)
	}
}

func TestProcReader_StatErrors(t *testing.T) {
	tests := []struct {
		name string
		stat string
	}{
		{name: "missing stat file", stat: ""},
		{name: "truncated cpu line with seven fields", stat: "cpu  1 2 3 4 5 6 7\ncpu0 1 2 3 4 5 6 7\n"},
		{name: "cpu line with no fields", stat: "cpu\nintr 1\n"},
		{name: "non-numeric field in the first eight", stat: "cpu  1 2 three 4 5 6 7 8 9 10\n"},
		{name: "negative field", stat: "cpu  1 2 -3 4 5 6 7 8\n"},
		{name: "no aggregate cpu line, only per-core lines", stat: "cpu0 1 2 3 4 5 6 7 8\ncpu1 1 2 3 4 5 6 7 8\n"},
		{name: "no cpu line at all", stat: "intr 1\nctxt 2\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeProcRoot(t, tt.stat, recordedMeminfo)

			if got, err := (procReader{root: root}).Read(); err == nil {
				t.Errorf("Read error = nil (sample %+v), want an error", got)
			}
		})
	}
}

func TestProcReader_MeminfoErrors(t *testing.T) {
	tests := []struct {
		name    string
		meminfo string
	}{
		{name: "missing meminfo file", meminfo: ""},
		{name: "missing MemAvailable", meminfo: "MemTotal:       16330000 kB\nMemFree:         1234567 kB\n"},
		{name: "missing MemTotal", meminfo: "MemFree:         1234567 kB\nMemAvailable:    9900000 kB\n"},
		{name: "non-numeric MemAvailable", meminfo: "MemTotal:       16330000 kB\nMemAvailable:    lots kB\n"},
		{name: "non-numeric MemTotal", meminfo: "MemTotal:       many kB\nMemAvailable:    9900000 kB\n"},
		{name: "MemAvailable without a value", meminfo: "MemTotal:       16330000 kB\nMemAvailable:\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeProcRoot(t, recordedStat, tt.meminfo)

			if got, err := (procReader{root: root}).Read(); err == nil {
				t.Errorf("Read error = nil (sample %+v), want an error", got)
			}
		})
	}
}

func TestProcReader_MemAvailableAboveMemTotalIsNotAnError(t *testing.T) {
	meminfo := "MemTotal:       1000 kB\nMemAvailable:    1500 kB\n"
	root := writeProcRoot(t, recordedStat, meminfo)

	got, err := procReader{root: root}.Read()
	if err != nil {
		t.Fatalf("Read error = %v, want nil (clamping is NewUsage's job)", err)
	}
	if got.MemTotalKiB != 1000 || got.MemAvailableKiB != 1500 {
		t.Errorf("mem = %d/%d, want the raw values 1000/1500", got.MemTotalKiB, got.MemAvailableKiB)
	}
}

func TestProcReader_RealSamplesFeedUsage(t *testing.T) {
	prevRoot := writeProcRoot(t, recordedStat, recordedMeminfo)
	laterStat := "cpu  10132253 290696 3084719 46828683 16683 0 25195 0 175628 0\n"
	curRoot := writeProcRoot(t, laterStat, recordedMeminfo)

	prev, err := procReader{root: prevRoot}.Read()
	if err != nil {
		t.Fatalf("Read prev error = %v", err)
	}
	cur, err := procReader{root: curRoot}.Read()
	if err != nil {
		t.Fatalf("Read cur error = %v", err)
	}

	// Between the two recordings user advanced 100 and idle 200: busy is 100 of
	// a 300 total delta, so one third rounds to 33.
	pct, ok := CPUPercent(prev, cur)
	if !ok {
		t.Fatalf("CPUPercent ok = false, want true")
	}
	if want := 33; pct != want {
		t.Errorf("CPUPercent = %d, want %d", pct, want)
	}

	usage := NewUsage(pct, cur)
	if want := recMemTotalKiB - recMemAvailableKiB; usage.MemUsedKiB != want {
		t.Errorf("MemUsedKiB = %d, want %d", usage.MemUsedKiB, want)
	}
}

func TestNewReader_ReturnsAReaderOnLinux(t *testing.T) {
	if NewReader() == nil {
		t.Error("NewReader() = nil, want a /proc-backed Reader on linux")
	}
}
