// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memory // import "modernc.org/memory"

import (
	"os"
	"syscall"
	"unsafe"
)

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = []uint32{
	/*12613*/ //72, 80, 96, 104, 112, 120, 128, 136, 152, 160, 168, 184, 200, 216, 240, 264, 289, 313, 352, 401, 434, 512, 624, 711, 837, 953, 1166, 1431, 1613, 1795, 2022, 2495, 2953, 3094, 3614, 4069, 4654, 5434, 6525, 7253, 9332, 10892, 13075, 16350, 21808, 32724,
	/*12578*/ //72, 80, 96, 104, 112, 120, 128, 136, 144, 152, 160, 168, 176, 184, 192, 200, 216, 224, 240, 248, 264, 272, 289, 313, 328, 352, 368, 401, 434, 472, 512, 560, 593, 624, 672, 711, 755, 837, 898, 953, 1032, 1166, 1235, 1340, 1431, 1535, 1613, 1795, 1960, 2022, 2234, 2495, 2705, 2823, 2953, 3094, 3250, 3614, 3828, 4069, 4654, 5014, 5434, 5930, 6525, 7253, 8163, 9332, 10892, 13075, 16350, 21808, 32724,
	/*12570*/ //72, 80, 96, 104, 112, 120, 128, 136, 144, 152, 160, 168, 176, 184, 192, 200, 208, 216, 224, 232, 240, 248, 257, 264, 272, 280, 289, 305, 313, 328, 346, 352, 368, 393, 401, 434, 457, 472, 491, 512, 531, 560, 593, 624, 644, 672, 711, 755, 784, 837, 873, 898, 953, 983, 1032, 1067, 1125, 1166, 1235, 1260, 1312, 1399, 1431, 1535, 1613, 1699, 1795, 1902, 1960, 2022, 2159, 2234, 2315, 2495, 2705, 2823, 2953, 3094, 3250, 3423, 3614, 3828, 4069, 4342, 4654, 5014, 5434, 5930, 6525, 7253, 8163, 9332, 10892, 13075, 16350, 21808, 32724,
	/*12566*/ 72, 80, 88, 96, 104, 112, 120, 128, 136, 144, 152, 160, 168, 176, 184, 192, 200, 208, 216, 224, 232, 240, 248, 257, 264, 272, 280, 289, 297, 305, 313, 320, 328, 337, 346, 352, 361, 368, 377, 385, 393, 401, 409, 418, 424, 434, 440, 450, 457, 464, 472, 483, 491, 499, 504, 512, 531, 545, 560, 576, 593, 611, 624, 644, 658, 672, 695, 711, 720, 737, 755, 774, 784, 805, 826, 837, 849, 873, 885, 898, 925, 939, 953, 968, 983, 999, 1015, 1032, 1067, 1105, 1125, 1166, 1188, 1235, 1260, 1285, 1312, 1369, 1399, 1431, 1464, 1499, 1535, 1573, 1613, 1655, 1699, 1746, 1795, 1847, 1902, 1960, 2022, 2088, 2159, 2234, 2315, 2401, 2495, 2595, 2705, 2823, 2953, 3094, 3250, 3423, 3614, 3828, 4069, 4342, 4654, 5014, 5434, 5930, 6525, 7253, 8163, 9332, 10892, 13075, 16350, 21808, 32724,
}

const (
	_MEM_COMMIT   = 0x1000
	_MEM_RESERVE  = 0x2000
	_MEM_DECOMMIT = 0x4000
	_MEM_RELEASE  = 0x8000

	_PAGE_READWRITE = 0x0004
	_PAGE_NOACCESS  = 0x0001

	MemExtendedParameterAddressRequirements = 1

	pageSizeLog   = 16
	mmapAlignment = 1 << pageSizeLog // always align to page size
)

type MEM_ADDRESS_REQUIREMENTS struct {
	LowestStartingAddress uintptr
	HighestEndingAddress  uintptr
	Alignment             uintptr
}

type MEM_EXTENDED_PARAMETER struct {
	Type    uint64
	Pointer uintptr
}

var (
	modkernel32       = syscall.NewLazyDLL("kernel32.dll")
	modkernelbase     = syscall.NewLazyDLL("kernelbase.dll")
	osPageMask        = osPageSize - 1
	osPageSize        = os.Getpagesize()
	procVirtualAlloc  = modkernel32.NewProc("VirtualAlloc")
	procVirtualAlloc2 = modkernelbase.NewProc("VirtualAlloc2")
	procVirtualFree   = modkernel32.NewProc("VirtualFree")
	mmap              func(int) (uintptr, int, error)
)

func init() {
	if pageSizeLog == 16 {
		mmap = mmap64
		println("Using VirtualAlloc for 64 KB pages")
	} else {
		mmap = mmapX
		println("Using VirtualAlloc2 for", 1<<(pageSizeLog-10), "KB pages")
	}
}

// pageSize aligned.
func mmap64(size int) (uintptr, int, error) {
	size = roundup(size, osPageSize) // Round to OS page (4KB), not allocator pageSize (64KB)
	addr, _, err := procVirtualAlloc.Call(0, uintptr(size), _MEM_COMMIT|_MEM_RESERVE, _PAGE_READWRITE)
	if err.(syscall.Errno) != 0 || addr == 0 {
		return addr, size, err
	}
	return addr, size, nil
}

// aby value aligned using VirtualAlloc2
func mmapX(size int) (uintptr, int, error) {
	size = roundup(size, osPageSize)

	var addressReqs MEM_ADDRESS_REQUIREMENTS
	addressReqs.Alignment = mmapAlignment

	var param MEM_EXTENDED_PARAMETER
	param.Type = MemExtendedParameterAddressRequirements
	param.Pointer = uintptr(unsafe.Pointer(&addressReqs))

	addr, _, err := procVirtualAlloc2.Call(
		0,                               // Process (NULL = current process)
		0,                               // BaseAddress (NULL = let system choose)
		uintptr(size),                   // Size
		_MEM_COMMIT|_MEM_RESERVE,        // AllocationType
		_PAGE_READWRITE,                 // PageProtection
		uintptr(unsafe.Pointer(&param)), // ExtendedParameters
		1,                               // ParameterCount
	)

	if err.(syscall.Errno) != 0 || addr == 0 {
		return addr, size, err
	}
	return addr, size, nil // Return original size, not rounded size
}

func unmap(addr uintptr, size int) error {
	r, _, err := procVirtualFree.Call(addr, 0, _MEM_RELEASE)
	if r == 0 {
		return err
	}

	return nil
}
