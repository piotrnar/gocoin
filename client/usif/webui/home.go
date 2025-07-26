package webui

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	mutexHrate sync.Mutex
	lastHrate  float64
	nextHrate  time.Time
)

func json_status(w http.ResponseWriter, r *http.Request) {
	var out struct {
		Hash                   string
		Diff                   float64
		MinValue               uint64
		Received               int64
		Time_now               int64
		Median                 uint32
		Height                 uint32
		Version                uint32
		Timestamp              uint32
		LastTrustedBlockHeight uint32
		LastHeaderHeight       uint32
		WalletON               bool
		BlockChainSynchronized bool
	}
	common.Last.Mutex.Lock()
	out.Height = common.Last.Block.Height
	out.Hash = common.Last.Block.BlockHash.String()
	out.Timestamp = common.Last.Block.Timestamp()
	out.Received = common.Last.Time.Unix()
	out.Time_now = time.Now().Unix()
	out.Diff = btc.GetDifficulty(common.Last.Block.Bits())
	out.Median = common.Last.Block.GetMedianTimePast()
	out.Version = common.Last.Block.BlockVersion()
	common.Last.Mutex.Unlock()
	out.MinValue = common.AllBalMinVal()
	out.WalletON = common.Get(&common.WalletON)
	out.LastTrustedBlockHeight = common.Get(&common.LastTrustedBlockHeight)
	network.MutexRcv.Lock()
	out.LastHeaderHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()
	out.BlockChainSynchronized = common.Get(&common.BlockChainSynchronized)

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func json_system(w http.ResponseWriter, r *http.Request) {
	var out struct {
		Heap_sysmem        uint64
		Qdb_allocs         int
		BlocksToGet        int
		Known_peers        int
		Node_uptime        uint64
		Net_block_qsize    int
		Net_tx_qsize       int
		Heap_size          uint64
		ProcessPID         int
		Blocks_cached      int
		Qdb_extramem       int
		Ecdsa_verify_cnt   uint64
		Average_block_size int
		Average_fee        float64
		NetworkHashRate    float64
		GC_Total           uint64
		GC_Num             uint32
		GC_Last            uint32
		LastHeaderHeight   uint32
		Blocks_on_disk     uint32
		SavingUTXO         bool
	}

	out.Blocks_cached = network.CachedBlocksLen()
	common.Last.Mutex.Lock()
	if common.Last.ParseTill != nil {
		out.Blocks_on_disk = common.Last.ParseTill.Height - common.Last.Block.Height
	}
	common.Last.Mutex.Unlock()
	out.BlocksToGet = network.BlocksToGetCnt()
	out.Known_peers = peersdb.PeerDB.Count()
	out.Node_uptime = uint64(time.Since(common.StartTime).Seconds())
	out.Net_block_qsize = len(network.NetBlocks)
	out.Net_tx_qsize = len(network.NetTxs)
	out.Qdb_extramem, out.Qdb_allocs = common.MemUsed()
	out.Ecdsa_verify_cnt = btc.EcdsaVerifyCnt()
	out.Average_block_size = common.AverageBlockSize.Get()
	out.Average_fee = usif.GetAverageFee()
	network.MutexRcv.Lock()
	out.LastHeaderHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()

	mutexHrate.Lock()
	if nextHrate.IsZero() || time.Now().After(nextHrate) {
		lastHrate = usif.GetNetworkHashRateNum()
		nextHrate = time.Now().Add(time.Minute)
	}
	out.NetworkHashRate = lastHrate
	mutexHrate.Unlock()

	out.SavingUTXO = common.BlockChain.Unspent.WritingInProgress.Get()
	out.ProcessPID = os.Getpid()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	out.Heap_size, out.Heap_sysmem = ms.Alloc, ms.Sys
	out.GC_Num = ms.NumGC
	out.GC_Last = uint32(ms.LastGC / uint64(time.Second))
	out.GC_Total = ms.PauseTotalNs

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
