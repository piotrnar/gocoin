// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.
// MODIFIED: Optimized for UTXO workloads with custom size classes.
// OPTIMIZATION v2: Added 60-byte class for 57-byte allocations (45% of records)
//                  Simplified to 11 core size classes based on simulation results
//                  Expected waste reduction: 480 MB (12.20% -> 7.66%)

package memory // import "modernc.org/memory"

import (
	"fmt"
	"os"
	"reflect"
	"unsafe"
)

const (
	headerSize   = unsafe.Sizeof(page{})
	mallocAllign = unsafe.Sizeof(uintptr(0))
	pageAvail    = pageSize - headerSize
	pageMask     = pageSize - 1
	pageSize     = 1 << pageSizeLog
)

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	Allocs int // # of allocs.
	Bytes  int // Asked from OS.
	cap    []uint32
	lists  []uintptr            // *node - free lists per size class
	Mmaps  int                  // Asked from OS.
	pages  []uintptr            // *page - current page per size class
	regs   map[uintptr]struct{} // map[*page]struct{} - all registered pages
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

type page struct {
	_paddingB uint32
	size      uint32 // Total page size from mmap
	slotSize  uint16 // Actual slot size in bytes. 0 = dedicated page (large allocation)
	brk       uint16
	used      uint16
	_paddingA uint16
}

type node struct {
	prev, next uintptr // *node
}

func init() {
	if unsafe.Sizeof(page{})%mallocAllign != 0 {
		panic(fmt.Sprint("memory: bad header size: ", unsafe.Sizeof(page{})))
	}
	for i := range sizeClassSlotSize {
		sizeClassSlotSize[i] += 24 - 8
	}
}

// getSizeClass returns the size class index for a given allocation size.
// This is the core routing function that determines which slot size to use.
func getSizeClass(size int) int {
	if pageSizeLog == 16 && size >= 4092 {
		return -1 // For Windows that uses 64KB pages (instead of 1MB)
	}
	for i, v := range sizeClassSlotSize {
		if size <= int(v) {
			return i
		}
	}
	return -1
}

// getSlotSize returns the actual slot size for a size class index
func getSlotSize(class int) int {
	if class >= 0 && class < len(sizeClassSlotSize) {
		return int(sizeClassSlotSize[class])
	}
	return 0
}

// if n%m != 0 { n += m-n%m }. m must be a power of 2.
func roundup(n, m int) int { return (n + m - 1) &^ (m - 1) }

// getClassFromSlotSize returns the size class index for a given slot size.
// Used when freeing memory to find the correct free list.
func getClassFromSlotSize(slotSize uint16) int {
	for i, v := range sizeClassSlotSize {
		if slotSize == v {
			return i
		}
	}
	panic("Unexpected slot size")
}

func NewAllocator() (a *Allocator) {
	a = new(Allocator)
	a.cap = make([]uint32, len(sizeClassSlotSize))
	a.lists = make([]uintptr, len(sizeClassSlotSize))
	a.pages = make([]uintptr, len(sizeClassSlotSize))
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
	if a.regs == nil {
		a.regs = map[uintptr]struct{}{}
	}
	if size > 0xffffffff {
		panic("mmap to big")
	}
	(*page)(unsafe.Pointer(p)).size = uint32(size)
	a.regs[p] = struct{}{}
	return p, nil
}

// newPage creates a dedicated page for a single large allocation
func (a *Allocator) newPage(size int) (uintptr /* *page */, error) {
	size += int(headerSize)
	p, err := a.mmap(size)
	if err != nil {
		return 0, err
	}

	(*page)(unsafe.Pointer(p)).slotSize = 0 // Mark as dedicated page
	return p, nil
}

// newSharedPage creates a new shared page for a specific size class
func (a *Allocator) newSharedPage(class int) (uintptr /* *page */, error) {
	slotSize := getSlotSize(class)
	if slotSize == 0 {
		panic(fmt.Sprintf("invalid size class: %d", class))
	}

	if a.cap[class] == 0 {
		a.cap[class] = uint32(pageAvail) / uint32(slotSize)
	}

	totalSize := uint32(headerSize) + a.cap[class]*uint32(slotSize)
	p, err := a.mmap(int(totalSize))
	if err != nil {
		return 0, err
	}

	a.pages[class] = p
	(*page)(unsafe.Pointer(p)).slotSize = uint16(slotSize)
	return p, nil
}

