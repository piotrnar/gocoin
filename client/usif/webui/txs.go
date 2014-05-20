package webui

import (
	"fmt"
	"time"
	"sync"
	"strings"
	"net/http"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif"
)

func p_txs(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var txloadresult string
	var wg sync.WaitGroup
	var tx2in []byte

	// Check if there is a tx upload request
	r.ParseMultipartForm(2e6)
	fil, _, _ := r.FormFile("txfile")
	if fil != nil {
		tx2in, _ = ioutil.ReadAll(fil)
	} else if len(r.Form["rawtx"])==1 {
		tx2in, _ = hex.DecodeString(r.Form["rawtx"][0])
	}

	if len(tx2in)>0 {
		wg.Add(1)
		req := &usif.OneUiReq{Param:string(tx2in)}
		req.Done.Add(1)
		req.Handler = func(dat string) {
			txloadresult = usif.LoadRawTx([]byte(dat))
			wg.Done()
		}
		usif.UiChannel <- req
	}

	s := load_template("txs.html")
	network.TxMutex.Lock()

	var sum uint64
	for _, v := range network.TransactionsToSend {
		sum += uint64(len(v.Data))
	}
	s = strings.Replace(s, "{T2S_CNT}", fmt.Sprint(len(network.TransactionsToSend)), 1)
	s = strings.Replace(s, "{T2S_SIZE}", common.BytesToString(sum), 1)

	sum = 0
	for _, v := range network.TransactionsRejected {
		sum += uint64(v.Size)
	}
	s = strings.Replace(s, "{TRE_CNT}", fmt.Sprint(len(network.TransactionsRejected)), 1)
	s = strings.Replace(s, "{TRE_SIZE}", common.BytesToString(sum), 1)
	s = strings.Replace(s, "{PTR1_CNT}", fmt.Sprint(len(network.TransactionsPending)), 1)
	s = strings.Replace(s, "{PTR2_CNT}", fmt.Sprint(len(network.NetTxs)), 1)
	s = strings.Replace(s, "{SPENT_OUTS_CNT}", fmt.Sprint(len(network.SpentOutputs)), 1)
	s = strings.Replace(s, "{AWAITING_INPUTS}", fmt.Sprint(len(network.WaitingForInputs)), 1)

	network.TxMutex.Unlock()

	wg.Wait()
	if txloadresult!="" {
		ld := load_template("txs_load.html")
		ld = strings.Replace(ld, "{TX_RAW_DATA}", txloadresult, 1)
		s = strings.Replace(s, "<!--TX_LOAD-->", ld, 1)
	}

	if common.CFG.TXPool.Enabled {
		s = strings.Replace(s, "<!--MEM_POOL_ENABLED-->", "Enabled", 1)
	} else {
		s = strings.Replace(s, "<!--MEM_POOL_ENABLED-->", "Disabled", 1)
	}

	if common.CFG.TXRoute.Enabled {
		s = strings.Replace(s, "<!--TX_ROUTE_ENABLED-->", "Enabled", 1)
	} else {
		s = strings.Replace(s, "<!--TX_ROUTE_ENABLED-->", "Disabled", 1)
	}

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}


func output_tx_xml(w http.ResponseWriter, id string) {
	txid := btc.NewUint256FromString(id)
	w.Write([]byte("<tx>"))
	fmt.Fprint(w, "<id>", id, "</id>")
	if t2s, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		w.Write([]byte("<status>OK</status>"))
		tx := t2s.Tx
		w.Write([]byte("<inputs>"))
		for i := range tx.TxIn {
			w.Write([]byte("<input>"))
			var po *btc.TxOut
			inpid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
			if txinmem, ok := network.TransactionsToSend[inpid.BIdx()]; ok {
				if int(tx.TxIn[i].Input.Vout) < len(txinmem.TxOut) {
					po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
				}
			} else {
				po, _ = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			}
			if po != nil {
				ok := script.VerifyTxScript(tx.TxIn[i].ScriptSig, po.Pk_script, i, tx, true)
				if !ok {
					w.Write([]byte("<status>Script FAILED</status>"))
				} else {
					w.Write([]byte("<status>OK</status>"))
				}
				fmt.Fprint(w, "<value>", po.Value, "</value>")
				ads := "???"
				if ad := btc.NewAddrFromPkScript(po.Pk_script, common.Testnet); ad != nil {
					ads = ad.String()
				}
				fmt.Fprint(w, "<addr>", ads, "</addr>")
				fmt.Fprint(w, "<block>", po.BlockHeight, "</block>")
			} else {
				w.Write([]byte("<status>UNKNOWN INPUT</status>"))
			}
			w.Write([]byte("</input>"))
		}
		w.Write([]byte("</inputs>"))

		w.Write([]byte("<outputs>"))
		for i := range tx.TxOut {
			w.Write([]byte("<output>"))
			fmt.Fprint(w, "<value>", tx.TxOut[i].Value, "</value>")
			adr := btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, common.Testnet)
			if adr != nil {
				fmt.Fprint(w, "<addr>", adr.String(), "</addr>")
			} else {
				fmt.Fprint(w, "<addr>", "scr:"+hex.EncodeToString(tx.TxOut[i].Pk_script), "</addr>")
			}
			w.Write([]byte("</output>"))
		}
		w.Write([]byte("</outputs>"))
	} else {
		w.Write([]byte("<status>Not found</status>"))
	}
	w.Write([]byte("</tx>"))
}


