package txpool

import (
	"unsafe"
)

func (t *OneTxToSend) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	size += t.Tx.SysSize()
	/*  exclude these for now, to not trigger mempool check errors.
	if t.InPackages != nil {
		size += 8 * len(t.InPackages)
	}
	*/
	if t.MemInputs != nil {
		size += (len(t.MemInputs) + 7) & ^7 // round the size up to the nearest 8 bytes
	}
	return
}

func (t *OneTxRejected) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	if t.Waiting4 != nil {
		size += int(unsafe.Sizeof(*t.Waiting4))
	}
	size += t.Tx.SysSize()
	return
}
