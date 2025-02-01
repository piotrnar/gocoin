package txpool

import (
	"fmt"

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

// UnMarkChildrenForMem clears the MemInput flag of all the children (used when a tx is mined).
func (tx *OneTxToSend) UnMarkChildrenForMem() {
	// Go through all the tx's outputs and unmark MemInputs in txs that have been spending it
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			if rec := TransactionsToSend[val]; rec != nil {
				if common.Get(&common.CFG.TXPool.CheckErrors) && rec.MemInputs == nil {
					common.CountSafe("TxMinedMeminER1")
					println("ERROR: ", po.String(), "just mined in", rec.Hash.String(), "- not marked as mem")
					continue
				}
				idx := rec.IIdx(uidx)
				if common.Get(&common.CFG.TXPool.CheckErrors) {
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
				common.CountSafe("TxMinedMeminOut")
				if rec.MemInputCnt == 0 {
					common.CountSafe("TxMinedMeminTx")
					reduxed_size := (len(rec.MemInputs) + 7) & ^7
					rec.MemInputs = nil
					rec.Footprint -= uint32(reduxed_size)
					TransactionsToSendSize -= uint64(reduxed_size)
				}
				rec.ResortWithChildren()
			} else if common.Get(&common.CFG.TXPool.CheckErrors) {
				common.CountSafe("TxMinedMeminERR")
				println("ERROR:", po.String(), " in SpentOutputs, but not in mempool")
			}
		}
	}
}

// tx_mined is called for each tx mined in a new block.
func tx_mined(tx *btc.Tx) {
	h := tx.Hash
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedAccepted")
		rec.UnMarkChildrenForMem()
		rec.Delete(false, 0)
	}
	if mr, ok := TransactionsRejected[h.BIdx()]; ok {
		common.CountSafePar("TxMinedRejected-", mr.Reason)
		DeleteRejectedByTxr(mr)
	}
	if _, ok := TransactionsPending[h.BIdx()]; ok {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, h.BIdx())
	}

	// now do through all the spent inputs and...
	for _, inp := range tx.TxIn {
		idx := inp.Input.UIdx()

		// 1. make sure we are not leaving them in SpentOutputs
		if val, ok := SpentOutputs[idx]; ok {
			if rec := TransactionsToSend[val]; rec != nil {
				// if we got here, the txs has been Malleabled
				if rec.Local {
					common.CountSafe("TxMinedMalleabled")
					fmt.Println("Input from own ", rec.Tx.Hash.String(), " mined in ", tx.Hash.String())
				} else {
					common.CountSafe("TxMinedOtherSpend")
				}
				rec.Delete(true, 0)
			} else if common.Get(&common.CFG.TXPool.CheckErrors) {
				println("ERROR: Input from ", inp.Input.String(), " in SpentOutputs, but tx not in mempool")
			}
			delete(SpentOutputs, idx)
		}

		// 2. remove data of any rejected txs that use this input
		if lst, ok := RejectedUsedUTXOs[idx]; ok {
			for _, bidx := range lst {
				if txr, ok := TransactionsRejected[bidx]; ok {
					common.CountSafePar("TxMinedRjctUTXO-", txr.Reason)
					DeleteRejectedByTxr(txr)
				} else if common.Get(&common.CFG.TXPool.CheckErrors) {
					println("ERROR: txr marked for removal but not present in TransactionsRejected")
				}
			}
			delete(RejectedUsedUTXOs, idx) // this record will not be needed anymore
		}
	}
}

// BlockMined removes all the block's tx from the mempool.
func BlockMined(bl *btc.Block) {
	wtgs := make([]*OneWaitingList, 0, len(bl.Txs)-1)
	TxMutex.Lock()
	for _, tx := range bl.Txs[1:] {
		tx_mined(tx)
	}
	for _, tx := range bl.Txs[1:] {
		bidx := tx.Hash.BIdx()
		if wtg := WaitingForInputs[bidx]; wtg != nil {
			wtgs = append(wtgs, wtg)
		}
	}
	TxMutex.Unlock()

	// Try to redo waiting txs
	if len(wtgs) > 0 {
		common.CountSafeAdd("TxMinedGotInput", uint64(len(wtgs)))
		for _, wtg := range wtgs {
			RetryWaitingForInput(wtg)
		}
	}
}

// MarkChildrenForMem sets the MemInput flag of all the children (used when a tx is mined).
func MarkChildrenForMem(tx *btc.Tx) {
	// Go through all the tx's outputs and mark MemInputs in txs that have been spending it
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			if rec := TransactionsToSend[val]; rec != nil {
				if rec.MemInputs == nil {
					rec.MemInputs = make([]bool, len(rec.TxIn))
				}
				idx := rec.IIdx(uidx)
				rec.MemInputs[idx] = true
				rec.MemInputCnt++
				rec.ResortWithChildren()
				common.CountSafe("TxPutBackMemIn")
			} else if common.Get(&common.CFG.TXPool.CheckErrors) {
				println("ERROR: MarkChildrenForMem", po.String(), " in SpentOutputs, but not in mempool")
				common.CountSafe("TxPutBackMeminERR")
			}
		}
	}
}

func BlockUndone(bl *btc.Block) {
	var cnt int
	for _, tx := range bl.Txs[1:] {
		// put it back into the mempool
		ntx := &TxRcvd{Tx: tx, Trusted: true}

		if NeedThisTx(&ntx.Hash, nil) {
			if HandleNetTx(ntx, true) {
				common.CountSafe("TxPutBackOK")
				cnt++
			} else {
				common.CountSafe("TxPutBackFail")
			}
		} else {
			common.CountSafe("TxPutBackNoNeed")
		}

		MarkChildrenForMem(tx)
	}
	if cnt != len(bl.Txs)-1 {
		println("WARNING: network.BlockUndone("+bl.Hash.String()+") - ", cnt, "of", len(bl.Txs)-1, "txs put back")
	}
}
