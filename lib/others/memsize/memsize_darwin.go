//go:build darwin

package memsize

import (
	"syscall"
	"unsafe"
)

// #include <mach/mach.h>
// But we'll use syscall directly to avoid cgo

// ResidentMemory returns the resident set size (RSS) in bytes.
// On macOS, we use the rusage syscall.
func ResidentMemory() (uint64, error) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0, err
	}
	// On macOS, ru_maxrss is in bytes (unlike Linux where it's in KB)
	return uint64(rusage.Maxrss), nil
}

// For more accurate current RSS on macOS, we need mach calls
// This is a simplified version using rusage

// MemoryStats returns detailed memory statistics.
type MemoryStats struct {
	RSS      uint64 // Resident Set Size - physical memory used
	VMS      uint64 // Virtual Memory Size - total virtual memory
	Shared   uint64 // Shared memory
	Text     uint64 // Text (code) segment
	Data     uint64 // Data + stack
}

// task_basic_info structure for mach calls
type taskBasicInfo struct {
	VirtualSize  uint64
	ResidentSize uint64
	// ... other fields we don't need
}

// DetailedMemory returns detailed memory statistics.
// Note: On macOS without cgo, we have limited information available.
func DetailedMemory() (*MemoryStats, error) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return nil, err
	}

	return &MemoryStats{
		RSS: uint64(rusage.Maxrss),
		// Other fields require mach API calls which need cgo
	}, nil
}

// For full mach-based memory info, here's a cgo version you could use:
/*
// +build darwin,cgo

// #include <mach/mach.h>
import "C"

func ResidentMemoryCgo() (uint64, error) {
	var info C.mach_task_basic_info_data_t
	var count C.mach_msg_type_number_t = C.MACH_TASK_BASIC_INFO_COUNT

	kr := C.task_info(
		C.mach_task_self(),
		C.MACH_TASK_BASIC_INFO,
		(*C.integer_t)(unsafe.Pointer(&info)),
		&count,
	)

	if kr != C.KERN_SUCCESS {
		return 0, fmt.Errorf("task_info failed: %d", kr)
	}

	return uint64(info.resident_size), nil
}
*/

// Unused import suppression
var _ = unsafe.Pointer(nil)
