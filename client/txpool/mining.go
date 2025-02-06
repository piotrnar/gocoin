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

// tx_mined is called for each tx mined in a new block.
func tx_mined(tx *btc.Tx) {
	h := tx.Hash
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedAccepted")
		rec.outputsMined()
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
}

// BlockMined removes all the block's tx from the mempool.
func BlockMined(bl *btc.Block) {
	common.CountSafe("TxPkgsBlockMined")

	if len(bl.Txs) < 2 {
		return
	}

	wtgs := make([]*OneWaitingList, 0, len(bl.Txs)-1)
	TxMutex.Lock()
	for i := len(bl.Txs) - 1; i > 0; i-- { // we go in reversed order to remove children before parents
		tx := bl.Txs[i]
		FeePackagesDirty = true // this will spare us all the struggle with trying to re-package
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
	var cnt int
	common.CountSafe("TxPkgsBlockUndo")
	for _, tx := range bl.Txs[1:] {
		FeePackagesDirty = true
		// put it back into the mempool
		ntx := &TxRcvd{Tx: tx, Trusted: true, Retry: true, Unmined: true}

		if NeedThisTx(&ntx.Hash, nil) {
			if HandleNetTx(ntx) {
				common.CountSafe("TxPutBackOK")
				cnt++
			} else {
				common.CountSafe("TxPutBackFail")
			}
		} else {
			common.CountSafe("TxPutBackNoNeed")
		}

	}
	if cnt != len(bl.Txs)-1 {
		println("WARNING: network.BlockUndone("+bl.Hash.String()+") - only ", cnt, "of", len(bl.Txs)-1, "txs put back")
	}
}
