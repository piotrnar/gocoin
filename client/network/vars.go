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
}

type OneBlockToGet struct {
	Started time.Time
	*btc.Block
	*chain.BlockTreeNode
	InProgress uint
	TmPreproc time.Time // how long it took to start downloading this block
}

var (
	ReceivedBlocks map[[btc.Uint256IdxLen]byte] *OneReceivedBlock = make(map[[btc.Uint256IdxLen]byte] *OneReceivedBlock, 400e3)
	BlocksToGet map[[btc.Uint256IdxLen]byte] *OneBlockToGet = make(map[[btc.Uint256IdxLen]byte] *OneBlockToGet)
	LastCommitedHeader *chain.BlockTreeNode
	MutexRcv sync.Mutex

	NetBlocks chan *BlockRcvd = make(chan *BlockRcvd, MAX_BLOCKS_FORWARD_CNT+10)
	NetTxs chan *TxRcvd = make(chan *TxRcvd, 2000)

	CachedBlocks []*BlockRcvd
	DiscardedBlocks map[[btc.Uint256IdxLen]byte] bool = make(map[[btc.Uint256IdxLen]byte] bool)
)
