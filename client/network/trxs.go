package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (c *OneConnection) SendGetMP() error {
	if len(c.GetMP) == 0 {
		// TODO: remove it at some point (should not be happening)
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
		b[0] = 1 // One inv
		if (c.Node.Services & btc.SERVICE_SEGWIT) != 0 {
			binary.LittleEndian.PutUint32(b[1:5], MSG_WITNESS_TX) // SegWit Tx
			//println(c.ConnID, "getdata", btc.NewUint256(hash).String())
		} else {
			b[1] = MSG_TX // Tx
		}
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}

func txPoolCB(connid uint32, info string) int {
	return 0
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

	if tx.Weight() > 4*int(common.Get(&common.CFG.TXPool.MaxTxSize)) {
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
	txs := txpool.GetSortedMempool() // we want to send parent txs first, thus the sorting
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
