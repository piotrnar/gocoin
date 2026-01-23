// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.
// MODIFIED: Optimized for UTXO workloads with custom size classes.

package memory // import "modernc.org/memory"

import (
	"fmt"
	"os"
	"unsafe"
)

const (
	headerSize   = unsafe.Sizeof(page{})
	mallocAllign = 2 * unsafe.Sizeof(uintptr(0))
	pageAvail    = pageSize - headerSize
	pageMask     = pageSize - 1
	pageSize     = 1 << pageSizeLog
)

// Custom size classes optimized for UTXO allocation patterns:
// Based on statistics: 69 bytes (32.3%), 57 bytes (24.8%), 55 bytes (15.3%), 59 bytes (10.2%)
//
// Size classes: 16, 32, 48, 64, 72, 80, 88, 96, 104, 112, 128, 256, 512, ...
// Class indices: 0,  1,  2,  3,  4,  5,  6,  7,   8,   9,  10,  11,  12, ...
const numSizeClasses = 32

// sizeClassSlotSize maps class index -> actual slot size in bytes
var sizeClassSlotSize = [numSizeClasses]int{
	0:  16,
	1:  32,
	2:  48,
	3:  64,
	4:  72, // Optimized for 69-byte allocations (36.8M records)
	5:  80,
	6:  88,
	7:  96,
	8:  104,
	9:  112,
	10: 128,
	11: 248,   // 264 slots per 64KB page, 32 bytes waste
	12: 496,   // 132 slots per 64KB page, 32 bytes waste
	13: 992,   // 66 slots per 64KB page, 32 bytes waste
	14: 1984,  // 33 slots per 64KB page, 32 bytes waste
	15: 4088,  // 16 slots per 64KB page (8-byte aligned)
	16: 8184,  // 8 slots per 64KB page (8-byte aligned)
	17: 16376, // 4 slots per 64KB page
	18: 32752, // 2 slots per 64KB page
	// Classes 19+ not used - allocations > 32KB use dedicated pages
}

// getSizeClass returns the size class index for a given allocation size.
// This is the core routing function that determines which slot size to use.
func getSizeClass(size int) int {
	// Fast path for common UTXO sizes (65-128 byte range)
	// Check raw size to ensure 69-byte records get 72-byte slots
	if size >= 65 && size <= 128 {
		switch {
		case size <= 72:
			return 4 // 72 bytes
		case size <= 80:
			return 5 // 80 bytes
		case size <= 88:
			return 6 // 88 bytes
		case size <= 96:
			return 7 // 96 bytes
		case size <= 104:
			return 8 // 104 bytes
		case size <= 112:
			return 9 // 112 bytes
		default:
			return 10 // 128 bytes
		}
	}

	// For sizes outside UTXO hot range, align to 16 bytes
	alignedSize := (size + int(mallocAllign) - 1) &^ (int(mallocAllign) - 1)

	switch {
	case alignedSize <= 16:
		return 0
	case alignedSize <= 32:
		return 1
	case alignedSize <= 48:
		return 2
	case alignedSize <= 64:
		return 3
	case alignedSize <= 248:
		return 11
	case alignedSize <= 496:
		return 12
	case alignedSize <= 992:
		return 13
	case alignedSize <= 1984:
		return 14
	case alignedSize <= 4088:
		return 15
	case alignedSize <= 8184:
		return 16
	case alignedSize <= 16376:
		return 17
	case alignedSize <= 32752:
		return 18
	default:
		return -1
	}
}

// getSlotSize returns the actual slot size for a size class index
func getSlotSize(class int) int {
	if class >= 0 && class < numSizeClasses {
		return sizeClassSlotSize[class]
	}
	return 0
}

func init() {
	if unsafe.Sizeof(page{})%mallocAllign != 0 {
		panic("internal error")
	}
}

// if n%m != 0 { n += m-n%m }. m must be a power of 2.
func roundup(n, m int) int { return (n + m - 1) &^ (m - 1) }

type node struct {
	prev, next uintptr // *node
}

type page struct {
	brk      int
	slotSize int // Actual slot size in bytes. 0 = dedicated page (large allocation)
	size     int // Total page size from mmap
	used     int
}

// getClassFromSlotSize returns the size class index for a given slot size.
// Used when freeing memory to find the correct free list.
func getClassFromSlotSize(slotSize int) int {
	switch slotSize {
	case 16:
		return 0
	case 32:
		return 1
	case 48:
		return 2
	case 64:
		return 3
	case 72:
		return 4
	case 80:
		return 5
	case 88:
		return 6
	case 96:
		return 7
	case 104:
		return 8
	case 112:
		return 9
	case 128:
		return 10
	case 248:
		return 11
	case 496:
		return 12
	case 992:
		return 13
	case 1984:
		return 14
	case 4088:
		return 15
	case 8184:
		return 16
	case 16376:
		return 17
	case 32752:
		return 18
	default:
		return -1 // Dedicated page or unknown
	}
}

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	Allocs int // # of allocs.
	Bytes  int // Asked from OS.
	cap    [numSizeClasses]int
	lists  [numSizeClasses]uintptr // *node - free lists per size class
	Mmaps  int                     // Asked from OS.
	pages  [numSizeClasses]uintptr // *page - current page per size class
	regs   map[uintptr]struct{}    // map[*page]struct{} - all registered pages
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
	(*page)(unsafe.Pointer(p)).size = size
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
		a.cap[class] = int(pageAvail) / slotSize
	}

	totalSize := int(headerSize) + a.cap[class]*slotSize
	p, err := a.mmap(totalSize)
	if err != nil {
		return 0, err
	}

	a.pages[class] = p
	(*page)(unsafe.Pointer(p)).slotSize = slotSize
	return p, nil
}

