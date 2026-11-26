package sysstat

import (
	"errors"
	"fmt"
	"math"
	"testing"
)

// cpuFields are the first eight numeric fields of the aggregate `cpu ` line in
// /proc/stat, in kernel order.
type cpuFields struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

// total is the AC2 definition: the sum of all eight fields.
func (f cpuFields) total() uint64 {
	return f.user + f.nice + f.system + f.idle + f.iowait + f.irq + f.softirq + f.steal
}

// idleTime is idle plus iowait.
func (f cpuFields) idleTime() uint64 { return f.idle + f.iowait }

// busy is the AC2 definition: total - idle - iowait.
func (f cpuFields) busy() uint64 { return f.total() - f.idleTime() }

// sample encodes the fields the way a reader reports them: Total and
// Idle (idle+iowait).
func (f cpuFields) sample() Sample {
	return Sample{Total: f.total(), Idle: f.idleTime()}
}

// wantPercent computes the expected CPU percent straight from the AC2 formula:
// round(100 x delta-busy / delta-total), clamped to 0..100.
func wantPercent(prev, cur cpuFields) int {
	dBusy := float64(cur.busy()) - float64(prev.busy())
	dTotal := float64(cur.total()) - float64(prev.total())
	pct := math.Round(100 * dBusy / dTotal)
	return int(math.Max(0, math.Min(100, pct)))
}

func TestCPUPercent_MatchesAC2Formula(t *testing.T) {
	base := cpuFields{user: 1000, nice: 10, system: 500, idle: 8000, iowait: 200, irq: 5, softirq: 15, steal: 3}

	tests := []struct {
		name string
		cur  cpuFields
	}{
		{
			name: "mostly busy",
			cur:  cpuFields{user: 1800, nice: 10, system: 700, idle: 8100, iowait: 210, irq: 5, softirq: 25, steal: 3},
		},
		{
			name: "mostly idle",
			cur:  cpuFields{user: 1010, nice: 10, system: 502, idle: 8990, iowait: 200, irq: 5, softirq: 15, steal: 3},
		},
		{
			name: "iowait counts as idle",
			cur:  cpuFields{user: 1000, nice: 10, system: 500, idle: 8000, iowait: 1200, irq: 5, softirq: 15, steal: 3},
		},
		{
			name: "irq softirq steal and nice count as busy",
			cur:  cpuFields{user: 1000, nice: 110, system: 500, idle: 8000, iowait: 200, irq: 55, softirq: 65, steal: 53},
		},
		{
			name: "one third rounds down",
			cur:  cpuFields{user: 1000 + 100, nice: 10, system: 500, idle: 8000 + 200, iowait: 200, irq: 5, softirq: 15, steal: 3},
		},
		{
			name: "two thirds rounds up",
			cur:  cpuFields{user: 1000 + 200, nice: 10, system: 500, idle: 8000 + 100, iowait: 200, irq: 5, softirq: 15, steal: 3},
		},
		{
			name: "fully busy",
			cur:  cpuFields{user: 2000, nice: 10, system: 500, idle: 8000, iowait: 200, irq: 5, softirq: 15, steal: 3},
		},
		{
			name: "fully idle",
			cur:  cpuFields{user: 1000, nice: 10, system: 500, idle: 9000, iowait: 200, irq: 5, softirq: 15, steal: 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CPUPercent(base.sample(), tt.cur.sample())
			if !ok {
				t.Fatalf("CPUPercent ok = false, want true for delta-total > 0")
			}
			if want := wantPercent(base, tt.cur); got != want {
				t.Errorf("CPUPercent = %d, want %d", got, want)
			}
		})
	}
}

