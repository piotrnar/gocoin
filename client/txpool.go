package main

import (
	"log"
	"time"
	"sync"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	FeePerKB = 10000  // in satoshis
	TxExpireAfter = time.Hour
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
	sentCount uint
	time.Time
	own bool
	spent []uint64 // Which records in SpentOutputs this TX added
}

type OneTxRejected struct {
	time.Time
	reason int
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
	NeedThisTx(tid, func() {
		tx, le := btc.NewTx(pl)
		if tx == nil {
			CountSafe("TxParseError")
			TransactionsRejected[tid.Hash] = &OneTxRejected{Time:time.Now(), reason:101}
			c.DoS()
			return
		}
		if le != len(pl) {
			CountSafe("TxParseLength")
			TransactionsRejected[tid.Hash] = &OneTxRejected{Time:time.Now(), reason:102}
			c.DoS()
			return
		}
		if len(tx.TxIn)<1 {
			CountSafe("TxParseEmpty")
			TransactionsRejected[tid.Hash] = &OneTxRejected{Time:time.Now(), reason:103}
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
			TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:200}
			tx_mutex.Unlock()
			CountSafe("TxRejectedDoubleSpend")
			//log.Println("ERROR: HandleNetTx No input", tx.TxIn[i].Input.String())
			return
		}

		pos[i], _ = BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		if pos[i] == nil {
			TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:201}
			tx_mutex.Unlock()
			CountSafe("TxRejectedNoInput")
			//log.Println("ERROR: HandleNetTx No input", tx.TxIn[i].Input.String())
			return
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
	}
	if totout > totinp {
		TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:202}
		tx_mutex.Unlock()
		ntx.conn.DoS()
		log.Println("ERROR: HandleNetTx Incorrect output values", totout, totinp)
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if fee < (uint64(len(ntx.raw))*FeePerKB)>>10 {
		TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:203}
		tx_mutex.Unlock()
		CountSafe("TxRejectedLowFee")
		//log.Println("ERROR: Tx fee too low", fee, len(ntx.raw))
		return
	}

	// Verify scripts
	for i := range tx.TxIn {
		if !btc.VerifyTxScript(tx.TxIn[i].ScriptSig, pos[i].Pk_script, i, tx) {
			TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:204}
			tx_mutex.Unlock()
			ntx.conn.DoS()
			log.Println("ERROR: HandleNetTx Invalid signature")
			return
		}
	}

	rec := &OneTxToSend{data:ntx.raw, spent:spent}
	TransactionsToSend[tx.Hash.Hash] = rec
	for i := range spent {
		SpentOutputs[spent[i]] = true
	}

	tx_mutex.Unlock()
	//log.Println("Accepted valid tx", tx.Hash.String())
	CountSafe("TxAccepted")

	rec.sentCount += NetRouteInv(1, tx.Hash, ntx.conn)
	rec.Time = time.Now()
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
	if ok && rec.own {
		return false // Assume own txs as non-trusted
	}
	if ok {
		CountSafe("ScriptsBoosted")
	}
	return ok
}


func txPoolManager() {
	btc.TrustedTxChecker = txChecker
	for {
		time.Sleep(60e9) // Wake up every minute
		expireTime := time.Now().Add(-TxExpireAfter)
		var cnt1, cnt2 uint64

		tx_mutex.Lock()
		for k, v := range TransactionsToSend {
			if v.Time.Before(expireTime) {
				delete(TransactionsToSend, k)
				cnt1++
			}
		}
		for k, v := range TransactionsRejected {
			if v.Time.Before(expireTime) {
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
