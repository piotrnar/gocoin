package network

import (
	"sync"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
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

	CachedBlocks map[[btc.Uint256IdxLen]byte] OneCachedBlock = make(map[[btc.Uint256IdxLen]byte] OneCachedBlock, common.MaxCachedBlocks)
)

type OneCachedBlock struct {
	time.Time
	*btc.Block
	Conn *OneConnection
}


// This one shall only be called from the chain thread (this no protection)
func AddBlockToCache(bl *btc.Block, conn *OneConnection) {
	// we use CachedBlocks only from one therad so no need for a mutex
	if len(CachedBlocks)==common.MaxCachedBlocks {
		// Remove the oldest one
		oldest := time.Now()
		var todel [btc.Uint256IdxLen]byte
		for k, v := range CachedBlocks {
			if v.Time.Before(oldest) {
				oldest = v.Time
				todel = k
			}
		}
		delete(CachedBlocks, todel)
		common.CountSafe("CacheBlocksExpired")
	}
	CachedBlocks[bl.Hash.BIdx()] = OneCachedBlock{Time:time.Now(), Block:bl, Conn:conn}
}
