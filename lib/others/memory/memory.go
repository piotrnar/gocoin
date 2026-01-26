// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.

package memory

import (
	"reflect"
	"unsafe"
)

const (
	shareHdrSize = unsafe.Sizeof(page_header{})
	dedicHdrSize = unsafe.Sizeof(page_header_common{})
	pageAvail    = pageSize - shareHdrSize
	pageMask     = pageSize - 1
	pageSize     = 1 << pageSizeLog
)

var (
	currentSequence uint64
)

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	Allocs int // # of allocs.
	Bytes  int // Asked from OS.
	Mmaps  int // Asked from OS.

	firstPage []*page_header // first page
	lastPage  []*page_header // last page
	freePage  []*page_header // page with lowest sequence and any free records
	slotSize  []int          // quickly get slot size from the class value
}

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = []uint16{
	64, 72, 80, 88, 96, 104, 112, 120, 128, 136, 144, 152, 160, 168, 176, 184, 192,
	200, 208, 216, 224, 232, 240, 248, 256, 272, 288,
	304, 320, 336, 352, 368, 400, 416, 432, 464, 480,
	512, 592, 704,
	1024, 1280, 1536, 1792, 2048, 2304, 2816,
	4096, 8192, 16378, 32768,
}

type page_header_common struct {
	class int16  // -1 for private page
	cap   uint16 // number of records (not used for private page)
	siz   uint32 // total page size from mmap (including header)
}

type page_header struct {
	page_header_common
	seq          uint64
	prev, next   *page_header
	freeListOffs uint32 // offset to the first free record (or 0 nor nil)
	dirty        uint16 // how many records were ever used
	used         uint16 // how many records are used now
}

type free_record struct {
	next_free_record uintptr
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
func getSizeClass(size int) int {
	for i, v := range sizeClassSlotSize {
		if size <= int(v) {
			return i
		}
	}
	return -1
}

// getSlotSize returns the actual slot size for a size class index
func (a *Allocator) getSlotSize(class int) int {
	if class >= len(a.slotSize) {
		panic("illegal class vallie")
	}
	return a.slotSize[class]
}

func (a *Allocator) getSlotSizeX(class int) int {
	if class >= 0 && class < len(sizeClassSlotSize) {
		return int(sizeClassSlotSize[class])
	}
	panic("invalid size class")
}

// if n%m != 0 { n += m-n%m }. m must be a power of 2.
func roundup(n, m int) int { return (n + m - 1) &^ (m - 1) }

func NewAllocator() (a *Allocator) {
	a = new(Allocator)
	a.firstPage = make([]*page_header, len(sizeClassSlotSize))
	a.lastPage = make([]*page_header, len(sizeClassSlotSize))
	a.freePage = make([]*page_header, len(sizeClassSlotSize))
	a.slotSize = make([]int, len(sizeClassSlotSize))
	for i := range a.slotSize {
		a.slotSize[i] = a.getSlotSizeX(i)
	}
	return
}

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

	(*page_header)(unsafe.Pointer(p)).class = -1 // Mark as dedicated page
	return p, nil
}

// newSharedPage creates a new shared page for a specific size class
func (a *Allocator) newSharedPage(class int) (uintptr /* *page */, error) {
	slotSize := a.getSlotSize(class)
	if slotSize == 0 {
		panic("invalid size class")
	}

	records_cnt := uint32(pageAvail) / uint32(slotSize)
	totalSize := uint32(shareHdrSize) + records_cnt*uint32(slotSize)
	p, err := a.mmap(int(totalSize))
	if err != nil {
		return 0, err
	}

	pag := (*page_header)(unsafe.Pointer(p))
	if a.lastPage[class] != nil {
		a.lastPage[class].next = pag
		pag.prev = a.lastPage[class]
	} else {
		if a.firstPage[class] != nil {
			panic("last page nil but first not")
		}
		a.firstPage[class] = pag
	}
	a.lastPage[class] = pag
	pag.class = int16(class)
	pag.cap = uint16(records_cnt)
	(*page_header)(unsafe.Pointer(p)).seq = currentSequence
	currentSequence++
	return p, nil
}

func (a *Allocator) unmap(p uintptr, l int) error {
	if counters {
		a.Mmaps--
	}
	return unmap(p, l)
}

