//go:build windows

package memsize

import (
	"syscall"
	"unsafe"
)

var (
	psapi                     = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo  = psapi.NewProc("GetProcessMemoryInfo")
)

// PROCESS_MEMORY_COUNTERS structure
type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr // This is RSS equivalent
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// ResidentMemory returns the working set size (RSS equivalent) in bytes.
func ResidentMemory() (uint64, error) {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, err
	}

	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))

	ret, _, err := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.CB),
	)

	if ret == 0 {
		return 0, err
	}

	return uint64(pmc.WorkingSetSize), nil
}

// MemoryStats returns detailed memory statistics.
type MemoryStats struct {
	RSS      uint64 // Working Set Size - physical memory used
	VMS      uint64 // PagefileUsage - committed memory
	Shared   uint64 // Not available on Windows via this API
	Text     uint64 // Not available
	Data     uint64 // Not available
}

// DetailedMemory returns detailed memory statistics.
func DetailedMemory() (*MemoryStats, error) {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}

	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))

	ret, _, err := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.CB),
	)

	if ret == 0 {
		return nil, err
	}

	return &MemoryStats{
		RSS: uint64(pmc.WorkingSetSize),
		VMS: uint64(pmc.PagefileUsage),
	}, nil
}
