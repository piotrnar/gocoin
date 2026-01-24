//go:build linux

package memsize

import (
	"os"
	"strconv"
	"strings"
)

// pageSize is cached at init time
var pageSize = os.Getpagesize()

// ResidentMemory returns the resident set size (RSS) in bytes.
// This is the actual physical memory used by the process.
func ResidentMemory() (uint64, error) {
	// Read /proc/self/statm - it's faster than /proc/self/status
	// Format: size resident shared text lib data dt (all in pages)
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, os.ErrInvalid
	}

	// Field 1 is resident pages
	rssPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}

	return rssPages * uint64(pageSize), nil
}

// MemoryStats returns detailed memory statistics.
type MemoryStats struct {
	RSS      uint64 // Resident Set Size - physical memory used
	VMS      uint64 // Virtual Memory Size - total virtual memory
	Shared   uint64 // Shared memory
	Text     uint64 // Text (code) segment
	Data     uint64 // Data + stack
}

// DetailedMemory returns detailed memory statistics.
func DetailedMemory() (*MemoryStats, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return nil, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 6 {
		return nil, os.ErrInvalid
	}

	stats := &MemoryStats{}
	ps := uint64(pageSize)

	if v, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
		stats.VMS = v * ps
	}
	if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
		stats.RSS = v * ps
	}
	if v, err := strconv.ParseUint(fields[2], 10, 64); err == nil {
		stats.Shared = v * ps
	}
	if v, err := strconv.ParseUint(fields[3], 10, 64); err == nil {
		stats.Text = v * ps
	}
	if v, err := strconv.ParseUint(fields[5], 10, 64); err == nil {
		stats.Data = v * ps
	}

	return stats, nil
}
