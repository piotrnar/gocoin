package txpool

import (
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
func txMined(tx *btc.Tx) {
	bidx := tx.Hash.BIdx()

	var was_rejected, was_inpool bool

	if rec, ok := TransactionsToSend[bidx]; ok {
		// if we have this tx in mempool, remove it and it should clean everything up nicely
		common.CountSafe("TxMinedAccepted")
		rec.mined()
		rec.Delete(false, 0) // NOTE: this does not take care of the RejectedSpentOutputs stuff
		// we will continue to check RejectedSpentOutputs and remove any rejected txs that are waiting for just-spent UTXOs ...
		was_inpool = true
	}

	for _, inp := range tx.TxIn {
		idx := inp.Input.UIdx()

		// if this tx was not in mempool, maybe another one is, that is spending the just-spent output(s)...
		// NOTE: if the tx was in mempool all relevant SpentOutputs records were purged by rec.Delete() above
		if !was_inpool {
			if val, ok := SpentOutputs[idx]; ok {
				// ... in such case, make sure to discard it, along with all its children
				if rec := TransactionsToSend[val]; rec != nil {
					// there is this one...
					common.CountSafePar("TxMinedUTXO-", rec.Local)
					rec.Delete(true, 0) // NOTE: this does not remove relevant RejectedSpentOutputs record(s)
					if CheckForErrors() {
						if _, ok := SpentOutputs[idx]; ok {
							println("ERROR: SpentOutput was supposed to be deleted, but still here\n  ", inp.Input.String())
						}
					}
				} else {
					println("ERROR: Input SpentOutputs, but tx not in mempool\n  ", inp.Input.String())
					delete(SpentOutputs, idx)
				}
			}
		}

		// check for the spent input in RejectedSpentOutputs
		if lst, ok := RejectedSpentOutputs[idx]; ok {
			// it is - remove all rejected tx that would use any of just mined inputs
			for _, rbidx := range lst {
				if txr, ok := TransactionsRejected[rbidx]; ok {
					txr.Delete()
					if rbidx == bidx {
						if was_inpool {
							common.CountSafePar("TxMinedAcptdA-", txr.Reason)
						} else {
							common.CountSafePar("TxMinedRjctdA-", txr.Reason)
						}
						was_rejected = true
					} else {
						if was_inpool {
							common.CountSafePar("TxMinedAcptdUTXO-", txr.Reason)
						} else {
							common.CountSafePar("TxMinedRjctdUTXO-", txr.Reason)

						}
					}
				} else {
					println("ERROR: UTXO present in RejectedSpentOutputs, not in TransactionsRejected\n  ", inp.Input.String())
				}
			}
			delete(RejectedSpentOutputs, idx) // this record will not be needed anymore
		}
	}

	if was_rejected || was_inpool {
		return
	}

	if mr, ok := TransactionsRejected[bidx]; ok {
		common.CountSafePar("TxMinedRjctd-", mr.Reason)
		mr.Delete()
		return
	}

	if TransactionsPending[bidx] {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, bidx)
	}
}

// BlockMined removes all the block's tx from the mempool.
func BlockMined(bl *btc.Block) {
	if len(bl.Txs) < 2 {
		return
	}

	TxMutex.Lock()
	FeePackagesDirty = true
	for i := len(bl.Txs) - 1; i > 0; i-- { // we go in reversed order to remove children before parents
		tx := bl.Txs[i]
		txMined(tx)
	}
	// now check if any mempool txs are waiting for inputs which were just mined
	for _, tx := range bl.Txs[1:] {
		txAccepted(tx.Hash.BIdx())
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
