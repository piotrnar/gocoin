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
		c.Mutex.Lock()
		c.InvStore(typ, h[4:36])
		c.Mutex.Unlock()

		common.CountSafe(fmt.Sprintf("GetdataType-%x",typ))
		if typ == MSG_BLOCK || typ == MSG_WITNESS_BLOCK {
			crec, _, er := common.BlockChain.Blocks.BlockGetExt(btc.NewUint256(h[4:]))
			//bl, _, er := common.BlockChain.Blocks.BlockGet(btc.NewUint256(h[4:]))
			if er == nil {
				bl := crec.Data
				if typ == MSG_BLOCK {
					// remove witness data from the block
					if crec.Block==nil {
						crec.Block, _ = btc.NewBlock(bl)
					}
					if crec.Block.OldData==nil {
						crec.Block.BuildTxList()
					}
					//println("block size", len(crec.Data), "->", len(bl))
					bl = crec.Block.OldData
				}
				c.SendRawMsg("block", bl)
			} else {
				notfound = append(notfound, h[:]...)
			}
		} else if typ == MSG_TX || typ == MSG_WITNESS_TX {
			// transaction
			TxMutex.Lock()
			if tx, ok := TransactionsToSend[btc.NewUint256(h[4:]).BIdx()]; ok && tx.Blocked==0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				TxMutex.Unlock()
				if tx.SegWit==nil || typ==MSG_WITNESS_TX {
					c.SendRawMsg("tx", tx.Data)
				} else {
					c.SendRawMsg("tx", tx.Serialize())
				}
			} else {
				TxMutex.Unlock()
				notfound = append(notfound, h[:]...)
			}
		} else if typ == MSG_CMPCT_BLOCK {
			c.SendCmpctBlk(btc.NewUint256(h[4:]))
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
		common.CountSafe("BlockSameRcvd")
		conn.Mutex.Lock()
		delete(conn.GetBlockInProgress, idx)
		conn.Mutex.Unlock()
		MutexRcv.Unlock()
		return
	}

	// remove from BlocksToGet:
	b2g := BlocksToGet[idx]
	if b2g==nil {
		//println("Block", hash.String(), " from", conn.PeerAddr.Ip(), conn.Node.Agent, " was not expected")

		var hdr [81]byte
		var sta int
		copy(hdr[:80], b[:80])
		sta, b2g = conn.ProcessNewHeader(hdr[:])
		if b2g==nil {
			if sta==PH_STATUS_FATAL {
				println("Unrequested Block: FAIL - Ban", conn.PeerAddr.Ip(), conn.Node.Agent)
				conn.DoS("BadUnreqBlock")
			} else {
				common.CountSafe("ErrUnreqBlock")
			}
			//conn.Disconnect()
			MutexRcv.Unlock()
			return;
		}
		//println(c.ConnID, " - taking this new block")
		common.CountSafe("UnxpectedBlockNEW")
	}

	//println("block", b2g.BlockTreeNode.Height," len", len(b), " got from", conn.PeerAddr.Ip(), b2g.InProgress)
	b2g.Block.Raw = b

	er := common.BlockChain.PostCheckBlock(b2g.Block)
	if er!=nil {
		b2g.InProgress--
		println("Corrupt block received from", conn.PeerAddr.Ip(), er.Error())
		//ioutil.WriteFile(hash.String() + ".bin", b, 0700)
		conn.DoS("BadBlock")
		MutexRcv.Unlock()
		return
	}

	orb := &OneReceivedBlock{TmStart:b2g.Started, TmPreproc:b2g.TmPreproc,
		TmDownload:conn.LastMsgTime, FromConID:conn.ConnID}

	conn.Mutex.Lock()
	bip := conn.GetBlockInProgress[idx]
	if bip==nil {
		//println(conn.ConnID, "received unrequested block", hash.String())
		common.CountSafe("UnreqBlockRcvd")
		conn.counters["NewBlock!"]++
		orb.TxMissing = -2
	} else {
		delete(conn.GetBlockInProgress, idx)
		conn.counters["NewBlock"]++
		orb.TxMissing = -1
	}
	conn.blocksreceived = append(conn.blocksreceived, time.Now())
	conn.Mutex.Unlock()

	ReceivedBlocks[idx] = orb
	delete(BlocksToGet, idx) //remove it from BlocksToGet if no more pending downloads

	MutexRcv.Unlock()

	NetBlocks <- &BlockRcvd{Conn:conn, Block:b2g.Block, BlockTreeNode:b2g.BlockTreeNode, OneReceivedBlock:orb}
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
	var block_type uint32

	if (c.Node.Services&SERVICE_SEGWIT) != 0 {
		block_type = MSG_WITNESS_BLOCK
	} else {
		block_type = MSG_BLOCK
	}

	//MAX_GETDATA_FORWARD
	avg_block_size := common.GetAverageBlockSize()

	// Need to send getdata...?
	MutexRcv.Lock()
	if len(BlocksToGet)>0 && uint(c.BlksInProgress()+1)*avg_block_size <= MAX_GETDATA_FORWARD {
		// We can issue getdata for this peer
		// Let's look for the lowest height block in BlocksToGet that isn't being downloaded yet

		common.Last.Mutex.Lock()
		max_height := common.Last.Block.Height + uint32(MAX_BLOCKS_FORWARD_SIZ/avg_block_size)
		if max_height > common.Last.Block.Height + MAX_BLOCKS_FORWARD_CNT {
			max_height = common.Last.Block.Height + MAX_BLOCKS_FORWARD_CNT
		}
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
				if common.BlockChain.Consensus.Enforce_SEGWIT!=0 {
					if (c.Node.Services&SERVICE_SEGWIT)==0 &&
						v.Block.Height>=common.BlockChain.Consensus.Enforce_SEGWIT {
						// We cannot get block data froim non-segwit peers
						continue
					}
				}

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

			binary.Write(invs, binary.LittleEndian, block_type)
			invs.Write(lowest_found.BlockHash.Hash[:])
			lowest_found.InProgress++
			cnt++

			c.Mutex.Lock()
			c.GetBlockInProgress[lowest_found.BlockHash.BIdx()] =
				&oneBlockDl{hash:lowest_found.BlockHash, start:time.Now()}
			c.Mutex.Unlock()

			if c.BlksInProgress()*int(avg_block_size) >= MAX_GETDATA_FORWARD || cnt==2000 {
				break
			}
		}

		if cnt > 0 {
			bu := new(bytes.Buffer)
			btc.WriteVlen(bu, uint64(cnt))
			pl := append(bu.Bytes(), invs.Bytes()...)
			println("fetching", cnt, "blocks from", c.PeerAddr.Ip(), len(invs.Bytes()), "...")
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
