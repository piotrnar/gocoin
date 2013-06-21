package main

import (
	"fmt"
	"time"
	"sort"
	"strings"
	"net/http"
	"io/ioutil"
	"github.com/piotrnar/gocoin/btc"
)

const page_head = `<html><head><title>Gocoin `+btc.SourcesTag+`</title>
<script type="text/javascript" src="webui/gocoin.js"></script>
<link rel="stylesheet" href="webui/gocoin.css" type="text/css">
</head><body>
<table align="center" width="990" cellpadding="0" cellspacing="0"><tr><td>
<table width="100%"><tr>
<td align="left"><a href="/">Home</a>
 | <a href="/net">Network</a>
 | <a href="/blocks">Blocks</a>
 | <a href="/miners">Miners</a>
<td align="right"><a href="/counts">Counters</a>
</table><hr>
`

const page_tail = `</table></body></html>`


func p_webui(w http.ResponseWriter, r *http.Request) {
	if len(strings.SplitN(r.URL.Path[1:], "/", 3))==2 {
		dat, _ := ioutil.ReadFile(r.URL.Path[1:])
		w.Write(dat)
	}
}

func write_html_head(w http.ResponseWriter) {
	w.Write([]byte(page_head))
}

func write_html_tail(w http.ResponseWriter) {
	fmt.Fprint(w, "</body></html>")
}

func p_home(w http.ResponseWriter, r *http.Request) {
	write_html_head(w)
	fmt.Fprint(w, "<h1>Home</h1>")

	fmt.Fprint(w, "<h2>Wallet</h2>")
	fmt.Fprintf(w, "Last known balance: <b>%.8f</b> BTC in <b>%d</b> outputs<br>\n",
		float64(LastBalance)/1e8, len(MyBalance))

	fmt.Fprint(w, "<h2>Last Block</h2>")
	mutex.Lock()
	fmt.Fprintln(w, "<table>")
	fmt.Fprintf(w, "<tr><td>Hash:<td><b>%s</b>\n", LastBlock.BlockHash.String())
	fmt.Fprintf(w, "<tr><td>Height:<td><b>%d</b>\n", LastBlock.Height)
	fmt.Fprintf(w, "<tr><td>Timestamp:<td><b>%s</b>\n",
		time.Unix(int64(LastBlock.Timestamp), 0).Format("2006/01/02 15:04:05"))
	fmt.Fprintf(w, "<tr><td>Difficulty:<td><b>%.3f</b>\n", btc.GetDifficulty(LastBlock.Bits))
	fmt.Fprintf(w, "<tr><td>Received:<td><b>%s</b> ago\n", time.Now().Sub(LastBlockReceived).String())
	fmt.Fprintln(w, "</table>")
	mutex.Unlock()

	fmt.Fprint(w, "<h2>Network</h2>")
	fmt.Fprintln(w, "<table>")
	bw_mutex.Lock()
	tick_recv()
	tick_sent()
	fmt.Fprintf(w, "<tr><td>Downloading at:<td><b>%d/%d</b> KB/s, <b>%s</b> total\n",
		dl_bytes_prv_sec>>10, DownloadLimit>>10, bts(dl_bytes_total))
	fmt.Fprintf(w, "<tr><td>Uploading at:<td><b>%d/%d</b> KB/s, <b>%s</b> total\n",
		ul_bytes_prv_sec>>10, UploadLimit>>10, bts(ul_bytes_total))
	bw_mutex.Unlock()
	fmt.Fprintf(w, "<tr><td>Net Queue Size:<td><b>%d</b>\n", len(netBlocks))
	fmt.Fprintf(w, "<tr><td>Open Connections:<td><b>%d</b> (<b>%d</b> outgoing + <b>%d</b> incomming)\n",
		len(openCons), OutConsActive, InConsActive)
	fmt.Fprint(w, "<tr><td>Extrenal IPs:<td>")
	for ip, cnt := range ExternalIp4 {
		fmt.Fprintf(w, "%d.%d.%d.%d (%d)&nbsp;&nbsp;", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), cnt)
	}
	fmt.Fprintln(w, "</table>")


	fmt.Fprint(w, "<h2>Others</h2>")
	fmt.Fprintln(w, "<table>")
	fmt.Fprintf(w, "<tr><td>Blocks Cached:<td><b>%d</b>\n", len(cachedBlocks))
	fmt.Fprintf(w, "<tr><td>Blocks Pending:<td><b>%d/%d</b>\n", len(pendingBlocks), len(pendingFifo))
	fmt.Fprintf(w, "<tr><td>Known Peers:<td><b>%d</b>\n", peerDB.Count())
	fmt.Fprintf(w, "<tr><td>Node's uptime:<td><b>%s</b>\n", time.Now().Sub(StartTime).String())
	fmt.Fprintln(w, "</table>")
	write_html_tail(w)
}

