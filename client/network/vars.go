package network

import (
	"sync"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

type OneReceivedBlock struct {
	TmStart time.Time // when we receioved message letting us about this block
	TmPreproc time.Time // when we added this block to BlocksToGet
	TmDownload time.Time // when we finished dowloading of this block
	TmQueue time.Time  // when we started comitting this block
	TmAccepted time.Time  // when the block was commited to blockchain
	Cnt uint
	TxMissing int
	FromConID uint32
	MinFeeKSPB uint64
	NonWitnessSize int
	DoInvs bool
}

type BlockRcvd struct {
	Conn *OneConnection
	*btc.Block
	*chain.BlockTreeNode
	*OneReceivedBlock
}

type TxRcvd struct {
	conn *OneConnection
	tx *btc.Tx
	raw []byte
	trusted bool  // used only when loading mempool orm disk
}

type OneBlockToGet struct {
	Started time.Time
	*btc.Block
	*chain.BlockTreeNode
	InProgress uint
	TmPreproc time.Time // how long it took to start downloading this block
	SendInvs bool
}

var (
	ReceivedBlocks map[BIDX] *OneReceivedBlock = make(map[BIDX] *OneReceivedBlock, 400e3)
	BlocksToGet map[BIDX] *OneBlockToGet = make(map[BIDX] *OneBlockToGet)
	IndexToBlocksToGet map[uint32][]BIDX = make(map[uint32][]BIDX)
	LowestIndexToBlocksToGet uint32
	LastCommitedHeader *chain.BlockTreeNode
	MutexRcv sync.Mutex

	NetBlocks chan *BlockRcvd = make(chan *BlockRcvd, MAX_BLOCKS_FORWARD_CNT+10)
	NetTxs chan *TxRcvd = make(chan *TxRcvd, 2000)

	CachedBlocks []*BlockRcvd
	DiscardedBlocks map[BIDX] bool = make(map[BIDX] bool)
)

func AddB2G(b2g *OneBlockToGet) {
	bidx := b2g.Block.Hash.BIdx()
	BlocksToGet[bidx] = b2g
	bh := b2g.BlockTreeNode.Height
	IndexToBlocksToGet[bh] = append(IndexToBlocksToGet[bh], bidx)
	if LowestIndexToBlocksToGet==0 || bh<LowestIndexToBlocksToGet {
		LowestIndexToBlocksToGet = bh
	}

	// Trigger each connection to as the peer for block data
	Mutex_net.Lock()
	for _, v := range OpenCons {
		v.X.GetBlocksDataNow = true
	}
	Mutex_net.Unlock()
}

func DelB2G(idx BIDX) {
	b2g := BlocksToGet[idx]
	if b2g==nil {
		println("DelB2G - not found")
		return
	}

	bh := b2g.BlockTreeNode.Height
	iii := IndexToBlocksToGet[bh]
	if len(iii) > 1 {
		var n []BIDX
		for _, cidx := range iii {
			if cidx!=idx {
				n = append(n, cidx)
			}
		}
		if len(n)+1 != len(iii) {
			println("DelB2G - index not found")
		}
		IndexToBlocksToGet[bh] = n
	} else {
		if iii[0]!=idx {
			println("DelB2G - index not matching")
		}
		delete(IndexToBlocksToGet, bh)
		if bh==LowestIndexToBlocksToGet {
			if len(IndexToBlocksToGet)>0 {
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
