package webui

import (
//	"os"
	"fmt"
	"sort"
//	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/client/common"
)

type many_counters []one_counter

type one_counter struct {
	key string
	cnt uint64
}

func (c many_counters) Len() int {
	return len(c)
}

func (c many_counters) Less(i, j int) bool {
	return c[i].key < c[j].key
}

func (c many_counters) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func p_counts(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	s := load_template("counts.html")
	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}


func json_counts(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	var net []string
	var gen, txs many_counters
	common.CounterMutex.Lock()
	for k, v := range common.Counter {
		if k[4]=='_' {
			var i int
			for i=0; i<len(net); i++ {
				if net[i]==k[5:] {
					break
				}
			}
			if i==len(net) {
				net = append(net, k[5:])
			}
		} else if k[:2]=="Tx" {
			txs = append(txs, one_counter{key:k[2:], cnt:v})
		} else {
			gen = append(gen, one_counter{key:k, cnt:v})
		}
	}
	common.CounterMutex.Unlock()
	sort.Sort(gen)
	sort.Sort(txs)
	sort.Strings(net)

	w.Header()["Content-Type"] = []string{"application/json"}
	w.Write([]byte("{\n"))

	w.Write([]byte(" \"gen\":["))
	for i := range gen {
		w.Write([]byte(fmt.Sprint("{\"var\":\"", gen[i].key, "\",\"cnt\":", gen[i].cnt, "}")))
		if i<len(gen)-1 {
			w.Write([]byte(","))
		}
	}
	w.Write([]byte("],\n \"txs\":["))

	for i := range txs {
		w.Write([]byte(fmt.Sprint("{\"var\":\"", txs[i].key, "\",\"cnt\":", txs[i].cnt, "}")))
		if i<len(txs)-1 {
			w.Write([]byte(","))
		}
	}
	w.Write([]byte("],\n \"net\":["))

	for i := range net {
		fin := "_"+net[i]
		w.Write([]byte("{\"var\":\"" + net[i] + "\","))
		common.CounterMutex.Lock()
		w.Write([]byte(fmt.Sprint("\"rcvd\":", common.Counter["rcvd"+fin], ",")))
		w.Write([]byte(fmt.Sprint("\"rbts\":", common.Counter["rbts"+fin], ",")))
		w.Write([]byte(fmt.Sprint("\"sent\":", common.Counter["sent"+fin], ",")))
		w.Write([]byte(fmt.Sprint("\"sbts\":", common.Counter["sbts"+fin], "}")))
		common.CounterMutex.Unlock()
		if i<len(net)-1 {
			w.Write([]byte(","))
		}
	}
	w.Write([]byte("]\n}\n"))
}
