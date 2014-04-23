package network

import (
	"fmt"
	"time"
	"sync"
	"sync/atomic"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
)


const (
	TX_REJECTED_DISABLED     = 1

	TX_REJECTED_TOO_BIG      = 101
	TX_REJECTED_FORMAT       = 102
	TX_REJECTED_LEN_MISMATCH = 103
	TX_REJECTED_EMPTY_INPUT  = 104

	TX_REJECTED_DOUBLE_SPEND = 201
	TX_REJECTED_NO_TXOU      = 202
	TX_REJECTED_DUST         = 203
	TX_REJECTED_OVERSPEND    = 204
	TX_REJECTED_LOW_FEE      = 205
	TX_REJECTED_SCRIPT_FAIL  = 206
	TX_REJECTED_BAD_INPUT    = 207
	TX_REJECTED_NOT_MINED    = 208
)

var (
	TxMutex sync.Mutex

	// The actual memory pool:
	TransactionsToSend map[[btc.Uint256IdxLen]byte] *OneTxToSend =
		make(map[[btc.Uint256IdxLen]byte] *OneTxToSend)

	// All the outputs that are currently spent in TransactionsToSend:
	SpentOutputs map[uint64] [btc.Uint256IdxLen]byte =
		make(map[uint64] [btc.Uint256IdxLen]byte)

	// Transactions that we downloaded, but rejected:
	TransactionsRejected map[[btc.Uint256IdxLen]byte] *OneTxRejected =
		make(map[[btc.Uint256IdxLen]byte] *OneTxRejected)


	// Transactions that are received from network (via "tx"), but not yet processed:
	TransactionsPending map[[btc.Uint256IdxLen]byte] bool =
		make(map[[btc.Uint256IdxLen]byte] bool)

	// Transactions that are waiting for inputs:
	WaitingForInputs map[[btc.Uint256IdxLen]byte] *OneWaitingList =
		make(map[[btc.Uint256IdxLen]byte] *OneWaitingList)
)


type OneTxToSend struct {
	Data []byte
	Invsentcnt, SentCnt uint
	Firstseen, Lastsent time.Time
	Own byte // 0-not own, 1-own and OK, 2-own but with UNKNOWN input
	Spent []uint64 // Which records in SpentOutputs this TX added
	Volume, Fee, Minout uint64
	*btc.Tx
	Blocked byte // if non-zero, it gives you the reason why this tx nas not been routed
}


type Wait4Input struct {
	missingTx *btc.Uint256
	*TxRcvd
}

type OneTxRejected struct {
	Id *btc.Uint256
	time.Time
	Size uint32
	Reason byte
	*Wait4Input
}

type OneWaitingList struct {
	TxID *btc.Uint256
	Ids map[[btc.Uint256IdxLen]byte] time.Time  // List of pending tx ids
}


func VoutIdx(po *btc.TxPrevOut) (uint64) {
	return binary.LittleEndian.Uint64(po.Hash[:8]) ^ uint64(po.Vout)
}

// Return false if we do not want to receive a data for this tx
func NeedThisTx(id *btc.Uint256, cb func()) (res bool) {
	TxMutex.Lock()
	if _, present := TransactionsToSend[id.BIdx()]; present {
		//res = false
	} else if _, present := TransactionsRejected[id.BIdx()]; present {
		//res = false
	} else if _, present := TransactionsPending[id.BIdx()]; present {
		//res = false
	} else if txo, _ := common.BlockChain.Unspent.UnspentGet(&btc.TxPrevOut{Hash:id.Hash}); txo != nil {
		// This assumes that tx's out #0 has not been spent yet, which may not always be the case, but well...
		common.CountSafe("TxMinedRejected")
	} else {
		res = true
		if cb != nil {
			cb()
		}
	}
	TxMutex.Unlock()
	return
}


