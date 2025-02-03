package txpool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/script"
)

const (
	FEEDBACK_TX_BAD      = 1 // param is string
	FEEDBACK_TX_ACCEPTED = 2 // param is nil
	FEEDBACK_TX_ROUTABLE = 3 // param is FeedbackRoutable
)

var (
	GetMPInProgressTicket = make(chan bool, 1)
)

type TxRcvd struct {
	*btc.Tx
	FeedbackCB     func(connid uint32, info int, param interface{}) int
	FromCID        uint32
	Trusted, Local bool
}

type FeedbackRoutable struct {
	TxID *btc.Uint256
	SPKB uint64
}

func (t *TxRcvd) feedback(info int, param interface{}) (res int) {
	if t.FeedbackCB != nil {
		res = t.FeedbackCB(t.FromCID, info, param)
	}
	return
}

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
	full_rbf := !common.Get(&common.CFG.TXPool.NotFullRBF)

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

			if !ntx.Trusted && ctx.Final {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
				TxMutex.Unlock()
				return
			}

			rbf_tx_list[ctx] = true
			if !ntx.Trusted && len(rbf_tx_list) > 100 {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
				TxMutex.Unlock()
				return
			}

			chlds := ctx.GetAllChildren()
			for _, ctx = range chlds {
				if !ntx.Trusted && ctx.Final {
					rejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
					TxMutex.Unlock()
					return
				}

				rbf_tx_list[ctx] = true

				if !ntx.Trusted && len(rbf_tx_list) > 100 {
					rejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
					TxMutex.Unlock()
					return
				}
			}
		}

		if txinmem, ok := TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				rejectTx(ntx.Tx, TX_REJECTED_BAD_INPUT, nil)
				TxMutex.Unlock()
				return
			}

			if !ntx.Trusted && !common.CFG.TXPool.AllowMemInputs {
				rejectTx(ntx.Tx, TX_REJECTED_NOT_MINED, nil)
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
					rejectTx(ntx.Tx, TX_REJECTED_NOT_MINED, nil)
					TxMutex.Unlock()
					return
				}

				if rej, ok := TransactionsRejected[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
					if rej.Reason > 200 && rej.Waiting4 == nil {
						// The parent has been softly rejected but not by NO_TXOU
						// We will keep the data, in case the parent gets mined
						rejectTx(ntx.Tx, TX_REJECTED_BAD_PARENT, nil)
						TxMutex.Unlock()
						common.CountSafePar("TxRejBadParent-", rej.Reason)
						return
					}
					common.CountSafe("TxWait4ParentsParent")
				}

				// In this case, let's "save" it for later...
				rejectTx(ntx.Tx, TX_REJECTED_NO_TXOU, btc.NewUint256(tx.TxIn[i].Input.Hash[:]))
				TxMutex.Unlock()
				return
			} else {
				if pos[i].WasCoinbase {
					if common.Last.BlockHeight()+1-pos[i].BlockHeight < chain.COINBASE_MATURITY {
						rejectTx(ntx.Tx, TX_REJECTED_CB_INMATURE, nil)
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
		rejectTx(ntx.Tx, TX_REJECTED_OVERSPEND, nil)
		TxMutex.Unlock()
		ntx.feedback(FEEDBACK_TX_BAD, "TxOverspend")
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if !ntx.Local && fee < (uint64(tx.VSize())*common.MinFeePerKB()/1000) { // do not check minimum fee for locally loaded txs
		//RejectTx(ntx.Tx, TX_REJECTED_LOW_FEE, nil)  - we do not store low fee txs in TransactionsRejected anymore
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

		if !ntx.Local && totfees*uint64(tx.VSize()) >= fee*uint64(totvsize) {
			rejectTx(ntx.Tx, TX_REJECTED_RBF_LOWFEE, nil)
			TxMutex.Unlock()
			return
		}
	}

	sigops := btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount()

	if !ntx.Trusted { // Verify scripts
		var wg sync.WaitGroup
		var ver_err_cnt uint32
		ver_flags := common.CurrentScriptFlags()

		tx.AllocVerVars()
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
			ntx.feedback(FEEDBACK_TX_BAD, "TxScriptFail")
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
		// we dont remove with children because it can't have any in the mempool yet
		ctx.Delete(false, TX_REJECTED_REPLACED)
	}

	rec := &OneTxToSend{Volume: totinp, Local: ntx.Local, Fee: fee,
		Firstseen: start_time, Lastseen: start_time, Tx: tx, MemInputs: frommem, MemInputCnt: frommemcnt,
		SigopsCost: uint64(sigops), Final: final, VerifyTime: time.Since(start_time)}

	rec.Clean()
	rec.Footprint = uint32(rec.SysSize())
	rec.Add(bidx)

	for i := range spent {
		SpentOutputs[spent[i]] = bidx
	}

	wtg := WaitingForInputs[bidx]
	if wtg != nil {
		defer RetryWaitingForInput(wtg) // Redo waiting txs when leaving this function
	}

	TxMutex.Unlock()
	common.CountSafe("TxAccepted")

	ntx.feedback(FEEDBACK_TX_ACCEPTED, nil)

	if frommem != nil && !common.Get(&common.CFG.TXRoute.MemInputs) {
		// By default Gocoin does not route txs that spend unconfirmed inputs
		rec.Blocked = TX_REJECTED_NOT_MINED
		common.CountSafe("TxRouteNotMined")
	} else if !ntx.Local && rec.isRoutable() {
		// do not automatically route loacally loaded txs
		rec.Invsentcnt += uint32(ntx.feedback(FEEDBACK_TX_ROUTABLE,
			&FeedbackRoutable{TxID: &tx.Hash, SPKB: 4000 * fee / uint64(ntx.Weight())}))
		common.CountSafe("TxRouteOK")
	} else {
		common.CountSafe("TxRouteNO")
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
	if rec.Weight() > 4*int(common.Get(&common.CFG.TXRoute.MaxTxSize)) {
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
	return HandleNetTx(&TxRcvd{Tx: tx, Trusted: true, Local: true}, true)
}
