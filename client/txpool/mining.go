package txpool

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (rec *OneTxToSend) IIdx(key uint64) int {
	for i, o := range rec.TxIn {
		if o.Input.UIdx() == key {
			return i
		}
	}
	return -1
}

// mined clears the MemInput flag of all the children (used when a tx is mined).
func (tx *OneTxToSend) mined() {
	// Go through all the tx's outputs and unmark MemInputs in txs that have been spending it
	for vout := range tx.TxOut {
		uidx := btc.UIdx(tx.Hash.Hash[:], uint32(vout))
		if val, ok := SpentOutputs[uidx]; ok {
			if rec, ok := TransactionsToSend[val]; ok {
				if CheckForErrors() && rec.MemInputs == nil {
					common.CountSafe("TxMinedMeminER1")
					println("ERROR: out just mined in", rec.Hash.String(), "- not marked as mem")
					continue
				}
				idx := rec.IIdx(uidx)
				if CheckForErrors() {
					if idx < 0 {
						common.CountSafe("TxMinedMeminER2")
						println("ERROR: out just mined. Was in SpentOutputs & mempool, but DUPA")
						continue
					}
					if !rec.MemInputs[idx] {
						println("ERROR: ", rec.Hash.String(), "meminp", idx, "is already false")
						println("  ", rec.MemInputCnt, rec.MemInputs, rec.Footprint, rec.SysSize())
					}
				}
				rec.MemInputs[idx] = false
				rec.MemInputCnt--
				common.CountSafe("TxMinedMeminCnt")
				if rec.MemInputCnt == 0 {
					common.CountSafe("TxMinedMeminTx")
					rec.memInputsSet(nil)
				}
				SortListDirty = true // will need to resort after
			} else {
				common.CountSafe("TxMinedMeminERR")
				println("ERROR: out in SpentOutputs, but not in mempool")
			}
		}
	}
}

// unmined sets the MemInput flag of all the children (used when a tx is unmined / block undone).
func (tx *OneTxToSend) unmined() {
	// Go through all the tx's outputs and mark MemInputs in txs that have been spending it
	for vout := range tx.TxOut {
		uidx := btc.UIdx(tx.Hash.Hash[:], uint32(vout))
		if val, ok := SpentOutputs[uidx]; ok {
			if rec := TransactionsToSend[val]; rec != nil {
				if rec.MemInputs == nil {
					rec.memInputsSet(make([]bool, len(rec.TxIn)))
				}
				idx := rec.IIdx(uidx)
				if rec.MemInputs[idx] {
					println("ERROR: out", btc.NewUint256(tx.Hash.Hash[:]).String(), "-", idx, "already marked as MI")
				} else {
					rec.MemInputs[idx] = true
					rec.MemInputCnt++
					SortListDirty = true // will need to resort after
					common.CountSafe("TxPutBackMemIn")
				}
				if CheckForErrors() && rec.Footprint != uint32(rec.SysSize()) {
					println("ERROR: MarkChildrenForMem footprint mismatch", rec.Footprint, uint32(rec.SysSize()))
				}
			} else {
				println("ERROR: MarkChildrenForMem: in SpentOutputs, but not in mempool")
				common.CountSafe("TxPutBackMeminERR")
			}
		}
	}
}

// txMined is called for each tx mined in a new block.
func txMined(tx *btc.Tx) int {
	bidx := tx.Hash.BIdx()

	if rec, ok := TransactionsToSend[bidx]; ok {
		// if we have this tx in mempool, remove it and it should clean everything up nicely
		common.CountSafe("TxMinedAccepted")
		rec.mined()
		rec.Delete(false, 0) // this should take care of the RejectedUsedUTXOs stuff
		return 1
	}

	// if this tx was not in mempool, maybe another one is, that was spending (any of) the outputs?
	var was_rejected bool
	for _, inp := range tx.TxIn {
		idx := inp.Input.UIdx()
		if val, ok := SpentOutputs[idx]; ok {
			// ... in such case, make sure to discard it, along with all its children
			if rec := TransactionsToSend[val]; rec != nil {
				// there is this one...
				common.CountSafePar("TxMinedUTXO-", rec.Local)
				rec.Delete(true, 0) // this should remove relevant RejectedUsedUTXOs record as well
				if CheckForErrors() {
					if _, ok := SpentOutputs[idx]; ok {
						println("ERROR: SpentOutput was supposed to be deleted, but still here\n  ", inp.Input.String())
					}
				}
			} else {
				println("ERROR: Input SpentOutputs, but tx not in mempool\n  ", inp.Input.String())
				delete(SpentOutputs, idx)
			}
			if CheckForErrors() {
				if _, ok := RejectedUsedUTXOs[idx]; ok {
					println("ERROR: we just removed t2s that was spending out, which is left in RejectedUsedUTXOs\n  ", inp.Input.String())
				}
			}
			continue
		}

		// if the input was not in SpentOutputs, then maybe it is still in RejectedUsedUTXOs
		if lst, ok := RejectedUsedUTXOs[idx]; ok {
			// it is - remove all rejected tx that would use any of just mined inputs
			for _, rbidx := range lst {
				if txr, ok := TransactionsRejected[rbidx]; ok {
					txr.Delete()
					if rbidx == bidx {
						common.CountSafePar("TxMinedRjctdA-", txr.Reason)
						was_rejected = true
					} else {
						common.CountSafePar("TxMinedRjctUTXO-", txr.Reason)
					}
				} else {
					println("ERROR: UTXO present in RejectedUsedUTXOs, not in TransactionsRejected\n  ", inp.Input.String())
				}
			}
			delete(RejectedUsedUTXOs, idx) // this record will not be needed anymore
		}
	}

	if was_rejected {
		return 2
	}

	if mr, ok := TransactionsRejected[bidx]; ok {
		common.CountSafePar("TxMinedRjctd-", mr.Reason)
		mr.Delete()
		return 3
	}

	if TransactionsPending[bidx] {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, bidx)
	}
	return 4
}

