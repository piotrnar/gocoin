package txpool

import (
	"fmt"
	"io"
	"unsafe"
)

func (t *OneTxToSend) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	size += t.Tx.SysSize()
	if t.MemInputs != nil {
		size += (len(t.MemInputs) + 7) & ^7 // round the size up to the nearest 8 bytes
	}
	return
}

func (t *OneTxToSend) SysSizeDbg(wr io.Writer) (size int) {
	size = int(unsafe.Sizeof(*t))
	fmt.Fprintln(wr, "OneTxToSend base:", size)
	size += t.Tx.SysSizeDbg(wr)
	fmt.Fprintln(wr, "OneTxToSend +Tx:", size)
	if t.MemInputs != nil {
		size += (len(t.MemInputs) + 7) & ^7 // round the size up to the nearest 8 bytes
		fmt.Fprintln(wr, "OneTxToSend +meminputs:", size)
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
