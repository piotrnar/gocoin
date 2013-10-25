package webui

import (
	"os"
	"fmt"
	"sync"
	"html"
	"bytes"
	"strings"
	"net/http"
	"io/ioutil"
	"archive/zip"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/config"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/textui"
)


func raw_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Write([]byte(wallet.UpdateBalanceFolder()))
}

func xml_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<unspent>"))

	//For safety, lets get the balance from the main thread
	var wg sync.WaitGroup
	wg.Add(1)
	req := new(textui.OneUiReq)
	req.Done.Add(1)
	req.Handler = func(dat string) {
		for i := range wallet.MyBalance {
			w.Write([]byte("<output>"))
			fmt.Fprint(w, "<txid>", btc.NewUint256(wallet.MyBalance[i].TxPrevOut.Hash[:]).String(), "</txid>")
			fmt.Fprint(w, "<vout>", wallet.MyBalance[i].TxPrevOut.Vout, "</vout>")
			fmt.Fprint(w, "<value>", wallet.MyBalance[i].Value, "</value>")
			fmt.Fprint(w, "<inblock>", wallet.MyBalance[i].MinedAt, "</inblock>")
			fmt.Fprint(w, "<addr>", wallet.MyBalance[i].BtcAddr.String(), "</addr>")
			fmt.Fprint(w, "<label>", html.EscapeString(wallet.MyBalance[i].BtcAddr.Label), "</label>")
			w.Write([]byte("</output>"))
		}
		wg.Done()
	}
	textui.UiChannel <- req
	wg.Wait()
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

	r.ParseForm()

	if checksid(r) && len(r.Form["wal"])>0 {
		wallet.LoadWallet(config.GocoinHomeDir + "wallet" + string(os.PathSeparator) + r.Form["wal"][0])
		http.Redirect(w, r, "/wal", http.StatusFound)
		return
	}

	page := load_template("wallet.html")
	wal1 := load_template("wallet_qsw.html")
	addr := load_template("wallet_adr.html")

	page = strings.Replace(page, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(wallet.LastBalance)/1e8), 1)
	page = strings.Replace(page, "{UNSPENT_OUTS}", fmt.Sprint(len(wallet.MyBalance)), 1)

	fis, er := ioutil.ReadDir(config.GocoinHomeDir+"wallet/")
	if er == nil {
		for i := range fis {
			s := strings.Replace(wal1, "{WALLET_NAME}", fis[i].Name(), -1)
			page = templ_add(page, "<!--ONEWALLET-->", s)
		}
	}

	if wallet.MyWallet!=nil {
		page = strings.Replace(page, "<!--WALLET_FILENAME-->", wallet.MyWallet.FileName, 1)
		wc, er := ioutil.ReadFile(wallet.MyWallet.FileName)
		if er==nil {
			page = strings.Replace(page, "{WALLET_DATA}", string(wc), 1)
		}
		for i := range wallet.MyWallet.Addrs {
			ad := addr
			ad = strings.Replace(ad, "<!--WAL_ADDR-->", wallet.MyWallet.Addrs[i].Enc58str, 1)
			ad = strings.Replace(ad, "<!--WAL_LABEL-->", wallet.MyWallet.Addrs[i].Label, 1)
			if btc, cnt := getbal(wallet.MyWallet.Addrs[i]); btc > 0 {
				ad = strings.Replace(ad, "<!--WAL_BALANCE-->", fmt.Sprintf("%.8f", float64(btc)/1e8), 1)
				ad = strings.Replace(ad, "<!--WAL_OUTCNT-->", fmt.Sprint(cnt), 1)
			}
			page = templ_add(page, "<!--ONE_WALLET_ADDR-->", ad)
		}
		page = strings.Replace(page, "{WALLET_NAME}", filepath.Base(wallet.MyWallet.FileName), 1)
	} else {
		strings.Replace(page, "<!--WALLET_FILENAME-->", "<i>no wallet loaded</i>", 1)
		page = strings.Replace(page, "{WALLET_NAME}", "", -1)
	}

	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}
