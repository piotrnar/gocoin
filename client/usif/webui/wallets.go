package webui

import (
	"os"
	"fmt"
	"html"
	"bytes"
	"strconv"
	"strings"
	"net/http"
	"io/ioutil"
	"archive/zip"
	"encoding/xml"
	"path/filepath"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)


func raw_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Write([]byte(wallet.UpdateBalanceFolder()))
}

func get_block_time(height uint32) (res uint32) {
	common.Last.Mutex.Lock()
	for bl:=common.Last.Block; bl!=nil && bl.Height>=height; bl=bl.Parent {
		res = bl.Timestamp()
	}
	common.Last.Mutex.Unlock()
	return
}

func xml_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<unspent>"))

	wallet.BalanceMutex.Lock()
	for i := range wallet.MyBalance {
		w.Write([]byte("<output>"))
		fmt.Fprint(w, "<txid>", btc.NewUint256(wallet.MyBalance[i].TxPrevOut.Hash[:]).String(), "</txid>")
		fmt.Fprint(w, "<vout>", wallet.MyBalance[i].TxPrevOut.Vout, "</vout>")
		fmt.Fprint(w, "<value>", wallet.MyBalance[i].Value, "</value>")
		fmt.Fprint(w, "<inblock>", wallet.MyBalance[i].MinedAt, "</inblock>")
		fmt.Fprint(w, "<blocktime>", get_block_time(wallet.MyBalance[i].MinedAt), "</blocktime>")
		fmt.Fprint(w, "<addr>", wallet.MyBalance[i].DestAddr(), "</addr>")
		fmt.Fprint(w, "<addrorg>", wallet.MyBalance[i].BtcAddr.String(), "</addrorg>")
		fmt.Fprint(w, "<wallet>", html.EscapeString(wallet.MyBalance[i].BtcAddr.Extra.Wallet), "</wallet>")
		fmt.Fprint(w, "<label>", html.EscapeString(wallet.MyBalance[i].BtcAddr.Extra.Label), "</label>")
		fmt.Fprint(w, "<virgin>", fmt.Sprint(wallet.MyBalance[i].BtcAddr.Extra.Virgin), "</virgin>")
		w.Write([]byte("</output>"))
	}
	wallet.BalanceMutex.Unlock()
	w.Write([]byte("</unspent>"))
}


func dl_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	wallet.UpdateBalanceFolder()
	buf := new(bytes.Buffer)
	zi := zip.NewWriter(buf)
	filepath.Walk("balance/", func(path string, fi os.FileInfo, err error) error {
		if !fi.IsDir() {
			f, _ := zi.Create(path)
			if f != nil {
				da, _ := ioutil.ReadFile(path)
				f.Write(da)
			}
		}
		return nil
	})
	if zi.Close() == nil {
		w.Header()["Content-Type"] = []string{"application/zip"}
		w.Write(buf.Bytes())
	} else {
		w.Write([]byte("Error"))
	}
}


func getbal(a *btc.BtcAddr) (sum uint64, cnt int) {
	for i := range wallet.MyBalance {
		if wallet.MyBalance[i].BtcAddr.Hash160 == a.Hash160 {
			sum += wallet.MyBalance[i].Value
			cnt++
		}
	}
	return
}


