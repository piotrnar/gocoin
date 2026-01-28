// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memory // import "modernc.org/memory"

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	_MEM_COMMIT   = 0x1000
	_MEM_RESERVE  = 0x2000
	_MEM_DECOMMIT = 0x4000
	_MEM_RELEASE  = 0x8000

	_PAGE_READWRITE = 0x0004
	_PAGE_NOACCESS  = 0x0001

	MemExtendedParameterAddressRequirements = 1

	mmapAlignment = 1 << 20 // 1MB alignment
)

const pageSizeLog = 20

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
)

// 1MB aligned using VirtualAlloc2
func mmap(size int) (uintptr, int, error) {
	originalSize := size              // Save original requested size
	size = roundup(size, mmapAlignment) // Round up to 1MB alignment

	var addressReqs MEM_ADDRESS_REQUIREMENTS
	addressReqs.Alignment = mmapAlignment

	var param MEM_EXTENDED_PARAMETER
	param.Type = MemExtendedParameterAddressRequirements
	param.Pointer = uintptr(unsafe.Pointer(&addressReqs))

	addr, _, err := procVirtualAlloc2.Call(
		0,                              // Process (NULL = current process)
		0,                              // BaseAddress (NULL = let system choose)
		uintptr(size),                  // Size
		_MEM_COMMIT|_MEM_RESERVE,       // AllocationType
		_PAGE_READWRITE,                // PageProtection
		uintptr(unsafe.Pointer(&param)), // ExtendedParameters
		1,                              // ParameterCount
	)

	if err.(syscall.Errno) != 0 || addr == 0 {
		return addr, originalSize, err
	}
	return addr, originalSize, nil // Return original size, not rounded size
}

func unmap(addr uintptr, size int) error {
	r, _, err := procVirtualFree.Call(addr, 0, _MEM_RELEASE)
	if r == 0 {
		return err
	}

	return nil
}