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

	// Step 1: Discover all pages and their utilization
	pageMap := make(map[uintptr]*pageUtilization)
	freeSlotsByPage := make(map[uintptr]int)

	if trace {
		fmt.Printf("Defrag class %d: scanning free list (head=%#x)\n", class, a.lists[class])
	}
	
	// Scan free list to discover pages and count free slots
	nodeCount := 0
	for n := a.lists[class]; n != 0; {
		nodeCount++
		if nodeCount > 1000000 {
			panic(fmt.Sprintf("Free list for class %d appears infinite (>1M nodes)", class))
		}
		
		// Validate node pointer
		if n&uintptr(pageMask) < headerSize {
			panic(fmt.Sprintf("Invalid node pointer %#x (below header size)", n))
		}
		
		pg := n &^ uintptr(pageMask)
		freeSlotsByPage[pg]++
		
		// Read next carefully
		nodePtr := (*node)(unsafe.Pointer(n))
		next := nodePtr.next
		
		// Validate next pointer
		if next != 0 {
			if next == ^uintptr(0) {
				panic(fmt.Sprintf("Node %#x (page %#x) has corrupted next pointer: 0x%x", n, pg, next))
			}
			if next&uintptr(pageMask) < headerSize && next&uintptr(pageMask) != 0 {
				panic(fmt.Sprintf("Node %#x has invalid next pointer: %#x", n, next))
			}
		}
		
		n = next
	}
	
	if trace {
		fmt.Printf("Defrag class %d: found %d nodes in free list across %d pages\n",
			class, nodeCount, len(freeSlotsByPage))
	}

	// Check current page
	if pg := a.pages[class]; pg != 0 {
		header := (*page_header)(unsafe.Pointer(pg))
		freeInCurrentPage := int(a.cap[class]) - int(header.brk)
		if freeInCurrentPage > 0 {
			freeSlotsByPage[pg] += freeInCurrentPage
		}
	}

	// Build utilization map
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
		fmt.Printf("Defrag class %d: relocating %d records from %d pages (target: free %d pages)\n",
			class, recordsToMove, len(pagesToEvacuate), targetFreedPages)
	}

	// Step 4: Mark all pages as evacuating and remove their free slots
	type evacInfo struct {
		pg           uintptr
		removedNodes []uintptr
		originalUsed uint32
	}

	evacPages := make([]evacInfo, len(pagesToEvacuate))

	for idx, pg := range pagesToEvacuate {
		header := (*page_header)(unsafe.Pointer(pg))
		
		// Mark page as evacuating
		header.evacuating = 1
		
		evacPages[idx].pg = pg
		evacPages[idx].originalUsed = uint32(header.used)

		// Remove from current allocation page
		if a.pages[class] == pg {
			a.pages[class] = 0
		}
	}

	// Build map for O(1) lookup of which pages are evacuating
	evacPageMap := make(map[uintptr]int, len(evacPages))
	for idx := range evacPages {
		evacPageMap[evacPages[idx].pg] = idx
	}

	// Remove free slots from evacuating pages in single pass
	var prev uintptr = 0
	for n := a.lists[class]; n != 0; {
		// Check if the node itself is from an evacuating page (shouldn't happen!)
		nodePage := n &^ uintptr(pageMask)
		if _, isEvac := evacPageMap[nodePage]; isEvac {
			nodeHeader := (*page_header)(unsafe.Pointer(nodePage))
			panic(fmt.Sprintf("Free list contains node %#x from evacuating page %#x (evacuating=%d, used=%d)",
				n, nodePage, nodeHeader.evacuating, nodeHeader.used))
		}
		
		// Safety check for corrupted pointers
		if n == ^uintptr(0) { // 0xffffffffffffffff
			panic(fmt.Sprintf("Corrupted free list: node at %#x is 0xffffffffffffffff", prev))
		}
		
		next := (*node)(unsafe.Pointer(n)).next
		
		// Safety check
		if next == ^uintptr(0) {
			panic(fmt.Sprintf("Corrupted free list: node %#x has next=0xffffffffffffffff", n))
		}
		
		pg := n &^ uintptr(pageMask)

		// Check if this slot belongs to an evacuating page (O(1) lookup)
		if evacIdx, isEvacuating := evacPageMap[pg]; isEvacuating {
			evacPages[evacIdx].removedNodes = append(evacPages[evacIdx].removedNodes, n)

			// Unlink from list
			if prev == 0 {
				a.lists[class] = next
			} else {
				(*node)(unsafe.Pointer(prev)).next = next
			}
			if next != 0 {
				(*node)(unsafe.Pointer(next)).prev = prev
			}
			// Don't update prev - we removed this node
		} else {
			// Keep this node - update prev
			prev = n
		}
		n = next
	}

	// Step 5: Evacuate all pages
	slotSize := int(sizeClassSlotSize[class])
	relocations := make([][]byte, 0, recordsToMove)

	for idx := range evacPages {
		info := &evacPages[idx]

		// Build free slots set
		freeSlots := make(map[uintptr]bool, len(info.removedNodes))
		for _, n := range info.removedNodes {
			freeSlots[n] = true
		}

		// Evacuate this page
		baseAddr := info.pg + headerSize
		header := (*page_header)(unsafe.Pointer(info.pg))

		for i := uint16(0); i < header.brk; i++ {
			slotAddr := baseAddr + uintptr(i)*uintptr(slotSize)

			if !freeSlots[slotAddr] {
				// Allocate new location
				newAddr, err := a.UintptrMalloc(slotSize)
				if err != nil {
					panic(fmt.Sprintf("Failed to allocate during defrag: %v", err))
				}

				// Copy data
				oldSlice := unsafe.Slice((*byte)(unsafe.Pointer(slotAddr)), slotSize)
				newSlice := unsafe.Slice((*byte)(unsafe.Pointer(newAddr)), slotSize)
				copy(newSlice, oldSlice)

				relocations = append(relocations, newSlice)

				// Free old slot - evacuation flag prevents it from re-entering free list
				if err := a.UintptrFree(slotAddr); err != nil {
					panic(fmt.Sprintf("Failed to free during defrag: %v", err))
				}
			}
		}
	}

	// Step 6: Clean up - unmap evacuated pages
	for idx := range evacPages {
		pg := evacPages[idx].pg
		header := (*page_header)(unsafe.Pointer(pg))
		
		// Page should be empty now
		if header.used != 0 {
			panic(fmt.Sprintf("Page %#x still has %d used slots after evacuation!", pg, header.used))
		}

		// Clear evacuation flag
		header.evacuating = 0

		// Unmap the page
		slotSize := sizeClassSlotSize[class]
		n := pg + headerSize
		bi := header.brk

		// Remove all slots from free list (should already be removed, but be safe)
		for {
			n += uintptr(slotSize)
			if (*node)(unsafe.Pointer(n)).next == 0 && (*node)(unsafe.Pointer(n)).prev == 0 {
				// Not in list
			} else {
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
			if bi == 1 {
				break
			}
			bi--
		}

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
