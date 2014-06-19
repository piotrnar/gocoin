package main

import (
	"os"
	"fmt"
	"time"
	"sync"
	"bytes"
	"errors"
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)

var (
	MemBlockChain *chain.Chain
	MemBlockChainMutex sync.Mutex

	LastBlock struct {
		sync.Mutex
		node *chain.BlockTreeNode
	}
	LastBlockHeight uint32

	PendingHdrs map[[32]byte] bool = make(map[[32]byte] bool)
	PendingHdrsLock sync.Mutex

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


func chkblock(bl *btc.Block) (er error) {
	// Check timestamp (must not be higher than now +2 hours)
	if int64(bl.BlockTime()) > time.Now().Unix() + 2 * 60 * 60 {
		er = errors.New("CheckBlock() : block timestamp too far in the future")
		return
	}

	MemBlockChainMutex.Lock()
	if prv, pres := MemBlockChain.BlockIndex[bl.Hash.BIdx()]; pres {
		MemBlockChainMutex.Unlock()
		if prv.Parent == nil {
			// This is genesis block
			er = errors.New("Genesis")
			return
		} else {
			return
		}
	}

	prevblk, ok := MemBlockChain.BlockIndex[btc.NewUint256(bl.ParentHash()).BIdx()]
	if !ok {
		er = errors.New("CheckBlock: "+bl.Hash.String()+" parent not found")
		return
	}

	// Check proof of work
	gnwr := MemBlockChain.GetNextWorkRequired(prevblk, bl.BlockTime())
	if bl.Bits() != gnwr {
		if !Testnet || ((prevblk.Height+1)%2016)!=0 {
			MemBlockChainMutex.Unlock()
			er = errors.New(fmt.Sprint("CheckBlock: Incorrect proof of work at block", prevblk.Height+1))
			return
		}
	}

	cur := new(chain.BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.Parent = prevblk
	cur.Height = prevblk.Height + 1
	cur.TxCount = uint32(bl.TxCount)
	copy(cur.BlockHeader[:], bl.Raw[:80])
	prevblk.Childs = append(prevblk.Childs, cur)
	MemBlockChain.BlockIndex[cur.BlockHash.BIdx()] = cur
	MemBlockChainMutex.Unlock()

	LastBlock.Mutex.Lock()
	if cur.Height > LastBlock.node.Height {
		LastBlock.node = cur
	}
	LastBlock.Mutex.Unlock()

	return
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
	var b [4+1+32+32]byte
	binary.LittleEndian.PutUint32(b[0:4], Version)
	b[4] = 1 // one inv
	LastBlock.Mutex.Lock()
	copy(b[5:37], LastBlock.node.BlockHash.Hash[:])
	LastBlock.Mutex.Unlock()
	c.sendmsg("getheaders", b[:])
}


func (c *one_net_conn) headers(d []byte) {
	var hdr [81]byte
	b := bytes.NewReader(d)
	cnt, er := btc.ReadVLen(b)
	if er != nil {
		return
	}
	if cnt==0 /*|| LastBlock.node.Height>=150e3*/ {
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
		if er == nil {
			er = chkblock(bl)
			if er != nil {
				fmt.Println(er.Error())
				os.Exit(1)
			}
		} else {
			fmt.Println(LastBlock.node.Height, er.Error())
		}
	}
	//fmt.Println("Height:", LastBlock.node.Height)
}


func get_headers() {
	if SeedNode!="" {
		pr, e := peersdb.NewPeerFromString(SeedNode)
		if e!=nil {
			fmt.Println("Seed node error:", e.Error())
		} else {
			fmt.Println("Seed node:", pr.Ip())
			new_connection(pr)
		}
	}
	LastBlock.Mutex.Lock()
	LastBlock.node = MemBlockChain.BlockTreeEnd
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
	os.RemoveAll("tmp/")
	MemBlockChain = chain.NewChain("tmp/", TheBlockChain.BlockTreeEnd.BlockHash, false)
	defer os.RemoveAll("tmp/")

	MemBlockChain.Genesis = GenesisBlock
	*MemBlockChain.BlockTreeRoot = *TheBlockChain.BlockTreeEnd
	fmt.Println("Loaded chain has height", MemBlockChain.BlockTreeRoot.Height,
		MemBlockChain.BlockTreeRoot.BlockHash.String())

	atomic.StoreUint32(&LastStoredBlock, MemBlockChain.BlockTreeRoot.Height)

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

	MemBlockChainMutex.Lock()
	MemBlockChain.Close()
	println("MemBlockChain closed")
	MemBlockChain = nil
	MemBlockChainMutex.Unlock()
}
