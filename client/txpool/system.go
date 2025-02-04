package txpool

import (
	"unsafe"
)

// Mind that tx.InPackages and t.MemInputs can be modyfied when TX is alerady in the mempool
// .. in which case tx.Footprint and TransactionsToSendSize are not updated.
func (t *OneTxToSend) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	size += t.Tx.SysSize()
	if t.InPackages != nil {
		size += 8 * len(t.InPackages)
	}
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
