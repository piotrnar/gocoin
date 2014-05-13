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

	s := load_template("counts.html")

	for i := range gen {
		row := "<tr class=\"hov\">"
		row += fmt.Sprint("<td class=\"gennam\">", gen[i].key, "</td>")
		row += fmt.Sprint("<td class=\"genval\">", gen[i].cnt, "</td>")
		row += "</tr>"
		s = templ_add(s, "<!--GEN_ROW-->", row)
	}

	for i := range txs {
		row := "<tr class=\"hov\">"
		row += fmt.Sprint("<td class=\"tsxnam\">", txs[i].key, "</td>")
		row += fmt.Sprint("<td class=\"txsval\">", txs[i].cnt, "</td>")
		row += "</tr>"
		s = templ_add(s, "<!--TXS_ROW-->", row)
	}

	for i := range net {
		fin := "_"+net[i]
		row := "<tr class=\"hov\">"
		row += fmt.Sprint("<td class=\"netnam\">", net[i], "</td>")
		if cnt:=common.Counter["rcvd"+fin]; cnt>0 {
			row += fmt.Sprint("<td class=\"netbts\">", cnt, "</td>")
			row += fmt.Sprint("<td class=\"netcnt\">", common.Counter["rbts"+fin], "</td>")
		} else {
			row += "<td><td>"
		}
		if cnt:=common.Counter["sent"+fin]; cnt>0 {
			row += fmt.Sprint("<td class=\"netbts\">", cnt, "</td>")
			row += fmt.Sprint("<td class=\"netcnt\">", common.Counter["sbts"+fin], "</td>")
		} else {
			row += "<td><td>"
		}
		if cnt:=common.Counter["hold"+fin]; cnt>0 {
			row += fmt.Sprint("<td class=\"netbts\">", cnt, "</td>")
			row += fmt.Sprint("<td class=\"netcnt\">", common.Counter["hbts"+fin], "</td>")
		} else {
			row += "<td><td>"
		}
		row += "</tr>"
		s = templ_add(s, "<!--NET_ROW-->", row)
	}

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
