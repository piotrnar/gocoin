// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.

package memory // import "modernc.org/memory"

import (
	"unsafe"
)

const (
	counters = true

	headerSize   = unsafe.Sizeof(page_header{})
	dedicHdrSize = unsafe.Sizeof(page_header_common{})
	pageAvail    = pageSize - headerSize
	pageMask     = pageSize - 1
	pageSize     = 1 << pageSizeLog
	sizeIncrease = 24 // part of the UTXO record may be stored as map's key and need to be substracted
)

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = []uint32{
	72, 80, 96, 104, 112, 120, 128, 136, 152, 160, 168, 184, 200, 216, 240, 272, 288, 320, 368, 400, 432, 512, 576, 640, 704, 768, 896, 1024, 1216, 1408, 1728, 2048, 2304, 2560, 2944, 3200, 3712, 4096, 5120, 6144, 6912, 8192, 9216, 10240, 11264, 13312, 15360, 17408, 21504, 28672, 32768,
}

type node struct {
	prev, next             uintptr // *node - global free list (across all pages in class)
	prevInPage, nextInPage uintptr // *node - per-page free list (only slots from same page)
}

type page_header_common struct {
	class int16  // -1 for private page
	cap   uint16 // number of records (not used for private page)
	siz   uint32 // total page size from mmap, including header, padded to osPageSize
}

type page_header struct {
	page_header_common
	brk        uint16  // high water mark of allocated slots
	used       uint16  // number of currently used slots
	evacuating uint16  // 1 during defragmentation to prevent freed slots from re-entering free list
	free       uint16  // number of free slots in this page (for quick defrag decisions)
	prev       uintptr // previous page in class (linked list of all pages)
	next       uintptr // next page in class (linked list of all pages)
	freeList   uintptr // head of per-page free list (using nextInPage/prevInPage pointers)
}

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	Allocs        int // # of allocs.
	Bytes         int // Asked from OS.
	cap           []uint32
	lists         []uintptr // *node - free lists per size class
	Mmaps         int       // Asked from OS.
	pages         []uintptr // *page - current page per size class
	classIdx      []byte
	maxSharedSize int

	// Defragmentation optimization fields
	firstPage []uintptr // first page in linked list per class
	lastPage  []uintptr // last page in linked list per class
	pageCount []uint32  // number of pages per class
	freeSlots []uint32  // total free slots per class
}

// if n%m != 0 { n += m-n%m }. m must be a power of 2.
func roundup(n, m int) int { return (n + m - 1) &^ (m - 1) }

// getSizeClass returns the size class index for a given allocation size.
// This is the core routing function that determines which slot size to use.
func (a *Allocator) getSizeClass(size int) int {
	if size >= a.maxSharedSize {
		return -1
	}
	return int(a.classIdx[size])
}

// getSlotSize returns the actual slot size for a size class index
func getSlotSize(class int) uint32 {
	if class >= 0 && class < len(sizeClassSlotSize) {
		return sizeClassSlotSize[class]
	}
	return 0
}

func NewAllocator() (a *Allocator) {
	a = new(Allocator)
	a.cap = make([]uint32, len(sizeClassSlotSize))
	a.lists = make([]uintptr, len(sizeClassSlotSize))
	a.pages = make([]uintptr, len(sizeClassSlotSize))

	// Initialize defragmentation optimization fields
	a.firstPage = make([]uintptr, len(sizeClassSlotSize))
	a.lastPage = make([]uintptr, len(sizeClassSlotSize))
	a.pageCount = make([]uint32, len(sizeClassSlotSize))
	a.freeSlots = make([]uint32, len(sizeClassSlotSize))

	for i := range a.cap {
		a.cap[i] = uint32(pageAvail) / sizeClassSlotSize[i]
	}

	a.maxSharedSize = int(sizeClassSlotSize[len(sizeClassSlotSize)-1])
	a.classIdx = make([]byte, a.maxSharedSize+1)
	for size := range a.classIdx {
		for i, v := range sizeClassSlotSize {
			if size <= int(v) {
				a.classIdx[size] = byte(i)
				break
			}
		}
	}
	return
}

func init() {
	// discard records that would be less then half of available page space
	max_class_size := ((1<<pageSizeLog)-uint32(headerSize))/2 - sizeIncrease
	println("page size:", (1 << pageSizeLog), " page header size:", headerSize, "  max class size:", max_class_size)
	print(len(sizeClassSlotSize), " slot sizes: ")
	for _, ss := range sizeClassSlotSize {
		print(ss, ", ")
	}
	println()
	if sizeClassSlotSize[len(sizeClassSlotSize)-1]+sizeIncrease > max_class_size {
		for mx := len(sizeClassSlotSize) - 1; mx > 0; mx-- {
			if sizeClassSlotSize[mx-1]+sizeIncrease <= max_class_size {
				sizeClassSlotSize = sizeClassSlotSize[:mx]
				println("sizeClassSlotSize trimmed to", len(sizeClassSlotSize), "records")
				break
			}
		}
	}

	// adjust slot sizes according to the actually expected alloc sizes
	for i := range sizeClassSlotSize {
		sizeClassSlotSize[i] -= sizeIncrease
	}
}
