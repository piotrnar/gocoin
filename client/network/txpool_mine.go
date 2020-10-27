package network

import (
	"bytes"
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
			if rec, _ := TransactionsToSend[val]; rec != nil {
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
func tx_mined(tx *btc.Tx) (wtg *OneWaitingList) {
	h := tx.Hash
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedToSend")
		rec.UnMarkChildrenForMem()
		rec.Delete(false, 0)
	}
	if mr, ok := TransactionsRejected[h.BIdx()]; ok {
		if mr.Tx != nil {
			common.CountSafe(fmt.Sprint("TxMinedROK-", mr.Reason))
		} else {
			common.CountSafe(fmt.Sprint("TxMinedRNO-", mr.Reason))
		}
		deleteRejected(h.BIdx())
	}
	if _, ok := TransactionsPending[h.BIdx()]; ok {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, h.BIdx())
	}

	// Go through all the inputs and make sure we are not leaving them in SpentOutputs
	for i := range tx.TxIn {
		idx := tx.TxIn[i].Input.UIdx()
		if val, ok := SpentOutputs[idx]; ok {
			if rec, _ := TransactionsToSend[val]; rec != nil {
				// if we got here, the txs has been Malleabled
				if rec.Local {
					common.CountSafe("TxMinedMalleabled")
					fmt.Println("Input from own ", rec.Tx.Hash.String(), " mined in ", tx.Hash.String())
				} else {
					common.CountSafe("TxMinedOtherSpend")
				}
				rec.Delete(true, 0)
			} else {
				common.CountSafe("TxMinedSpentERROR")
				fmt.Println("WTF? Input from ", rec.Tx.Hash.String(), " in mem-spent, but tx not in the mem-pool")
			}
			delete(SpentOutputs, idx)
		}
	}

	wtg = WaitingForInputs[h.BIdx()]
	return
}

// BlockMined removes all the block's tx from the mempool.
func BlockMined(bl *btc.Block) {
	wtgs := make([]*OneWaitingList, len(bl.Txs)-1)
	var wtg_cnt int
	TxMutex.Lock()
	for i := 1; i < len(bl.Txs); i++ {
		wtg := tx_mined(bl.Txs[i])
		if wtg != nil {
			wtgs[wtg_cnt] = wtg
			wtg_cnt++
		}
	}
	TxMutex.Unlock()

	// Try to redo waiting txs
	if wtg_cnt > 0 {
		common.CountSafeAdd("TxMinedGotInput", uint64(wtg_cnt))
		for _, wtg := range wtgs[:wtg_cnt] {
			RetryWaitingForInput(wtg)
		}
	}

	expireTxsNow = true
}

func (c *OneConnection) SendGetMP() error {
	b := new(bytes.Buffer)
	TxMutex.Lock()
	tcnt := len(TransactionsToSend) + len(TransactionsRejected)
	if tcnt > MAX_GETMP_TXS {
		fmt.Println("Too many transactions in the current pool", tcnt, "/", MAX_GETMP_TXS)
		tcnt = MAX_GETMP_TXS
	}
	btc.WriteVlen(b, uint64(tcnt))
	var cnt int
	for k, _ := range TransactionsToSend {
		b.Write(k[:])
		cnt++
		if cnt == MAX_GETMP_TXS {
			break
		}
	}
	for k, _ := range TransactionsRejected {
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
		if c.BytesToSent() > SendBufSize/4 {
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
