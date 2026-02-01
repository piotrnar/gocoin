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
	class := int((*page_header)(unsafe.Pointer(pg)).class)

	// Dedicated page (large allocation) - slotSize == 0
	if class < 0 {
		if counters {
			a.Bytes -= int((*page_header)(unsafe.Pointer(pg)).siz)
		}
		return a.unmap(pg)
	}

	// Shared page - Add to free list
	if (*page_header)(unsafe.Pointer(pg)).used >= 1 {
		(*node)(unsafe.Pointer(p)).prev = 0
		if next := a.lists[class]; next != 0 {
			(*node)(unsafe.Pointer(p)).next = next
			(*node)(unsafe.Pointer(next)).prev = p
		} else {
			(*node)(unsafe.Pointer(p)).next = a.lists[class]
		}
		a.lists[class] = p
		(*page_header)(unsafe.Pointer(pg)).used--
		return nil
	}

	// Page is completely free - unmap it
	slotSize := sizeClassSlotSize[class]
	n := pg + headerSize
	bi := (*page_header)(unsafe.Pointer(pg)).brk
	for {
		n += uintptr(slotSize)
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
		if bi == 1 {
			break
		}
		bi--
	}

	/*
		for i := 0; i < int((*page_header)(unsafe.Pointer(pg)).brk); i++ {
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
	*/

	if a.pages[class] == pg {
		a.pages[class] = 0
	}
	if counters {
		a.Bytes -= int((*page_header)(unsafe.Pointer(pg)).siz)
	}
	return a.unmap(pg)
}

// Free deallocates memory (as in C.free).
func (a *Allocator) Free(b []byte) (err error) {
	if b = b[:cap(b)]; len(b) == 0 {
		return nil
	}

	return a.UintptrFree(uintptr(unsafe.Pointer(&b[0])))
}
