package memory

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	defragFromWasteMB = 12 // When number of free slots exceeds this many MB ...
	defragToWasteMB   = 4  // ... defragment until it falls below this many MB.

	minFreePagesFrom = (defragFromWasteMB << 20) / pageSize
	minFreePagesTo   = (defragToWasteMB << 20) / pageSize
)

var trace bool

func (a *Allocator) Trace(on bool) {
	trace = on
}

// DefragAllImproved defragments all classes in parallel.
// Each class has independent page lists and free lists, so they can be
// defragmented concurrently. Only the shared counters (Bytes, Allocs,
// SharedMmaps) need atomic access.
func (a *Allocator) DefragAllImproved(relocate func(oldslice, newslice *[]byte)) (cnt int) {
	var wg sync.WaitGroup
	var totalCnt atomic.Int64
	var totalBytes atomic.Int64
	var totalMmaps atomic.Int64

	for class := range sizeClassSlotSize {
		cap := int(a.cap[class])

		// Quick check using counters - O(1)
		potentialFreePages := int(a.freeSlots[class]) / cap
		if potentialFreePages <= minFreePagesFrom {
			continue
		}

		wg.Add(1)
		go func(class int) {
			defer wg.Done()
			c, b, m := a.defragClass(class, relocate)
			totalCnt.Add(int64(c))
			totalBytes.Add(int64(b))
			totalMmaps.Add(int64(m))
		}(class)
	}

	wg.Wait()

	// Apply accumulated counter deltas
	a.Bytes.Add(totalBytes.Load())
	a.SharedMmaps.Add(totalMmaps.Load())

	return int(totalCnt.Load())
}

