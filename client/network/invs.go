package network

import (

	//"time"
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	MSG_WITNESS_FLAG = 0x40000000

	MSG_TX            = uint32(1)
	MSG_BLOCK         = uint32(2)
	MSG_CMPCT_BLOCK   = uint32(4)
	MSG_WITNESS_TX    = uint32(MSG_TX | MSG_WITNESS_FLAG)
	MSG_WITNESS_BLOCK = uint32(MSG_BLOCK | MSG_WITNESS_FLAG)
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
		//fmt.Println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		c.DoS("InvEmpty")
		return
	}
	c.Mutex.Lock()
	c.X.InvsReceived++
	c.Mutex.Unlock()

	cnt, of := btc.VLen(pl)
	// Limit cnt first, as a huge value would overflow the multiplication below
	if of == 0 || cnt > 50000 || len(pl) != of+36*cnt {
		fmt.Println("inv payload length mismatch", len(pl), of, cnt)
		c.DoS("InvErr")
		return
	}

	var txs2get []byte // txids
	for i := 0; i < cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of : of+4])
		c.Mutex.Lock()
		c.InvStore(typ, pl[of+4:of+36])
		ahr := c.X.AllHeadersReceived
		c.Mutex.Unlock()
		switch typ {
		case MSG_BLOCK:
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
						//fmt.Println(c.PeerAddr.Ip(), c.Node.Version, "also knows the block", b2g.Block.Height, bhash.String())
						b2g.addFetchSource(c) // he has it, so we are allowed to fetch it from him
						c.MutexSetBool(&c.X.GetBlocksDataNow, true)
					} else {
						common.CountSafe("InvBlockNew")
						c.ReceiveHeadersNow()
						//fmt.Println(c.PeerAddr.Ip(), c.Node.Version, "possibly new block", bhash.String())
					}
					MutexRcv.Unlock()
				} else {
					common.CountSafe("InvBlockOld")
				}
			}
		case MSG_TX:
			if common.AcceptTx() {
				if c.TxInvNotify(pl[of+4 : of+36]) {
					txs2get = append(txs2get, pl[of+4:of+36]...)
				}
			} else {
				common.CountSafe("InvTxIgnored")
			}
		default:
			common.CountSafePar("InvUnknTyp-", typ)
		}
		of += 36
	}

	// ask for all the txs that we want from this inv, in a single message
	c.sendGetTxs(txs2get)
}

func NetRouteInv(typ uint32, h *btc.Uint256, fromConn *OneConnection) uint32 {
	var fee_spkb uint64
	if typ == MSG_TX {
		txpool.TxMutex.Lock()
		if tx, ok := txpool.TransactionsToSend[h.BIdx()]; ok {
			fee_spkb = (4000 * tx.Fee) / uint64(tx.Weight())
		} else {
			fmt.Println("NetRouteInv: txid", h.String(), "not in mempool")
		}
		txpool.TxMutex.Unlock()
	}
	return NetRouteInvExt(typ, h, fromConn, fee_spkb)
}

// NetRouteInvExt is called from the main thread (or from a UI).
func NetRouteInvExt(typ uint32, h *btc.Uint256, fromConn *OneConnection, fee_spkb uint64) (cnt uint32) {
	common.CountSafePar("NetRouteInv-", typ)

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
			}
			if send_inv {
				if len(v.PendingInvs) < 500 {
					if typ, ok := v.InvDone.Map[hash2invid(inv[4:36])]; ok {
						common.CountSafePar("SendInvSame-", typ)
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

// GetBlocks handles "getblocks": it sends "inv" with (up to 500) blocks from our main
// chain that follow the first block from the locator that is in our main chain.
func (c *OneConnection) GetBlocks(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)

	if e != nil || len(h2get) < 1 || hashstop == nil {
		fmt.Println("GetBlocks: error parsing payload from", c.PeerAddr.Ip())
		c.DoS("BadGetBlks")
		return
	}

	// Never lock Last.Mutex while holding BlockIndexAccess (main thread locks them in the opposite order)
	common.Last.Mutex.Lock()
	last := common.Last.Block
	common.Last.Mutex.Unlock()

	var invs []*btc.Uint256 // in the order of the chain
	common.BlockChain.BlockIndexAccess.Lock()
	for i := range h2get {
		bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]
		if !ok || bl.Height >= last.Height {
			continue
		}
		cnt := last.Height - bl.Height
		if cnt > 500 {
			cnt = 500
		}
		// go from the top down to the block from the locator, to see if it is in our main chain
		hashes := make([]*btc.Uint256, cnt)
		end := last
		for ; end != nil && end.Height > bl.Height; end = end.Parent {
			if idx := end.Height - bl.Height - 1; idx < cnt {
				hashes[idx] = end.BlockHash
			}
		}
		if end == bl { // Yes - this is the main chain
			invs = hashes
			break
		}
	}
	common.BlockChain.BlockIndexAccess.Unlock()

	// do not go past hashstop
	for i, h := range invs {
		if h.Equal(hashstop) {
			invs = invs[:i+1]
			break
		}
	}

	if len(invs) == 0 {
		common.CountSafe("GetblksMissed")
		return
	}

	inv := new(bytes.Buffer)
	btc.WriteVlen(inv, uint64(len(invs)))
	for _, h := range invs {
		binary.Write(inv, binary.LittleEndian, MSG_BLOCK)
		inv.Write(h.Hash[:])
	}
	c.SendRawMsg("inv", inv.Bytes(), false)
}

func (c *OneConnection) SendInvs() (res bool) {
	b_txs := new(bytes.Buffer)
	b_blk := new(bytes.Buffer)
	var c_blk []*btc.Uint256

	c.Mutex.Lock()
	if len(c.PendingInvs) > 0 {
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
				b_txs.Write((*c.PendingInvs[i])[:])
			}
		}
		res = true
	}
	c.PendingInvs = nil
	c.sendInvsNow.Clr()
	c.Mutex.Unlock()

	if len(c_blk) > 0 {
		for _, h := range c_blk {
			c.SendCmpctBlk(h)
		}
	}

	if b_blk.Len() > 0 {
		common.CountSafe("InvSentAsHeader")
		var b [5]byte
		ll := btc.PutVlen(b[:], b_blk.Len()/81)
		c.SendRawMsg("headers", append(b[:ll], b_blk.Bytes()...), false)
	}

	if b_txs.Len() > 0 {
		var b [5]byte
		ll := btc.PutVlen(b[:], b_txs.Len()/36)
		c.SendRawMsg("inv", append(b[:ll], b_txs.Bytes()...), false)
	}

	return
}
