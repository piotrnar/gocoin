package network

import (
	"sync"
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
	ReceivedBlocks           map[btc.BIDX]*OneReceivedBlock = make(map[btc.BIDX]*OneReceivedBlock, 400e3)
	BlocksToGet              map[btc.BIDX]*OneBlockToGet    = make(map[btc.BIDX]*OneBlockToGet)
	BlocksToGetFailed        map[btc.BIDX]struct{}          = make(map[btc.BIDX]struct{})
	BlocksToGetFailedCheck   time.Time                      // set to zero to check ASAP
	IndexToBlocksToGet       map[uint32][]btc.BIDX          = make(map[uint32][]btc.BIDX)
	LowestIndexToBlocksToGet uint32
	LastCommitedHeader       *chain.BlockTreeNode
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

func CachedBlocksDel(oldbl *BlockRcvd) {
	CachedBlocksMutex.Lock()
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
	CachedBlocksMutex.Unlock()
}

// make sure to call it with MutexRcv locked
func DiscardBlock(n *chain.BlockTreeNode) {
	if LastCommitedHeader == n {
		LastCommitedHeader = n.Parent
		println("Revert LastCommitedHeader to", LastCommitedHeader.Height)
	}
	for _, c := range n.Childs {
		DiscardBlock(c)
	}
	DiscardedBlocks[n.BlockHash.BIdx()] = true
	delete(ReceivedBlocks, n.BlockHash.BIdx())
	if cl, ok := CachedBlocksIdx[n.Height]; ok {
		for _, clb := range cl {
			if clb.BlockTreeNode == n {
				CachedBlocksDel(clb)
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
	if LowestIndexToBlocksToGet == 0 || bh < LowestIndexToBlocksToGet {
		LowestIndexToBlocksToGet = bh
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
		if bh == LowestIndexToBlocksToGet {
			if len(IndexToBlocksToGet) > 0 {
				for LowestIndexToBlocksToGet++; ; LowestIndexToBlocksToGet++ {
					if _, ok := IndexToBlocksToGet[LowestIndexToBlocksToGet]; ok {
						break
					}
				}
			} else {
				LowestIndexToBlocksToGet = 0
			}
		}
	}

	delete(BlocksToGet, idx)
}