// defragClass performs defragmentation for a specific size class.
// It does NOT modify a.Bytes, a.Allocs, or a.SharedMmaps directly.
// Instead it returns the deltas for these counters.
// All per-class fields (lists, pages, firstPage, lastPage, pageCount, freeSlots, cap)
// are safe to access without synchronization because each class is processed by
// exactly one goroutine.
func (a *Allocator) defragClass(class int, relocate func(oldslice, newslice *[]byte)) (cnt, deltaBytes, deltaMmaps int) {
	cap := int(a.cap[class])

	potentialFreePages := int(a.freeSlots[class]) / cap

	// Build an array of non fully used pages
	pages := make([]uintptr, 0, a.pageCount[class])
	for pg := a.firstPage[class]; pg != 0; pg = (*page_header)(unsafe.Pointer(pg)).next {
		if int((*page_header)(unsafe.Pointer(pg)).used) < cap {
			pages = append(pages, pg)
		}
	}

	if len(pages) == 0 {
		println("ERROR: Unexpected empty pageMap for class", class)
		return
	}

	// Sort pages by utilization (lowest first)
	sort.Slice(pages, func(i, j int) bool {
		page_i := (*page_header)(unsafe.Pointer(pages[i]))
		page_j := (*page_header)(unsafe.Pointer(pages[j]))
		return page_i.used < page_j.used
	})

	// Mark all pages as evacuating
	type evacInfo struct {
		pg        uintptr
		freeSlots map[uintptr]bool
	}

	// Select pages to evacuate
	targetFreedRecords := cap * (potentialFreePages - minFreePagesTo)
	var recordsToMove, recordsToFree int
	pagesToEvacuate := make([]uintptr, 0, len(pages))

	if trace {
		fmt.Printf("Defragment class %d,  cap %d  -  non full pages: %d,  target free recs: %d\n",
			class, cap, len(pages), targetFreedRecords)
	}
	for i := range pages {
		page_i_used := int((*page_header)(unsafe.Pointer(pages[i])).used)
		pagesToEvacuate = append(pagesToEvacuate, pages[i])
		recordsToFree += cap - int(page_i_used)
		recordsToMove += int(page_i_used)
		freedPages := recordsToFree / cap
		if trace {
			fmt.Printf(" + page with %d used records => total %d  / fred:%d = (%d + %d) / %d\n",
				page_i_used, recordsToMove, freedPages, a.freeSlots[class], recordsToMove, cap)
		}
		if recordsToFree >= targetFreedRecords {
			if trace {
				fmt.Println(" * enough to free:", recordsToFree, "/", targetFreedRecords, " -  to move:", recordsToMove)
			}
			break
		}
	}

	if recordsToMove == 0 {
		if trace {
			fmt.Printf("No records to move\n")
		}
		return
	}

	if trace {
		fmt.Printf("Defrag class %d: relocating %d records from %d pages (target: free %d records)\n",
			class, recordsToMove, len(pagesToEvacuate), targetFreedRecords)
	}

	freeSlotsArr := make([]map[uintptr]struct{}, 0, len(pagesToEvacuate))
	for _, pg := range pagesToEvacuate {
		header := (*page_header)(unsafe.Pointer(pg))
		header.evacuating = true
		if a.pages[class] == pg {
			a.pages[class] = 0
		}

		freeSlots := make(map[uintptr]struct{}, header.free)
		for n := header.freeList; n != 0; n = (*node)(unsafe.Pointer(n)).nextInPage {
			freeSlots[n] = struct{}{}
		}
		// Remove these slots from global free list
		for n := header.freeList; n != 0; {
			nextInPage := (*node)(unsafe.Pointer(n)).nextInPage
			next := (*node)(unsafe.Pointer(n)).next
			prev := (*node)(unsafe.Pointer(n)).prev
			if prev == 0 {
				a.lists[class] = next
				if next != 0 {
					(*node)(unsafe.Pointer(next)).prev = 0
				}
			} else {
				(*node)(unsafe.Pointer(prev)).next = next
				if next != 0 {
					(*node)(unsafe.Pointer(next)).prev = prev
				}
			}
			n = nextInPage
		}
		freeSlotsArr = append(freeSlotsArr, freeSlots)
		header.freeList = 0
	}

	// Evacuate all pages
	slotSize := uintptr(sizeClassSlotSize[class])
	for idx, pg := range pagesToEvacuate {
		slotAddr := pg + headerSize
		for left := int((*page_header)(unsafe.Pointer(pg)).brk); left > 0; left-- {
			if _, ok := freeSlotsArr[idx][slotAddr]; !ok {
				// This is a used slot - relocate it
				newAddr, newPage, err := a.classMalloc(class)
				if err != nil {
					panic(fmt.Sprintf("Failed to allocate during defrag: %v", err))
				}
				if newPage {
					deltaBytes += pageSize
					deltaMmaps++
				}
				// Copy data
				oldSlice := (*reflect.SliceHeader)(unsafe.Pointer(slotAddr))
				os := (*[]byte)(unsafe.Pointer(oldSlice))
				newSlice := (*reflect.SliceHeader)(unsafe.Pointer(newAddr))
				ns := (*[]byte)(unsafe.Pointer(newSlice))
				newSlice.Data = uintptr(newAddr + 24)
				newSlice.Len = oldSlice.Len
				newSlice.Cap = oldSlice.Cap
				copy(*ns, *os)
				relocate(os, ns)
				cnt++
				// Free old slot (page is evacuating, so slot won't re-enter free list)
				a.classFree(class, slotAddr)
			}
			slotAddr += slotSize
		}

		// Unmap evacuated page
		header := (*page_header)(unsafe.Pointer(pg))
		header.evacuating = false
		// Remove from page linked list
		if header.prev != 0 {
			(*page_header)(unsafe.Pointer(header.prev)).next = header.next
		} else {
			a.firstPage[class] = header.next
		}
		if header.next != 0 {
			(*page_header)(unsafe.Pointer(header.next)).prev = header.prev
		} else {
			a.lastPage[class] = header.prev
		}
		// Update per-class counters
		a.pageCount[class]--
		a.freeSlots[class] -= uint32(header.free)
		if a.pages[class] == pg {
			a.pages[class] = 0
		}
		// Accumulate shared counter deltas
		deltaBytes -= pageSize
		deltaMmaps--
		unmap(pg, pageSize)
	}
	if trace {
		fmt.Printf("Defrag class %d: relocated %d records, freed %d pages\n",
			class, cnt, len(pagesToEvacuate))
	}
	return
}

