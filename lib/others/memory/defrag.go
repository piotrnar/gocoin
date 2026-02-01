package memory

import (
	"fmt"
	"sort"
	"unsafe"
)

const trace = false

// pageUtilization tracks utilization metrics for defragmentation
type pageUtilization struct {
	pageAddr uintptr
	used     uint32
	cap      uint32
	ratio    float32 // used/cap
}

// Defrag performs defragmentation for a specific size class
func (a *Allocator) Defrag(class int) [][]byte {
	const minFreePages = 10    // minimum free pages to keep after defrag
	const minUtilization = 0.5 // only defrag pages below 50% utilization

	cap := int(a.cap[class])

	// Step 1: Discover all pages and their utilization by scanning free list
	pageMap := make(map[uintptr]*pageUtilization)
	freeSlotsByPage := make(map[uintptr]int)

	// Scan free list to discover pages and count free slots
	for n := a.lists[class]; n != 0; n = (*node)(unsafe.Pointer(n)).next {
		pg := n &^ uintptr(pageMask)
		freeSlotsByPage[pg]++
	}

	// Check current page if it exists
	if pg := a.pages[class]; pg != 0 {
		header := (*page_header)(unsafe.Pointer(pg))
		// Free slots in current page = capacity - brk
		freeInCurrentPage := int(a.cap[class]) - int(header.brk)
		if freeInCurrentPage > 0 {
			freeSlotsByPage[pg] += freeInCurrentPage
		}
	}

	// Build utilization map for pages with partial usage
	var totalFreeSlots int
	for pg, freeCount := range freeSlotsByPage {
		totalFreeSlots += freeCount

		header := (*page_header)(unsafe.Pointer(pg))
		used := int(header.used)

		if used > 0 && used < cap {
			pageMap[pg] = &pageUtilization{
				pageAddr: pg,
				used:     uint32(used),
				cap:      uint32(cap),
				ratio:    float32(used) / float32(cap),
			}
		}
	}

	// Calculate potential free pages
	potentialFreePages := totalFreeSlots / cap

	if potentialFreePages <= minFreePages {
		return nil
	}

	// Step 2: Sort pages by utilization (lowest first)
	pages := make([]pageUtilization, 0, len(pageMap))
	for _, pu := range pageMap {
		pages = append(pages, *pu)
	}

	if len(pages) == 0 {
		return nil
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].ratio < pages[j].ratio
	})

	// Step 3: Select pages to evacuate
	targetFreedPages := potentialFreePages - minFreePages
	var recordsToMove int
	var pagesToEvacuate []uintptr

	for i := range pages {
		if pages[i].ratio > minUtilization {
			break
		}

		pagesToEvacuate = append(pagesToEvacuate, pages[i].pageAddr)
		recordsToMove += int(pages[i].used)

		freedPages := (totalFreeSlots + recordsToMove) / cap
		if freedPages-minFreePages >= targetFreedPages {
			break
		}
	}

	if recordsToMove == 0 {
		return nil
	}

	if trace {
		fmt.Printf("Defrag class %d: will relocate %d records from %d pages (target: free %d pages)\n",
			class, recordsToMove, len(pagesToEvacuate), targetFreedPages)
	}

	// Step 4: Perform relocation
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
		fmt.Printf("Defrag class %d: relocated %d records\n", class, len(relocations))
	}

	return relocations
}

