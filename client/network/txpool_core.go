package network

import (
	"encoding/binary"
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
	TX_REJECTED_DISABLED = 1

	TX_REJECTED_TOO_BIG      = 101
	TX_REJECTED_FORMAT       = 102
	TX_REJECTED_LEN_MISMATCH = 103
	TX_REJECTED_EMPTY_INPUT  = 104

	TX_REJECTED_OVERSPEND = 154
	TX_REJECTED_BAD_INPUT = 157

	// Anything from the list below might eventually get mined
	TX_REJECTED_NO_TXOU     = 202
	TX_REJECTED_LOW_FEE     = 205
	TX_REJECTED_NOT_MINED   = 208
	TX_REJECTED_CB_INMATURE = 209
	TX_REJECTED_RBF_LOWFEE  = 210
	TX_REJECTED_RBF_FINAL   = 211
	TX_REJECTED_RBF_100     = 212
	TX_REJECTED_REPLACED    = 213
)

var (
	TxMutex sync.Mutex

	// The actual memory pool:
	TransactionsToSend       map[BIDX]*OneTxToSend = make(map[BIDX]*OneTxToSend)
	TransactionsToSendSize   uint64
	TransactionsToSendWeight uint64

	// All the outputs that are currently spent in TransactionsToSend:
	SpentOutputs map[uint64]BIDX = make(map[uint64]BIDX)

	// Transactions that we downloaded, but rejected:
	TransactionsRejected     map[BIDX]*OneTxRejected = make(map[BIDX]*OneTxRejected)
	TransactionsRejectedSize uint64                  // only include those that have *Tx pointer set

	// Transactions that are received from network (via "tx"), but not yet processed:
	TransactionsPending map[BIDX]bool = make(map[BIDX]bool)

	// Transactions that are waiting for inputs:
	WaitingForInputs     map[BIDX]*OneWaitingList = make(map[BIDX]*OneWaitingList)
	WaitingForInputsSize uint64
)

type OneTxToSend struct {
	Invsentcnt, SentCnt uint32
	Firstseen, Lastsent time.Time
	Local               bool
	Spent               []uint64 // Which records in SpentOutputs this TX added
	Volume, Fee         uint64
	*btc.Tx
	Blocked     byte   // if non-zero, it gives you the reason why this tx nas not been routed
	MemInputs   []bool // transaction is spending inputs from other unconfirmed tx(s)
	MemInputCnt int
	SigopsCost  uint64
	Final       bool // if true RFB will not work on it
	VerifyTime  time.Duration
}

type OneTxRejected struct {
	Id *btc.Uint256
	time.Time
	Size     uint32
	Reason   byte
	Waiting4 *btc.Uint256
	*btc.Tx
}

type OneWaitingList struct {
	TxID  *btc.Uint256
	TxLen uint32
	Ids   map[BIDX]time.Time // List of pending tx ids
}

func ReasonToString(reason byte) string {
	switch reason {
	case 0:
		return ""
	case TX_REJECTED_DISABLED:
		return "RELAY_OFF"
	case TX_REJECTED_TOO_BIG:
		return "TOO_BIG"
	case TX_REJECTED_FORMAT:
		return "FORMAT"
	case TX_REJECTED_LEN_MISMATCH:
		return "LEN_MISMATCH"
	case TX_REJECTED_EMPTY_INPUT:
		return "EMPTY_INPUT"
	case TX_REJECTED_OVERSPEND:
		return "OVERSPEND"
	case TX_REJECTED_BAD_INPUT:
		return "BAD_INPUT"
	case TX_REJECTED_NO_TXOU:
		return "NO_TXOU"
	case TX_REJECTED_LOW_FEE:
		return "LOW_FEE"
	case TX_REJECTED_NOT_MINED:
		return "NOT_MINED"
	case TX_REJECTED_CB_INMATURE:
		return "CB_INMATURE"
	case TX_REJECTED_RBF_LOWFEE:
		return "RBF_LOWFEE"
	case TX_REJECTED_RBF_FINAL:
		return "RBF_FINAL"
	case TX_REJECTED_RBF_100:
		return "RBF_100"
	case TX_REJECTED_REPLACED:
		return "REPLACED"
	}
	return fmt.Sprint("UNKNOWN_", reason)
}

func NeedThisTx(id *btc.Uint256, cb func()) (res bool) {
	return NeedThisTxExt(id, cb) == 0
}

