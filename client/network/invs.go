package network

import (
	"fmt"
	"time"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)



func (c *OneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		return
	}
	c.InvsRecieved++

	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	var blinv2ask []byte

	for i:=0; i<cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		common.CountSafe(fmt.Sprint("InvGot",typ))
		if typ==2 {
			if blockWanted(pl[of+4:of+36]) {
				blinv2ask = append(blinv2ask, pl[of+4:of+36]...)
			}
		} else if typ==1 {
			if common.CFG.TXPool.Enabled {
				c.TxInvNotify(pl[of+4:of+36])
			}
		}
		of+= 36
	}

	if len(blinv2ask)>0 {
		bu := new(bytes.Buffer)
		btc.WriteVlen(bu, uint64(len(blinv2ask)/32))
		for i:=0; i<len(blinv2ask); i+=32 {
			bh := btc.NewUint256(blinv2ask[i:i+32])
			c.Mutex.Lock()
			c.GetBlockInProgress[bh.BIdx()] = &oneBlockDl{hash:bh, start:time.Now()}
			c.Mutex.Unlock()
			binary.Write(bu, binary.LittleEndian, uint32(2))
			bu.Write(bh.Hash[:])
		}
		c.SendRawMsg("getdata", bu.Bytes())
	}

	return
}


// This function is called from the main thread (or from an UI)
func NetRouteInv(typ uint32, h *btc.Uint256, fromConn *OneConnection) (cnt uint) {
	common.CountSafe(fmt.Sprint("NetRouteInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

	// Append it to PendingInvs in each open connection
	Mutex_net.Lock()
	for _, v := range OpenCons {
		if v != fromConn { // except the one that this inv came from
			v.Mutex.Lock()
			if v.Node.DoNotRelayTxs && typ==1 {
				// This node does not want tx inv (it came with its version message)
				common.CountSafe("SendInvNoTxNode")
			} else {
				if fromConn==nil && v.InvsRecieved==0 {
					// Do not broadcast own txs to nodes that never sent any invs to us
					common.CountSafe("SendInvOwnBlocked")
				} else if len(v.PendingInvs)<500 {
					v.PendingInvs = append(v.PendingInvs, inv)
					cnt++
				} else {
					common.CountSafe("SendInvIgnored")
				}
			}
			v.Mutex.Unlock()
		}
	}
	Mutex_net.Unlock()
	if cnt == 0 {
		NetAlerts <- "WARNING: your tx has not been broadcasted to any peer"
	}
	return
}


// Call this function only when BlockIndexAccess is locked
func addInvBlockBranch(inv map[[32]byte] bool, bl *chain.BlockTreeNode, stop *btc.Uint256) {
	if len(inv)>=500 || bl.BlockHash.Equal(stop) {
		return
	}
	inv[bl.BlockHash.Hash] = true
	for i := range bl.Childs {
		if len(inv)>=500 {
			return
		}
		addInvBlockBranch(inv, bl.Childs[i], stop)
	}
}


func (c *OneConnection) GetBlocks(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)

	if e!=nil || len(h2get)<1 || hashstop==nil {
		println("GetBlocks: error parsing payload from", c.PeerAddr.Ip())
		c.DoS("BadGetBlks")
		return
	}

	invs := make(map[[32]byte] bool, 500)
	for i := range h2get {
		common.BlockChain.BlockIndexAccess.Lock()
		if bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
			// make sure that this block is in our main chain
			common.Last.Mutex.Lock()
			end := common.Last.Block
			common.Last.Mutex.Unlock()
			for ; end!=nil && end.Height>=bl.Height; end = end.Parent {
				if end==bl {
					addInvBlockBranch(invs, bl, hashstop)  // Yes - this is the main chain
					if common.DebugLevel>0 {
						fmt.Println(c.PeerAddr.Ip(), "getblocks from", bl.Height,
							"stop at",  hashstop.String(), "->", len(invs), "invs")
					}

					if len(invs)>0 {
						common.BlockChain.BlockIndexAccess.Unlock()

						inv := new(bytes.Buffer)
						btc.WriteVlen(inv, uint64(len(invs)))
						for k, _ := range invs {
							binary.Write(inv, binary.LittleEndian, uint32(2))
							inv.Write(k[:])
						}
						c.SendRawMsg("inv", inv.Bytes())
						return
					}
				}
			}
		}
		common.BlockChain.BlockIndexAccess.Unlock()
	}

	common.CountSafe("GetblksMissed")
	return
}


func (c *OneConnection) SendInvs() (res bool) {
	b := new(bytes.Buffer)
	c.Mutex.Lock()
	if len(c.PendingInvs)>0 {
		btc.WriteVlen(b, uint64(len(c.PendingInvs)))
		for i := range c.PendingInvs {
			b.Write((*c.PendingInvs[i])[:])
		}
		res = true
	}
	c.PendingInvs = nil
	c.Mutex.Unlock()
	if res {
		c.SendRawMsg("inv", b.Bytes())
	}
	return
}


func (c *OneConnection) getblocksNeeded() bool {
	common.Last.Mutex.Lock()
	lb := common.Last.Block
	common.Last.Mutex.Unlock()
	if lb != c.LastBlocksFrom || time.Now().After(c.NextBlocksAsk) {
		c.Mutex.Lock()
		c.LastBlocksFrom = lb
		c.Mutex.Unlock()

		common.Last.Mutex.Lock()
		GetBlocksAskBack := int(time.Now().Sub(common.Last.Time) / time.Minute)
		common.Last.Mutex.Unlock()
		if GetBlocksAskBack >= chain.MovingCheckopintDepth {
			GetBlocksAskBack = chain.MovingCheckopintDepth
		}

		b := make([]byte, 37)
		binary.LittleEndian.PutUint32(b[0:4], common.Version)
		b[4] = 1 // one locator so far...
		copy(b[5:37], lb.BlockHash.Hash[:])

		if GetBlocksAskBack > 0 {
			common.BlockChain.BlockIndexAccess.Lock()
			cnt_each := 0
			for i:=0; i < GetBlocksAskBack && lb.Parent != nil; i++ {
				lb = lb.Parent
				cnt_each++
				if cnt_each==200 {
					b[4]++
					b = append(b, lb.BlockHash.Hash[:]...)
					cnt_each = 0
				}
			}
			if cnt_each!=0 {
				b[4]++
				b = append(b, lb.BlockHash.Hash[:]...)
			}
			common.BlockChain.BlockIndexAccess.Unlock()
		}
		var null_stop [32]byte
		b = append(b, null_stop[:]...)
		c.SendRawMsg("getblocks", b)
		c.Mutex.Lock()
		c.NextBlocksAsk = time.Now().Add(NewBlocksAskDuration)
		c.Mutex.Unlock()
		return true
	}
	return false
}
