package txpool

import (
	"unsafe"
)

const (
	SYSIZE_COUNT_MEMINPUTS = true
	SYSIZE_COUNT_PACKAGES  = true
)

func (t *OneTxToSend) SysSize() (size int) {
	size = int(unsafe.Sizeof(*t))
	size += t.Tx.SysSize()
	if SYSIZE_COUNT_PACKAGES && t.inPackages != nil {
		size += 8 * cap(t.inPackages)
	}
	if SYSIZE_COUNT_MEMINPUTS && t.MemInputs != nil {
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

func (t2s *OneTxToSend) memInputsSet(newval []bool) {
	if t2s.Footprint == 0 {
		panic("memInputsSet when Footprint is 0")
	}
	if !SYSIZE_COUNT_MEMINPUTS || cap(newval) == cap(t2s.MemInputs) {
		t2s.MemInputs = newval
		return
	}
	var old_size, new_size int
	if t2s.MemInputs != nil {
		old_size = (cap(t2s.MemInputs) + 7) & ^7 // round the size up to the nearest 8 bytes
	}
	t2s.MemInputs = newval
	if t2s.MemInputs != nil {
		new_size = (cap(t2s.MemInputs) + 7) & ^7 // round the size up to the nearest 8 bytes
	}
	if old_size != new_size {
		t2s.Footprint -= uint32(old_size)
		t2s.Footprint += uint32(new_size)
		TransactionsToSendSize -= uint64(old_size)
		TransactionsToSendSize += uint64(new_size)
	}
}

func (t2s *OneTxToSend) inPackagesSet(newval []*OneTxsPackage) {
	if t2s.Footprint == 0 {
		panic("inPackagesSet when Footprint is 0")
	}
	if !SYSIZE_COUNT_PACKAGES || cap(newval) == cap(t2s.inPackages) {
		t2s.inPackages = newval
		return
	}
	var old_size, new_size int
	if t2s.inPackages != nil {
		old_size = 8 * cap(t2s.inPackages)
	}
	t2s.inPackages = newval
	if t2s.inPackages != nil {
		new_size = 8 * cap(t2s.inPackages)
	}
	if old_size != new_size {
		t2s.Footprint -= uint32(old_size)
		t2s.Footprint += uint32(new_size)
		TransactionsToSendSize -= uint64(old_size)
		TransactionsToSendSize += uint64(new_size)
	}
}

func FeePackagesSysSize() (size int) {
	size = int(unsafe.Sizeof(FeePackages)) + cap(FeePackages)*int(unsafe.Sizeof(FeePackages[0]))
	if len(FeePackages) > 0 {
		size += len(FeePackages) * int(unsafe.Sizeof(*FeePackages[0]))
		for _, fp := range FeePackages {
			size += cap(fp.Txs) * int(unsafe.Sizeof(fp.Txs[0]))
		}
	}
	return
}
