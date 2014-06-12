package network

import (
	"fmt"
	"time"
	"bytes"
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)


func (c *OneConnection) ProcessGetData(pl []byte) {
	var notfound []byte

	//println(c.PeerAddr.Ip(), "getdata")
	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
		return
	}
	for i:=0; i<int(cnt); i++ {
		var typ uint32
		var h [36]byte

		n, _ := b.Read(h[:])
		if n!=36 {
			println("ProcessGetData: pl too short", c.PeerAddr.Ip())
			return
		}

		typ = binary.LittleEndian.Uint32(h[:4])

		common.CountSafe(fmt.Sprint("GetdataType",typ))
		if typ == 2 {
			uh := btc.NewUint256(h[4:])
			bl, _, er := common.BlockChain.Blocks.BlockGet(uh)
			if er == nil {
				c.SendRawMsg("block", bl)
			} else {
				notfound = append(notfound, h[:]...)
			}
		} else if typ == 1 {
			// transaction
			uh := btc.NewUint256(h[4:])
			TxMutex.Lock()
			if tx, ok := TransactionsToSend[uh.BIdx()]; ok && tx.Blocked==0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				TxMutex.Unlock()
				c.SendRawMsg("tx", tx.Data)
			} else {
				TxMutex.Unlock()
				notfound = append(notfound, h[:]...)
			}
		} else {
			if common.DebugLevel>0 {
				println("getdata for type", typ, "not supported yet")
			}
			if typ>0 && typ<=3 /*3 is a filtered block(we dont support it)*/ {
				notfound = append(notfound, h[:]...)
			}
		}
	}

	if len(notfound)>0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint64(len(notfound)/36))
		buf.Write(notfound)
		c.SendRawMsg("notfound", buf.Bytes())
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
		conn.DoS("BrokenBlock")
		println("NewBlock:", e.Error())
		return
	}

	idx := bl.Hash.BIdx()
	MutexRcv.Lock()
	if rb, got := ReceivedBlocks[idx]; got {
		rb.Cnt++
		MutexRcv.Unlock()
		common.CountSafe("BlockSameRcvd")
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

// Read VLen followed by the number of locators
// parse the payload of getblocks and getheaders messages
func parseLocatorsPayload(pl []byte) (h2get []*btc.Uint256, hashstop *btc.Uint256, er error) {
	var cnt uint64
	var h [32]byte
	var ver uint32

	b := bytes.NewReader(pl)

	// version
	if er = binary.Read(b, binary.LittleEndian, &ver); er != nil {
		return
	}

	// hash count
	cnt, er = btc.ReadVLen(b)
	if er != nil {
		return
	}

	// block locator hashes
	if cnt>0 {
		h2get = make([]*btc.Uint256, cnt)
		for i:=0; i<int(cnt); i++ {
			if _, er = b.Read(h[:]); er!=nil {
				return
			}
			h2get[i] = btc.NewUint256(h[:])
		}
	}

	// hash_stop
	if _, er = b.Read(h[:]); er!=nil {
		return
	}
	hashstop = btc.NewUint256(h[:])

	return
}


// Handle getheaders protocol command
// https://en.bitcoin.it/wiki/Protocol_specification#getheaders
func (c *OneConnection) GetHeaders(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)
	if e != nil || hashstop==nil {
		println("GetHeaders: error parsing payload from", c.PeerAddr.Ip())
		c.DoS("BadGetHdrs")
		return
	}

	if common.DebugLevel > 1 {
		println("GetHeaders", len(h2get), hashstop.String())
	}

	var best_block, last_block *chain.BlockTreeNode

	common.BlockChain.BlockIndexAccess.Lock()
	if len(h2get) > 0 {
		for i := range h2get {
			if bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
				if best_block==nil || bl.Height > best_block.Height {
					best_block = bl
				}
			}
		}
	} else {
		best_block = common.BlockChain.BlockIndex[hashstop.BIdx()]
	}
	last_block = common.BlockChain.BlockTreeEnd
	common.BlockChain.BlockIndexAccess.Unlock()

	var resp []byte
	var cnt uint32
	for cnt<2000 {
		best_block = best_block.FindPathTo(last_block)
		if best_block==nil {
			break
		}
		resp = append(resp, append(best_block.BlockHeader[:], 0)...) // 81st byte is always zero
		cnt++
	}

	out := new(bytes.Buffer)
	btc.WriteVlen(out, uint64(cnt))
	out.Write(resp)
	c.SendRawMsg("headers", out.Bytes())
	return
}
