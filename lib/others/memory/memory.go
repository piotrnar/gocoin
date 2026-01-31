// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.

package memory // import "modernc.org/memory"

import (
	"unsafe"
)

const (
	headerSize   = unsafe.Sizeof(page_header{})
	dedicHdrSize = unsafe.Sizeof(page_header_common{})
	pageAvail    = pageSize - headerSize
	pageMask     = pageSize - 1
	pageSize     = 1 << pageSizeLog
)

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = []uint32{
	72, 80, 96, 104, 120, 128, 152, 160, 184, 200, 240, 272, 288, 320, 368, 432, 512, 640, 768, 1024, 1408, 1728, 2048, 2560, 2944, 4096, 5120, 6144, 8192, 11264, 13312, 17408, 21504, 32768,
	//72, 80, 96, 104, 112, 120, 128, 136, 152, 160, 168, 184, 200, 216, 240, 272, 288, 320, 368, 400, 432, 512, 576, 640, 704, 768, 896, 1024, 1216, 1408, 1728, 2048, 2304, 2560, 2944, 3200, 3712, 4096, 5120, 6144, 6912, 8192, 9216, 10240, 11264, 13312, 15360, 17408, 21504, 28672, 32768,
}

type free_record struct {
	next_free_record uintptr
}

type page_header_common struct {
	cap   uint16 // number of records (not used for private page)
	class int16  // -1 for private page
	siz   uint32 // total page size from mmap, including header, padded to osPageSize
}

type page_header struct {
	page_header_common
	next, prev   *page_header
	seq          uint64
	freeListOffs uint32 // offset to the first free record (or 0 nor nil)
	dirty        uint16 // how many records were ever used
	used         uint16 // how many records are used now
}

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	Allocs        int      // # of allocs.
	Bytes         int      // Asked from OS.
	Mmaps         int      // Asked from OS.
	Capacity      []uint32 // how many slots per page this class has
	ClassIdx      []byte   // record size to class number
	MaxSharedSize int

	currentSequence uint64

	pages []uintptr // *page - current page per size class

	fcnt []uint32 // how many free slots in all the pages
	pcnt []uint32 // how many pages we have for this

	firstPage []*page_header // first page
	lastPage  []*page_header // last page
	freePage  []*page_header // page with lowest sequence and any free records

	DerfagClass int
}

// this will return 0 if offs is zero, otherwise the sum of the two uints
func addOffset(p uintptr, offs uint32) uintptr {
	if offs == 0 {
		return 0
	}
	return p + uintptr(offs)
}

// sets page's freeListOffset to point to the given record
func (h *page_header) updateFreeList(rec uintptr) {
	if rec == 0 {
		h.freeListOffs = 0
	} else {
		h.freeListOffs = uint32(rec - uintptr(unsafe.Pointer(&h.cap)))
	}
}

// getSizeClass returns the size class index for a given allocation size.
// This is the core routing function that determines which slot size to use.
func (a *Allocator) getSizeClass(size int) int {
	if size >= a.MaxSharedSize {
		return -1
	}
	return int(a.ClassIdx[size])
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
	a.Capacity = make([]uint32, len(sizeClassSlotSize))
	a.pages = make([]uintptr, len(sizeClassSlotSize))
	a.pcnt = make([]uint32, len(sizeClassSlotSize))
	a.fcnt = make([]uint32, len(sizeClassSlotSize))
	a.firstPage = make([]*page_header, len(sizeClassSlotSize))
	a.lastPage = make([]*page_header, len(sizeClassSlotSize))
	a.freePage = make([]*page_header, len(sizeClassSlotSize))

	for i := range a.Capacity {
		a.Capacity[i] = uint32(pageAvail) / sizeClassSlotSize[i]
	}

	a.MaxSharedSize = int(sizeClassSlotSize[len(sizeClassSlotSize)-1])
	a.ClassIdx = make([]byte, a.MaxSharedSize+1)
	for size := range a.ClassIdx {
		for i, v := range sizeClassSlotSize {
			if size <= int(v) {
				a.ClassIdx[size] = byte(i)
				break
			}
		}
	}
	return
}

func init() {
	// discard records that would be less then half of available page space
	max_class_size := ((1 << pageSizeLog) - uint32(headerSize)) / 2
	println("psize:", (1 << pageSizeLog), " pheader:", headerSize, "  max:", max_class_size)
	print("slots[", len(sizeClassSlotSize), "] : ")
	for _, ss := range sizeClassSlotSize {
		print(ss, ", ")
	}
	println()
	if sizeClassSlotSize[len(sizeClassSlotSize)-1] > max_class_size {
		for mx := len(sizeClassSlotSize) - 1; mx > 0; mx-- {
			if sizeClassSlotSize[mx-1] <= max_class_size {
				sizeClassSlotSize = sizeClassSlotSize[:mx]
				println("sizeClassSlotSize trimmed to", len(sizeClassSlotSize), "records")
				break
			}
		}
	}
}
