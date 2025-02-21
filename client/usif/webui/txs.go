package webui

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
)

func txs_page_modify(r *http.Request, page *[]byte) {
	var txloadresult string
	var wg sync.WaitGroup
	var tx2in []byte

	// Check if there is a tx upload request
	r.ParseMultipartForm(2e6)
	fil, _, _ := r.FormFile("txfile")
	if fil != nil {
		tx2in, _ = io.ReadAll(fil)
	} else if len(r.Form["rawtx"]) == 1 {
		tx2in, _ = hex.DecodeString(r.Form["rawtx"][0])
	}

	if len(tx2in) > 0 {
		wg.Add(1)
		req := &usif.OneUiReq{Param: string(tx2in)}
		req.Done.Add(1)
		req.Handler = func(dat string) {
			txloadresult = usif.LoadRawTx([]byte(dat))
			wg.Done()
		}
		usif.UiChannel <- req
	}

	wg.Wait()

	if txloadresult != "" {
		ld := load_template("txs_load.html")
		ld = strings.Replace(ld, "{TX_RAW_DATA}", txloadresult, 1)
		*page = []byte(strings.Replace(string(*page), "<!--TX_LOAD-->", ld, 1))
	}
}

func output_tx_xml(w http.ResponseWriter, tx *btc.Tx) {
	tx.AllocVerVars()
	defer tx.Clean()
	tx.Spent_outputs = make([]*btc.TxOut, len(tx.TxIn))
	for i := range tx.TxIn {
		var po *btc.TxOut
		inpid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
		if txinmem, ok := txpool.TransactionsToSend[inpid.BIdx()]; ok {
			if int(tx.TxIn[i].Input.Vout) < len(txinmem.TxOut) {
				po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			}
		} else {
			po = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		}
		tx.Spent_outputs[i] = po
	}
	w.Write([]byte("<input_list>"))
	ver_flags := common.CurrentScriptFlags()
	for i := range tx.TxIn {
		w.Write([]byte("<input>"))
		w.Write([]byte("<script_sig>"))
		w.Write([]byte(hex.EncodeToString(tx.TxIn[i].ScriptSig)))
		w.Write([]byte("</script_sig>"))
		fmt.Fprint(w, "<txid-vout>", tx.TxIn[i].Input.String(), "</txid-vout>")
		po := tx.Spent_outputs[i]
		if po != nil {
			ok := script.VerifyTxScript(po.Pk_script, &script.SigChecker{Amount: po.Value, Idx: i, Tx: tx}, ver_flags)
			if !ok {
				w.Write([]byte("<status>Script FAILED</status>"))
			} else {
				w.Write([]byte("<status>OK</status>"))
			}
			fmt.Fprint(w, "<value>", po.Value, "</value>")
			fmt.Fprint(w, "<pkscript>", hex.EncodeToString(po.Pk_script), "</pkscript>")
			if ad := btc.NewAddrFromPkScript(po.Pk_script, common.Testnet); ad != nil {
				fmt.Fprint(w, "<addr>", ad.String(), "</addr>")
			}
			fmt.Fprint(w, "<block>", po.BlockHeight, "</block>")

			if btc.IsP2SH(po.Pk_script) {
				fmt.Fprint(w, "<input_sigops>", btc.WITNESS_SCALE_FACTOR*btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig), "</input_sigops>")
			}
			fmt.Fprint(w, "<witness_sigops>", tx.CountWitnessSigOps(i, po.Pk_script), "</witness_sigops>")
		} else {
			w.Write([]byte("<status>Unknown input</status>"))
		}
		fmt.Fprint(w, "<sequence>", tx.TxIn[i].Sequence, "</sequence>")

		if tx.SegWit != nil {
			w.Write([]byte("<segwit>"))
			for _, wit := range tx.SegWit[i] {
				w.Write([]byte("<witness>"))
				w.Write([]byte(hex.EncodeToString(wit)))
				w.Write([]byte("</witness>"))
			}
			w.Write([]byte("</segwit>"))
		}
		w.Write([]byte("</input>"))
	}
	w.Write([]byte("</input_list>"))

	w.Write([]byte("<output_list>"))
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
	w.Write([]byte("</output_list>"))
}

