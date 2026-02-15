package memory

import (
	"fmt"
	"reflect"
	"unsafe"
)

// it the size is 0, it will allocate pageSize bytes aligned to pageSize
// otherwise it will allocate size bytes allignd to OS page size
func (a *Allocator) mmap(size int) (uintptr /* *page */, error) {
	p, size, err := mmap(size)
	if err != nil {
		return 0, err
	}
	a.Bytes.Add(int64(size))
	return p, nil
}

// newPrivatePage creates a dedicated page for a single large allocation
// the size must be ronded up to OS page size
func (a *Allocator) newPrivatePage(size int) (uintptr /* *page */, error) {
	p, err := a.mmap(size)
	if err != nil {
		return 0, err
	}
	a.PrivateMmaps.Add(1)
	return p, nil
}

// newSharedPage creates a new shared page for a specific size class
func (a *Allocator) newSharedPage(class int) (uintptr /* *page */, error) {
	slotSize := getSlotSize(class)
	if slotSize == 0 {
		panic(fmt.Sprintf("invalid size class: %d", class))
	}

	p, err := a.mmap(0)
	if err != nil {
		return 0, err
	}

	header := (*page_header)(unsafe.Pointer(p))
	header.class = byte(class)
	header.free = uint16(a.cap[class]) // All slots initially free

	// Link into page list
	header.prev = a.lastPage[class]
	header.next = 0

	if a.lastPage[class] != 0 {
		(*page_header)(unsafe.Pointer(a.lastPage[class])).next = p
	}
	if a.firstPage[class] == 0 {
		a.firstPage[class] = p
	}
	a.lastPage[class] = p

	// Update counters
	a.pageCount[class]++
	a.freeSlots[class] += a.cap[class]

	a.pages[class] = p
	a.SharedMmaps.Add(1)
	return p, nil
}

// uintptrMalloc is like Malloc except it returns an uintptr.
func (a *Allocator) uintptrMalloc(size int) (r uintptr, err error) {
	a.Allocs.Add(1)

	class := a.getSizeClass(size)

	// Large allocation - use dedicated page
	if class < 0 {
		size = roundup(size, osPageSize)
		p, err := a.newPrivatePage(size)
		if err != nil {
			return 0, err
		}
		(*reflect.SliceHeader)(unsafe.Pointer(p)).Cap = size - sliceHdrLen
		return p, nil
	}

	// Small allocation - use shared page
	if a.lists[class] == 0 && a.pages[class] == 0 {
		if _, err := a.newSharedPage(class); err != nil {
			return 0, err
		}
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
		(*reflect.SliceHeader)(unsafe.Pointer(ptr)).Cap = int(slotSize) - sliceHdrLen
		return ptr, nil
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
		// We're removing the head of the per-page list
		header.freeList = nextInPage
		if nextInPage != 0 {
			(*node)(unsafe.Pointer(nextInPage)).prevInPage = 0
		}
	} else {
		// We're in the middle or end of the per-page list
		(*node)(unsafe.Pointer(prevInPage)).nextInPage = nextInPage
		if nextInPage != 0 {
			(*node)(unsafe.Pointer(nextInPage)).prevInPage = prevInPage
		}
	}

	header.used++
	header.free--
	a.freeSlots[class]--
	(*reflect.SliceHeader)(unsafe.Pointer(n)).Cap = int(sizeClassSlotSize[class]) - sliceHdrLen
	return n, nil
}

// Malloc allocates size bytes and returns a byte slice.
func (a *Allocator) Malloc(size int) (r *[]byte) {
	a.Lock()
	p, err := a.uintptrMalloc(size + sliceHdrLen)
	a.Unlock()
	if p == 0 || err != nil {
		return nil
	}

	sh := (*reflect.SliceHeader)(unsafe.Pointer(p))
	sh.Data = p + uintptr(sliceHdrLen)
	sh.Len = size
	return (*[]byte)(unsafe.Pointer(sh))
}
