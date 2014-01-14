package main

import (
	"fmt"
	"sync"
	"time"
	"bytes"
//	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


const (
	MAX_CONNECTIONS = 80

	GETBLOCKS_AT_ONCE_1 = 10
	GETBLOCKS_AT_ONCE_2 = 3
	GETBLOCKS_AT_ONCE_3 = 1
	MAX_BLOCKS_FORWARD = 100
	BLOCK_TIMEOUT = 3*time.Second
)

var (
	_DoBlocks bool
	BlocksToGet []*btc.BlockTreeNode
	BlocksInProgress map[[32]byte] uint32 // hash -> blockheight
	BlocksCached map[uint32] *btc.Block
	BlocksMutex sync.Mutex
	BlocksIndex uint32
	BlocksComplete uint32

	same_block_received uint

	DlStartTime time.Time
	DlBytesProcesses, DlBytesDownloaded uint
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
			if BlocksComplete == LastBlock.Node.Height {
				SetDoBlocks(true)
				println("all blocks done")
			} else {
				println("BlocksIndex", BlocksIndex, blocks_from, BlocksComplete)
				time.Sleep(1e8)
			}
			break
		}


		BlocksIndex++
		if BlocksIndex > BlocksComplete+MAX_BLOCKS_FORWARD {
			BlocksIndex = BlocksComplete
		}

		if _, done := BlocksCached[BlocksIndex]; done {
			//println(" cached ->", BlocksIndex)
			continue
		}

		bl := BlocksToGet[BlocksIndex]

		//_, inpro := BlocksInProgress[bl.BlockHash.Hash]
		c.Mutex.Lock()
		if c.blockinprogress[bl.BlockHash.Hash] {
			c.Mutex.Unlock()
			//println(" inpr ->", BlocksIndex)
			continue
		}

		BlocksInProgress[bl.BlockHash.Hash] = bl.Height
		//dmppr()
		c.blockinprogress[bl.BlockHash.Hash] = true
		c.Mutex.Unlock()

		b.Write([]byte{2,0,0,0})
		b.Write(bl.BlockHash.Hash[:])
		cnt++

		//println(c.peerip, "- getblock", bl.Height)
		//println(" get block", BlocksIndex, bl.Height, len(BlocksCached), len(BlocksInProgress), bl.BlockHash.String())
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
		println(c.peerip, "- unexpected block", h.String())
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

	height := BlocksInProgress[bl.Hash.Hash]
	if height==0 {
		same_block_received++
		//println(bl.Hash.String(), "- already received")
		return
	}
	DlBytesDownloaded += uint(len(bl.Raw))
	BlocksCached[height] = bl
	delete(BlocksInProgress, bl.Hash.Hash)

	bl.BuildTxList()
	if !bytes.Equal(btc.GetMerkel(bl.Txs), bl.MerkleRoot) {
		println(c.peerip, " - MerkleRoot mismatch at block", height)
		c.setbroken(true)
		return
	}

	//println("  got block", height)
}


func (c *one_net_conn) blk_idle() {
	if len(c.blockinprogress)==0 {
		c.getnextblock()
	} else {
		if !c.last_blk_rcvd.Add(BLOCK_TIMEOUT).After(time.Now()) {
			COUNTER("DROP_TOUT")
			c.setbroken(true)
		}
	}
}


func process_new_block(bl *btc.Block) {
	e, _, _ := BlockChain.CheckBlock(bl)
	if e != nil {
		panic(e.Error())
	}
	e = BlockChain.AcceptBlock(bl)
	if e != nil {
		panic(e.Error())
	}
	DlBytesProcesses += uint(len(bl.Raw))
}


func get_blocks() {
	BlocksInProgress = make(map[[32]byte] uint32, len(BlocksToGet))
	BlocksCached = make(map[uint32] *btc.Block, len(BlocksToGet))

	//println("opening connections")
	DlStartTime = time.Now()
	/*AddrMutex.Lock()
	for k, v := range AddrDatbase {
		if !v {
			new_connection(k)
		}
	}
	AddrMutex.Unlock()*/

	println("downloading blockchain data...")
	SetDoBlocks(true)
	pt := time.Now().Unix()
	savepeers := pt
	lastdrop := pt
	for BlocksComplete < LastBlock.Node.Height {
		ct := time.Now().Unix()

		BlocksMutex.Lock()
		bl, pres := BlocksCached[BlocksComplete+1]
		if pres {
			BlocksComplete++
			BlocksCached[BlocksComplete] = nil
			BlocksMutex.Unlock()

			process_new_block(bl)
		} else {
			BlocksMutex.Unlock()
			time.Sleep(1e7)
		}

		if ct - lastdrop > 10 {
			lastdrop = ct  // drop slowest peers every 10 seconds
			drop_worst_peers()
		}

		if ct - savepeers > 20 {
			savepeers = ct // save peers every 20 seconds
			//save_peers()
		}

		add_new_connections()

		if ct - pt >= 1 {
			pt = ct

			AddrMutex.Lock()
			adrs := len(AddrDatbase)
			AddrMutex.Unlock()
			BlocksMutex.Lock()
			indx := BlocksIndex
			inpr := len(BlocksInProgress)
			cach := len(BlocksCached)
			BlocksMutex.Unlock()
			sec := float64(time.Now().Sub(DlStartTime)) / 1e6
			fmt.Printf("H:%d/%d/%d  InPr:%d  Got:%d  Cons:%d/%d  Indx:%d, Dups:%d  DL:%.1fKBps  PR:%.1fKBps  %s\n",
				BlockChain.BlockTreeEnd.Height, BlocksComplete, LastBlock.Node.Height,
				inpr, cach, open_connection_count(), adrs, indx, same_block_received,
				float64(DlBytesDownloaded)/sec, float64(DlBytesProcesses)/sec,
				stats())
		}
	}
	println("all blocks done...")
}