// classMalloc allocates a slot within a specific class without touching shared counters.
// Returns the pointer, whether a new page was allocated, and any error.
func (a *Allocator) classMalloc(class int) (r uintptr, newPage bool, err error) {
	if a.lists[class] == 0 && a.pages[class] == 0 {
		if _, err := a.newSharedPageLocal(class); err != nil {
			return 0, false, err
		}
		newPage = true
	}

	// Try to allocate from current page
	if p := a.pages[class]; p != 0 {
		header := (*page_header)(unsafe.Pointer(p))
		header.used++
		header.brk++
		header.free--
		a.freeSlots[class]--

		if int(header.brk) == int(a.cap[class]) {
			a.pages[class] = 0
		}
		slotSize := int(sizeClassSlotSize[class])
		ptr := p + headerSize + uintptr(header.brk-1)*uintptr(slotSize)
		return ptr, newPage, nil
	}

	// Allocate from free list
	n := a.lists[class]
	pg := n &^ uintptr(pageMask)
	header := (*page_header)(unsafe.Pointer(pg))

	// Remove from global free list
	a.lists[class] = (*node)(unsafe.Pointer(n)).next
	if next := (*node)(unsafe.Pointer(n)).next; next != 0 {
		(*node)(unsafe.Pointer(next)).prev = 0
	}

	// Remove from per-page free list
	nextInPage := (*node)(unsafe.Pointer(n)).nextInPage
	prevInPage := (*node)(unsafe.Pointer(n)).prevInPage

	if prevInPage == 0 {
		header.freeList = nextInPage
		if nextInPage != 0 {
			(*node)(unsafe.Pointer(nextInPage)).prevInPage = 0
		}
	} else {
		(*node)(unsafe.Pointer(prevInPage)).nextInPage = nextInPage
		if nextInPage != 0 {
			(*node)(unsafe.Pointer(nextInPage)).prevInPage = prevInPage
		}
	}

	header.used++
	header.free--
	a.freeSlots[class]--
	return n, newPage, nil
}

// classFree frees a slot within a specific class without touching shared counters.
func (a *Allocator) classFree(class int, p uintptr) {
	pg := p &^ uintptr(pageMask)
	header := (*page_header)(unsafe.Pointer(pg))

	header.used--
	header.free++
	a.freeSlots[class]++

	// If page is being evacuated, don't add to free list
	if header.evacuating {
		return
	}

	// Add to global free list
	(*node)(unsafe.Pointer(p)).prev = 0
	if next := a.lists[class]; next != 0 {
		(*node)(unsafe.Pointer(p)).next = next
		(*node)(unsafe.Pointer(next)).prev = p
	} else {
		(*node)(unsafe.Pointer(p)).next = 0
	}
	a.lists[class] = p

	// Add to per-page free list
	(*node)(unsafe.Pointer(p)).prevInPage = 0
	if nextInPage := header.freeList; nextInPage != 0 {
		(*node)(unsafe.Pointer(p)).nextInPage = nextInPage
		(*node)(unsafe.Pointer(nextInPage)).prevInPage = p
	} else {
		(*node)(unsafe.Pointer(p)).nextInPage = 0
	}
	header.freeList = p
}

// newSharedPageLocal creates a new shared page without updating shared counters.
func (a *Allocator) newSharedPageLocal(class int) (uintptr, error) {
	slotSize := getSlotSize(class)
	if slotSize == 0 {
		panic(fmt.Sprintf("invalid size class: %d", class))
	}

	p, _, err := mmap(0)
	if err != nil {
		return 0, err
	}

	header := (*page_header)(unsafe.Pointer(p))
	header.class = byte(class)
	header.free = uint16(a.cap[class])

	// Link into page list (per-class, safe without lock)
	header.prev = a.lastPage[class]
	header.next = 0

	if a.lastPage[class] != 0 {
		(*page_header)(unsafe.Pointer(a.lastPage[class])).next = p
	}
	if a.firstPage[class] == 0 {
		a.firstPage[class] = p
	}
	a.lastPage[class] = p

	a.pageCount[class]++
	a.freeSlots[class] += a.cap[class]

	a.pages[class] = p
	return p, nil
}
