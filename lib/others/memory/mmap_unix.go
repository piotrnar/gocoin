// Copyright 2011 Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE-MMAP-GO file.

//go:build darwin || dragonfly || freebsd || linux || openbsd || solaris || netbsd
// +build darwin dragonfly freebsd linux openbsd solaris netbsd

// Modifications (c) 2017 The Memory Authors.

package memory // import "modernc.org/memory"

import (
	"os"
	"syscall"
)

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = []uint32{
	72, 80, 96, 104, 112, 120, 128, 136, 144, 152, 160, 168, 176, 184, 192, 200, 208, 216, 224, 232, 240, 248, 256, 264, 272, 280, 288, 304, 312, 328, 344, 352, 368, 392, 400, 432, 456, 472, 488, 512, 528, 560, 592, 624, 640, 672, 704, 752, 784, 832, 864, 896, 944, 976, 1024, 1056, 1120, 1152, 1216, 1248, 1313, 1377, 1408, 1504, 1601, 1697, 1762, 1858, 1921, 2019, 2115, 2178, 2306, 2437, 2630, 2757, 2817, 2880, 2946, 3078, 3202, 3336, 3459, 3591, 3720, 3978, 4040, 4238, 4495, 4742, 4992, 5380, 5523, 5769, 6036, 6292, 6529, 6784, 7060, 7308, 7685, 7980, 8232, 8713, 9255, 9775, 10255, 11013, 11626, 12168, 12920, 13418, 14145, 15172, 16359, 17165, 18054, 19040, 19759, 20535, 21374, 22770, 24941, 26189, 29102, 30815, 31749, 33799,
}

const pageSizeLog = 20

var (
	osPageMask = osPageSize - 1
	osPageSize = os.Getpagesize()
)

func unmap(addr uintptr, size int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_MUNMAP, addr, uintptr(size), 0)
	if errno != 0 {
		return errno
	}

	return nil
}

// pageSize aligned.
func mmap(size int) (uintptr, int, error) {
	size = roundup(size, osPageSize)
	// The actual mmap syscall varies by architecture. mmapSyscall provides same
	// functionality as the unexported funtion syscall.mmap and is declared in
	// mmap_*_*.go and mmap_fallback.go. To add support for a new architecture,
	// check function mmap in src/syscall/syscall_*_*.go or
	// src/syscall/zsyscall_*_*.go in Go's source code.
	p, err := mmapSyscall(0, uintptr(size+pageSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON, -1, 0)
	if err != nil {
		return 0, 0, err
	}

	n := size + pageSize
	if p&uintptr(osPageMask) != 0 {
		panic("internal error")
	}

	mod := int(p) & pageMask
	if mod != 0 {
		m := pageSize - mod
		if err := unmap(p, m); err != nil {
			return 0, 0, err
		}

		n -= m
		p += uintptr(m)
	}

	if p&uintptr(pageMask) != 0 {
		panic("internal error")
	}

	if n-size != 0 {
		if err := unmap(p+uintptr(size), n-size); err != nil {
			return 0, 0, err
		}
	}

	return p, size, nil
}
