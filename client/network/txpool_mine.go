package network

import (
	"bytes"
	"errors"
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
				if rec.MemInputs == nil {
					common.CountSafe("TxMinedMeminER1")
					fmt.Println("WTF?", po.String(), "just mined in", rec.Hash.String(), "- not marked as mem")
					continue
				}
				idx := rec.IIdx(uidx)
				if idx < 0 {
					common.CountSafe("TxMinedMeminER2")
					fmt.Println("WTF?", po.String(), " just mined. Was in SpentOutputs & mempool, but DUPA")
					continue
				}
				rec.MemInputs[idx] = false
				rec.MemInputCnt--
				common.CountSafe("TxMinedMeminOut")
				if rec.MemInputCnt == 0 {
					common.CountSafe("TxMinedMeminTx")
					rec.MemInputs = nil
				}
			} else {
				common.CountSafe("TxMinedMeminERR")
				fmt.Println("WTF?", po.String(), " in SpentOutputs, but not in mempool")
			}
		}
	}
}

// tx_mined is called for each tx mined in a new block.
func tx_mined(tx *btc.Tx) {
	h := tx.Hash
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedToSend")
		rec.UnMarkChildrenForMem()
		rec.Delete(false, 0)
	}
	if mr, ok := TransactionsRejected[h.BIdx()]; ok {
		common.CountSafe(fmt.Sprint("TxMinedROK-", mr.Tx != nil))
		delete(TransactionsPending, h.BIdx())
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
			} else {
				common.CountSafe("Tx**MinedSpentERROR")
				fmt.Println("WTF? Input from ", inp.Input.String(), " in SpentOutputs, but tx not in mempool")
			}
			delete(SpentOutputs, idx)
		}

		// 2. remove data of any rejected txs that use this input
		if lst, ok := RejectedUsedUTXOs[idx]; ok {
			for _, bidx := range lst {
				if txr, ok := TransactionsRejected[bidx]; ok {
					common.CountSafe(fmt.Sprint("TxMinedROK-", txr.Tx != nil))
					delete(TransactionsPending, bidx)
				} else {
					common.CountSafe("Tx***MinedRTxIn-NoT2S")
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

	limitTxpoolSizeNow = true
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
				common.CountSafe("TxPutBackMemIn")
			} else {
				common.CountSafe("TxPutBackMeminERR")
				fmt.Println("MarkChildrenForMem WTF?", po.String(), " in SpentOutputs, but not in mempool")
			}
		}
	}
}

func BlockUndone(bl *btc.Block) {
	var cnt int
	for _, tx := range bl.Txs[1:] {
		// put it back into the mempool
		ntx := &TxRcvd{Tx: tx, trusted: true}

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

		// TODO: make sure to set MemInputs of ones using it back to true (issue #58)
		MarkChildrenForMem(tx)
	}
	if cnt != len(bl.Txs)-1 {
		println("WARNING: network.BlockUndone("+bl.Hash.String()+") - ", cnt, "of", len(bl.Txs)-1, "txs put back")
	}
}

func (c *OneConnection) SendGetMP() error {
	if len(c.GetMP) == 0 {
		// TODO: remove it at some point (should not be happening)
		println("ERROR: SendGetMP() called with no GetMP lock")
		return nil
	}
	b := new(bytes.Buffer)
	TxMutex.Lock()
	if TransactionsToSendSize > 7*common.MaxMempoolSize()/8 {
		// Don't send "getmp" messages, if the mempool if more than 7/8 full
		TxMutex.Unlock()
		c.cntInc("GetMPHold")
		return errors.New("SendGetMP: Mempool more than half full")
	}
	tcnt := len(TransactionsToSend) + len(TransactionsRejected)
	if tcnt > MAX_GETMP_TXS {
		fmt.Println("Too many transactions in the current pool", tcnt, "/", MAX_GETMP_TXS)
		tcnt = MAX_GETMP_TXS
	}
	btc.WriteVlen(b, uint64(tcnt))
	var cnt int
	for k := range TransactionsToSend {
		b.Write(k[:])
		cnt++
		if cnt == MAX_GETMP_TXS {
			break
		}
	}
	for k := range TransactionsRejected {
		b.Write(k[:])
		cnt++
		if cnt == MAX_GETMP_TXS {
			break
		}
	}
	TxMutex.Unlock()
	return c.SendRawMsg("getmp", b.Bytes())
}

func (c *OneConnection) ProcessGetMP(pl []byte) {
	br := bytes.NewBuffer(pl)

	cnt, er := btc.ReadVLen(br)
	if er != nil {
		println("getmp message does not have the length field")
		c.DoS("GetMPError1")
		return
	}

	has_this_one := make(map[BIDX]bool, cnt)
	for i := 0; i < int(cnt); i++ {
		var idx BIDX
		if n, _ := br.Read(idx[:]); n != len(idx) {
			println("getmp message too short")
			c.DoS("GetMPError2")
			return
		}
		has_this_one[idx] = true
	}

	var data_sent_so_far int
	var redo [1]byte

	TxMutex.Lock()
	for k, v := range TransactionsToSend {
		c.Mutex.Lock()
		bts := c.BytesToSent()
		c.Mutex.Unlock()
		if bts > SendBufSize/4 {
			redo[0] = 1
			break
		}
		if !has_this_one[k] {
			c.SendRawMsg("tx", v.Raw)
			data_sent_so_far += 24 + len(v.Raw)
		}
	}
	TxMutex.Unlock()

	c.SendRawMsg("getmpdone", redo[:])
}
