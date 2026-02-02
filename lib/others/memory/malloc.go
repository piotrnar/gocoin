package memory

import (
	"fmt"
	"reflect"
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

	(*page_header)(unsafe.Pointer(p)).class = -1 // Mark as dedicated page
	return p, nil
}

// newSharedPage creates a new shared page for a specific size class
func (a *Allocator) newSharedPage(class int) (uintptr /* *page */, error) {
	slotSize := getSlotSize(class)
	if slotSize == 0 {
		panic(fmt.Sprintf("invalid size class: %d", class))
	}

	totalSize := uint32(headerSize) + a.cap[class]*slotSize
	p, err := a.mmap(int(totalSize))
	if err != nil {
		return 0, err
	}

	header := (*page_header)(unsafe.Pointer(p))
	header.class = int16(class)
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
		slotSize := sizeClassSlotSize[class]
		return p + headerSize + uintptr(header.brk-1)*uintptr(slotSize), nil
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
	return n, nil
}

// Malloc allocates size bytes and returns a byte slice.
func (a *Allocator) Malloc(size int) (r *[]byte, err error) {
	size += 24
	p, err := a.UintptrMalloc(size)
	if p == 0 || err != nil {
		return nil, err
	}

	//r = unsafe.Slice((*byte)(unsafe.Pointer(p)), size)
	//return r[:size], nil
	sh := (*reflect.SliceHeader)(unsafe.Pointer(p))
	sh.Cap = size - 24
	sh.Data = uintptr(p + 24)
	sh.Len = size - 24
	return (*[]byte)(unsafe.Pointer(sh)), nil
}
