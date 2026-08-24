package network

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"slices"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

type OneReceivedBlock struct {
	TmStart    time.Time // when we received message telling us about this block
	TmPreproc  time.Time // when we added this block to BlocksToGet
	TmDownload time.Time // when we finished downloading this block
	TmQueue    time.Time // when we started committing this block
	TmAccepted time.Time // when the block was committed to blockchain
	*btc.BlockUserInfo
	TxMissing   int
	FromConID   uint32
	DownloadCnt uint16
	DoInvs      bool
}

type BlockRcvd struct {
	Conn *OneConnection
	*btc.Block
	*chain.BlockTreeNode
	*OneReceivedBlock
	*btc.BlockExtraInfo
	Size int
}

type OneBlockToGet struct {
	Started   time.Time
	TmPreproc time.Time
	*btc.Block
	*chain.BlockTreeNode
	OnlyFetchFrom []uint32
	InProgress    uint32
	SendInvs      bool
}

var (
	ReceivedBlocks           map[btc.BIDX]*OneReceivedBlock = make(map[btc.BIDX]*OneReceivedBlock, 950e3)
	BlocksToGet              map[btc.BIDX]*OneBlockToGet    = make(map[btc.BIDX]*OneBlockToGet)
	BlocksToGetFailed        map[btc.BIDX]struct{}          = make(map[btc.BIDX]struct{})
	BlocksToGetFailedCheck   time.Time                      // set to zero to check ASAP
	IndexToBlocksToGet       map[uint32][]btc.BIDX          = make(map[uint32][]btc.BIDX)
	LowestIndexToBlocksToGet atomic.Uint32
	LastCommitedHeader       *chain.BlockTreeNode
	HeadersSyncDone          sys.SyncBool // set when our header chain reaches the network's tip
	MutexRcv                 sync.Mutex

	NetBlocks chan *BlockRcvd     = make(chan *BlockRcvd, 512)
	NetTxs    chan *txpool.TxRcvd = make(chan *txpool.TxRcvd, 2048)

	CachedBlocksMutex   sync.Mutex
	CachedBlocksIdx     map[uint32][]*BlockRcvd = make(map[uint32][]*BlockRcvd, MAX_BLOCKS_FORWARD_CNT)
	CachedMinHeight     uint32
	CachedMaxHeight     uint32
	CachedBlocksBytes   sys.SyncInt
	MaxCachedBlocksSize sys.SyncInt
	DiscardedBlocks     map[btc.BIDX]bool = make(map[btc.BIDX]bool)

	HeadersReceived sys.SyncInt
)

/*
func check_cache() {
	var lowest_h uint32
	for h, idxs := range CachedBlocksIdx {
		if lowest_h == 0 || h < lowest_h {
			lowest_h = h
		}
		if h < CachedMinHeight {
			println(h, CachedMinHeight)
			panic("h < CachedMinHeight")
		}
		for _, bl := range idxs {
			if bl.BlockTreeNode.Height != h {
				panic("bl.BlockTreeNode.Height != h")
			}
		}
	}
	if lowest_h != CachedMinHeight {
		println(lowest_h, CachedMinHeight)
		panic("lowest_h != CachedMinHeight")
	}
}
*/

func CachedBlocksLen() (l int) {
	CachedBlocksMutex.Lock()
	l = len(CachedBlocksIdx)
	CachedBlocksMutex.Unlock()
	return
}

func CachedBlocksAdd(newbl *BlockRcvd) {
	CachedBlocksMutex.Lock()
	//check_cache()
	height := newbl.BlockTreeNode.Height
	idxrec, ok := CachedBlocksIdx[height]
	if !ok {
		if len(CachedBlocksIdx) == 0 || height < CachedMinHeight {
			CachedMinHeight = height
		}
		if height > CachedMaxHeight {
			CachedMaxHeight = height
		}
		CachedBlocksIdx[height] = []*BlockRcvd{newbl}
	} else {
		CachedBlocksIdx[height] = append(idxrec, newbl)
		//println(len(idxrec)+1, "blocks at height", height)
	}
	CachedBlocksBytes.Add(newbl.Size)
	if CachedBlocksBytes.Get() > MaxCachedBlocksSize.Get() {
		MaxCachedBlocksSize.Store(CachedBlocksBytes.Get())
	}
	CachedBlocksMutex.Unlock()
}

// caller must hold CachedBlocksMutex
func cachedBlocksDel(oldbl *BlockRcvd) {
	height := oldbl.BlockTreeNode.Height
	if idxrec, ok := CachedBlocksIdx[height]; ok {
		if len(idxrec) == 1 {
			delete(CachedBlocksIdx, height)
			if CachedMinHeight == height && len(CachedBlocksIdx) > 0 {
				for {
					CachedMinHeight++
					if _, ok := CachedBlocksIdx[uint32(CachedMinHeight)]; ok {
						break
					}
				}
			}
		} else {
			if i := slices.Index(idxrec, oldbl); i >= 0 {
				CachedBlocksIdx[height] = slices.Delete(idxrec, i, i+1)
			} else {
				panic("CachedBlocksDel called on block that is in CachedBlocksIdx but does not point back to it")
			}
		}
	} else {
		panic("CachedBlocksDel called on block that is not in CachedBlocksIdx")
	}
	CachedBlocksBytes.Add(-oldbl.Size)
}