// Handle tx-inv notifications
func (c *OneConnection) TxInvNotify(hash []byte) {
	if NeedThisTx(btc.NewUint256(hash), nil) {
		var b [1+4+32]byte
		b[0] = 1 // One inv
		b[1] = 1 // Tx
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}


// Adds a transaction to the rejected list or not, it it has been mined already
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func RejectTx(id *btc.Uint256, size int, why byte) *OneTxRejected {
	rec := new(OneTxRejected)
	rec.Id = id
	rec.Time = time.Now()
	rec.Size = uint32(size)
	rec.Reason = why
	TransactionsRejected[id.BIdx()] = rec
	return rec
}


// Handle incoming "tx" msg
func (c *OneConnection) ParseTxNet(pl []byte) {
	tid := btc.NewSha2Hash(pl)
	NeedThisTx(tid, func() {
		// This body is called with a locked TxMutex
		if uint32(len(pl)) > atomic.LoadUint32(&common.CFG.TXPool.MaxTxSize) {
			common.CountSafe("TxTooBig")
			RejectTx(tid, len(pl), TX_REJECTED_TOO_BIG)
			return
		}
		tx, le := btc.NewTx(pl)
		if tx == nil {
			common.CountSafe("TxParseError")
			RejectTx(tid, len(pl), TX_REJECTED_FORMAT)
			c.DoS()
			return
		}
		if le != len(pl) {
			common.CountSafe("TxParseLength")
			RejectTx(tid, len(pl), TX_REJECTED_LEN_MISMATCH)
			c.DoS()
			return
		}
		if len(tx.TxIn)<1 {
			common.CountSafe("TxParseEmpty")
			RejectTx(tid, len(pl), TX_REJECTED_EMPTY_INPUT)
			c.DoS()
			return
		}

		tx.Hash = tid
		select {
			case NetTxs <- &TxRcvd{conn:c, tx:tx, raw:pl}:
				TransactionsPending[tid.BIdx()] = true
			default:
				common.CountSafe("NetTxsFULL")
		}
	})
}


func bidx2str(idx [btc.Uint256IdxLen]byte) string {
	return hex.EncodeToString(idx[:])
}


// Must be called from the chain's thread
func HandleNetTx(ntx *TxRcvd, retry bool) (accepted bool) {
	common.CountSafe("HandleNetTx")

	tx := ntx.tx
	var totinp, totout uint64
	var frommem bool

	TxMutex.Lock()

	if !retry {
		if _, present := TransactionsPending[tx.Hash.BIdx()]; !present {
			// It had to be mined in the meantime, so just drop it now
			TxMutex.Unlock()
			common.CountSafe("TxNotPending")
			return
		}
		delete(TransactionsPending, ntx.tx.Hash.BIdx())
	} else {
		// In case case of retry, it is on the rejected list,
		// ... so remove it now to free any tied WaitingForInputs
		deleteRejected(tx.Hash.BIdx())
	}

	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		spent[i] = VoutIdx(&tx.TxIn[i].Input)

		if _, ok := SpentOutputs[spent[i]]; ok {
			RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_DOUBLE_SPEND)
			TxMutex.Unlock()
			common.CountSafe("TxRejectedDoubleSpend")
			return
		}

		inptx := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
		if txinmem, ok := TransactionsToSend[inptx.BIdx()]; common.CFG.TXPool.AllowMemInputs && ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_BAD_INPUT)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedBadInput")
				return
			}
			pos[i] = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			common.CountSafe("TxInputInMemory")
			frommem = true
		} else {
			pos[i], _ = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if pos[i] == nil {
				var newone bool

				if !common.CFG.TXPool.AllowMemInputs {
					RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_NOT_MINED)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedMemInput")
					return
				}
				// In this case, let's "save" it for later...
				missingid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
				nrtx := RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_NO_TXOU)

				if nrtx != nil {
					nrtx.Wait4Input = &Wait4Input{missingTx: missingid, TxRcvd: ntx}

					// Add to waiting list:
					var rec *OneWaitingList
					if rec, _ = WaitingForInputs[nrtx.Wait4Input.missingTx.BIdx()]; rec==nil {
						rec = new(OneWaitingList)
						rec.TxID = nrtx.Wait4Input.missingTx
						rec.Ids = make(map[[btc.Uint256IdxLen]byte] time.Time)
						newone = true
					}
					rec.Ids[tx.Hash.BIdx()] = time.Now()
					WaitingForInputs[nrtx.Wait4Input.missingTx.BIdx()] = rec
				}

				TxMutex.Unlock()
				if newone {
					common.CountSafe("TxRejectedNoInputNew")
				} else {
					common.CountSafe("TxRejectedNoInputOld")
				}
				return
			}
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	minout := uint64(btc.MAX_MONEY)
	for i := range tx.TxOut {
		if tx.TxOut[i].Value < atomic.LoadUint64(&common.CFG.TXPool.MinVoutValue) {
			RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_DUST)
			TxMutex.Unlock()
			common.CountSafe("TxRejectedDust")
			return
		}
		if tx.TxOut[i].Value < minout {
			minout = tx.TxOut[i].Value
		}
		totout += tx.TxOut[i].Value
	}


	if totout > totinp {
		RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_OVERSPEND)
		TxMutex.Unlock()
		ntx.conn.DoS()
		common.CountSafe("TxRejectedOverspend")
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if fee < (uint64(len(ntx.raw)) * atomic.LoadUint64(&common.CFG.TXPool.FeePerByte)) {
		RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_LOW_FEE)
		TxMutex.Unlock()
		common.CountSafe("TxRejectedLowFee")
		return
	}

	// Verify scripts
	for i := range tx.TxIn {
		if !btc.VerifyTxScript(tx.TxIn[i].ScriptSig, pos[i].Pk_script, i, tx, true) {
			RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_SCRIPT_FAIL)
			TxMutex.Unlock()
			common.CountSafe("TxRejectedScriptFail")
			ntx.conn.DoS()
			return
		}
	}

	rec := &OneTxToSend{Data:ntx.raw, Spent:spent, Volume:totinp, Fee:fee, Firstseen:time.Now(), Tx:tx, Minout:minout}
	TransactionsToSend[tx.Hash.BIdx()] = rec
	for i := range spent {
		SpentOutputs[spent[i]] = tx.Hash.BIdx()
	}

	wtg := WaitingForInputs[tx.Hash.BIdx()]
	if wtg != nil {
		defer RetryWaitingForInput(wtg) // Redo waiting txs when leaving this function
	}

	TxMutex.Unlock()
	common.CountSafe("TxAccepted")

	if frommem {
		// Gocoin does not route txs that need unconfirmed inputs
		rec.Blocked = TX_REJECTED_NOT_MINED
		common.CountSafe("TxRouteNotMined")
	} else if isRoutable(rec) {
		rec.Invsentcnt += NetRouteInv(1, tx.Hash, ntx.conn)
		common.CountSafe("TxRouteOK")
	}

	accepted = true
	return
}


