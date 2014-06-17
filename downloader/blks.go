package main

import (
	"fmt"
	"sync"
	"time"
	"bytes"
	"sync/atomic"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	MEM_CACHE = 64<<20
	MAX_GET_FROM_PEER = 2e6

	MIN_BLOCKS_AHEAD = 10
	MAX_BLOCKS_AHEAD = 10e3

	BLOCK_TIMEOUT = 10*time.Second

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
	BlocksInProgress map[[32]byte] *one_bip
	BlocksCached map[uint32] *btc.Block
	BlocksCachedSize uint64
	BlocksMutex sync.Mutex
	BlocksComplete uint32

	LastBlockNotified uint32

	FetchBlocksTo uint32

	DlStartTime time.Time
	DlBytesProcessed, DlBytesDownloaded uint64

	BlockQueue chan *btc.Block
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
	cnt := 0
	for _, v := range BlocksInProgress {
		cnt++
		fmt.Println(cnt, v.Height, v.Count)
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
	blocksize_update(len(d))

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
		EmptyInProgressCnt++
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


func (c *one_net_conn) getnextblock() {
	var cnt int
	b := new(bytes.Buffer)
	vl := new(bytes.Buffer)

	avs := avg_block_size()
	blks_to_get := uint32(MEM_CACHE / avs)
	max_cnt_to_get := (MAX_GET_FROM_PEER / avs) + 1
	if max_cnt_to_get > MAX_BLOCKS_AT_ONCE {
		max_cnt_to_get = MAX_BLOCKS_AT_ONCE
	}

	BlocksMutex.Lock()

	FetchBlocksTo = BlocksComplete + blks_to_get
	if FetchBlocksTo > LastBlockHeight {
		FetchBlocksTo = LastBlockHeight
	}

	bl_stage := uint32(0)
	for curblk:=BlocksComplete; cnt<max_cnt_to_get; curblk++ {
		if curblk>FetchBlocksTo {
			if bl_stage==MAX_SAME_BLOCKS_AT_ONCE {
				break
			}
			bl_stage++
			curblk = BlocksComplete
		}
		if _, done := BlocksCached[curblk]; done {
			continue
		}

		bh, ok := BlocksToGet[curblk]
		if !ok {
			continue
		}

		cbip := BlocksInProgress[bh]
		if cbip==nil {
			// if not in progress then we always take it
			cbip = &one_bip{Height:curblk}
			cbip.Conns = make(map[uint32]bool, MaxNetworkConns)
		} else if cbip.Count!=bl_stage || cbip.Conns[c.id]{
			continue
		}
		if LastBlockAsked < curblk {
			LastBlockAsked = curblk
		}

		cbip.Count = bl_stage+1
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
		COUNTER("GDAT")
	} else {
		COUNTER("FULL")
		time.Sleep(250*time.Millisecond)
	}
	c.last_blk_rcvd = time.Now()
}


const BSLEN = 0x1000

var (
	BSMut sync.Mutex
	BSSum int
	BSCnt int
	BSIdx int
	BSLen [BSLEN]int
)


func blocksize_update(le int) {
	BSMut.Lock()
	BSLen[BSIdx] = le
	BSSum += le
	if BSCnt<BSLEN {
		BSCnt++
	}
	BSIdx = (BSIdx+1) % BSLEN
	BSSum -= BSLen[BSIdx]
	BSMut.Unlock()
}


func avg_block_size() (le int) {
	BSMut.Lock()
	if BSCnt>0 {
		le = BSSum/BSCnt
	} else {
		le = 220
	}
	BSMut.Unlock()
	return
}


func (c *one_net_conn) blk_idle() {
	c.Lock()
	doit := c.inprogress==0
	if !doit && !c.last_blk_rcvd.Add(BLOCK_TIMEOUT).After(time.Now()) {
		c.inprogress = 0
		doit = true
		COUNTER("TOUT")
	}
	c.Unlock()
	if doit {
		c.getnextblock()
	}
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
			COUNTER("IDLE")
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
		COUNTER("SLOW")
		minbps_rec.setbroken(true)
	}

	open_connection_mutex.Unlock()
}


func get_blocks() {
	var bl *btc.Block
	BlocksInProgress = make(map[[32]byte] *one_bip)
	BlocksCached = make(map[uint32] *btc.Block)

	DlStartTime = time.Now()
	BlocksComplete = TheBlockChain.BlockTreeEnd.Height
	CurrentBlockHeight := BlocksComplete+1
	BlockQueue = make(chan *btc.Block, LastBlockHeight-TheBlockChain.BlockTreeEnd.Height)

	TheBlockChain.DoNotSync = true

	tickSec := time.Tick(time.Second)
	tickDrop := time.Tick(DROP_PEER_EVERY_SEC*time.Second)
	tickStat := time.Tick(6*time.Second)

	for !GlobalExit && CurrentBlockHeight<=LastBlockHeight {
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
						bl.LastKnownHeight = BlocksComplete
						TheBlockChain.AcceptBlock(bl)
					}
				}
				atomic.AddUint64(&DlBytesProcessed, uint64(len(bl.Raw)))
				CurrentBlockHeight++

			case <-time.After(100*time.Millisecond):
				StallCount++
				TheBlockChain.Unspent.Idle()
		}
	}
}
