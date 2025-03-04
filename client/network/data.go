package network

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (c *OneConnection) ProcessGetData(pl []byte) {
	//println(c.PeerAddr.Ip(), "getdata")
	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
		return
	}

	if b.Len() != int(cnt)*36 {
		println(c.ConnID, "Inconsistent getdata message:", b.Len(), "!=", cnt*36)
		c.DoS("GetDataLenERR")
		return
	}

	if c.unfinished_getdata != nil {
		//println(c.ConnID, "appending pending getdata with", cnt, "more invs")
		if c.unfinished_getdata.Len()+b.Len() > 36*50000 {
			c.DoS("GetDataTooBigA")
		} else {
			io.Copy(c.unfinished_getdata, b)
			common.CountSafe("GetDataPauseExt")
		}
		return
	}
	c.processGetData(b)
}

func (c *OneConnection) processGetData(b *bytes.Reader) {
	var typ uint32
	var h [36]byte
	for b.Len() > 0 {
		if c.SendingPaused() {
			// note that this function should not be called when c.unfinished_getdata is not nil
			c.unfinished_getdata = new(bytes.Buffer)
			//println(c.ConnID, "postpone getdata for", b.Len()/36, "invs")
			io.Copy(c.unfinished_getdata, b)
			common.CountSafe("GetDataPaused")
			break
		}

		b.Read(h[:])

		typ = binary.LittleEndian.Uint32(h[:4])
		c.Mutex.Lock()
		c.InvStore(typ, h[4:36])
		c.Mutex.Unlock()

		if typ == MSG_WITNESS_BLOCK { // Note: MSG_BLOCK is not longer supported
			common.CountSafe("GetdataBlockSw")
			hash := btc.NewUint256(h[4:])
			crec, trusted, er := common.BlockChain.Blocks.BlockGetExt(hash)
			if er == nil {
				bl := crec.Data
				c.SendRawMsg("block", bl, c.X.AuthAckGot && trusted)
			} else {
				//fmt.Println("BlockGetExt-2 failed for", hash.String(), er.Error())
				//notfound = append(notfound, h[:]...)
			}
		} else if typ == MSG_WITNESS_TX { // Note: MSG_TX is no longer supported
			common.CountSafe("GetdataTxSw")
			// ransaction
			txpool.TxMutex.Lock()
			if tx, ok := txpool.TransactionsToSend[btc.NewUint256(h[4:]).BIdx()]; ok && tx.Blocked == 0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				txpool.TxMutex.Unlock()
				c.SendRawMsg("tx", tx.Raw, c.X.AuthAckGot)
			} else {
				txpool.TxMutex.Unlock()
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
		}
	}
}

