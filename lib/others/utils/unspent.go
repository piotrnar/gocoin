package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"io/ioutil"
	"net/http"
	"time"
)

func GetUnspentFromExplorer(addr *btc.BtcAddr, testnet bool) (res utxo.AllUnspentTx, er error) {
	var r *http.Response
	if testnet {
		r, er = http.Get("https://testnet.blockexplorer.com/api/addr/" + addr.String() + "/utxo")
	} else {
		r, er = http.Get("https://blockexplorer.com/api/addr/" + addr.String() + "/utxo")
	}
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
			TxID  string `json:"tx_hash_big_endian"`
			Vout  uint32 `json:"tx_output_n"`
			Value uint64 `json:"value"`
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

func GetUnspentFromBlockcypher(addr *btc.BtcAddr, currency string) (res utxo.AllUnspentTx, er error) {
	var r *http.Response
	var try_cnt int

	for {
		r, er = http.Get("https://api.blockcypher.com/v1/" + currency + "/main/addrs/" + addr.String() + "?unspentOnly=true")

		if er != nil {
			return
		}
		if r.StatusCode == 429 && try_cnt < 5 {
			try_cnt++
			println("Retry blockcypher.com in", try_cnt, "seconds...")
			time.Sleep( time.Duration(try_cnt) * time.Second)
			continue
		}

		if r.StatusCode != 200 {
			er = errors.New(fmt.Sprint("HTTP StatusCode ", r.StatusCode))
			return
		}

		break
	}

	c, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()

	var result struct {
		Addr string `json:"address"`
		Outs []struct {
			TxID   string `json:"tx_hash"`
			Vout   uint32 `json:"tx_output_n"`
			Value  uint64 `json:"value"`
			Height uint32 `json:"block_height"`
		} `json:"txrefs"`
	}

	er = json.Unmarshal(c, &result)
	if er != nil {
		return
	}

	for _, r := range result.Outs {
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

// currency is either "bitcoin" or "bitcoin-cash"
func GetUnspentFromBlockchair(addr *btc.BtcAddr, currency string) (res utxo.AllUnspentTx, er error) {
	var r *http.Response

	// https://api.blockchair.com/bitcoin/outputs?q=is_spent(false),recipient(bc1qdvpxmyvyu9urhadl6sk69gcjsfqsvrjsqfk5aq)
	r, er = http.Get("https://api.blockchair.com/" + currency + "/outputs?q=is_spent(false),recipient(" + addr.String() + ")")

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
		Outs []struct {
			TxID   string `json:"transaction_hash"`
			Vout   uint32 `json:"index"`
			Value  uint64 `json:"value"`
			Height uint32 `json:"block_id"`
			Spent  bool   `json:"is_spent"`
		} `json:"data"`
	}

	er = json.Unmarshal(c, &result)
	if er != nil {
		return
	}

	for _, r := range result.Outs {
		if r.Spent {
			continue
		}
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

func GetUnspent(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	var er error

	res, er = GetUnspentFromBlockchair(addr, "bitcoin")
	if er == nil {
		return
	}
	println("GetUnspentFromBlockchair:", er.Error())

	res, er = GetUnspentFromExplorer(addr, false)
	if er == nil {
		return
	}
	println("GetUnspentFromExplorer:", er.Error())

	res, er = GetUnspentFromBlockcypher(addr, "btc")
	if er == nil {
		return
	}
	println("GetUnspentFromBlockcypher:", er.Error())

	return
}

func GetUnspentTestnet(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	var er error

	res, er = GetUnspentFromExplorer(addr, true)
	if er == nil {
		return
	}
	println("GetUnspentFromExplorer:", er.Error())

	res, er = GetUnspentFromBlockcypher(addr, "btc-testnet")
	if er == nil {
		return
	}
	println("GetUnspentFromBlockcypher:", er.Error())

	return
}
