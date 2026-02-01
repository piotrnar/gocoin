package memory

import (
	"fmt"
	"sort"
	"unsafe"
)

const trace = false

// pageUtilization tracks utilization metrics for defragmentation
type pageUtilization struct {
	page  *page_header
	used  uint16
	cap   uint16
	ratio float32 // used/cap
}

// DefragAll performs defragmentation and returns list of newly allocated records
func (a *Allocator) DefragAll() [][]byte {
	a.DerfagClass = -1
	for class := range sizeClassSlotSize {
		relocations := a.Defrag(class)
		if len(relocations) > 0 {
			a.DerfagClass = class
			return relocations
			// TODO: continue to next class instead of returning after first
			// You could accumulate: allRelocations = append(allRelocations, relocations...)
		}
	}
	return nil
}

// Defrag performs defragmentation for a specific size class
// It allocates new pages, copies data, frees old records, and returns the new record slices
func (a *Allocator) Defrag(class int) [][]byte {
	const minFreePages = 10    // minimum free pages to keep after defrag
	const minUtilization = 0.5 // only defrag pages below 50% utilization

	cap := int(a.Capacity[class])
	freeSlots := int(a.fcnt[class])

	// Calculate how many empty pages we could have
	potentialFreePages := freeSlots / cap

	if potentialFreePages <= minFreePages {
		return nil // not worth defragmenting
	}

	// Step 1: Build list of all pages with their utilization
	var pages []pageUtilization
	for pg := a.firstPage[class]; pg != nil; pg = pg.next {
		if pg.used == 0 {
			continue // skip completely empty pages
		}
		if pg.used < pg.cap {
			ratio := float32(pg.used) / float32(pg.cap)
			pages = append(pages, pageUtilization{
				page:  pg,
				used:  pg.used,
				cap:   pg.cap,
				ratio: ratio,
			})
		}
	}

	if len(pages) == 0 {
		return nil
	}

	// Step 2: Sort by utilization (lowest first) - these are best candidates
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].ratio < pages[j].ratio
	})

	// Step 3: Determine how many records to relocate
	targetFreedPages := potentialFreePages - minFreePages

	// Step 4: Select pages to evacuate (starting with lowest utilization)
	var recordsToMove int
	var pagesToEvacuate []*page_header

	for i := range pages {
		if pages[i].ratio > minUtilization {
			break // stop at pages with decent utilization
		}

		pagesToEvacuate = append(pagesToEvacuate, pages[i].page)
		recordsToMove += int(pages[i].used)

		// Calculate how many pages we could free with current selection
		freedPages := (freeSlots + recordsToMove) / cap

		if freedPages-minFreePages >= targetFreedPages {
			break // we've selected enough
		}
	}

	if recordsToMove == 0 {
		return nil
	}

	if trace {
		fmt.Printf("Defrag class %d: will relocate %d records from %d low-utilization pages (target: free %d pages)\n",
			class, recordsToMove, len(pagesToEvacuate), targetFreedPages)
	}

	// Step 5: Perform the actual relocation
	slotSize := int(sizeClassSlotSize[class])
	relocations := make([][]byte, 0, recordsToMove)

	for _, pg := range pagesToEvacuate {
		pageRelocations := a.relocatePageRecords(pg, slotSize, class)
		relocations = append(relocations, pageRelocations...)
	}

	if len(relocations) != recordsToMove {
		panic(fmt.Sprintf("Expected %d relocations, got %d", recordsToMove, len(relocations)))
	}

	if trace {
		fmt.Printf("Defrag class %d: relocated %d records, freed pages should follow\n",
			class, len(relocations))
	}

	return relocations
}