func p_net(w http.ResponseWriter, r *http.Request) {
	write_html_head(w)
	mutex.Lock()
	srt := make(sortedkeys, len(openCons))
	cnt := 0
	for k, v := range openCons {
		srt[cnt].key = k
		srt[cnt].ConnID = v.ConnID
		cnt++
	}
	sort.Sort(srt)
	fmt.Fprint(w, "<h1>Network</h1>")
	fmt.Fprintln(w, "<table class=\"netcons\" border=\"1\" cellspacing=\"0\" cellpadding=\"0\">")
	fmt.Fprint(w, "<tr><th>ID<th colspan=\"2\">IP<th>Ping<th colspan=\"2\">Last Rcvd<th colspan=\"2\">Last Sent")
	fmt.Fprintln(w, "<th>Total Rcvd<th>Total Sent<th colspan=\"2\">Version<th>Sending")
	for idx := range srt {
		v := openCons[srt[idx].key]
		fmt.Fprintf(w, "<tr class=\"hov\"><td align=\"right\">%d", v.ConnID)
		if v.Incomming {
			fmt.Fprint(w, "<td aling=\"center\">From")
		} else {
			fmt.Fprint(w, "<td aling=\"center\">To")
		}
		fmt.Fprint(w, "<td align=\"right\">", v.PeerAddr.Ip())
		fmt.Fprint(w, "<td align=\"right\">", v.GetAveragePing(), "ms")
		fmt.Fprint(w, "<td align=\"right\">", v.LastBtsRcvd)
		fmt.Fprint(w, "<td class=\"mono\">", v.LastCmdRcvd)
		fmt.Fprint(w, "<td align=\"right\">", v.LastBtsSent)
		fmt.Fprint(w, "<td class=\"mono\">", v.LastCmdSent)
		fmt.Fprint(w, "<td align=\"right\">", bts(v.BytesReceived))
		fmt.Fprint(w, "<td align=\"right\">", bts(v.BytesSent))
		fmt.Fprint(w, "<td align=\"right\">", v.node.version)
		fmt.Fprint(w, "<td>", v.node.agent)
		fmt.Fprintf(w, "<td align=\"right\">%d/%d", v.send.sofar, len(v.send.buf))
	}
	fmt.Fprintln(w, "</table><br>")
	fmt.Fprintln(w, OutConsActive, "outgoing and", InConsActive, "incomming connections")
	mutex.Unlock()
	write_html_tail(w)
}

func p_counts(w http.ResponseWriter, r *http.Request) {
	write_html_head(w)
	fmt.Fprint(w, "<h1>Counters</h1>")
	counter_mutex.Lock()
	ck := make([]string, 0)
	for k, _ := range Counter {
		ck = append(ck, k)
	}
	sort.Strings(ck)
	fmt.Fprint(w, "<table class=\"mono\">")
	for i := range ck {
		fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td></tr>\n", ck[i], Counter[ck[i]])
	}
	fmt.Fprint(w, "</table>")
	counter_mutex.Unlock()
	write_html_tail(w)
}

func p_blocks(w http.ResponseWriter, r *http.Request) {
	write_html_head(w)
	fmt.Fprint(w, "<h1>Blocks</h1>")
	end := BlockChain.BlockTreeEnd
	fmt.Fprint(w, "<table border=\"1\" cellspacing=\"0\" cellpadding=\"0\">\n")
	fmt.Fprintf(w, "<tr><th>Height<th>Timestamp<th>Hash<th>Txs<th>Size<th>Mined by</tr>\n")
	for cnt:=0; end!=nil && cnt<100; cnt++ {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		block, e := btc.NewBlock(bl)
		if e!=nil {
			return
		}
		block.BuildTxList()
		miner := blocks_miner(bl)
		fmt.Fprintf(w, "<tr class=\"hov\"><td>%d<td>%s", end.Height,
			time.Unix(int64(block.BlockTime), 0).Format("2006-01-02 15:04:05"))
		fmt.Fprintf(w, "<td><a class=\"mono\" href=\"http://blockchain.info/block/%s\">%s",
			end.BlockHash.String(), end.BlockHash.String())
		fmt.Fprintf(w, "<td align=\"right\">%d<td align=\"right\">%d<td>%s</tr>\n", len(block.Txs), len(bl), miner)
		end = end.Parent
	}
	fmt.Fprint(w, "</table>")
	write_html_tail(w)
}

type onemiernstat []struct {
	name string
	cnt int
}

func (x onemiernstat) Len() int {
	return len(x)
}

func (x onemiernstat) Less(i, j int) bool {
	return x[i].cnt > x[j].cnt
}

func (x onemiernstat) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func p_miners(w http.ResponseWriter, r *http.Request) {
	write_html_head(w)
	fmt.Fprint(w, "<h1>Miners of the last 144 blocks</h1>")
	m := make(map[string]int, 20)
	cnt, unkn := 0, 0
	end := BlockChain.BlockTreeEnd
	for ; end!=nil && cnt<144; cnt++ {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		miner := blocks_miner(bl)
		if miner!="" {
			m[miner]++
		} else {
			unkn++
		}
		end = end.Parent
	}
	srt := make(onemiernstat, len(m))
	i := 0
	for k, v := range m {
		srt[i].name = k
		srt[i].cnt = v
		i++
	}
	sort.Sort(srt)
	fmt.Fprint(w, "<table border=\"1\" cellspacing=\"0\" cellpadding=\"0\">\n")
	fmt.Fprint(w, "<tr><th>Miner<th>Blocks<th>Share</tr>\n")
	for i := range srt {
		fmt.Fprintf(w, "<tr class=\"hov\"><td>%s<td align=\"right\">%d<td align=\"right\">%.0f%%</tr>\n",
			srt[i].name, srt[i].cnt, 100*float64(srt[i].cnt)/float64(cnt))
	}
	fmt.Fprintf(w, "<tr class=\"hov\"><td><i>Unknown</i><td align=\"right\">%d<td align=\"right\">%.0f%%</tr>\n",
		unkn, 100*float64(unkn)/float64(cnt))
	fmt.Fprint(w, "</table>")
	write_html_tail(w)
}

func webserver() {
	http.HandleFunc("/webui/", p_webui)
	http.HandleFunc("/", p_home)
	http.HandleFunc("/net", p_net)
	http.HandleFunc("/blocks", p_blocks)
	http.HandleFunc("/miners", p_miners)
	http.HandleFunc("/counts", p_counts)
	http.ListenAndServe(*webui, nil)
}
