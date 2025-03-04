package network

import (
	"sync"
	"time"

	"slices"

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
	InProgress uint
	SendInvs   bool
}

var (
	ReceivedBlocks           map[btc.BIDX]*OneReceivedBlock = make(map[btc.BIDX]*OneReceivedBlock, 400e3)
	BlocksToGet              map[btc.BIDX]*OneBlockToGet    = make(map[btc.BIDX]*OneBlockToGet)
	IndexToBlocksToGet       map[uint32][]btc.BIDX          = make(map[uint32][]btc.BIDX)
	LowestIndexToBlocksToGet uint32
	LastCommitedHeader       *chain.BlockTreeNode
	MutexRcv                 sync.Mutex

	NetBlocks chan *BlockRcvd     = make(chan *BlockRcvd, 512)
	NetTxs    chan *txpool.TxRcvd = make(chan *txpool.TxRcvd, 2048)

	CachedBlocksMutex   sync.Mutex
	CachedBlocks        []*BlockRcvd
	CachedBlocksIdx     map[uint32][]int = make(map[uint32][]int)
	CachedMinHeight     uint32
	CachedBlocksBytes   sys.SyncInt
	MaxCachedBlocksSize sys.SyncInt
	DiscardedBlocks     map[btc.BIDX]bool = make(map[btc.BIDX]bool)

	HeadersReceived sys.SyncInt
)

func CachedBlocksLen() (l int) {
	CachedBlocksMutex.Lock()
	l = len(CachedBlocks)
	CachedBlocksMutex.Unlock()
	return
}

// make sure to call it with locked mutex
func CachedBlocksAdd(newbl *BlockRcvd) {
	height := newbl.BlockTreeNode.Height
	idxrec, ok := CachedBlocksIdx[height]
	if !ok {
		idxrec = make([]int, 0, 2)
		if len(CachedBlocksIdx) == 0 || height < CachedMinHeight {
			CachedMinHeight = height
		}
	}
	CachedBlocksIdx[height] = append(idxrec, len(CachedBlocks))
	CachedBlocks = append(CachedBlocks, newbl)
	CachedBlocksBytes.Add(newbl.Size)
	if CachedBlocksBytes.Get() > MaxCachedBlocksSize.Get() {
		MaxCachedBlocksSize.Store(CachedBlocksBytes.Get())
	}
}

// make sure to call it with locked mutex
func CachedBlocksDel(idx int) {
	oldbl := CachedBlocks[idx]
	height := oldbl.BlockTreeNode.Height
	if idxrec, ok := CachedBlocksIdx[uint32(height)]; ok {
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
			if i := slices.Index(idxrec, idx); i >= 0 {
				CachedBlocksIdx[height] = slices.Delete(idxrec, i, i+1)
			} else {
				panic("CachedBlocksDel called on block that is in CachedBlocksIdx but does not point back to it")
			}
		}
	} else {
		panic("CachedBlocksDel called on block that is not in CachedBlocksIdx")
	}
	CachedBlocksBytes.Add(-oldbl.Size)
	CachedBlocks = slices.Delete(CachedBlocks, idx, idx+1)
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
