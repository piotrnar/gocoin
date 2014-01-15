package main

import (
	"sync"
	"time"
	"bytes"
	"sync/atomic"
	"github.com/piotrnar/gocoin/btc"
)


const (
	MAX_BLOCKS_FORWARD = 10000

	GETBLOCKS_AT_ONCE_1 = 10
	GETBLOCKS_AT_ONCE_2 = 3
	GETBLOCKS_AT_ONCE_3 = 1
	BLOCK_TIMEOUT = 3*time.Second
)

type one_bip struct {
	Height uint32
	Count uint32
}

var (
	_DoBlocks bool
	BlocksToGet [][32]byte
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
	var cnt byte
	var maxcnt byte
	b := new(bytes.Buffer)
	b.WriteByte(0)
	BlocksMutex.Lock()

	blocks_from := BlocksIndex

	if BlocksIndex<100e3 {
		maxcnt = GETBLOCKS_AT_ONCE_1
	} else if BlocksIndex<200e3 {
		maxcnt = GETBLOCKS_AT_ONCE_2
	} else {
		maxcnt = GETBLOCKS_AT_ONCE_3
	}

	for secondloop:=false; cnt<maxcnt; secondloop=true {
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

		bh := BlocksToGet[BlocksIndex]

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
	}
	BlocksMutex.Unlock()

	pl := b.Bytes()
	pl[0] = cnt
	//println("getdata", hex.EncodeToString(pl))
	c.sendmsg("getdata", pl)
	c.last_blk_rcvd = time.Now()
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

		if v.connected_at.IsZero() {
			// still connecting
			continue
		}

		if time.Now().Sub(v.connected_at) < 3*time.Second {
			// give him 3 seconds
			continue
		}

		v.Lock()
		br := v.bytes_received
		v.Unlock()

		if br==0 {
			// if zero bytes received after 3 seconds - drop it!
			v.setbroken(true)
			//println(" -", v.peerip, "- idle")
			COUNTER("IDLE")
			continue
		}

		bps := v.bps()
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
	BlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)
	if btc.AbortNow || BlockChain==nil {
		return
	}

	BlocksInProgress = make(map[[32]byte] *one_bip, MAX_BLOCKS_FORWARD)
	BlocksCached = make(map[uint32] *btc.Block, len(BlocksToGet))

	//println("opening connections")
	DlStartTime = time.Now()

	println("downloading blockchain data...")
	SetDoBlocks(true)
	lastdrop := time.Now().Unix()
	for GetDoBlocks() {
		ct := time.Now().Unix()

		for {
			BlocksMutex.Lock()
			bl, pres := BlocksCached[BlocksComplete+1]
			if !pres {
				break
			}
			BlocksComplete++
			BlocksCached[BlocksComplete] = nil
			if false {
				BlockChain.CheckBlock(bl)
				BlockChain.AcceptBlock(bl)
			} else {
				BlockChain.Blocks.BlockAdd(BlocksComplete, bl)
			}
			atomic.AddUint64(&DlBytesProcesses, uint64(len(bl.Raw)))

			BlocksMutex.Unlock()
			time.Sleep(time.Millisecond) // reschedule
		}
		BlocksMutex.Unlock()

		time.Sleep(1e8)

		if ct - lastdrop > 15 {
			lastdrop = ct  // drop slowest peers once for awhile
			drop_slowest_peers()
		}

		add_new_connections()
	}
	println("all blocks done...")
}
