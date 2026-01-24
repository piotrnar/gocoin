// Package memsize provides functions to get the physical memory usage
// of the current process across different platforms.
//
// The main function ResidentMemory() returns the RSS (Resident Set Size),
// which represents the actual physical RAM used by the process.
//
// Platform-specific implementations:
//   - Linux: reads /proc/self/statm (fast, no syscalls)
//   - macOS: uses getrusage (returns max RSS, not current)
//   - Windows: uses GetProcessMemoryInfo from psapi.dll
//   - BSD: uses getrusage
package memsize

import "fmt"

// FormatBytes returns a human-readable string representation of bytes.
func FormatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// MustResidentMemory returns RSS or panics on error.
// Useful for logging/debugging where errors are unexpected.
func MustResidentMemory() uint64 {
	rss, err := ResidentMemory()
	if err != nil {
		panic(err)
	}
	return rss
}
