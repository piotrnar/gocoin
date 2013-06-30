package main

import (
	"log"
	"time"
	"sync"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
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
	TX_REJECTED_MEM_INPUT    = 207
	TX_REJECTED_NOT_MINED    = 208
)

var (
	tx_mutex sync.Mutex
	TransactionsToSend map[[32]byte] *OneTxToSend = make(map[[32]byte] *OneTxToSend)
	TransactionsRejected map[[btc.Uint256IdxLen]byte] *OneTxRejected =
		make(map[[btc.Uint256IdxLen]byte] *OneTxRejected)
	TransactionsPending map[[32]byte] bool = make(map[[32]byte] bool)
	WaitingForInputs map[[btc.Uint256IdxLen]byte] *OneWaitingList =
		make(map[[btc.Uint256IdxLen]byte] *OneWaitingList)
	SpentOutputs map[uint64] bool = make(map[uint64] bool)
)


type OneTxToSend struct {
	data []byte
	sentcnt uint
	firstseen, lastsent time.Time
	own byte // 0-not own, 1-own and OK, 2-own but with UNKNOWN input
	spent []uint64 // Which records in SpentOutputs this TX added
	volume, fee, minout uint64
	*btc.Tx
	blocked byte // if non-zero, it gives you the reason why this tx nas not been routed
}

type Wait4Input struct {
	missingTx *btc.Uint256
	*txRcvd
}

type OneTxRejected struct {
	id *btc.Uint256
	time.Time
	size uint32
	reason byte
	*Wait4Input
}

type OneWaitingList struct {
	TxID *btc.Uint256
	Ids map[[btc.Uint256IdxLen]byte] time.Time  // List of pending tx ids
}


func NewRejectedTx(id *btc.Uint256, size int, why byte) (result *OneTxRejected) {
	result = new(OneTxRejected)
	result.id = id
	result.Time = time.Now()
	result.size = uint32(size)
	result.reason = why
	return
}


func VoutIdx(po *btc.TxPrevOut) (uint64) {
	return binary.LittleEndian.Uint64(po.Hash[:8]) ^ uint64(po.Vout)
}

// Return false if we do not want to receive a data fotr this tx
func NeedThisTx(id *btc.Uint256, cb func()) (res bool) {
	tx_mutex.Lock()
	if _, present := TransactionsToSend[id.Hash]; present {
		//res = false
	} else if _, present := TransactionsRejected[id.BIdx()]; present {
		//res = false
	} else if _, present := TransactionsPending[id.Hash]; present {
		//res = false
	} else {
		res = true
		if cb != nil {
			cb()
		}
	}
	tx_mutex.Unlock()
	return
}


