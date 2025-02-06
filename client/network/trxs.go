package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (c *OneConnection) SendGetMP() error {
	if len(c.GetMP) == 0 {
		println("ERROR: SendGetMP() called with no GetMP lock")
		return nil
	}
	b := new(bytes.Buffer)
	txpool.TxMutex.Lock()
	if int(common.MaxMempoolSize()-txpool.TransactionsToSendSize) < 5e6 {
		// Don't send "getmp" messages, if we have less than 5MB of free space in mempool
		txpool.TxMutex.Unlock()
		c.cntInc("GetMPHold")
		return errors.New("SendGetMP: Mempool almost full")
	}
	tcnt := len(txpool.TransactionsToSend) + len(txpool.TransactionsRejected)
	if tcnt > MAX_GETMP_TXS {
		fmt.Println("Too many transactions in the current pool", tcnt, "/", MAX_GETMP_TXS)
		tcnt = MAX_GETMP_TXS
	}
	btc.WriteVlen(b, uint64(tcnt))
	var cnt int
	for k := range txpool.TransactionsToSend {
		b.Write(k[:])
		cnt++
		if cnt == MAX_GETMP_TXS {
			break
		}
	}
	for k := range txpool.TransactionsRejected {
		b.Write(k[:])
		cnt++
		if cnt == MAX_GETMP_TXS {
			break
		}
	}
	txpool.TxMutex.Unlock()
	return c.SendRawMsg("getmp", b.Bytes())
}

// TxInvNotify handles tx-inv notifications.
func (c *OneConnection) TxInvNotify(hash []byte) {
	if txpool.NeedThisTx(btc.NewUint256(hash), nil) {
		var b [1 + 4 + 32]byte
		b[0] = 1                                              // One inv
		binary.LittleEndian.PutUint32(b[1:5], MSG_WITNESS_TX) // SegWit Tx
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}

func isRoutable(rec *txpool.OneTxToSend) (yes bool, spkb uint64) {
	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	if !common.CFG.TXRoute.Enabled {
		common.CountSafe("TxRouteDisabled")
		rec.Blocked = txpool.TX_REJECTED_DISABLED
		return
	}
	if rec.Local {
		common.CountSafe("TxRouteLocal")
		return
	}
	if rec.MemInputCnt > 0 && !common.Get(&common.CFG.TXRoute.MemInputs) {
		common.CountSafe("TxRouteNotMined")
		rec.Blocked = txpool.TX_REJECTED_NOT_MINED
		return
	}

	if rec.Weight() > int(common.Get(&common.CFG.TXRoute.MaxTxWeight)) {
		common.CountSafe("TxRouteTooBig")
		rec.Blocked = txpool.TX_REJECTED_TOO_BIG
		return
	}
	spkb = 4000 * rec.Fee / uint64(rec.Weight())
	if spkb < common.RouteMinFeePerKB() {
		common.CountSafe("TxRouteLowFee")
		rec.Blocked = txpool.TX_REJECTED_LOW_FEE
		return
	}
	yes = true
	return
}

func txPoolCB(conid uint32, result byte, t2s *txpool.OneTxToSend) {
	Mutex_net.Lock()
	c := GetConnFromID(conid)
	Mutex_net.Unlock()
	if c == nil {
		// the connection has been closed since
		return
	}

	if result != 0 {
		if result == txpool.TX_REJECTED_OVERSPEND {
			c.DoS("TxOversend")
		} else if result == txpool.TX_REJECTED_SCRIPT_FAIL {
			c.DoS("TxScriptFail")
		} else {
			c.Mutex.Lock()
			c.cntInc(fmt.Sprint("TxRej", result))
			c.Mutex.Unlock()
		}
		return
	}

	c.Mutex.Lock()
	c.txsCha[c.txsCurIdx]++
	c.X.TxsReceived++
	c.Mutex.Unlock()

	if yes, spkb := isRoutable(t2s); yes {
		if cnt := NetRouteInvExt(MSG_TX, &t2s.Hash, c, spkb); cnt > 0 {
			atomic.AddUint32(&t2s.Invsentcnt, 1)
		}
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

	if tx.Weight() > int(common.Get(&common.CFG.TXPool.MaxTxWeight)) {
		txpool.RejectTx(tx, txpool.TX_REJECTED_TOO_BIG, nil)
		return
	}

	txpool.NeedThisTx(&tx.Hash, func() {
		// This body is called with a locked TxMutex
		tx.Raw = pl
		select {
		case NetTxs <- &txpool.TxRcvd{FeedbackCB: txPoolCB, FromCID: c.ConnID, Tx: tx, Trusted: c.X.Authorized}:
			txpool.TransactionsPending[tx.Hash.BIdx()] = true
		default:
			common.CountSafe("TxChannelFULL")
			//println("NetTxsFULL")
		}
	})
}

func (c *OneConnection) ProcessGetMP(pl []byte) {
	br := bytes.NewBuffer(pl)

	cnt, er := btc.ReadVLen(br)
	if er != nil {
		println("getmp message does not have the length field")
		c.DoS("GetMPError1")
		return
	}

	has_this_one := make(map[btc.BIDX]bool, cnt)
	for i := 0; i < int(cnt); i++ {
		var idx btc.BIDX
		if n, _ := br.Read(idx[:]); n != len(idx) {
			println("getmp message too short")
			c.DoS("GetMPError2")
			return
		}
		has_this_one[idx] = true
	}

	var data_sent_so_far int
	var redo [1]byte

	txpool.TxMutex.Lock()
	txs := txpool.GetSortedMempoolRBF() // we want to send parent txs first, thus the sorting
	for _, v := range txs {
		c.Mutex.Lock()
		bts := c.BytesToSent()
		c.Mutex.Unlock()
		if bts > SendBufSize/4 {
			redo[0] = 1
			break
		}
		if !has_this_one[v.Hash.BIdx()] {
			c.SendRawMsg("tx", v.Raw)
			data_sent_so_far += 24 + len(v.Raw)
		}
	}
	txpool.TxMutex.Unlock()

	c.SendRawMsg("getmpdone", redo[:])
}
