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

var (
	GetMPInProgressTicket = make(chan bool, 1)

	lastFeeAdjustedTime    time.Time
	CurrentFeeAdjustedSPKB uint64
)

type TxRcvd struct {
	*btc.Tx
	FeedbackCB func(uint32, byte, *OneTxToSend)
	FromCID    uint32
	Trusted    bool
	Local      bool
	Unmined    bool
}

// NeedThisTx returns 0 if mempool wants this tx.
// Before that, the given function is called (if not nil)
// Otherwise, it returns the reason why the tx is not wanted and no callback is called.
func NeedThisTxExt(id *btc.Uint256, cb func()) (why_not int) {
	TxMutex.Lock()
	why_not = needThisTxExt(id, cb)
	TxMutex.Unlock()
	return
}

// See description to NeedThisTxExt
// make sure to call this one with TxMutex Locked
func needThisTxExt(id *btc.Uint256, cb func()) (why_not int) {
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
	return
}

func processTx(ntx *TxRcvd) (byte, *OneTxToSend) {
	tx := ntx.Tx
	bidx := tx.Hash.BIdx()
	start_time := time.Now()
	var final bool // set to true if any of the inpits has a final sequence

	var totinp, totout uint64
	var frommem []bool
	var frommemcnt uint32

	common.CountSafe("Tx Procesed")

	if !ntx.Unmined && ntx.Weight() > int(common.Get(&common.CFG.TXPool.MaxTxWeight)) {
		rejectTx(tx, TX_REJECTED_TOO_BIG, nil)
		return TX_REJECTED_TOO_BIG, nil
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

			if !ntx.Unmined && !ntx.Trusted && ctx.Final {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
				return TX_REJECTED_RBF_FINAL, nil
			}

			rbf_tx_list[ctx] = true
			if !ntx.Unmined && !ntx.Trusted && len(rbf_tx_list) > 100 {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
				return TX_REJECTED_RBF_100, nil
			}

			chlds := ctx.GetAllChildren()
			for _, ctx = range chlds {
				if !ntx.Unmined && !ntx.Trusted && ctx.Final {
					rejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
					return TX_REJECTED_RBF_FINAL, nil
				}

				rbf_tx_list[ctx] = true

				if !ntx.Unmined && !ntx.Trusted && len(rbf_tx_list) > 100 {
					rejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
					return TX_REJECTED_RBF_100, nil
				}
			}
		}

		if txinmem, ok := TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				rejectTx(ntx.Tx, TX_REJECTED_BAD_INPUT, nil)
				return TX_REJECTED_BAD_INPUT, nil
			}

			if !ntx.Trusted && !common.CFG.TXPool.AllowMemInputs {
				rejectTx(ntx.Tx, TX_REJECTED_NOT_MINED, nil)
				return TX_REJECTED_NOT_MINED, nil
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
				if ntx.Unmined {
					println("ERROR: No UTXO for unmined tx", tx.TxIn[i].Input.String(), txinmem, ok)
					return TX_REJECTED_NO_TXOU, nil
				}

				if !ntx.Trusted && !common.CFG.TXPool.AllowMemInputs {
					rejectTx(ntx.Tx, TX_REJECTED_NOT_MINED, nil)
					return TX_REJECTED_NOT_MINED, nil
				}

				if rej, ok := TransactionsRejected[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
					if rej.Reason > 200 && rej.Waiting4 == nil {
						// The parent has been softly rejected but not by NO_TXOU
						// We will keep the data, in case the parent gets mined
						common.CountSafePar("TxRejBadParent-", rej.Reason)
						rejectTx(ntx.Tx, TX_REJECTED_BAD_PARENT, nil)
						return TX_REJECTED_BAD_PARENT, nil
					}
					common.CountSafe("TxWait4ParentsParent")
				}

				// In this case, let's "save" it for later...
				rejectTx(ntx.Tx, TX_REJECTED_NO_TXOU, btc.NewUint256(tx.TxIn[i].Input.Hash[:]))
				return TX_REJECTED_NO_TXOU, nil
			} else if !ntx.Unmined {
				if pos[i].WasCoinbase {
					if common.Last.BlockHeight()+1-pos[i].BlockHeight < chain.COINBASE_MATURITY {
						rejectTx(ntx.Tx, TX_REJECTED_CB_INMATURE, nil)
						fmt.Println(tx.Hash.String(), "trying to spend inmature coinbase block", pos[i].BlockHeight, "at", common.Last.BlockHeight())
						return TX_REJECTED_CB_INMATURE, nil
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
		return TX_REJECTED_OVERSPEND, nil
	}

	// Check for a proper fee
	fee := totinp - totout

	if !ntx.Unmined { // ignore low fees when puting back txs from unmined blocks
		if !ntx.Local && 4000*fee < uint64(tx.Weight())*common.MinFeePerKB() { // do not check minimum fee for locally loaded txs
			//rejectTx(ntx.Tx, TX_REJECTED_LOW_FEE, nil) - we do not store low fee txs in TransactionsRejected anymore
			common.CountSafe("TxRejected-LowFee") // we count it here
			return TX_REJECTED_LOW_FEE, nil
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
				return TX_REJECTED_RBF_LOWFEE, nil
			}
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
			if len(rbf_tx_list) > 0 {
				fmt.Println("RBF try", ver_err_cnt, "script(s) failed!")
				fmt.Print("> ")
			}
			return TX_REJECTED_SCRIPT_FAIL, nil
		}
	}

	for i := range tx.TxIn {
		if btc.IsP2SH(pos[i].Pk_script) {
			sigops += btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
		}
		sigops += uint(tx.CountWitnessSigOps(i, pos[i].Pk_script))
	}

	for len(rbf_tx_list) > 0 {
		var ctx *OneTxToSend
		for ctx = range rbf_tx_list {
			if ctx.HasNoChildren() {
				break
			}
		}
		if ctx == nil {
			println("ERROR: rbf_tx_list not empty, but cannot find a tx with no children")
			break
		}

		ctx.Delete(false, TX_REJECTED_REPLACED)
		delete(rbf_tx_list, ctx)
	}

	rec := &OneTxToSend{Volume: totinp, Local: ntx.Local, Fee: fee,
		Firstseen: start_time, Lastseen: start_time, Tx: tx, MemInputs: frommem, MemInputCnt: frommemcnt,
		SigopsCost: uint64(sigops), Final: final, VerifyTime: time.Since(start_time)}

	rec.Clean()
	rec.Add(bidx)
	if !ntx.Unmined {
		if wtg := WaitingForInputs[bidx]; wtg != nil {
			retryWaitingForInput(wtg) // Redo waiting txs when leaving this function
		}
		// do not remove any txs in the middle of block undo, to keep the mempool consistant
		removeExcessiveTxs()
	}

	common.CountSafe("TxAccepted")
	return 0, rec
}