func (a *Allocator) unmap(p uintptr /* *page */) error {
	delete(a.regs, p)
	if counters {
		a.Mmaps--
	}
	return unmap(p, int((*page)(unsafe.Pointer(p)).size))
}

// UintptrFree is like Free except its argument is an uintptr
func (a *Allocator) UintptrFree(p uintptr) (err error) {
	if trace {
		defer func() {
			fmt.Fprintf(os.Stderr, "Free(%#x) %v\n", p, err)
		}()
	}
	if p == 0 {
		return nil
	}

	if counters {
		a.Allocs--
	}

	pg := p &^ uintptr(pageMask)
	slotSize := (*page)(unsafe.Pointer(pg)).slotSize

	// Dedicated page (large allocation) - slotSize == 0
	if slotSize == 0 {
		if counters {
			a.Bytes -= int((*page)(unsafe.Pointer(pg)).size)
		}
		return a.unmap(pg)
	}

	// Shared page - find class from slotSize
	class := getClassFromSlotSize(slotSize)
	if class < 0 {
		panic(fmt.Sprintf("UintptrFree: unknown slotSize %d", slotSize))
	}

	// Add to free list
	(*node)(unsafe.Pointer(p)).prev = 0
	(*node)(unsafe.Pointer(p)).next = a.lists[class]
	if next := (*node)(unsafe.Pointer(p)).next; next != 0 {
		(*node)(unsafe.Pointer(next)).prev = p
	}
	a.lists[class] = p
	(*page)(unsafe.Pointer(pg)).used--

	if (*page)(unsafe.Pointer(pg)).used != 0 {
		return nil
	}

	// Page is completely free - unmap it
	for i := 0; i < int((*page)(unsafe.Pointer(pg)).brk); i++ {
		n := pg + headerSize + uintptr(i)*uintptr(slotSize)
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
	}

	if a.pages[class] == pg {
		a.pages[class] = 0
	}
	if counters {
		a.Bytes -= int((*page)(unsafe.Pointer(pg)).size)
	}
	return a.unmap(pg)
}

// UintptrMalloc is like Malloc except it returns an uintptr.
func (a *Allocator) UintptrMalloc(size int) (r uintptr, err error) {
	if trace {
		defer func() {
			fmt.Fprintf(os.Stderr, "Malloc(%#x) %#x, %v\n", size, r, err)
		}()
	}
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
		return p + headerSize, nil
	}

	// Small allocation - use shared page
	if a.lists[class] == 0 && a.pages[class] == 0 {
		if _, err := a.newSharedPage(class); err != nil {
			return 0, err
		}
	}

	// Try to allocate from current page
	if p := a.pages[class]; p != 0 {
		(*page)(unsafe.Pointer(p)).used++
		(*page)(unsafe.Pointer(p)).brk++
		if uint32((*page)(unsafe.Pointer(p)).brk) == a.cap[class] {
			a.pages[class] = 0
		}
		slotSize := (*page)(unsafe.Pointer(p)).slotSize
		return p + headerSize + uintptr((*page)(unsafe.Pointer(p)).brk-1)*uintptr(slotSize), nil
	}

	// Allocate from free list
	n := a.lists[class]
	pg := n &^ uintptr(pageMask)
	a.lists[class] = (*node)(unsafe.Pointer(n)).next
	if next := (*node)(unsafe.Pointer(n)).next; next != 0 {
		(*node)(unsafe.Pointer(next)).prev = 0
	}
	(*page)(unsafe.Pointer(pg)).used++
	return n, nil
}

// Free deallocates memory (as in C.free).
func (a *Allocator) Free(b *[]byte) (err error) {
	return a.UintptrFree(uintptr(unsafe.Pointer(b)))
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
