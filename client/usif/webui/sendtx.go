package webui

import (
	"fmt"
	"html"
	"bytes"
	"strings"
	"strconv"
	"net/http"
	"archive/zip"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)


func dl_payment(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var err string

	r.ParseForm()
	if len(r.Form["outcnt"])==1 {
		var thisbal btc.AllUnspentTx
		var pay_cmd string

		outcnt, _ := strconv.ParseUint(r.Form["outcnt"][0], 10, 32)

		wallet.LockBal()
		for i:=1; i<=int(outcnt); i++ {
			is := fmt.Sprint(i)
			if len(r.Form["txout"+is])==1 && r.Form["txout"+is][0]=="on" {
				hash := btc.NewUint256FromString(r.Form["txid"+is][0])
				if hash!=nil {
					vout, er := strconv.ParseUint(r.Form["txvout"+is][0], 10, 32)
					if er==nil {
						var po = btc.TxPrevOut{Hash:hash.Hash, Vout:uint32(vout)}
						for j := range wallet.MyBalance {
							if wallet.MyBalance[j].TxPrevOut==po {
								thisbal = append(thisbal, wallet.MyBalance[j])
							}
						}
					}
				}
			}
		}
		wallet.UnlockBal()

		for i:=1; ; i++ {
			is := fmt.Sprint(i)

			if len(r.Form["adr"+is])!=1 {
				break
			}

			if len(r.Form["btc"+is])!=1 {
				break
			}

			if len(r.Form["adr"+is][0])>1 {
				addr, er := btc.NewAddrFromString(r.Form["adr"+is][0])
				if er == nil {
					am, er := strconv.ParseFloat(r.Form["btc"+is][0], 64)
					if er==nil && am>0 {
						if pay_cmd=="" {
							pay_cmd = "wallet -useallinputs -send "
						} else {
							pay_cmd += ","
						}
						pay_cmd += addr.Enc58str + "=" + fmt.Sprintf("%.8f", am)
					} else {
						err = "Incorrect amount (" + r.Form["btc"+is][0] + ") for Output #" + is
						goto error
					}
				} else {
					err = "Incorrect address (" + r.Form["adr"+is][0] + ") for Output #" + is
					goto error
				}
			}
		}

		if pay_cmd!="" && len(r.Form["txfee"])==1 {
			pay_cmd += " -fee " + r.Form["txfee"][0]
		}

		if pay_cmd!="" && len(r.Form["change"])==1 && len(r.Form["change"][0])>1 {
			pay_cmd += " -change " + r.Form["change"][0]
		}

		buf := new(bytes.Buffer)
		zi := zip.NewWriter(buf)

		was_tx := make(map [[32]byte] bool, len(thisbal))
		for i := range thisbal {
			if was_tx[thisbal[i].TxPrevOut.Hash] {
				continue
			}
			was_tx[thisbal[i].TxPrevOut.Hash] = true
			txid := btc.NewUint256(thisbal[i].TxPrevOut.Hash[:])
			fz, _ := zi.Create("balance/" + txid.String() + ".tx")
			wallet.GetRawTransaction(thisbal[i].MinedAt, txid, fz)
		}

		fz, _ := zi.Create("balance/unspent.txt")
		for i := range thisbal {
			fmt.Fprintf(fz, "%s # %.8f BTC @ %s, %d confs\n", thisbal[i].TxPrevOut.String(),
				float64(thisbal[i].Value)/1e8, thisbal[i].BtcAddr.StringLab(),
				1+common.Last.Block.Height-thisbal[i].MinedAt)
		}

		if pay_cmd!="" {
			fz, _ = zi.Create("pay_cmd.txt")
			fz.Write([]byte(pay_cmd))
		}

		zi.Close()
		w.Header()["Content-Type"] = []string{"application/zip"}
		w.Write(buf.Bytes())
		return
	} else {
		err = "Bad request"
	}
error:
	s := load_template("send_error.html")
	write_html_head(w, r)
	s = strings.Replace(s, "<!--ERROR_MSG-->", err, 1)
	w.Write([]byte(s))
	write_html_tail(w)
}


func p_snd(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	s := load_template("send.html")

	wallet.LockBal()
	if wallet.MyWallet!=nil && len(wallet.MyBalance)>0 {
		wal := load_template("send_wal.html")
		row_tmp := load_template("send_wal_row.html")
		wal = strings.Replace(wal, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(wallet.LastBalance)/1e8), 1)
		wal = strings.Replace(wal, "{UNSPENT_OUTS}", fmt.Sprint(len(wallet.MyBalance)), -1)
		for i := range wallet.MyBalance {
			row := row_tmp
			row = strings.Replace(row, "{ADDR_LABEL}", html.EscapeString(wallet.MyBalance[i].BtcAddr.Label), 1)
			row = strings.Replace(row, "{ROW_NUMBER}", fmt.Sprint(i+1), -1)
			row = strings.Replace(row, "{MINED_IN}", fmt.Sprint(wallet.MyBalance[i].MinedAt), 1)
			row = strings.Replace(row, "{TX_ID}", btc.NewUint256(wallet.MyBalance[i].TxPrevOut.Hash[:]).String(), -1)
			row = strings.Replace(row, "{TX_VOUT}", fmt.Sprint(wallet.MyBalance[i].TxPrevOut.Vout), -1)
			row = strings.Replace(row, "{BTC_AMOUNT}", fmt.Sprintf("%.8f", float64(wallet.MyBalance[i].Value)/1e8), 1)
			row = strings.Replace(row, "{OUT_VALUE}", fmt.Sprint(wallet.MyBalance[i].Value), 1)
			row = strings.Replace(row, "{BTC_ADDR}", wallet.MyBalance[i].BtcAddr.String(), 1)
			wal = templ_add(wal, "<!--UTXOROW-->", row)
		}
		for i := range wallet.MyWallet.Addrs {
			op := "<option value=\"" + wallet.MyWallet.Addrs[i].Enc58str +
				"\">" + wallet.MyWallet.Addrs[i].Enc58str + " (" +
				html.EscapeString(wallet.MyWallet.Addrs[i].Label) + ")</option>"
			//wal = strings.Replace(wal, "<!--ONECHANGEADDR-->", op, 1)
			wal = templ_add(wal, "<!--ONECHANGEADDR-->", op)
		}
		s = strings.Replace(s, "<!--WALLET-->", wal, 1)
	} else {
		if wallet.MyWallet==nil {
			s = strings.Replace(s, "<!--WALLET-->", "You have no wallet", 1)
		} else {
			s = strings.Replace(s, "<!--WALLET-->", "Your current wallet is empty", 1)
		}
	}
	wallet.UnlockBal()

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
