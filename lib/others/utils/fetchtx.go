package utils

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
)

// GetTxFromMempoolSpace downloads (and re-assembles) raw transaction from blockchain.info.
func GetTxFromMempoolSpace(txid *btc.Uint256, chain string) (rawtx []byte) {
	url := "https://mempool.space/" + chain + "api/tx/" + txid.String() + "/raw"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			defer r.Body.Close()
			rawtx, er = io.ReadAll(r.Body)
		} else {
			fmt.Println("mempool.space StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("mempool.space:", er.Error())
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
	raw = GetTxFromBlockstream(txid, "https://blockstream.info/api/tx/")
	if raw != nil && verify_txid(txid, raw) {
		println("GetTxFromBlockstream - OK")
		return
	}

	raw = GetTxFromMempoolSpace(txid, "")
	if raw != nil && verify_txid(txid, raw) {
		println("GetTxFromMempoolSpace - OK")
		return
	}

	raw = GetTxFromBlockchair(txid, "bitcoin")
	if raw != nil && verify_txid(txid, raw) {
		println("GetTxFromBlockchair - OK")
		return
	}

	return
}

// GetTestnetTxFromWeb downloads a testnet's raw transaction from a web server.
func GetTestnetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	raw = GetTxFromBlockstream(txid, "https://blockstream.info/testnet/api/tx/")
	if raw != nil && verify_txid(txid, raw) {
		//println("Testnet GetTxFromBlockstream - OK")
		return
	}

	raw = GetTxFromMempoolSpace(txid, "testnet/")
	if raw != nil && verify_txid(txid, raw) {
		//println("GetTxFromMempoolSpace - OK")
		return
	}

	return
}