func p_wal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if checksid(r) {
		if len(r.Form["wal"])>0 {
			wallet.LoadWallet(common.CFG.Walletdir + string(os.PathSeparator) + r.Form["wal"][0])
			http.Redirect(w, r, "/wal", http.StatusFound)
			return
		}

		if len(r.Form["setunused"])>0 {
			i, er := strconv.ParseUint(r.Form["setunused"][0], 10, 32)
			if er==nil && int(i)<len(wallet.MyWallet.Addrs) {
				if wallet.MoveToUnused(wallet.MyWallet.Addrs[i].Enc58str, wallet.MyWallet.Addrs[i].Extra.Wallet) {
					wallet.LoadWallet(wallet.MyWallet.FileName)
				}
			}
			http.Redirect(w, r, "/wal", http.StatusFound)
			return
		}

		if len(r.Form["setlabel"])>0 && len(r.Form["lab"])>0 {
			i, er := strconv.ParseUint(r.Form["setlabel"][0], 10, 32)
			if er==nil && int(i)<len(wallet.MyWallet.Addrs) {
				if wallet.SetLabel(int(i), r.Form["lab"][0]) {
					wallet.LoadWallet(wallet.MyWallet.FileName)
				}
			}
			http.Redirect(w, r, "/wal", http.StatusFound)
			return
		}
	}

	page := load_template("wallet.html")
	wal1 := load_template("wallet_qsw.html")
	addr := load_template("wallet_adr.html")

	page = strings.Replace(page, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(wallet.LastBalance)/1e8), 1)
	page = strings.Replace(page, "{UNSPENT_OUTS}", fmt.Sprint(len(wallet.MyBalance)), 1)

	fis, er := ioutil.ReadDir(common.CFG.Walletdir+string(os.PathSeparator))
	if er == nil {
		for i := range fis {
			if !fis[i].IsDir() && fis[i].Size()>1 && fis[i].Name()[0]!='.' {
				s := strings.Replace(wal1, "{WALLET_NAME}", fis[i].Name(), -1)
				page = templ_add(page, "<!--ONEWALLET-->", s)
			}
		}
	}

	wallet.BalanceMutex.Lock()

	if wallet.MyWallet!=nil {
		page = strings.Replace(page, "<!--WALLET_FILENAME-->", wallet.MyWallet.FileName, 1)
		wc, er := ioutil.ReadFile(wallet.MyWallet.FileName)
		if er==nil {
			page = strings.Replace(page, "{WALLET_DATA}", string(wc), 1)
		}
		for i := range wallet.MyWallet.Addrs {
			ad := addr
			lab := wallet.MyWallet.Addrs[i].Extra.Label
			if wallet.MyWallet.Addrs[i].Extra.Virgin {
				lab += " ***"
			}
			ad = strings.Replace(ad, "<!--WAL_ROW_IDX-->", fmt.Sprint(i), -1)
			ad = strings.Replace(ad, "<!--WAL_ADDR-->", wallet.MyWallet.Addrs[i].Enc58str, 1)
			if len(wallet.MyWallet.Addrs[i].Enc58str) > 80 {
				ad = strings.Replace(ad, "<!--WAL_ADDR_STYLE-->", "addr_long", 1)
			} else {
				ad = strings.Replace(ad, "<!--WAL_ADDR_STYLE-->", "addr_norm", 1)
			}
			ad = strings.Replace(ad, "<!--WAL_WALLET-->", html.EscapeString(wallet.MyWallet.Addrs[i].Extra.Wallet), 1)
			ad = strings.Replace(ad, "<!--WAL_LABEL-->", html.EscapeString(lab), 1)

			ms, msr := wallet.IsMultisig(wallet.MyWallet.Addrs[i])
			if ms {
				if msr != nil {
					ad = strings.Replace(ad, "<!--WAL_MULTISIG-->",
						fmt.Sprintf("%d of %d", msr.KeysRequired, msr.KeysProvided), 1)
				} else {
					ad = strings.Replace(ad, "<!--WAL_MULTISIG-->", "Yes", 1)
				}
			} else {
				ad = strings.Replace(ad, "<!--WAL_MULTISIG-->", "No", 1)
			}

			rec := wallet.CachedAddrs[wallet.MyWallet.Addrs[i].Hash160]
			if rec == nil {
				ad = strings.Replace(ad, "<!--WAL_BALANCE-->", "?", 1)
				ad = strings.Replace(ad, "<!--WAL_OUTCNT-->", "?", 1)
				page = templ_add(page, "<!--ONE_WALLET_ADDR-->", ad)
				continue
			}

			if !rec.InWallet {
				ad = strings.Replace(ad, "<!--WAL_BALANCE-->", "WTF", 1)
				ad = strings.Replace(ad, "<!--WAL_OUTCNT-->", "-2", 1)
				page = templ_add(page, "<!--ONE_WALLET_ADDR-->", ad)
				continue
			}

			ucu := wallet.CacheUnspent[rec.CacheIndex]
			if ucu==nil {
				ad = strings.Replace(ad, "<!--WAL_BALANCE-->", "WTF", 1)
				ad = strings.Replace(ad, "<!--WAL_OUTCNT-->", "-3", 1)
				page = templ_add(page, "<!--ONE_WALLET_ADDR-->", ad)
				continue
			}

			if len(ucu.AllUnspentTx) > 0 {
				ad = strings.Replace(ad, "<!--WAL_BALANCE-->", fmt.Sprintf("%.8f", float64(rec.Value)/1e8), 1)
				ad = strings.Replace(ad, "<!--WAL_OUTCNT-->", fmt.Sprint(len(ucu.AllUnspentTx)), 1)
			} else if wallet.MyWallet.Addrs[i].Extra.Virgin {
				// Do not display virgin addresses with zero balance
				continue
			} else if wallet.MyWallet.Addrs[i].Extra.Wallet!=wallet.UnusedFileName &&
				wallet.MyWallet.Addrs[i].Extra.Wallet!=wallet.AddrBookFileName {
				ad = strings.Replace(ad, "<!--WAL_OUTCNT-->",
					fmt.Sprint("<a href=\"javascript:setunused(", i, ")\" title=\"Move to " +
					wallet.UnusedFileName + "\"><img src=\"webui/del.png\"></a>"), 1)
			}
			page = templ_add(page, "<!--ONE_WALLET_ADDR-->", ad)
		}
		page = strings.Replace(page, "{WALLET_NAME}", filepath.Base(wallet.MyWallet.FileName), 1)
	} else {
		strings.Replace(page, "<!--WALLET_FILENAME-->", "<i>no wallet loaded</i>", 1)
		page = strings.Replace(page, "{WALLET_NAME}", "", -1)
	}
	wallet.BalanceMutex.Unlock()

	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}


