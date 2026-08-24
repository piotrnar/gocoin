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

const (
	// UnpinBlockAfter is how long we keep fetching a block only from the peers that
	// announced it. After that time any peer can serve it to us.
	UnpinBlockAfter = 5 * time.Minute

	// GiveUpOnBlockAfter is how long we keep trying to fetch a block before dropping it.
	// Such a block is not marked as invalid, so we will fetch it again (and not ban
	// anyone) if its header gets announced to us once more.
	GiveUpOnBlockAfter = 60 * time.Minute
)

type OneBlockToGet struct {
	Started   time.Time
	TmPreproc time.Time
	*btc.Block
	*chain.BlockTreeNode
	// OnlyFetchFrom holds peersdb.PeerAddr.UniqID() of the peers that announced this
	// block to us. While it is not empty, we only fetch the block from those peers.
	// Note: it must not be ConnID, as the same peer reconnecting gets a new ConnID.
	OnlyFetchFrom []uint64
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

	// DiscardedBlocks are the blocks we do not want to fetch anymore.
	// The value tells whether we know the block to be invalid:
	//   true  - the block (or one of its ancestors) failed verification. A peer
	//           announcing it is misbehaving, so we ban it.
	//   false - we only gave up on downloading it. Nobody is to blame and the block
	//           may well be valid, so if a peer announces its header again, we undo
	//           the discard and fetch it once more.
	DiscardedBlocks map[btc.BIDX]bool = make(map[btc.BIDX]bool)

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

// DiscardBlock marks the given block, and all of its descendants, as not to be
// fetched. Set invalid to true only if the block has actually failed verification -
// see the comment at DiscardedBlocks. The tree nodes are left in place - see
// DiscardBranch() if you also need them gone.
// Make sure to call it with MutexRcv locked.
func DiscardBlock(n *chain.BlockTreeNode, invalid bool) {
	resetLastCommitedHeaderBelow(n)
	CachedBlocksMutex.Lock()
	discardBlock(n, invalid)
	CachedBlocksMutex.Unlock()
}

// DiscardBranch does what DiscardBlock() does, but additionally removes the given
// block and all of its descendants from the chain's tree. Use it for the blocks that
// we know are invalid, so their hashes are remembered in DiscardedBlocks and we do
// not fetch them again when a peer re-announces the header.
// Make sure to call it with MutexRcv locked and BlockIndexAccess unlocked.
func DiscardBranch(n *chain.BlockTreeNode) {
	// do the bookkeeping first - after DeleteBranch() the child links are gone
	DiscardBlock(n, true)
	common.BlockChain.DeleteBranch(n, nil)
	common.CountSafe("BlockBranchDiscrd")
}

// caller must hold both MutexRcv and CachedBlocksMutex
func discardBlock(n *chain.BlockTreeNode, invalid bool) {
	for _, c := range n.Childs {
		discardBlock(c, invalid)
	}
	bidx := n.BlockHash.BIdx()
	// never downgrade a block that we already know to be invalid
	DiscardedBlocks[bidx] = invalid || DiscardedBlocks[bidx]
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
