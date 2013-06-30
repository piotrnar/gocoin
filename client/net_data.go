package main

import (
	"fmt"
	"time"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


func (c *oneConnection) ProcessGetData(pl []byte) {
	//println(c.PeerAddr.Ip(), "getdata")
	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
		return
	}
	for i:=0; i<int(cnt); i++ {
		var typ uint32
		var h [32]byte

		e = binary.Read(b, binary.LittleEndian, &typ)
		if e != nil {
			println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
			return
		}

		n, _ := b.Read(h[:])
		if n!=32 {
			println("ProcessGetData: pl too short", c.PeerAddr.Ip())
			return
		}

		if typ == 2 {
			uh := btc.NewUint256(h[:])
			bl, _, er := BlockChain.Blocks.BlockGet(uh)
			if er == nil {
				c.SendRawMsg("block", bl)
			} else {
				//println("block", uh.String(), er.Error())
			}
		} else if typ == 1 {
			// transaction
			uh := btc.NewUint256(h[:])
			tx_mutex.Lock()
			if tx, ok := TransactionsToSend[uh.Hash]; ok && tx.blocked==0 {
				tx_mutex.Unlock()
				c.SendRawMsg("tx", tx.data)
				if dbg > 0 {
					println("sent tx to", c.PeerAddr.Ip())
				}
			} else {
				tx_mutex.Unlock()
			}
		} else {
			CountSafe(fmt.Sprint("GetdataFor",typ))
			if dbg>0 {
				println("getdata for type", typ, "not supported yet")
			}
		}
	}
}


func (c *oneConnection) GetBlockData(h []byte) {
	var b [1+4+32]byte
	b[0] = 1 // One inv
	b[1] = 2 // Block
	copy(b[5:37], h[:32])
	if dbg > 1 {
		println("GetBlockData", btc.NewUint256(h).String())
	}
	bh := btc.NewUint256(h)
	c.GetBlockInProgress[bh.BIdx()] = &oneBlockDl{hash:bh, start:time.Now()}
	c.SendRawMsg("getdata", b[:])
}


// This function is called from a net conn thread
func netBlockReceived(conn *oneConnection, b []byte) {
	bl, e := btc.NewBlock(b)
	if e != nil {
		conn.DoS()
		println("NewBlock:", e.Error())
		return
	}

	if _, ok := conn.GetBlockInProgress[bl.Hash.BIdx()]; ok {
		delete(conn.GetBlockInProgress, bl.Hash.BIdx())
	} else {
		CountSafe("UnxpectedBlockRcvd")
	}

	idx := bl.Hash.BIdx()
	mutex.Lock()
	if rb, got := receivedBlocks[idx]; got {
		rb.cnt++
		conn.BlockTiming.hash = nil
		CountSafe("SameBlockReceived")
		mutex.Unlock()
		return
	}
	receivedBlocks[idx] = &oneReceivedBlock{Time:time.Now()}
	mutex.Unlock()

	netBlocks <- &blockRcvd{conn:conn, bl:bl}
}


// Called from the blockchain thread
func HandleNetBlock(newbl *blockRcvd) {
	CountSafe("HandleNetBlock")
	bl := newbl.bl
	Busy("CheckBlock "+bl.Hash.String())
	e, dos, maybelater := BlockChain.CheckBlock(bl)
	if e != nil {
		if maybelater {
			addBlockToCache(bl, newbl.conn)
		} else {
			println(dos, e.Error())
			if dos {
				newbl.conn.DoS()
			}
		}
	} else {
		Busy("LocalAcceptBlock "+bl.Hash.String())
		e = LocalAcceptBlock(bl, newbl.conn)
		if e == nil {
			retryCachedBlocks = retry_cached_blocks()
		} else {
			println("AcceptBlock:", e.Error())
			newbl.conn.DoS()
		}
	}
}


// Called from network threads
func blockWanted(h []byte) (yes bool) {
	mutex.Lock()
	if _, ok := receivedBlocks[btc.NewUint256(h).BIdx()]; !ok {
		yes = true
	} else {
		CountSafe("BlockUnwanted")
	}
	mutex.Unlock()
	return
}


// This function is called from the main thread (or from an UI)
// It is only used to ask for tx that we need for an input,
// but it does not seem to help, so it will likely be removed.
func AskPeersForData(typ uint32, h *btc.Uint256) {
	CountSafe(fmt.Sprint("AskPeersForData", typ))

	// Prepare the inv
	pl := make([]byte, 37)
	pl[0] = 1 // only one hash
	binary.LittleEndian.PutUint32(pl[1:5], typ)
	copy(pl[4:36], h.Bytes())

	// Append it to PendingInvs in each open connection
	mutex.Lock()
	for _, v := range openCons {
		v.SendRawMsg("getdata", pl)
	}
	mutex.Unlock()
}
