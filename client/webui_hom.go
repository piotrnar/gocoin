package main

import (
	"fmt"
	"time"
	"strings"
	"runtime"
	"net/http"
	"github.com/piotrnar/gocoin/btc"
)

func p_home(w http.ResponseWriter, r *http.Request) {
	s := load_template("home.html")

	mutex.Lock()
	s = strings.Replace(s, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(LastBalance)/1e8), 1)
	s = strings.Replace(s, "{UNSPENT_OUTS}", fmt.Sprint(len(MyBalance)), 1)
	s = strings.Replace(s, "{LAST_BLOCK_HASH}", LastBlock.BlockHash.String(), 1)
	s = strings.Replace(s, "{LAST_BLOCK_HEIGHT}", fmt.Sprint(LastBlock.Height), 1)
	s = strings.Replace(s, "{LAST_BLOCK_TIME}",
		time.Unix(int64(LastBlock.Timestamp), 0).Format("2006/01/02 15:04:05"), 1)
	s = strings.Replace(s, "{LAST_BLOCK_DIFF}", fmt.Sprintf("%.3f", btc.GetDifficulty(LastBlock.Bits)), 1)
	s = strings.Replace(s, "{LAST_BLOCK_RCVD}", time.Now().Sub(LastBlockReceived).String(), 1)
	s = strings.Replace(s, "{BLOCKS_CACHED}", fmt.Sprint(len(cachedBlocks)), 1)
	s = strings.Replace(s, "{KNOWN_PEERS}", fmt.Sprint(peerDB.Count()), 1)
	s = strings.Replace(s, "{NODE_UPTIME}", time.Now().Sub(StartTime).String(), 1)
	s = strings.Replace(s, "{NET_BLOCK_QSIZE}", fmt.Sprint(len(netBlocks)), 1)
	s = strings.Replace(s, "{NET_TX_QSIZE}", fmt.Sprint(len(netTxs)), 1)
	s = strings.Replace(s, "{OPEN_CONNS_TOTAL}", fmt.Sprint(len(openCons)), 1)
	s = strings.Replace(s, "{OPEN_CONNS_OUT}", fmt.Sprint(OutConsActive), 1)
	s = strings.Replace(s, "{OPEN_CONNS_IN}", fmt.Sprint(InConsActive), 1)
	mutex.Unlock()

	bw_mutex.Lock()
	tick_recv()
	tick_sent()
	s = strings.Replace(s, "{DL_SPEED_NOW}", fmt.Sprint(dl_bytes_prv_sec>>10), 1)
	s = strings.Replace(s, "{DL_SPEED_MAX}", fmt.Sprint(DownloadLimit>>10), 1)
	s = strings.Replace(s, "{DL_TOTAL}", bts(dl_bytes_total), 1)
	s = strings.Replace(s, "{UL_SPEED_NOW}", fmt.Sprint(ul_bytes_prv_sec>>10), 1)
	s = strings.Replace(s, "{UL_SPEED_MAX}", fmt.Sprint(UploadLimit>>10), 1)
	s = strings.Replace(s, "{UL_TOTAL}", bts(ul_bytes_total), 1)
	bw_mutex.Unlock()


	ExternalIpMutex.Lock()
	for ip, cnt := range ExternalIp4 {
		s = strings.Replace(s, "{ONE_EXTERNAL_IP}",
			fmt.Sprintf("%dx%d.%d.%d.%d&nbsp;&nbsp;{ONE_EXTERNAL_IP}", cnt,
				byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)), 1)
	}
	ExternalIpMutex.Unlock()
	s = strings.Replace(s, "{ONE_EXTERNAL_IP}", "", 1)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s = strings.Replace(s, "{HEAP_SIZE_MB}", fmt.Sprint(ms.Alloc>>20), 1)
	s = strings.Replace(s, "{SYSMEM_USED_MB}", fmt.Sprint(ms.Sys>>20), 1)
	s = strings.Replace(s, "{ECDSA_VERIFY_COUNT}", fmt.Sprint(btc.EcdsaVerifyCnt), 1)

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