// NeedThisTxExt returns false if we do not want to receive a data for this tx.
func NeedThisTxExt(id *btc.Uint256, cb func()) (why_not int) {
	TxMutex.Lock()
	if _, present := TransactionsToSend[id.BIdx()]; present {
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
		if (c.Node.Services & SERVICE_SEGWIT) != 0 {
			binary.LittleEndian.PutUint32(b[1:5], MSG_WITNESS_TX) // SegWit Tx
			//println(c.ConnID, "getdata", btc.NewUint256(hash).String())
		} else {
			b[1] = MSG_TX // Tx
		}
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}

// RejectTx adds a transaction to the rejected list or not, if it has been mined already.
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func RejectTx(tx *btc.Tx, why byte) *OneTxRejected {
	rec := new(OneTxRejected)
	rec.Time = time.Now()
	rec.Size = uint32(len(tx.Raw))
	rec.Reason = why

	// TODO: only store tx for selected reasons
	if why >= 200 {
		rec.Tx = tx
		rec.Id = &tx.Hash
		TransactionsRejectedSize += uint64(rec.Size)
	} else {
		rec.Id = new(btc.Uint256)
		rec.Id.Hash = tx.Hash.Hash
	}

	bidx := tx.Hash.BIdx()
	TransactionsRejected[bidx] = rec

	return rec
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
		RejectTx(tx, TX_REJECTED_TOO_BIG)
		TxMutex.Unlock()
		common.CountSafe("TxRejectedBig")
		return
	}

	NeedThisTx(&tx.Hash, func() {
		// This body is called with a locked TxMutex
		tx.Raw = pl
		select {
		case NetTxs <- &TxRcvd{conn: c, Tx: tx, trusted: c.X.Authorized}:
			TransactionsPending[tx.Hash.BIdx()] = true
		default:
			common.CountSafe("TxRejectedFullQ")
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

	TxMutex.Lock()

	if !retry {
		if _, present := TransactionsPending[bidx]; !present {
			// It had to be mined in the meantime, so just drop it now
			TxMutex.Unlock()
			common.CountSafe("TxNotPending")
			return
		}
		delete(TransactionsPending, bidx)
	} else {
		// In case case of retry, it is on the rejected list,
		// so remove it now to free any tied WaitingForInputs
		deleteRejected(bidx)
	}

	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	var rbf_tx_list map[*OneTxToSend]bool

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		if !final && tx.TxIn[i].Sequence >= 0xfffffffe {
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
				RejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedRBFFinal")
				return
			}

			rbf_tx_list[ctx] = true
			if !ntx.trusted && len(rbf_tx_list) > 100 {
				RejectTx(ntx.Tx, TX_REJECTED_RBF_100)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedRBF100+")
				return
			}

			chlds := ctx.GetAllChildren()
			for _, ctx = range chlds {
				if !ntx.trusted && ctx.Final {
					RejectTx(ntx.Tx, TX_REJECTED_RBF_FINAL)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedRBF_Final")
					return
				}

				rbf_tx_list[ctx] = true

				if !ntx.trusted && len(rbf_tx_list) > 100 {
					RejectTx(ntx.Tx, TX_REJECTED_RBF_100)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedRBF100+")
					return
				}
			}
		}

		if txinmem, ok := TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				RejectTx(ntx.Tx, TX_REJECTED_BAD_INPUT)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedBadInput")
				return
			}

			if !ntx.trusted && !common.CFG.TXPool.AllowMemInputs {
				RejectTx(ntx.Tx, TX_REJECTED_NOT_MINED)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedMemInput1")
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
				var newone bool

				if !common.CFG.TXPool.AllowMemInputs {
					RejectTx(ntx.Tx, TX_REJECTED_NOT_MINED)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedMemInput2")
					return
				}

				if rej, ok := TransactionsRejected[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
					if rej.Reason != TX_REJECTED_NO_TXOU || rej.Waiting4 == nil {
						RejectTx(ntx.Tx, TX_REJECTED_NO_TXOU)
						TxMutex.Unlock()
						common.CountSafe("TxRejectedParentRej")
						return
					}
					common.CountSafe("TxWait4ParentsParent")
				}

				// In this case, let's "save" it for later...
				missingid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
				nrtx := RejectTx(ntx.Tx, TX_REJECTED_NO_TXOU)

				if nrtx != nil && nrtx.Tx != nil {
					nrtx.Waiting4 = missingid
					//nrtx.Tx = ntx.Tx

					// Add to waiting list:
					var rec *OneWaitingList
					if rec, _ = WaitingForInputs[missingid.BIdx()]; rec == nil {
						rec = new(OneWaitingList)
						rec.TxID = missingid
						rec.TxLen = uint32(len(ntx.Raw))
						rec.Ids = make(map[BIDX]time.Time)
						newone = true
						WaitingForInputsSize += uint64(rec.TxLen)
					}
					rec.Ids[bidx] = time.Now()
					WaitingForInputs[missingid.BIdx()] = rec
				}

				TxMutex.Unlock()
				if newone {
					common.CountSafe("TxRejectedNoInpNew")
				} else {
					common.CountSafe("TxRejectedNoInpOld")
				}
				return
			} else {
				if pos[i].WasCoinbase {
					if common.Last.BlockHeight()+1-pos[i].BlockHeight < chain.COINBASE_MATURITY {
						RejectTx(ntx.Tx, TX_REJECTED_CB_INMATURE)
						TxMutex.Unlock()
						common.CountSafe("TxRejectedCBInmature")
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
		RejectTx(ntx.Tx, TX_REJECTED_OVERSPEND)
		TxMutex.Unlock()
		if ntx.conn != nil {
			ntx.conn.DoS("TxOverspend")
		}
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if !ntx.local && fee < (uint64(tx.VSize())*common.MinFeePerKB()/1000) { // do not check minimum fee for locally loaded txs
		RejectTx(ntx.Tx, TX_REJECTED_LOW_FEE)
		TxMutex.Unlock()
		common.CountSafe("TxRejectedLowFee")
		return
	}

	if rbf_tx_list != nil {
		var totweight int
		var totfees uint64

		for ctx := range rbf_tx_list {
			totweight += ctx.Weight()
			totfees += ctx.Fee
		}

		if !ntx.local && totfees*uint64(tx.Weight()) >= fee*uint64(totweight) {
			RejectTx(ntx.Tx, TX_REJECTED_RBF_LOWFEE)
			TxMutex.Unlock()
			common.CountSafe("TxRejectedRBFLowFee")
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

	if rbf_tx_list != nil {
		for ctx := range rbf_tx_list {
			// we dont remove with children because we have all of them on the list
			ctx.Delete(false, TX_REJECTED_REPLACED)
			common.CountSafe("TxRemovedByRBF")
		}
	}

	rec := &OneTxToSend{Spent: spent, Volume: totinp, Local: ntx.local,
		Fee: fee, Firstseen: time.Now(), Tx: tx, MemInputs: frommem, MemInputCnt: frommemcnt,
		SigopsCost: uint64(sigops), Final: final, VerifyTime: time.Now().Sub(start_time)}

	TransactionsToSend[bidx] = rec

	if maxpoolsize := common.MaxMempoolSize(); maxpoolsize != 0 {
		newsize := TransactionsToSendSize + uint64(len(rec.Raw))
		if TransactionsToSendSize < maxpoolsize && newsize >= maxpoolsize {
			expireTxsNow = true
		}
		TransactionsToSendSize = newsize
	} else {
		TransactionsToSendSize += uint64(len(rec.Raw))
	}
	TransactionsToSendWeight += uint64(rec.Tx.Weight())

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

func RetryWaitingForInput(wtg *OneWaitingList) {
	for k := range wtg.Ids {
		pendtxrcv := &TxRcvd{Tx: TransactionsRejected[k].Tx}
		if HandleNetTx(pendtxrcv, true) {
			common.CountSafe("TxRetryAccepted")
		} else {
			common.CountSafe("TxRetryRejected")
		}
	}
}

// Delete deletes the tx from the mempool.
// Deletes all the children as well if with_children is true.
// If reason is not zero, add the deleted txs to the rejected list.
// Make sure to call it with locked TxMutex.
func (tx *OneTxToSend) Delete(with_children bool, reason byte) {
	if with_children {
		// remove all the children that are spending from tx
		var po btc.TxPrevOut
		po.Hash = tx.Hash.Hash
		for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
			if so, ok := SpentOutputs[po.UIdx()]; ok {
				if child, ok := TransactionsToSend[so]; ok {
					child.Delete(true, reason)
				}
			}
		}
	}

	for i := range tx.Spent {
		delete(SpentOutputs, tx.Spent[i])
	}

	TransactionsToSendSize -= uint64(len(tx.Raw))
	TransactionsToSendWeight -= uint64(tx.Weight())
	delete(TransactionsToSend, tx.Hash.BIdx())
	if reason != 0 {
		RejectTx(tx.Tx, reason)
	}
}

func txChecker(tx *btc.Tx) bool {
	TxMutex.Lock()
	rec, ok := TransactionsToSend[tx.Hash.BIdx()]
	TxMutex.Unlock()
	if ok && rec.Local {
		common.CountSafe("TxScrOwn")
		return false // Assume own txs as non-trusted
	}
	if ok {
		ok = tx.WTxID().Equal(rec.WTxID())
		if !ok {
			//println("wTXID mismatch at", tx.Hash.String(), tx.WTxID().String(), rec.WTxID().String())
			common.CountSafe("TxScrSWErr")
		}
	}
	if ok {
		common.CountSafe("TxScrBoosted")
	} else {
		common.CountSafe("TxScrMissed")
	}
	return ok
}

// Make sure to call it with locked TxMutex
func deleteRejected(bidx BIDX) {
	if tr, ok := TransactionsRejected[bidx]; ok {
		if tr.Waiting4 != nil {
			w4i, _ := WaitingForInputs[tr.Waiting4.BIdx()]
			delete(w4i.Ids, bidx)
			if len(w4i.Ids) == 0 {
				WaitingForInputsSize -= uint64(w4i.TxLen)
				delete(WaitingForInputs, tr.Waiting4.BIdx())
			}
		}
		if tr.Tx != nil {
			TransactionsRejectedSize -= uint64(TransactionsRejected[bidx].Size)
		}
		delete(TransactionsRejected, bidx)
	}
}

func RemoveFromRejected(hash *btc.Uint256) {
	TxMutex.Lock()
	deleteRejected(hash.BIdx())
	TxMutex.Unlock()
}

func SubmitLocalTx(tx *btc.Tx, rawtx []byte) bool {
	return HandleNetTx(&TxRcvd{Tx: tx, trusted: true, local: true}, true)
}

func init() {
	chain.TrustedTxChecker = txChecker
}
