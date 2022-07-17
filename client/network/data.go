package network

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (c *OneConnection) ProcessGetData(pl []byte) {
	//var notfound []byte

	//println(c.PeerAddr.Ip(), "getdata")
	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
		return
	}
	for i := 0; i < int(cnt); i++ {
		var typ uint32
		var h [36]byte

		if c.SendingPaused() {
			out := new(bytes.Buffer)
			btc.WriteVlen(out, cnt-uint64(i))
			for ; i < int(cnt); i++ {
				if n, _ := b.Read(h[:]); n != 36 {
					//println("ProcessGetData - 2: pl too short", c.PeerAddr.Ip())
					c.DoS("GetDataA")
					return
				}
				out.Write(h[:])
			}
			c.unfinished_getdata = out.Bytes()
			common.CountSafe("GetDataPaused")
			break
		}

		if n, _ := b.Read(h[:]); n != 36 {
			//println("ProcessGetData: pl too short", c.PeerAddr.Ip())
			c.DoS("GetDataB")
			return
		}

		typ = binary.LittleEndian.Uint32(h[:4])
		c.Mutex.Lock()
		c.InvStore(typ, h[4:36])
		c.Mutex.Unlock()

		if typ == MSG_BLOCK || typ == MSG_WITNESS_BLOCK {
			if typ == MSG_BLOCK {
				common.CountSafe("GetdataBlock")
			} else {
				common.CountSafe("GetdataBlockSw")
			}
			hash := btc.NewUint256(h[4:])
			crec, _, er := common.BlockChain.Blocks.BlockGetExt(hash)

			if er == nil {
				bl := crec.Data
				if typ == MSG_BLOCK {
					// remove witness data from the block
					if crec.Block == nil {
						crec.Block, _ = btc.NewBlock(bl)
					}
					if crec.Block.NoWitnessData == nil {
						crec.Block.BuildNoWitnessData()
					}
					//println("block size", len(crec.Data), "->", len(bl))
					bl = crec.Block.NoWitnessData
				}
				c.SendRawMsg("block", bl)
			} else {
				//fmt.Println("BlockGetExt-2 failed for", hash.String(), er.Error())
				//notfound = append(notfound, h[:]...)
			}
		} else if typ == MSG_TX || typ == MSG_WITNESS_TX {
			if typ == MSG_TX {
				common.CountSafe("GetdataTx")
			} else {
				common.CountSafe("GetdataTxSw")
			}
			// ransaction
			TxMutex.Lock()
			if tx, ok := TransactionsToSend[btc.NewUint256(h[4:]).BIdx()]; ok && tx.Blocked == 0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				TxMutex.Unlock()
				if tx.SegWit == nil || typ == MSG_WITNESS_TX {
					c.SendRawMsg("tx", tx.Raw)
				} else {
					c.SendRawMsg("tx", tx.Serialize())
				}
			} else {
				TxMutex.Unlock()
				//notfound = append(notfound, h[:]...)
			}
		} else if typ == MSG_CMPCT_BLOCK {
			common.CountSafe("GetdataCmpctBlk")
			if !c.SendCmpctBlk(btc.NewUint256(h[4:])) {
				println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "asked for CmpctBlk we don't have", btc.NewUint256(h[4:]).String())
				if c.Misbehave("GetCmpctBlk", 100) {
					break
				}
			}
		} else {
			common.CountSafe("GetdataTypeInvalid")
			if typ > 0 && typ <= 3 /*3 is a filtered block(we dont support it)*/ {
				//notfound = append(notfound, h[:]...)
			}
		}
	}

	/*
		if len(notfound)>0 {
			buf := new(bytes.Buffer)
			btc.WriteVlen(buf, uint64(len(notfound)/36))
			buf.Write(notfound)
			c.SendRawMsg("notfound", buf.Bytes())
		}
	*/
}

