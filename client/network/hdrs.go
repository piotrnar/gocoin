package network

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

const (
	PH_STATUS_NEW   = 1
	PH_STATUS_FRESH = 2
	PH_STATUS_OLD   = 3
	PH_STATUS_ERROR = 4
	PH_STATUS_FATAL = 5
)

func (c *OneConnection) ProcessNewHeader(hdr []byte) (int, *OneBlockToGet) {
	var ok bool
	var b2g *OneBlockToGet
	bl, _ := btc.NewBlock(hdr)

	c.Mutex.Lock()
	c.InvStore(MSG_BLOCK, bl.Hash.Hash[:])
	c.Mutex.Unlock()

	if _, ok = DiscardedBlocks[bl.Hash.BIdx()]; ok {
		common.CountSafe("HdrRejected")
		//fmt.Println("", bl.Hash.String(), "-header for already rejected block")
		return PH_STATUS_FATAL, nil
	}

	if _, ok = ReceivedBlocks[bl.Hash.BIdx()]; ok {
		common.CountSafe("HeaderOld")
		//fmt.Println("", i, bl.Hash.String(), "-already received")
		return PH_STATUS_OLD, nil
	}

	if b2g, ok = BlocksToGet[bl.Hash.BIdx()]; ok {
		common.CountSafe("HeaderFresh")
		//fmt.Println(c.PeerAddr.Ip(), "block", bl.Hash.String(), " not new but get it")
		if len(b2g.OnlyFetchFrom) > 0 {
			b2g.OnlyFetchFrom = append(b2g.OnlyFetchFrom, c.ConnID)
		}
		return PH_STATUS_FRESH, b2g
	}

	common.CountSafe("HeaderNew")
	//fmt.Println("", i, bl.Hash.String(), " - NEW!")

	common.BlockChain.BlockIndexAccess.Lock()
	defer common.BlockChain.BlockIndexAccess.Unlock()

	if dos, _, er := common.BlockChain.PreCheckBlock(bl); er != nil {
		common.CountSafe("PreCheckBlockFail")
		if c.X.Authorized {
			println("Error from PreCheckBlock", bl.Height, bl.Hash.String(), "\n  ", er.Error(), "  dos:", dos, "  ts:", bl.BlockTime(), "/", time.Now().Unix())
		}
		if dos {
			return PH_STATUS_FATAL, nil
		} else {
			return PH_STATUS_ERROR, nil
		}
	}

	node := common.BlockChain.AcceptHeader(bl)
	b2g = &OneBlockToGet{Started: c.LastMsgTime, Block: bl, BlockTreeNode: node, InProgress: 0}
	AddB2G(b2g)
	if node.Height > LastCommitedHeader.Height {
		LastCommitedHeader = node
		//println("LastCommitedHeader:", LastCommitedHeader.Height, "-change to", LastCommitedHeader.BlockHash.String())
	} else {
		//println("LastCommitedHeader:", LastCommitedHeader.Height, "new:", node.Height, "-keep!")
	}

	if common.LastTrustedBlockMatch(node.BlockHash) {
		common.Set(&common.LastTrustedBlockHeight, node.Height)
		for node != nil {
			node.Trusted.Set()
			node = node.Parent
		}
	}
	b2g.Block.Trusted.Store(b2g.BlockTreeNode.Trusted.Get())

	if common.Get(&common.BlockChainSynchronized) {
		b2g.OnlyFetchFrom = []uint32{c.ConnID}
	}
	return PH_STATUS_NEW, b2g
}

func (c *OneConnection) HandleHeaders(pl []byte) (new_headers_got int) {
	var highest_block_found uint32

	c.MutexSetBool(&c.X.GetHeadersInProgress, false)

	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("HandleHeaders:", e.Error(), c.PeerAddr.Ip())
		return
	}

	if cnt > 2000 {
		println("HandleHeaders: too many headers", cnt, c.PeerAddr.Ip(), c.Node.Agent)
		c.DoS("HdrErrX")
		return
	}

	HeadersReceived.Add(1)

	if cnt > 0 {
		MutexRcv.Lock()
		defer MutexRcv.Unlock()

		for i := 0; i < int(cnt); i++ {
			hdr := make([]byte, 80)

			if n, _ := b.Read(hdr); n != 80 {
				println("HandleHeaders: pl too short 1", c.PeerAddr.Ip(), c.Node.Agent)
				c.DoS("HdrErr1")
				return
			}

			if _, e = btc.ReadVLen(b); e != nil {
				println("HandleHeaders: pl too short 2", c.PeerAddr.Ip(), c.Node.Agent)
				c.DoS("HdrErr2")
				return
			}

			sta, b2g := c.ProcessNewHeader(hdr)
			if b2g == nil {
				if sta == PH_STATUS_FATAL {
					//println("c.DoS(BadHeader)")
					c.DoS("BadHeader")
					return
				} else if sta == PH_STATUS_ERROR {
					//println("c.Misbehave(BadHeader)")
					c.Misbehave("BadHeader", 50) // do it 20 times and you are banned
				}
			} else {
				if sta == PH_STATUS_NEW {
					if cnt == 1 {
						b2g.SendInvs = true
					}
					new_headers_got++
				}
				if b2g.Block.Height > highest_block_found {
					highest_block_found = b2g.Block.Height
				}
				if c.Node.Height < b2g.Block.Height {
					c.Mutex.Lock()
					c.Node.Height = b2g.Block.Height
					c.Mutex.Unlock()
				}
				c.MutexSetBool(&c.X.GetBlocksDataNow, true)
				if b2g.TmPreproc.IsZero() { // do not overwrite TmPreproc (in case of PH_STATUS_FRESH)
					b2g.TmPreproc = time.Now()
				}
			}
		}
	} else {
		common.CountSafe("EmptyHeadersRcvd")
		HeadersReceived.Add(4)
	}

	c.Mutex.Lock()
	c.X.LastHeadersEmpty = highest_block_found <= c.X.LastHeadersHeightAsk
	c.X.TotalNewHeadersCount += new_headers_got
	if new_headers_got == 0 {
		c.X.AllHeadersReceived = true
	}
	c.Mutex.Unlock()

	return
}