// UintptrFree is like Free except its argument is an uintptr
func (a *Allocator) UintptrFree(p uintptr, siz int) (err error) {
	if p == 0 {
		return nil
	}

	if counters {
		a.Allocs--
	}

	pg := p &^ uintptr(pageMask)
	pag := (*page_header)(unsafe.Pointer(pg))

	// Dedicated page (large allocation) - slotSize == 0
	if pag.class < 0 {
		if counters {
			a.Bytes -= int(pag.siz)
		}
		return a.unmap(pg, siz)
	}

	// Shared page - find class from slotSize
	class := (int(pag.class))

	// Add to page's free list (insert at head)
	(*free_record)(unsafe.Pointer(p)).next_free_record = addOffset(uintptr(unsafe.Pointer(&pag.cap)), pag.freeListOffs)
	pag.updateFreeList(p)
	pag.used--

	// Update freePage if this page has lower sequence than current freePage
	if a.freePage[class] == nil || pag.seq < a.freePage[class].seq {
		a.freePage[class] = pag
	}

	if pag.used != 0 {
		return nil
	}

	// Page is completely free - unmap it

	// If we're removing freePage, find the next best page starting from pag.next
	if a.freePage[class] == pag {
		a.freePage[class] = nil
		for pg := pag.next; pg != nil; pg = pg.next {
			if pg.freeListOffs != 0 {
				a.freePage[class] = pg
				break // pages are in sequence order, first one found is the lowest
			}
		}
	}

	// Remove from linked list
	if pag.prev != nil {
		pag.prev.next = pag.next
	} else {
		a.firstPage[class] = pag.next // this was the first page - set the next one
	}
	if pag.next != nil {
		pag.next.prev = pag.prev
	} else {
		a.lastPage[class] = pag.prev // this was the last page - set the previous one
	}
	if counters {
		a.Bytes -= int(pag.siz)
	}
	return a.unmap(pg, int(pag.siz))
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

	class := getSizeClass(size)

	// Large allocation - use dedicated page
	if class < 0 {
		p, err := a.newPage(size)
		if err != nil {
			return 0, err
		}
		return p + dedicHdrSize, nil
	}

	// Small allocation - use shared page
	// First try freePage (page with lowest seq that has free slots)
	if p := a.freePage[class]; p != nil {
		// Allocate from freePage's free list (remove from head)
		var n uintptr
		if p.freeListOffs != 0 {
			n = addOffset(uintptr(unsafe.Pointer(&p.cap)), p.freeListOffs)
			p.updateFreeList((*free_record)(unsafe.Pointer(n)).next_free_record)
		} else {
			if p.dirty < p.cap {
				n = uintptr(unsafe.Pointer(p)) + shareHdrSize + uintptr(p.dirty)*uintptr(a.getSlotSize(class))
				p.dirty++
			} else {
				panic("p.freeList is 0 and p.brk >= p.cap")
			}
		}
		p.used++

		// If page has no more free slots, find next best freePage starting from p.next
		if p.freeListOffs == 0 && p.dirty == p.cap {
			a.freePage[class] = nil
			for pg := p.next; pg != nil; pg = pg.next {
				if pg.freeListOffs != 0 {
					a.freePage[class] = pg
					break // pages are in sequence order, first one found is the lowest
				}
			}
		}
		return n, nil
	}

	// if we got heer, we have no pages or all are full
	if _, err := a.newSharedPage(class); err != nil {
		return 0, err
	}

	// Allocate from the new page via bump
	p := a.lastPage[class]
	a.freePage[class] = p
	p.used, p.dirty = 1, 1
	return uintptr(unsafe.Pointer(p)) + shareHdrSize, nil
}

// Free deallocates memory (as in C.free).
func (a *Allocator) Free(b *[]byte) (err error) {
	return a.UintptrFree(uintptr(unsafe.Pointer(b)), 24+2+len(*b))
}

// Malloc allocates size bytes and returns a byte slice.
func (a *Allocator) Malloc(size int) (r *[]byte, err error) {
	size += 24
	p, err := a.UintptrMalloc(size)
	if p == 0 || err != nil {
		return nil, err
	}

	//r = unsafe.Slice((*byte)(unsafe.Pointer(p)), usableSize(p))
	//return r[:size], nil
	sh := (*reflect.SliceHeader)(unsafe.Pointer(p))
	sh.Cap = size - 24
	sh.Data = uintptr(p + 24)
	sh.Len = size - 24
	return (*[]byte)(unsafe.Pointer(sh)), nil
}

func init() {
	println("memory: page_header len is", unsafe.Sizeof(page_header{}))
	for i := range sizeClassSlotSize {
		sizeClassSlotSize[i] += 24 - 8
	}
}
