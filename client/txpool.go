package main

import (
	"log"
	"time"
	"sync"
	"github.com/piotrnar/gocoin/btc"
)


const (
	FeePerKb = 10000
)

var (
	TransactionsToSend map[[32]byte] *OneTxToSend = make(map[[32]byte] *OneTxToSend)
	TransactionsRejected map[[32]byte] *OneTxRejected = make(map[[32]byte] *OneTxRejected)

	// those that are already in the net queue:
	TransactionsPending map[[32]byte] bool = make(map[[32]byte] bool)

	tx_mutex sync.Mutex
)


type OneTxToSend struct {
	data []byte
	sentCount uint
	lastTime time.Time
	own bool
}

type OneTxRejected struct {
	time.Time
	reason int
}


// Return false if we do not want to receive a data fotr this tx
func NeedThisTx(id *btc.Uint256, unlockMutex bool) (res bool) {
	tx_mutex.Lock()
	if _, present := TransactionsToSend[id.Hash]; present {
		//res = false
	} else if _, present := TransactionsRejected[id.Hash]; present {
		//res = false
	} else if _, present := TransactionsPending[id.Hash]; present {
		//res = false
	} else {
		res = true
	}
	if unlockMutex {
		tx_mutex.Unlock()
	}
	return
}


// This transaction is not valid - add it to Rejected
func BanTx(id *btc.Uint256, reason int) {
	tx_mutex.Lock()
	TransactionsRejected[id.Hash] = &OneTxRejected{Time:time.Now(), reason:reason}
	tx_mutex.Unlock()
}


// Handle tx-inv notifications
func (c *oneConnection) TxInvNotify(hash []byte) {
	if NeedThisTx(btc.NewUint256(hash), true) {
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
	defer tx_mutex.Unlock() // so we order to not unlock it in NeedThisTx()
	if NeedThisTx(tid, false) {
		tx, le := btc.NewTx(pl)
		if tx == nil {
			//log.Println("ERROR: ParseTxNet Tx format")
			CountSafe("ParseTxNetError")
			BanTx(tid, 101)
			c.DoS()
			return
		}
		if le != len(pl) {
			CountSafe("ParseTxNetBadLen")
			//log.Println("ERROR: ParseTxNet length", le, len(pl))
			BanTx(tid, 102)
			c.DoS()
			return
		}
		if len(tx.TxIn)<1 {
			CountSafe("ParseTxNetNoInp")
			//log.Println("ERROR: ParseTxNet No inputs")
			BanTx(tid, 103)
			c.DoS()
			return
		}

		tx.Hash = tid
		TransactionsPending[tid.Hash] = true
		netTxs <- &txRcvd{conn:c, tx:tx, raw:pl}
	}
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

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		pos[i], _ = BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		if pos[i] == nil {
			TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:201}
			tx_mutex.Unlock()
			CountSafe("TxNoInput")
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
	fee := totout - totinp
	kb := uint64((len(ntx.raw)+1023)>>10)
	if fee < kb*FeePerKb {
		TransactionsRejected[tx.Hash.Hash] = &OneTxRejected{Time:time.Now(), reason:203}
		tx_mutex.Unlock()
		CountSafe("TxFeeTooLow")
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

	rec := &OneTxToSend{data:ntx.raw}
	TransactionsToSend[tx.Hash.Hash] = rec
	tx_mutex.Unlock()
	//log.Println("Accepted valid tx", tx.Hash.String())
	CountSafe("TxAccepted")

	rec.sentCount += NetRouteInv(1, tx.Hash, ntx.conn)
	rec.lastTime = time.Now()
}


func TxMined(h [32]byte) {
	tx_mutex.Lock()
	if _, ok := TransactionsToSend[h]; ok {
		CountSafe("TxMinedToSend")
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
