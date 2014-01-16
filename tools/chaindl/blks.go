package main

import (
	"fmt"
	"sync"
	"time"
	"bytes"
	"sync/atomic"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

const (
	MIN_BLOCKS_AHEAD = 5
	MAX_BLOCKS_AHEAD = 10e3

	MAX_BLOCKS_IM_MEM = 256<<20 // Use up to 256MB of memory for block cache
	BLOCK_TIMEOUT = 10*time.Second

	GETBLOCKS_BYTES_ONCE = 250e3
)


type one_bip struct {
	Height uint32
	Count uint32
	Conns map[uint32]bool
}

var (
	_DoBlocks bool
	BlocksToGet map[uint32][32]byte
	BlocksInProgress map[[32]byte] *one_bip
	BlocksCached map[uint32] *btc.Block
	BlocksCachedSize uint
	BlocksMutex sync.Mutex
	BlocksIndex uint32
	BlocksComplete uint32

	DlStartTime time.Time
	DlBytesProcesses, DlBytesDownloaded uint64
)


func GetDoBlocks() (res bool) {
	BlocksMutex_Lock()
	res = _DoBlocks
	BlocksMutex_Unlock()
	return
}

func SetDoBlocks(res bool) {
	BlocksMutex_Lock()
	_DoBlocks = res
	BlocksMutex_Unlock()
}


func show_pending() {
	BlocksMutex_Lock()
	defer BlocksMutex_Unlock()
	fmt.Println("bocks pending:")
	for k, v := range BlocksToGet {
		fmt.Println(k, hex.EncodeToString(v[:]))
	}
}


func show_inprogress() {
	BlocksMutex_Lock()
	defer BlocksMutex_Unlock()
	fmt.Println("bocks in progress:")
	cnt := 0
	for _, v := range BlocksInProgress {
		cnt++
		fmt.Println(cnt, v.Height, v.Count)
	}
}


func (c *one_net_conn) getnextblock() {
	var cnt, lensofar int
	b := new(bytes.Buffer)
	vl := new(bytes.Buffer)

	BlocksMutex_Lock()

	if BlocksComplete > BlocksIndex {
		fmt.Println("dupa", BlocksComplete, BlocksIndex)
		BlocksIndex = BlocksComplete
	}

	blocks_from := BlocksIndex

	avg_len := avg_block_size()
	max_block_forward := uint32((MAX_BLOCKS_IM_MEM-BlocksCachedSize) / uint(avg_len))
	if max_block_forward < MIN_BLOCKS_AHEAD {
		max_block_forward = MIN_BLOCKS_AHEAD
	} else if max_block_forward > MAX_BLOCKS_AHEAD {
		max_block_forward = MAX_BLOCKS_AHEAD
	}

	for secondloop:=false; cnt<10e3 && lensofar<GETBLOCKS_BYTES_ONCE; secondloop = true {
		if secondloop && BlocksIndex==blocks_from {
			if BlocksComplete == LastBlockHeight {
				_DoBlocks = false
			} else {
				COUNTER("WRAP")
				time.Sleep(1e8)
			}
			break
		}

		BlocksIndex++
		if BlocksIndex > BlocksComplete+max_block_forward || BlocksIndex > LastBlockHeight {
			//fmt.Println("wrap", BlocksIndex, BlocksComplete)
			BlocksIndex = BlocksComplete
		}

		if _, done := BlocksCached[BlocksIndex]; done {
			//fmt.Println(" cached ->", BlocksIndex)
			continue
		}

		bh, ok := BlocksToGet[BlocksIndex]
		if !ok {
			//fmt.Println(" toget ->", BlocksIndex)
			continue
		}

		cbip := BlocksInProgress[bh]
		if cbip==nil {
			cbip = &one_bip{Height:BlocksIndex, Count:1}
			cbip.Conns = make(map[uint32]bool, MAX_CONNECTIONS)
		} else {
			if cbip.Conns[c.id] {
				//fmt.Println(" cbip.Conns ->", c.id)
				continue
			}
			cbip.Count++
		}
		cbip.Conns[c.id] = true
		c.inprogress++
		BlocksInProgress[bh] = cbip

		b.Write([]byte{2,0,0,0})
		b.Write(bh[:])
		cnt++
		lensofar += avg_len
	}
	BlocksMutex_Unlock()

	btc.WriteVlen(vl, uint32(cnt))

	c.sendmsg("getdata", append(vl.Bytes(), b.Bytes()...))
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


func (c *one_net_conn) block(d []byte) {
	BlocksMutex_Lock()
	defer BlocksMutex_Unlock()
	h := btc.NewSha2Hash(d[:80])

	c.Lock()
	c.last_blk_rcvd = time.Now()
	c.Unlock()

	bip := BlocksInProgress[h.Hash]
	if bip==nil || !bip.Conns[c.id] {
		COUNTER("UNEX")
		//fmt.Println(h.String(), "- already received", bip)
		return
	}

	delete(bip.Conns, c.id)
	c.Lock()
	c.inprogress--
	c.Unlock()
	atomic.AddUint64(&DlBytesDownloaded, uint64(len(d)))
	blocksize_update(len(d))

	bl, er := btc.NewBlock(d)
	if er != nil {
		fmt.Println(c.peerip, "-", er.Error())
		c.setbroken(true)
		return
	}

	BlocksCachedSize += uint(len(d))
	BlocksCached[bip.Height] = bl
	delete(BlocksToGet, bip.Height)
	delete(BlocksInProgress, h.Hash)

	bl.BuildTxList()
	if !bytes.Equal(btc.GetMerkel(bl.Txs), bl.MerkleRoot) {
		fmt.Println(c.peerip, " - MerkleRoot mismatch at block", bip.Height)
		c.setbroken(true)
		return
	}

	//fmt.Println("  got block", height)
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
	if open_connection_count() < MAX_CONNECTIONS {
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
			//fmt.Println(" -", v.peerip, "- idle")
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
		//fmt.Printf(" - %s - slowest (%.3f KBps, %d KB)\n", minbps_rec.peerip, min_bps/1e3, minbps_rec.bytes_received>>10)
		COUNTER("SLOW")
		minbps_rec.setbroken(true)
	}

	open_connection_mutex.Unlock()
}


func get_blocks() {
	var hadblock bool
	var bl *btc.Block
	BlocksInProgress = make(map[[32]byte] *one_bip)
	BlocksCached = make(map[uint32] *btc.Block)

	//fmt.Println("opening connections")
	DlStartTime = time.Now()
	BlocksComplete = TheBlockChain.BlockTreeEnd.Height
	BlocksIndex = BlocksComplete

	SetDoBlocks(true)
	ct := time.Now().Unix()
	lastdrop := ct
	laststat := ct
	TheBlockChain.DoNotSync = true
	for GetDoBlocks() {
		BlocksMutex_Lock()
		if BlocksComplete>=LastBlockHeight {
			BlocksMutex_Unlock()
			break
		}

		in := time.Now()
		for {
			bl, hadblock = BlocksCached[BlocksComplete+1]
			if !hadblock {
				break
			}
			BlocksComplete++
			if BlocksComplete > BlocksIndex {
				BlocksIndex = BlocksComplete
			}
			BlocksCachedSize -= uint(len(bl.Raw))
			delete(BlocksCached, BlocksComplete)
			if true {
				bl.Trusted = BlocksComplete<=TrustUpTo
				er, _, _ := TheBlockChain.CheckBlock(bl)
				if er != nil {
					fmt.Println(er.Error())
					return
				} else {
					TheBlockChain.AcceptBlock(bl)
				}
			} else {
				TheBlockChain.Blocks.BlockAdd(BlocksComplete, bl)
			}
			atomic.AddUint64(&DlBytesProcesses, uint64(len(bl.Raw)))
			if in.Add(time.Second).After(time.Now()) {
				break // reschedule once a second
			}
		}
		inpr := len(BlocksInProgress)
		BlocksMutex_Unlock()

		if !hadblock && inpr>10 {
			TheBlockChain.Unspent.Idle()
			COUNTER("IDLE")
		}

		time.Sleep(1e8)

		ct = time.Now().Unix()

		if open_connection_count() > MAX_CONNECTIONS {
			drop_slowest_peers()
		} else {
			// drop slowest peers once for awhile
			occ := MAX_CONNECTIONS
			if occ > 0 {
				occ = 1200 / occ // For 20 open connections: drop one per minute
				if occ < 3 {
					occ = 3 // .. drop not more often then once sper 3 seconds
				}
				if ct - lastdrop > int64(occ) {
					lastdrop = ct
					drop_slowest_peers()
				}
			}
		}

		add_new_connections()

		if ct - laststat >= 5 {
			laststat = ct
			print_stats()
			usif_prompt()
		}
	}
	fmt.Println("all blocks done...")
}
