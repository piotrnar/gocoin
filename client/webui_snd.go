package main

import (
	"fmt"
	"html"
//	"time"
	"strings"
//	"runtime"
	"net/http"
	"github.com/piotrnar/gocoin/btc"
)

func p_snd(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	s := load_template("send.html")

	mutex_bal.Lock()
	if MyWallet!=nil && len(MyBalance)>0 {
		wal := load_template("send_wal.html")
		wal = strings.Replace(wal, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(LastBalance)/1e8), 1)
		wal = strings.Replace(wal, "{UNSPENT_OUTS}", fmt.Sprint(len(MyBalance)), 2)
		row := load_template("send_wal_row.html")
		for i := range MyBalance {
			row = strings.Replace(row, "{ADDR_LABEL}", html.EscapeString(MyBalance[i].BtcAddr.Label), 1)
			row = strings.Replace(row, "{ROW_NUMBER}", fmt.Sprint(i+1), -1)
			row = strings.Replace(row, "{MINED_IN}", fmt.Sprint(MyBalance[i].MinedAt), 1)
			row = strings.Replace(row, "{TX_ID}", btc.NewUint256(MyBalance[i].TxPrevOut.Hash[:]).String(), 2)
			row = strings.Replace(row, "{TX_VOUT}", fmt.Sprint(MyBalance[i].TxPrevOut.Vout), 2)
			row = strings.Replace(row, "{BTC_AMOUNT}", fmt.Sprintf("%.8f", float64(MyBalance[i].Value)/1e8), 1)
			row = strings.Replace(row, "{OUT_VALUE}", fmt.Sprint(MyBalance[i].Value), 1)
			row = strings.Replace(row, "{BTC_ADDR}", MyBalance[i].BtcAddr.String(), 1)
			wal = strings.Replace(wal, "<!--UTXOROW-->", row, 1)
		}
		for i := range MyWallet.addrs {
			op := "<option value=\"" + MyWallet.addrs[i].Enc58str +
				"\">" + MyWallet.addrs[i].Enc58str + " (" +
				html.EscapeString(MyWallet.addrs[i].Label) + ")</option>"
			//wal = strings.Replace(wal, "<!--ONECHANGEADDR-->", op, 1)
			wal = templ_add(wal, "<!--ONECHANGEADDR-->", op)
		}
		s = strings.Replace(s, "<!--WALLET-->", wal, 1)
	} else {
		if MyWallet==nil {
			s = strings.Replace(s, "<!--WALLET-->", "You have no wallet", 1)
		} else {
			s = strings.Replace(s, "<!--WALLET-->", "Your current wallet is empty", 1)
		}
	}
	mutex_bal.Unlock()

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
