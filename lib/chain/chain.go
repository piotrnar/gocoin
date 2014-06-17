package chain

import (
	"fmt"
	"sync"
	"github.com/piotrnar/gocoin/lib/btc"
)


var AbortNow bool  // set it to true to abort any activity


type Chain struct {
	Blocks *BlockDB      // blockchain.dat and blockchain.idx
	Unspent *UnspentDB    // unspent folder

	BlockTreeRoot *BlockTreeNode
	BlockTreeEnd *BlockTreeNode
	Genesis *btc.Uint256

	BlockIndexAccess sync.Mutex
	BlockIndex map[[btc.Uint256IdxLen]byte] *BlockTreeNode

	DoNotSync bool // do not flush all the files after each block

	CB NewChanOpts // callbacks used by Unspent database
}

type NewChanOpts struct {
	// If NotifyTx is set, it will be called each time a new unspent
	// output is being added or removed. When being removed, btc.TxOut is nil.
	NotifyTxAdd func (*QdbRec)
	NotifyTxDel func ([]byte, []bool)

	// These two are used only during loading
	LoadWalk FunctionWalkUnspent // this one is called for each UTXO record that has just been loaded
}


func NewChain(dbrootdir string, genesis *btc.Uint256, rescan bool) (ch *Chain) {
	return NewChainExt(dbrootdir, genesis, rescan, nil)
}


// This is the very first function one should call in order to use this package
func NewChainExt(dbrootdir string, genesis *btc.Uint256, rescan bool, opts *NewChanOpts) (ch *Chain) {
	var undo_last_block bool
	ch = new(Chain)
	ch.Genesis = genesis
	if opts != nil {
		ch.CB = *opts
	}
	ch.Blocks = NewBlockDB(dbrootdir)
	ch.Unspent, undo_last_block = NewUnspentDb(dbrootdir, rescan, ch)

	if AbortNow {
		return
	}

	ch.loadBlockIndex()
	if AbortNow {
		return
	}

	if rescan {
		ch.BlockTreeEnd = ch.BlockTreeRoot
	}

	if AbortNow {
		return
	}

	if undo_last_block {
		fmt.Println("Undo last block after the previous commit was interrupted..")
		ch.UndoLastBlock()
		fmt.Println("DONE")
	}

	// And now re-apply the blocks which you have just reverted :)
	end, _ := ch.BlockTreeRoot.FindFarthestNode()
	if end.Height > ch.BlockTreeEnd.Height {
		ch.ParseTillBlock(end)
	}
	ch.Unspent.LastBlockHeight = end.Height

	return
}


// Forces all database changes to be flushed to disk.
func (ch *Chain) Sync() {
	ch.DoNotSync = false
	ch.Blocks.Sync()
	ch.Unspent.Sync()
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
func (ch *Chain) PickUnspent(txin *btc.TxPrevOut) (*btc.TxOut) {
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


// Returns true if we are on Testnet3 chain
func (ch *Chain) testnet() bool {
	return ch.Genesis.Hash[0]==0x43 // it's simple, but works
}