// netBlockReceived is called from a net conn thread.
func netBlockReceived(conn *OneConnection, b []byte) {
	if len(b) < 100 {
		conn.DoS("ShortBlock")
		return
	}

	hash := btc.NewSha2Hash(b[:80])
	idx := hash.BIdx()
	//println("got block data", hash.String())

	MutexRcv.Lock()

	// the blocks seems to be fine
	if rb, got := ReceivedBlocks[idx]; got {
		rb.Cnt++
		common.CountSafeAdd("BlockBytesWasted", uint64(len(b)))
		common.CountSafe("BlockSameRcvd")
		conn.Mutex.Lock()
		delete(conn.GetBlockInProgress, idx)
		conn.Mutex.Unlock()
		MutexRcv.Unlock()
		return
	}

	// remove from BlocksToGet:
	b2g := BlocksToGet[idx]
	if b2g == nil {
		//println("Block", hash.String(), " from", conn.PeerAddr.Ip(), conn.Node.Agent, " was not expected")

		var sta int
		sta, b2g = conn.ProcessNewHeader(b[:80])
		if b2g == nil {
			if sta == PH_STATUS_FATAL {
				println("Unrequested Block: FAIL - Ban", conn.PeerAddr.Ip(), conn.Node.Agent)
				conn.DoS("BadUnreqBlock")
			} else {
				common.CountSafe("ErrUnreqBlock")
			}
			//conn.Disconnect()
			MutexRcv.Unlock()
			return
		}
		if sta == PH_STATUS_NEW {
			b2g.SendInvs = true
		}
		//println(c.ConnID, " - taking this new block")
		common.CountSafe("UnxpectedBlockNEW")
	}

	//println("block", b2g.BlockTreeNode.Height," len", len(b), " got from", conn.PeerAddr.Ip(), b2g.InProgress)

	prev_block_raw := b2g.Block.Raw // in case if it's a corrupt one
	b2g.Block.Raw = b
	if conn.X.Authorized {
		b2g.Block.Trusted.Set()
	}

	er := common.BlockChain.PostCheckBlock(b2g.Block)
	if er != nil {
		println("Corrupt block", hash.String(), b2g.BlockTreeNode.Height)
		println(" ... received from", conn.PeerAddr.Ip(), er.Error())
		//ioutil.WriteFile(hash.String()+"-"+conn.PeerAddr.Ip()+".bin", b, 0700)
		conn.DoS("BadBlock")

		// We don't need to remove from conn.GetBlockInProgress as we're disconnecting
		// ... decreasing of b2g.InProgress will also be done then.

		if b2g.Block.MerkleRootMatch() && !strings.Contains(er.Error(), "RPC_Result:bad-witness-nonce-size") {
			println(" <- It was a wrongly mined one - give it up")
			DelB2G(idx) //remove it from BlocksToGet
			if b2g.BlockTreeNode == LastCommitedHeader {
				LastCommitedHeader = LastCommitedHeader.Parent
			}
			common.BlockChain.DeleteBranch(b2g.BlockTreeNode, delB2G_callback)
		} else {
			println(" <- Merkle Root not matching - discard the data:", len(b2g.Block.Txs), b2g.Block.TxCount,
				b2g.Block.TxOffset, b2g.Block.NoWitnessSize, b2g.Block.BlockWeight, b2g.TotalInputs)
			// We just recived a corrupt copy from the peer. We will ask another peer for it.
			// But discard the data we extracted from this one, so it won't confuse us later.
			b2g.Block.Raw = prev_block_raw
			b2g.Block.NoWitnessSize, b2g.Block.BlockWeight, b2g.TotalInputs = 0, 0, 0
			b2g.Block.TxCount, b2g.Block.TxOffset = 0, 0
			b2g.Block.Txs = nil
		}

		MutexRcv.Unlock()
		return
	}

	orb := &OneReceivedBlock{TmStart: b2g.Started, TmPreproc: b2g.TmPreproc,
		TmDownload: conn.LastMsgTime, FromConID: conn.ConnID, DoInvs: b2g.SendInvs}

	conn.Mutex.Lock()
	bip := conn.GetBlockInProgress[idx]
	if bip == nil {
		//println(conn.ConnID, "received unrequested block", hash.String())
		common.CountSafe("UnreqBlockRcvd")
		conn.cntInc("NewBlock!")
		orb.TxMissing = -2
	} else {
		delete(conn.GetBlockInProgress, idx)
		conn.cntInc("NewBlock")
		orb.TxMissing = -1
	}
	conn.blocksreceived = append(conn.blocksreceived, time.Now())
	conn.Mutex.Unlock()

	ReceivedBlocks[idx] = orb
	DelB2G(idx) //remove it from BlocksToGet if no more pending downloads

	store_on_disk := len(BlocksToGet) > 10 && common.GetBool(&common.CFG.Memory.CacheOnDisk) && len(b2g.Block.Raw) > 16*1024
	MutexRcv.Unlock()

	var bei *btc.BlockExtraInfo

	size := len(b2g.Block.Raw)
	if store_on_disk {
		if e := ioutil.WriteFile(common.TempBlocksDir()+hash.String(), b2g.Block.Raw, 0600); e == nil {
			bei = new(btc.BlockExtraInfo)
			*bei = b2g.Block.BlockExtraInfo
			b2g.Block = nil
		} else {
			println("write tmp block:", e.Error())
		}
	}

	NetBlocks <- &BlockRcvd{Conn: conn, Block: b2g.Block, BlockTreeNode: b2g.BlockTreeNode,
		OneReceivedBlock: orb, BlockExtraInfo: bei, Size: size}
}

