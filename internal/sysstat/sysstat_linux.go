package sysstat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cpuFieldCount is how many numeric fields of the aggregate cpu line feed the
// totals: user, nice, system, idle, iowait, irq, softirq, steal. The trailing
// guest and guest_nice fields are already counted inside user/nice, so they
// are deliberately excluded.
const cpuFieldCount = 8

// Positions of the idle and iowait fields within the cpu line's numbers.
const (
	cpuIdleIndex   = 3
	cpuIowaitIndex = 4
)

// procReader reads /proc-shaped files from root, so tests can point it at a
// directory of recorded samples.
type procReader struct {
	root string
}

// NewReader returns a Reader backed by the real /proc filesystem.
func NewReader() Reader {
	return procReader{root: "/proc"}
}

// Read gathers one Sample from root/stat and root/meminfo. A missing or
// malformed file is an error, never a partial sample.
func (p procReader) Read() (Sample, error) {
	statData, err := os.ReadFile(filepath.Join(p.root, "stat"))
	if err != nil {
		return Sample{}, err
	}
	total, idle, err := parseStat(string(statData))
	if err != nil {
		return Sample{}, err
	}

	memData, err := os.ReadFile(filepath.Join(p.root, "meminfo"))
	if err != nil {
		return Sample{}, err
	}
	memTotal, memAvailable, err := parseMeminfo(string(memData))
	if err != nil {
		return Sample{}, err
	}

	return Sample{
		Total:           total,
		Idle:            idle,
		MemTotalKiB:     memTotal,
		MemAvailableKiB: memAvailable,
	}, nil
}

// parseStat extracts total and idle(+iowait) jiffies from the aggregate `cpu `
// line of /proc/stat content. The line needs at least cpuFieldCount numeric
// fields; per-core `cpuN` lines are ignored.
func parseStat(content string) (total, idle uint64, err error) {
	for line := range strings.SplitSeq(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		nums := fields[1:]
		if len(nums) < cpuFieldCount {
			return 0, 0, fmt.Errorf("cpu line has %d numeric fields, need %d", len(nums), cpuFieldCount)
		}
		values := make([]uint64, cpuFieldCount)
		for i := range cpuFieldCount {
			v, perr := strconv.ParseUint(nums[i], 10, 64)
			if perr != nil {
				return 0, 0, fmt.Errorf("cpu field %d: %w", i+1, perr)
			}
			values[i] = v
			total += v
		}
		return total, values[cpuIdleIndex] + values[cpuIowaitIndex], nil
	}
	return 0, 0, errors.New("no aggregate cpu line in stat")
}

// parseMeminfo extracts MemTotal and MemAvailable (both KiB) from
// /proc/meminfo content. Both lines are required.
func parseMeminfo(content string) (memTotal, memAvailable uint64, err error) {
	var haveTotal, haveAvailable bool
	for line := range strings.SplitSeq(content, "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found || (key != "MemTotal" && key != "MemAvailable") {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, 0, fmt.Errorf("%s has no value", key)
		}
		v, perr := strconv.ParseUint(fields[0], 10, 64)
		if perr != nil {
			return 0, 0, fmt.Errorf("%s: %w", key, perr)
		}
		if key == "MemTotal" {
			memTotal, haveTotal = v, true
		} else {
			memAvailable, haveAvailable = v, true
		}
	}
	if !haveTotal {
		return 0, 0, errors.New("meminfo has no MemTotal")
	}
	if !haveAvailable {
		return 0, 0, errors.New("meminfo has no MemAvailable")
	}
	return memTotal, memAvailable, nil
}
