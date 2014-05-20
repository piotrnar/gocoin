package main

import (
	"os"
	"fmt"
	"time"
	"sync"
	"bytes"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/chain"
)

var (
	MemBlockChain *chain.Chain

	LastBlock struct {
		sync.Mutex
		node *chain.BlockTreeNode
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
	if int64(bl.BlockTime()) > time.Now().Unix() + 2 * 60 * 60 {
		er = errors.New("CheckBlock() : block timestamp too far in the future")
		return
	}

	if prv, pres := MemBlockChain.BlockIndex[bl.Hash.BIdx()]; pres {
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
			er = errors.New(fmt.Sprint("CheckBlock: Incorrect proof of work at block", prevblk.Height+1))
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
	new_connection(FirstIp)
	LastBlock.Mutex.Lock()
	LastBlock.node = MemBlockChain.BlockTreeEnd
	LastBlock.Mutex.Unlock()
	lt := time.Now().Unix()
	for !GetAllHeadersDone() {
		time.Sleep(1e8)
		ct := time.Now().Unix()
		if ct-lt > 5 {
			lt = ct
			LastBlock.Mutex.Lock()
			fmt.Println("Last Header Height:", LastBlock.node.Height, "...")
			LastBlock.Mutex.Unlock()
			usif_prompt()
		}
	}
	LastBlockHeight = LastBlock.node.Height
}


func download_headers() {
	os.RemoveAll("tmp/")
	MemBlockChain = chain.NewChain("tmp/", TheBlockChain.BlockTreeEnd.BlockHash, false)
	defer os.RemoveAll("tmp/")

	MemBlockChain.Genesis = GenesisBlock
	*MemBlockChain.BlockTreeRoot = *TheBlockChain.BlockTreeEnd
	fmt.Println("Loaded chain has height", MemBlockChain.BlockTreeRoot.Height,
		MemBlockChain.BlockTreeRoot.BlockHash.String())

	get_headers()
	fmt.Println("AllHeadersDone after", time.Now().Sub(StartTime).String())

	AddrMutex.Lock()
	for !GlobalExit && len(AddrDatbase) < 60 {
		fmt.Println(len(AddrDatbase), "known peers at the moment - wait for more...")
		AddrMutex.Unlock()
		time.Sleep(3e9)
		AddrMutex.Lock()
	}
	AddrMutex.Unlock()

	BlocksToGet = make(map[uint32][32]byte, LastBlockHeight)
	for n:=LastBlock.node; ; n=n.Parent {
		BlocksToGet[n.Height] = n.BlockHash.Hash
		if n.Height==TheBlockChain.BlockTreeEnd.Height {
			break
		}
	}

	close_all_connections()

	MemBlockChain.Close()
	MemBlockChain = nil
}
