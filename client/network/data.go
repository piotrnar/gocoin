package network

import (
	"fmt"
	"time"
	"bytes"
	"encoding/hex"
	"crypto/sha256"
	"encoding/binary"
	"github.com/dchest/siphash"
	"github.com/piotrnar/gocoin/lib/btc"
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


// This function is called from a net conn thread
func netBlockReceived(conn *OneConnection, b []byte) {
    now := time.Now()

	if len(b)<100 {
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
		MutexRcv.Unlock()
		common.CountSafe("BlockSameRcvd")
		conn.Mutex.Lock()
		delete(conn.GetBlockInProgress, idx)
		conn.Mutex.Unlock()
		return
	}

	// remove from BlocksToGet:
	rec := BlocksToGet[idx]
	if rec==nil {
		MutexRcv.Unlock()
		println("Block", hash.String(), " from", conn.PeerAddr.Ip(), " was not expected")

		if _, got := ReceivedBlocks[idx]; got {
			println("Already received it")
		}

		bip := conn.GetBlockInProgress[idx]
		if bip != nil {
			println("... but is in progress")
		} else {
			println("... NOT in progress")
		}

		conn.Disconnect()
		return
	}

	//println("block", rec.BlockTreeNode.Height," len", len(b), " got from", conn.PeerAddr.Ip(), rec.InProgress)
	rec.InProgress--
	rec.Block.Raw = b

	er := common.BlockChain.PostCheckBlock(rec.Block)
	if er!=nil {
		MutexRcv.Unlock()
		println("Corrupt block received from", conn.PeerAddr.Ip())
		conn.DoS("BadBlock")
		return
	}

	conn.Mutex.Lock()
	bip := conn.GetBlockInProgress[idx]
	if bip==nil {
		conn.Mutex.Unlock()
		MutexRcv.Unlock()
		println("unexpected block received from", conn.PeerAddr.Ip())
		common.CountSafe("UnxpectedBlockRcvd")
		conn.DoS("UnexpBlock")
		return
	}

	orb := &OneReceivedBlock{Time:bip.start, TmDownload:now.Sub(bip.start)}
	delete(conn.GetBlockInProgress, idx)
	conn.Mutex.Unlock()

	ReceivedBlocks[idx] = orb
	delete(BlocksToGet, idx) //remove it from BlocksToGet if no more pending downloads
	MutexRcv.Unlock()

	NetBlocks <- &BlockRcvd{Conn:conn, Block:rec.Block,
		BlockTreeNode:rec.BlockTreeNode, OneReceivedBlock:orb}
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


// Call it with locked MutexRcv
func getBlockToFetch(max_height uint32, cnt_in_progress, avg_block_size uint) (lowest_found *OneBlockToGet) {
	for _, v := range BlocksToGet {
		if v.InProgress==cnt_in_progress && v.Block.Height <= max_height &&
			(lowest_found==nil || v.Block.Height < lowest_found.Block.Height) {
				lowest_found = v
		}
	}
	return
}

func (c *OneConnection) GetBlockData() (yes bool) {
	//MAX_GETDATA_FORWARD
	avg_block_size := common.GetAverageBlockSize()

	// Need to send getdata...?
	MutexRcv.Lock()
	if len(BlocksToGet)>0 && uint(len(c.GetBlockInProgress)+1)*avg_block_size <= MAX_GETDATA_FORWARD {
		//uint32(len(c.GetBlockInProgress)) < atomic.LoadUint32(&common.CFG.Net.MaxBlockAtOnce)
		// We can issue getdata for this peer
		// Let's look for the lowest height block in BlocksToGet that isn't being downloaded yet

		common.Last.Mutex.Lock()
		max_height := common.Last.Block.Height + MAX_BLOCKS_FORWARD
		common.Last.Mutex.Unlock()
		if max_height > c.Node.Height {
			max_height = c.Node.Height
		}

		invs := new(bytes.Buffer)
		var cnt uint64
		var cnt_in_progress uint

		for {
			var lowest_found *OneBlockToGet

			// Get block to fetch:

			for _, v := range BlocksToGet {
				if v.InProgress==cnt_in_progress && v.Block.Height <= max_height &&
					(lowest_found==nil || v.Block.Height < lowest_found.Block.Height) {
						c.Mutex.Lock()
						if _, ok := c.GetBlockInProgress[v.BlockHash.BIdx()]; !ok {
							lowest_found = v
						}
						c.Mutex.Unlock()
				}
			}

			if lowest_found==nil {
				cnt_in_progress++
				if cnt_in_progress>=uint(common.CFG.Net.MaxBlockAtOnce) {
					break
				}
				continue
			}

			binary.Write(invs, binary.LittleEndian, uint32(2))
			invs.Write(lowest_found.BlockHash.Hash[:])
			lowest_found.InProgress++
			cnt++

			c.Mutex.Lock()
			c.GetBlockInProgress[lowest_found.BlockHash.BIdx()] =
				&oneBlockDl{hash:lowest_found.BlockHash, start:time.Now()}
			c.Mutex.Unlock()

			if len(c.GetBlockInProgress)*int(avg_block_size) >= MAX_GETDATA_FORWARD || cnt==2000 {
				break
			}
		}

		if cnt > 0 {
			bu := new(bytes.Buffer)
			btc.WriteVlen(bu, uint64(cnt))
			pl := append(bu.Bytes(), invs.Bytes()...)
			//println("fetching", cnt, "blocks from", c.PeerAddr.Ip(), len(invs.Bytes()), "...")
			c.SendRawMsg("getdata", pl)
			yes = true
		} else {
			//println("fetch nothing from", c.PeerAddr.Ip())
			c.counters["FetchNothing"]++
		}
	}
	MutexRcv.Unlock()

	return
}


func (c *OneConnection) CheckGetBlockData() bool {
	if c.X.GetBlocksDataNow {
		c.X.GetBlocksDataNow = false
		c.X.LastFetchTried = time.Now()
		if c.GetBlockData() {
			return true
		}
	}
	return false
}


func ShortIDToU64(d []byte) uint64 {
	return uint64(d[0]) | (uint64(d[1])<<8) | (uint64(d[2])<<16) |
		(uint64(d[3])<<24) | (uint64(d[4])<<32) | (uint64(d[5])<<40)
}


func (c *OneConnection) ProcessCmpctBlock(pl []byte) {
	if len(pl)<90 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error A", hex.EncodeToString(pl))
		c.DoS("CmpctBlkErrA")
		return
	}

	MutexRcv.Lock()
	defer MutexRcv.Unlock()


	var tmp_hdr [81]byte
	copy(tmp_hdr[:80], pl[:80])
	sta, b2g := ProcessNewHeader(tmp_hdr[:]) // ProcessNewHeader() needs byte(0) after the header,
	// but don't try to change it to ProcessNewHeader(append(pl[:80], 0)) as it'd overwrite pl[80]

	if b2g==nil {
		/*fmt.Println(c.ConnID, "Don't process CompactBlk", btc.NewSha2Hash(pl[:80]),
			hex.EncodeToString(pl[80:88]), "->", sta)*/
		common.CountSafe("CmpctBlockHdrNo")
		if sta==PH_STATUS_ERROR {
			c.Misbehave("BadCmpct", 50) // do it 20 times and you are banned
		} else if sta==PH_STATUS_FATAL {
			c.DoS("BadCmpct")
		}
		return
	}
	fmt.Println("============================", b2g.Block.Height, len(pl), "==================================")
	fmt.Println(c.ConnID, "Process CompactBlk", btc.NewSha2Hash(pl[:80]),
		hex.EncodeToString(pl[80:88]), "->", sta, "inp", b2g.InProgress)

	// if we got here, we shall download this block
	if c.Node.Height < b2g.Block.Height {
		c.Node.Height = b2g.Block.Height
	}

	var n, idx, shortidscnt, shortidx_idx, prefilledcnt int

	col := new(CmpctBlockCollector)
	col.Header = b2g.Block.Raw[:80]

	offs := 88
	shortidscnt, n = btc.VLen(pl[offs:])
	if shortidscnt<0 || n>3 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error B", hex.EncodeToString(pl))
		c.DoS("CmpctBlkErrB")
		return
	}
	offs += n
	shortidx_idx = offs
	shortids := make(map[uint64] *OneTxToSend, shortidscnt)
	for i:=0; i<int(shortidscnt); i++ {
		if len(pl[offs:offs+6])<6 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error B2", hex.EncodeToString(pl))
			c.DoS("CmpctBlkErrB2")
			return
		}
		shortids[ShortIDToU64(pl[offs:offs+6])] = nil
		offs += 6
	}

	prefilledcnt, n = btc.VLen(pl[offs:])
	if prefilledcnt<0 || n>3 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error C", hex.EncodeToString(pl))
		c.DoS("CmpctBlkErrC")
		return
	}
	offs += n

	col.Txs = make([]interface{}, prefilledcnt+shortidscnt)

	exp := 0
	for i:=0; i<int(prefilledcnt); i++ {
		idx, n = btc.VLen(pl[offs:])
		if idx<0 || n>3 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error D", hex.EncodeToString(pl))
			c.DoS("CmpctBlkErrD")
			return
		}
		idx += exp
		offs += n
		n = btc.TxSize(pl[offs:])
		if n==0 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error E", hex.EncodeToString(pl))
			c.DoS("CmpctBlkErrE")
			return
		}
		col.Txs[idx] = pl[offs:offs+n]
		//fmt.Println("  prefilledtxn", i, idx, ":", btc.NewSha2Hash(pl[offs:offs+n]).String())
		offs += n
		exp = int(idx)+1
	}


	// calculate K0 and K1 params for siphash-4-2
	sha := sha256.New()
	sha.Write(pl[:88])
	kks := sha.Sum(nil)
	col.K0 = binary.LittleEndian.Uint64(kks[0:8])
	col.K1 = binary.LittleEndian.Uint64(kks[8:16])

	var cnt_found int

	TxMutex.Lock()

	for _, v := range TransactionsToSend {
		sid := siphash.Hash(col.K0, col.K1, v.Tx.Hash.Hash[:]) & 0xffffffffffff
		if ptr, ok := shortids[sid]; ok {
			if ptr!=nil {
				common.CountSafe("ShortIDSame")
				println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "Same short ID - abort")
				return
			}
			shortids[sid] = v
			cnt_found++
		}
	}

	var msg *bytes.Buffer

	missing := len(shortids) - cnt_found
	fmt.Println(c.ConnID, "ShortIDs", cnt_found, "/", shortidscnt, "  Prefilled", prefilledcnt, "  Missing", missing, "  MemPool:", len(TransactionsToSend))
	if missing > 0 {
		msg = new(bytes.Buffer)
		msg.Write(b2g.Block.Hash.Hash[:])
		btc.WriteVlen(msg, uint64(missing))
		exp = 0
		col.Sid2idx = make(map[uint64]int, missing)
	}
	for n=0; n<len(col.Txs); n++ {
		switch col.Txs[n].(type) {
			case []byte: // prefilled transaction

			default:
				sid := ShortIDToU64(pl[shortidx_idx:shortidx_idx+6])
				if t2s, ok := shortids[sid]; ok {
					if t2s!=nil {
						col.Txs[n] = t2s.Data
					} else {
						col.Txs[n] = sid
						col.Sid2idx[sid] = n
						if missing > 0 {
							btc.WriteVlen(msg, uint64(n-exp))
							exp = n+1
						}
					}
				} else {
					panic(fmt.Sprint("Tx idx ", n, " is missing - this should not happen!!!"))
				}
				shortidx_idx += 6
		}
	}
	TxMutex.Unlock()

	if missing==0 {
		fmt.Println(c.ConnID, "Instant Assembling block #", b2g.Block.Height)
		sta := time.Now()
		b2g.Block.UpdateContent(assemble_compact_block(col))
		sto := time.Now()
		er := common.BlockChain.PostCheckBlock(b2g.Block)
		if er!=nil {
			println(c.ConnID, "Corrupt CmpctBlkA")
			c.DoS("BadCmpctBlockA")
			return
		}
		fmt.Println(c.ConnID, "Instatnt PostCheckBlock OK", sto.Sub(sta), time.Now().Sub(sta))
		idx := b2g.Block.Hash.BIdx()
		orb := &OneReceivedBlock{Time:time.Now()}
		ReceivedBlocks[idx] = orb
		delete(BlocksToGet, idx) //remove it from BlocksToGet if no more pending downloads
		NetBlocks <- &BlockRcvd{Conn:c, Block:b2g.Block, BlockTreeNode:b2g.BlockTreeNode, OneReceivedBlock:orb}
	} else {
		b2g.InProgress++
		c.Mutex.Lock()
		c.GetBlockInProgress[b2g.Block.Hash.BIdx()] = &oneBlockDl{hash:b2g.Block.Hash, start:time.Now(), col:col}
		c.Mutex.Unlock()
		fmt.Println(c.ConnID, "Sending getblocktxn for", missing, "missing txs.  ", msg.Len(), "bytes")
		c.SendRawMsg("getblocktxn", msg.Bytes())
	}
}