func (c *OneConnection) ReceiveHeadersNow() {
	c.Mutex.Lock()
	if c.X.Debug {
		println(c.ConnID, "- ReceiveHeadersNow()")
	}
	c.X.AllHeadersReceived = false
	c.Mutex.Unlock()
}

// GetHeaders handles getheaders protocol command.
// https://en.bitcoin.it/wiki/Protocol_specification#getheaders
func (c *OneConnection) GetHeaders(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)

	if e != nil {
		//println(time.Now().Format("2006-01-02 15:04:05"), c.ConnID, "GetHeaders: error parsing payload from", c.PeerAddr.Ip(), c.Node.Agent, e.Error())
		c.DoS("BadGetHdrsA")
		return
	}

	if len(h2get) > 101 || hashstop == nil {
		//println("GetHeaders: too many locators", len(h2get), "or missing hashstop", hashstop, "from", c.PeerAddr.Ip(), c.Node.Agent, e.Error())
		c.DoS("BadGetHdrsB")
		return
	}

	var best_block, last_block *chain.BlockTreeNode

	//common.Last.Mutex.Lock()
	MutexRcv.Lock()
	last_block = LastCommitedHeader
	MutexRcv.Unlock()
	//common.Last.Mutex.Unlock()

	common.BlockChain.BlockIndexAccess.Lock()

	//println("GetHeaders", len(h2get), hashstop.String())
	if len(h2get) > 0 {
		for i := range h2get {
			if bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
				if best_block == nil || bl.Height > best_block.Height {
					//println(" ... bbl", i, bl.Height, bl.BlockHash.String())
					best_block = bl
				}
			}
		}
	} else {
		best_block = common.BlockChain.BlockIndex[hashstop.BIdx()]
	}

	if best_block == nil {
		common.CountSafe("GetHeadersBadBlock")
		best_block = common.BlockChain.BlockTreeRoot
	}

	var resp []byte
	var cnt uint32

	defer func() {
		// If we get a hash of an old orphaned blocks, FindPathTo() will panic, so...
		if r := recover(); r != nil {
			common.CountSafe("GetHeadersOrphBlk")
		}

		common.BlockChain.BlockIndexAccess.Unlock()

		// send the response
		out := new(bytes.Buffer)
		btc.WriteVlen(out, uint64(cnt))
		out.Write(resp)
		c.SendRawMsg("headers", out.Bytes(), false)
	}()

	for cnt < 2000 {
		if last_block.Height <= best_block.Height {
			break
		}
		best_block = best_block.FindPathTo(last_block)
		if best_block == nil {
			break
		}
		resp = append(resp, append(best_block.BlockHeader[:], 0)...) // 81st byte is always zero
		cnt++
	}

	// Note: the deferred function will be called before exiting
}

func (c *OneConnection) sendGetHeaders() {
	MutexRcv.Lock()
	lb := LastCommitedHeader
	MutexRcv.Unlock()
	min_height := int(lb.Height) - chain.MovingCheckopintDepth
	if min_height < 0 {
		min_height = 0
	}

	var cnt uint64
	var step int
	step = 1
	hashes := make([]byte, 0, 50*32)
	for cnt < 50 /*it should never get that far, but just in case...*/ {
		hashes = append(hashes, lb.BlockHash.Hash[:]...)
		cnt++
		//println(" geth", cnt, "height", lb.Height, lb.BlockHash.String())
		if int(lb.Height) <= min_height {
			break
		}
		for tmp := 0; tmp < step && lb != nil && int(lb.Height) > min_height; tmp++ {
			lb = lb.Parent
		}
		if lb == nil {
			break
		}
		if cnt >= 10 {
			step = step * 2
		}
	}

	bmsg := bytes.NewBuffer(make([]byte, 0, 4+9+len(hashes)+32))
	binary.Write(bmsg, binary.LittleEndian, common.Version)
	btc.WriteVlen(bmsg, cnt)
	bmsg.Write(hashes)
	bmsg.Write(make([]byte, 32)) // null_stop

	c.SendRawMsg("getheaders", bmsg.Bytes(), false)
	c.X.LastHeadersHeightAsk = lb.Height
	c.MutexSetBool(&c.X.GetHeadersInProgress, true)
	c.X.GetHeadersTimeOutAt = time.Now().Add(GetHeadersTimeout)
	c.X.GetHeadersSentAtPingCnt = c.X.PingSentCnt

	/*if c.X.Debug {
		println(c.ConnID, "- GetHeadersSentAtPingCnt", c.X.GetHeadersSentAtPingCnt)
	}*/
}
