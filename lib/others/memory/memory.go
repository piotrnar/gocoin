// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory implements a memory allocator.

package memory // import "modernc.org/memory"

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	headerSize   = unsafe.Sizeof(page_header{})
	pageAvail    = pageSize - headerSize
	pageMask     = pageSize - 1
	pageSize     = 1 << pageSizeLog
	sliceHdrLen  = int(unsafe.Sizeof([]byte{}))
	sizeIncrease = sliceHdrLen
)

type node struct {
	prev, next             uintptr // *node - global free list (across all pages in class)
	prevInPage, nextInPage uintptr // *node - per-page free list (only slots from same page)
}

type page_header struct {
	class      byte
	evacuating bool    // true during defragmentation to prevent freed slots from re-entering free list
	brk        uint16  // high water mark of allocated slots
	used       uint16  // number of currently used slots
	free       uint16  // number of free slots in this page (for quick defrag decisions)
	prev       uintptr // previous page in class (linked list of all pages)
	next       uintptr // next page in class (linked list of all pages)
	freeList   uintptr // head of per-page free list (using nextInPage/prevInPage pointers)
}

// Allocator allocates and frees memory. Its zero value is ready for use.
type Allocator struct {
	sync.Mutex
	Allocs        atomic.Int64 // # of allocs.
	Bytes         atomic.Int64 // Asked from OS.
	PrivateMmaps  atomic.Int64 // Asked from OS.
	SharedMmaps   atomic.Int64
	MaxSharedSize int
	ClassCont     int
	cap           []uint32
	lists         []uintptr // *node - free lists per size class
	pages         []uintptr // *page - current page per size class
	classIdx      []byte

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
	if size > a.MaxSharedSize {
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

func (a *Allocator) GetInfo(verbose bool) string {
	a.Lock()
	defer a.Unlock()
	var pcnt, scnt, fcnt int
	w := new(bytes.Buffer)
	for class := range sizeClassSlotSize {
		siz := int(sizeClassSlotSize[class])
		if verbose {
			fmt.Fprintf(w, "%3d)  siz: %-5d   cap: %-5d  pagCnt: %-6d   freeSl: %-6d   waste: %2.1f pages\n",
				class, siz, a.cap[class], a.pageCount[class], a.freeSlots[class],
				float64(a.freeSlots[class])/float64(a.cap[class]))
		}
		pcnt += int(a.pageCount[class])
		scnt += int(a.pageCount[class]) * int(a.cap[class]) * siz
		fcnt += int(a.freeSlots[class]) * siz
	}
	if !verbose {
		fmt.Fprint(w, len(sizeClassSlotSize), " slots: ")
		for _, siz := range sizeClassSlotSize {
			fmt.Fprint(w, int(siz)-int(sizeIncrease), ", ")
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Bytes: %d,  Allocs: %d,  Maps: %d sh + %d pr  MaxSize: %d\n",
		a.Bytes.Load(), a.Allocs.Load(), a.SharedMmaps.Load(), a.PrivateMmaps.Load(), a.MaxSharedSize)
	fmt.Fprintf(w, "Page Header Size: %d,   Slot Extra Size: %d,   Page Size: %d\n",
		headerSize, sizeIncrease, pageSize)
	fmt.Fprintf(w, "Classes: %d,  Total slots: %d MB,  pages: %d,   free slots: %d MB\n",
		len(sizeClassSlotSize), scnt>>20, pcnt, fcnt>>20)
	return w.String()
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

	a.ClassCont = len(sizeClassSlotSize)
	a.MaxSharedSize = int(sizeClassSlotSize[len(sizeClassSlotSize)-1])
	a.classIdx = make([]byte, a.MaxSharedSize+1)
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
	// add the slice header to each slot size
	if len(sizeClassSlotSize) > 255 {
		panic("Too many classes")
	}
	for i := range sizeClassSlotSize {
		sizeClassSlotSize[i] += uint32(sizeIncrease)
	}
}