func isRoutable(rec *OneTxToSend) bool {
	if !common.CFG.TXRoute.Enabled {
		common.CountSafe("TxRouteDisabled")
		rec.Blocked = TX_REJECTED_DISABLED
		return false
	}
	if uint32(len(rec.Data)) > atomic.LoadUint32(&common.CFG.TXRoute.MaxTxSize) {
		common.CountSafe("TxRouteTooBig")
		rec.Blocked = TX_REJECTED_TOO_BIG
		return false
	}
	if rec.Fee < (uint64(len(rec.Data))*atomic.LoadUint64(&common.CFG.TXRoute.FeePerByte)) {
		common.CountSafe("TxRouteLowFee")
		rec.Blocked = TX_REJECTED_LOW_FEE
		return false
	}
	if rec.Minout < atomic.LoadUint64(&common.CFG.TXRoute.MinVoutValue) {
		common.CountSafe("TxRouteDust")
		rec.Blocked = TX_REJECTED_DUST
		return false
	}
	return true
}


func RetryWaitingForInput(wtg *OneWaitingList) {
	for k, t := range wtg.Ids {
		pendtxrcv := TransactionsRejected[k].Wait4Input.TxRcvd
		if HandleNetTx(pendtxrcv, true) {
			common.CountSafe("TxRetryAccepted")
			if common.DebugLevel>0 {
				fmt.Println(pendtxrcv.tx.Hash.String(), "accepted after", time.Now().Sub(t).String())
			}
		} else {
			common.CountSafe("TxRetryRejected")
			if common.DebugLevel>0 {
				fmt.Println(pendtxrcv.tx.Hash.String(), "still rejected", TransactionsRejected[k].Reason)
			}
		}
	}
}


