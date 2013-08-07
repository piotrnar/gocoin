package main

import (
	"os"
	"fmt"
	"sync"
	"bytes"
	"strings"
	"net/http"
	"io/ioutil"
	"archive/zip"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
)


func raw_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Write([]byte(UpdateBalanceFolder()))
}

func xml_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<unspent>"))

	//For safety, lets get the balance from teh main thread
	var wg sync.WaitGroup
	wg.Add(1)
	req := new(oneUiReq)
	req.done.Add(1)
	req.handler = func(dat string) {
		for i := range MyBalance {
			w.Write([]byte("<output>"))
			fmt.Fprint(w, "<txid>", btc.NewUint256(MyBalance[i].TxPrevOut.Hash[:]).String(), "</txid>")
			fmt.Fprint(w, "<vout>", MyBalance[i].TxPrevOut.Vout, "</vout>")
			fmt.Fprint(w, "<value>", MyBalance[i].Value, "</value>")
			fmt.Fprint(w, "<inblock>", MyBalance[i].MinedAt, "</inblock>")
			fmt.Fprint(w, "<addr>", MyBalance[i].BtcAddr.String(), "</addr>")
			fmt.Fprint(w, "<label>", MyBalance[i].BtcAddr.Label, "</label>")
			w.Write([]byte("</output>"))
		}
		wg.Done()
	}
	uiChannel <- req
	wg.Wait()
	w.Write([]byte("</unspent>"))
}


func dl_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	UpdateBalanceFolder()
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


func p_wal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	r.ParseForm()

	if checksid(r) && len(r.Form["wal"])>0 {
		load_wallet(GocoinHomeDir + "wallet" + string(os.PathSeparator) + r.Form["wal"][0])
		http.Redirect(w, r, "/wal", http.StatusFound)
		return
	}

	page := load_template("wallet.html")
	wal1 := load_template("wallet_one.html")

	page = strings.Replace(page, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(LastBalance)/1e8), 1)
	page = strings.Replace(page, "{UNSPENT_OUTS}", fmt.Sprint(len(MyBalance)), 1)

	fis, er := ioutil.ReadDir(GocoinHomeDir+"wallet/")
	if er == nil {
		for i := range fis {
			s := strings.Replace(wal1, "{WALLET_NAME}", fis[i].Name(), -1)
			page = templ_add(page, "<!--ONEWALLET-->", s)
		}
	}

	if MyWallet!=nil {
		page = strings.Replace(page, "<!--WALLET_FILENAME-->", MyWallet.filename, 1)
		wc, er := ioutil.ReadFile(MyWallet.filename)
		if er==nil {
			page = strings.Replace(page, "{WALLET_DATA}", string(wc), 2)
		} else {
			page = strings.Replace(page, "{WALLET_DATA}", "", 2)
		}
		page = strings.Replace(page, "{WALLET_NAME}", filepath.Base(MyWallet.filename), 1)
	} else {
		strings.Replace(page, "<!--WALLET_FILENAME-->", "<i>no wallet loaded</i>", 1)
		page = strings.Replace(page, "{WALLET_DATA}", "", 2)
		page = strings.Replace(page, "{WALLET_NAME}", "", -1)
	}

	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}
