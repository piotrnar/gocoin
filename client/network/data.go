package network

import (
	"fmt"
	"time"
	"bytes"
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
)


func (c *OneConnection) ProcessGetData(pl []byte) {
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

		common.CountSafe(fmt.Sprint("GetdataType",typ))
		if typ == 2 {
			uh := btc.NewUint256(h[:])
			bl, _, er := common.BlockChain.Blocks.BlockGet(uh)
			if er == nil {
				c.SendRawMsg("block", bl)
			} else {
				//println("block", uh.String(), er.Error())
			}
		} else if typ == 1 {
			// transaction
			uh := btc.NewUint256(h[:])
			TxMutex.Lock()
			if tx, ok := TransactionsToSend[uh.Hash]; ok && tx.Blocked==0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				TxMutex.Unlock()
				c.SendRawMsg("tx", tx.Data)
				if common.DebugLevel > 0 {
					println("sent tx to", c.PeerAddr.Ip())
				}
			} else {
				TxMutex.Unlock()
			}
		} else {
			if common.DebugLevel>0 {
				println("getdata for type", typ, "not supported yet")
			}
		}
	}
}


func (c *OneConnection) GetBlockData(h []byte) {
	var b [1+4+32]byte
	b[0] = 1 // One inv
	b[1] = 2 // Block
	copy(b[5:37], h[:32])
	if common.DebugLevel > 1 {
		println("GetBlockData", btc.NewUint256(h).String())
	}
	bh := btc.NewUint256(h)
	c.Mutex.Lock()
	c.GetBlockInProgress[bh.BIdx()] = &oneBlockDl{hash:bh, start:time.Now()}
	c.Mutex.Unlock()
	c.SendRawMsg("getdata", b[:])
}


// This function is called from a net conn thread
func netBlockReceived(conn *OneConnection, b []byte) {
	bl, e := btc.NewBlock(b)
	if e != nil {
		conn.DoS()
		println("NewBlock:", e.Error())
		return
	}

	idx := bl.Hash.BIdx()
	MutexRcv.Lock()
	if rb, got := ReceivedBlocks[idx]; got {
		rb.Cnt++
		MutexRcv.Unlock()
		common.CountSafe("SameBlockReceived")
		return
	}
	orb := &OneReceivedBlock{Time:time.Now()}
	if bip, ok := conn.GetBlockInProgress[idx]; ok {
		orb.TmDownload = orb.Time.Sub(bip.start)
		conn.Mutex.Lock()
		delete(conn.GetBlockInProgress, idx)
		conn.Mutex.Unlock()
	} else {
		common.CountSafe("UnxpectedBlockRcvd")
	}
	ReceivedBlocks[idx] = orb
	MutexRcv.Unlock()

	NetBlocks <- &BlockRcvd{Conn:conn, Block:bl}
}


// It goes through all the netowrk connections and checks
// ... how many of them have a given block download in progress
// Returns true if it's at the max already - don't want another one.
func blocksLimitReached(idx [btc.Uint256IdxLen]byte) (res bool) {
	var cnt uint32
	Mutex_net.Lock()
	for _, v := range OpenCons {
		v.Mutex.Lock()
		_, ok := v.GetBlockInProgress[idx]
		v.Mutex.Unlock()
		if ok {
			if cnt+1 >= atomic.LoadUint32(&common.CFG.Net.MaxBlockAtOnce) {
				res = true
				break
			}
			cnt++
		}
	}
	Mutex_net.Unlock()
	return
}

// Called from network threads
func blockWanted(h []byte) (yes bool) {
	idx := btc.NewUint256(h).BIdx()
	MutexRcv.Lock()
	_, ok := ReceivedBlocks[idx]
	MutexRcv.Unlock()
	if !ok {
		if atomic.LoadUint32(&common.CFG.Net.MaxBlockAtOnce)==0 || !blocksLimitReached(idx) {
			yes = true
			common.CountSafe("BlockWanted")
		} else {
			common.CountSafe("BlockInProgress")
		}
	} else {
		common.CountSafe("BlockUnwanted")
	}
	return
}
