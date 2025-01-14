package common

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/memory"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/others/utils"
)

const (
	Version = uint32(70015)

	TX_SIZE_RAM_MULTIPLIER = 3 // we assume that each tx takes so much more RAM than its raw size
)

var (
	Services uint64 = btc.SERVICE_SEGWIT | btc.SERVICE_NETWORK_LIMITED // It updates this value in InitConfig()

	LogBuffer             = new(bytes.Buffer)
	Log       *log.Logger = log.New(LogBuffer, "", 0)

	BlockChain     *chain.Chain
	GenesisBlock   *btc.Uint256
	Magic          [4]byte
	Testnet        bool
	DefaultTcpPort uint16 = 8333

	Last TheLastBlock

	GocoinHomeDir string
	StartTime     time.Time

	CounterMutex sync.Mutex
	Counter      map[string]uint64 = make(map[string]uint64)

	busyLine int32

	NetworkClosed sys.SyncBool

	AverageBlockSize  sys.SyncInt
	avg_bsize_history []uint32
	avg_bsize_idx     int
	avg_bsize_next    uint32
	avg_bsize_sum     uint

	allBalMinVal uint64

	DropSlowestEvery time.Duration
	BlockExpireEvery time.Duration
	PingPeerEvery    time.Duration

	UserAgent string

	ListenTCP bool

	minFeePerKB, routeMinFeePerKB, minminFeePerKB uint64
	maxMempoolSizeBytes, maxRejectedSizeBytes     uint64

	KillChan chan os.Signal = make(chan os.Signal)

	SecretKey []byte // 32 bytes of secret key
	PublicKey string

	WalletON       bool
	WalletProgress uint32 // 0 for not / 1000 for max
	WalletOnIn     uint32

	BlockChainSynchronized bool

	lastTrustedBlock       *btc.Uint256
	LastTrustedBlockHeight uint32

	Memory   memory.Allocator
	MemMutex sync.Mutex

	NoCounters sys.SyncBool

	SyncMaxCacheBytes sys.SyncInt
)

type TheLastBlock struct {
	sync.Mutex // use it for writing and reading from non-chain thread
	Block      *chain.BlockTreeNode
	time.Time
	ParseTill   *chain.BlockTreeNode
	ScriptFlags uint32
}

func (b *TheLastBlock) BlockHeight() (res uint32) {
	b.Mutex.Lock()
	res = b.Block.Height
	b.Mutex.Unlock()
	return
}

func CountSafe(k string) {
	if !NoCounters.Get() {
		CounterMutex.Lock()
		Counter[k]++
		CounterMutex.Unlock()
	}
}

func CountSafeAdd(k string, val uint64) {
	if !NoCounters.Get() {
		CounterMutex.Lock()
		Counter[k] += val
		CounterMutex.Unlock()
	}
}

func CountSafeStore(k string, val uint64) {
	if !NoCounters.Get() {
		CounterMutex.Lock()
		Counter[k] = val
		CounterMutex.Unlock()
	}
}

func CounterGet(k string) (val uint64) {
	CounterMutex.Lock()
	val = Counter[k]
	CounterMutex.Unlock()
	return
}

func Count(k string) {
	if !NoCounters.Get() {
		Counter[k]++
	}
}

func CountAdd(k string, val uint64) {
	if !NoCounters.Get() {
		Counter[k] += val
	}
}

func Busy() {
	var line int
	_, _, line, _ = runtime.Caller(1)
	atomic.StoreInt32(&busyLine, int32(line))
}

func BusyIn() int {
	return int(atomic.LoadInt32(&busyLine))
}

func BytesToString(val uint64) string {
	if val < 1e6 {
		return fmt.Sprintf("%.1f KB", float64(val)/1e3)
	} else if val < 1e9 {
		return fmt.Sprintf("%.2f MB", float64(val)/1e6)
	}
	return fmt.Sprintf("%.2f GB", float64(val)/1e9)
}

// max 6 characters
func UintToString(num uint64) string {
	if num < 1e5 {
		return fmt.Sprint(num) + " "
	}
	if num < 10e6 {
		return fmt.Sprintf("%.0f K", float64(num)/1e3)
	}
	if num < 10e7 {
		return fmt.Sprintf("%.1f M", float64(num)/1e6)
	}
	if num < 10e9 {
		return fmt.Sprintf("%.0f M", float64(num)/1e6)
	}
	if num < 10e10 {
		return fmt.Sprintf("%.1f G", float64(num)/1e9)
	}
	if num < 10e12 {
		return fmt.Sprintf("%.0f G", float64(num)/1e9)
	}
	if num < 10e13 {
		return fmt.Sprintf("%.1f T", float64(num)/1e12)
	}
	if num < 10e15 {
		return fmt.Sprintf("%.0f T", float64(num)/1e12)
	}
	if num < 10e16 {
		return fmt.Sprintf("%.1f P", float64(num)/1e15)
	}
	if num < 10e18 {
		return fmt.Sprintf("%.0f P", float64(num)/1e15)
	}
	return fmt.Sprintf("%.1f E", float64(num)/1e18)
}