// Handle tx-inv notifications
func (c *oneConnection) TxInvNotify(hash []byte) {
	if NeedThisTx(btc.NewUint256(hash), nil) {
		var b [1+4+32]byte
		b[0] = 1 // One inv
		b[1] = 1 // Tx
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}


// Handle incomming "tx" msg
func (c *oneConnection) ParseTxNet(pl []byte) {
	tid := btc.NewSha2Hash(pl)
	if uint(len(pl))>CFG.TXPool.MaxTxSize {
		CountSafe("TxTooBig")
		TransactionsRejected[tid.BIdx()] = NewRejectedTx(tid, len(pl), TX_REJECTED_TOO_BIG)
		return
	}
	NeedThisTx(tid, func() {
		tx, le := btc.NewTx(pl)
		if tx == nil {
			CountSafe("TxParseError")
			TransactionsRejected[tid.BIdx()] = NewRejectedTx(tid, len(pl), TX_REJECTED_FORMAT)
			c.DoS()
			return
		}
		if le != len(pl) {
			CountSafe("TxParseLength")
			TransactionsRejected[tid.BIdx()] = NewRejectedTx(tid, len(pl), TX_REJECTED_LEN_MISMATCH)
			c.DoS()
			return
		}
		if len(tx.TxIn)<1 {
			CountSafe("TxParseEmpty")
			TransactionsRejected[tid.BIdx()] = NewRejectedTx(tid, len(pl), TX_REJECTED_EMPTY_INPUT)
			c.DoS()
			return
		}

		tx.Hash = tid
		select {
			case netTxs <- &txRcvd{conn:c, tx:tx, raw:pl}:
				TransactionsPending[tid.Hash] = true
			default:
				CountSafe("NetTxsFULL")
		}
	})
}


func bidx2str(idx [btc.Uint256IdxLen]byte) string {
	return hex.EncodeToString(idx[:])
}


// Must be called from the chain's thread
func HandleNetTx(ntx *txRcvd, retry bool) (accepted bool) {
	CountSafe("HandleNetTx")

	tx_mutex.Lock()

	if !retry {
		if _, present := TransactionsPending[ntx.tx.Hash.Hash]; !present {
			// It had to be mined in the meantime, so just drop it now
			tx_mutex.Unlock()
			CountSafe("TxNotPending")
			return
		}
		delete(TransactionsPending, ntx.tx.Hash.Hash)
	}

	tx := ntx.tx
	var totinp, totout uint64
	var frommem bool
	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		spent[i] = VoutIdx(&tx.TxIn[i].Input)

		if _, ok := SpentOutputs[spent[i]]; ok {
			TransactionsRejected[tx.Hash.BIdx()] = NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_DOUBLE_SPEND)
			tx_mutex.Unlock()
			CountSafe("TxRejectedDoubleSpend")
			return
		}

		if txinmem, ok := TransactionsToSend[tx.TxIn[i].Input.Hash]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				TransactionsRejected[tx.Hash.BIdx()] = NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_MEM_INPUT)
				tx_mutex.Unlock()
				CountSafe("TxRejectedMemInput")
				return
			}
			pos[i] = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			CountSafe("TxInputInMemory")
			frommem = true
		} else {
			pos[i], _ = BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if pos[i] == nil {
				// In this casem let's "save" it for later...
				missingid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
				nrtx := NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_NO_TXOU)
				nrtx.Wait4Input = &Wait4Input{missingTx: missingid, txRcvd: ntx}
				TransactionsRejected[tx.Hash.BIdx()] = nrtx

				// Add to waiting list:
				var rec *OneWaitingList
				var newone bool
				if rec, _ = WaitingForInputs[nrtx.Wait4Input.missingTx.BIdx()]; rec==nil {
					rec = new(OneWaitingList)
					rec.TxID = nrtx.Wait4Input.missingTx
					rec.Ids = make(map[[btc.Uint256IdxLen]byte] time.Time)
					newone = true
				}
				rec.Ids[tx.Hash.BIdx()] = time.Now()
				WaitingForInputs[nrtx.Wait4Input.missingTx.BIdx()] = rec

				tx_mutex.Unlock()
				if newone {
					CountSafe("TxRejectedNoInputUniq")
					if dbg > 0 {
						println("Tx No Input NEW", nrtx.Wait4Input.missingTx.String(),
							"->", bidx2str(tx.Hash.BIdx()), len(rec.Ids))
						ui_show_prompt()
					}
					//AskPeersForData(1, missingid)  // This does not seem to be helping at all
				} else {
					CountSafe("TxRejectedNoInputSame")
					if dbg > 0 {
						println("Tx No Input  + ", nrtx.Wait4Input.missingTx.String(),
							"->", bidx2str(tx.Hash.BIdx()), len(rec.Ids))
						ui_show_prompt()
					}
				}
				return
			}
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	minout := uint64(btc.MAX_MONEY)
	for i := range tx.TxOut {
		if tx.TxOut[i].Value < uint64(CFG.TXPool.MinVoutValue) {
			TransactionsRejected[tx.Hash.BIdx()] = NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_DUST)
			tx_mutex.Unlock()
			CountSafe("TxRejectedDust")
			return
		}
		if tx.TxOut[i].Value < minout {
			minout = tx.TxOut[i].Value
		}
		totout += tx.TxOut[i].Value
	}


	if totout > totinp {
		TransactionsRejected[tx.Hash.BIdx()] = NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_OVERSPEND)
		tx_mutex.Unlock()
		ntx.conn.DoS()
		CountSafe("TxRejectedOverspend")
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if fee < (uint64(len(ntx.raw))*uint64(CFG.TXPool.FeePerByte)) {
		TransactionsRejected[tx.Hash.BIdx()] = NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_LOW_FEE)
		tx_mutex.Unlock()
		CountSafe("TxRejectedLowFee")
		//log.Println("ERROR: Tx fee too low", fee, len(ntx.raw))
		return
	}

	// Verify scripts
	for i := range tx.TxIn {
		if !btc.VerifyTxScript(tx.TxIn[i].ScriptSig, pos[i].Pk_script, i, tx) {
			TransactionsRejected[tx.Hash.BIdx()] = NewRejectedTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_SCRIPT_FAIL)
			tx_mutex.Unlock()
			CountSafe("TxRejectedScriptFail")
			ntx.conn.DoS()
			log.Println("ERROR: HandleNetTx Invalid signature")
			return
		}
	}

	if retry {
		deleteRejected(tx.Hash.BIdx())
	}

	rec := &OneTxToSend{data:ntx.raw, spent:spent, volume:totinp, fee:fee, firstseen:time.Now(), Tx:tx, minout:minout}
	TransactionsToSend[tx.Hash.Hash] = rec
	for i := range spent {
		SpentOutputs[spent[i]] = true
	}

	wtg := WaitingForInputs[tx.Hash.BIdx()]

	tx_mutex.Unlock()
	//log.Println("Accepted valid tx", tx.Hash.String())
	CountSafe("TxAccepted")

	if frommem {
		// Gocoin does not route txs that use not mined inputs
		rec.blocked = TX_REJECTED_NOT_MINED
		CountSafe("TxRouteNotMined")
	} else if isRoutable(rec) {
		rec.sentcnt += NetRouteInv(1, tx.Hash, ntx.conn)
		rec.lastsent = time.Now()
		CountSafe("TxRouteOK")
	}

	// Try to redo waiting txs
	if wtg != nil {
		RetryWaitingForInput(wtg)
	}

	accepted = true
	return
}


