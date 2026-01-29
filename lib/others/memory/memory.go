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
	headerSize      = unsafe.Sizeof(page_header{})
	dedicHdrSize    = unsafe.Sizeof(page_header_common{})
	pageAvail       = pageSize - headerSize
	pageMask        = pageSize - 1
	pageSize        = 1 << pageSizeLog
	sizeAdjustement = 8 // part of the UTXO record may be stored as map's key and need to be substracted
)

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = []uint32{
	72, 80, 96, 104, 112, 120, 128, 136, 152, 160, 168, 184, 200, 216, 240, 272, 288, 320, 368, 400, 432, 512, 576, 640, 704, 768, 896, 1024, 1216, 1408, 1728, 2048, 2304, 2560, 2944, 3200, 3712, 4096, 5120, 6144, 6912, 7936, 9216, 10240, 11264, 13312, 15360, 17408, 21504, 28672, 32768,
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
	brk  uint32 // it would be enough to have it 16-bits, but we
	used uint32 // want the header size to be multiple of 8 bytes
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
}

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

	(*page_header)(unsafe.Pointer(p)).class = -1 // Mark as dedicated page
	return p, nil
}

// newSharedPage creates a new shared page for a specific size class
func (a *Allocator) newSharedPage(class int) (uintptr /* *page */, error) {
	slotSize := getSlotSize(class)
	if slotSize == 0 {
		panic(fmt.Sprintf("invalid size class: %d", class))
	}

	totalSize := uint32(headerSize) + a.cap[class]*slotSize
	p, err := a.mmap(int(totalSize))
	if err != nil {
		return 0, err
	}

	a.pages[class] = p
	(*page_header)(unsafe.Pointer(p)).class = int16(class)
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

	pg := p &^ uintptr(pageMask)
	class := int((*page_header)(unsafe.Pointer(pg)).class)

	// Dedicated page (large allocation) - slotSize == 0
	if class < 0 {
		if counters {
			a.Bytes -= int((*page_header)(unsafe.Pointer(pg)).siz)
		}
		return a.unmap(pg)
	}

	// Shared page - Add to free list
	if (*page_header)(unsafe.Pointer(pg)).used >= 1 {
		(*node)(unsafe.Pointer(p)).prev = 0
		if next := a.lists[class]; next != 0 {
			(*node)(unsafe.Pointer(p)).next = next
			(*node)(unsafe.Pointer(next)).prev = p
		} else {
			(*node)(unsafe.Pointer(p)).next = a.lists[class]
		}
		a.lists[class] = p
		(*page_header)(unsafe.Pointer(pg)).used--
		return nil
	}

	// Page is completely free - unmap it
	slotSize := sizeClassSlotSize[class]
	n := pg + headerSize
	bi := (*page_header)(unsafe.Pointer(pg)).brk
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

	/*
		for i := 0; i < int((*page_header)(unsafe.Pointer(pg)).brk); i++ {
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
	*/

	if a.pages[class] == pg {
		a.pages[class] = 0
	}
	if counters {
		a.Bytes -= int((*page_header)(unsafe.Pointer(pg)).siz)
	}
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

	// Small allocation - use shared page
	if a.lists[class] == 0 && a.pages[class] == 0 {
		if _, err := a.newSharedPage(class); err != nil {
			return 0, err
		}
	}

	// Try to allocate from current page
	if p := a.pages[class]; p != 0 {
		(*page_header)(unsafe.Pointer(p)).used++
		(*page_header)(unsafe.Pointer(p)).brk++
		if int((*page_header)(unsafe.Pointer(p)).brk) == int(a.cap[class]) {
			a.pages[class] = 0
		}
		slotSize := sizeClassSlotSize[class]
		return p + headerSize + uintptr((*page_header)(unsafe.Pointer(p)).brk-1)*uintptr(slotSize), nil
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

func NewAllocator() (a *Allocator) {
	a = new(Allocator)
	a.cap = make([]uint32, len(sizeClassSlotSize))
	a.lists = make([]uintptr, len(sizeClassSlotSize))
	a.pages = make([]uintptr, len(sizeClassSlotSize))

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
	if pageSizeLog == 16 {
		// for Windows 64KB page sizes
		for mx := len(sizeClassSlotSize); mx > 0; mx-- {
			if sizeClassSlotSize[mx-1] >= 4092-sizeAdjustement {
				sizeClassSlotSize = sizeClassSlotSize[:mx]
				break
			}
		}
	}
	println("memory: page_header len is", unsafe.Sizeof(page_header{}), len(sizeClassSlotSize))
	print("slot sizes: ")
	for _, ss := range sizeClassSlotSize {
		print(ss, ", ")
	}
	println("\nnumber of slots:", len(sizeClassSlotSize))
	for i := range sizeClassSlotSize {
		sizeClassSlotSize[i] -= sizeAdjustement
	}
}
