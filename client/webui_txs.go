package main

import (
	"fmt"
	"time"
	"sync"
	"strings"
	"net/http"
	"io/ioutil"
	"github.com/piotrnar/gocoin/btc"
)

func p_txs(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var txloadresult string
	var wg sync.WaitGroup

	// Check if there is a tx upload request
	r.ParseMultipartForm(2e6)
	fil, _, _ := r.FormFile("txfile")
	if fil != nil {
		tx2in, _ := ioutil.ReadAll(fil)
		if len(tx2in)>0 {
			wg.Add(1)
			req := &oneUiReq{param:string(tx2in)}
			req.done.Add(1)
			req.handler = func(dat string) {
				txloadresult = load_raw_tx([]byte(dat))
				wg.Done()
			}
			uiChannel <- req
		}
	}


	s := load_template("txs.html")
	tx_mutex.Lock()

	var sum uint64
	for _, v := range TransactionsToSend {
		sum += uint64(len(v.data))
	}
	s = strings.Replace(s, "{T2S_CNT}", fmt.Sprint(len(TransactionsToSend)), 1)
	s = strings.Replace(s, "{T2S_SIZE}", bts(sum), 1)

	sum = 0
	for _, v := range TransactionsRejected {
		sum += uint64(v.size)
	}
	s = strings.Replace(s, "{TRE_CNT}", fmt.Sprint(len(TransactionsRejected)), 1)
	s = strings.Replace(s, "{TRE_SIZE}", bts(sum), 1)
	s = strings.Replace(s, "{PTR1_CNT}", fmt.Sprint(len(TransactionsPending)), 1)
	s = strings.Replace(s, "{PTR2_CNT}", fmt.Sprint(len(netTxs)), 1)
	s = strings.Replace(s, "{SPENT_OUTS_CNT}", fmt.Sprint(len(SpentOutputs)), 1)
	s = strings.Replace(s, "{AWAITING_INPUTS}", fmt.Sprint(len(WaitingForInputs)), 1)

	tx_mutex.Unlock()

	wg.Wait()
	if txloadresult!="" {
		ld := load_template("txs_load.html")
		ld = strings.Replace(ld, "{TX_RAW_DATA}", txloadresult, 1)
		s = strings.Replace(s, "<!--TX_LOAD-->", ld, 1)
	}

	if CFG.TXPool.Enabled {
		s = strings.Replace(s, "<!--MEM_POOL_ENABLED-->", "Enabled", 1)
	} else {
		s = strings.Replace(s, "<!--MEM_POOL_ENABLED-->", "Disabled", 1)
	}

	if CFG.TXRoute.Enabled {
		s = strings.Replace(s, "<!--TX_ROUTE_ENABLED-->", "Enabled", 1)
	} else {
		s = strings.Replace(s, "<!--TX_ROUTE_ENABLED-->", "Disabled", 1)
	}

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}

func xmp_txs2s(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	r.ParseForm()

	if checksid(r) && len(r.Form["del"])>0 {
		tid := btc.NewUint256FromString(r.Form["del"][0])
		if tid!=nil {
			tx_mutex.Lock()
			delete(TransactionsToSend, tid.Hash)
			tx_mutex.Unlock()
		}
	}

	if checksid(r) && len(r.Form["send"])>0 {
		tid := btc.NewUint256FromString(r.Form["send"][0])
		if tid!=nil {
			tx_mutex.Lock()
			if ptx, ok := TransactionsToSend[tid.Hash]; ok {
				tx_mutex.Unlock()
				cnt := NetRouteInv(1, tid, nil)
				ptx.sentcnt += cnt
				ptx.lastsent = time.Now()
			}
		}
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<txpool>"))
	tx_mutex.Lock()
	for k, v := range TransactionsToSend {
		w.Write([]byte("<tx>"))
		fmt.Fprint(w, "<id>", btc.NewUint256(k[:]).String(), "</id>")
		fmt.Fprint(w, "<time>", v.firstseen.Unix(), "</time>")
		fmt.Fprint(w, "<len>", len(v.data), "</len>")
		fmt.Fprint(w, "<own>", v.own, "</own>")
		fmt.Fprint(w, "<sentcnt>", v.sentcnt, "</sentcnt>")
		fmt.Fprint(w, "<sentlast>", v.lastsent.Unix(), "</sentlast>")
		fmt.Fprint(w, "<volume>", v.volume, "</volume>")
		fmt.Fprint(w, "<fee>", v.fee, "</fee>")
		fmt.Fprint(w, "<blocked>", v.blocked, "</blocked>")
		w.Write([]byte("</tx>"))
	}
	tx_mutex.Unlock()
	w.Write([]byte("</txpool>"))
}


func xml_txsre(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<txbanned>"))
	tx_mutex.Lock()
	for _, v := range TransactionsRejected {
		w.Write([]byte("<tx>"))
		fmt.Fprint(w, "<id>", v.id.String(), "</id>")
		fmt.Fprint(w, "<time>", v.Time.Unix(), "</time>")
		fmt.Fprint(w, "<len>", v.size, "</len>")
		fmt.Fprint(w, "<reason>", v.reason, "</reason>")
		w.Write([]byte("</tx>"))
	}
	tx_mutex.Unlock()
	w.Write([]byte("</txbanned>"))
}


func xml_txw4i(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<pending>"))
	tx_mutex.Lock()
	for _, v := range WaitingForInputs {
		w.Write([]byte("<wait4>"))
		fmt.Fprint(w, "<id>", v.TxID.String(), "</id>")
		for x, t := range v.Ids {
			w.Write([]byte("<tx>"))
			if v, ok := TransactionsRejected[x]; ok {
				fmt.Fprint(w, "<id>", v.id.String(), "</id>")
				fmt.Fprint(w, "<time>", t.Unix(), "</time>")
			} else {
				fmt.Fprint(w, "<id>FATAL ERROR!!! This should not happen! Please report</id>")
				fmt.Fprint(w, "<time>", time.Now().Unix(), "</time>")
			}
			w.Write([]byte("</tx>"))
		}
		w.Write([]byte("</wait4>"))
	}
	tx_mutex.Unlock()
	w.Write([]byte("</pending>"))
}
