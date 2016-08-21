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
	DROP_PEER_EVERY_SEC = 10
	MAX_BLOCKS_AT_ONCE = 400
	MAX_SAME_BLOCKS_AT_ONCE = 1
	AVERAGE_BLOCK_SIZE_BLOCKS_BACK = 144 /*one day*/
	MIN_BLOCK_SIZE = 1000
	MAX_FORWARD_DATA = 1e9 // 1GB
)


type one_bip struct {
	Count uint32
	Conns map[uint32]bool
	*btc.Block
}

var (
	BlocksToGet map[uint32][32]byte
	BlocksInProgress map[[32]byte] *one_bip = make(map[[32]byte] *one_bip)
	BlocksCached map[uint32] *btc.Block = make(map[uint32] *btc.Block)
	BlocksCachedSize uint64
	BlocksMutex sync.Mutex
	BlocksComplete uint32

	LastBlockNotified uint32
	LastStoredBlock uint32

	DlStartTime time.Time
	DlBytesProcessed, DlBytesDownloaded uint64

	BlockQueue chan *btc.Block = make(chan *btc.Block, 100e3)
	BlocksQueuedSize uint64

	BlStructCache map[[32]byte] *btc.Block = make(map[[32]byte] *btc.Block)
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


func submit_block(bl *btc.Block) {
	BlocksComplete++
	bl.Trusted = bl.Height <= TrustUpTo
	update_bslen_history(len(bl.Raw))
	atomic.AddUint64(&BlocksQueuedSize, uint64(len(bl.Raw)))
	BlockQueue <- bl
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

	delete(BlocksInProgress, h.Hash)
	if len(BlocksInProgress)==0 {
		COUNTER("EMPT")
	}

	bl := bip.Block
	if er:=bl.UpdateContent(d); er!=nil {
		fmt.Println(c.Ip(), "-", er.Error())
		c.setbroken(true)
		return
	}

	if OnlyStoreBlocks {
		// only check if the payload matches MerkleRoot from the header
		merkel, mutated := bl.ComputeMerkel()
		if mutated {
			fmt.Println(c.Ip(), " - MerkleRoot mutated at block", bip.Height)
			c.setbroken(true)
			return
		}

		if !bytes.Equal(merkel, bl.MerkleRoot()) {
			fmt.Println(c.Ip(), " - MerkleRoot mismatch at block", bip.Height)
			c.setbroken(true)
			return
		}
	}

	delete(BlocksToGet, bip.Height)

	//println("got-", bip.Height, BlocksComplete+1)
	if BlocksComplete+1==bip.Height {
		submit_block(bl)
		for {
			if bn:=BlocksCached[BlocksComplete+1]; bn!=nil {
				delete(BlocksCached, BlocksComplete+1)
				BlocksCachedSize -= uint64(len(bn.Raw))
				submit_block(bn)
			} else {
				break
			}
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

	//bl_stage := uint32(0)  - TODO
	maxbl := LastBlockHeight
	n := atomic.LoadUint32(&LastStoredBlock) + uint32(MAX_FORWARD_DATA/uint(atomic.LoadUint32(&BSAvg))) + 1
	if maxbl > n {
		maxbl = n
	}
	for curblk:=BlocksComplete; cnt<MAX_BLOCKS_AT_ONCE && curblk<=maxbl; curblk++ {
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
		cbip = new(one_bip)
		cbip.Count++
		cbip.Conns = make(map[uint32]bool, MaxNetworkConns)
		cbip.Conns[c.id] = true
		cbip.Block = BlStructCache[bh]
		delete(BlStructCache, bh)
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


var (
	BSAvg uint32 = MIN_BLOCK_SIZE
	bslen_history [AVERAGE_BLOCK_SIZE_BLOCKS_BACK]int
	bslen_total int
	bslen_count int
	bslen_history_index int
)


func update_bslen_history(le int) {
	if bslen_count==AVERAGE_BLOCK_SIZE_BLOCKS_BACK {
		bslen_total -= bslen_history[bslen_history_index]
	} else {
		bslen_count++
	}
	bslen_history[bslen_history_index] = le
	bslen_total += le
	bslen_history_index++
	if bslen_history_index==AVERAGE_BLOCK_SIZE_BLOCKS_BACK {
		bslen_history_index = 0
	}
	newval := uint32(bslen_total/bslen_count)
	if newval < MIN_BLOCK_SIZE {
		newval = MIN_BLOCK_SIZE
	}
	atomic.StoreUint32(&BSAvg, newval)
}

func calc_new_block_size() {
	cnt := 0
	for n:=TheBlockChain.BlockTreeEnd; n!=nil && cnt<AVERAGE_BLOCK_SIZE_BLOCKS_BACK; n=n.Parent {
		update_bslen_history(int(n.BlockSize))
		cnt++
	}
}


func avg_block_size() (le int) {
	le = int(atomic.LoadUint32(&BSAvg))
	if le < MIN_BLOCK_SIZE {
		le = MIN_BLOCK_SIZE
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
	BlocksMutex.Unlock()

	TheBlockChain.DoNotSync = true

	tickSec := time.Tick(time.Second)
	tickDrop := time.Tick(DROP_PEER_EVERY_SEC*time.Second)
	tickStat := time.Tick(6*time.Second)

	for !GlobalExit() && atomic.LoadUint32(&LastStoredBlock) < LastBlockHeight {
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
				if OnlyStoreBlocks {
					TheBlockChain.Blocks.BlockAdd(bl.Height, bl)
				} else {
					TheBlockChain.BlockIndexAccess.Lock()
					cur := TheBlockChain.BlockIndex[bl.Hash.BIdx()]
					TheBlockChain.BlockIndexAccess.Unlock()
					if cur==nil {
						fmt.Println("Block", bl.Hash.String(), "unknown")
						Exit()
						break
					}
					er := TheBlockChain.PostCheckBlock(bl)
					if er != nil {
						fmt.Println("CheckBlock", bl.Height, bl.Hash.String(), er.Error())
						Exit()
						break
					}
					bl.LastKnownHeight = TheBlockChain.BlockTreeEnd.Height
					er = TheBlockChain.CommitBlock(bl, cur)
					if er != nil {
						fmt.Println("CommitBlock:", er.Error())
						Exit()
						break
					}
				}
				atomic.StoreUint32(&LastStoredBlock, bl.Height)
				atomic.AddUint64(&DlBytesProcessed, uint64(len(bl.Raw)))
				atomic.AddUint64(&BlocksQueuedSize, uint64(-len(bl.Raw)))

			case <-time.After(1000*time.Millisecond):
				COUNTER("IDLE")
				TheBlockChain.Unspent.Idle()
		}
	}
	TheBlockChain.Sync()
}