// parseLocatorsPayload parses the payload of "getblocks" or "getheaders" messages.
// It reads Version and VLen followed by the number of locators.
// Return zero-ed stop_hash is not present in the payload.
func parseLocatorsPayload(pl []byte) (h2get []*btc.Uint256, hashstop *btc.Uint256, er error) {
	var cnt uint64
	var ver uint32

	b := bytes.NewReader(pl)

	// version
	if er = binary.Read(b, binary.LittleEndian, &ver); er != nil {
		return
	}

	// hash count
	if cnt, er = btc.ReadVLen(b); er != nil {
		return
	}

	// block locator hashes
	if cnt > 0 {
		h2get = make([]*btc.Uint256, cnt)
		for i := 0; i < int(cnt); i++ {
			h2get[i] = new(btc.Uint256)
			if _, er = b.Read(h2get[i].Hash[:]); er != nil {
				return
			}
		}
	}

	// hash_stop
	hashstop = new(btc.Uint256)
	b.Read(hashstop.Hash[:]) // if not there, don't make a big deal about it

	return
}

// Call it with locked MutexRcv
func getBlockToFetch(max_height uint32, cnt_in_progress, avg_block_size uint) (lowest_found *OneBlockToGet) {
	for _, v := range BlocksToGet {
		if v.InProgress == cnt_in_progress && v.Block.Height <= max_height &&
			(lowest_found == nil || v.Block.Height < lowest_found.Block.Height) {
			lowest_found = v
		}
	}
	return
}

