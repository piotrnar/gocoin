package main

import (
	"fmt"
	"time"
	"sync"
	"bytes"
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)

var (
	LastBlock struct {
		sync.Mutex
		node *chain.BlockTreeNode
	}
	LastBlockHeight uint32

	allheadersdone uint32
	AllHeadersMutex sync.Mutex
)


func GetAllHeadersDone() (res bool) {
	return atomic.LoadUint32(&allheadersdone) != 0
}


func SetAllHeadersDone(res bool) {
	if res {
		atomic.StoreUint32(&allheadersdone, 1)
	} else {
		atomic.StoreUint32(&allheadersdone, 0)
	}
}


func (c *one_net_conn) hdr_idle() bool {
	if !GetAllHeadersDone() && !c.gethdrsinprogress() {
		c.getheaders()
		c.sethdrsinprogress(true)
		return true
	}
	return false
}


func (c *one_net_conn) getheaders() {
	var b [4+1+32+32+32]byte
	binary.LittleEndian.PutUint32(b[0:4], Version)
	b[4] = 2 // one inv(s)
	LastBlock.Mutex.Lock()
	copy(b[5:37], LastBlock.node.BlockHash.Hash[:])

	// In case if the head is an orphaned block, this will help
	var i int
	var n *chain.BlockTreeNode
	for n=LastBlock.node; n!=nil && i<2016; i++ {
		n = n.Parent
	}
	if n==nil {
		c.sendmsg("getheaders", b[:4+1+2*32])
	} else {
		copy(b[37:37+32], n.BlockHash.Hash[:])
		c.sendmsg("getheaders", b[:])
	}

	LastBlock.Mutex.Unlock()
}


func (c *one_net_conn) headers(d []byte) {
	var hdr [81]byte
	b := bytes.NewReader(d)
	cnt, er := btc.ReadVLen(b)
	if er != nil {
		return
	}
	if cnt==0 /*|| LastBlock.node.Height>=140e3*/ {
		SetAllHeadersDone(true)
		return
	}
	for i := uint64(0); i < cnt; i++ {
		if _, er = b.Read(hdr[:]); er != nil {
			return
		}
		if hdr[80]!=0 {
			fmt.Println(LastBlock.node.Height, "Unexpected value of txn_count")
			continue
		}
		bl, er := btc.NewBlock(hdr[:])
		TheBlockChain.BlockIndexAccess.Lock()
		er, _, _ = TheBlockChain.PreCheckBlock(bl)
		if er == nil {
			node := TheBlockChain.AcceptHeader(bl)
			LastBlock.Mutex.Lock()
			LastBlock.node = node
			LastBlock.Mutex.Unlock()
		}
		TheBlockChain.BlockIndexAccess.Unlock()
	}
	//fmt.Println("Height:", LastBlock.node.Height)
}


func get_headers() {
	if SeedNode!="" {
		pr, e := peersdb.NewPeerFromString(SeedNode, false)
		if e!=nil {
			fmt.Println("Seed node error:", e.Error())
		} else {
			fmt.Println("Seed node:", pr.Ip())
			new_connection(pr)
		}
	}
	LastBlock.Mutex.Lock()
	LastBlock.node = TheBlockChain.BlockTreeEnd
	LastBlock.Mutex.Unlock()

	tickTick := time.Tick(100*time.Millisecond)
	tickStat := time.Tick(6*time.Second)

	for !GlobalExit() && !GetAllHeadersDone() {
		select {
			case <-tickTick:
				add_new_connections()

			case <-tickStat:
				LastBlock.Mutex.Lock()
				fmt.Println("Last Header Height:", LastBlock.node.Height, "...")
				LastBlock.Mutex.Unlock()
				usif_prompt()
		}
	}
}


func download_headers() {
	fmt.Println("Loaded chain has height", TheBlockChain.BlockTreeEnd.Height,
		TheBlockChain.BlockTreeEnd.BlockHash.String())

	atomic.StoreUint32(&LastStoredBlock, TheBlockChain.BlockTreeEnd.Height)

	fmt.Println("Downloading headers...")
	get_headers()

	if GlobalExit() {
		fmt.Println("Fetching headers aborted")
		return
	}
	fmt.Println("AllHeadersDone after", time.Now().Sub(StartTime).String())

	BlocksMutex.Lock()
	LastBlock.Mutex.Lock()
	BlocksToGet = make(map[uint32][32]byte, LastBlockHeight)
	for n:=LastBlock.node; ; n=n.Parent {
		BlocksToGet[n.Height] = n.BlockHash.Hash
		if n.Height==TheBlockChain.BlockTreeEnd.Height {
			break
		}
	}
	LastBlockHeight = LastBlock.node.Height
	LastBlock.Mutex.Unlock()
	BlocksMutex.Unlock()

	SetAllHeadersDone(true)
	mark_all_hdrs_done()
}
