package utils

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"io/ioutil"
	"net/http"
)

// Download (and re-assemble) raw transaction from blockexplorer.com
func GetTxFromExplorer(txid *btc.Uint256, testnet bool) (rawtx []byte) {
	var url string
	if testnet {
		url = "http://testnet.blockexplorer.com/api/rawtx/" + txid.String()
	} else {
		url = "http://blockexplorer.com/api/rawtx/" + txid.String()
	}
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			defer r.Body.Close()
			c, _ := ioutil.ReadAll(r.Body)
			var txx struct {
				Raw string `json:"rawtx"`
			}
			er = json.Unmarshal(c[:], &txx)
			if er == nil {
				rawtx, er = hex.DecodeString(txx.Raw)
			}
		} else {
			fmt.Println("blockexplorer.com StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("blockexplorer.com:", er.Error())
	}
	return
}


// Download raw transaction from webbtc.com
func GetTxFromWebBTC(txid *btc.Uint256) (raw []byte) {
	url := "https://webbtc.com/tx/" + txid.String() + ".bin"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			raw, _ = ioutil.ReadAll(r.Body)
			r.Body.Close()
		} else {
			fmt.Println("webbtc.com StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("webbtc.com:", er.Error())
	}
	return
}

// Download (and re-assemble) raw transaction from blockexplorer.com
func GetTxFromBlockchainInfo(txid *btc.Uint256) (rawtx []byte) {
	url := "https://blockchain.info/tx/" + txid.String() + "?format=hex"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			defer r.Body.Close()
			rawhex, _ := ioutil.ReadAll(r.Body)
			rawtx, er = hex.DecodeString(string(rawhex))
		} else {
			fmt.Println("blockchain.info StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("blockexplorer.com:", er.Error())
	}
	return
}


// Download raw transaction from a web server (try one after another)
func GetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	raw = GetTxFromExplorer(txid, false)
	if raw != nil && txid.Equal(btc.NewSha2Hash(raw)) {
		//println("GetTxFromExplorer - OK")
		return
	}

	raw = GetTxFromWebBTC(txid)
	if raw != nil && txid.Equal(btc.NewSha2Hash(raw)) {
		//println("GetTxFromWebBTC - OK")
		return
	}

	raw = GetTxFromBlockchainInfo(txid)
	if raw != nil && txid.Equal(btc.NewSha2Hash(raw)) {
		//println("GetTxFromBlockchainInfo - OK")
		return
	}

	return
}


// Download testnet's raw transaction from a web server
func GetTestnetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	raw = GetTxFromExplorer(txid, true)
	if raw != nil && txid.Equal(btc.NewSha2Hash(raw)) {
		//println("GetTxFromExplorer - OK")
		return
	}

	return
}