func isRoutable(rec *OneTxToSend) bool {
	if !CFG.TXRoute.Enabled {
		CountSafe("TxRouteDisabled")
		rec.blocked = TX_REJECTED_DISABLED
		return false
	}
	if len(rec.data) > int(CFG.TXRoute.MaxTxSize) {
		CountSafe("TxRouteTooBig")
		rec.blocked = TX_REJECTED_TOO_BIG
		return false
	}
	if rec.fee < (uint64(len(rec.data))*uint64(CFG.TXRoute.FeePerByte)) {
		CountSafe("TxRouteLowFee")
		rec.blocked = TX_REJECTED_LOW_FEE
		return false
	}
	if rec.minout < uint64(CFG.TXRoute.MinVoutValue) {
		CountSafe("TxRouteDust")
		rec.blocked = TX_REJECTED_DUST
		return false
	}
	return true
}


func RetryWaitingForInput(wtg *OneWaitingList) {
	for k, t := range wtg.Ids {
		pdg := TransactionsRejected[k]
		if HandleNetTx(pdg.Wait4Input.txRcvd, true) {
			CountSafe("TxRetryAccepted")
			println(pdg.Wait4Input.txRcvd.tx.Hash.String(), "accepted after", time.Now().Sub(t).String())
		} else {
			CountSafe("TxRetryRejected")
			println(pdg.Wait4Input.txRcvd.tx.Hash.String(), "still rejected", TransactionsRejected[k].reason)
		}
		ui_show_prompt()
	}
}


func TxMined(h *btc.Uint256) {
	tx_mutex.Lock()
	if rec, ok := TransactionsToSend[h.Hash]; ok {
		CountSafe("TxMinedToSend")
		for i := range rec.spent {
			delete(SpentOutputs, rec.spent[i])
		}
		delete(TransactionsToSend, h.Hash)
	}
	if _, ok := TransactionsRejected[h.BIdx()]; ok {
		CountSafe("TxMinedRejected")
		deleteRejected(h.BIdx())
	}
	if _, ok := TransactionsPending[h.Hash]; ok {
		CountSafe("TxMinedPending")
		delete(TransactionsPending, h.Hash)
	}
	wtg := WaitingForInputs[h.BIdx()]
	tx_mutex.Unlock()

	// Try to redo waiting txs
	if wtg != nil {
		CountSafe("TxMinedGotInput")
		RetryWaitingForInput(wtg)
	}
}


func txChecker(h *btc.Uint256) bool {
	tx_mutex.Lock()
	rec, ok := TransactionsToSend[h.Hash]
	tx_mutex.Unlock()
	if ok && rec.own!=0 {
		return false // Assume own txs as non-trusted
	}
	if ok {
		CountSafe("TxScrBoosted")
	} else {
		CountSafe("TxScrMissed")
	}
	return ok
}


func init() {
	btc.TrustedTxChecker = txChecker
}


func expireTime(size int) time.Time {
	return time.Now().Add(-time.Duration((uint64(size)*uint64(time.Minute)*uint64(CFG.TXPool.TxExpirePerKB))>>10))
}


// Make sure to call it with locked tx_mutex
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


func txPoolManager() {
	for {
		time.Sleep(60e9) // Wake up every minute
		var cnt1, cnt2 uint64

		tx_mutex.Lock()
		for k, v := range TransactionsToSend {
			if v.own==0 && v.firstseen.Before(expireTime(len(v.data))) {  // Do not expire own txs
				delete(TransactionsToSend, k)
				cnt1++
			}
		}
		for k, v := range TransactionsRejected {
			if v.Time.Before(expireTime(int(v.size))) {
				deleteRejected(k)
				cnt2++
			}
		}
		tx_mutex.Unlock()

		counter_mutex.Lock()
		Counter["TxPurgedTicks"]++
		if cnt1 > 0 {
			Counter["TxPurgedToSend"] += cnt1
		}
		if cnt2 > 0 {
			Counter["TxPurgedRejected"] += cnt2
		}
		counter_mutex.Unlock()
	}
}
