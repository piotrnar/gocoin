package webui

import (
	"fmt"
	"time"
	"sort"
	"sync"
	"strings"
	"strconv"
	"net/http"
	"io/ioutil"
	"encoding/hex"
	"encoding/json"
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


func output_tx_xml(w http.ResponseWriter, tx *btc.Tx) {
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
			ok := script.VerifyTxScript(po.Pk_script, po.Value, i, tx, script.VER_P2SH|script.VER_DERSIG|script.VER_CLTV)
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

			if btc.IsP2SH(po.Pk_script) {
				fmt.Fprint(w, "<input_sigops>", btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig), "</input_sigops>")
			}
		} else {
			w.Write([]byte("<status>UNKNOWN INPUT</status>"))
		}
		fmt.Fprint(w, "<sequence>", tx.TxIn[i].Sequence, "</sequence>")
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
}


func output_utxo_tx_xml(w http.ResponseWriter, minedid, minedat string) {
	txid := btc.NewUint256FromString(minedid)
	if txid==nil {
		return
	}

	block_number, er := strconv.ParseUint(minedat, 10, 32)
	if er != nil {
		return
	}

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	w.Write([]byte("<tx>"))
	fmt.Fprint(w, "<id>", minedid, "</id>")
	if dat, er := common.BlockChain.GetRawTx(uint32(block_number), txid); er == nil {
		w.Write([]byte("<status>OK</status>"))
		w.Write([]byte(fmt.Sprint("<len>", len(dat), "</len>")))
		tx, _ := btc.NewTx(dat)
		output_tx_xml(w, tx)
	} else {
		w.Write([]byte("<status>Not found</status>"))
	}
	w.Write([]byte("</tx>"))

	lck.Out.Done()

}


func output_mempool_tx_xml(w http.ResponseWriter, id string) {
	txid := btc.NewUint256FromString(id)
	if txid==nil {
		return
	}
	w.Write([]byte("<tx>"))
	fmt.Fprint(w, "<id>", id, "</id>")
	if t2s, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		w.Write([]byte("<status>OK</status>"))
		w.Write([]byte(fmt.Sprint("<len>", len(t2s.Data), "</len>")))
		fmt.Fprint(w, "<inputs>", len(t2s.TxIn), "</inputs>")
		fmt.Fprint(w, "<outputs>", len(t2s.TxOut), "</outputs>")
		w.Write([]byte(fmt.Sprint("<time_received>", t2s.Firstseen.Unix(), "</time_received>")))
		output_tx_xml(w, t2s.Tx)
		fmt.Fprint(w, "<tx_sigops>", t2s.Sigops, "</tx_sigops>")
		fmt.Fprint(w, "<final>", t2s.Final, "</final>")
		fmt.Fprint(w, "<verify_us>", uint(t2s.VerifyTime/time.Microsecond), "</verify_us>")
	} else {
		w.Write([]byte("<status>Not found</status>"))
	}
	w.Write([]byte("</tx>"))
}


/* memory pool transaction sorting stuff */
type sortedTxList []*network.OneTxToSend

