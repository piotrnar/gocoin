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
	FeedbackCB func(*TxRcvd, *OneTxToSend)
	FromCID    uint32
	Trusted    bool
	Local      bool
	Unmined    bool
	Result     byte // this is an output value (sent to callback from HandleNetTx)
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

// txChecked holds the things that checkTx() has found out about the tx.
type txChecked struct {
	pos         []*btc.TxOut          // the outputs being spent
	frommem     []bool                // which of them come from the mempool
	rbf_tx_list map[*OneTxToSend]bool // the txs (with all their children) that need to be replaced
	totinp, fee uint64
	frommemcnt  uint32
	final       bool // set to true if any of the inputs has a final sequence
}

// checkTx does all the checks of the tx that depend on the current state of the mempool and UTXO db,
// which is everything except for verifying the scripts.
// It does not change the mempool, unless the tx ends up on the rejected list.
// Set recheck to true when calling it again for the same tx.
// Make sure to call it with locked TxMutex.
func checkTx(ntx *TxRcvd, recheck bool) (byte, *txChecked) {
	tx := ntx.Tx
	chk := new(txChecked)
	var totout uint64

	if !ntx.Unmined && ntx.Weight() > int(common.Get(&common.CFG.TXPool.MaxTxWeight)) {
		rejectTx(tx, TX_REJECTED_TOO_BIG, nil)
		return TX_REJECTED_TOO_BIG, nil
	}

	// The same output must not be spent more than once (it would also count its value more than once)
	if len(tx.TxIn) > 1 {
		seen := make(map[btc.TxPrevOut]struct{}, len(tx.TxIn))
		for i := range tx.TxIn {
			if _, dup := seen[tx.TxIn[i].Input]; dup {
				common.CountSafe("TxInputDuplicate")
				rejectTx(tx, TX_REJECTED_BAD_INPUT, nil)
				return TX_REJECTED_BAD_INPUT, nil
			}
			seen[tx.TxIn[i].Input] = struct{}{}
		}
	}

	chk.pos = make([]*btc.TxOut, len(tx.TxIn))
	pos := chk.pos

	full_rbf := !common.Get(&common.CFG.TXPool.NotFullRBF)

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		if !full_rbf && !chk.final && tx.TxIn[i].Sequence >= 0xfffffffe {
			chk.final = true
		}

		if so, ok := SpentOutputs[tx.TxIn[i].Input.UIdx()]; ok {
			// Can only be accepted as RBF...

			if chk.rbf_tx_list == nil {
				chk.rbf_tx_list = make(map[*OneTxToSend]bool)
			}

			ctx := TransactionsToSend[so]

			if !ntx.Unmined && !ntx.Trusted && ctx.Final {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
				return TX_REJECTED_RBF_FINAL, nil
			}

			chk.rbf_tx_list[ctx] = true
			if !ntx.Unmined && !ntx.Trusted && len(chk.rbf_tx_list) > 100 {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_100, nil)
				return TX_REJECTED_RBF_100, nil
			}

			chlds := ctx.GetAllChildren()
			for _, ctx = range chlds {
				if !ntx.Unmined && !ntx.Trusted && ctx.Final {
					rejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL, nil)
					return TX_REJECTED_RBF_FINAL, nil
				}

				chk.rbf_tx_list[ctx] = true

				if !ntx.Unmined && !ntx.Trusted && len(chk.rbf_tx_list) > 100 {
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
			if !recheck {
				common.CountSafe("TxInputInMemory")
			}
			if chk.frommem == nil {
				chk.frommem = make([]bool, len(tx.TxIn))
			}
			chk.frommem[i] = true
			chk.frommemcnt++
		} else {
			pos[i] = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if pos[i] == nil {
				if ntx.Unmined {
					fmt.Println("ERROR: No UTXO for unmined tx", tx.TxIn[i].Input.String(), txinmem, ok)
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
						fmt.Println(tx.Hash.String(), "trying to spend immature coinbase block", pos[i].BlockHeight, "at", common.Last.BlockHeight())
						return TX_REJECTED_CB_INMATURE, nil
					}
				}
			}
		}
		chk.totinp += pos[i].Value
		if pos[i].Value > btc.MAX_MONEY || chk.totinp > btc.MAX_MONEY {
			rejectTx(ntx.Tx, TX_REJECTED_BAD_VALUE, nil)
			return TX_REJECTED_BAD_VALUE, nil
		}
	}

	// A replacement must not spend outputs of the txs that it is replacing (nor of their children),
	// as they are going to be removed from the mempool, leaving it with a non-existing input.
	if chk.rbf_tx_list != nil && chk.frommem != nil {
		for i := range tx.TxIn {
			if chk.frommem[i] && chk.rbf_tx_list[TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])]] {
				common.CountSafe("TxRBFSpendsReplaced")
				rejectTx(ntx.Tx, TX_REJECTED_BAD_INPUT, nil)
				return TX_REJECTED_BAD_INPUT, nil
			}
		}
	}

	// Do not let the clusters of connected txs grow too big, as the work needed to sort them for mining
	// (see pkgs.go) grows much faster than their size. The tx being checked counts as one, and the ones
	// that it replaces do not count. We do not limit the txs loaded locally, nor the ones from undone blocks.
	if max := int(common.Get(&common.CFG.TXPool.MaxClusterTxs)); max > 0 && chk.frommem != nil && !ntx.Unmined && !ntx.Local {
		parents := make([]*OneTxToSend, 0, chk.frommemcnt)
		for i := range tx.TxIn {
			if chk.frommem[i] {
				parents = append(parents, TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])])
			}
		}
		if clusterBiggerThan(parents, chk.rbf_tx_list, max-1) {
			rejectTx(ntx.Tx, TX_REJECTED_CLUSTER_BIG, nil)
			return TX_REJECTED_CLUSTER_BIG, nil
		}
	}

	// Check if total output value does not exceed total input
	for i := range tx.TxOut {
		if tx.TxOut[i].Value > btc.MAX_MONEY {
			rejectTx(ntx.Tx, TX_REJECTED_BAD_VALUE, nil)
			return TX_REJECTED_BAD_VALUE, nil
		}
		totout += tx.TxOut[i].Value
		if totout > btc.MAX_MONEY {
			rejectTx(ntx.Tx, TX_REJECTED_BAD_VALUE, nil)
			return TX_REJECTED_BAD_VALUE, nil
		}
	}

	if totout > chk.totinp {
		rejectTx(ntx.Tx, TX_REJECTED_OVERSPEND, nil)
		return TX_REJECTED_OVERSPEND, nil
	}

	// Check for a proper fee
	chk.fee = chk.totinp - totout

	if !ntx.Unmined { // ignore low fees when puting back txs from unmined blocks
		if !ntx.Local && 4000*chk.fee < uint64(tx.Weight())*common.MinFeePerKB() { // do not check minimum fee for locally loaded txs
			//rejectTx(ntx.Tx, TX_REJECTED_LOW_FEE, nil) - we do not store low fee txs in TransactionsRejected anymore
			common.CountSafe("TxRejected-LowFee") // we count it here
			return TX_REJECTED_LOW_FEE, nil
		}

		if chk.rbf_tx_list != nil && !ntx.Local {
			var totvsize int
			var totfees uint64

			for ctx := range chk.rbf_tx_list {
				totvsize += ctx.VSize()
				totfees += ctx.Fee
			}

			// the fee rate must be higher than of what is being replaced
			if totfees*uint64(tx.VSize()) >= chk.fee*uint64(totvsize) {
				rejectTx(ntx.Tx, TX_REJECTED_RBF_LOWFEE, nil)
				return TX_REJECTED_RBF_LOWFEE, nil
			}

			// It must also pay more in total: the fees of all the txs that it removes plus for its own relaying.
			// Otherwise a small tx could replace a big one (or a chain of them) for a fraction of its fee.
			if chk.fee < totfees+uint64(tx.Weight())*common.MinFeePerKB()/4000 {
				common.CountSafe("TxRBFLowTotalFee")
				rejectTx(ntx.Tx, TX_REJECTED_RBF_LOWFEE, nil)
				return TX_REJECTED_RBF_LOWFEE, nil
			}
		}
	}

	return 0, chk
}

