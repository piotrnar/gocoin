package webui

import (
	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/client/wallet"
)


func dl_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	/*
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
	*/
}


func p_wal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	/*
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

		if len(r.Form["delete_file"])>0 {
			os.Remove(wallet.MyWallet.FileName)
			wallet.LoadWallet(common.CFG.Walletdir + string(os.PathSeparator) + wallet.DefaultFileName)
			http.Redirect(w, r, "/wal", http.StatusFound)
			return
		}
	}
	*/

	page := load_template("wallet.html")
	//addr := load_template("wallet_adr.html")

	page = strings.Replace(page, "{TOTAL_BTC}", "???", 1)
	page = strings.Replace(page, "{UNSPENT_OUTS}", "???", 1)

	wallet.BalanceMutex.Lock()

	/*
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
	*/
	wallet.BalanceMutex.Unlock()

	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}
