package memory

import (
	"fmt"
	"unsafe"
)

func count_free_slots(page *page_header) (cnt int) {
	rec := addOffset(uintptr(unsafe.Pointer(&page.cap)), page.freeListOffs)
	cnt = int(page.cap) - int(page.dirty)
	for rec != 0 {
		cnt++
		rec = (*free_record)(unsafe.Pointer(rec)).next_free_record
	}
	return
}

func (a *Allocator) DumpClass(class int) {
	var cnt int
	fmt.Println("Dumping class", class)
	for page := a.firstPage[class]; page != nil; page = page.next {
		cnt++
		fmt.Printf(" %d)  ptr:%p  seq:%d  cap:%d  dirty:%d  used:%d  free:%d\n", cnt, page, page.seq,
			page.cap, page.dirty, page.used, count_free_slots(page))
	}
}

func (a *Allocator) GetInfo() string {
	return fmt.Sprintf("%d/%d/%d", a.Bytes, a.Allocs, a.Mmaps)
}

func (a *Allocator) PrintStats() {
	var to_save int
	for class := range a.fcnt {
		var ts int
		if a.fcnt[class] > a.Capacity[class] {
			ts = int(a.fcnt[class] / a.Capacity[class])
		}
		fmt.Printf("%3d) ... %5d: %10d (%3d%%) free slots in %6d pgs - %6.2f pages can be free: %5d\n",
			class, sizeClassSlotSize[class], a.fcnt[class], 100*a.fcnt[class]/(a.pcnt[class]*a.Capacity[class]),
			a.pcnt[class], float64(a.fcnt[class])/float64(a.Capacity[class]), ts)
		to_save += ts
	}
	fmt.Println("Total to save if all defragmented:", (to_save*pageSize)>>20, "MB")
}