func xml_wallets(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<Wallets>"))
	fis, er := ioutil.ReadDir(common.CFG.Walletdir+string(os.PathSeparator))
	if er == nil {
		for i := range fis {
			if !fis[i].IsDir() && fis[i].Size()>1 && fis[i].Name()[0]!='.' {
				w.Write([]byte("<Wallet>"))
				w.Write([]byte("<Name>"))
				xml.EscapeText(w, []byte(fis[i].Name()))
				w.Write([]byte("</Name>"))
				selected := wallet.MyWallet!=nil &&
					strings.HasSuffix(wallet.MyWallet.FileName, string(os.PathSeparator) + fis[i].Name())
				w.Write([]byte("<Selected>" + fmt.Sprint(selected) + "</Selected>"))
				w.Write([]byte("</Wallet>"))
			}
		}
	}
	w.Write([]byte("</Wallets>"))
}


func xml_addrs(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}


	w.Write([]byte("<addrbook>"))
	// Address Book
	book := wallet.LoadWalfile(common.CFG.Walletdir+string(os.PathSeparator)+wallet.AddrBookFileName, 0)
	for i := range book {
		w.Write([]byte("<entry>"))
		w.Write([]byte("<addr>" + book[i].Enc58str + "</addr>" ))
		w.Write([]byte("<label>"))
		xml.EscapeText(w, []byte(book[i].Extra.Label))
		w.Write([]byte("</label>"))
		w.Write([]byte("<wallet>"))
		xml.EscapeText(w, []byte(book[i].Extra.Wallet))
		w.Write([]byte("</wallet>"))
		w.Write([]byte("</entry>"))
	}
	w.Write([]byte("</addrbook>"))
}
