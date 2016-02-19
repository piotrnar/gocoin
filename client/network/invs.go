package network

import (
	"fmt"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)


func blockReceived(bh *btc.Uint256) (ok bool) {
	MutexRcv.Lock()
	_, ok = ReceivedBlocks[bh.BIdx()]
	MutexRcv.Unlock()
	return
}

func blockPending(bh *btc.Uint256) (ok bool) {
	MutexRcv.Lock()
	_, ok = BlocksToGet[bh.BIdx()]
	MutexRcv.Unlock()
	return
}


func (c *OneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		//println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		c.DoS("InvEmpty")
		return
	}
	c.InvsRecieved++

	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	for i:=0; i<cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		common.CountSafe(fmt.Sprint("InvGot",typ))
		if typ==2 {
			bhash := btc.NewUint256(pl[of+4:of+36])
			if c.AllHeadersReceived && !blockReceived(bhash) && !blockPending(bhash) {
				common.CountSafe("BlockInvTaken")
				c.AllHeadersReceived = false
				//println("possibly new block", bhash.String())
			} else {
				common.CountSafe("BlockInvIgnored")
			}
		} else if typ==1 {
			if common.CFG.TXPool.Enabled {
				MutexRcv.Lock()
				pending_blocks := len(BlocksToGet) + len(CachedBlocks) + len(NetBlocks)
				MutexRcv.Unlock()

				if pending_blocks > 10 {
					common.CountSafe("InvTxIgnored") // do not process TXs if the chain is not synchronized
				} else {
					c.TxInvNotify(pl[of+4:of+36])
				}
			}
		}
		of+= 36
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
	if typ==1 && cnt==0 {
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
