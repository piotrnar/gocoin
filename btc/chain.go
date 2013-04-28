package btc

import (
	"fmt"
	"sort"
)


type Chain struct {
	Blocks *BlockDB
	Unspent UnspentDB
	
	BlockTreeRoot *BlockTreeNode
	BlockTreeEnd *BlockTreeNode
	Genesis *Uint256

	BlockIndex map[[Uint256IdxLen]byte] *BlockTreeNode

	DoNotSync bool // do not flush all trhe files after each block
}


func NewChain(dbrootdir string, genesis *Uint256, rescan bool) (ch *Chain) {
	testnet = genesis.Hash[0]==0x43
	
	ch = new(Chain)
	ch.Genesis = genesis
	ch.Blocks = NewBlockDB(dbrootdir)
	ch.Unspent = NewUnspentDb(dbrootdir, rescan)

	ch.loadBlockIndex() 
	if rescan {
		ch.BlockTreeEnd = ch.BlockTreeRoot
	}
	
	// Unwind some blocks, in case if unspent DB update was interrupted last time
	for i:=0; i<3 && ch.BlockTreeEnd.Height>0; i++ {
		ch.Unspent.UndoBlockTransactions(ch.BlockTreeEnd.Height)
		ch.BlockTreeEnd = ch.BlockTreeEnd.parent
	}
	
	end, _ := ch.BlockTreeRoot.FindFarthestNode()
	if end.Height > ch.BlockTreeEnd.Height {
		ch.ParseTillBlock(end)
	}

	return 
}


func NewBlockIndex(h []byte) (o [Uint256IdxLen]byte) {
	copy(o[:], h[:Uint256IdxLen])
	return 
}


func (ch *Chain) Sync() {
	ch.DoNotSync = false
	ch.Blocks.Sync()
	ch.Unspent.Sync()
}


func (ch *Chain) Idle() {
	ch.Unspent.Idle()
}

func (ch *Chain) Save() {
	ch.Blocks.Sync()
	ch.Unspent.Save()
}


func (ch *Chain) PickUnspent(txin *TxPrevOut) (*TxOut) {
	o, e := ch.Unspent.UnspentGet(txin)
	if e == nil {
		return o
	}
	return nil
}


func (ch *Chain)Stats() (s string) {
	s = fmt.Sprintf("CHAIN: blocks:%d  nosync:%t  Height:%d\n", 
		len(ch.BlockIndex), ch.DoNotSync, ch.BlockTreeEnd.Height)
	s += ch.Blocks.GetStats()
	s += ch.Unspent.GetStats()
	return
}

func (ch *Chain)GetHeight() uint32 {
	return ch.BlockTreeEnd.Height
}


func (ch *Chain) Close() {
	ch.Blocks.Close()
	ch.Unspent.Close()
}

func (x AllUnspentTx) Len() int {
	return len(x)
}

func (x AllUnspentTx) Less(i, j int) bool {
	return x[i].MinedAt < x[j].MinedAt
}

func (x AllUnspentTx) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

// Returns list of unspent output fro a given address
func (ch *Chain) GetAllUnspent(addr []*BtcAddr) AllUnspentTx {
	unsp := ch.Unspent.GetAllUnspent(addr)
	if unsp!=nil && len(unsp)>0 {
		sort.Sort(unsp)
	}
	return unsp
}

