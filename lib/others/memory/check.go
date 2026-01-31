package memory

func (a *Allocator) isCorrupt(class int) bool {
	var cnt, fcnt int
	var prev_seq uint64
	var prev_page *page_header
	var free_page_found bool
	var pages_with_freed_slots int

	if a.firstPage[class] == nil {
		if a.lastPage[class] != nil {
			println("first page nil but last not")
			return true
		}
		if a.fcnt[class] != 0 || a.pcnt[class] != 0 {
			println("no pages but fcnt or pcnt not zero", a.fcnt[class], a.pcnt[class])
			return true
		}
	}

	if a.lastPage[class] == nil && a.firstPage[class] != nil {
		println("last page nil but first not")
		return true
	}

	first_page := true
	for page := a.firstPage[class]; page != nil; page = page.next {
		if first_page {
			if page.prev != nil {
				println("First page's prev not nil")
				return true
			}
			first_page = false
		} else {
			if page.seq <= prev_seq {
				println("page sequecne out of order", page.seq, prev_seq)
				return true
			}
		}
		prev_seq = page.seq
		prev_page = page
		cnt++
		if page == a.freePage[class] {
			if free_page_found {
				println("free_page_found twice")
				return true
			}
			free_page_found = true
			if page.freeListOffs == 0 {
				println("free_page has no free offset")
				return true
			}
		}
		if page.freeListOffs != 0 {
			pages_with_freed_slots++
		}
		fcnt += count_free_slots(page)
	}
	if fcnt != int(a.fcnt[class]) {
		println("fcnt mismatch", fcnt, a.fcnt[class])
		return true
	}
	if pages_with_freed_slots > 0 && !free_page_found {
		println("free_page not found")
		return true
	}
	if a.pcnt[class] != uint32(cnt) {
		println("a.pcnt / linked pages mismatch:", a.pcnt[class], cnt)
		return true
	}
	if a.lastPage[class] != prev_page {
		println("wrong LastPage")
		return true

	}
	return false
}

func (a *Allocator) IsCorrupt() bool {
	for class := range sizeClassSlotSize {
		if a.isCorrupt(class) {
			println("Class", class, "is corrupt as above. Dumping:")
			a.DumpClass(class)
			return true
		}
	}
	return false
}