func (a *Allocator) unmap(p uintptr /* *page */) error {
	delete(a.regs, p)
	if counters {
		a.Mmaps--
	}
	return unmap(p, (*page)(unsafe.Pointer(p)).size)
}

// UintptrCalloc is like Calloc except it returns an uintptr.
func (a *Allocator) UintptrCalloc(size int) (r uintptr, err error) {
	if trace {
		defer func() {
			fmt.Fprintf(os.Stderr, "Calloc(%#x) %#x, %v\n", size, r, err)
		}()
	}
	if r, err = a.UintptrMalloc(size); r == 0 || err != nil {
		return 0, err
	}
	b := ((*rawmem)(unsafe.Pointer(r)))[:size:size]
	for i := range b {
		b[i] = 0
	}
	return r, nil
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
			a.Bytes -= (*page)(unsafe.Pointer(pg)).size
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
	for i := 0; i < (*page)(unsafe.Pointer(pg)).brk; i++ {
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
		a.Bytes -= (*page)(unsafe.Pointer(pg)).size
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
		if (*page)(unsafe.Pointer(p)).brk == a.cap[class] {
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

// UintptrRealloc is like Realloc except its first argument is an uintptr
func (a *Allocator) UintptrRealloc(p uintptr, size int) (r uintptr, err error) {
	if trace {
		defer func() {
			fmt.Fprintf(os.Stderr, "UnsafeRealloc(%#x, %#x) %#x, %v\n", p, size, r, err)
		}()
	}
	switch {
	case p == 0:
		return a.UintptrMalloc(size)
	case size == 0 && p != 0:
		return 0, a.UintptrFree(p)
	}

	us := UintptrUsableSize(p)
	if us >= size {
		return p, nil
	}

	if r, err = a.UintptrMalloc(size); err != nil {
		return 0, err
	}

	if us < size {
		size = us
	}
	copy((*rawmem)(unsafe.Pointer(r))[:size:size], (*rawmem)(unsafe.Pointer(p))[:size:size])
	return r, a.UintptrFree(p)
}

// UintptrUsableSize returns the usable size of an allocation
func UintptrUsableSize(p uintptr) (r int) {
	if trace {
		defer func() {
			fmt.Fprintf(os.Stderr, "UsableSize(%#x) %#x\n", p, r)
		}()
	}
	if p == 0 {
		return 0
	}

	return usableSize(p)
}

func usableSize(p uintptr) (r int) {
	pg := p &^ uintptr(pageMask)
	slotSize := (*page)(unsafe.Pointer(pg)).slotSize

	// Dedicated page - slotSize == 0
	if slotSize == 0 {
		return (*page)(unsafe.Pointer(pg)).size - int(headerSize)
	}

	// Shared page - return the stored slot size
	return slotSize
}

// Calloc is like Malloc except the allocated memory is zeroed.
func (a *Allocator) Calloc(size int) (r []byte, err error) {
	p, err := a.UintptrCalloc(size)
	if err != nil {
		return nil, err
	}

	b := unsafe.Slice((*byte)(unsafe.Pointer(p)), usableSize(p))
	return b[:size], nil
}

// Close releases all OS resources used by a and sets it to its zero value.
func (a *Allocator) Close() (err error) {
	for p := range a.regs {
		if e := a.unmap(p); e != nil && err == nil {
			err = e
		}
	}
	*a = Allocator{}
	return err
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

	r = unsafe.Slice((*byte)(unsafe.Pointer(p)), usableSize(p))
	return r[:size], nil
}

// Realloc changes the size of the backing array of b to size bytes.
func (a *Allocator) Realloc(b []byte, size int) (r []byte, err error) {
	var p uintptr
	if b = b[:cap(b)]; len(b) != 0 {
		p = uintptr(unsafe.Pointer(&b[0]))
	}
	if p, err = a.UintptrRealloc(p, size); p == 0 || err != nil {
		return nil, err
	}

	r = unsafe.Slice((*byte)(unsafe.Pointer(p)), usableSize(p))
	return r[:size], nil
}

// UsableSize reports the size of the memory block allocated at p.
func UsableSize(p *byte) (r int) { return UintptrUsableSize(uintptr(unsafe.Pointer(p))) }

// UnsafeCalloc is like Calloc except it returns an unsafe.Pointer.
func (a *Allocator) UnsafeCalloc(size int) (r unsafe.Pointer, err error) {
	p, err := a.UintptrCalloc(size)
	if err != nil {
		return nil, err
	}

	return unsafe.Pointer(p), nil
}

// UnsafeFree is like Free except its argument is an unsafe.Pointer.
func (a *Allocator) UnsafeFree(p unsafe.Pointer) (err error) { return a.UintptrFree(uintptr(p)) }

// UnsafeMalloc is like Malloc except it returns an unsafe.Pointer.
func (a *Allocator) UnsafeMalloc(size int) (r unsafe.Pointer, err error) {
	p, err := a.UintptrMalloc(size)
	if err != nil {
		return nil, err
	}

	return unsafe.Pointer(p), nil
}

// UnsafeRealloc is like Realloc except its first argument is an unsafe.Pointer.
func (a *Allocator) UnsafeRealloc(p unsafe.Pointer, size int) (r unsafe.Pointer, err error) {
	q, err := a.UintptrRealloc(uintptr(p), size)
	if err != nil {
		return nil, err
	}

	return unsafe.Pointer(q), nil
}

// UnsafeUsableSize is like UsableSize except its argument is an unsafe.Pointer.
func UnsafeUsableSize(p unsafe.Pointer) (r int) { return UintptrUsableSize(uintptr(p)) }