func xml_txs2s(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if checksid(r) {
		if len(r.Form["del"])>0 {
			tid := btc.NewUint256FromString(r.Form["del"][0])
			if tid!=nil {
				network.TxMutex.Lock()
				delete(network.TransactionsToSend, tid.BIdx())
				network.TxMutex.Unlock()
			}
		}

		if len(r.Form["send"])>0 {
			tid := btc.NewUint256FromString(r.Form["send"][0])
			if tid!=nil {
				network.TxMutex.Lock()
				if ptx, ok := network.TransactionsToSend[tid.BIdx()]; ok {
					network.TxMutex.Unlock()
					cnt := network.NetRouteInv(1, tid, nil)
					if cnt==0 {
						usif.SendInvToRandomPeer(1, tid)
					} else {
						ptx.Invsentcnt += cnt
					}
				} else {
					network.TxMutex.Unlock()
				}
			}
		}

		if len(r.Form["sendone"])>0 {
			tid := btc.NewUint256FromString(r.Form["sendone"][0])
			if tid!=nil {
				network.TxMutex.Lock()
				if ptx, ok := network.TransactionsToSend[tid.BIdx()]; ok {
					network.TxMutex.Unlock()
					usif.SendInvToRandomPeer(1, tid)
					ptx.Invsentcnt++
				} else {
					network.TxMutex.Unlock()
				}
			}
		}

	}

	w.Header()["Content-Type"] = []string{"text/xml"}

	if len(r.Form["id"])>0 {
		output_tx_xml(w, r.Form["id"][0])
		return
	}

	w.Write([]byte("<txpool>"))
	network.TxMutex.Lock()
	for _, v := range network.TransactionsToSend {
		w.Write([]byte("<tx>"))
		fmt.Fprint(w, "<id>", v.Tx.Hash.String(), "</id>")
		fmt.Fprint(w, "<time>", v.Firstseen.Unix(), "</time>")
		fmt.Fprint(w, "<len>", len(v.Data), "</len>")
		fmt.Fprint(w, "<own>", v.Own, "</own>")
		fmt.Fprint(w, "<firstseen>", v.Firstseen.Unix(), "</firstseen>")
		fmt.Fprint(w, "<invsentcnt>", v.Invsentcnt, "</invsentcnt>")
		fmt.Fprint(w, "<sentcnt>", v.SentCnt, "</sentcnt>")
		fmt.Fprint(w, "<sentlast>", v.Lastsent.Unix(), "</sentlast>")
		fmt.Fprint(w, "<volume>", v.Volume, "</volume>")
		fmt.Fprint(w, "<fee>", v.Fee, "</fee>")
		fmt.Fprint(w, "<blocked>", v.Blocked, "</blocked>")
		w.Write([]byte("</tx>"))
	}
	network.TxMutex.Unlock()
	w.Write([]byte("</txpool>"))
}


func xml_txsre(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<txbanned>"))
	network.TxMutex.Lock()
	for _, v := range network.TransactionsRejected {
		w.Write([]byte("<tx>"))
		fmt.Fprint(w, "<id>", v.Id.String(), "</id>")
		fmt.Fprint(w, "<time>", v.Time.Unix(), "</time>")
		fmt.Fprint(w, "<len>", v.Size, "</len>")
		fmt.Fprint(w, "<reason>", v.Reason, "</reason>")
		w.Write([]byte("</tx>"))
	}
	network.TxMutex.Unlock()
	w.Write([]byte("</txbanned>"))
}


func xml_txw4i(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<pending>"))
	network.TxMutex.Lock()
	for _, v := range network.WaitingForInputs {
		w.Write([]byte("<wait4>"))
		fmt.Fprint(w, "<id>", v.TxID.String(), "</id>")
		for x, t := range v.Ids {
			w.Write([]byte("<tx>"))
			if v, ok := network.TransactionsRejected[x]; ok {
				fmt.Fprint(w, "<id>", v.Id.String(), "</id>")
				fmt.Fprint(w, "<time>", t.Unix(), "</time>")
			} else {
				fmt.Fprint(w, "<id>FATAL ERROR!!! This should not happen! Please report</id>")
				fmt.Fprint(w, "<time>", time.Now().Unix(), "</time>")
			}
			w.Write([]byte("</tx>"))
		}
		w.Write([]byte("</wait4>"))
	}
	network.TxMutex.Unlock()
	w.Write([]byte("</pending>"))
}


func raw_tx(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(w, "Error")
			if err, ok := r.(error); ok {
				fmt.Fprintln(w, err.Error())
			}
		}
	}()

	if len(r.Form["id"])==0 {
		fmt.Println("No id given")
		return
	}

	txid := btc.NewUint256FromString(r.Form["id"][0])
	fmt.Fprintln(w, "TxID:", txid.String())
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		s, _, _, _, _ := usif.DecodeTx(tx.Tx)
		w.Write([]byte(s))
	} else {
		fmt.Fprintln(w, "Not found")
	}
}
