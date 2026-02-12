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
	/*140MB-33-12687MB*/ //72, 80, 96, 104, 120, 128, 144, 160, 184, 200, 240, 264, 288, 312, 368, 432, 512, 624, 824, 1064, 1400, 1728, 2224, 2952, 3992, 4992, 6408, 8024, 10152, 12744, 16352, 21024, 32736,
	/*154MB-35-12796MB*/ //72, 80, 96, 104, 120, 128, 144, 160, 184, 200, 240, 264, 288, 312, 368, 432, 512, 624, 824, 1064, 1400, 1728, 2224, 2952, 3992, 4992, 6408, 8024, 10152, 12744, 16352, 21216, 31568, 41608, 65504,
	/*186MB-33-12883MB*/ //72, 80, 96, 104, 120, 128, 160, 200, 240, 264, 288, 352, 432, 512, 624, 824, 1064, 1400, 1728, 2224, 2952, 3992, 5464, 7152, 9248, 11976, 16088, 21320, 32664, 45480, 65272, 86960, 131040,
	/*159MB-37-12829MB*/ //72, 80, 96, 104, 120, 128, 144, 160, 184, 200, 240, 264, 288, 312, 368, 432, 512, 624, 824, 1064, 1400, 1728, 2224, 2952, 3992, 4992, 6408, 8024, 10152, 12744, 16352, 21216, 29928, 41904, 57832, 86960, 131040,
	/*145MB-40-12802MB*/ 72, 80, 96, 104, 120, 128, 144, 160, 184, 200, 240, 264, 288, 312, 368, 432, 512, 624, 752, 864, 1064, 1400, 1728, 2224, 2896, 3304, 4040, 4960, 6208, 7896, 9768, 11888, 15600, 20040, 24880, 32664, 45480, 65272, 86960, 262112,
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

// pageSize aligned if size 0.
func mmap(size int) (uintptr, int, error) {
	if size != 0 {
		p, err := mmapSyscall(0, uintptr(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON, -1, 0)
		if err != nil {
			return 0, 0, err
		}
		return p, size, nil
	}

	size = pageSize
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
