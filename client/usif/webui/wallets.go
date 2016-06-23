package webui

import (
	"io/ioutil"
	"net/http"
	"encoding/hex"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
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

	wallet.BalanceMutex.Lock()
	for _, a := range addrs {
		aa, e := btc.NewAddrFromString(a)
		if e==nil {
			var newrec OneOuts
			if rec, ok := wallet.AllBalances[aa.Hash160]; ok {
				newrec.Value = rec.Value
				for _, v := range rec.Unsp {
					if qr, vout := v.GetRec(); qr!=nil {
						if oo := qr.Outs[vout]; oo!=nil {
							newrec.Outs = append(newrec.Outs, OneOut{
								TxId : hex.EncodeToString(qr.TxID[:]), Vout : vout,
								Value : oo.Value, Height : qr.InBlock, Coinbase : qr.Coinbase})
							}
					}
				}
			}
			out[aa.String()] = newrec
		}
	}
	wallet.BalanceMutex.Unlock()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
