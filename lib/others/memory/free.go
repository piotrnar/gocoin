package memory

import "unsafe"

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

// Free deallocates memory (as in C.free).
func (a *Allocator) Free(b []byte) (err error) {
	if b = b[:cap(b)]; len(b) == 0 {
		return nil
	}

	return a.UintptrFree(uintptr(unsafe.Pointer(&b[0])))
}