// relocatePageRecords relocates all used records from a page to new locations
func (a *Allocator) relocatePageRecords(pg *page_header, slotSize int, class int) [][]byte {
	if pg.used == 0 {
		return nil
	}

	pgAddr := uintptr(unsafe.Pointer(pg))

	// CRITICAL: Prevent allocations into this page during evacuation
	// The allocator can allocate from this page in two ways:
	// 1. a.pages[class] - current allocation page (allocates from unallocated slots)
	// 2. a.freePage[class] - page with lowest seq that has freed slots (allocates from free list)
	// 3. During freePage search, any page with freeListOffs != 0
	//
	// Solution: Temporarily clear the page's freeListOffs so it appears to have no free slots

	savedFreePage := a.freePage[class]
	savedPages := a.pages[class]
	savedFreeListOffs := pg.freeListOffs

	// Remove from current allocation page
	if a.pages[class] == pgAddr {
		a.pages[class] = 0
	}

	// Clear the page's free list temporarily (makes it invisible to allocator)
	pg.freeListOffs = 0

	// Remove from freePage if it points to this page
	if a.freePage[class] == pg {
		a.freePage[class] = nil
		// Find next best freePage (excluding the one we're evacuating)
		for candidate := a.firstPage[class]; candidate != nil; candidate = candidate.next {
			if candidate != pg && candidate.freeListOffs != 0 {
				a.freePage[class] = candidate
				break
			}
		}
	}

	// Track if page was unmapped during freeing
	pageWasUnmapped := false

	// Ensure we restore state even if panic occurs
	defer func() {
		// Only restore if the page wasn't unmapped
		if !pageWasUnmapped {
			// Restore free list
			pg.freeListOffs = savedFreeListOffs

			// Restore pages pointer
			if savedPages == pgAddr {
				a.pages[class] = pgAddr
			}

			// Restore freePage if needed
			if savedFreePage == pg {
				a.freePage[class] = pg
			}
		}
	}()

	// Capture the original used count BEFORE we start freeing (which decrements pg.used)
	originalUsed := pg.used

	relocations := make([][]byte, 0, originalUsed)
	oldAddresses := make([]uintptr, 0, originalUsed)

	// Build a map of freed slots for O(1) lookup
	// IMPORTANT: Use savedFreeListOffs because we cleared pg.freeListOffs to prevent allocations
	freed := make(map[uintptr]bool, pg.cap-pg.used)
	rec := addOffset(uintptr(unsafe.Pointer(&pg.cap)), savedFreeListOffs)
	for rec != 0 {
		freed[rec] = true
		frec := (*free_record)(unsafe.Pointer(rec))
		rec = frec.next_free_record
	}

	// Scan all dirty slots in the page
	baseAddr := uintptr(unsafe.Pointer(&pg.cap)) + headerSize
	for i := uint16(0); i < pg.dirty; i++ {
		oldAddr := baseAddr + uintptr(i)*uintptr(slotSize)

		if !freed[oldAddr] {
			// This slot is in use - allocate new location
			newAddr, err := a.UintptrMalloc(slotSize)
			if err != nil {
				panic(fmt.Sprintf("Failed to allocate during defrag: %v", err))
			}

			// Copy data from old to new location
			oldSlice := unsafe.Slice((*byte)(unsafe.Pointer(oldAddr)), slotSize)
			newSlice := unsafe.Slice((*byte)(unsafe.Pointer(newAddr)), slotSize)
			copy(newSlice, oldSlice)

			// Add the new slice to relocations
			relocations = append(relocations, newSlice)

			// Save old address for freeing later (AFTER we're done with the page)
			oldAddresses = append(oldAddresses, oldAddr)
		}
	}

	if len(relocations) != int(originalUsed) {
		// Debug: understand the discrepancy
		freeCount := len(freed)
		dirtyCount := int(pg.dirty)
		capCount := int(pg.cap)
		currentUsed := int(pg.used)

		fmt.Printf("DEBUG: Page seq=%d count mismatch:\n", pg.seq)
		fmt.Printf("  originalUsed (captured): %d\n", originalUsed)
		fmt.Printf("  pg.used (current):       %d\n", currentUsed)
		fmt.Printf("  relocations (found):     %d\n", len(relocations))
		fmt.Printf("  pg.dirty:                %d\n", dirtyCount)
		fmt.Printf("  pg.cap:                  %d\n", capCount)
		fmt.Printf("  freed map size:          %d\n", freeCount)
		fmt.Printf("  expected free:           %d (cap - used)\n", capCount-int(originalUsed))

		// Verify the freed map was built correctly
		freeListCount := 0
		rec := addOffset(uintptr(unsafe.Pointer(&pg.cap)), savedFreeListOffs)
		for rec != 0 {
			freeListCount++
			frec := (*free_record)(unsafe.Pointer(rec))
			rec = frec.next_free_record
		}
		fmt.Printf("  free list count:         %d\n", freeListCount)

		panic(fmt.Sprintf("Expected %d relocations, found %d in page seq=%d",
			originalUsed, len(relocations), pg.seq))
	}

	// Now free all the old records (this may unmap the page, so we do it AFTER iteration)
	for _, oldAddr := range oldAddresses {
		if err := a.UintptrFree(oldAddr); err != nil {
			panic(fmt.Sprintf("Failed to free old record during defrag: %v", err))
		}
	}

	// Check if the page was unmapped (all records were freed)
	if len(oldAddresses) == int(originalUsed) {
		// We freed all records, so the page should be unmapped
		pageWasUnmapped = true
	}

	return relocations
}

// DefragAllImproved is an alternative that defragments all classes
// Returns all newly allocated records across all classes
func (a *Allocator) DefragAllImproved() [][]byte {
	var allRelocations [][]byte

	for class := range sizeClassSlotSize {
		relocations := a.Defrag(class)
		if len(relocations) > 0 {
			allRelocations = append(allRelocations, relocations...)
			if trace {
				fmt.Printf("Defragged class %d: %d relocations\n", class, len(relocations))
			}
		}
	}

	if trace && len(allRelocations) > 0 {
		fmt.Printf("Total defragmentation: %d records relocated across all classes\n",
			len(allRelocations))
	}

	return allRelocations
}
