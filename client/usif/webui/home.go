package webui

import (
	"time"
	"sync"
	"strings"
	"net/http"
	"sync/atomic"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)

var (
	mutexHrate sync.Mutex
	lastHrate float64
	nextHrate time.Time
)


func p_home(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	// The handler also gets called for /favicon.ico
	if r.URL.Path!="/" {
		http.NotFound(w, r)
		return
	}

	s := load_template("home.html")

	if !common.CFG.WebUI.ServerMode {
		common.LockCfg()
		dat, _ := json.Marshal(&common.CFG)
		common.UnlockCfg()
		s = strings.Replace(s, "{CONFIG_FILE}", strings.Replace(string(dat), ",\"", ", \"", -1), 1)
	}

	s = strings.Replace(s, "<!--PUB_AUTH_KEY-->", common.PublicKey, 1)

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}


func json_status(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		Height uint32
		Hash string
		Timestamp uint32
		Received int64
		Time_now int64
		Diff float64
		Median uint32
		Version uint32
		MinValue uint64
		NoWallet bool
	}
	common.Last.Mutex.Lock()
	out.Height = common.Last.Block.Height
	out.Hash =  common.Last.Block.BlockHash.String()
	out.Timestamp =  common.Last.Block.Timestamp()
	out.Received =  common.Last.Time.Unix()
	out.Time_now =  time.Now().Unix()
	out.Diff =  btc.GetDifficulty(common.Last.Block.Bits())
	out.Median =  common.Last.Block.GetMedianTimePast()
	out.Version = common.Last.Block.BlockVersion()
	out.MinValue = atomic.LoadUint64(&common.CFG.AllBalances.MinValue)
	out.NoWallet = common.GetBool(&common.FLAG.NoWallet)
	common.Last.Mutex.Unlock()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}


func json_system(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		Blocks_cached int
		BlocksToGet int
		Known_peers int
		Node_uptime uint64
		Net_block_qsize int
		Net_tx_qsize int
		Heap_size uint64
		Heap_sysmem uint64
		Qdb_extramem int64
		Ecdsa_verify_cnt uint64
		Average_block_size int
		Average_fee float64
		LastHeaderHeight uint32
		NetworkHashRate float64
		SavingUTXO bool
	}

	out.Blocks_cached = network.CachedBlocksLen.Get()
	network.MutexRcv.Lock()
	out.BlocksToGet = len(network.BlocksToGet)
	network.MutexRcv.Unlock()
	out.Known_peers = peersdb.PeerDB.Count()
	out.Node_uptime = uint64(time.Now().Sub(common.StartTime).Seconds())
	out.Net_block_qsize = len(network.NetBlocks)
	out.Net_tx_qsize = len(network.NetTxs)
	out.Heap_size, out.Heap_sysmem = sys.MemUsed()
	out.Qdb_extramem = utxo.ExtraMemoryConsumed()
	out.Ecdsa_verify_cnt = btc.EcdsaVerifyCnt()
	out.Average_block_size = common.AverageBlockSize.Get()
	out.Average_fee = common.GetAverageFee()
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

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
