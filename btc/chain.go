package btc

import (
	"fmt"
	"sort"
	"sync"
    "encoding/binary"
)


type Chain struct {
	Blocks *BlockDB      // blockchain.dat and blockchain.idx
	Unspent UnspentDB    // unspent folder

	BlockTreeRoot *BlockTreeNode
	BlockTreeEnd *BlockTreeNode
	Genesis *Uint256

	BlockIndexAccess sync.Mutex
	BlockIndex map[[Uint256IdxLen]byte] *BlockTreeNode

	DoNotSync bool // do not flush all the files after each block
}


// This is the very first function one should call in order to use this package
func NewChain(dbrootdir string, genesis *Uint256, rescan bool) (ch *Chain) {
	testnet = genesis.Hash[0]==0x43 // it's simple, but works

	ch = new(Chain)
	ch.Genesis = genesis
	ch.Blocks = NewBlockDB(dbrootdir)
	ch.Unspent = NewUnspentDb(dbrootdir, rescan)

	ch.loadBlockIndex()
	if rescan {
		ch.BlockTreeEnd = ch.BlockTreeRoot
	} else {
		// Unwind some blocks, in case if unspent DB update was interrupted last time
		for i:=0; i<3 && ch.BlockTreeEnd.Height>0; i++ {
			ch.Unspent.UndoBlockTransactions(ch.BlockTreeEnd.Height)
			ch.BlockTreeEnd = ch.BlockTreeEnd.Parent
		}
	}

	// And now re-apply the blocks which you have just reverted :)
	end, _ := ch.BlockTreeRoot.FindFarthestNode()
	if end.Height > ch.BlockTreeEnd.Height {
		ch.ParseTillBlock(end)
	}

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
	ch.BlockIndexAccess.Lock()
	s = fmt.Sprintf("CHAIN: blocks:%d  nosync:%t  Height:%d\n",
		len(ch.BlockIndex), ch.DoNotSync, ch.BlockTreeEnd.Height)
	ch.BlockIndexAccess.Unlock()
	s += ch.Blocks.GetStats()
	s += ch.Unspent.GetStats()
	return
}


func (ch *Chain) Close() {
	ch.Blocks.Close()
	ch.Unspent.Close()
}

func (x AllUnspentTx) Len() int {
	return len(x)
}

func (x AllUnspentTx) Less(i, j int) bool {
	if x[i].MinedAt == x[j].MinedAt {
		if x[i].TxPrevOut.Hash==x[j].TxPrevOut.Hash {
			return x[i].TxPrevOut.Vout < x[j].TxPrevOut.Vout
		}
		return binary.LittleEndian.Uint64(x[i].TxPrevOut.Hash[24:32]) <
			binary.LittleEndian.Uint64(x[j].TxPrevOut.Hash[24:32])
	}
	return x[i].MinedAt < x[j].MinedAt
}

func (x AllUnspentTx) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

// Returns list of unspent output from given address
// In the quick mode we only look for: 76 a9 14 [HASH160] 88 AC
func (ch *Chain) GetAllUnspent(addr []*BtcAddr, quick bool) AllUnspentTx {
	unsp := ch.Unspent.GetAllUnspent(addr, quick)
	if unsp!=nil && len(unsp)>0 {
		sort.Sort(unsp)
	}
	return unsp
}