func (c *OneConnection) ProcessBlockTxn(pl []byte) {
	if len(pl)<33 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn error A", hex.EncodeToString(pl))
		c.DoS("BlkTxnErrLen")
		return
	}
	hash := btc.NewUint256(pl[:32])
	le, n := btc.VLen(pl[32:])
	if le<0 || n>3 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn error B", hex.EncodeToString(pl))
		c.DoS("BlkTxnErrCnt")
		return
	}
	MutexRcv.Lock()
	defer MutexRcv.Unlock()

	idx := hash.BIdx()

	c.Mutex.Lock()
	bip := c.GetBlockInProgress[idx]
	if bip==nil {
		c.Mutex.Unlock()
		println(c.ConnID, "Unexpected BlkTxn1 received from", c.PeerAddr.Ip())
		common.CountSafe("BlkTxnUnexp1")
		c.DoS("BlkTxnErrBip")
		return
	}
	col := bip.col
	if col==nil {
		c.Mutex.Unlock()
		println("Unexpected BlockTxn2 not expected from this peer", c.PeerAddr.Ip())
		common.CountSafe("UnxpectedBlockTxn")
		c.DoS("BlkTxnErrCol")
		return
	}
	delete(c.GetBlockInProgress, idx)
	c.Mutex.Unlock()

	// the blocks seems to be fine
	if rb, got := ReceivedBlocks[idx]; got {
		rb.Cnt++
		common.CountSafe("BlkTxnSameRcvd")
		return
	}

	b2g := BlocksToGet[idx]
	if b2g==nil {
		panic("BlockTxn: Block missing from BlocksToGet")
		return
	}
	delete(BlocksToGet, idx)
	//b2g.InProgress--

	fmt.Println(c.ConnID, "BlockTxn:", le, "new txs for block #", b2g.Block.Height, "   ", len(pl), "bytes")

	offs := 32+n
	for offs < len(pl) {
		n = btc.TxSize(pl[offs:])
		if n==0 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn corrupt TX")
			c.DoS("BlkTxnErrTx")
			return
		}
		raw_tx := pl[offs:offs+n]
		tx_hash := btc.NewSha2Hash(raw_tx)
		offs += n

		sid := siphash.Hash(col.K0, col.K1, tx_hash.Hash[:]) & 0xffffffffffff
		if idx, ok := col.Sid2idx[sid]; ok {
			col.Txs[idx] = raw_tx
		} else {
			common.CountSafe("ShortIDUnknown")
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn TX (short) ID unknown")
			return
		}
	}

	fmt.Println(c.ConnID, "Assembling block #", b2g.Block.Height)
	sta := time.Now()
	b2g.Block.UpdateContent(assemble_compact_block(col))
	sto := time.Now()
	er := common.BlockChain.PostCheckBlock(b2g.Block)
	if er!=nil {
		println(c.ConnID, "Corrupt CmpctBlkB")
		c.DoS("BadCmpctBlockB")
		return
	}
	fmt.Println(c.ConnID, "PostCheckBlock OK", sto.Sub(sta), time.Now().Sub(sta))
	orb := &OneReceivedBlock{Time:bip.start, TmDownload:time.Now().Sub(bip.start)}
	ReceivedBlocks[idx] = orb
	NetBlocks <- &BlockRcvd{Conn:c, Block:b2g.Block, BlockTreeNode:b2g.BlockTreeNode, OneReceivedBlock:orb}
}


func assemble_compact_block(col *CmpctBlockCollector) []byte {
	bdat := new(bytes.Buffer)
	bdat.Write(col.Header)
	btc.WriteVlen(bdat, uint64(len(col.Txs)))
	for _, txd := range col.Txs {
		bdat.Write(txd.([]byte))
	}
	return bdat.Bytes()
}
