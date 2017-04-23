package chain

import (
	"fmt"
	"sync"
	"math/big"
	"encoding/binary"
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

	Consensus struct {
		Window, EnforceUpgrade, RejectBlock uint
		MaxPOWBits uint32
		MaxPOWValue *big.Int
		GensisTimestamp uint32
		Enforce_CSV uint32 // if non zero CVS verifications will be enforced from this block onwards
		Enforce_SEGWIT uint32 // if non zero CVS verifications will be enforced from this block onwards
		BIP9_Treshold uint32 // It is not really used at this moment, but maybe one day...
		BIP34Height uint32
		BIP65Height uint32
		BIP66Height uint32
	}
}

type NewChanOpts struct {
	// If NotifyTx is set, it will be called each time a new unspent
	// output is being added or removed. When being removed, btc.TxOut is nil.
	NotifyTxAdd func (*QdbRec)
	NotifyTxDel func (*QdbRec, []bool)

	// These two are used only during loading
	LoadWalk FunctionWalkUnspent // this one is called for each UTXO record that has just been loaded

	DoNotParseTillEnd bool // Do not rebuild UTXO database up to the highest known block (used by the downloader)<F2>

	UTXOVolatileMode bool

	UndoBlocks uint // undo this many blocks when opening the chain

	SetBlocksDBCacheSize bool
	BlocksDBCacheSize int // this value is only taken if SetBlocksDBCacheSize is true
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

	ch.Consensus.GensisTimestamp = 1231006505
	ch.Consensus.MaxPOWBits = 0x1d00ffff
	ch.Consensus.MaxPOWValue, _ = new(big.Int).SetString("00000000FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", 16)
	if ch.testnet() {
		ch.Consensus.BIP34Height = 21111
		ch.Consensus.BIP65Height = 581885
		ch.Consensus.BIP66Height = 330776
		ch.Consensus.Enforce_CSV = 770112
		ch.Consensus.Enforce_SEGWIT = 834624
		ch.Consensus.BIP9_Treshold = 1512
	} else {
		ch.Consensus.BIP34Height = 227931
		ch.Consensus.BIP65Height = 388381
		ch.Consensus.BIP66Height = 363725
		ch.Consensus.Enforce_CSV = 419328
		ch.Consensus.Enforce_SEGWIT = 0
		ch.Consensus.BIP9_Treshold = 1916
	}

	if opts.SetBlocksDBCacheSize {
		ch.Blocks = NewBlockDBExt(dbrootdir, &BlockDBOpts{MaxCachedBlocks:opts.BlocksDBCacheSize})
	} else {
		ch.Blocks = NewBlockDB(dbrootdir)
	}
	ch.Unspent, undo_last_block = NewUnspentDb(&NewUnspentOpts{
		Dir:dbrootdir, Chain:ch, Rescan:rescan, VolatimeMode:opts.UTXOVolatileMode})

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

	if opts!=nil && opts.DoNotParseTillEnd {
		ch.BlockTreeEnd, _ = ch.BlockTreeRoot.FindFarthestNode()
		return
	}

	if AbortNow {
		return
	}

	if undo_last_block {
		fmt.Println("Undo last block after the previous commit was interrupted..")
		ch.UndoLastBlock()
		fmt.Println("DONE")
	}

	if opts.UndoBlocks > 0 {
		fmt.Println("Undo", opts.UndoBlocks, "block(s) and exit...")
		for opts.UndoBlocks > 0 {
			ch.UndoLastBlock()
			opts.UndoBlocks--
		}
		return
	}

	// And now re-apply the blocks which you have just reverted :)
	end, _ := ch.BlockTreeRoot.FindFarthestNode()
	if end.Height > ch.BlockTreeEnd.Height {
		ch.ParseTillBlock(end)
	} else {
		ch.Unspent.LastBlockHeight = end.Height
	}

	return
}


// Calculate an imaginary header of the genesis block (for Timestamp() and Bits() functions from chain_tree.go)
func (ch *Chain) RebuildGenesisHeader() {
	binary.LittleEndian.PutUint32(ch.BlockTreeRoot.BlockHeader[0:4], 1) // Version
	// [4:36] - prev_block
	// [36:68] - merkle_root
	binary.LittleEndian.PutUint32(ch.BlockTreeRoot.BlockHeader[68:72], ch.Consensus.GensisTimestamp) // Timestamp
	binary.LittleEndian.PutUint32(ch.BlockTreeRoot.BlockHeader[72:76], ch.Consensus.MaxPOWBits) // Bits
	// [76:80] - nonce
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
	s = fmt.Sprintf("CHAIN: blocks:%d  nosync:%t  Height:%d  MedianTime:%d\n",
		len(ch.BlockIndex), ch.DoNotSync, ch.BlockTreeEnd.Height, ch.BlockTreeEnd.GetMedianTimePast())
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
