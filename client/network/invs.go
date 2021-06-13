package network

import (
	"fmt"
	//"time"
	"bytes"
	"encoding/binary"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

const (
	MSG_WITNESS_FLAG = 0x40000000

	MSG_TX            = 1
	MSG_BLOCK         = 2
	MSG_CMPCT_BLOCK   = 4
	MSG_WITNESS_TX    = MSG_TX | MSG_WITNESS_FLAG
	MSG_WITNESS_BLOCK = MSG_BLOCK | MSG_WITNESS_FLAG

	MAX_INVS_AT_ONCE = 50000
)

func blockReceived(bh *btc.Uint256) (ok bool) {
	MutexRcv.Lock()
	_, ok = ReceivedBlocks[bh.BIdx()]
	MutexRcv.Unlock()
	return
}

func hash2invid(hash []byte) uint64 {
	return binary.LittleEndian.Uint64(hash[4:12])
}

// Make sure c.Mutex is locked when calling it
func (c *OneConnection) InvStore(typ uint32, hash []byte) {
	inv_id := hash2invid(hash)
	if len(c.InvDone.History) < MAX_INV_HISTORY {
		c.InvDone.History = append(c.InvDone.History, inv_id)
		c.InvDone.Map[inv_id] = typ
		c.InvDone.Idx++
		return
	}
	if c.InvDone.Idx == MAX_INV_HISTORY {
		c.InvDone.Idx = 0
	}
	delete(c.InvDone.Map, c.InvDone.History[c.InvDone.Idx])
	c.InvDone.History[c.InvDone.Idx] = inv_id
	c.InvDone.Map[inv_id] = typ
	c.InvDone.Idx++
}

func (c *OneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		//println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		c.DoS("InvEmpty")
		return
	}
	c.Mutex.Lock()
	c.X.InvsRecieved++
	c.Mutex.Unlock()

	cnt, of := btc.VLen(pl)
	if of == 0 || len(pl) != of+36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
		c.DoS("InvErr")
		return
	}

	for i := 0; i < cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of : of+4])
		c.Mutex.Lock()
		c.InvStore(typ, pl[of+4:of+36])
		ahr := c.X.AllHeadersReceived
		c.Mutex.Unlock()
		common.CountSafe(fmt.Sprint("InvGot-", typ))
		if typ == MSG_BLOCK {
			bhash := btc.NewUint256(pl[of+4 : of+36])
			if !ahr {
				common.CountSafe("InvBlockIgnored")
			} else {
				if !blockReceived(bhash) {
					MutexRcv.Lock()
					if b2g, ok := BlocksToGet[bhash.BIdx()]; ok {
						if c.Node.Height < b2g.Block.Height {
							c.Node.Height = b2g.Block.Height
						}
						common.CountSafe("InvBlockFresh")
						//println(c.PeerAddr.Ip(), c.Node.Version, "also knows the block", b2g.Block.Height, bhash.String())
						c.MutexSetBool(&c.X.GetBlocksDataNow, true)
					} else {
						common.CountSafe("InvBlockNew")
						c.ReceiveHeadersNow()
						//println(c.PeerAddr.Ip(), c.Node.Version, "possibly new block", bhash.String())
					}
					MutexRcv.Unlock()
				} else {
					common.CountSafe("InvBlockOld")
				}
			}
		} else if typ == MSG_TX {
			if common.AcceptTx() {
				c.TxInvNotify(pl[of+4 : of+36])
			} else {
				common.CountSafe("InvTxIgnored")
			}
		}
		of += 36
	}

	return
}

func NetRouteInv(typ uint32, h *btc.Uint256, fromConn *OneConnection) uint32 {
	var fee_spkb uint64
	if typ == MSG_TX {
		TxMutex.Lock()
		if tx, ok := TransactionsToSend[h.BIdx()]; ok {
			fee_spkb = (1000 * tx.Fee) / uint64(tx.VSize())
		} else {
			println("NetRouteInv: txid", h.String(), "not in mempool")
		}
		TxMutex.Unlock()
	}
	return NetRouteInvExt(typ, h, fromConn, fee_spkb)
}

