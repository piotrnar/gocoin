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
		size += (len(ti.ScriptSig) + 7) & ^7
	}
	return
}

func (to *TxOut) SysSize() (size int) {
	size = int(unsafe.Sizeof(*to))
	if to.Pk_script != nil {
		size += (len(to.Pk_script) + 7) &^ 7
	}
	return
}

func (tx *Tx) SysSize() (size int) {
	size = int(unsafe.Sizeof(*tx))

	size += int(unsafe.Sizeof(tx.TxIn))
	if tx.TxIn != nil {
		size += len(tx.TxIn) * 8
		for _, ti := range tx.TxIn {
			size += ti.SysSize()
		}
	}

	size += int(unsafe.Sizeof(tx.TxOut))
	if tx.TxOut != nil {
		size += len(tx.TxOut) * 8
		for _, to := range tx.TxOut {
			size += to.SysSize()
		}
	}

	if tx.SegWit != nil {
		size += len(tx.SegWit) * 8
		for _, sw := range tx.SegWit {
			if sw != nil {
				size += len(sw) * 8
				for _, sww := range sw {
					if sww != nil {
						size += (len(sww) + 7) &^ 7
					}
				}
			}
		}
	}
	if tx.Raw != nil {
		size += (len(tx.Raw) + 7) &^ 7
	}

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
		size += 8 * len(tx.Spent_outputs)
		for _, so := range tx.Spent_outputs {
			size += so.SysSize()
		}
	}

	if tx.m_taproot != nil {
		size += int(unsafe.Sizeof(*tx.m_taproot))
	}
	if tx.m_outputs_single_hash != nil {
		size += int(unsafe.Sizeof(*tx.m_outputs_single_hash))
	}

	return
}
