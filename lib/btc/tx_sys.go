package btc

import (
	"fmt"
	"io"
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

	if tx.TxIn != nil {
		size += len(tx.TxIn) * 8
		for _, ti := range tx.TxIn {
			size += ti.SysSize()
		}
	}

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
		// should we not include these as they belong to another tx (account for its size) ???
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

func (tx *Tx) SysSizeDbg(wr io.Writer) (size int) {
	defer func() {
		fmt.Fprintln(wr, "Tx-final", size)
	}()

	size = int(unsafe.Sizeof(*tx))
	fmt.Fprintln(wr, "OneTxToSend base:", size)

	if tx.TxIn != nil {
		size += len(tx.TxIn) * 8
		fmt.Fprintln(wr, "OneTxToSend TxIn cnt", len(tx.TxIn), size)
		for i, ti := range tx.TxIn {
			size += ti.SysSize()
			fmt.Fprintln(wr, "OneTxToSend TxIn", i, size)
		}
	}

	if tx.TxOut != nil {
		size += len(tx.TxOut) * 8
		fmt.Fprintln(wr, "OneTxToSend TxOut cnt", len(tx.TxOut), size)
		for i, to := range tx.TxOut {
			size += to.SysSize()
			fmt.Fprintln(wr, "OneTxToSend TxOut", i, size)
		}
	}

	if tx.SegWit != nil {
		size += len(tx.SegWit) * 8
		fmt.Fprintln(wr, "SegWit cnt", len(tx.SegWit), size)
		for i1, sw := range tx.SegWit {
			if sw != nil {
				size += len(sw) * 8
				fmt.Fprintln(wr, "SegWit A", i1, len(sw), size)
				for i2, sww := range sw {
					if sww != nil {
						size += (len(sww) + 7) &^ 7
						fmt.Fprintln(wr, "SegWit B", i2, size)
					}
				}
			}
		}
	}
	if tx.Raw != nil {
		size += (len(tx.Raw) + 7) &^ 7
		fmt.Fprintln(wr, "Raw", size)
	}

	if tx.hashPrevouts != nil {
		size += int(unsafe.Sizeof(*tx.hashPrevouts))
		fmt.Fprintln(wr, "hashPrevouts", size)
	}
	if tx.hashSequence != nil {
		size += int(unsafe.Sizeof(*tx.hashSequence))
		fmt.Fprintln(wr, "hashSequence", size)
	}
	if tx.hashOutputs != nil {
		size += int(unsafe.Sizeof(*tx.hashOutputs))
		fmt.Fprintln(wr, "hashOutputs", size)
	}

	if tx.Spent_outputs != nil {
		size += 8 * len(tx.Spent_outputs)
		fmt.Fprintln(wr, "Spent_outputs", len(tx.Spent_outputs), size)
		for _, so := range tx.Spent_outputs {
			size += so.SysSize()
		}
	}

	if tx.tapSingleHashes != nil {
		size += int(unsafe.Sizeof(*tx.tapSingleHashes))
		fmt.Fprintln(wr, "tapSingleHashes", size)
	}
	if tx.tapOutSingleHash != nil {
		size += int(unsafe.Sizeof(*tx.tapOutSingleHash))
		fmt.Fprintln(wr, "tapOutSingleHash", size)
	}

	return
}
