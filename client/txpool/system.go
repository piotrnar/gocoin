package txpool

import (
	"unsafe"
)

func (t *OneTxToSend) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	size += t.Tx.SysSize()
	if t.inPackages != nil {
		size += 8 * cap(t.inPackages)
	}
	if t.MemInputs != nil {
		size += (cap(t.MemInputs) + 7) & ^7 // round the size up to the nearest 8 bytes
	}
	return
}

func (t *OneTxRejected) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	if t.Waiting4 != nil {
		size += int(unsafe.Sizeof(*t.Waiting4))
	}
	if t.Tx != nil {
		size += t.Tx.SysSize()
	}
	return
}
