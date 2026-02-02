package memory

import (
	"fmt"
	"reflect"
	"sort"
	"unsafe"
)

const trace = false

// pageUtilization tracks utilization metrics for defragmentation
type pageUtilization struct {
	pageAddr uintptr
	used     uint16
	free     uint16
	cap      uint16
	ratio    float32 // used/cap
}

// Defrag performs defragmentation for a specific size class
func (a *Allocator) Defrag(class int) []*[]byte {
	const minFreePages = 10    // minimum free pages to keep after defrag
	const minUtilization = 0.5 // only defrag pages below 50% utilization

	cap := int(a.cap[class])

	// Step 1: Quick check using counters - O(1)
	potentialFreePages := int(a.freeSlots[class]) / cap

	if potentialFreePages <= minFreePages {
		return nil // not worth defragmenting
	}

	// Step 2: Walk page linked list to build utilization map - O(P)
	pageMap := make(map[uintptr]*pageUtilization)

	for pg := a.firstPage[class]; pg != 0; pg = (*page_header)(unsafe.Pointer(pg)).next {
		header := (*page_header)(unsafe.Pointer(pg))
		used := int(header.used)

		if used > 0 && used < cap {
			pageMap[pg] = &pageUtilization{
				pageAddr: pg,
				used:     header.used,
				free:     header.free,
				cap:      uint16(cap),
				ratio:    float32(used) / float32(cap),
			}
		}
	}

	if len(pageMap) == 0 {
		return nil
	}

	// Step 3: Sort pages by utilization (lowest first)
	pages := make([]pageUtilization, 0, len(pageMap))
	for _, pu := range pageMap {
		pages = append(pages, *pu)
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].ratio < pages[j].ratio
	})

	// Step 4: Select pages to evacuate
	targetFreedPages := potentialFreePages - minFreePages
	var recordsToMove int
	var pagesToEvacuate []uintptr

	for i := range pages {
		if pages[i].ratio > minUtilization {
			break
		}

		pagesToEvacuate = append(pagesToEvacuate, pages[i].pageAddr)
		recordsToMove += int(pages[i].used)

		freedPages := (int(a.freeSlots[class]) + recordsToMove) / cap
		if freedPages-minFreePages >= targetFreedPages {
			break
		}
	}

	if recordsToMove == 0 {
		return nil
	}

	if trace {
		fmt.Printf("Defrag class %d: relocating %d records from %d pages (target: free %d pages)\n",
			class, recordsToMove, len(pagesToEvacuate), targetFreedPages)
	}

	// Step 5: Mark all pages as evacuating
	type evacInfo struct {
		pg           uintptr
		originalUsed uint16
		freeSlots    map[uintptr]bool // will be populated in step 6
	}

	evacPages := make([]evacInfo, len(pagesToEvacuate))

	for idx, pg := range pagesToEvacuate {
		header := (*page_header)(unsafe.Pointer(pg))

		header.evacuating = 1

		evacPages[idx].pg = pg
		evacPages[idx].originalUsed = header.used

		// Remove from current allocation page
		if a.pages[class] == pg {
			a.pages[class] = 0
		}
	}

	// Step 6: For each evacuating page, collect its free slots and remove them from global list
	// This is O(P_evac × F_per_page) instead of O(F_total) - MUCH faster!
	for idx := range evacPages {
		pg := evacPages[idx].pg
		header := (*page_header)(unsafe.Pointer(pg))

		// Build set of free slots by walking per-page free list
		freeSlots := make(map[uintptr]bool, header.free)

		for n := header.freeList; n != 0; n = (*node)(unsafe.Pointer(n)).nextInPage {
			freeSlots[n] = true
		}

		// Now remove these slots from global free list
		for n := header.freeList; n != 0; {
			nextInPage := (*node)(unsafe.Pointer(n)).nextInPage

			// Remove from global list
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

		// Store free slots for evacuation phase
		evacPages[idx].freeSlots = freeSlots

		// Clear per-page free list
		header.freeList = 0
	}

	// Step 7: Evacuate all pages
	slotSize := int(sizeClassSlotSize[class])
	relocations := make([]*[]byte, 0, recordsToMove)

	for idx := range evacPages {
		pg := evacPages[idx].pg
		header := (*page_header)(unsafe.Pointer(pg))
		freeSlots := evacPages[idx].freeSlots

		// Scan all slots up to brk
		baseAddr := pg + headerSize

		for i := uint16(0); i < header.brk; i++ {
			slotAddr := baseAddr + uintptr(i)*uintptr(slotSize)

			// Skip free slots
			if freeSlots[slotAddr] {
				continue
			}

			// This is a used slot - relocate it
			newAddr, err := a.UintptrMalloc(slotSize)
			if err != nil {
				panic(fmt.Sprintf("Failed to allocate during defrag: %v", err))
			}

			// Copy data
			//oldSlice := unsafe.Slice((*byte)(unsafe.Pointer(slotAddr)), slotSize)
			oldSlice := (*reflect.SliceHeader)(unsafe.Pointer(slotAddr))
			//newSlice := unsafe.Slice((*byte)(unsafe.Pointer(newAddr)), slotSize)
			newSlice := (*reflect.SliceHeader)(unsafe.Pointer(newAddr))
			newSlice.Cap = oldSlice.Cap
			newSlice.Data = uintptr(newAddr + 24)
			newSlice.Len = oldSlice.Cap
			ns := (*[]byte)(unsafe.Pointer(newSlice))
			copy(*ns, *((*[]byte)(unsafe.Pointer(oldSlice))))

			relocations = append(relocations, ns)

			// Free old slot
			if err := a.UintptrFree(slotAddr); err != nil {
				panic(fmt.Sprintf("Failed to free during defrag: %v", err))
			}
		}
	}

	// Step 8: Unmap evacuated pages
	for idx := range evacPages {
		pg := evacPages[idx].pg
		header := (*page_header)(unsafe.Pointer(pg))

		if header.used != 0 {
			panic(fmt.Sprintf("Page %#x still has %d used slots after evacuation!", pg, header.used))
		}

		header.evacuating = 0

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

		// Update counters
		a.pageCount[class]--
		a.freeSlots[class] -= uint32(header.free)

		if a.pages[class] == pg {
			a.pages[class] = 0
		}

		if counters {
			a.Bytes -= int(header.siz)
			a.Mmaps--
		}

		if err := unmap(pg, int(header.siz)); err != nil {
			panic(fmt.Sprintf("Failed to unmap page %#x: %v", pg, err))
		}
	}

	if trace {
		fmt.Printf("Defrag class %d: relocated %d records, freed %d pages\n",
			class, len(relocations), len(evacPages))
	}

	return relocations
}

// DefragAllImproved defragments all classes
func (a *Allocator) DefragAllImproved() []*[]byte {
	var allRelocations []*[]byte
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
		bytesFreed := bytesBefore - a.Bytes
		if bytesFreed <= 0 {
			panic(fmt.Sprintf("%d records moved, but no memory freed: %d -> %d",
				len(allRelocations), bytesBefore, a.Bytes))
		}
		if trace {
			fmt.Printf("Total: %d records relocated, freed %d bytes (%.2f MB)\n",
				len(allRelocations), bytesFreed, float64(bytesFreed)/(1024*1024))
		}
	}

	return allRelocations
}
