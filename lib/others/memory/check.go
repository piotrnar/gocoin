package memory

func (a *Allocator) isCorrupt(class int) bool {
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