func TestCPUPercent_ClampsSkewedCounters(t *testing.T) {
	prev := cpuFields{user: 1000, idle: 1000}

	// Idle counter went backwards while total moved forward: busy delta exceeds
	// the total delta, so the raw percentage is above 100.
	above := Sample{Total: prev.total() + 100, Idle: prev.idleTime() - 50}
	// Idle advanced by more than the total did: busy delta is negative.
	below := Sample{Total: prev.total() + 100, Idle: prev.idleTime() + 150}

	tests := []struct {
		name string
		cur  Sample
		want int
	}{
		{name: "above 100 clamps to 100", cur: above, want: 100},
		{name: "below 0 clamps to 0", cur: below, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CPUPercent(prev.sample(), tt.cur)
			if !ok {
				t.Fatalf("CPUPercent ok = false, want true for delta-total > 0")
			}
			if got != tt.want {
				t.Errorf("CPUPercent = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCPUPercent_NonPositiveTotalDeltaIsNotComputable(t *testing.T) {
	prev := Sample{Total: 50_000, Idle: 40_000}

	tests := []struct {
		name string
		cur  Sample
	}{
		{name: "identical samples", cur: prev},
		{name: "total went backwards after a counter reset", cur: Sample{Total: 100, Idle: 80}},
		{name: "total went backwards by one", cur: Sample{Total: prev.Total - 1, Idle: prev.Idle}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CPUPercent(prev, tt.cur)
			if ok {
				t.Errorf("CPUPercent ok = true (percent %d), want false for delta-total <= 0", got)
			}
		})
	}
}

func TestCPUPercent_LargeCountersDoNotOverflow(t *testing.T) {
	// Counters near the top of the uint64 range: the delta must be computed in
	// signed space, so a decrease is detected rather than wrapping to a huge
	// positive number.
	const near = math.MaxUint64 - 10
	prev := Sample{Total: near, Idle: near / 2}
	cur := Sample{Total: 5, Idle: 2}

	if got, ok := CPUPercent(prev, cur); ok {
		t.Errorf("CPUPercent ok = true (percent %d), want false when total wrapped below the previous sample", got)
	}
}

func TestNewUsage_MemoryUsedIsTotalMinusAvailable(t *testing.T) {
	const (
		memTotalKiB     uint64 = 16330000
		memAvailableKiB uint64 = 9900000
		cpu                    = 12
	)
	cur := Sample{MemTotalKiB: memTotalKiB, MemAvailableKiB: memAvailableKiB}

	got := NewUsage(cpu, cur)

	if got.CPUPercent != cpu {
		t.Errorf("CPUPercent = %d, want %d", got.CPUPercent, cpu)
	}
	if want := memTotalKiB - memAvailableKiB; got.MemUsedKiB != want {
		t.Errorf("MemUsedKiB = %d, want %d (MemTotal - MemAvailable)", got.MemUsedKiB, want)
	}
	if got.MemTotalKiB != memTotalKiB {
		t.Errorf("MemTotalKiB = %d, want %d", got.MemTotalKiB, memTotalKiB)
	}
}

func TestNewUsage_AC3ExampleConvertsToGiB(t *testing.T) {
	const kibPerGiB = 1048576.0
	cur := Sample{MemTotalKiB: 16330000, MemAvailableKiB: 9900000}

	got := NewUsage(0, cur)

	used := float64(got.MemUsedKiB) / kibPerGiB
	total := float64(got.MemTotalKiB) / kibPerGiB
	if gotRender, want := fmt.Sprintf("%.1f/%.1f", used, total), "6.1/15.6"; gotRender != want {
		t.Errorf("used/total in GiB = %q, want %q", gotRender, want)
	}
}

func TestNewUsage_AvailableAboveTotalClampsUsedToZero(t *testing.T) {
	cur := Sample{MemTotalKiB: 1000, MemAvailableKiB: 1500}

	got := NewUsage(40, cur)

	if got.MemUsedKiB != 0 {
		t.Errorf("MemUsedKiB = %d, want 0 when MemAvailable exceeds MemTotal", got.MemUsedKiB)
	}
	if got.MemTotalKiB != cur.MemTotalKiB {
		t.Errorf("MemTotalKiB = %d, want %d", got.MemTotalKiB, cur.MemTotalKiB)
	}
}

func TestFakeReader_ReturnsConfiguredSampleAndError(t *testing.T) {
	want := Sample{Total: 10, Idle: 4, MemTotalKiB: 2048, MemAvailableKiB: 1024}
	readErr := errors.New("boom")

	var r Reader = FakeReader{Sample: want}
	got, err := r.Read()
	if err != nil {
		t.Fatalf("Read error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Read sample = %+v, want %+v", got, want)
	}

	r = FakeReader{Err: readErr}
	if _, err := r.Read(); !errors.Is(err, readErr) {
		t.Errorf("Read error = %v, want %v", err, readErr)
	}
}
