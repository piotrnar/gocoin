package memory

import (
	"reflect"
	"unsafe"
)

func (a *Allocator) unmap(p uintptr, size int) error {
	return unmap(p, size)
}

// uintptrFreePrivate handles freeing a large (private page) allocation.
// No class mutex needed - only touches atomic counters and does unmap.
func (a *Allocator) uintptrFreePrivate(p uintptr) error {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(p))
	size := sh.Cap + sliceHdrLen
	a.Bytes.Add(int64(-size))
	a.PrivateMmaps.Add(-1)
	return a.unmap(p, size)
}

// uintptrFreeShared handles freeing a slot from a shared page.
// Must be called while holding classMu[class].
// Returns the page pointer and true if the page became empty and should be unmapped.
func (a *Allocator) uintptrFreeShared(p uintptr) (unmapPage uintptr) {
	pg := p &^ uintptr(pageMask)
	class := int((*page_header)(unsafe.Pointer(pg)).class)

	header := (*page_header)(unsafe.Pointer(pg))

	if header.used >= 1 {
		header.used--
		header.free++
		a.freeSlots[class]++

		// If page is being evacuated, don't add to free list
		if header.evacuating {
			return 0
		}

		// Add to global free list
		(*node)(unsafe.Pointer(p)).prev = 0
		if next := a.lists[class]; next != 0 {
			(*node)(unsafe.Pointer(p)).next = next
			(*node)(unsafe.Pointer(next)).prev = p
		} else {
			(*node)(unsafe.Pointer(p)).next = a.lists[class]
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

		return 0
	}

	// Page is completely free - unmap it
	// Remove all free slots from global free list using per-page list - O(F_page) not O(brk)!
	for n := header.freeList; n != 0; {
		nextInPage := (*node)(unsafe.Pointer(n)).nextInPage

		// Remove from global free list
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
	a.SharedMmaps.Add(-1)

	// Return the page to caller - caller decides whether to cache or unmap.
	// Bytes accounting is NOT done here - caller must handle it.
	return pg
}

// uintptrFree is like Free except its argument is an uintptr.
// Used by defrag code which runs exclusively (no concurrent Malloc/Free).
func (a *Allocator) uintptrFree(p uintptr) (err error) {
	a.Allocs.Add(-1)

	if sh := (*reflect.SliceHeader)(unsafe.Pointer(p)); sh.Cap+sliceHdrLen > a.MaxSharedSize {
		return a.uintptrFreePrivate(p)
	}

	pg := a.uintptrFreeShared(p)
	if pg != 0 {
		a.Bytes.Add(-pageSize)
		return a.unmap(pg, osPageSize)
	}
	return nil
}

// Free deallocates memory (as in C.free).
func (a *Allocator) Free(b *[]byte) {
	a.Allocs.Add(-1)

	p := uintptr(unsafe.Pointer(b))

	// Check if this is a private (large) allocation - no class mutex needed
	if sh := (*reflect.SliceHeader)(unsafe.Pointer(p)); sh.Cap+sliceHdrLen > a.MaxSharedSize {
		if er := a.uintptrFreePrivate(p); er != nil {
			panic(er.Error())
		}
		return
	}

	// Shared allocation - lock the class mutex
	pg := p &^ uintptr(pageMask)
	class := int((*page_header)(unsafe.Pointer(pg)).class)
	a.classMu[class].Lock()
	emptyPage := a.uintptrFreeShared(p)
	a.classMu[class].Unlock()

	// If a page became empty, try to recycle it into the page cache
	if emptyPage != 0 {
		if !a.pageCachePut(emptyPage) {
			// Cache is full - actually unmap and adjust Bytes
			a.Bytes.Add(-pageSize)
			if er := a.unmap(emptyPage, osPageSize); er != nil {
				panic(er.Error())
			}
		}
	}
}
