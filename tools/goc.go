package main

import (
	"os"
	"fmt"
	"bytes"
	"strings"
	"net/url"
	"net/http"
	"io/ioutil"
	"archive/zip"
	"encoding/xml"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	HOST string
	SID string
)


func http_get(url string) (res []byte) {
	req, _ := http.NewRequest("GET", url, nil)
	if SID!="" {
		req.AddCookie(&http.Cookie{Name:"sid", Value:SID})
	}
	r, er := new(http.Client).Do(req)
	if er != nil {
		println(url, er.Error())
		os.Exit(1)
	}
	if SID=="" {
		for i := range r.Cookies() {
			if r.Cookies()[i].Name=="sid" {
				SID = r.Cookies()[i].Value
				//fmt.Println("sid", SID)
			}
		}
	}
	if r.StatusCode == 200 {
		defer r.Body.Close()
		res, _ = ioutil.ReadAll(r.Body)
	} else {
		println(url, "http.Get returned code", r.StatusCode)
		os.Exit(1)
	}
	return
}


func fetch_balance() {
	os.RemoveAll("balance/")

	d := http_get(HOST+"balance.zip")
	r, er := zip.NewReader(bytes.NewReader(d), int64(len(d)))
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}

	os.Mkdir("balance/", 0777)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		dat, _ := ioutil.ReadAll(rc)
		rc.Close()
		ioutil.WriteFile(f.Name, dat, 0666)
	}
}


func list_wallets() {
	d := http_get(HOST+"wallets.xml")
	var wls struct {
		Wallet [] struct {
			Name string
			Selected bool
		}
	}
	er := xml.Unmarshal(d, &wls)
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	for i := range wls.Wallet {
		fmt.Print(wls.Wallet[i].Name)
		if wls.Wallet[i].Selected {
			fmt.Print(" (selected)")
		}
		fmt.Println()
	}
}

func switch_to_wallet(s string) {
	http_get(HOST+"cfg") // get SID
	u, _ := url.Parse(HOST+"cfg")
	ps := url.Values{}
	ps.Add("sid", SID)
	ps.Add("qwalsel", s)
	u.RawQuery = ps.Encode()
	http_get(u.String())
}


func push_tx(rawtx string) {
	dat := sys.GetRawData(rawtx)
	if dat == nil {
		println("Cannot fetch the raw transaction data (specify hexdump or filename)")
		return
	}

	val := make(url.Values)
	val["rawtx"] = []string{hex.EncodeToString(dat)}

	r, er := http.PostForm(HOST+"txs", val)
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	if r.StatusCode == 200 {
		defer r.Body.Close()
		res, _ := ioutil.ReadAll(r.Body)
		if len(res)>100 {
			txid := btc.NewSha2Hash(dat)
			fmt.Println("TxID", txid.String(), "loaded")

			http_get(HOST+"cfg") // get SID
			//fmt.Println("sid", SID)

			u, _ := url.Parse(HOST+"txs2s.xml")
			ps := url.Values{}
			ps.Add("sid", SID)
			ps.Add("send", txid.String())
			u.RawQuery = ps.Encode()
			http_get(u.String())
		}
	} else {
		println("http.Post returned code", r.StatusCode)
		os.Exit(1)
	}
}


func show_help() {
	fmt.Println("Specify the command and (optionally) its arguments:")
	fmt.Println("  wal [wallet_name] - switch to a given wallet (or list them)")
	fmt.Println("  bal - creates balance/ folder for current wallet")
	fmt.Println("  ptx <rawtx> - pushes raw tx into the network")
}


func main() {
	if len(os.Args)<2 {
		show_help()
		return
	}

	HOST = os.Getenv("GOCOIN_WEBUI")
	if HOST == "" {
		HOST = "http://127.0.0.1:8833/"
	} else {
		if !strings.HasPrefix(HOST, "http://") {
			HOST = "http://" + HOST
		}
		if !strings.HasSuffix(HOST, "/") {
			HOST = HOST + "/"
		}
	}
	fmt.Println("Gocoin WebUI at", HOST, "(you can overwrite it via env variable GOCOIN_WEBUI)")

	switch os.Args[1] {
		case "wal":
			if len(os.Args)>2 {
				switch_to_wallet(os.Args[2])
			} else {
				list_wallets()
			}

		case "bal":
			fetch_balance()

		case "ptx":
			push_tx(os.Args[2])

		default:
			show_help()
	}
}
