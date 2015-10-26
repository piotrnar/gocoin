package webui

import (
	"fmt"
	"time"
	"strings"
	"net/http"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


func p_home(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	// The handler also gets called for /favicon.ico
	if r.URL.Path!="/" {
		http.NotFound(w, r)
	}

	s := load_template("home.html")

	wallet.BalanceMutex.Lock()
	if len(wallet.MyBalance)>0 {
		wal := load_template("home_wal.html")
		wal = strings.Replace(wal, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(wallet.LastBalance)/1e8), 1)
		wal = strings.Replace(wal, "{UNSPENT_OUTS}", fmt.Sprint(len(wallet.MyBalance)), 1)
		s = strings.Replace(s, "<!--WALLET-->", wal, 1)
	} else {
		if wallet.MyWallet==nil {
			s = strings.Replace(s, "<!--WALLET-->", "You have no wallet", 1)
		} else {
			s = strings.Replace(s, "<!--WALLET-->", "Your balance is <b>zero</b>", 1)
		}
	}
	wallet.BalanceMutex.Unlock()

	s = strings.Replace(s, "<--NETWORK_HASHRATE-->", usif.GetNetworkHashRate(), 1)

	common.LockCfg()
	dat, _ := json.Marshal(&common.CFG)
	common.UnlockCfg()
	s = strings.Replace(s, "{CONFIG_FILE}", strings.Replace(string(dat), ",\"", ", \"", -1), 1)

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
	}
	common.Last.Mutex.Lock()
	out.Height = common.Last.Block.Height
	out.Hash =  common.Last.Block.BlockHash.String()
	out.Timestamp =  common.Last.Block.Timestamp()
	out.Received =  common.Last.Time.Unix()
	out.Time_now =  time.Now().Unix()
	out.Diff =  btc.GetDifficulty(common.Last.Block.Bits())
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
		Known_peers int
		Node_uptime uint64
		Net_block_qsize int
		Net_tx_qsize int
		Heap_size uint64
		Heap_sysmem uint64
		Qdb_extramem int64
		Ecdsa_verify_cnt uint64
	}

	out.Blocks_cached = len(network.CachedBlocks)
	out.Known_peers = peersdb.PeerDB.Count()
	out.Node_uptime = uint64(time.Now().Sub(common.StartTime).Seconds())
	out.Net_block_qsize = len(network.NetBlocks)
	out.Net_tx_qsize = len(network.NetTxs)
	out.Heap_size, out.Heap_sysmem = sys.MemUsed()
	out.Qdb_extramem = qdb.ExtraMemoryConsumed
	out.Ecdsa_verify_cnt = btc.EcdsaVerifyCnt

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
