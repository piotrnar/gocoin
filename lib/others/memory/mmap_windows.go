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
	/*144MB-33-12558MB*/ 72, 80, 96, 104, 120, 128, 152, 160, 184, 200, 240, 264, 288, 312, 368, 432, 520, 672, 824, 1032, 1392, 1744, 2232, 2952, 4064, 5008, 6520, 8160, 10888, 13072, 16336, 21760, 32724,
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
		//println("Using VirtualAlloc for 64 KB pages")
	} else {
		mmap = mmapX
		///println("Using VirtualAlloc2 for", 1<<(pageSizeLog-10), "KB pages")
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
