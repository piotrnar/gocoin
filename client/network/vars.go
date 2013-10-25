package network

import (
	"sync"
	"time"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/config"
)

type OneReceivedBlock struct {
	time.Time
	TmDownload time.Duration
	TmAccept time.Duration
	Cnt uint
}

type BlockRcvd struct {
	Conn *OneConnection
	*btc.Block
}

type TxRcvd struct {
	conn *OneConnection
	tx *btc.Tx
	raw []byte
}

var (
	ReceivedBlocks map[[btc.Uint256IdxLen]byte] *OneReceivedBlock = make(map[[btc.Uint256IdxLen]byte] *OneReceivedBlock, 300e3)
	MutexRcv sync.Mutex
	NetBlocks chan *BlockRcvd = make(chan *BlockRcvd, 1000)
	NetTxs chan *TxRcvd = make(chan *TxRcvd, 1000)

	CachedBlocks map[[btc.Uint256IdxLen]byte] OneCachedBlock = make(map[[btc.Uint256IdxLen]byte] OneCachedBlock, config.MaxCachedBlocks)
)

type OneCachedBlock struct {
	time.Time
	*btc.Block
	Conn *OneConnection
}
