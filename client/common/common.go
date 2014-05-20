package common

import (
	"fmt"
	"sync"
	"time"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/chain"
	"github.com/piotrnar/gocoin/others/ver"
)

const (
	ConfigFile = "gocoin.conf"

	Version = 70001
	DefaultUserAgent = "/Gocoin:"+ver.SourcesTag+"/"
	Services = uint64(0x00000001)

	MaxCachedBlocks = 600
)

var (
	BlockChain *chain.Chain
	GenesisBlock *btc.Uint256
	Magic [4]byte
	Testnet bool

	Last struct {
		sync.Mutex // use it for writing and reading from non-chain thread
		Block *chain.BlockTreeNode
		time.Time
	}

	GocoinHomeDir string
	StartTime time.Time
	MaxPeersNeeded int

	DefaultTcpPort uint16

	MaxExpireTime time.Duration
	ExpirePerKB time.Duration

	DebugLevel int64

	CounterMutex sync.Mutex
	Counter map[string] uint64 = make(map[string]uint64)

	BusyWith string
	Busy_mutex sync.Mutex

	NetworkClosed bool
)


func CountSafe(k string) {
	CounterMutex.Lock()
	Counter[k]++
	CounterMutex.Unlock()
}

func CountSafeAdd(k string, val uint64) {
	CounterMutex.Lock()
	Counter[k] += val
	CounterMutex.Unlock()
}


func Busy(b string) {
	Busy_mutex.Lock()
	BusyWith = b
	Busy_mutex.Unlock()
}


func BytesToString(val uint64) string {
	if val < 1e6 {
		return fmt.Sprintf("%.1f KB", float64(val)/1e3)
	} else if val < 1e9 {
		return fmt.Sprintf("%.2f MB", float64(val)/1e6)
	}
	return fmt.Sprintf("%.2f GB", float64(val)/1e9)
}


func NumberToString(num float64) string {
	if num>1e15 {
		return fmt.Sprintf("%.2f P", num/1e15)
	}
	if num>1e12 {
		return fmt.Sprintf("%.2f T", num/1e12)
	}
	if num>1e9 {
		return fmt.Sprintf("%.2f G", num/1e9)
	}
	if num>1e6 {
		return fmt.Sprintf("%.2f M", num/1e6)
	}
	if num>1e3 {
		return fmt.Sprintf("%.2f K", num/1e3)
	}
	return fmt.Sprintf("%.2f", num)
}


func HashrateToString(hr float64) string {
	return NumberToString(hr)+"H/s"
}


func IsIPBlocked(ip4 []byte) bool {
	// 129.132.230.70 - 129.132.230.100 - https://bitcointalk.org/index.php?topic=319465.msg3443572#msg3443572
	/*if ip4[0]==129 && ip4[1]==132 && ip4[2]==230 && ip4[3]>=70 && ip4[3]<=100 {
		return true
	}*/
	return false
}