func CachedBlocksDel(oldbl *BlockRcvd) {
	CachedBlocksMutex.Lock()
	cachedBlocksDel(oldbl)
	CachedBlocksMutex.Unlock()
}

// resetLastCommitedHeaderBelow moves LastCommitedHeader to root.Parent if
// LastCommitedHeader is root itself or any of root's descendants - i.e. if it
// sits inside the branch that is about to be removed by DeleteBranch().
// Call with MutexRcv locked.
func resetLastCommitedHeaderBelow(root *chain.BlockTreeNode) {
	for n := LastCommitedHeader; n != nil && n.Height >= root.Height; n = n.Parent {
		if n == root {
			LastCommitedHeader = root.Parent
			return
		}
	}
}

// delBlockFromDiskCache removes the block's data that we stored in the temp folder.
func delBlockFromDiskCache(hash *btc.Uint256) {
	tmpfn := common.TempBlocksDir() + hash.String()
	os.Remove(tmpfn)
	os.Remove(tmpfn + ".hashes")
}

// DiscardBlock marks the given block, and all of its descendants, as never to be
// fetched again. The tree nodes are left in place - see DiscardBranch() if you also
// need them gone.
// Make sure to call it with MutexRcv locked.
func DiscardBlock(n *chain.BlockTreeNode) {
	resetLastCommitedHeaderBelow(n)
	CachedBlocksMutex.Lock()
	discardBlock(n)
	CachedBlocksMutex.Unlock()
}

// DiscardBranch does what DiscardBlock() does, but additionally removes the given
// block and all of its descendants from the chain's tree. Use it for the blocks that
// we know are invalid, so their hashes are remembered in DiscardedBlocks and we do
// not fetch them again when a peer re-announces the header.
// Make sure to call it with MutexRcv locked and BlockIndexAccess unlocked.
func DiscardBranch(n *chain.BlockTreeNode) {
	// do the bookkeeping first - after DeleteBranch() the child links are gone
	DiscardBlock(n)
	common.BlockChain.DeleteBranch(n, nil)
	common.CountSafe("BlockBranchDiscrd")
}

// caller must hold both MutexRcv and CachedBlocksMutex
func discardBlock(n *chain.BlockTreeNode) {
	for _, c := range n.Childs {
		discardBlock(c)
	}
	bidx := n.BlockHash.BIdx()
	DiscardedBlocks[bidx] = true
	delete(ReceivedBlocks, bidx)
	delete(BlocksToGetFailed, bidx)
	if _, ok := BlocksToGet[bidx]; ok {
		DelB2G(bidx) // no point in fetching it anymore
		common.CountSafe("BlockDiscardB2G")
	}
	if cl, ok := CachedBlocksIdx[n.Height]; ok {
		for _, clb := range cl {
			if clb.BlockTreeNode == n {
				if clb.Block == nil {
					delBlockFromDiskCache(n.BlockHash)
				}
				cachedBlocksDel(clb)
				common.CountSafe("BlockDiscardCach")
				break
			}
		}
	}
}

func AddB2G(b2g *OneBlockToGet) {
	bidx := b2g.Block.Hash.BIdx()
	BlocksToGet[bidx] = b2g
	bh := b2g.BlockTreeNode.Height
	IndexToBlocksToGet[bh] = append(IndexToBlocksToGet[bh], bidx)
	if LowestIndexToBlocksToGet.Load() == 0 || bh < LowestIndexToBlocksToGet.Load() {
		LowestIndexToBlocksToGet.Store(bh)
	}
}

func DelB2G(idx btc.BIDX) {
	b2g := BlocksToGet[idx]
	if b2g == nil {
		println("DelB2G - not found")
		return
	}

	bh := b2g.BlockTreeNode.Height
	iii := IndexToBlocksToGet[bh]
	if len(iii) > 1 {
		var n []btc.BIDX
		for _, cidx := range iii {
			if cidx != idx {
				n = append(n, cidx)
			}
		}
		if len(n)+1 != len(iii) {
			println("DelB2G - index not found")
		}
		IndexToBlocksToGet[bh] = n
	} else {
		if iii[0] != idx {
			println("DelB2G - index not matching")
		}
		delete(IndexToBlocksToGet, bh)
		if bh == LowestIndexToBlocksToGet.Load() {
			if len(IndexToBlocksToGet) > 0 {
				for LowestIndexToBlocksToGet.Add(1); ; LowestIndexToBlocksToGet.Add(1) {
					if _, ok := IndexToBlocksToGet[LowestIndexToBlocksToGet.Load()]; ok {
						break
					}
				}
			} else {
				LowestIndexToBlocksToGet.Store(0)
			}
		}
	}

	delete(BlocksToGet, idx)
}
