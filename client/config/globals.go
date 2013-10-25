package config

import (
	"sync"
	"time"
	"github.com/piotrnar/gocoin/btc"
)


var (
	DebugLevel int64

	BlockChain *btc.Chain
	GenesisBlock *btc.Uint256
	Magic [4]byte
	AddrVersion byte

	Last struct {
		sync.Mutex // use it for writing and reading from non-chain thread
		Block *btc.BlockTreeNode
		time.Time
	}

	GocoinHomeDir string
	StartTime time.Time
	MaxPeersNeeded int

	DefaultTcpPort uint16

	MaxExpireTime time.Duration
	ExpirePerKB time.Duration

	Exit_now bool
	DefragBlocksDB bool
)