// relocatePageRecords relocates all used records from a page to new locations
func (a *Allocator) relocatePageRecords(pg uintptr, slotSize int, class int) [][]byte {
	header := (*page_header)(unsafe.Pointer(pg))

	if header.used == 0 {
		return nil
	}

	// CRITICAL: Prevent allocations into this page during evacuation
	// Strategy: Temporarily remove all free slots from this page from a.lists[class]

	// Save state
	savedPages := a.pages[class]

	// Remove from current page
	if a.pages[class] == pg {
		a.pages[class] = 0
	}

	// Remove all free slots from this page from the global free list
	var removedNodes []uintptr // Nodes we removed
	var prev uintptr = 0

	for n := a.lists[class]; n != 0; {
		next := (*node)(unsafe.Pointer(n)).next

		if (n &^ uintptr(pageMask)) == pg {
			// This free slot is from the page we're evacuating - remove it
			removedNodes = append(removedNodes, n)

			// Unlink from list
			if prev == 0 {
				// Removing head
				a.lists[class] = next
			} else {
				(*node)(unsafe.Pointer(prev)).next = next
			}

			if next != 0 {
				(*node)(unsafe.Pointer(next)).prev = prev
			}
		} else {
			prev = n
		}
		n = next
	}

	// Capture original used count
	originalUsed := header.used

	// Track if page was unmapped
	pageWasUnmapped := false

	// Ensure restoration
	defer func() {
		if !pageWasUnmapped {
			// Restore pages pointer
			if savedPages == pg {
				a.pages[class] = pg
			}

			// Re-insert removed nodes at head of list
			for _, n := range removedNodes {
				(*node)(unsafe.Pointer(n)).prev = 0
				(*node)(unsafe.Pointer(n)).next = a.lists[class]
				if a.lists[class] != 0 {
					(*node)(unsafe.Pointer(a.lists[class])).prev = n
				}
				a.lists[class] = n
			}
		}
	}()

	// Build set of free slots for this page
	freeSlots := make(map[uintptr]bool, len(removedNodes))
	for _, n := range removedNodes {
		freeSlots[n] = true
	}

	// Relocate used slots
	relocations := make([][]byte, 0, originalUsed)
	oldAddresses := make([]uintptr, 0, originalUsed)

	baseAddr := pg + headerSize
	for i := uint32(0); i < header.brk; i++ {
		slotAddr := baseAddr + uintptr(i)*uintptr(slotSize)

		if !freeSlots[slotAddr] {
			// This slot is in use - allocate new location
			newAddr, err := a.UintptrMalloc(slotSize)
			if err != nil {
				panic(fmt.Sprintf("Failed to allocate during defrag: %v", err))
			}

			// Copy data
			oldSlice := unsafe.Slice((*byte)(unsafe.Pointer(slotAddr)), slotSize)
			newSlice := unsafe.Slice((*byte)(unsafe.Pointer(newAddr)), slotSize)
			copy(newSlice, oldSlice)

			relocations = append(relocations, newSlice)
			oldAddresses = append(oldAddresses, slotAddr)
		}
	}

	if len(relocations) != int(originalUsed) {
		panic(fmt.Sprintf("Expected %d relocations, found %d in page=%#x",
			originalUsed, len(relocations), pg))
	}

	// Free all old records
	for _, oldAddr := range oldAddresses {
		if err := a.UintptrFree(oldAddr); err != nil {
			panic(fmt.Sprintf("Failed to free old record during defrag: %v", err))
		}
	}

	// Mark page as unmapped if we freed everything
	if len(oldAddresses) == int(originalUsed) {
		pageWasUnmapped = true
	}

	return relocations
}

// DefragAllImproved defragments all classes
func (a *Allocator) DefragAllImproved() [][]byte {
	var allRelocations [][]byte

	bytesBefore := a.Bytes

	for class := range sizeClassSlotSize {
		relocations := a.Defrag(class)
		if len(relocations) > 0 {
			allRelocations = append(allRelocations, relocations...)
			if trace {
				fmt.Printf("Defragged class %d: %d relocations\n", class, len(relocations))
			}
		}
	}

	if len(allRelocations) > 0 {
		if a.Bytes <= bytesBefore {
			panic(fmt.Sprintf("%d records moved, but no improvement: %d -> %d", len(allRelocations), bytesBefore, a.Bytes))
		}
		if trace {
			fmt.Printf("Total: %d records relocated\n", len(allRelocations))
		}
	}

	return allRelocations
}
