package network

import (
	"fmt"
	"time"
	"bytes"
	"encoding/binary"
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
			bl, _, er := common.BlockChain.Blocks.BlockGet(btc.NewUint256(h[4:]))
			if er == nil {
				c.SendRawMsg("block", bl)
			} else {
				notfound = append(notfound, h[:]...)
			}
		} else if typ == 1 {
			// transaction
			TxMutex.Lock()
			if tx, ok := TransactionsToSend[btc.NewUint256(h[4:]).BIdx()]; ok && tx.Blocked==0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				TxMutex.Unlock()
				c.SendRawMsg("tx", tx.Data)
			} else {
				TxMutex.Unlock()
				notfound = append(notfound, h[:]...)
			}
		} else if typ == 4 {
			fmt.Println(c.ConnID, "is asking for cmpctblock")
			c.SendCmpctBlk(btc.NewUint256(h[4:]))
			fmt.Println(c.ConnID, "getdata cmpctblock processing finished")
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

	orb := &OneReceivedBlock{Time:bip.start, TmDownload:conn.LastMsgTime.Sub(bip.start)}
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
