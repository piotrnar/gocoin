package webui

import (
	"fmt"
	"time"
	"strings"
	"runtime"
	"net/http"
	"encoding/json"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
)


func p_home(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	s := load_template("home.html")

	wallet.LockBal()
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
	wallet.UnlockBal()

	common.Last.Mutex.Lock()
	s = strings.Replace(s, "{LAST_BLOCK_HASH}", common.Last.Block.BlockHash.String(), 1)
	s = strings.Replace(s, "{LAST_BLOCK_HEIGHT}", fmt.Sprint(common.Last.Block.Height), 1)
	s = strings.Replace(s, "{LAST_BLOCK_TIME}", time.Unix(int64(common.Last.Block.Timestamp()), 0).Format("2006/01/02 15:04:05"), 1)
	s = strings.Replace(s, "{LAST_BLOCK_DIFF}", common.NumberToString(btc.GetDifficulty(common.Last.Block.Bits())), 1)
	s = strings.Replace(s, "{LAST_BLOCK_RCVD}", time.Now().Sub(common.Last.Time).String(), 1)
	common.Last.Mutex.Unlock()

	s = strings.Replace(s, "{BLOCKS_CACHED}", fmt.Sprint(len(network.CachedBlocks)), 1)
	s = strings.Replace(s, "{KNOWN_PEERS}", fmt.Sprint(network.PeerDB.Count()), 1)
	s = strings.Replace(s, "{NODE_UPTIME}", time.Now().Sub(common.StartTime).String(), 1)
	s = strings.Replace(s, "{NET_BLOCK_QSIZE}", fmt.Sprint(len(network.NetBlocks)), 1)
	s = strings.Replace(s, "{NET_TX_QSIZE}", fmt.Sprint(len(network.NetTxs)), 1)

	network.Mutex_net.Lock()
	s = strings.Replace(s, "{OPEN_CONNS_TOTAL}", fmt.Sprint(len(network.OpenCons)), 1)
	s = strings.Replace(s, "{OPEN_CONNS_OUT}", fmt.Sprint(network.OutConsActive), 1)
	s = strings.Replace(s, "{OPEN_CONNS_IN}", fmt.Sprint(network.InConsActive), 1)
	network.Mutex_net.Unlock()

	common.LockBw()
	common.TickRecv()
	common.TickSent()
	s = strings.Replace(s, "{DL_SPEED_NOW}", fmt.Sprint(common.DlBytesPrevSec>>10), 1)
	s = strings.Replace(s, "{DL_SPEED_MAX}", fmt.Sprint(common.DownloadLimit>>10), 1)
	s = strings.Replace(s, "{DL_TOTAL}", common.BytesToString(common.DlBytesTotal), 1)
	s = strings.Replace(s, "{UL_SPEED_NOW}", fmt.Sprint(common.UlBytesPrevSec>>10), 1)
	s = strings.Replace(s, "{UL_SPEED_MAX}", fmt.Sprint(common.UploadLimit>>10), 1)
	s = strings.Replace(s, "{UL_TOTAL}", common.BytesToString(common.UlBytesTotal), 1)
	common.UnlockBw()


	network.ExternalIpMutex.Lock()
	for ip, rec := range network.ExternalIp4 {
		ips := fmt.Sprintf("<b title=\"%d times. Last seen %d min ago\">%d.%d.%d.%d</b> ",
				rec[0], (uint(time.Now().Unix())-rec[1])/60,
				byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
		s = templ_add(s, "<!--ONE_EXTERNAL_IP-->", ips)
	}
	network.ExternalIpMutex.Unlock()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s = strings.Replace(s, "{HEAP_SIZE_MB}", fmt.Sprint(ms.Alloc>>20), 1)
	s = strings.Replace(s, "<!--HEAPSYS_MB-->", fmt.Sprint(ms.HeapInuse>>20), 1)
	s = strings.Replace(s, "{SYSMEM_USED_MB}", fmt.Sprint(ms.Sys>>20), 1)
	s = strings.Replace(s, "{ECDSA_VERIFY_COUNT}", fmt.Sprint(btc.EcdsaVerifyCnt), 1)

	common.LockCfg()
	dat, _ := json.Marshal(&common.CFG)
	common.UnlockCfg()
	s = strings.Replace(s, "{CONFIG_FILE}", strings.Replace(string(dat), ",\"", ", \"", -1), 1)

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
