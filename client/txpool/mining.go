package txpool

import (
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

// outputsMined clears the MemInput flag of all the children (used when a tx is mined).
func (tx *OneTxToSend) outputsMined() {
	// Go through all the tx's outputs and unmark MemInputs in txs that have been spending it
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			if rec, ok := TransactionsToSend[val]; ok {
				if CheckForErrors() && rec.MemInputs == nil {
					common.CountSafe("TxMinedMeminER1")
					println("ERROR: ", po.String(), "just mined in", rec.Hash.String(), "- not marked as mem")
					continue
				}
				idx := rec.IIdx(uidx)
				if CheckForErrors() {
					if idx < 0 {
						common.CountSafe("TxMinedMeminER2")
						println("ERROR: ", po.String(), " just mined. Was in SpentOutputs & mempool, but DUPA")
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
					reduced_size := (len(rec.MemInputs) + 7) & ^7
					rec.MemInputs = nil
					rec.Footprint -= uint32(reduced_size)
					TransactionsToSendSize -= uint64(reduced_size)
				}
				rec.resortWithChildren()
			} else if CheckForErrors() {
				common.CountSafe("TxMinedMeminERR")
				println("ERROR:", po.String(), " in SpentOutputs, but not in mempool")
			}
		}
	}
}

// txMined is called for each tx mined in a new block.
func txMined(tx *btc.Tx) {
	bidx := tx.Hash.BIdx()

	if rec, ok := TransactionsToSend[bidx]; ok {
		// if we have this tx in mempool, remove it and it should clean everything up nicely
		common.CountSafe("TxMinedAccepted")
		rec.outputsMined()
		rec.Delete(false, 0) // this should take care of the RejectedUsedUTXOs stuff
		return
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
					DeleteRejectedByTxr(txr)
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
		return
	}

	if mr, ok := TransactionsRejected[bidx]; ok {
		common.CountSafePar("TxMinedRjctd-", mr.Reason)
		DeleteRejectedByTxr(mr)
		return
	}

	if TransactionsPending[bidx] {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, bidx)
	}
}

// BlockMined removes all the block's tx from the mempool.
func BlockMined(bl *btc.Block) {
	common.CountSafe("TxPkgsBlockMined")
	if len(bl.Txs) < 2 {
		return
	}

	wtgs := make([]*OneWaitingList, 0, len(bl.Txs)-1)
	TxMutex.Lock()
	FeePackagesDirty = true                // this will spare us all the struggle with trying to re-package each tx
	for i := len(bl.Txs) - 1; i > 0; i-- { // we go in reversed order to remove children before parents
		tx := bl.Txs[i]
		txMined(tx)
	}
	for _, tx := range bl.Txs[1:] {
		bidx := tx.Hash.BIdx()
		if wtg := WaitingForInputs[bidx]; wtg != nil {
			wtgs = append(wtgs, wtg)
		}
	}
	if len(wtgs) > 0 { // Try to redo waiting txs
		common.CountSafeAdd("TxMinedGotInput", uint64(len(wtgs)))
		for _, wtg := range wtgs {
			retryWaitingForInput(wtg)
		}
	}
	TxMutex.Unlock()

}

// outputsUnmined sets the MemInput flag of all the children (used when a tx is unmined / block undone).
func outputsUnmined(tx *btc.Tx) {
	// Go through all the tx's outputs and mark MemInputs in txs that have been spending it
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			if rec := TransactionsToSend[val]; rec != nil {
				if rec.MemInputs == nil {
					rec.MemInputs = make([]bool, len(rec.TxIn))
					extra_size := (len(rec.MemInputs) + 7) & ^7
					rec.Footprint += uint32(extra_size)
					TransactionsToSendSize += uint64(extra_size)
				}
				idx := rec.IIdx(uidx)
				rec.MemInputs[idx] = true
				rec.MemInputCnt++
				rec.resortWithChildren()
				common.CountSafe("TxPutBackMemIn")
				if CheckForErrors() && rec.Footprint != uint32(rec.SysSize()) {
					println("ERROR: MarkChildrenForMem footprint mismatch", rec.Footprint, uint32(rec.SysSize()))
				}
			} else if CheckForErrors() {
				println("ERROR: MarkChildrenForMem", po.String(), " in SpentOutputs, but not in mempool")
				common.CountSafe("TxPutBackMeminERR")
			}
		}
	}
}

func BlockUndone(bl *btc.Block) {
	common.CountSafe("TxPkgsBlockUndo")
	if len(bl.Txs) < 2 {
		return
	}

	TxMutex.Lock()
	FeePackagesDirty = true // this will spare us all the struggle with trying to re-package each tx
	for _, tx := range bl.Txs[1:] {
		ntx := &TxRcvd{Tx: tx, Trusted: true, Unmined: true}
		if need := needThisTxExt(&ntx.Hash, nil); need == 0 {
			if res, _ := processTx(ntx); res == 0 {
				common.CountSafe("TxUnmineOK")
			} else {
				common.CountSafePar("TxUnmineFail-", res)
			}
		} else {
			common.CountSafePar("TxUnmineNoNeed-", need)
		}
	}
	TxMutex.Unlock()
}
