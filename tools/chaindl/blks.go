package main

import (
	"sync"
	"time"
	"bytes"
	"sync/atomic"
	"github.com/piotrnar/gocoin/btc"
)

const (
	MAX_BLOCKS_FORWARD = 10e3
	BLOCK_TIMEOUT = 2*time.Second

	GETBLOCKS_BYTES_ONCE = 250e3
)


type one_bip struct {
	Height uint32
	Count uint32
}

var (
	_DoBlocks bool
	BlocksToGet map[uint32][32]byte
	BlocksInProgress map[[32]byte] *one_bip
	BlocksCached map[uint32] *btc.Block
	BlocksMutex sync.Mutex
	BlocksIndex uint32
	BlocksComplete uint32

	DlStartTime time.Time
	DlBytesProcesses, DlBytesDownloaded uint64
)

func GetDoBlocks() (res bool) {
	BlocksMutex.Lock()
	res = _DoBlocks
	BlocksMutex.Unlock()
	return
}

func SetDoBlocks(res bool) {
	BlocksMutex.Lock()
	_DoBlocks = res
	BlocksMutex.Unlock()
}


func show_inprogress() {
	BlocksMutex.Lock()
	defer BlocksMutex.Unlock()
	println("bocks in progress:")
	cnt := 0
	for _, v := range BlocksInProgress {
		cnt++
		println(cnt, v.Height, v.Count)
	}
}


func (c *one_net_conn) getnextblock() {
	var cnt, lensofar int
	b := new(bytes.Buffer)
	b.WriteByte(0)
	BlocksMutex.Lock()

	blocks_from := BlocksIndex

	avg_len := avg_block_size()
	for secondloop:=false; cnt<250 && lensofar<GETBLOCKS_BYTES_ONCE; secondloop=true {
		if secondloop && BlocksIndex==blocks_from {
			if BlocksComplete == LastBlockHeight {
				SetDoBlocks(false)
				println("all blocks done")
			} else {
				//println("BlocksIndex", BlocksIndex, blocks_from, BlocksComplete)
				COUNTER("WRAP")
				time.Sleep(1e8)
			}
			break
		}


		BlocksIndex++
		if BlocksIndex > BlocksComplete+MAX_BLOCKS_FORWARD || BlocksIndex > LastBlockHeight {
			BlocksIndex = BlocksComplete
		}

		if _, done := BlocksCached[BlocksIndex]; done {
			//println(" cached ->", BlocksIndex)
			continue
		}

		bh, ok := BlocksToGet[BlocksIndex]
		if !ok {
			continue
		}

		c.Mutex.Lock()
		if c.blockinprogress[bh] {
			c.Mutex.Unlock()
			continue
		}

		cbip := BlocksInProgress[bh]
		if cbip==nil {
			cbip = &one_bip{Height:BlocksIndex, Count:1}
		} else {
			cbip.Count++
		}
		BlocksInProgress[bh] = cbip
		//dmppr()
		c.blockinprogress[bh] = true
		c.Mutex.Unlock()

		b.Write([]byte{2,0,0,0})
		b.Write(bh[:])
		cnt++
		lensofar += avg_len
	}
	BlocksMutex.Unlock()

	pl := b.Bytes()
	pl[0] = byte(cnt)
	//println("getdata", hex.EncodeToString(pl))
	c.sendmsg("getdata", pl)
	c.last_blk_rcvd = time.Now()
}


var (
	BSMut sync.Mutex
	BSSum int
	BSCnt int
	BSIdx int
	BSLen [0x100]int
)


func blocksize_update(le int) {
	BSMut.Lock()
	BSLen[BSIdx] = le
	BSSum += le
	if BSCnt<0x100 {
		BSCnt++
	}
	BSIdx = (BSIdx+1) & 0xff
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
	BlocksMutex.Lock()
	defer BlocksMutex.Unlock()
	h := btc.NewSha2Hash(d[:80])

	c.Mutex.Lock()
	if !c.blockinprogress[h.Hash] {
		c.Mutex.Unlock()
		//println(c.peerip, "- unexpected block", h.String())
		COUNTER("UNEX")
		return
	}

	blocksize_update(len(d))

	//println(c.peerip, " - block expected", h.String(), len(c.blockinprogress))
	delete(c.blockinprogress, h.Hash)
	c.last_blk_rcvd = time.Now()
	c.Mutex.Unlock()

	bl, er := btc.NewBlock(d)
	if er != nil {
		println(c.peerip, "-", er.Error())
		c.setbroken(true)
		return
	}

	bip := BlocksInProgress[bl.Hash.Hash]
	if bip==nil {
		COUNTER("SAME")
		//println(bl.Hash.String(), "- already received")
		return
	}
	atomic.AddUint64(&DlBytesDownloaded, uint64(len(bl.Raw)))
	BlocksCached[bip.Height] = bl
	delete(BlocksToGet, bip.Height)
	delete(BlocksInProgress, bl.Hash.Hash)

	bl.BuildTxList()
	if !bytes.Equal(btc.GetMerkel(bl.Txs), bl.MerkleRoot) {
		println(c.peerip, " - MerkleRoot mismatch at block", bip.Height)
		c.setbroken(true)
		return
	}

	//println("  got block", height)
}


