package main

import (
	"log"
	"time"
	"sync"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	TX_REJECTED_TOO_BIG      = 101
	TX_REJECTED_FORMAT       = 102
	TX_REJECTED_LEN_MISMATCH = 103
	TX_REJECTED_NO_INPUTS    = 104

	TX_REJECTED_DOUBLE_SPEND = 201
	TX_REJECTED_NO_INPUT     = 202
	TX_REJECTED_DUST         = 203
	TX_REJECTED_OVERSPEND    = 204
	TX_REJECTED_LOW_FEE      = 205
	TX_REJECTED_SCRIPT_FAIL  = 206
)

var (
	tx_mutex sync.Mutex
	TransactionsToSend map[[32]byte] *OneTxToSend = make(map[[32]byte] *OneTxToSend)
	TransactionsRejected map[[32]byte] *OneTxRejected = make(map[[32]byte] *OneTxRejected)
	TransactionsPending map[[32]byte] bool = make(map[[32]byte] bool)
	SpentOutputs map[uint64] bool = make(map[uint64] bool)
)


type OneTxToSend struct {
	data []byte
	sentcnt uint
	firstseen, lastsent time.Time
	own byte // 0-not own, 1-own and OK, 2-own but with UNKNOWN input
	spent []uint64 // Which records in SpentOutputs this TX added
	volume, fee uint64
}

type OneTxRejected struct {
	time.Time
	size uint32
	reason byte
}


func NewRejectedTx(size int, why byte) (result *OneTxRejected) {
	result = new(OneTxRejected)
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
	} else if _, present := TransactionsRejected[id.Hash]; present {
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
	if uint(len(pl))>CFG.TXRouting.MaxTxSize {
		CountSafe("TxTooBig")
		TransactionsRejected[tid.Hash] = NewRejectedTx(len(pl), TX_REJECTED_TOO_BIG)
		return
	}
	NeedThisTx(tid, func() {
		tx, le := btc.NewTx(pl)
		if tx == nil {
			CountSafe("TxParseError")
			TransactionsRejected[tid.Hash] = NewRejectedTx(len(pl), TX_REJECTED_FORMAT)
			c.DoS()
			return
		}
		if le != len(pl) {
			CountSafe("TxParseLength")
			TransactionsRejected[tid.Hash] = NewRejectedTx(len(pl), TX_REJECTED_LEN_MISMATCH)
			c.DoS()
			return
		}
		if len(tx.TxIn)<1 {
			CountSafe("TxParseEmpty")
			TransactionsRejected[tid.Hash] = NewRejectedTx(len(pl), TX_REJECTED_NO_INPUTS)
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


// Must be called from the chain's thread
func HandleNetTx(ntx *txRcvd) {
	CountSafe("HandleNetTx")

	tx_mutex.Lock()

	if _, present := TransactionsPending[ntx.tx.Hash.Hash]; !present {
		// It had to be mined in the meantime, so just drop it now
		tx_mutex.Unlock()
		CountSafe("TxNotPending")
		return
	}

	delete(TransactionsPending, ntx.tx.Hash.Hash)

	tx := ntx.tx
	var totinp, totout uint64
	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		spent[i] = VoutIdx(&tx.TxIn[i].Input)

		if _, ok := SpentOutputs[spent[i]]; ok {
			TransactionsRejected[tx.Hash.Hash] = NewRejectedTx(len(ntx.raw), TX_REJECTED_DOUBLE_SPEND)
			tx_mutex.Unlock()
			CountSafe("TxRejectedDoubleSpend")
			return
		}

		pos[i], _ = BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		if pos[i] == nil {
			TransactionsRejected[tx.Hash.Hash] = NewRejectedTx(len(ntx.raw), TX_REJECTED_NO_INPUT)
			tx_mutex.Unlock()
			CountSafe("TxRejectedNoInput")
			return
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	for i := range tx.TxOut {
		if tx.TxOut[i].Value < uint64(CFG.TXRouting.MinVoutValue) {
			TransactionsRejected[tx.Hash.Hash] = NewRejectedTx(len(ntx.raw), TX_REJECTED_DUST)
			tx_mutex.Unlock()
			CountSafe("TxRejectedDust")
			return
		}
		totout += tx.TxOut[i].Value
	}
	if totout > totinp {
		TransactionsRejected[tx.Hash.Hash] = NewRejectedTx(len(ntx.raw), TX_REJECTED_OVERSPEND)
		tx_mutex.Unlock()
		ntx.conn.DoS()
		CountSafe("TxRejectedOverspend")
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if fee < (uint64(len(ntx.raw))*uint64(CFG.TXRouting.FeePerByte)) {
		TransactionsRejected[tx.Hash.Hash] = NewRejectedTx(len(ntx.raw), TX_REJECTED_LOW_FEE)
		tx_mutex.Unlock()
		CountSafe("TxRejectedLowFee")
		//log.Println("ERROR: Tx fee too low", fee, len(ntx.raw))
		return
	}

	// Verify scripts
	for i := range tx.TxIn {
		if !btc.VerifyTxScript(tx.TxIn[i].ScriptSig, pos[i].Pk_script, i, tx) {
			TransactionsRejected[tx.Hash.Hash] = NewRejectedTx(len(ntx.raw), TX_REJECTED_SCRIPT_FAIL)
			tx_mutex.Unlock()
			CountSafe("TxRejectedScriptFail")
			ntx.conn.DoS()
			log.Println("ERROR: HandleNetTx Invalid signature")
			return
		}
	}

	rec := &OneTxToSend{data:ntx.raw, spent:spent, volume:totinp, fee:fee, firstseen:time.Now()}
	TransactionsToSend[tx.Hash.Hash] = rec
	for i := range spent {
		SpentOutputs[spent[i]] = true
	}

	tx_mutex.Unlock()
	//log.Println("Accepted valid tx", tx.Hash.String())
	CountSafe("TxAccepted")

	rec.sentcnt += NetRouteInv(1, tx.Hash, ntx.conn)
	rec.lastsent = time.Now()
}


func TxMined(h [32]byte) {
	tx_mutex.Lock()
	if rec, ok := TransactionsToSend[h]; ok {
		CountSafe("TxMinedToSend")
		for i := range rec.spent {
			delete(SpentOutputs, rec.spent[i])
		}
		delete(TransactionsToSend, h)
	}
	if _, ok := TransactionsRejected[h]; ok {
		CountSafe("TxMinedRejected")
		delete(TransactionsRejected, h)
	}
	if _, ok := TransactionsPending[h]; ok {
		CountSafe("TxMinedPending")
		delete(TransactionsPending, h)
	}
	tx_mutex.Unlock()
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
	return time.Now().Add(-time.Duration((uint64(size)*uint64(time.Minute)*uint64(CFG.TXRouting.TxExpirePerKB))>>10))
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
				delete(TransactionsRejected, k)
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
