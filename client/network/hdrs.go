package network

import (
	"fmt"
	//"time"
	"bytes"
	//"sync/atomic"
	//"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)



func (c *OneConnection) HandleHeaders(pl []byte) {

	c.X.GetHeadersInProgress = false

	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("HandleHeaders:", e.Error(), c.PeerAddr.Ip())
		return
	}

	var new_headers_got int

	if cnt>0 {
		for i:=0; i<int(cnt); i++ {
			var hdr [81]byte

			n, _ := b.Read(hdr[:])
			if n!=81 {
				println("HandleHeaders: pl too short", c.PeerAddr.Ip())
				c.DoS("HdrErr1")
				return
			}

			if hdr[80]!=0 {
				fmt.Println("Unexpected value of txn_count from", c.PeerAddr.Ip())
				c.DoS("HdrErr2")
				return
			}

			bh := btc.NewSha2Hash(hdr[:80])
			MutexRcv.Lock()
			if _, ok := ReceivedBlocks[bh.BIdx()]; !ok {
				if b2g, ok := BlocksToGet[bh.BIdx()]; !ok {
					common.CountSafe("HeaderNew")
					new_headers_got++
					//fmt.Println("", i, bh.String(), " - NEW!")

					bl, er := btc.NewBlock(hdr[:])
					if er == nil {
						common.BlockChain.BlockIndexAccess.Lock()
						er, dos, _ := common.BlockChain.PreCheckBlock(bl)
						if er == nil {
							c.X.GetBlocksDataNow = true
							node := common.BlockChain.AcceptHeader(bl)
							LastCommitedHeader = node
							//println("checked ok - height", node.Height)
							if node.Height > c.Node.Height {
								c.Node.Height = node.Height
								//println(c.PeerAddr.Ip(), c.Node.Version, "- new block", bh.String(), "@", node.Height)
							}
							BlocksToGet[bh.BIdx()] = &OneBlockToGet{Block:bl, BlockTreeNode:node, InProgress:0}
						} else {
							common.CountSafe("HeaderCheckFail")
							if dos {
								c.DoS("BadHeader")
							} else {
								c.Misbehave("BadHeader", 50) // do it 20 times and you are banned
							}
						}
						common.BlockChain.BlockIndexAccess.Unlock()
						if dos {
							c.DoS("HdrErr3")
						}
					}
				} else {
					common.CountSafe("HeaderFresh")
					//fmt.Println(c.PeerAddr.Ip(), "block", bh.String(), " not new but get it")
					if c.Node.Height < b2g.Block.Height {
						c.Node.Height = b2g.Block.Height
					}
					c.X.GetBlocksDataNow = true
				}
			} else {
				common.CountSafe("HeaderOld")
				//fmt.Println("", i, bh.String(), "-already received")
			}
			MutexRcv.Unlock()
		}
	}

	if new_headers_got==0 {
		c.X.AllHeadersReceived = true
	}
}


// Handle getheaders protocol command
// https://en.bitcoin.it/wiki/Protocol_specification#getheaders
func (c *OneConnection) GetHeaders(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)
	if e != nil || hashstop==nil {
		println("GetHeaders: error parsing payload from", c.PeerAddr.Ip())
		c.DoS("BadGetHdrs")
		return
	}

	if common.DebugLevel > 1 {
		println("GetHeaders", len(h2get), hashstop.String())
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
				if best_block==nil || bl.Height > best_block.Height {
					//println(" ... bbl", i, bl.Height, bl.BlockHash.String())
					best_block = bl
				}
			}
		}
	} else {
		best_block = common.BlockChain.BlockIndex[hashstop.BIdx()]
	}

	if best_block==nil {
		println("GetHeaders: best_block not found")
		common.BlockChain.BlockIndexAccess.Unlock()
		common.CountSafe("GetHeadersBadBlock")
		return
	}

	best_bl_ch := len(best_block.Childs)
	//last_block = common.BlockChain.BlockTreeEnd

	var resp []byte
	var cnt uint32

	defer func() {
		// If we get a hash of an old orphaned blocks, FindPathTo() will panic, so...
		if r := recover(); r != nil {
			common.CountSafe("GetHeadersOrphBlk")
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			// This happens the you receive request for headers from an orphaned block
			fmt.Println("GetHeaders panic recovered:", err.Error())
			fmt.Println("Cnt:", cnt, "  len(h2get):", len(h2get))
			if best_block!=nil {
				fmt.Println("BestBlock:", best_block.Height, best_block.BlockHash.String(),
					len(best_block.Childs), best_bl_ch)
			}
			if last_block!=nil {
				fmt.Println("LastBlock:", last_block.Height, last_block.BlockHash.String(), len(last_block.Childs))
			}
		}

		common.BlockChain.BlockIndexAccess.Unlock()

		// send the response
		out := new(bytes.Buffer)
		btc.WriteVlen(out, uint64(cnt))
		out.Write(resp)
		c.SendRawMsg("headers", out.Bytes())
	}()

	for cnt<100 {
		if last_block.Height <= best_block.Height {
			break
		}
		best_block = best_block.FindPathTo(last_block)
		if best_block==nil {
			//println("FindPathTo failed", last_block.BlockHash.String(), cnt)
			//println("resp:", hex.EncodeToString(resp))
			break
		}
		resp = append(resp, append(best_block.BlockHeader[:], 0)...) // 81st byte is always zero
		cnt++
	}

	// Note: the deferred function will be called before exiting

	return
}

func (c *OneConnection) sendGetHeaders() {
	MutexRcv.Lock()
	lb := LastCommitedHeader
	MutexRcv.Unlock()
	min_height := int(lb.Height) - chain.MovingCheckopintDepth
	if min_height<0 {
		min_height = 0
	}

	blks := new(bytes.Buffer)
	var cnt uint64
	var step int
	step = 1
	for cnt<50/*it shoudl never get that far, but just in case...*/ {
		blks.Write(lb.BlockHash.Hash[:])
		cnt++
		//println(" geth", cnt, "height", lb.Height, lb.BlockHash.String())
		if int(lb.Height) <= min_height {
			break
		}
		for tmp:=0; tmp<step && lb!=nil && int(lb.Height)>min_height; tmp++ {
			lb = lb.Parent
		}
		if lb==nil {
			break
		}
		if cnt>=10 {
			step = step*2
		}
	}
	var null_stop [32]byte
	blks.Write(null_stop[:])

	bhdr := new(bytes.Buffer)
	binary.Write(bhdr, binary.LittleEndian, common.Version)
	btc.WriteVlen(bhdr, cnt)

	c.SendRawMsg("getheaders", append(bhdr.Bytes(), blks.Bytes()...))
	c.X.GetHeadersInProgress = true
}