func (tl sortedTxList) Len() int {return len(tl)}
func (tl sortedTxList) Swap(i, j int)      { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool {
	var res bool
	switch txs2s_sort {
		case "age":
			res = tl[j].Firstseen.UnixNano() > tl[i].Firstseen.UnixNano()
		case "len":
			res = len(tl[j].Data) < len(tl[i].Data)
		case "inp":
			res = len(tl[j].TxIn) < len(tl[i].TxIn)
		case "out":
			res = len(tl[j].TxOut) < len(tl[i].TxOut)
		case "btc":
			res = tl[j].Volume < tl[i].Volume
		case "fee":
			res = tl[j].Fee < tl[i].Fee
		case "ops":
			res = tl[j].Sigops < tl[i].Sigops
		case "rbf":
			res = !tl[j].Final && tl[i].Final
		case "ver":
			res = int(tl[j].VerifyTime) < int(tl[i].VerifyTime)
		default: /*spb*/
			spb_i := float64(tl[i].Fee)/float64(len(tl[i].Data))
			spb_j := float64(tl[j].Fee)/float64(len(tl[j].Data))
			res = spb_j < spb_i
	}
	if txs2s_sort_desc {
		return res
	} else {
		return !res
	}
}

var txs2s_count int = 1000
var txs2s_sort string = "spb"
var txs2s_sort_desc bool = true


func xml_txs2s(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}

	if len(r.Form["minedid"])>0 && len(r.Form["minedat"])>0 {
		output_utxo_tx_xml(w, r.Form["minedid"][0], r.Form["minedat"][0])
		return
	}

	if len(r.Form["id"])>0 {
		output_mempool_tx_xml(w, r.Form["id"][0])
		return
	}

	if checksid(r) {
		if len(r.Form["del"])>0 {
			tid := btc.NewUint256FromString(r.Form["del"][0])
			if tid!=nil {
				network.TxMutex.Lock()
				if tts, ok := network.TransactionsToSend[tid.BIdx()]; ok {
					network.DeleteToSend(tts)
				}
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

		if len(r.Form["cnt"])>0 {
			u, e := strconv.ParseUint(r.Form["cnt"][0], 10, 32)
			if e==nil && u>0 && u<10e3 {
				txs2s_count = int(u)
			}
		}

		if len(r.Form["sort"])>0 && len(r.Form["sort"][0])==3 {
			txs2s_sort = r.Form["sort"][0]
		}

		txs2s_sort_desc = len(r.Form["descending"])>0
	}

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := make(sortedTxList, len(network.TransactionsToSend))
	var cnt int
	for _, v := range network.TransactionsToSend {
		if len(r.Form["ownonly"])>0 && v.Own==0 {
			continue
		}
		sorted[cnt] = v
		cnt++
	}
	sorted = sorted[:cnt]
	sort.Sort(sorted)

	w.Write([]byte("<txpool>"))
	for cnt=0; cnt<len(sorted) && cnt<txs2s_count; cnt++ {
		v := sorted[cnt]
		w.Write([]byte("<tx>"))
		fmt.Fprint(w, "<id>", v.Tx.Hash.String(), "</id>")
		fmt.Fprint(w, "<time>", v.Firstseen.Unix(), "</time>")
		fmt.Fprint(w, "<len>", len(v.Data), "</len>")
		fmt.Fprint(w, "<inputs>", len(v.TxIn), "</inputs>")
		fmt.Fprint(w, "<outputs>", len(v.TxOut), "</outputs>")
		fmt.Fprint(w, "<own>", v.Own, "</own>")
		fmt.Fprint(w, "<firstseen>", v.Firstseen.Unix(), "</firstseen>")
		fmt.Fprint(w, "<invsentcnt>", v.Invsentcnt, "</invsentcnt>")
		fmt.Fprint(w, "<sigops>", v.Sigops, "</sigops>")
		fmt.Fprint(w, "<sentcnt>", v.SentCnt, "</sentcnt>")
		fmt.Fprint(w, "<sentlast>", v.Lastsent.Unix(), "</sentlast>")
		fmt.Fprint(w, "<volume>", v.Volume, "</volume>")
		fmt.Fprint(w, "<fee>", v.Fee, "</fee>")
		fmt.Fprint(w, "<blocked>", v.Blocked, "</blocked>")
		fmt.Fprint(w, "<final>", v.Final, "</final>")
		fmt.Fprint(w, "<verify_us>", uint(v.VerifyTime/time.Microsecond), "</verify_us>")
		w.Write([]byte("</tx>"))
	}
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


func json_txstat(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"application/json"}
	w.Write([]byte("{"))

	network.TxMutex.Lock()

	w.Write([]byte(fmt.Sprint("\"t2s_cnt\":", len(network.TransactionsToSend), ",")))
	w.Write([]byte(fmt.Sprint("\"t2s_size\":", network.TransactionsToSendSize, ",")))
	w.Write([]byte(fmt.Sprint("\"tre_cnt\":", len(network.TransactionsRejected), ",")))
	w.Write([]byte(fmt.Sprint("\"tre_size\":", network.TransactionsRejectedSize, ",")))
	w.Write([]byte(fmt.Sprint("\"ptr1_cnt\":", len(network.TransactionsPending), ",")))
	w.Write([]byte(fmt.Sprint("\"ptr2_cnt\":", len(network.NetTxs), ",")))
	w.Write([]byte(fmt.Sprint("\"spent_outs_cnt\":", len(network.SpentOutputs), ",")))
	w.Write([]byte(fmt.Sprint("\"awaiting_inputs\":", len(network.WaitingForInputs), "")))

	network.TxMutex.Unlock()

	w.Write([]byte("}\n"))
}


func txt_mempool_fees(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"text/plain"}
	w.Write([]byte(usif.MemoryPoolFees()))
}


func json_mempool_stats(w http.ResponseWriter, r *http.Request) {
	var cnt int
	var division uint64

	if !ipchecker(r) {
		return
	}

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	division = network.TransactionsToSendSize/100

	if len(r.Form["div"])>0 {
		d, e := strconv.ParseUint(r.Form["div"][0], 10, 64)
		if e==nil {
			division = d
		}
	}

	if division<100 {
		division = 100
	} else if division>1e6 {
		division = 1e6
	}

	sorted := make(usif.SortedTxToSend, len(network.TransactionsToSend))
	for _, v := range network.TransactionsToSend {
		sorted[cnt] = v
		cnt++
	}
	sort.Sort(sorted)

	type one_stat_row struct {
		Txs_so_far uint
		Offset_in_block uint
		Current_tx_length uint
		Current_tx_spb float64
		Current_tx_id string
		Time_received uint
	}
	var mempool_stats []one_stat_row

	var totlen uint64
	for cnt=0; cnt<len(sorted); cnt++ {
		v := sorted[cnt]
		newlen := totlen+uint64(len(v.Data))

		if cnt==0 || cnt+1==len(sorted) || (newlen/division)!=(totlen/division) {
			mempool_stats = append(mempool_stats, one_stat_row{
				Txs_so_far : uint(cnt),
				Offset_in_block : uint(totlen),
				Current_tx_length : uint(len(v.Data)),
				Current_tx_spb : float64(v.Fee)/float64(len(v.Data)),
				Current_tx_id : v.Hash.String(),
				Time_received : uint(v.Firstseen.Unix())})
		}
		totlen = newlen
	}

	bx, er := json.Marshal(mempool_stats)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