func tx_xml(w http.ResponseWriter, v *txpool.OneTxToSend, verbose bool) {
	w.Write([]byte("<tx><status>OK</status>"))
	fmt.Fprint(w, "<id>", v.Tx.Hash.String(), "</id>")
	fmt.Fprint(w, "<version>", v.Tx.Version, "</version>")
	fmt.Fprint(w, "<time>", v.Firstseen.Unix(), "</time>")
	if int(v.Size) != len(v.Raw) {
		panic("TX size does not match data length")
	}

	fmt.Fprint(w, "<size>", v.Size, "</size>")
	fmt.Fprint(w, "<nwsize>", v.NoWitSize, "</nwsize>")
	fmt.Fprint(w, "<weight>", v.Weight(), "</weight>")
	fmt.Fprint(w, "<sw_compress>", 1000*(int(v.Size)-int(v.NoWitSize))/int(v.Size), "</sw_compress>")
	fmt.Fprint(w, "<inputs>", len(v.TxIn), "</inputs>")
	fmt.Fprint(w, "<outputs>", len(v.TxOut), "</outputs>")
	fmt.Fprint(w, "<lock_time>", v.Lock_time, "</lock_time>")
	fmt.Fprint(w, "<witness_cnt>", len(v.SegWit), "</witness_cnt>")
	if verbose {
		output_tx_xml(w, v.Tx)
	}
	fmt.Fprint(w, "<own>", v.Local, "</own>")
	fmt.Fprint(w, "<firstseen>", v.Firstseen.Unix(), "</firstseen>")
	fmt.Fprint(w, "<invsentcnt>", atomic.LoadUint32(&v.Invsentcnt), "</invsentcnt>")
	fmt.Fprint(w, "<sigops>", v.SigopsCost, "</sigops>")
	fmt.Fprint(w, "<sentcnt>", v.SentCnt, "</sentcnt>")
	fmt.Fprint(w, "<sentlast>", v.Lastsent.Unix(), "</sentlast>")
	fmt.Fprint(w, "<volume>", v.Volume, "</volume>")
	fmt.Fprint(w, "<fee>", v.Fee, "</fee>")
	fmt.Fprint(w, "<blocked>", txpool.ReasonToString(v.Blocked), "</blocked>")
	fmt.Fprint(w, "<final>", v.Final, "</final>")
	fmt.Fprint(w, "<verify_us>", uint(v.VerifyTime/time.Microsecond), "</verify_us>")
	fmt.Fprint(w, "<meminputcnt>", v.MemInputCnt, "</meminputcnt>")
	if verbose {
		fmt.Fprint(w, "<raw>", hex.EncodeToString(v.Raw), "</raw>")
	}
	w.Write([]byte("</tx>"))
}

