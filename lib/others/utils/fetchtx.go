package utils

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
)

// GetTxFromExplorer downloads (and re-assembles) raw transaction from blockexplorer.com.
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
			c, _ := io.ReadAll(r.Body)
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

// GetTxFromWebBTC downloads a raw transaction from webbtc.com.
func GetTxFromWebBTC(txid *btc.Uint256) (raw []byte) {
	url := "https://webbtc.com/tx/" + txid.String() + ".bin"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			raw, _ = io.ReadAll(r.Body)
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

// GetTxFromBlockchainInfo downloads (and re-assembles) raw transaction from blockchain.info.
func GetTxFromBlockchainInfo(txid *btc.Uint256) (rawtx []byte) {
	url := "https://blockchain.info/tx/" + txid.String() + "?format=hex"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			defer r.Body.Close()
			rawhex, _ := io.ReadAll(r.Body)
			rawtx, er = hex.DecodeString(string(rawhex))
		} else {
			fmt.Println("blockchain.info StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("blockchain.info:", er.Error())
	}
	return
}

// GetTxFromBlockcypher downloads (and re-assembles) raw transaction from blockcypher.com.
func GetTxFromBlockcypher(txid *btc.Uint256, currency string) (rawtx []byte) {
	var r *http.Response
	var er error
	var try_cnt int

	token := os.Getenv("BLOCKCYPHER_TOKEN")
	if token == "" {
		println("WARNING: BLOCKCYPHER_TOKEN envirionment variable not set (get it from blockcypher.com)")
	} else {
		token = "&token=" + token
	}

	url := "https://api.blockcypher.com/v1/" + currency + "/main/txs/" + txid.String() + "?limit=1000&instart=1000&outstart=1000&includeHex=true" + token

	for {
		r, er = http.Get(url)
		if er != nil {
			fmt.Println("blockcypher.com:", er.Error())
			return
		}

		if r.StatusCode == 429 && try_cnt < 5 {
			try_cnt++
			println("Retry blockcypher.com in", try_cnt, "seconds...")
			time.Sleep(time.Duration(try_cnt) * time.Second)
			continue
		}

		break
	}

	if er == nil {
		if r.StatusCode == 200 {
			defer r.Body.Close()
			c, _ := io.ReadAll(r.Body)
			var txx struct {
				Raw string `json:"hex"`
			}
			er = json.Unmarshal(c[:], &txx)
			if er == nil {
				rawtx, er = hex.DecodeString(txx.Raw)
			}
		} else {
			fmt.Println("blockcypher.com StatusCode=", r.StatusCode)
		}
	}
	return
}

func GetTxFromBlockchair(txid *btc.Uint256, currency string) (rawtx []byte) {
	var r *http.Response
	var er error
	var try_cnt int

	for {
		r, er = http.Get("https://api.blockchair.com/" + currency + "/raw/transaction/" + txid.String())

		if er != nil {
			return
		}
		if (r.StatusCode == 402 || r.StatusCode == 429) && try_cnt < 5 {
			try_cnt++
			println("Retry blockchair.com in", try_cnt, "seconds...")
			time.Sleep(time.Duration(try_cnt) * time.Second)
			continue
		}
		if r.StatusCode != 200 {
			return
		}
		break
	}

	c, _ := io.ReadAll(r.Body)
	r.Body.Close()

	var result struct {
		Data map[string]struct {
			Raw string `json:"raw_transaction"`
		} `json:"data"`
	}

	er = json.Unmarshal(c, &result)
	if er != nil {
		return
	}

	if rec, ok := result.Data[txid.String()]; ok {
		rawtx, er = hex.DecodeString(rec.Raw)
	}

	return
}

// GetTxFromBlockstream downloads a raw transaction from webbtc.com.
func GetTxFromBlockstream(txid *btc.Uint256, api_url string) (raw []byte) {
	url := api_url + txid.String() + "/raw"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			raw, _ = io.ReadAll(r.Body)
			r.Body.Close()
		} else {
			fmt.Println("blockstream.info get_tx StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("blockstream.info get_tx:", er.Error())
	}
	return
}

func verify_txid(txid *btc.Uint256, rawtx []byte) bool {
	tx, _ := btc.NewTx(rawtx)
	if tx == nil {
		return false
	}
	tx.SetHash(rawtx)
	return txid.Equal(&tx.Hash)
}

// GetTxFromWeb downloads a raw transaction from a web server (try one after another).
func GetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	//

	raw = GetTxFromBlockstream(txid, "https://blockstream.info/api/tx/")
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromBlockstream - OK")
		return
	}

	raw = GetTxFromBlockchair(txid, "bitcoin")
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromBlockchair - OK")
		return
	}

	raw = GetTxFromExplorer(txid, false)
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromExplorer - OK")
		return
	}

	raw = GetTxFromBlockchainInfo(txid)
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromBlockchainInfo - OK")
		return
	}

	raw = GetTxFromBlockcypher(txid, "btc")
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromBlockcypher - OK")
		return
	}

	raw = GetTxFromWebBTC(txid)
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromWebBTC - OK")
		return
	}

	return
}

// GetTestnetTxFromWeb downloads a testnet's raw transaction from a web server.
func GetTestnetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	raw = GetTxFromBlockstream(txid, "https://blockstream.info/testnet/api/tx/")
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromBlockstream - OK")
		return
	}

	raw = GetTxFromExplorer(txid, true)
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromExplorer - OK")
		return
	}

	raw = GetTxFromBlockcypher(txid, "btc-testnet")
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromBlockcypher - OK")
		return
	}

	return
}
