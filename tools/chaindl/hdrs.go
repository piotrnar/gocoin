package main

import (
	"os"
	"time"
	"sync"
	"bytes"
	"github.com/piotrnar/gocoin/btc"
)

var (
	LastBlock struct {
		sync.Mutex
		Node *btc.BlockTreeNode
	}

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


func (c *one_net_conn) hdr_idle() bool {
	if !GetAllHeadersDone() && !c.gethdrsinprogress() {
		c.getheaders()
		c.sethdrsinprogress(true)
		return true
	}
	return false
}


func (c *one_net_conn) headers(d []byte) {
	var hdr [81]byte
	b := bytes.NewReader(d)
	cnt, er := btc.ReadVLen(b)
	if er != nil {
		return
	}
	if cnt==0 || LastBlock.Node.Height>=10e3 {
		SetAllHeadersDone(true)
		return
	}
	for i := uint64(0); i < cnt; i++ {
		if _, er = b.Read(hdr[:]); er != nil {
			return
		}
		if hdr[80]!=0 {
			println(LastBlock.Node.Height, "Unexpected value of txn_count")
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
			println(LastBlock.Node.Height, er.Error())
		}
	}
	//println("Height:", LastBlock.Node.Height)
}


func get_headers() {
	new_connection(FirstIp)
	LastBlock.Mutex.Lock()
	LastBlock.Node = BlockChain.BlockTreeEnd
	LastBlock.Mutex.Unlock()
	//println("get_headers...")
	for !GetAllHeadersDone() {
		time.Sleep(1e9)
		LastBlock.Mutex.Lock()
		println(LastBlock.Node.Height)
		LastBlock.Mutex.Unlock()
	}
}