// Make sure to call it with locked TxMutex
func deleteToSend(rec *OneTxToSend) {
	for i := range rec.Spent {
		delete(SpentOutputs, rec.Spent[i])
	}
	delete(TransactionsToSend, rec.Tx.Hash.BIdx())
}

func TxMined(tx *btc.Tx) {
	h := tx.Hash
	TxMutex.Lock()
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedToSend")
		for i := range rec.Spent {
			delete(SpentOutputs, rec.Spent[i])
		}
		delete(TransactionsToSend, h.BIdx())
	}
	if _, ok := TransactionsRejected[h.BIdx()]; ok {
		common.CountSafe("TxMinedRejected")
		deleteRejected(h.BIdx())
	}
	if _, ok := TransactionsPending[h.BIdx()]; ok {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, h.BIdx())
	}

	// Go through all the inputs and make sure we are not leaving them in SpentOutputs
	for i := range tx.TxIn {
		idx := VoutIdx(&tx.TxIn[i].Input)
		if val, ok := SpentOutputs[idx]; ok {
			if rec, _ := TransactionsToSend[val]; rec != nil {
				println("\007TxMined as", tx.Hash.String(), "instead of", rec.Tx.Hash.String())
				delete(TransactionsToSend, val)
				common.CountSafe("TxMinedMalleabled")
			} else {
				println("\007TxMined as", tx.Hash.String(), "but wasnt in mempool?")
				common.CountSafe("TxMinedInconsistent")
			}
			delete(SpentOutputs, idx)
		}
	}

	wtg := WaitingForInputs[h.BIdx()]
	TxMutex.Unlock()

	// Try to redo waiting txs
	if wtg != nil {
		common.CountSafe("TxMinedGotInput")
		RetryWaitingForInput(wtg)
	}
}


func txChecker(h *btc.Uint256) bool {
	TxMutex.Lock()
	rec, ok := TransactionsToSend[h.BIdx()]
	TxMutex.Unlock()
	if ok && rec.Own!=0 {
		return false // Assume own txs as non-trusted
	}
	if ok {
		common.CountSafe("TxScrBoosted")
	} else {
		common.CountSafe("TxScrMissed")
	}
	return ok
}


func init() {
	btc.TrustedTxChecker = txChecker
}


func expireTime(size int) (t time.Time) {
	if !common.CFG.TXPool.Enabled {
		return // return zero time which should expire immediatelly
	}
	exp := (time.Duration(size)*common.ExpirePerKB) >> 10
	if exp > common.MaxExpireTime {
		exp = common.MaxExpireTime
	}
	return time.Now().Add(-exp)
}


// Make sure to call it with locked TxMutex
func deleteRejected(bidx [btc.Uint256IdxLen]byte) {
	if tr, ok := TransactionsRejected[bidx]; ok {
		if tr.Wait4Input!=nil {
			w4i, _ := WaitingForInputs[tr.Wait4Input.missingTx.BIdx()]
			delete(w4i.Ids, bidx)
			if len(w4i.Ids)==0 {
				delete(WaitingForInputs, tr.Wait4Input.missingTx.BIdx())
			}
		}
		delete(TransactionsRejected, bidx)
	}
}


func ExpireTxs() {
	var cnt1a, cnt1b, cnt2 uint64

	TxMutex.Lock()
	for k, v := range TransactionsToSend {
		if v.Own==0 && v.Firstseen.Before(expireTime(len(v.Data))) {  // Do not expire own txs
			delete(TransactionsToSend, k)
			if v.Blocked==0 {
				cnt1a++
			} else {
				cnt1b++
			}
		}
	}
	for k, v := range TransactionsRejected {
		if v.Time.Before(expireTime(int(v.Size))) {
			deleteRejected(k)
			cnt2++
		}
	}
	TxMutex.Unlock()

	common.CounterMutex.Lock()
	common.Counter["TxPurgedTicks"]++
	if cnt1a>0 {
		common.Counter["TxPurgedOK"] += cnt1a
	}
	if cnt1b>0 {
		common.Counter["TxPurgedBlocked"] += cnt1b
	}
	if cnt2 > 0 {
		common.Counter["TxPurgedRejected"] += cnt2
	}
	common.CounterMutex.Unlock()
}
