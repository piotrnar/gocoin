package webui

import (
	"sort"
	"encoding/json"
	"github.com/piotrnar/gocoin/client/common"
	"net/http"
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
	type one_var_cnt struct {
		Var string `json:"var"`
		Cnt uint64 `json:"cnt"`
	}
	type one_net_rec struct {
		Var  string `json:"var"`
		Rcvd uint64 `json:"rcvd"`
		Rbts uint64 `json:"rbts"`
		Sent uint64 `json:"sent"`
		Sbts uint64 `json:"sbts"`
	}

	var all_var_cnt struct {
		Gen []*one_var_cnt  `json:"gen"`
		Txs []*one_var_cnt  `json:"txs"`
		Net []*one_net_rec `json:"net"`
	}

	common.CounterMutex.Lock()
	for k, v := range common.Counter {
		if k[4] == '_' {
			var i int
			for i = 0; i < len(all_var_cnt.Net); i++ {
				if all_var_cnt.Net[i].Var == k[5:] {
					break
				}
			}
			if i == len(all_var_cnt.Net) {
				fin := k[5:]
				var nrec one_net_rec
				nrec.Var = fin
				nrec.Rcvd = common.Counter["rcvd_"+fin]
				nrec.Rbts = common.Counter["rbts_"+fin]
				nrec.Sent = common.Counter["sent_"+fin]
				nrec.Sbts = common.Counter["sbts_"+fin]
				all_var_cnt.Net = append(all_var_cnt.Net, &nrec)
			}
		} else if k[:2] == "Tx" {
			all_var_cnt.Txs = append(all_var_cnt.Txs, &one_var_cnt{Var: k[2:], Cnt: v})
		} else {
			all_var_cnt.Gen = append(all_var_cnt.Gen, &one_var_cnt{Var: k, Cnt: v})
		}
	}
	common.CounterMutex.Unlock()
	sort.Slice(all_var_cnt.Gen, func(i, j int) bool {
		return all_var_cnt.Gen[i].Var < all_var_cnt.Gen[j].Var
	})
	sort.Slice(all_var_cnt.Txs, func(i, j int) bool {
		return all_var_cnt.Txs[i].Var < all_var_cnt.Txs[j].Var
	})
	sort.Slice(all_var_cnt.Net, func(i, j int) bool {
		return all_var_cnt.Net[i].Var < all_var_cnt.Net[j].Var
	})

	bx, er := json.Marshal(all_var_cnt)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
	/*
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
	*/
}