func (c *OneConnection) GetBlockData() (yes bool) {
	//MAX_GETDATA_FORWARD
	// Need to send getdata...?
	MutexRcv.Lock()
	defer MutexRcv.Unlock()

	if LowestIndexToBlocksToGet == 0 || len(BlocksToGet) == 0 {
		c.cntLockInc("FetchNoBlocksToGet")
		// wake up in one minute, just in case
		c.nextGetData = time.Now().Add(60 * time.Second)
		return
	}

	c.Mutex.Lock()
	if c.X.BlocksExpired > 0 { // Do not fetch blocks from nodes that had not given us some in the past
		c.Mutex.Unlock()
		c.cntLockInc("FetchHasBlocksExpired")
		return
	}
	cbip := len(c.GetBlockInProgress)
	c.Mutex.Unlock()

	if cbip >= MAX_PEERS_BLOCKS_IN_PROGRESS {
		c.cntLockInc("FetchMaxCountInProgress")
		// wake up in a few seconds, maybe some blocks will complete by then
		c.nextGetData = time.Now().Add(1 * time.Second)
		return
	}

	avg_block_size := common.AverageBlockSize.Get()
	block_data_in_progress := cbip * avg_block_size

	if block_data_in_progress > 0 && (block_data_in_progress+avg_block_size) > MAX_GETDATA_FORWARD {
		c.cntLockInc("FetchMaxBytesInProgress")
		// wake up in a few seconds, maybe some blocks will complete by then
		c.nextGetData = time.Now().Add(1 * time.Second) // wait for some blocks to complete
		return
	}

	var cnt uint64
	var block_type uint32

	if (c.Node.Services & btc.SERVICE_SEGWIT) != 0 {
		block_type = MSG_WITNESS_BLOCK
	} else {
		block_type = MSG_BLOCK
	}

	// We can issue getdata for this peer
	// Let's look for the lowest height block in BlocksToGet that isn't being downloaded yet

	common.Last.Mutex.Lock()
	max_height := common.Last.Block.Height + uint32(common.SyncMaxCacheBytes.Get()/avg_block_size)
	if max_height > common.Last.Block.Height+MAX_BLOCKS_FORWARD_CNT {
		max_height = common.Last.Block.Height + MAX_BLOCKS_FORWARD_CNT
	}
	common.Last.Mutex.Unlock()
	if max_height > c.Node.Height {
		max_height = c.Node.Height
	}
	if max_height > LastCommitedHeader.Height {
		max_height = LastCommitedHeader.Height
	}

	if common.BlockChain.Consensus.Enforce_SEGWIT != 0 && (c.Node.Services&btc.SERVICE_SEGWIT) == 0 { // no segwit node
		if max_height >= common.BlockChain.Consensus.Enforce_SEGWIT-1 {
			max_height = common.BlockChain.Consensus.Enforce_SEGWIT - 1
			if max_height <= common.Last.BlockHeight() {
				c.cntLockInc("FetchNoWitness")
				c.nextGetData = time.Now().Add(3600 * time.Second) // never do getdata
				return
			}
		}
	}

	invs := new(bytes.Buffer)
	var cnt_in_progress uint

	for {
		var lowest_found *OneBlockToGet

		// Get block to fetch:

		for bh := LowestIndexToBlocksToGet; bh <= max_height; bh++ {
			if idxlst, ok := IndexToBlocksToGet[bh]; ok {
				for _, idx := range idxlst {
					v := BlocksToGet[idx]
					if v.InProgress == cnt_in_progress && (lowest_found == nil || v.Block.Height < lowest_found.Block.Height) {
						c.Mutex.Lock()
						if _, ok := c.GetBlockInProgress[idx]; !ok {
							lowest_found = v
						}
						c.Mutex.Unlock()
					}
				}
			}
		}

		if lowest_found == nil {
			cnt_in_progress++
			if cnt_in_progress >= uint(common.CFG.Net.MaxBlockAtOnce) {
				break
			}
			continue
		}

		binary.Write(invs, binary.LittleEndian, block_type)
		invs.Write(lowest_found.BlockHash.Hash[:])
		lowest_found.InProgress++
		cnt++

		c.Mutex.Lock()
		c.GetBlockInProgress[lowest_found.BlockHash.BIdx()] =
			&oneBlockDl{hash: lowest_found.BlockHash, start: time.Now(), SentAtPingCnt: c.X.PingSentCnt}
		cbip = len(c.GetBlockInProgress)
		c.Mutex.Unlock()

		if cbip >= MAX_PEERS_BLOCKS_IN_PROGRESS {
			break // no more than 2000 blocks in progress / peer
		}
		block_data_in_progress += avg_block_size
		if block_data_in_progress > MAX_GETDATA_FORWARD {
			break
		}
	}

	if cnt == 0 {
		//println(c.ConnID, "fetch nothing", cbip, block_data_in_progress, max_height-common.Last.BlockHeight(), cnt_in_progress)
		c.cntLockInc("FetchNothing")
		// wake up in a few seconds, maybe it will be different next time
		c.nextGetData = time.Now().Add(5 * time.Second)
		return
	}

	bu := new(bytes.Buffer)
	btc.WriteVlen(bu, uint64(cnt))
	pl := append(bu.Bytes(), invs.Bytes()...)
	//println(c.ConnID, "fetching", cnt, "new blocks ->", cbip)
	c.SendRawMsg("getdata", pl)
	yes = true

	// we don't set c.nextGetData here, as it will be done in tick.go after "block" message
	/*
		c.Mutex.Lock()
		// we will come back here only after receiving half of the blocks that we have requested
		c.keepBlocksOver = len(c.GetBlockInProgress) / 2
		c.Mutex.Unlock()
	*/

	return
}
