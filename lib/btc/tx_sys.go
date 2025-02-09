package btc

import (
	"unsafe"
)

func (po *TxPrevOut) SysSize() int {
	return int(unsafe.Sizeof(*po))
}

func (ti *TxIn) SysSize() (size int) {
	size = int(unsafe.Sizeof(*ti))
	if ti.ScriptSig != nil {
		size += (cap(ti.ScriptSig) + 7) & ^7
	}
	return
}

func (to *TxOut) SysSize() (size int) {
	size = int(unsafe.Sizeof(*to))
	if to.Pk_script != nil {
		size += (cap(to.Pk_script) + 7) &^ 7
	}
	return
}

func (tx *Tx) SysSize() (size int) {
	size = int(unsafe.Sizeof(*tx))

	if tx.TxIn != nil {
		size += cap(tx.TxIn) * 8
		for _, ti := range tx.TxIn {
			size += ti.SysSize()
		}
	}

	if tx.TxOut != nil {
		size += cap(tx.TxOut) * 8
		for _, to := range tx.TxOut {
			size += to.SysSize()
		}
	}

	if tx.SegWit != nil {
		size += cap(tx.SegWit) * 8
		for _, sw := range tx.SegWit {
			if sw != nil {
				size += cap(sw) * 8
				for _, sww := range sw {
					if sww != nil {
						size += (cap(sww) + 7) &^ 7
					}
				}
			}
		}
	}
	if tx.Raw != nil {
		size += (cap(tx.Raw) + 7) &^ 7
	}

	if tx.TxVerVars == nil {
		return
	}
	size += int(unsafe.Sizeof(*tx.TxVerVars))

	if tx.hashPrevouts != nil {
		size += int(unsafe.Sizeof(*tx.hashPrevouts))
	}
	if tx.hashSequence != nil {
		size += int(unsafe.Sizeof(*tx.hashSequence))
	}
	if tx.hashOutputs != nil {
		size += int(unsafe.Sizeof(*tx.hashOutputs))
	}

	if tx.Spent_outputs != nil {
		size += 8 * cap(tx.Spent_outputs)
		for _, so := range tx.Spent_outputs {
			size += so.SysSize()
		}
	}

	if tx.tapSingleHashes != nil {
		size += int(unsafe.Sizeof(*tx.tapSingleHashes))
	}
	if tx.tapOutSingleHash != nil {
		size += int(unsafe.Sizeof(*tx.tapOutSingleHash))
	}

	return
}
