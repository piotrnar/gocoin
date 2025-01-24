package network

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/script"
)

func NeedThisTx(id *btc.Uint256, cb func()) (res bool) {
	return NeedThisTxExt(id, cb) == 0
}

// NeedThisTxExt returns false if we do not want to receive a data for this tx.
func NeedThisTxExt(id *btc.Uint256, cb func()) (why_not int) {
	TxMutex.Lock()
	if tx, present := TransactionsToSend[id.BIdx()]; present {
		tx.Lastseen = time.Now()
		why_not = 1
	} else if _, present := TransactionsRejected[id.BIdx()]; present {
		why_not = 2
	} else if _, present := TransactionsPending[id.BIdx()]; present {
		why_not = 3
	} else if common.BlockChain.Unspent.TxPresent(id) {
		why_not = 4
		// This assumes that tx's out #0 has not been spent yet, which may not always be the case, but well...
		common.CountSafe("TxAlreadyMined")
	} else {
		// why_not = 0
		if cb != nil {
			cb()
		}
	}
	TxMutex.Unlock()
	return
}

// TxInvNotify handles tx-inv notifications.
func (c *OneConnection) TxInvNotify(hash []byte) {
	if NeedThisTx(btc.NewUint256(hash), nil) {
		var b [1 + 4 + 32]byte
		b[0] = 1 // One inv
		if (c.Node.Services & btc.SERVICE_SEGWIT) != 0 {
			binary.LittleEndian.PutUint32(b[1:5], MSG_WITNESS_TX) // SegWit Tx
			//println(c.ConnID, "getdata", btc.NewUint256(hash).String())
		} else {
			b[1] = MSG_TX // Tx
		}
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}

// ParseTxNet handles incoming "tx" messages.
func (c *OneConnection) ParseTxNet(pl []byte) {
	tx, le := btc.NewTx(pl)
	if tx == nil {
		c.DoS("TxRejectedBroken")
		return
	}
	if le != len(pl) {
		c.DoS("TxRejectedLenMismatch")
		return
	}
	if len(tx.TxIn) < 1 {
		c.Misbehave("TxRejectedNoInputs", 100)
		return
	}

	tx.SetHash(pl)

	if tx.Weight() > 4*int(common.GetUint32(&common.CFG.TXPool.MaxTxSize)) {
		TxMutex.Lock()
		RejectTx(tx, TX_REJECTED_TOO_BIG, nil)
		TxMutex.Unlock()
		return
	}

	NeedThisTx(&tx.Hash, func() {
		// This body is called with a locked TxMutex
		tx.Raw = pl
		select {
		case NetTxs <- &TxRcvd{conn: c, Tx: tx, trusted: c.X.Authorized}:
			TransactionsPending[tx.Hash.BIdx()] = true
		default:
			common.CountSafe("TxChannelFULL")
			//println("NetTxsFULL")
		}
	})
}

// HandleNetTx must be called from the chain's thread.
func HandleNetTx(ntx *TxRcvd, retry bool) (accepted bool) {
	common.CountSafe("HandleNetTx")

	tx := ntx.Tx
	bidx := tx.Hash.BIdx()
	start_time := time.Now()
	var final bool // set to true if any of the inpits has a final sequence

	var totinp, totout uint64
	var frommem []bool
	var frommemcnt int

	TxMutex.Lock() // Make sure to Unlock it before each possible return

	if !retry {
		if _, present := TransactionsPending[bidx]; !present {
			// It had to be mined in the meantime, so just drop it now
			TxMutex.Unlock()
			common.CountSafe("TxNotPending")
			return
		}
		delete(TransactionsPending, bidx)
	} else {
		// In case of retry, it is on the rejected list,
		// so remove it now to free any tied WaitingForInputs
		DeleteRejectedByIdx(bidx)
	}

	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	var rbf_tx_list map[*OneTxToSend]bool
	full_rbf := !common.GetBool(&common.CFG.TXPool.NotFullRBF)

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		if !full_rbf && !final && tx.TxIn[i].Sequence >= 0xfffffffe {
			final = true
		}

		spent[i] = tx.TxIn[i].Input.UIdx()

		if so, ok := SpentOutputs[spent[i]]; ok {
			// Can only be accepted as RBF...

			if rbf_tx_list == nil {
				rbf_tx_list = make(map[*OneTxToSend]bool)
			}

			ctx := TransactionsToSend[so]

			if !ntx.trusted && ctx.Final {
				RejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
				TxMutex.Unlock()
				return
			}

			rbf_tx_list[ctx] = true
			if !ntx.trusted && len(rbf_tx_list) > 100 {
				RejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
				TxMutex.Unlock()
				return
			}

			chlds := ctx.GetAllChildren()
			for _, ctx = range chlds {
				if !ntx.trusted && ctx.Final {
					RejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
					TxMutex.Unlock()
					return
				}

				rbf_tx_list[ctx] = true

				if !ntx.trusted && len(rbf_tx_list) > 100 {
					RejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
					TxMutex.Unlock()
					return
				}
			}
		}

		if txinmem, ok := TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				RejectTx(ntx.Tx, TX_REJECTED_BAD_INPUT, nil)
				TxMutex.Unlock()
				return
			}

			if !ntx.trusted && !common.CFG.TXPool.AllowMemInputs {
				RejectTx(ntx.Tx, TX_REJECTED_NOT_MINED, nil)
				TxMutex.Unlock()
				return
			}

			pos[i] = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			common.CountSafe("TxInputInMemory")
			if frommem == nil {
				frommem = make([]bool, len(tx.TxIn))
			}
			frommem[i] = true
			frommemcnt++
		} else {
			pos[i] = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if pos[i] == nil {
				if !common.CFG.TXPool.AllowMemInputs {
					RejectTx(ntx.Tx, TX_REJECTED_NOT_MINED, nil)
					TxMutex.Unlock()
					return
				}

				if rej, ok := TransactionsRejected[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
					if rej.Reason > 200 && rej.Waiting4 == nil {
						// The parent has been softly rejected but not by NO_TXOU
						// We will keep the data, in case the parent gets mined
						RejectTx(ntx.Tx, TX_REJECTED_BAD_PARENT, nil)
						TxMutex.Unlock()
						if !common.NoCounters.Get() {
							common.CountSafe(fmt.Sprint("TxRejBadParent-", rej.Reason))
						}
						return
					}
					common.CountSafe("TxWait4ParentsParent")
				}

				// In this case, let's "save" it for later...
				RejectTx(ntx.Tx, TX_REJECTED_NO_TXOU, btc.NewUint256(tx.TxIn[i].Input.Hash[:]))
				TxMutex.Unlock()
				return
			} else {
				if pos[i].WasCoinbase {
					if common.Last.BlockHeight()+1-pos[i].BlockHeight < chain.COINBASE_MATURITY {
						RejectTx(ntx.Tx, TX_REJECTED_CB_INMATURE, nil)
						TxMutex.Unlock()
						fmt.Println(tx.Hash.String(), "trying to spend inmature coinbase block", pos[i].BlockHeight, "at", common.Last.BlockHeight())
						return
					}
				}
			}
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
	}

	if totout > totinp {
		RejectTx(ntx.Tx, TX_REJECTED_OVERSPEND, nil)
		TxMutex.Unlock()
		if ntx.conn != nil {
			ntx.conn.DoS("TxOverspend")
		}
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if !ntx.local && fee < (uint64(tx.VSize())*common.MinFeePerKB()/1000) { // do not check minimum fee for locally loaded txs
		RejectTx(ntx.Tx, TX_REJECTED_LOW_FEE, nil)
		TxMutex.Unlock()
		return
	}

	if rbf_tx_list != nil {
		var totvsize int
		var totfees uint64

		for ctx := range rbf_tx_list {
			totvsize += ctx.VSize()
			totfees += ctx.Fee
		}

		if !ntx.local && totfees*uint64(tx.VSize()) >= fee*uint64(totvsize) {
			RejectTx(ntx.Tx, TX_REJECTED_RBF_LOWFEE, nil)
			TxMutex.Unlock()
			return
		}
	}

	sigops := btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount()

	if !ntx.trusted { // Verify scripts
		var wg sync.WaitGroup
		var ver_err_cnt uint32
		ver_flags := common.CurrentScriptFlags()

		tx.Spent_outputs = pos
		prev_dbg_err := script.DBG_ERR
		script.DBG_ERR = false // keep quiet for incorrect txs
		for i := range tx.TxIn {
			wg.Add(1)
			go func(i int, tx *btc.Tx) {
				if !script.VerifyTxScript(tx.Spent_outputs[i].Pk_script,
					&script.SigChecker{Amount: tx.Spent_outputs[i].Value, Idx: i, Tx: tx}, ver_flags) {
					atomic.AddUint32(&ver_err_cnt, 1)
				}
				wg.Done()
			}(i, tx)
		}

		wg.Wait()
		script.DBG_ERR = prev_dbg_err

		if ver_err_cnt > 0 {
			// not moving it to rejected, but baning the peer
			TxMutex.Unlock()
			if ntx.conn != nil {
				ntx.conn.DoS("TxScriptFail")
			}
			if len(rbf_tx_list) > 0 {
				fmt.Println("RBF try", ver_err_cnt, "script(s) failed!")
				fmt.Print("> ")
			}
			return
		}
	}

	for i := range tx.TxIn {
		if btc.IsP2SH(pos[i].Pk_script) {
			sigops += btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
		}
		sigops += uint(tx.CountWitnessSigOps(i, pos[i].Pk_script))
	}

	for ctx := range rbf_tx_list {
		// we dont remove with children because we have all of them on the list
		ctx.Delete(false, TX_REJECTED_REPLACED)
	}

	tx.Fee = fee
	rec := &OneTxToSend{Volume: totinp, Local: ntx.local,
		Firstseen: start_time, Lastseen: start_time, Tx: tx, MemInputs: frommem, MemInputCnt: frommemcnt,
		SigopsCost: uint64(sigops), Final: final, VerifyTime: time.Since(start_time)}

	if f, _ := os.OpenFile("bidx_list.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660); f != nil {
		fmt.Fprintf(f, "%s: %s %s\n", time.Now().Format("01/02 15:04:05"), btc.BIdxString(bidx), tx.Hash.String())
		f.Close()
	}
	TransactionsToSend[bidx] = rec
	tx.Clean()

	if maxpoolsize := common.MaxMempoolSize(); maxpoolsize != 0 {
		newsize := TransactionsToSendSize + uint64(len(tx.Raw))
		if TransactionsToSendSize < maxpoolsize && newsize >= maxpoolsize {
			limitTxpoolSizeNow = true
		}
		TransactionsToSendSize = newsize
	} else {
		TransactionsToSendSize += uint64(len(tx.Raw))
	}
	TransactionsToSendWeight += uint64(tx.Weight())

	for i := range spent {
		SpentOutputs[spent[i]] = bidx
	}

	wtg := WaitingForInputs[bidx]
	if wtg != nil {
		defer RetryWaitingForInput(wtg) // Redo waiting txs when leaving this function
	}

	TxMutex.Unlock()
	common.CountSafe("TxAccepted")

	if frommem != nil && !common.GetBool(&common.CFG.TXRoute.MemInputs) {
		// By default Gocoin does not route txs that spend unconfirmed inputs
		rec.Blocked = TX_REJECTED_NOT_MINED
		common.CountSafe("TxRouteNotMined")
	} else if !ntx.trusted && rec.isRoutable() {
		// do not automatically route loacally loaded txs
		rec.Invsentcnt += NetRouteInvExt(1, &tx.Hash, ntx.conn, 1000*fee/uint64(len(ntx.Raw)))
		common.CountSafe("TxRouteOK")
	}

	if ntx.conn != nil {
		ntx.conn.Mutex.Lock()
		ntx.conn.txsCur++
		ntx.conn.X.TxsReceived++
		ntx.conn.Mutex.Unlock()
	}

	accepted = true
	return
}

func (rec *OneTxToSend) isRoutable() bool {
	if !common.CFG.TXRoute.Enabled {
		common.CountSafe("TxRouteDisabled")
		rec.Blocked = TX_REJECTED_DISABLED
		return false
	}
	if rec.Weight() > 4*int(common.GetUint32(&common.CFG.TXRoute.MaxTxSize)) {
		common.CountSafe("TxRouteTooBig")
		rec.Blocked = TX_REJECTED_TOO_BIG
		return false
	}
	if rec.Fee < (uint64(rec.VSize()) * common.RouteMinFeePerKB() / 1000) {
		common.CountSafe("TxRouteLowFee")
		rec.Blocked = TX_REJECTED_LOW_FEE
		return false
	}
	return true
}

func SubmitLocalTx(tx *btc.Tx, rawtx []byte) bool {
	return HandleNetTx(&TxRcvd{Tx: tx, trusted: true, local: true}, true)
}
