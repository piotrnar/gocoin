package utils

import (
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"io/ioutil"
	"net/http"
	"os"
)

func GetUnspent(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	type one_unsp struct {
		Address string `json:"address"`
		TxID string `json:"txid"`
		Vout uint32 `json:"vout"`
		Value uint64 `json:"satoshis"`
	}

	r, er := http.Get("https://blockexplorer.com/api/addr/" + addr.String() + "/utxo")
	if er == nil {
		if r.StatusCode != 200 {
			println("get_unspent: StatusCode", r.StatusCode)
			os.Exit(1)
		}

		c, _ := ioutil.ReadAll(r.Body)
		r.Body.Close()

		var result []struct {
			Addr string `json:"address"`
			TxID string `json:"txid"`
			Vout uint32 `json:"vout"`
			Value uint64 `json:"satoshis"`
			Height uint32 `json:"height"`
		}

		er = json.Unmarshal(c, &result)
		if er != nil {
			println("get_unspent:", er.Error())
			os.Exit(1)
		}

		for _, r := range result {
			ur := new(utxo.OneUnspentTx)
			id := btc.NewUint256FromString(r.TxID)
			if id == nil {
				println("Bad TXID:", r.TxID)
				os.Exit(1)
			}
			copy(ur.TxPrevOut.Hash[:], id.Hash[:])
			ur.TxPrevOut.Vout = r.Vout
			ur.Value = r.Value
			ur.MinedAt = r.Height
			ur.BtcAddr = addr
			res = append(res, ur)
		}
	}

	if er != nil {
		println("get_unspent:", er.Error())
	}
	return
}
