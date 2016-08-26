package network

import (
	"sync"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

type OneReceivedBlock struct {
	time.Time
	TmPreproc time.Duration // how long it took to start downloading this block
	TmDownload time.Duration // how long it took to dowlod this block
	TmQueuing time.Duration  // how long it took to start processing
	TmAccept time.Duration   // how long it took to commit this block
	Cnt uint
	TxMissing uint
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
	*btc.Block
	*chain.BlockTreeNode
	InProgress uint
}

var (
	ReceivedBlocks map[[btc.Uint256IdxLen]byte] *OneReceivedBlock = make(map[[btc.Uint256IdxLen]byte] *OneReceivedBlock, 400e3)
	BlocksToGet map[[btc.Uint256IdxLen]byte] *OneBlockToGet = make(map[[btc.Uint256IdxLen]byte] *OneBlockToGet)
	LastCommitedHeader *chain.BlockTreeNode
	MutexRcv sync.Mutex

	NetBlocks chan *BlockRcvd = make(chan *BlockRcvd, MAX_BLOCKS_FORWARD+10)
	NetTxs chan *TxRcvd = make(chan *TxRcvd, 2000)

	CachedBlocks []*BlockRcvd
	DiscardedBlocks map[[btc.Uint256IdxLen]byte] bool = make(map[[btc.Uint256IdxLen]byte] bool)
)