// NetRouteInvExt is called from the main thread (or from a UI).
func NetRouteInvExt(typ uint32, h *btc.Uint256, fromConn *OneConnection, fee_spkb uint64) (cnt uint32) {
	common.CountSafe(fmt.Sprint("NetRouteInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

	// Append it to PendingInvs in each open connection
	Mutex_net.Lock()
	for _, v := range OpenCons {
		if v != fromConn { // except the one that this inv came from
			send_inv := true
			v.Mutex.Lock()
			if typ == MSG_TX {
				if v.Node.DoNotRelayTxs {
					send_inv = false
					common.CountSafe("SendInvNoTxNode")
				} else if v.X.MinFeeSPKB > 0 && uint64(v.X.MinFeeSPKB) > fee_spkb {
					send_inv = false
					common.CountSafe("SendInvFeeTooLow")
				}

				/* This is to prevent sending own txs to "spying" peers:
				else if fromConn==nil && v.X.InvsRecieved==0 {
					send_inv = false
					common.CountSafe("SendInvOwnBlocked")
				}
				*/
			}
			if send_inv {
				if len(v.PendingInvs) < 500 {
					if typ, ok := v.InvDone.Map[hash2invid(inv[4:36])]; ok {
						common.CountSafe(fmt.Sprint("SendInvSame-", typ))
					} else {
						v.PendingInvs = append(v.PendingInvs, inv)
						cnt++
						if typ == MSG_BLOCK {
							v.sendInvsNow.Set() // for faster block propagation
						}
					}
				} else {
					common.CountSafe("SendInvFull")
				}
			}
			v.Mutex.Unlock()
		}
	}
	Mutex_net.Unlock()
	return
}

// Call this function only when BlockIndexAccess is locked
func addInvBlockBranch(inv map[[32]byte]bool, bl *chain.BlockTreeNode, stop *btc.Uint256) {
	if len(inv) >= 500 || bl.BlockHash.Equal(stop) {
		return
	}
	inv[bl.BlockHash.Hash] = true
	for i := range bl.Childs {
		if len(inv) >= 500 {
			return
		}
		addInvBlockBranch(inv, bl.Childs[i], stop)
	}
}

func (c *OneConnection) GetBlocks(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)

	if e != nil || len(h2get) < 1 || hashstop == nil {
		println("GetBlocks: error parsing payload from", c.PeerAddr.Ip())
		c.DoS("BadGetBlks")
		return
	}

	invs := make(map[[32]byte]bool, 500)
	for i := range h2get {
		common.BlockChain.BlockIndexAccess.Lock()
		if bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
			// make sure that this block is in our main chain
			common.Last.Mutex.Lock()
			end := common.Last.Block
			common.Last.Mutex.Unlock()
			for ; end != nil && end.Height >= bl.Height; end = end.Parent {
				if end == bl {
					addInvBlockBranch(invs, bl, hashstop) // Yes - this is the main chain
					if len(invs) > 0 {
						common.BlockChain.BlockIndexAccess.Unlock()

						inv := new(bytes.Buffer)
						btc.WriteVlen(inv, uint64(len(invs)))
						for k := range invs {
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
	b_txs := new(bytes.Buffer)
	b_blk := new(bytes.Buffer)
	var c_blk []*btc.Uint256

	c.Mutex.Lock()
	if len(c.PendingInvs) > 0 {
		var invs_count int
		for i := range c.PendingInvs {
			var inv_sent_otherwise bool
			typ := binary.LittleEndian.Uint32((*c.PendingInvs[i])[:4])
			c.InvStore(typ, (*c.PendingInvs[i])[4:36])
			if typ == MSG_BLOCK {
				if c.Node.SendCmpctVer >= 1 && c.Node.HighBandwidth {
					c_blk = append(c_blk, btc.NewUint256((*c.PendingInvs[i])[4:]))
					inv_sent_otherwise = true
				} else if c.Node.SendHeaders {
					// convert block inv to block header
					common.BlockChain.BlockIndexAccess.Lock()
					bl := common.BlockChain.BlockIndex[btc.NewUint256((*c.PendingInvs[i])[4:]).BIdx()]
					if bl != nil {
						b_blk.Write(bl.BlockHeader[:])
						b_blk.Write([]byte{0}) // 0 txs
					}
					common.BlockChain.BlockIndexAccess.Unlock()
					inv_sent_otherwise = true
				}
			}

			if !inv_sent_otherwise {
				if invs_count == MAX_INVS_AT_ONCE {
					common.CountSafe("InvTooManyAtOnce")
					println("SendInvs:", MAX_INVS_AT_ONCE, "/", len(c.PendingInvs), "invs to send to peer", c.PeerAddr.Ip(), c.Node.Agent)
					c.PendingInvs = c.PendingInvs[i:]
					goto not_all_sent
				}
				invs_count++
				b_txs.Write((*c.PendingInvs[i])[:])
			}
		}
		res = true
	}
	c.PendingInvs = nil
not_all_sent:
	c.Mutex.Unlock()

	if len(c_blk) > 0 {
		for _, h := range c_blk {
			c.SendCmpctBlk(h)
		}
	}

	if b_blk.Len() > 0 {
		common.CountSafe("InvSentAsHeader")
		b := new(bytes.Buffer)
		btc.WriteVlen(b, uint64(b_blk.Len()/81))
		c.SendRawMsg("headers", append(b.Bytes(), b_blk.Bytes()...))
		//println("sent block's header(s)", b_blk.Len(), uint64(b_blk.Len()/81))
	}

	if b_txs.Len() > 0 {
		b := new(bytes.Buffer)
		btc.WriteVlen(b, uint64(b_txs.Len()/36))
		c.SendRawMsg("inv", append(b.Bytes(), b_txs.Bytes()...))
	}

	return
}