// isTxFinal returns false if the tx could not be mined in the next block, because of its lock-time
// (absolute or relative). Such txs could stay in the mempool for a long time, without ever having to pay the fee.
// We do not check it for locally loaded txs, to let the user keep them here until they are ready.
// Checking a time based relative lock needs to walk down the chain, to the block that the spent output
// comes from. That is why we only do this after the scripts are verified - sending us a tx with an invalid
// script gets the peer banned, so it cannot be used to keep us busy for free.
func isTxFinal(ntx *TxRcvd, chk *txChecked) bool {
	tx := ntx.Tx
	if ntx.Unmined || ntx.Local || tx.Lock_time == 0 && tx.Version < 2 {
		return true
	}
	heights := make([]uint32, len(tx.TxIn)) // zero means: not mined yet
	for i := range tx.TxIn {
		if chk.frommem == nil || !chk.frommem[i] {
			heights[i] = chk.pos[i].BlockHeight
		}
	}
	csv := (common.CurrentScriptFlags() & script.VER_CSV) != 0
	return common.BlockChain.CheckTxFinalAtTip(tx, heights, csv) == nil
}

// used by verifyScripts() to keep script.DBG_ERR off, as long as any of them is running
var scriptsQuiet struct {
	sync.Mutex
	cnt  int
	prev bool
}

