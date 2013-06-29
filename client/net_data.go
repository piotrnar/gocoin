package main

import (
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
			if tx, ok := TransactionsToSend[uh.Hash]; ok {
				tx_mutex.Unlock()
				c.SendRawMsg("tx", tx.data)
				if dbg > 0 {
					println("sent tx to", c.PeerAddr.Ip())
				}
			} else {
				tx_mutex.Unlock()
			}
		} else {
			println("getdata for type", typ, "not supported yet")
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
	c.GetBlockInProgress = btc.NewUint256(h)
	c.GetBlockInProgressAt = time.Now()
	c.GetBlockHeaderGot = false
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

	if conn.GetBlockInProgress!=nil && conn.GetBlockInProgress.Equal(bl.Hash) {
		conn.GetBlockInProgress = nil
	} else {
		CountSafe("EnxpectedBlockRcvd")
	}

	idx := bl.Hash.BIdx()
	mutex.Lock()
	if _, got := receivedBlocks[idx]; got {
		CountSafe("SameBlockReceived")
		mutex.Unlock()
		return
	}
	pbl := pendingBlocks[idx]
	if pbl==nil {
		println("WTF? Received block that isn't pending", bl.Hash.String())
		ui_show_prompt()
	} else {
		if CFG.MeasureBlockTiming {
			println("New Block", bl.Hash.String(), "received after",
				time.Now().Sub(pbl.noticed).String())
			ui_show_prompt()
		}
		delete(pendingBlocks, idx)
	}
	receivedBlocks[idx] = pbl
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


// Called from network threads (quite often)
func blockDataNeeded() ([]byte) {
	if len(pendingFifo)>0 && len(netBlocks)<200 {
		idx := <- pendingFifo
		mutex.Lock()

		if pbl, ok := pendingBlocks[idx]; ok {
			if pbl.single && time.Now().After(pbl.noticed.Add(GetBlockSwitchOffSingle)) {
				CountSafe("FromFifoUnsingle")
				pbl.single = false
			}
			mutex.Unlock()
			pendingFifo <- idx // put it back to the channel
			if !pbl.single {
				CountSafe("FromFifoPending")
				return pbl.hash.Hash[:]
			}
			CountSafe("FromFifoSingle")
			return nil
		}

		if _, ok := receivedBlocks[idx]; ok {
			mutex.Unlock()
			CountSafe("FromFifoReceived")
			return nil
		}

		mutex.Unlock()
		println("blockDataNeeded: It should not end up here") // TODO: remove this
		CountSafe("FromFifoObsolete")
	}
	return nil
}


// Called from network threads
func blockWanted(h []byte) (yes bool) {
	ha := btc.NewUint256(h)
	idx := ha.BIdx()
	mutex.Lock()
	if _, ok := receivedBlocks[idx]; !ok {
		yes = true
	} else {
		CountSafe("Block not wanted")
	}
	mutex.Unlock()
	return
}
