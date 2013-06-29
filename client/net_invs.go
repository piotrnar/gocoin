package main

import (
	"fmt"
	"time"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


func (c *oneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		return
	}

	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	var blinv2ask []byte
	for i:=0; i<cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		CountSafe(fmt.Sprint("InvGot",typ))
		if typ==2 {
			if blockWanted(pl[of+4:of+36]) {
				blinv2ask = append(blinv2ask, pl[of+4:of+36]...)
			}
		} else if typ==1 {
			if CFG.TXRouting.Enabled {
				c.TxInvNotify(pl[of+4:of+36])
			}
		}
		of+= 36
	}

	if len(blinv2ask)>0 {
		bu := new(bytes.Buffer)
		btc.WriteVlen(bu, uint32(len(blinv2ask)/32))
		for i:=0; i<len(blinv2ask); i+=32 {
			bh := btc.NewUint256(blinv2ask[i:i+32])
			c.GetBlockInProgress[bh.BIdx()] = &oneBlockDl{hash:bh, start:time.Now()}
			binary.Write(bu, binary.LittleEndian, uint32(2))
			bu.Write(bh.Hash[:])
		}
		c.SendRawMsg("getdata", bu.Bytes())
	}

	return
}


// This function is called from the main thread (or from an UI)
func NetRouteInv(typ uint32, h *btc.Uint256, fromConn *oneConnection) (cnt uint) {
	CountSafe(fmt.Sprint("NetRouteInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

	// Append it to PendingInvs in each open connection
	mutex.Lock()
	for _, v := range openCons {
		if v != fromConn { // except for the one that this inv came from
			if len(v.PendingInvs)<500 {
				v.PendingInvs = append(v.PendingInvs, inv)
				cnt++
			} else {
				CountSafe("SendInvIgnored")
			}
		}
	}
	mutex.Unlock()
	return
}


// Call this function only when BlockIndexAccess is locked
func addInvBlockBranch(inv map[[32]byte] bool, bl *btc.BlockTreeNode, stop *btc.Uint256) {
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


func (c *oneConnection) ProcessGetBlocks(pl []byte) {
	b := bytes.NewReader(pl)
	var ver uint32
	e := binary.Read(b, binary.LittleEndian, &ver)
	if e != nil {
		println("ProcessGetBlocks:", e.Error(), c.PeerAddr.Ip())
		CountSafe("GetblksNoVer")
		c.DoS()
		return
	}
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetBlocks:", e.Error(), c.PeerAddr.Ip())
		CountSafe("GetblksNoVlen")
		c.DoS()
		return
	}

	if cnt<1 {
		println("ProcessGetBlocks: empty inv list", c.PeerAddr.Ip())
		CountSafe("GetblksNoInvs")
		c.DoS()
		return
	}

	h2get := make([]*btc.Uint256, cnt)
	var h [32]byte
	for i:=0; i<int(cnt); i++ {
		n, _ := b.Read(h[:])
		if n != 32 {
			if dbg>0 {
				println("getblocks too short", c.PeerAddr.Ip())
			}
			CountSafe("GetblksTooShort")
			c.DoS()
			return
		}
		h2get[i] = btc.NewUint256(h[:])
		if dbg>2 {
			println(c.PeerAddr.Ip(), "getbl", h2get[i].String())
		}
	}
	n, _ := b.Read(h[:])
	if n != 32 {
		if dbg>0 {
			println("getblocks does not have hash_stop", c.PeerAddr.Ip())
		}
		CountSafe("GetblksNoStop")
		c.DoS()
		return
	}
	hashstop := btc.NewUint256(h[:])

	invs := make(map[[32]byte] bool, 500)
	for i := range h2get {
		BlockChain.BlockIndexAccess.Lock()
		if bl, ok := BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
			// make sure that this block is in our main chain
			for end := LastBlock; end!=nil && end.Height>=bl.Height; end = end.Parent {
				if end==bl {
					addInvBlockBranch(invs, bl, hashstop)  // Yes - this is the main chain
					if dbg>0 {
						fmt.Println(c.PeerAddr.Ip(), "getblocks from", bl.Height,
							"stop at",  hashstop.String(), "->", len(invs), "invs")
					}

					if len(invs)>0 {
						BlockChain.BlockIndexAccess.Unlock()

						inv := new(bytes.Buffer)
						btc.WriteVlen(inv, uint32(len(invs)))
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
		BlockChain.BlockIndexAccess.Unlock()
	}

	CountSafe("GetblksMissed")
	return
}


func (c *oneConnection) SendInvs() (res bool) {
	b := new(bytes.Buffer)
	mutex.Lock()
	if len(c.PendingInvs)>0 {
		btc.WriteVlen(b, uint32(len(c.PendingInvs)))
		for i := range c.PendingInvs {
			b.Write((*c.PendingInvs[i])[:])
		}
		res = true
	}
	c.PendingInvs = nil
	mutex.Unlock()
	if res {
		c.SendRawMsg("inv", b.Bytes())
	}
	return
}


func (c *oneConnection) getblocksNeeded() bool {
	mutex.Lock()
	lb := LastBlock
	if lb!=c.LastBlocksFrom && len(c.GetBlockInProgress)>0 {
		// We have more than 200 pending blocks, so hold on for now...
		mutex.Unlock()
		return false
	}
	mutex.Unlock()
	if lb != c.LastBlocksFrom || time.Now().After(c.NextBlocksAsk) {
		c.LastBlocksFrom = LastBlock

		GetBlocksAskBack := int(time.Now().Sub(LastBlockReceived) / time.Minute)
		if GetBlocksAskBack >= btc.MovingCheckopintDepth {
			GetBlocksAskBack = btc.MovingCheckopintDepth
		}

		b := make([]byte, 37)
		binary.LittleEndian.PutUint32(b[0:4], Version)
		b[4] = 1 // one locator so far...
		copy(b[5:37], LastBlock.BlockHash.Hash[:])

		if GetBlocksAskBack > 0 {
			BlockChain.BlockIndexAccess.Lock()
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
			BlockChain.BlockIndexAccess.Unlock()
		}
		var null_stop [32]byte
		b = append(b, null_stop[:]...)
		c.SendRawMsg("getblocks", b)
		c.NextBlocksAsk = time.Now().Add(NewBlocksAskDuration)
		return true
	}
	return false
}