// verifyScripts returns true if the scripts of all the tx's inputs are fine.
// It does not need TxMutex.
func verifyScripts(tx *btc.Tx, pos []*btc.TxOut) bool {
	var wg sync.WaitGroup
	var ver_err_cnt uint32
	ver_flags := common.CurrentScriptFlags()

	scriptsQuiet.Lock()
	if scriptsQuiet.cnt == 0 {
		scriptsQuiet.prev = script.DBG_ERR
		script.DBG_ERR = false // keep quiet for incorrect txs
	}
	scriptsQuiet.cnt++
	scriptsQuiet.Unlock()

	tx.AllocVerVars()
	tx.Spent_outputs = pos
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

	scriptsQuiet.Lock()
	if scriptsQuiet.cnt--; scriptsQuiet.cnt == 0 {
		script.DBG_ERR = scriptsQuiet.prev
	}
	scriptsQuiet.Unlock()

	return ver_err_cnt == 0
}

// Only for the tests: called by processTxExt() right after it unlocks TxMutex, to change the mempool behind its back.
var testHookUnlocked func()

// Make sure to call it with locked TxMutex.
func processTx(ntx *TxRcvd) (byte, *OneTxToSend) {
	return processTxExt(ntx, false)
}

// processTxExt checks the tx and adds it to the mempool, if it is fine.
// Make sure to call it with locked TxMutex.
// If unlock is set, it releases TxMutex for the time of verifying the scripts (which takes most of the time),
// so other threads (e.g. the ones handling invs from the peers) do not have to wait for it.
// Use it only if the caller does not depend on the mempool staying unchanged across this call.
func processTxExt(ntx *TxRcvd, unlock bool) (byte, *OneTxToSend) {
	tx := ntx.Tx
	bidx := tx.Hash.BIdx()
	start_time := time.Now()

	common.CountSafe("Tx Procesed")

	res, chk := checkTx(ntx, false)
	if res != 0 {
		return res, nil
	}

	if !ntx.Trusted { // Verify scripts
		if unlock {
			TxMutex.Unlock()
			if testHookUnlocked != nil {
				testHookUnlocked()
			}
		}
		scripts_ok := verifyScripts(tx, chk.pos)
		if unlock {
			TxMutex.Lock()
		}

		if !scripts_ok {
			// not moving it to rejected, but baning the peer
			if len(chk.rbf_tx_list) > 0 {
				fmt.Println("RBF try - script(s) failed!")
				fmt.Print("> ")
			}
			return TX_REJECTED_SCRIPT_FAIL, nil
		}

		if unlock {
			// The mempool might have changed while it was unlocked (e.g. a parent was removed or
			// a conflicting tx was added), so everything that we have found out before must be checked again.
			// Mind that an input that still exists is the same one that the scripts were verified with.
			if res, chk = checkTx(ntx, true); res != 0 {
				common.CountSafePar("TxRecheckFailed-", res)
				return res, nil
			}
		}
	}

	if !isTxFinal(ntx, chk) {
		// like with the low fee, we do not store it in TransactionsRejected, as it may be fine later
		tx.Clean()
		common.CountSafe("TxRejected-NotFinal")
		return TX_REJECTED_NOT_FINAL, nil
	}

	sigops := btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount()
	for i := range tx.TxIn {
		if btc.IsP2SH(chk.pos[i].Pk_script) {
			sigops += btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
		}
		sigops += uint(tx.CountWitnessSigOps(i, chk.pos[i].Pk_script))
	}

	// remove the txs that are being replaced - children first
	for len(chk.rbf_tx_list) > 0 {
		var leaf *OneTxToSend
		for ctx := range chk.rbf_tx_list {
			if ctx.HasNoChildren() {
				leaf = ctx
				break
			}
		}
		if leaf == nil {
			fmt.Println("ERROR: rbf_tx_list not empty, but cannot find a tx with no children")
			break
		}

		leaf.Delete(false, TX_REJECTED_REPLACED)
		delete(chk.rbf_tx_list, leaf)
	}

	rec := &OneTxToSend{Volume: chk.totinp, Local: ntx.Local, Fee: chk.fee,
		Firstseen: start_time, Lastseen: start_time, Tx: tx, MemInputs: chk.frommem, MemInputCnt: chk.frommemcnt,
		SigopsCost: uint64(sigops), Final: chk.final, VerifyTime: time.Since(start_time)}

	rec.Clean()
	rec.Add(bidx)
	common.CountSafe("TxAccepted")
	return 0, rec
}

