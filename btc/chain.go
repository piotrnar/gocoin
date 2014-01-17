package btc

import (
	"fmt"
	"sync"
)


var AbortNow bool  // set it to true to abort any activity


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

	ch = new(Chain)
	ch.Genesis = genesis
	ch.Blocks = NewBlockDB(dbrootdir)
	ch.Unspent = NewUnspentDb(dbrootdir, rescan)

	if AbortNow {
		return
	}

	ch.loadBlockIndex()
	if AbortNow {
		return
	}

	if rescan {
		ch.BlockTreeEnd = ch.BlockTreeRoot
	} /*else if ch.BlockTreeEnd.Height>0 {
		// Redo the last block in case if unspent DB update was interrupted last time
		ch.Unspent.UndoBlockTransactions(ch.BlockTreeEnd.Height)
		ch.BlockTreeEnd = ch.BlockTreeEnd.Parent
	}*/

	if AbortNow {
		return
	}
	// And now re-apply the blocks which you have just reverted :)
	end, _ := ch.BlockTreeRoot.FindFarthestNode()
	if end.Height > ch.BlockTreeEnd.Height {
		ch.ParseTillBlock(end)
	}

	return
}


// Forces all database changes to be flushed to disk.
func (ch *Chain) Sync() {
	ch.DoNotSync = false
	ch.Blocks.Sync()
}


// Call this function periodically (i.e. each second)
// when your client is idle, to defragment databases.
func (ch *Chain) Idle() bool {
	return ch.Unspent.Idle()
}


// Save all the databases. Defragment when needed.
func (ch *Chain) Save() {
	ch.Blocks.Sync()
	ch.Unspent.Save()
}


// Returns detauils of an unspent output, it there is such.
func (ch *Chain) PickUnspent(txin *TxPrevOut) (*TxOut) {
	o, e := ch.Unspent.UnspentGet(txin)
	if e == nil {
		return o
	}
	return nil
}


// Return blockchain stats in one string.
func (ch *Chain) Stats() (s string) {
	ch.BlockIndexAccess.Lock()
	s = fmt.Sprintf("CHAIN: blocks:%d  nosync:%t  Height:%d\n",
		len(ch.BlockIndex), ch.DoNotSync, ch.BlockTreeEnd.Height)
	ch.BlockIndexAccess.Unlock()
	s += ch.Blocks.GetStats()
	s += ch.Unspent.GetStats()
	return
}


// Close the databases.
func (ch *Chain) Close() {
	ch.Blocks.Close()
	ch.Unspent.Close()
}


// Returns list of unspent output from given address
// In the quick mode we only look for: 76 a9 14 [HASH160] 88 AC
func (ch *Chain) GetAllUnspent(addr []*BtcAddr, quick bool) AllUnspentTx {
	unsp := ch.Unspent.GetAllUnspent(addr, quick)
	return unsp
}


// Returns true if we are on Testnet3 chain
func (ch *Chain) testnet() bool {
	return ch.Genesis.Hash[0]==0x43 // it's simple, but works
}