func (c *one_net_conn) blk_idle() {
	c.Lock()
	cc := len(c.blockinprogress)
	c.Unlock()
	if cc==0 {
		c.getnextblock()
	} else {
		if !c.last_blk_rcvd.Add(BLOCK_TIMEOUT).After(time.Now()) {
			COUNTER("TOUT")
			c.setbroken(true)
		}
	}
}


func drop_slowest_peers() {
atomic.StoreUint32(&iii, 1001)
	if open_connection_count() < MAX_CONNECTIONS {
atomic.StoreUint32(&iii, 1002)
		return
	}
atomic.StoreUint32(&iii, 1003)
	open_connection_mutex.Lock()

atomic.StoreUint32(&iii, 1004)
	var min_bps float64
	var minbps_rec *one_net_conn
	for _, v := range open_connection_list {
atomic.StoreUint32(&iii, 1005)
		if v.isbroken() {
			// alerady broken
			continue
		}
atomic.StoreUint32(&iii, 1006)

		if !v.isconnected() {
			// still connecting
			continue
		}

atomic.StoreUint32(&iii, 1007)
		if time.Now().Sub(v.connected_at) < 3*time.Second {
			// give him 3 seconds
			continue
		}

atomic.StoreUint32(&iii, 1008)
		v.Lock()
atomic.StoreUint32(&iii, 1009)
		br := v.bytes_received
		v.Unlock()

		if br==0 {
atomic.StoreUint32(&iii, 1010)
			// if zero bytes received after 3 seconds - drop it!
			v.setbroken(true)
atomic.StoreUint32(&iii, 1011)
			//println(" -", v.peerip, "- idle")
			COUNTER("IDLE")
			continue
		}

atomic.StoreUint32(&iii, 1012)
		bps := v.bps()
atomic.StoreUint32(&iii, 1013)
		if minbps_rec==nil || bps<min_bps {
			minbps_rec = v
			min_bps = bps
		}
	}
	if minbps_rec!=nil {
		//fmt.Printf(" - %s - slowest (%.3f KBps, %d KB)\n", minbps_rec.peerip, min_bps/1e3, minbps_rec.bytes_received>>10)
atomic.StoreUint32(&iii, 1014)
		COUNTER("SLOW")
		minbps_rec.setbroken(true)
	}

atomic.StoreUint32(&iii, 1015)
	open_connection_mutex.Unlock()
}


func get_blocks() {
	BlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)
	if btc.AbortNow || BlockChain==nil {
		return
	}

	BlocksInProgress = make(map[[32]byte] *one_bip, MAX_BLOCKS_FORWARD)
	BlocksCached = make(map[uint32] *btc.Block, len(BlocksToGet))

	//println("opening connections")
	DlStartTime = time.Now()

	SetDoBlocks(true)
	lastdrop := time.Now().Unix()
	for GetDoBlocks() {
		ct := time.Now().Unix()

atomic.StoreUint32(&iii, 1)
		BlocksMutex.Lock()
atomic.StoreUint32(&iii, 2)
		in := time.Now().Unix()
		for {
atomic.StoreUint32(&iii, 3)
			bl, pres := BlocksCached[BlocksComplete+1]
			if !pres {
				break
			}
			BlocksComplete++
atomic.StoreUint32(&iii, 5)
			delete(BlocksCached, BlocksComplete)
			if false {
atomic.StoreUint32(&iii, 65)
				BlockChain.CheckBlock(bl)
atomic.StoreUint32(&iii, 66)
				BlockChain.AcceptBlock(bl)
			} else {
atomic.StoreUint32(&iii, 6)
				BlockChain.Blocks.BlockAdd(BlocksComplete, bl)
			}
atomic.StoreUint32(&iii, 7)
			atomic.AddUint64(&DlBytesProcesses, uint64(len(bl.Raw)))
atomic.StoreUint32(&iii, 8)
            cu := time.Now().Unix()
			if cu!=in {
				in = cu // reschedule once a second
				BlocksMutex.Unlock()
				time.Sleep(time.Millisecond)
				BlocksMutex.Lock()
			}
		}
atomic.StoreUint32(&iii, 111)
		BlocksMutex.Unlock()

atomic.StoreUint32(&iii, 112)
		time.Sleep(1e8)

		if ct - lastdrop > 15 {
			lastdrop = ct  // drop slowest peers once for awhile
atomic.StoreUint32(&iii, 113)
			drop_slowest_peers()
		}

atomic.StoreUint32(&iii, 114)
		add_new_connections()
atomic.StoreUint32(&iii, 115)
	}
	println("all blocks done...")
}