// OrphanAcceptedCB, if set, is called for each tx that had been waiting for its inputs (in TransactionsRejected)
// and has just been accepted to the mempool, so it can be announced to the peers.
// It is called with TxMutex unlocked.
var OrphanAcceptedCB func(*OneTxToSend)

// the txs to call OrphanAcceptedCB for, added by txAccepted()
var orphansAccepted []*OneTxToSend

// takeOrphansAccepted returns (and forgets) the txs to pass to notifyOrphansAccepted(), after unlocking TxMutex.
// Make sure to call it with locked TxMutex.
func takeOrphansAccepted() (res []*OneTxToSend) {
	for _, t2s := range orphansAccepted {
		if t2s.isInMap() { // it could have been removed already (e.g. by removeExcessiveTxs)
			res = append(res, t2s)
		}
	}
	orphansAccepted = nil
	return
}

// Make sure to call it with TxMutex unlocked.
func notifyOrphansAccepted(txs []*OneTxToSend) {
	if OrphanAcceptedCB != nil {
		for _, t2s := range txs {
			OrphanAcceptedCB(t2s)
		}
	}
}

// HandleNetTx must be called from the chain's thread.
func HandleNetTx(ntx *TxRcvd) bool {
	var result byte
	var t2s *OneTxToSend
	var orphans []*OneTxToSend
	bidx := ntx.Hash.BIdx()

	TxMutex.Lock() // Make sure to Unlock it before each possible return
	if _, present := TransactionsPending[bidx]; !present {
		// It had to be mined in the meantime, so just drop it now
		common.CountSafe("TxNotPending")
		result = TX_REJECTED_NOT_PENDING
	} else {
		// The tx stays pending while it is being processed, as TxMutex gets unlocked for a while
		// and we do not want anyone to download (or submit) the same tx again during this time.
		result, t2s = processTxExt(ntx, true)
		delete(TransactionsPending, bidx)
		if t2s != nil {
			txAccepted(bidx)
			removeExcessiveTxs()
			orphans = takeOrphansAccepted()
		}
	}
	TxMutex.Unlock()

	if ntx.FeedbackCB != nil {
		ntx.Result = result
		ntx.FeedbackCB(ntx, t2s)
	}
	notifyOrphansAccepted(orphans)

	return result == 0
}

func SubmitLocalTx(tx *btc.Tx, rawtx []byte) bool {
	var orphans []*OneTxToSend
	TxMutex.Lock()
	bidx := tx.Hash.BIdx()
	// It may be on the rejected list, so remove it first
	DeleteRejectedByIdx(bidx, false)
	res, t2s := processTx(&TxRcvd{Tx: tx, Trusted: true, Local: true})
	if t2s != nil {
		txAccepted(bidx)
		removeExcessiveTxs()
		orphans = takeOrphansAccepted()
	}
	TxMutex.Unlock()
	notifyOrphansAccepted(orphans)
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

func Tick() {
	TxMutex.Lock()
	expireOldTxs()
	limitRejected()
	removeExcessiveTxs()
	adjustMinimalFee()
	TxMutex.Unlock()
}