func FloatToString(num float64) string {
	if num > 1e24 {
		return fmt.Sprintf("%.2f Y", num/1e24)
	}
	if num > 1e21 {
		return fmt.Sprintf("%.2f Z", num/1e21)
	}
	if num > 1e18 {
		return fmt.Sprintf("%.2f E", num/1e18)
	}
	if num > 1e15 {
		return fmt.Sprintf("%.2f P", num/1e15)
	}
	if num > 1e12 {
		return fmt.Sprintf("%.2f T", num/1e12)
	}
	if num > 1e9 {
		return fmt.Sprintf("%.2f G", num/1e9)
	}
	if num > 1e6 {
		return fmt.Sprintf("%.2f M", num/1e6)
	}
	if num > 1e3 {
		return fmt.Sprintf("%.2f K", num/1e3)
	}
	return fmt.Sprintf("%.2f", num)
}

func HashrateToString(hr float64) string {
	return FloatToString(hr) + "H/s"
}

// RecalcAverageBlockSize calculates the average blocks size over the last 2016 blocks.
// Only call from blockchain thread.
func RecalcAverageBlockSize() {
	var le uint
	var new_avg_size int
	mutex_cfg.Lock()
	AVG_BSIZE_SPAN := int(CFG.Stat.BSizeBlks)
	mutex_cfg.Unlock()
	n := BlockChain.LastBlock()
	new_height := n.Height
	if avg_bsize_next != 0 && n.Height == avg_bsize_next {
		le = uint(n.BlockSize)
		if len(avg_bsize_history) == AVG_BSIZE_SPAN {
			if avg_bsize_idx >= AVG_BSIZE_SPAN {
				avg_bsize_idx = 0
			}
			avg_bsize_sum -= uint(avg_bsize_history[avg_bsize_idx])
			avg_bsize_history[avg_bsize_idx] = uint32(le)
		} else {
			avg_bsize_history = append(avg_bsize_history, uint32(le))
		}
		avg_bsize_idx++
		avg_bsize_sum += le
		new_avg_size = int(avg_bsize_sum) / len(avg_bsize_history)
	} else {
		//println("Recalc avg_bsize @", new_height)
		avg_bsize_history = make([]uint32, 0, AVG_BSIZE_SPAN)
		avg_bsize_idx = 0
		avg_bsize_sum = 0
		for maxcnt := AVG_BSIZE_SPAN; maxcnt > 0 && n != nil; maxcnt-- {
			le = uint(n.BlockSize)
			avg_bsize_history = append(avg_bsize_history, uint32(le))
			avg_bsize_idx++
			avg_bsize_sum += le
			n = n.Parent
		}
		hl := len(avg_bsize_history)
		for i := 0; i < hl>>1; i++ {
			avg_bsize_history[i], avg_bsize_history[hl-1-i] = avg_bsize_history[hl-1-i], avg_bsize_history[i]
		}

		if avg_bsize_sum == 0 || avg_bsize_idx == 0 {
			new_avg_size = 204
		} else {
			new_avg_size = int(avg_bsize_sum) / avg_bsize_idx
		}
	}
	AverageBlockSize.Store(new_avg_size)
	avg_bsize_next = new_height + 1
}

func GetRawTx(BlockHeight uint32, txid *btc.Uint256) (data []byte, er error) {
	data, er = BlockChain.GetRawTx(BlockHeight, txid)
	if er != nil {
		if Testnet {
			data = utils.GetTestnetTxFromWeb(txid)
		} else {
			data = utils.GetTxFromWeb(txid)
		}
		if data != nil {
			er = nil
		} else {
			er = errors.New("GetRawTx and GetTxFromWeb failed for " + txid.String())
		}
	}
	return
}

func WalletPendingTick() (res bool) {
	mutex_cfg.Lock()
	if WalletOnIn > 0 && BlockChainSynchronized {
		WalletOnIn--
		res = WalletOnIn == 0
	}
	mutex_cfg.Unlock()
	return
}

// Make sure to call it with mutex_cfg locked
func ApplyLTB(hash *btc.Uint256, height uint32) {
	if height != 0 && LastTrustedBlockHeight > height {
		return // Do not apply if older bock than the current one we have
	}
	lastTrustedBlock = hash
	LastTrustedBlockHeight = height

	if hash != nil && BlockChain != nil {
		BlockChain.BlockIndexAccess.Lock()
		node := BlockChain.BlockIndex[hash.BIdx()]
		BlockChain.BlockIndexAccess.Unlock()
		if node != nil {
			LastTrustedBlockHeight = node.Height
			for node != nil {
				node.Trusted.Set()
				node = node.Parent
			}
		}
	}
}

// Make sure to call it with mutex_cfg locked
func ApplyLastTrustedBlock() {
	ApplyLTB(btc.NewUint256FromString(CFG.LastTrustedBlock), 0)
}

func LastTrustedBlockMatch(h *btc.Uint256) (res bool) {
	mutex_cfg.Lock()
	res = lastTrustedBlock != nil && lastTrustedBlock.Equal(h)
	mutex_cfg.Unlock()
	return
}

func AcceptTx() (res bool) {
	mutex_cfg.Lock()
	res = CFG.TXPool.Enabled && BlockChainSynchronized
	mutex_cfg.Unlock()
	return
}

func UpdateScriptFlags(flags uint32) {
	if flags == 0 {
		// We pass timestamp of 0 to always use P2SH flag
		flags = BlockChain.GetBlockFlags(Last.Block.Height, 0)
	}
	atomic.StoreUint32(&Last.ScriptFlags, flags)
}

func CurrentScriptFlags() uint32 {
	return atomic.LoadUint32(&Last.ScriptFlags)
}

func MemUsed() (bts int, alcs int) {
	MemMutex.Lock()
	bts, alcs = Memory.Bytes, Memory.Allocs
	MemMutex.Unlock()
	return
}
