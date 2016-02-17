package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
	"bytes"
	"sync/atomic"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	MAX_GET_FROM_PEER = 2e6 // two full size (1MB) blocks

	MIN_BLOCKS_AHEAD = 10
	MAX_BLOCKS_AHEAD = 10e3

	BLOCKSIZE_AVERAGE_DAYS = 7

	DROP_PEER_EVERY_SEC = 10

	MAX_BLOCKS_AT_ONCE = 1000

	MAX_SAME_BLOCKS_AT_ONCE = 1
)


type one_bip struct {
	Height uint32
	Count uint32
	Conns map[uint32]bool
}

var (
	BlocksToGet map[uint32][32]byte
	BlocksInProgress map[[32]byte] *one_bip = make(map[[32]byte] *one_bip, 300e3)
	BlocksCached map[uint32] *btc.Block = make(map[uint32] *btc.Block, 300e3)
	BlocksCachedSize uint64
	BlocksMutex sync.Mutex
	BlocksComplete uint32

	LastBlockNotified uint32
	LastStoredBlock uint32

	DlStartTime time.Time
	DlBytesProcessed, DlBytesDownloaded uint64

	BlockQueue chan *btc.Block = make(chan *btc.Block, 100e3)
)


func show_pending() {
	BlocksMutex.Lock()
	defer BlocksMutex.Unlock()
	fmt.Println("bocks pending:")
	for k, v := range BlocksToGet {
		fmt.Println(k, hex.EncodeToString(v[:]))
	}
}


func show_inprogress() {
	BlocksMutex.Lock()
	defer BlocksMutex.Unlock()
	fmt.Println("bocks in progress:")

	heights := make([]int, len(BlocksInProgress))
	cnt := 0
	for _, v := range BlocksInProgress {
		heights[cnt] = int(v.Height)
		cnt++
	}
	sort.Ints(heights)

	for cnt = range heights {
		fmt.Print(cnt, ") ", heights[cnt])
		for _, v := range BlocksInProgress {
			if int(v.Height)==heights[cnt] {
				fmt.Print(" in progress by:")
				for cid, _ := range v.Conns {
					fmt.Print(" ", cid)
				}
			}
		}
		fmt.Println()
	}
}


func show_cached() {
	BlocksMutex.Lock()
	defer BlocksMutex.Unlock()
	fmt.Println("bocks in memory:")

	heights := make([]int, len(BlocksCached))
	cnt := 0
	for he, _ := range BlocksCached {
		heights[cnt] = int(he)
		cnt++
	}
	sort.Ints(heights)

	for cnt = range heights {
		fmt.Println(cnt, ")", heights[cnt], "  len:", len(BlocksCached[uint32(heights[cnt])].Raw))
	}
}


func (c *one_net_conn) block(d []byte) {
	atomic.AddUint64(&DlBytesDownloaded, uint64(len(d)))

	BlocksMutex.Lock()
	defer BlocksMutex.Unlock()
	h := btc.NewSha2Hash(d[:80])

	c.Lock()
	c.last_blk_rcvd = time.Now()
	c.Unlock()

	bip := BlocksInProgress[h.Hash]
	if bip==nil || !bip.Conns[c.id] {
		COUNTER("BNOT")
		//fmt.Println(h.String(), "- already received", bip)
		return
	}
	//fmt.Println(h.String(), "- new", bip.Height)
	COUNTER("BYES")

	delete(bip.Conns, c.id)
	c.Lock()
	c.inprogress--
	c.Unlock()

	bl, er := btc.NewBlock(d)
	if er != nil {
		fmt.Println(c.Ip(), "-", er.Error())
		c.setbroken(true)
		return
	}

	bl.BuildTxList()
	if !bytes.Equal(btc.GetMerkel(bl.Txs), bl.MerkleRoot()) {
		fmt.Println(c.Ip(), " - MerkleRoot mismatch at block", bip.Height)
		c.setbroken(true)
		return
	}

	delete(BlocksToGet, bip.Height)
	delete(BlocksInProgress, h.Hash)
	if len(BlocksInProgress)==0 {
		println("EmptyInProgress")
	}

	//println("got-", bip.Height, BlocksComplete+1)
	if BlocksComplete+1==bip.Height {
		BlocksComplete++
		BlockQueue <- bl
		for {
			bl = BlocksCached[BlocksComplete+1]
			if bl == nil {
				break
			}
			BlocksComplete++
			delete(BlocksCached, BlocksComplete)
			BlocksCachedSize -= uint64(len(bl.Raw))
			BlockQueue <- bl
		}
	} else {
		BlocksCached[bip.Height] = bl
		BlocksCachedSize += uint64(len(d))
	}
}