func output_utxo_tx_xml(w http.ResponseWriter, minedid, minedat string) {
	txid := btc.NewUint256FromString(minedid)
	if txid == nil {
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
	if dat, er := common.GetRawTx(uint32(block_number), txid); er == nil {
		w.Write([]byte("<status>OK</status>"))
		w.Write([]byte(fmt.Sprint("<size>", len(dat), "</size>")))
		tx, _ := btc.NewTx(dat)
		output_tx_xml(w, tx)
	} else {
		w.Write([]byte("<status>Not found</status>"))
	}
	w.Write([]byte("</tx>"))

	lck.Out.Done()

}

/* memory pool transaction sorting stuff */
type sortedTxList []*txpool.OneTxToSend

func (tl sortedTxList) Len() int      { return len(tl) }
func (tl sortedTxList) Swap(i, j int) { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool {
	var res bool
	switch txs2s_sort {
	case "age":
		res = tl[j].Firstseen.UnixNano() > tl[i].Firstseen.UnixNano()
	case "siz":
		res = tl[j].Size < tl[i].Size
	case "nws":
		res = tl[j].NoWitSize < tl[i].NoWitSize
	case "wgh":
		res = tl[j].Weight() < tl[i].Weight()
	case "inp":
		res = len(tl[j].TxIn) < len(tl[i].TxIn)
	case "out":
		res = len(tl[j].TxOut) < len(tl[i].TxOut)
	case "btc":
		res = tl[j].Volume < tl[i].Volume
	case "fee":
		res = tl[j].Fee < tl[i].Fee
	case "ops":
		res = tl[j].SigopsCost < tl[i].SigopsCost
	case "rbf":
		res = !tl[j].Final && tl[i].Final
	case "ver":
		res = int(tl[j].VerifyTime) < int(tl[i].VerifyTime)
	case "swc":
		sw_compr_i := float64(int(tl[i].Size)-int(tl[i].NoWitSize)) / float64(tl[i].Size)
		sw_compr_j := float64(int(tl[j].Size)-int(tl[j].NoWitSize)) / float64(tl[j].Size)
		res = sw_compr_i > sw_compr_j
	default: /*spb*/
		spb_i := float64(tl[i].Fee) / float64(tl[i].Weight())
		spb_j := float64(tl[j].Fee) / float64(tl[j].Weight())
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

	if len(r.Form["minedid"]) > 0 && len(r.Form["minedat"]) > 0 {
		output_utxo_tx_xml(w, r.Form["minedid"][0], r.Form["minedat"][0])
		return
	}

	if len(r.Form["id"]) > 0 {
		txid := btc.NewUint256FromString(r.Form["id"][0])
		if txid == nil {
			return
		}
		txpool.TxMutex.Lock()
		defer txpool.TxMutex.Unlock()
		if t2s, ok := txpool.TransactionsToSend[txid.BIdx()]; ok {
			tx_xml(w, t2s, true)
		} else {
			w.Write([]byte("<tx>"))
			fmt.Fprint(w, "<id>", txid.String(), "</id>")
			w.Write([]byte("<status>Not found</status>"))
			w.Write([]byte("</tx>"))
		}
		return
	}

	if checksid(r) {
		if len(r.Form["del"]) > 0 {
			tid := btc.NewUint256FromString(r.Form["del"][0])
			if tid != nil {
				txpool.TxMutex.Lock()
				if tts, ok := txpool.TransactionsToSend[tid.BIdx()]; ok {
					tts.Delete(true, 0)
				}
				txpool.TxMutex.Unlock()
			}
		}

		if len(r.Form["send"]) > 0 {
			tid := btc.NewUint256FromString(r.Form["send"][0])
			if tid != nil {
				txpool.TxMutex.Lock()
				if ptx, ok := txpool.TransactionsToSend[tid.BIdx()]; ok {
					txpool.TxMutex.Unlock()
					cnt := network.NetRouteInv(1, tid, nil)
					if cnt == 0 {
						usif.SendInvToRandomPeer(1, tid)
					} else {
						atomic.AddUint32(&ptx.Invsentcnt, cnt)
					}
				} else {
					txpool.TxMutex.Unlock()
				}
			}
		}

		if len(r.Form["sendone"]) > 0 {
			tid := btc.NewUint256FromString(r.Form["sendone"][0])
			if tid != nil {
				txpool.TxMutex.Lock()
				if ptx, ok := txpool.TransactionsToSend[tid.BIdx()]; ok {
					txpool.TxMutex.Unlock()
					usif.SendInvToRandomPeer(1, tid)
					atomic.AddUint32(&ptx.Invsentcnt, 1)
				} else {
					txpool.TxMutex.Unlock()
				}
			}
		}

		if len(r.Form["quiet"]) > 0 {
			return
		}

		if len(r.Form["cnt"]) > 0 {
			u, e := strconv.ParseUint(r.Form["cnt"][0], 10, 32)
			if e == nil && u > 0 && u < 10e3 {
				txs2s_count = int(u)
			}
		}

		if len(r.Form["sort"]) > 0 && len(r.Form["sort"][0]) == 3 {
			txs2s_sort = r.Form["sort"][0]
		}

		txs2s_sort_desc = len(r.Form["descending"]) > 0
	}

	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	sorted := make(sortedTxList, len(txpool.TransactionsToSend))
	var cnt int
	for _, v := range txpool.TransactionsToSend {
		if len(r.Form["ownonly"]) > 0 && !v.Local {
			continue
		}
		sorted[cnt] = v
		cnt++
	}
	sorted = sorted[:cnt]
	sort.Sort(sorted)

	w.Write([]byte("<txpool>"))
	for cnt = 0; cnt < len(sorted) && cnt < txs2s_count; cnt++ {
		v := sorted[cnt]
		tx_xml(w, v, false)
	}
	w.Write([]byte("</txpool>"))
}

func xml_txsre(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<txbanned>"))
	txpool.TxMutex.Lock()
	for idx := txpool.TRIdxTail; idx != txpool.TRIdxHead; idx = txpool.TRIdxPrev(idx) {
		if v := txpool.TransactionsRejected[txpool.TRIdxArray[idx]]; v != nil {
			w.Write([]byte("<tx>"))
			fmt.Fprint(w, "<id>", v.Id.String(), "</id>")
			fmt.Fprint(w, "<time>", v.Time.Unix(), "</time>")
			fmt.Fprint(w, "<size>", v.Size, "</size>")
			fmt.Fprint(w, "<syssize>", v.Footprint, "</syssize>")
			fmt.Fprint(w, "<inmem>", v.Tx != nil, "</inmem>")
			fmt.Fprint(w, "<reason>", txpool.ReasonToString(v.Reason), "</reason>")
			w.Write([]byte("</tx>"))
		}
	}
	txpool.TxMutex.Unlock()
	w.Write([]byte("</txbanned>"))
}

func xml_txw4i(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<pending>"))
	txpool.TxMutex.Lock()
	type onerec struct {
		val  uint64
		bidx btc.BIDX
	}
	w4ilist := make([]onerec, 0, len(txpool.WaitingForInputs))
	for k, v := range txpool.WaitingForInputs {
		r := onerec{val: binary.LittleEndian.Uint64((v.TxID.Hash[24:32])), bidx: k}
		w4ilist = append(w4ilist, r)
	}
	sort.Slice(w4ilist, func(i, j int) bool {
		return w4ilist[i].val < w4ilist[j].val
	})
	for _, k := range w4ilist {
		v := txpool.WaitingForInputs[k.bidx]
		w.Write([]byte("<wait4>"))
		fmt.Fprint(w, "<id>", v.TxID.String(), "</id>")
		for _, x := range v.Ids {
			w.Write([]byte("<tx>"))
			if v, ok := txpool.TransactionsRejected[x]; ok {
				fmt.Fprint(w, "<id>", v.Id.String(), "</id>")
				fmt.Fprint(w, "<time>", v.Time.Unix(), "</time>")
				fmt.Fprint(w, "<size>", v.Size, "</size>")
			} else {
				fmt.Fprint(w, "<id>FATAL ERROR!!! This should not happen! Please report</id>")
				fmt.Fprint(w, "<time>", time.Now().Unix(), "</time>")
				fmt.Fprint(w, "<size>666</size>")
			}
			w.Write([]byte("</tx>"))
		}
		w.Write([]byte("</wait4>"))
	}
	txpool.TxMutex.Unlock()
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

	if len(r.Form["id"]) == 0 {
		fmt.Println("No id given")
		return
	}

	txid := btc.NewUint256FromString(r.Form["id"][0])
	if txid == nil {
		fmt.Fprintln(w, "Incorrect id")
		return
	}
	var tx *btc.Tx
	var header string
	txpool.TxMutex.Lock()
	if t2s, ok := txpool.TransactionsToSend[txid.BIdx()]; ok {
		header = "From TransactionsToSend"
		tx = t2s.Tx
	} else if txr, ok := txpool.TransactionsRejected[txid.BIdx()]; ok && txr.Tx != nil {
		header = "From TransactionsRejected"
		tx = txr.Tx
	}
	txpool.TxMutex.Unlock()
	if tx != nil {
		fmt.Fprintln(w, header)
		usif.DecodeTx(w, tx)
	} else {
		fmt.Fprint(w, "TxID ", txid.String(), " not found")
	}

}

func json_txstat(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"application/json"}
	w.Write([]byte("{"))

	txpool.TxMutex.Lock()

	w.Write([]byte(fmt.Sprint("\"t2s_cnt\":", len(txpool.TransactionsToSend), ",")))
	w.Write([]byte(fmt.Sprint("\"t2s_size\":", txpool.TransactionsToSendSize, ",")))
	w.Write([]byte(fmt.Sprint("\"t2s_weight\":", txpool.TransactionsToSendWeight, ",")))
	w.Write([]byte(fmt.Sprint("\"tre_cnt\":", len(txpool.TransactionsRejected), ",")))
	w.Write([]byte(fmt.Sprint("\"tre_size\":", txpool.TransactionsRejectedSize, ",")))
	w.Write([]byte(fmt.Sprint("\"ptr1_cnt\":", len(txpool.TransactionsPending), ",")))
	w.Write([]byte(fmt.Sprint("\"ptr2_cnt\":", len(network.NetTxs), ",")))
	w.Write([]byte(fmt.Sprint("\"spent_outs_cnt\":", len(txpool.SpentOutputs), ",")))
	w.Write([]byte(fmt.Sprint("\"awaiting_inputs\":", len(txpool.WaitingForInputs), ",")))
	w.Write([]byte(fmt.Sprint("\"awaiting_inputs_size\":", txpool.WaitingForInputsSize, ",")))
	w.Write([]byte(fmt.Sprint("\"min_fee_per_kb\":", common.MinFeePerKB(), ",")))
	w.Write([]byte(fmt.Sprint("\"tx_pool_on\":", common.Get(&common.CFG.TXPool.Enabled), ",")))
	w.Write([]byte(fmt.Sprint("\"tx_routing_on\":", common.Get(&common.CFG.TXRoute.Enabled), ",")))
	w.Write([]byte(fmt.Sprint("\"sorting_disabled\":", txpool.SortingDisabled, ",")))
	w.Write([]byte(fmt.Sprint("\"sorting_list_dirty\":", txpool.SortListDirty, ",")))
	w.Write([]byte(fmt.Sprint("\"fee_packages_dirty\":", txpool.FeePackagesDirty, ",")))
	w.Write([]byte(fmt.Sprint("\"current_fee_adjusted_spkb\":", txpool.CurrentFeeAdjustedSPKB, "")))

	txpool.TxMutex.Unlock()

	w.Write([]byte("}\n"))
}

func txt_mempool_fees(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"text/plain"}
	w.Write([]byte(usif.MemoryPoolFees()))
}

func json_mpfees(w http.ResponseWriter, r *http.Request) {
	var division, maxweight uint64
	var e error

	if !ipchecker(r) {
		return
	}

	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	if len(r.Form["max"]) > 0 {
		maxweight, e = strconv.ParseUint(r.Form["max"][0], 10, 64)
		if e != nil {
			maxweight = txpool.TransactionsToSendWeight
		}
	} else {
		maxweight = txpool.TransactionsToSendWeight
	}

	if maxweight > txpool.TransactionsToSendWeight {
		maxweight = txpool.TransactionsToSendWeight
	}

	if len(r.Form["div"]) > 0 {
		division, e = strconv.ParseUint(r.Form["div"][0], 10, 64)
		if e != nil {
			division = maxweight / 100
		}
	} else {
		division = maxweight / 100
	}

	if division < 1 {
		division = 1
	}

	sorted := txpool.GetMempoolFees(maxweight)

	var bx []byte
	var er error

	if len(r.Form["full"]) > 0 {
		type one_stat_row struct {
			Txs_so_far        uint
			Txs_cnt_here      uint
			Real_len_so_far   uint
			Weight_so_far     uint
			Current_tx_weight uint
			Current_tx_spb    float64
			Current_tx_id     string
			Time_received     uint
			Fees_so_far       uint64
			Ord_weight_so_far uint
			Ord_fees_so_far   uint64
		}
		var mempool_stats []one_stat_row

		var totweight, totfee, ordweight, ordfees uint64
		var txcntsofar int
		for cnt, v := range sorted {
			newtotweight := totweight + uint64(v.Weight)
			totfee += v.Fee
			for _, tx := range v.Txs {
				if yes, _ := tx.ContainsOrdFile(true); yes {
					ordweight += uint64(tx.Weight())
					ordfees += v.Fee
				}
			}

			if cnt == 0 || cnt+1 == len(sorted) || (newtotweight/division) != (totweight/division) {
				cur_spb := 4.0 * float64(v.Fee) / float64(v.Weight)
				mempool_stats = append(mempool_stats, one_stat_row{
					Txs_so_far:        uint(txcntsofar),
					Txs_cnt_here:      uint(len(v.Txs)),
					Weight_so_far:     uint(newtotweight),
					Current_tx_weight: uint(v.Weight),
					Current_tx_spb:    cur_spb,
					Current_tx_id:     v.Txs[0].Hash.String(),
					Fees_so_far:       totfee,
					Time_received:     uint(v.Txs[0].Firstseen.Unix()),
					Ord_weight_so_far: uint(ordweight),
					Ord_fees_so_far:   ordfees,
				})
			}
			txcntsofar += len(v.Txs)
			totweight = newtotweight
			if totweight >= maxweight {
				break
			}
		}
		bx, er = json.Marshal(mempool_stats)
	} else {
		var mempool_stats [][3]uint64
		var totweight uint64
		var totfeessofar uint64
		for cnt, r := range sorted {
			wgh := r.Weight
			fee := r.Fee
			totfeessofar += fee
			newtotweight := totweight + wgh

			if cnt == 0 || cnt+1 == len(sorted) || (newtotweight/division) != (totweight/division) {
				mempool_stats = append(mempool_stats, [3]uint64{newtotweight, 4000 * fee / wgh, totfeessofar})
			}
			totweight = newtotweight
		}
		bx, er = json.Marshal(mempool_stats)
	}
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
