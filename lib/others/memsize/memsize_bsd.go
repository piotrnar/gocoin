//go:build freebsd || openbsd || netbsd

package memsize

import (
	"syscall"
)

// ResidentMemory returns the resident set size (RSS) in bytes.
// On BSD systems, we use getrusage - ru_maxrss is in KB.
func ResidentMemory() (uint64, error) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0, err
	}
	// On BSD, ru_maxrss is in kilobytes
	return uint64(rusage.Maxrss) * 1024, nil
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
// Note: On BSD without procfs, detailed info is limited.
func DetailedMemory() (*MemoryStats, error) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return nil, err
	}

	return &MemoryStats{
		RSS: uint64(rusage.Maxrss) * 1024,
	}, nil
}
