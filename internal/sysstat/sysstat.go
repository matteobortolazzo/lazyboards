// Package sysstat reads host-wide CPU and memory counters for the status bar.
//
// A Reader is stateless: each Read returns one raw Sample. The caller keeps the
// previous Sample and derives the CPU percentage from the pair with CPUPercent,
// so concurrent reads share no mutable state.
package sysstat

import "math"

// Reader reads one raw sample of the host's CPU and memory counters.
type Reader interface {
	Read() (Sample, error)
}

// Sample is one raw reading of the host counters.
//
// Total is the sum of the aggregate cpu line's user, nice, system, idle,
// iowait, irq, softirq and steal fields; Idle is idle plus iowait. Both are
// cumulative since boot. Memory fields are in KiB.
type Sample struct {
	Total           uint64
	Idle            uint64
	MemTotalKiB     uint64
	MemAvailableKiB uint64
}

// Usage is the derived figure the status bar renders: an integer CPU percent
// and memory used/total in KiB.
type Usage struct {
	CPUPercent  int
	MemUsedKiB  uint64
	MemTotalKiB uint64
}

// CPUPercent returns round(100 x delta-busy / delta-total) between two samples,
// clamped to 0..100, where busy is Total - Idle. It returns ok=false when the
// total did not advance (delta-total <= 0, e.g. a counter reset after
// suspend), because no percentage can be computed then.
//
// Deltas are taken in signed space so a counter that went backwards is
// detected rather than wrapping to a huge unsigned value.
func CPUPercent(prev, cur Sample) (int, bool) {
	dTotal := float64(cur.Total) - float64(prev.Total)
	if dTotal <= 0 {
		return 0, false
	}
	dIdle := float64(cur.Idle) - float64(prev.Idle)
	dBusy := dTotal - dIdle
	pct := math.Round(100 * dBusy / dTotal)
	return int(math.Max(0, math.Min(100, pct))), true
}

// NewUsage combines a computed CPU percent with the memory figures of cur.
// Memory used is MemTotal - MemAvailable, clamped to 0 when a skewed kernel
// report makes MemAvailable exceed MemTotal.
func NewUsage(cpuPercent int, cur Sample) Usage {
	var used uint64
	if cur.MemTotalKiB > cur.MemAvailableKiB {
		used = cur.MemTotalKiB - cur.MemAvailableKiB
	}
	return Usage{
		CPUPercent:  cpuPercent,
		MemUsedKiB:  used,
		MemTotalKiB: cur.MemTotalKiB,
	}
}

// FakeReader is a test double for Reader that returns a fixed Sample/error
// without touching the host.
type FakeReader struct {
	Sample Sample
	Err    error
}

// Read returns the configured Sample and error.
func (f FakeReader) Read() (Sample, error) {
	return f.Sample, f.Err
}
