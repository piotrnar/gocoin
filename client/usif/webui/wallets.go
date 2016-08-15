package webui

import (
	"fmt"
	"bytes"
	"strconv"
	"net/http"
	"io/ioutil"
	"archive/zip"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)


func p_wal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	page := load_template("wallet.html")
	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}


func json_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if r.Method!="POST" {
		return
	}

	inp, er := ioutil.ReadAll(r.Body)
	if er != nil {
		println(er.Error())
		return
	}

	var addrs []string
	er = json.Unmarshal(inp, &addrs)
	if er != nil {
		println(er.Error())
		return
	}

	type OneOut struct {
		TxId string
		Vout uint32
		Value uint64
		Height uint32
		Coinbase bool
	}
	type OneOuts struct {
		Value uint64
		Outs []OneOut
	}

	var out map[string] OneOuts

	out = make(map[string]OneOuts)

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	wallet.BalanceMutex.Lock()
	for _, a := range addrs {
		aa, e := btc.NewAddrFromString(a)
		if e==nil {
			var newrec OneOuts
			var rec *wallet.OneAllAddrBal
			if aa.Version==btc.AddrVerPubkey(common.Testnet) {
				rec = wallet.AllBalancesP2KH[aa.Hash160]
			} else if aa.Version==btc.AddrVerScript(common.Testnet) {
				rec = wallet.AllBalancesP2SH[aa.Hash160]
			} else {
				continue
			}
			if rec!=nil {
				newrec.Value = rec.Value
				for _, v := range rec.Unsp {
					if qr, vout := v.GetRec(); qr!=nil {
						if oo := qr.Outs[vout]; oo!=nil {
							newrec.Outs = append(newrec.Outs, OneOut{
								TxId : btc.NewUint256(qr.TxID[:]).String(), Vout : vout,
								Value : oo.Value,
								Height : qr.InBlock, Coinbase : qr.Coinbase})
							}
					}
				}
			}
			out[aa.String()] = newrec
		}
	}
	wallet.BalanceMutex.Unlock()

	lck.Out.Done()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func dl_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if r.Method!="POST" {
		return
	}

	var addrs []string
	var labels []string

	if len(r.Form["addrcnt"])!=1 {
		println("no addrcnt")
		return
	}
	addrcnt, _ := strconv.ParseUint(r.Form["addrcnt"][0], 10, 32)

	for i:=0; i<int(addrcnt); i++ {
		is := fmt.Sprint(i)
		if len(r.Form["addr"+is])==1 {
			addrs = append(addrs, r.Form["addr"+is][0])
			if len(r.Form["label"+is])==1 {
				labels = append(labels, r.Form["label"+is][0])
			} else {
				labels = append(labels, "")
			}
		}
	}

	type one_unsp_rec struct {
		btc.TxPrevOut
		Value uint64
		Addr string
		MinedAt uint32
		Coinbase bool
	}

	var thisbal chain.AllUnspentTx

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	wallet.BalanceMutex.Lock()
	for idx, a := range addrs {
		aa, e := btc.NewAddrFromString(a)
		aa.Extra.Label = labels[idx]
		if e==nil {
			var rec *wallet.OneAllAddrBal
			if aa.Version==btc.AddrVerPubkey(common.Testnet) {
				rec = wallet.AllBalancesP2KH[aa.Hash160]
			} else if aa.Version==btc.AddrVerScript(common.Testnet) {
				rec = wallet.AllBalancesP2SH[aa.Hash160]
			} else {
				continue
			}
			if rec!=nil {
				for _, v := range rec.Unsp {
					if qr, vout := v.GetRec(); qr!=nil {
						if oo := qr.Outs[vout]; oo!=nil {
							unsp := &chain.OneUnspentTx{TxPrevOut:btc.TxPrevOut{Hash:qr.TxID, Vout:vout},
								Value:oo.Value, MinedAt:qr.InBlock, Coinbase:qr.Coinbase, BtcAddr:aa}

							thisbal = append(thisbal, unsp)
						}
					}
				}
			}
		}
	}
	wallet.BalanceMutex.Unlock()
	lck.Out.Done()

	buf := new(bytes.Buffer)
	zi := zip.NewWriter(buf)
	was_tx := make(map [[32]byte] bool)

	for i := range thisbal {
		if was_tx[thisbal[i].TxPrevOut.Hash] {
			continue
		}
		was_tx[thisbal[i].TxPrevOut.Hash] = true
		txid := btc.NewUint256(thisbal[i].TxPrevOut.Hash[:])
		fz, _ := zi.Create("balance/" + txid.String() + ".tx")
		if dat, er := common.BlockChain.GetRawTx(thisbal[i].MinedAt, txid); er == nil {
			fz.Write(dat)
		} else {
			println(er.Error())
		}
	}

	fz, _ := zi.Create("balance/unspent.txt")
	for i := range thisbal {
		fmt.Fprintln(fz, thisbal[i].UnspentTextLine())
	}

	zi.Close()
	w.Header()["Content-Type"] = []string{"application/zip"}
	w.Write(buf.Bytes())

}
