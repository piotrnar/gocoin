package main

import (
	"os"
	"fmt"
	"sync"
	"bytes"
	"net/http"
	"io/ioutil"
	"archive/zip"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
)

func raw_balance(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(UpdateBalanceFolder()))
}

func xml_balance(w http.ResponseWriter, r *http.Request) {
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
