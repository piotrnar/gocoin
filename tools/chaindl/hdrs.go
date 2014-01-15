package main

import (
	"os"
	"time"
	"sync"
	"bytes"
	"errors"
	"io/ioutil"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)

var (
	LastBlock struct {
		sync.Mutex
		node *btc.BlockTreeNode
	}
	LastBlockHeight uint32

	PendingHdrs map[[32]byte] bool = make(map[[32]byte] bool)
	PendingHdrsLock sync.Mutex

	_AllHeadersDone bool
	AllHeadersMutex sync.Mutex
)


func GetAllHeadersDone() (res bool) {
	AllHeadersMutex.Lock()
	res = _AllHeadersDone
	AllHeadersMutex.Unlock()
	return
}


func SetAllHeadersDone(res bool) {
	AllHeadersMutex.Lock()
	_AllHeadersDone = res
	AllHeadersMutex.Unlock()
}


func chkblock(bl *btc.Block) (er error) {
	// Check timestamp (must not be higher than now +2 hours)
	if int64(bl.BlockTime) > time.Now().Unix() + 2 * 60 * 60 {
		er = errors.New("CheckBlock() : block timestamp too far in the future")
		return
	}

	if prv, pres := BlockChain.BlockIndex[bl.Hash.BIdx()]; pres {
		if prv.Parent == nil {
			// This is genesis block
			prv.Timestamp = bl.BlockTime
			prv.Bits = bl.Bits
			er = errors.New("Genesis")
			return
		} else {
			return
		}
	}

	prevblk, ok := BlockChain.BlockIndex[btc.NewUint256(bl.Parent).BIdx()]
	if !ok {
		er = errors.New("CheckBlock: "+bl.Hash.String()+" parent not found")
		return
	}

	// Check proof of work
	gnwr := BlockChain.GetNextWorkRequired(prevblk, bl.BlockTime)
	if bl.Bits != gnwr {
		er = errors.New("CheckBlock: incorrect proof of work")
	}

	cur := new(btc.BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.Parent = prevblk
	cur.Height = prevblk.Height + 1
	cur.Bits = bl.Bits
	cur.Timestamp = bl.BlockTime
	prevblk.Childs = append(prevblk.Childs, cur)
	BlockChain.BlockIndex[cur.BlockHash.BIdx()] = cur

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
	if cnt==0 /*|| LastBlock.node.Height>=10e3*/ {
		SetAllHeadersDone(true)
		return
	}
	for i := uint64(0); i < cnt; i++ {
		if _, er = b.Read(hdr[:]); er != nil {
			return
		}
		if hdr[80]!=0 {
			println(LastBlock.node.Height, "Unexpected value of txn_count")
			continue
		}
		bl, er := btc.NewBlock(hdr[:])
		if er == nil {
			er = chkblock(bl)
			if er != nil {
				println(er.Error())
				os.Exit(1)
			}
		} else {
			println(LastBlock.node.Height, er.Error())
		}
	}
	//println("Height:", LastBlock.node.Height)
}


func get_headers() {
	new_connection(FirstIp)
	LastBlock.Mutex.Lock()
	LastBlock.node = BlockChain.BlockTreeEnd
	LastBlock.Mutex.Unlock()
	//println("get_headers...")
	for !GetAllHeadersDone() {
		time.Sleep(1e9)
		LastBlock.Mutex.Lock()
		println(LastBlock.node.Height)
		LastBlock.Mutex.Unlock()
	}
	LastBlockHeight = LastBlock.node.Height
}


func download_headers() {
	BlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)
	if btc.AbortNow || BlockChain==nil {
		return
	}

	StartTime = time.Now()
	get_headers()
	println("AllHeadersDone after", time.Now().Sub(StartTime).String())

	BlocksToGet = make([][32]byte, LastBlockHeight+1)
	for n:=LastBlock.node; ; n=n.Parent {
		BlocksToGet[n.Height] = n.BlockHash.Hash
		if n.Height==0 {
			break
		}
	}

	BlockChain.Close()
	BlockChain = nil
}


// Store the chain hashes in a file
func save_headers() {
	f, _ := os.Create("blocks.bin")
	if f != nil {
		for i := range BlocksToGet {
			f.Write(BlocksToGet[i][:])
		}
		f.Close()
	}
}


func load_headers() {
	d, e := ioutil.ReadFile("blocks.bin")
	if e!=nil {
		println(e.Error())
		os.Exit(1)
	}
	SetAllHeadersDone(true)
	cnt := len(d)/32
	BlocksToGet = make([][32]byte, cnt)
	for i:=0; i<cnt; i++ {
		copy(BlocksToGet[i][:], d[32*i:32*(i+1)])
	}
	LastBlockHeight = uint32(cnt-1)
}