func txVerifyClean(tx *btc.Tx, mode int, bl *btc.Block) {
	for i1, inp := range tx.TxIn {
		bidx := btc.BIdx(inp.Input.Hash[:])
		if w4i, ok := WaitingForInputs[bidx]; ok {
			for _, txrb := range w4i.Ids {
				if txr := TransactionsRejected[txrb]; txr.Tx != nil {
					for i2, tii := range txr.Tx.TxIn {
						if tii.Input == inp.Input {
							common.CountSafe("Tx**NotClean")
							println("ERROR: Dirty tx", mode, tx.Hash.String(), "at inp", i1,
								"\n still rejected:", txr.Id.String(), "with inp", i2, txr.Reason, "\n ", tii.Input.String())
							if txr.Waiting4 != nil {
								println(" waiting4:", txr.Waiting4.String())
							} else {
								println(" not waiting4")
							}
							if txr.Raw != nil {
								println(" raw:", hex.EncodeToString(txr.Raw))
							} else {
								println(" NOW raw data!!!")
							}
							println(" in block:", bl.Height, bl.Hash.String())
						}
					}
				}
			}
		}
	}
	//WaitingForInputs

}

// BlockMined removes all the block's tx from the mempool.
var txdbg_xtra_info string

func BlockMined(bl *btc.Block) {
	if len(bl.Txs) < 2 {
		return
	}

	TxMutex.Lock()
	FeePackagesDirty = true
	for i := len(bl.Txs) - 1; i > 0; i-- { // we go in reversed order to remove children before parents
		tx := bl.Txs[i]
		res := txMined(tx)
		txVerifyClean(tx, res, bl)
	}
	// now check if any mempool txs are waiting for inputs which were just mined
	for ii, tx := range bl.Txs[1:] {
		txVerifyClean(tx, -1, bl)
		if common.Testnet {
			txdbg_xtra_info = fmt.Sprintf("Block %d %s tx:%d/%d %s\n", bl.Height, bl.Hash.String(), ii+1, len(bl.Txs), tx.Hash.String())
		}
		txAccepted(tx.Hash.BIdx())
		txdbg_xtra_info = ""
	}
	TxMutex.Unlock()
}

func BlockUndone(bl *btc.Block) {
	common.CountSafe("TxPkgsBlockUndo")
	if len(bl.Txs) < 2 {
		return
	}

	TxMutex.Lock()
	// this will spare us all the struggle with trying to re-package each tx
	// .. plus repackaging and resorting of unmined txs is not implemented :)
	FeePackagesDirty = true
	for _, tx := range bl.Txs[1:] {
		if tr, ok := TransactionsRejected[tx.Hash.BIdx()]; ok {
			tr.Delete()
			common.CountSafePar("TxUnmineRejected-", tr.Reason)
		}

		ntx := &TxRcvd{Tx: tx, Trusted: true, Unmined: true}
		if res, t2s := processTx(ntx); res == 0 {
			t2s.unmined()
			common.CountSafe("TxUnmineOK")
		} else {
			println("ERROR: TxUnmineFail:", ntx.Hash.String(), res)
			common.CountSafePar("TxUnmineFail-", res)
			os.Exit(1)
		}
	}
	c, b := removeExcessiveTxs() // now we can limit the mempool size, if it went too far
	if c > 0 || b > 0 {
		println(c, b, "txs removed while undoing block", bl.Height, bl.Hash.String())
	}
	TxMutex.Unlock()
}