// returns true if block has been added to the queue
func (c *one_net_conn) get_more_blocks() {
	var cnt int
	b := new(bytes.Buffer)
	vl := new(bytes.Buffer)

	BlocksMutex.Lock()

	//bl_stage := uint32(0)
	for curblk:=BlocksComplete; curblk<=LastBlockHeight; curblk++ {
		if (c.inprogress+1) * avg_block_size() > MAX_GET_FROM_PEER {
			break
		}

		if _, done := BlocksCached[curblk]; done {
			continue
		}

		bh, ok := BlocksToGet[curblk]
		if !ok {
			continue
		}

		cbip := BlocksInProgress[bh]
		if cbip!=nil {
			continue
		}

		// if not in progress then we always take it
		cbip = &one_bip{Height:curblk}
		cbip.Count++
		cbip.Conns = make(map[uint32]bool, MaxNetworkConns)
		cbip.Conns[c.id] = true
		c.inprogress++
		BlocksInProgress[bh] = cbip

		b.Write([]byte{2,0,0,0})
		b.Write(bh[:])
		cnt++
	}
	BlocksMutex.Unlock()

	if cnt > 0 {
		btc.WriteVlen(vl, uint64(cnt))
		c.sendmsg("getdata", append(vl.Bytes(), b.Bytes()...))
		COUNTER("GDYE")
	} else {
		COUNTER("GDNO")
		time.Sleep(100*time.Millisecond)
	}
	c.Lock()
	c.last_blk_rcvd = time.Now()
	c.Unlock()
}


const BSLEN = 0x1000

var (
	BSAvg uint32
)


func calc_new_block_size() {
	var cnt, tlen int
	for n:=TheBlockChain.BlockTreeEnd; n!=nil && cnt<BLOCKSIZE_AVERAGE_DAYS*24*6; n=n.Parent {
		tlen += int(n.BlockSize)
		cnt++
	}
	atomic.StoreUint32(&BSAvg, uint32(tlen/cnt))
}


func avg_block_size() (le int) {
	le = int(atomic.LoadUint32(&BSAvg))
	if le < 1024 {
		le = 1024
	}
	return
}


func drop_slowest_peers() {
	if open_connection_count() < MaxNetworkConns {
		return
	}
	open_connection_mutex.Lock()

	var min_bps float64
	var minbps_rec *one_net_conn
	for _, v := range open_connection_list {
		if v.isbroken() {
			// alerady broken
			continue
		}

		if !v.isconnected() {
			// still connecting
			continue
		}

		if time.Now().Sub(v.connected_at) < 3*time.Second {
			// give him 3 seconds
			continue
		}

		v.Lock()

		if v.bytes_received==0 {
			v.Unlock()
			// if zero bytes received after 3 seconds - drop it!
			v.setbroken(true)
			//fmt.Println(" -", v.Ip(), "- idle")
			COUNTER("CNOD")
			continue
		}

		bps := v.bps()
		v.Unlock()

		if minbps_rec==nil || bps<min_bps {
			minbps_rec = v
			min_bps = bps
		}
	}
	if minbps_rec!=nil {
		//fmt.Printf(" - %s - slowest (%.3f KBps, %d KB)\n", minbps_rec.Ip(), min_bps/1e3, minbps_rec.bytes_received>>10)
		COUNTER("CSLO")
		minbps_rec.setbroken(true)
	}

	open_connection_mutex.Unlock()
}


func get_blocks() {
	var bl *btc.Block

	DlStartTime = time.Now()
	BlocksMutex.Lock()
	BlocksComplete = TheBlockChain.BlockTreeEnd.Height
	CurrentBlockHeight := BlocksComplete+1
	BlocksMutex.Unlock()

	TheBlockChain.DoNotSync = true

	tickSec := time.Tick(time.Second)
	tickDrop := time.Tick(DROP_PEER_EVERY_SEC*time.Second)
	tickStat := time.Tick(6*time.Second)

	for !GlobalExit() && CurrentBlockHeight<=LastBlockHeight {
		select {
			case <-tickSec:
				cc := open_connection_count()
				if cc > MaxNetworkConns {
					drop_slowest_peers()
				} else if cc < MaxNetworkConns {
					add_new_connections()
				}

			case <-tickStat:
				print_stats()
				usif_prompt()

			case <-tickDrop:
				if open_connection_count() >= MaxNetworkConns {
					drop_slowest_peers()
				}

			case bl = <-BlockQueue:
				bl.Trusted = CurrentBlockHeight <= TrustUpTo
				if OnlyStoreBlocks {
					TheBlockChain.Blocks.BlockAdd(CurrentBlockHeight, bl)
				} else {
					er, _, _ := TheBlockChain.CheckBlock(bl)
					if er != nil {
						fmt.Println("CheckBlock:", er.Error())
						return
					} else {
						bl.LastKnownHeight = CurrentBlockHeight + uint32(len(BlockQueue))
						TheBlockChain.AcceptBlock(bl)
					}
				}
				atomic.StoreUint32(&LastStoredBlock, CurrentBlockHeight)
				atomic.AddUint64(&DlBytesProcessed, uint64(len(bl.Raw)))
				CurrentBlockHeight++

			case <-time.After(1000*time.Millisecond):
				COUNTER("IDLE")
				TheBlockChain.Unspent.Idle()
		}
	}
	TheBlockChain.Sync()
}