// HandleNetTx must be called from the chain's thread.
func HandleNetTx(ntx *TxRcvd) bool {
	var result byte
	var t2s *OneTxToSend
	bidx := ntx.Hash.BIdx()

	TxMutex.Lock() // Make sure to Unlock it before each possible return
	if _, present := TransactionsPending[bidx]; !present {
		// It had to be mined in the meantime, so just drop it now
		common.CountSafe("TxNotPending")
		result = TX_REJECTED_NOT_PENDING
	} else {
		delete(TransactionsPending, bidx)
		result, t2s = processTx(ntx)
	}
	TxMutex.Unlock()

	if ntx.FeedbackCB != nil {
		ntx.FeedbackCB(ntx.FromCID, result, t2s)
	}

	return result == 0
}

func SubmitLocalTx(tx *btc.Tx, rawtx []byte) bool {
	TxMutex.Lock()
	// It may be on the rejected list, so remove it first
	DeleteRejectedByIdx(tx.Hash.BIdx())
	res, _ := processTx(&TxRcvd{Tx: tx, Trusted: true, Local: true})
	TxMutex.Unlock()
	return res == 0
}

func adjustMinimalFee() {
	if CurrentFeeAdjustedSPKB != 0 && time.Since(lastFeeAdjustedTime) > time.Minute {
		if TransactionsToSendSize < common.MaxMempoolSize() {
			feeAdjustDecrementSPKB := CurrentFeeAdjustedSPKB / 20
			if feeAdjustDecrementSPKB < 10 {
				feeAdjustDecrementSPKB = 10
			}
			if CurrentFeeAdjustedSPKB > feeAdjustDecrementSPKB {
				CurrentFeeAdjustedSPKB -= feeAdjustDecrementSPKB
			} else {
				CurrentFeeAdjustedSPKB = 0
			}
			if !common.SetMinFeePerKB(CurrentFeeAdjustedSPKB) {
				CurrentFeeAdjustedSPKB = 0 // stop decreasing if we can't get any lower
			}
		}
		lastFeeAdjustedTime = time.Now()
	}
}

var memcheck_hold bool

func Tick() {
	TxMutex.Lock()
	expireOldTxs()
	limitRejected()
	removeExcessiveTxs()
	adjustMinimalFee()

	if CheckForErrors() && !memcheck_hold && MempoolCheck() {
		println("******** Mempool check error inside the tick ********")
		memcheck_hold = true
	}
	TxMutex.Unlock()
}
