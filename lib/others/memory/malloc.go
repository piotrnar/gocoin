package memory

import (
	"fmt"
	"unsafe"
)

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

// Malloc allocates size bytes and returns a byte slice.
func (a *Allocator) Malloc(size int) (r []byte, err error) {
	p, err := a.UintptrMalloc(size)
	if p == 0 || err != nil {
		return nil, err
	}

	r = unsafe.Slice((*byte)(unsafe.Pointer(p)), size)
	return r[:size], nil
}
