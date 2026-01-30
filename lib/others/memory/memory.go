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
	if a.firstPage[class] == nil {
		if a.lastPage[class] != nil {
			panic("lastPage not nil but expected")
		}
		a.firstPage[class] = new_page
	} else {
		// we already have some pages, so just add it at the end
		a.lastPage[class].next = new_page
		new_page.prev = a.lastPage[class]
	}
	a.lastPage[class] = new_page
	new_page.cap = uint16(cap) // it's redundant, but we have a spare 16 bits
	new_page.class = int16(class)
	new_page.seq = a.currentSequence
	a.currentSequence++

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
	// Add to page's free list (insert at head)
	(*free_record)(unsafe.Pointer(p)).next_free_record = addOffset(uintptr(unsafe.Pointer(&page2free.cap)), page2free.freeListOffs)
	page2free.updateFreeList(p)
	page2free.used--

	// Update freePage if this page has lower sequence than current freePage
	if a.freePage[class] == nil || page2free.seq < a.freePage[class].seq {
		a.freePage[class] = page2free
	}

	if page2free.used != 0 {
		return nil
	}

	// Page is completely free - unmap it

	// If we're removing freePage, find the next best page starting from pag.next
	if a.freePage[class] == page2free {
		a.freePage[class] = nil
		for pg := page2free.next; pg != nil; pg = pg.next {
			if pg.freeListOffs != 0 {
				a.freePage[class] = pg
				break // pages are in sequence order, first one found is the lowest
			}
		}
	}

	// Remove from linked list
	if page2free.prev != nil {
		page2free.prev.next = page2free.next
	} else {
		if a.firstPage[class] != page2free {
			panic("a.firstPage != page2free")
		}
		a.firstPage[class] = page2free.next // this was the first page - set the next one
	}
	if page2free.next != nil {
		page2free.next.prev = page2free.prev
	} else {
		if a.lastPage[class] != page2free {
			panic("a.lastPage != page2free")
		}
		a.lastPage[class] = page2free.prev // this was the last page - set the previous one
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
	if a.freePage[class] == nil && a.pages[class] == 0 {
		if _, err := a.newSharedPage(class); err != nil {
			return 0, err
		}
	}

	// Try to allocate from current page
	// First use all the records from the most recently allocated page, before reuing freed slots
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

	// Allocate from freePage's free list (remove from head)
	p := a.freePage[class]
	var n uintptr
	if p.freeListOffs != 0 {
		n = addOffset(uintptr(unsafe.Pointer(&p.cap)), p.freeListOffs)
		p.updateFreeList((*free_record)(unsafe.Pointer(n)).next_free_record)
	} else {
		if p.dirty < p.cap {
			n = uintptr(unsafe.Pointer(p)) + headerSize + uintptr(p.dirty)*uintptr(getSlotSize(class))
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

func (a *Allocator) DefragAll() (res []uintptr) {
	for i := range sizeClassSlotSize {
		recs := a.Defrag(i)
		if len(recs) > 0 {
			res = append(res, recs...)
		}
	}
	return
}

func (a *Allocator) Defrag(class int) (res []uintptr) {
	const keep_pages = 10
	free_pages := int(a.fcnt[class] / a.Capacity[class])
	if free_pages > keep_pages {
		r2free := (int(free_pages) - keep_pages) * int(a.Capacity[class])
		//println("Class", class, "will try to re-allocate", r2free, "records:", a.fcnt[class], a.pcnt[class], a.Capacity[class])
		res = make([]uintptr, 0, r2free+int(a.Capacity[class])-1)
		page := a.firstPage[class]
		slot_size := uintptr(sizeClassSlotSize[class])
		for len(res) < r2free {
			if page.used == 0 {
				panic("no used records")
			}
			if page.used < page.cap {
				if page.freeListOffs == 0 {
					panic("page.freeListOffs is zero, but should not")
				}

				//println(" .. page", page.seq, "has", page.used, "/", page.cap, "used record")
				freed := make(map[uintptr]struct{}, page.cap-page.used)

				// first let's build map of all the records inside the page
				rec := addOffset(uintptr(unsafe.Pointer(&page.cap)), page.freeListOffs)
				for rec != 0 {
					freed[rec] = struct{}{}
					frec := (*free_record)(unsafe.Pointer(rec))
					rec = uintptr(unsafe.Pointer(frec))
					rec = frec.next_free_record
				}
				//println(" ... freed map_size:", len(freed))

				// now go through all the records and add the non-free ones to the list
				rec = addOffset(uintptr(unsafe.Pointer(&page.cap)), uint32(headerSize))
				for cnt := 0; cnt < int(page.used); {
					if _, free := freed[rec]; !free {
						res = append(res, rec)
						cnt++
					}
					rec += slot_size
				}
				if len(res) == 0 {
					panic("no records found")
				}
			}
			//println(" ...", len(res), "after page seq", page.seq)
			page = page.next
			if page == nil {
				println("class:", class, "  free_pages:", free_pages,
					"  r2free:", r2free, "  sofar:", len(res), "  cap:", a.Capacity[class])

				var tot int
				for page = a.firstPage[class]; page != nil; page = page.next {
					tot += int(page.cap - page.used)
					println(" - page", page.seq, "- used / cap / dirty:", page.used, page.cap, page.dirty,
						"  sofar:", tot)
				}
				panic("reached last page but not enought records")
			}
		}
		if len(res) < r2free {
			panic("did not gather enough")
		}
		//println("Class", class, "re-allocate", len(res), "records!")
	}
	return
}

func (a *Allocator) PrintStats() {
	var to_save int
	for class := range a.fcnt {
		var ts int
		if a.fcnt[class] > a.Capacity[class] {
			ts = int(a.fcnt[class] / a.Capacity[class])
		}
		fmt.Printf("%3d) ... %5d: %10d (%3d%%) free slots in %6d pgs - %6.2f pages can be free: %5d\n",
			class, sizeClassSlotSize[class], a.fcnt[class], 100*a.fcnt[class]/(a.pcnt[class]*a.Capacity[class]),
			a.pcnt[class], float64(a.fcnt[class])/float64(a.Capacity[class]), ts)
		to_save += ts
	}
	fmt.Println("Total to save if all defragmented:", (to_save*pageSize)>>20, "MB")
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
