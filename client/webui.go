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
 | <a href="/counts">Counters</a>
 | <a href="/blocks">Blocks</a>
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
	fmt.Fprint(w, "<pre>")
	fmt.Fprintln(w, busy)
	mutex.Lock()
	fmt.Fprintln(w, "LastBlock:", LastBlock.BlockHash.String())
	fmt.Fprintf(w, "Height: %d @ %s,  Diff: %.0f,  Got: %s ago\n",
		LastBlock.Height,
		time.Unix(int64(LastBlock.Timestamp), 0).Format("2006/01/02 15:04:05"),
		btc.GetDifficulty(LastBlock.Bits), time.Now().Sub(LastBlockReceived).String())
	fmt.Fprintf(w, "BlocksCached: %d,  BlocksPending: %d/%d,  NetQueueSize: %d,  NetConns: %d,  Peers: %d\n",
		len(cachedBlocks), len(pendingBlocks), len(pendingFifo), len(netBlocks), len(openCons),
		peerDB.Count())
	mutex.Unlock()
	fmt.Fprint(w, "</pre>")
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
	fmt.Fprintf(w, "<tr><th>Height<th>Timestamp<th>Hash<th>Txs<th>Size<th>Difficulty</tr>\n")
	for cnt:=0; end!=nil && cnt<40; cnt++ {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		block, e := btc.NewBlock(bl)
		if e!=nil {
			return
		}
		block.BuildTxList()
		fmt.Fprintf(w, "<tr class=\"hov\"><td>%d<td>%s<td class=\"mono\">%s<td>%d<td>%d<td>%.2f</tr>\n",
			end.Height, time.Unix(int64(block.BlockTime), 0).Format("2006-01-02 15:04:05"),
			end.BlockHash.String(), len(block.Txs), len(bl),
			btc.GetDifficulty(block.Bits))

		end = end.Parent
	}
	fmt.Fprint(w, "</table>")
	write_html_tail(w)
}

func webui() {
	http.HandleFunc("/webui/", p_webui)
	http.HandleFunc("/", p_home)
	http.HandleFunc("/blocks", p_blocks)
	http.HandleFunc("/counts", p_counts)
	http.ListenAndServe("127.0.0.1:8833", nil)
}