// netBlockReceived is called from a net conn thread.
func (c *OneConnection) netBlockReceived(cmd *BCmsg) {
	b := cmd.pl
	if len(b) < 100 {
		c.DoS("ShortBlock")
		return
	}

	hash := btc.NewSha2Hash(b[:80])
	idx := hash.BIdx()
	//println("got block data", hash.String())

	MutexRcv.Lock()

	// the blocks seems to be fine
	if rb, got := ReceivedBlocks[idx]; got {
		rb.DownloadCnt++
		Fetch.BlockBytesWasted += uint64(len(b))
		Fetch.BlockSameRcvd++
		c.Mutex.Lock()
		delete(c.GetBlockInProgress, idx)
		c.Mutex.Unlock()
		MutexRcv.Unlock()
		return
	}

	// remove from BlocksToGet:
	b2g := BlocksToGet[idx]
	if b2g == nil {
		//println("Block", hash.String(), " from", conn.PeerAddr.Ip(), conn.Node.Agent, " was not expected")

		var sta int
		sta, b2g = c.ProcessNewHeader(b[:80])
		if b2g == nil {
			if sta == PH_STATUS_FATAL {
				println("Unrequested Block: FAIL - Ban", c.PeerAddr.Ip(), c.Node.Agent)
				c.DoS("BadUnreqBlock")
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
	if cmd.trusted {
		b2g.Block.Trusted.Set()
		common.CountSafe("TrustedMsg-Block")
	}

	er := common.BlockChain.PostCheckBlock(b2g.Block)
	if er != nil {
		println("Corrupt block", hash.String(), b2g.BlockTreeNode.Height)
		println(" ... received from", c.PeerAddr.Ip(), er.Error())
		//ioutil.WriteFile(hash.String()+"-"+conn.PeerAddr.Ip()+".bin", b, 0700)
		c.DoS("BadBlock")

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
				b2g.Block.TxOffset, b2g.Block.BlockWeight, b2g.TotalInputs)
			// We just recived a corrupt copy from the peer. We will ask another peer for it.
			// But discard the data we extracted from this one, so it won't confuse us later.
			b2g.Block.Raw = prev_block_raw
			b2g.Block.BlockWeight, b2g.TotalInputs = 0, 0
			b2g.Block.TxCount, b2g.Block.TxOffset = 0, 0
			b2g.Block.Txs = nil
		}

		MutexRcv.Unlock()
		return
	}

	orb := &OneReceivedBlock{TmStart: b2g.Started, TmPreproc: b2g.TmPreproc,
		TmDownload: c.LastMsgTime, FromConID: c.ConnID, DoInvs: b2g.SendInvs}

	c.Mutex.Lock()
	bip := c.GetBlockInProgress[idx]
	if bip == nil {
		//println(conn.ConnID, "received unrequested block", hash.String())
		common.CountSafe("UnreqBlockRcvd")
		c.cntInc("NewBlock!")
		orb.TxMissing = -2
	} else {
		delete(c.GetBlockInProgress, idx)
		c.cntInc("NewBlock")
		orb.TxMissing = -1
	}
	c.blocksreceived = append(c.blocksreceived, time.Now())
	c.Mutex.Unlock()

	ReceivedBlocks[idx] = orb
	DelB2G(idx) //remove it from BlocksToGet if no more pending downloads

	store_on_disk := len(BlocksToGet) > 10 && common.Get(&common.CFG.Memory.CacheOnDisk) &&
		len(b2g.Block.Raw) > 16*1024 && LastCommitedHeader.Height-b2g.BlockTreeNode.Height > 10
	MutexRcv.Unlock()

	var bei *btc.BlockExtraInfo

	size := len(b2g.Block.Raw)
	if store_on_disk {
		fname := common.TempBlocksDir() + hash.String()
		if e := os.WriteFile(fname, b2g.Block.Raw, 0600); e == nil {
			buf := make([]byte, 0, 64*len(b2g.Block.Txs))
			for _, tx := range b2g.Block.Txs {
				buf = append(buf, tx.WTxID().Hash[:]...)
				if tx.SegWit != nil { // if segwit was nil, the previous append already did this hash
					buf = append(buf, tx.Hash.Hash[:]...)
				}
			}
			os.WriteFile(fname+".hashes", buf, 0600)
			bei = new(btc.BlockExtraInfo)
			*bei = b2g.Block.BlockExtraInfo
			b2g.Block = nil
		} else {
			println("write tmp block data:", e.Error())
		}
	}

	queueNewBlock(&BlockRcvd{Conn: c, Block: b2g.Block, BlockTreeNode: b2g.BlockTreeNode,
		OneReceivedBlock: orb, BlockExtraInfo: bei, Size: size})
}

func queueNewBlock(newbl *BlockRcvd) {
	select {
	case NetBlocks <- newbl:
		common.CountSafe("NetBlockQueued")
	default:
		CachedBlocksAdd(newbl)
		common.CountSafe("NetBlockCached")
	}
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

var Fetc struct {
	HeightA uint64
	HeightB uint64
	HeightC uint64
	HeightD uint64
	B2G     uint64
	C       [6]uint64
}

var Fetch struct {
	LastCacheEmpty     time.Time
	NoBlocksToGet      uint64
	HasBlocksExpired   uint64
	MaxCountInProgress uint64
	MaxBytesInProgress uint64
	NoWitness          uint64
	Nothing            uint64
	BlksCntMax         [6]uint64
	ReachEndOfLoop     uint64
	ReachMaxCnt        uint64
	ReachMaxData       uint64

	BlockBytesWasted uint64
	BlockSameRcvd    uint64

	CacheEmpty uint64
}

func (c *OneConnection) GetBlockData() (yes bool) {
	//MAX_GETDATA_FORWARD
	// Need to send getdata...?
	MutexRcv.Lock()
	defer MutexRcv.Unlock()

	if LowestIndexToBlocksToGet == 0 || len(BlocksToGet) == 0 {
		Fetch.NoBlocksToGet++
		// wake up in one minute, just in case
		c.nextGetData = time.Now().Add(60 * time.Second)
		return
	}

	c.Mutex.Lock()
	if c.X.BlocksExpired > 0 { // Do not fetch blocks from nodes that had not given us some in the past
		c.Mutex.Unlock()
		Fetch.HasBlocksExpired++
		return
	}
	cbip := len(c.GetBlockInProgress)
	c.Mutex.Unlock()

	if cbip >= MAX_PEERS_BLOCKS_IN_PROGRESS {
		Fetch.MaxCountInProgress++
		// wake up in a few seconds, maybe some blocks will complete by then
		c.nextGetData = time.Now().Add(1 * time.Second)
		return
	}

	avg_block_size := common.AverageBlockSize.Get()
	block_data_in_progress := cbip * avg_block_size

	if block_data_in_progress > 0 && (block_data_in_progress+avg_block_size) > MAX_GETDATA_FORWARD {
		Fetch.MaxBytesInProgress++
		// wake up in a few seconds, maybe some blocks will complete by then
		c.nextGetData = time.Now().Add(1 * time.Second) // wait for some blocks to complete
		return
	}

	var cnt uint64

	// We can issue getdata for this peer
	// Let's look for the lowest height block in BlocksToGet that isn't being downloaded yet

	last_block_height := common.Last.BlockHeight()
	max_height := last_block_height + uint32(common.SyncMaxCacheBytes.Get()/avg_block_size)

	Fetc.HeightA = uint64(last_block_height)
	Fetc.HeightB = uint64(LowestIndexToBlocksToGet)
	Fetc.HeightC = uint64(max_height)

	if max_height > last_block_height+MAX_BLOCKS_FORWARD_CNT {
		max_height = last_block_height + MAX_BLOCKS_FORWARD_CNT
	}
	max_max_height := max_height
	if max_max_height > c.Node.Height {
		max_max_height = c.Node.Height
	}
	if max_max_height > LastCommitedHeader.Height {
		max_max_height = LastCommitedHeader.Height
	}

	Fetc.HeightD = uint64(max_height)

	max_blocks_at_once := common.Get(&common.CFG.Net.MaxBlockAtOnce)
	max_blocks_forward := max_height - last_block_height
	invs := new(bytes.Buffer)
	var cnt_in_progress uint32
	var lowest_found *OneBlockToGet

	for {
		// Find block to fetch:
		max_height = last_block_height + max_blocks_forward/(cnt_in_progress+1)
		if max_height > max_max_height {
			max_height = max_max_height
		}
		if max_height < LowestIndexToBlocksToGet {
			Fetch.BlksCntMax[cnt_in_progress]++
			break
		}
		for bh := LowestIndexToBlocksToGet; bh <= max_height; bh++ {
			if idxlst, ok := IndexToBlocksToGet[bh]; ok {
				for _, idx := range idxlst {
					v := BlocksToGet[idx]
					if uint32(v.InProgress) == cnt_in_progress {
						c.Mutex.Lock()
						_, ok := c.GetBlockInProgress[idx]
						c.Mutex.Unlock()
						if !ok {
							lowest_found = v
							goto found_it
						}
					}
				}
			}
		}

		// If we came here, we did not find it.
		if cnt_in_progress++; cnt_in_progress >= max_blocks_at_once {
			Fetch.ReachEndOfLoop++
			break
		}
		continue

	found_it:
		Fetc.C[lowest_found.InProgress]++

		binary.Write(invs, binary.LittleEndian, MSG_WITNESS_BLOCK)
		invs.Write(lowest_found.BlockHash.Hash[:])
		lowest_found.InProgress++
		cnt++

		c.Mutex.Lock()
		c.GetBlockInProgress[lowest_found.BlockHash.BIdx()] =
			&oneBlockDl{hash: lowest_found.BlockHash, start: time.Now(), SentAtPingCnt: c.X.PingSentCnt}
		cbip = len(c.GetBlockInProgress)
		c.Mutex.Unlock()

		if cbip >= MAX_PEERS_BLOCKS_IN_PROGRESS {
			Fetch.ReachMaxCnt++
			break // no more than 2000 blocks in progress / peer
		}
		block_data_in_progress += avg_block_size
		if block_data_in_progress > MAX_GETDATA_FORWARD {
			Fetch.ReachMaxData++
			break
		}
	}

	Fetc.B2G = uint64(cnt)

	if cnt == 0 {
		//println(c.ConnID, "fetch nothing", cbip, block_data_in_progress, max_height-common.Last.BlockHeight(), cnt_in_progress)
		Fetch.Nothing++
		// wake up in a few seconds, maybe it will be different next time
		c.nextGetData = time.Now().Add(5 * time.Second)
		return
	}

	bu := new(bytes.Buffer)
	btc.WriteVlen(bu, uint64(cnt))
	pl := append(bu.Bytes(), invs.Bytes()...)
	//println(c.ConnID, "fetching", cnt, "new blocks ->", cbip)
	c.SendRawMsg("getdata", pl, false)
	yes = true

	// we don't set c.nextGetData here, as it will be done in tick.go after "block" message
	c.Mutex.Lock()
	// we will come back here only after receiving half of the blocks that we have requested
	c.keepBlocksOver = 3 * len(c.GetBlockInProgress) / 4
	c.Mutex.Unlock()

	return
}
