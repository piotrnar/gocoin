package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"io/ioutil"
	"net/http"
)

func GetUnspentFromExplorer(addr *btc.BtcAddr) (res utxo.AllUnspentTx, er error) {
	type one_unsp struct {
		Address string `json:"address"`
		TxID    string `json:"txid"`
		Vout    uint32 `json:"vout"`
		Value   uint64 `json:"satoshis"`
	}

	var r *http.Response
	r, er = http.Get("https://blockexplorer.com/api/addr/" + addr.String() + "/utxo")
	if er != nil {
		return
	}
	if r.StatusCode != 200 {
		er = errors.New(fmt.Sprint("HTTP StatusCode ", r.StatusCode))
		return
	}

	c, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()

	var result []struct {
		Addr   string `json:"address"`
		TxID   string `json:"txid"`
		Vout   uint32 `json:"vout"`
		Value  uint64 `json:"satoshis"`
		Height uint32 `json:"height"`
	}

	er = json.Unmarshal(c, &result)
	if er != nil {
		return
	}

	for _, r := range result {
		ur := new(utxo.OneUnspentTx)
		id := btc.NewUint256FromString(r.TxID)
		if id == nil {
			er = errors.New(fmt.Sprint("Bad TXID:", r.TxID))
			return
		}
		copy(ur.TxPrevOut.Hash[:], id.Hash[:])
		ur.TxPrevOut.Vout = r.Vout
		ur.Value = r.Value
		ur.MinedAt = r.Height
		ur.BtcAddr = addr
		res = append(res, ur)
	}

	return
}

func GetUnspentFromBlockchainInfo(addr *btc.BtcAddr) (res utxo.AllUnspentTx, er error) {
	type one_unsp struct {
		Address string `json:"address"`
		TxID    string `json:"txid"`
		Vout    uint32 `json:"vout"`
		Value   uint64 `json:"satoshis"`
	}

	var r *http.Response
	r, er = http.Get("https://blockchain.info/unspent?active=" + addr.String())
	if er != nil {
		return
	}
	if r.StatusCode != 200 {
		er = errors.New(fmt.Sprint("HTTP StatusCode ", r.StatusCode))
		return
	}

	c, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()

	var result struct {
		U []struct {
			TxID   string `json:"tx_hash_big_endian"`
			Vout   uint32 `json:"tx_output_n"`
			Value  uint64 `json:"value"`
		} `json:"unspent_outputs"`
	}

	er = json.Unmarshal(c, &result)
	if er != nil {
		return
	}

	for _, r := range result.U {
		ur := new(utxo.OneUnspentTx)
		id := btc.NewUint256FromString(r.TxID)
		if id == nil {
			er = errors.New(fmt.Sprint("Bad TXID:", r.TxID))
			return
		}
		copy(ur.TxPrevOut.Hash[:], id.Hash[:])
		ur.TxPrevOut.Vout = r.Vout
		ur.Value = r.Value
		//ur.MinedAt = r.Height
		ur.BtcAddr = addr
		res = append(res, ur)
	}

	return
}


func GetUnspent(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	var er error

	res, er = GetUnspentFromExplorer(addr)
	if er == nil {
		return
	}
	println("GetUnspentFromExplorer:", er.Error())

	res, er = GetUnspentFromBlockchainInfo(addr)
	if er == nil {
		return
	}
	println("BlockchainInfo:", er.Error())

	return
}
