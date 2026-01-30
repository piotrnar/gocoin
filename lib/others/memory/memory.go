// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.

package memory // import "modernc.org/memory"

import (
	"fmt"
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

type node struct {
	prev, next uintptr // *node
}

type page_header_common struct {
	class int16  // -1 for private page
	cap   uint16 // number of records (not used for private page)
	siz   uint32 // total page size from mmap, including header, padded to osPageSize
}

type page_header struct {
	page_header_common
	next, prev *page_header
	dirty      uint16 // how many records were ever used
	used       uint16 // how many records are used now
	_unused    uint32
}

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	Allocs        int      // # of allocs.
	Bytes         int      // Asked from OS.
	Mmaps         int      // Asked from OS.
	Capacity      []uint32 // how many slots per page this class has
	ClassIdx      []byte   // record size to class number
	MaxSharedSize int

	lists []uintptr // *node - free lists per size class
	pages []uintptr // *page - current page per size class

	fcnt []uint32 // how many free slots in all the pages
	pcnt []uint32 // how many pages we have for this

	firstPage []*page_header // first page
	lastPage  []*page_header // last page
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

// if n%m != 0 { n += m-n%m }. m must be a power of 2.
func roundup(n, m int) int { return (n + m - 1) &^ (m - 1) }

func (a *Allocator) mmap(size int) (uintptr /* *page */, error) {
	p, size, err := mmap(size)
	if err != nil {
		return 0, err
	}

	if counters {
		a.Mmaps++
		a.Bytes += size
	}
	if size > 0xffffffff {
		panic("mmap to big")
	}
	(*page_header)(unsafe.Pointer(p)).siz = uint32(size)
	return p, nil
}

// newPage creates a dedicated page for a single large allocation
func (a *Allocator) newPage(size int) (uintptr /* *page */, error) {
	size += int(dedicHdrSize)
	p, err := a.mmap(size)
	if err != nil {
		return 0, err
	}
	new_page := (*page_header)(unsafe.Pointer(p))
	new_page.cap = uint16((size + 4095) >> 10) // store number of 4K pages used (not needed now)
	new_page.class = -1                        // Mark as dedicated page
	return p, nil
}

// newSharedPage creates a new shared page for a specific size class
func (a *Allocator) newSharedPage(class int) (uintptr /* *page */, error) {
	slotSize := getSlotSize(class)
	if slotSize == 0 {
		panic(fmt.Sprintf("invalid size class: %d", class))
	}

	cap := a.Capacity[class]
	totalSize := uint32(headerSize) + cap*slotSize
	p, err := a.mmap(int(totalSize))
	if err != nil {
		return 0, err
	}
	a.fcnt[class] += cap
	a.pcnt[class]++

	new_page := (*page_header)(unsafe.Pointer(p))
	new_page.cap = uint16(cap) // it's redundant, but we have a spare 16 bits
	new_page.class = int16(class)
	if a.firstPage[class] == nil {
		if a.lastPage[class] != nil {
			panic("lastPage not nil but expected")
		}
		a.firstPage[class] = new_page
		a.lastPage[class] = new_page
	} else {
		// we already have some pages, so just add it at the end
		a.lastPage[class].next = new_page
		new_page.prev = a.lastPage[class]
		a.lastPage[class] = new_page
	}

	a.pages[class] = p
	return p, nil
}

func (a *Allocator) unmap(p uintptr /* *page */) error {
	if counters {
		a.Mmaps--
	}
	return unmap(p, int((*page_header)(unsafe.Pointer(p)).siz))
}

// UintptrFree is like Free except its argument is an uintptr
func (a *Allocator) UintptrFree(p uintptr) (err error) {
	if p == 0 {
		return nil
	}

	if counters {
		a.Allocs--
	}

	node2free := (*node)(unsafe.Pointer(p))
	pg := p &^ uintptr(pageMask)
	page2free := (*page_header)(unsafe.Pointer(pg))
	class := int(page2free.class)

	// Dedicated page (large allocation) - slotSize == 0
	if class < 0 {
		if counters {
			a.Bytes -= int(page2free.siz)
		}
		return a.unmap(pg)
	}

	a.fcnt[class]++

	// Shared page - Add to free list
	if page2free.used >= 1 {
		node2free.prev = 0
		if next := a.lists[class]; next != 0 {
			node2free.next = next
			(*node)(unsafe.Pointer(next)).prev = p
		} else {
			node2free.next = a.lists[class]
		}
		a.lists[class] = p
		page2free.used--
		return nil
	}

	// Page is completely free - unmap it
	slotSize := sizeClassSlotSize[class]
	n := pg + headerSize
	bi := page2free.dirty
	for {
		n += uintptr(slotSize)
		next := (*node)(unsafe.Pointer(n)).next
		prev := (*node)(unsafe.Pointer(n)).prev
		switch {
		case prev == 0:
			a.lists[class] = next
			if next != 0 {
				(*node)(unsafe.Pointer(next)).prev = 0
			}
		case next == 0:
			(*node)(unsafe.Pointer(prev)).next = 0
		default:
			(*node)(unsafe.Pointer(prev)).next = next
			(*node)(unsafe.Pointer(next)).prev = prev
		}
		if bi == 1 {
			break
		}
		bi--
	}

	if a.pages[class] == pg {
		a.pages[class] = 0
	}
	if counters {
		a.Bytes -= int(page2free.siz)
	}
	a.pcnt[class]--
	a.fcnt[class] -= uint32(page2free.cap)
	return a.unmap(pg)
}

// UintptrMalloc is like Malloc except it returns an uintptr.
func (a *Allocator) UintptrMalloc(size int) (r uintptr, err error) {
	if size < 0 {
		panic("invalid malloc size")
	}

	if size == 0 {
		return 0, nil
	}

	if counters {
		a.Allocs++
	}

	class := a.getSizeClass(size)

	// Large allocation - use dedicated page
	if class < 0 {
		p, err := a.newPage(size)
		if err != nil {
			return 0, err
		}
		return p + dedicHdrSize, nil
	}

	a.fcnt[class]--
	// Small allocation - use shared page
	if a.lists[class] == 0 && a.pages[class] == 0 {
		if _, err := a.newSharedPage(class); err != nil {
			return 0, err
		}
	}

	// Try to allocate from current page
	if p := a.pages[class]; p != 0 {
		page := (*page_header)(unsafe.Pointer(p))
		page.used++
		page.dirty++
		if page.dirty == page.cap {
			a.pages[class] = 0
		}
		slotSize := sizeClassSlotSize[class]
		return p + headerSize + uintptr((*page_header)(unsafe.Pointer(p)).dirty-1)*uintptr(slotSize), nil
	}

	// Allocate from free list
	n := a.lists[class]
	pg := n &^ uintptr(pageMask)
	a.lists[class] = (*node)(unsafe.Pointer(n)).next
	if next := (*node)(unsafe.Pointer(n)).next; next != 0 {
		(*node)(unsafe.Pointer(next)).prev = 0
	}
	(*page_header)(unsafe.Pointer(pg)).used++
	return n, nil
}

// Free deallocates memory (as in C.free).
func (a *Allocator) Free(b []byte) (err error) {
	if b = b[:cap(b)]; len(b) == 0 {
		return nil
	}

	return a.UintptrFree(uintptr(unsafe.Pointer(&b[0])))
}

// Malloc allocates size bytes and returns a byte slice.
func (a *Allocator) Malloc(size int) (r []byte, err error) {
	p, err := a.UintptrMalloc(size)
	if p == 0 || err != nil {
		return nil, err
	}

	r = unsafe.Slice((*byte)(unsafe.Pointer(p)), size)
	return r[:size], nil
}

func (a *Allocator) Defrag(class int, max_wasted_pages int) (res []uintptr) {
	return
}

func (a *Allocator) PrintStats() {
	for i := range a.fcnt {
		fmt.Printf("%3d) up to %5d bytes: %10d (%3d%%) free slots in %6d pages - %6.2f pages can be free\n",
			i, sizeClassSlotSize[i], a.fcnt[i], 100*a.fcnt[i]/(a.pcnt[i]*a.Capacity[i]),
			a.pcnt[i], float64(a.fcnt[i])/float64(a.Capacity[i]))
	}
}

func NewAllocator() (a *Allocator) {
	a = new(Allocator)
	a.Capacity = make([]uint32, len(sizeClassSlotSize))
	a.lists = make([]uintptr, len(sizeClassSlotSize))
	a.pages = make([]uintptr, len(sizeClassSlotSize))
	a.pcnt = make([]uint32, len(sizeClassSlotSize))
	a.fcnt = make([]uint32, len(sizeClassSlotSize))
	a.firstPage = make([]*page_header, len(sizeClassSlotSize))
	a.lastPage = make([]*page_header, len(sizeClassSlotSize))

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
