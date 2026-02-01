package memory

import (
	"fmt"
)

func (a *Allocator) DumpClass(class int) {
	fmt.Println("Dumping class", class)
}

func (a *Allocator) GetInfo() string {
	return fmt.Sprintf("%d/%d/%d", a.Bytes, a.Allocs, a.Mmaps)
}

func (a *Allocator) PrintStats() {
}
