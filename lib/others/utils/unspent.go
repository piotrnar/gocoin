package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
)

// currency is either "bitcoin" or "bitcoin-cash"
func GetUnspentFromBlockchair(addr *btc.BtcAddr, currency string) (res utxo.AllUnspentTx, er error) {
	var r *http.Response
	var try_cnt int

	for {
		// https://api.blockchair.com/bitcoin/outputs?q=is_spent(false),recipient(bc1qdvpxmyvyu9urhadl6sk69gcjsfqsvrjsqfk5aq)
		r, er = http.Get("https://api.blockchair.com/" + currency + "/outputs?q=is_spent(false),recipient(" + addr.String() + ")")

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
			er = errors.New(fmt.Sprint("HTTP StatusCode ", r.StatusCode))
			return
		}
		break
	}

	c, _ := io.ReadAll(r.Body)
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

func GetUnspentFromBlockstream(addr *btc.BtcAddr, api_url string) (res utxo.AllUnspentTx, er error) {
	var r *http.Response

	r, er = http.Get(api_url + addr.String() + "/utxo")

	if er != nil {
		return
	}
	if r.StatusCode != 200 {
		er = errors.New(fmt.Sprint("HTTP StatusCode ", r.StatusCode))
		return
	}

	c, _ := io.ReadAll(r.Body)
	r.Body.Close()

	var result []struct {
		TxID   string `json:"txid"`
		Vout   uint32 `json:"vout"`
		Status struct {
			Confirmed bool   `json:"confirmed"`
			Height    uint32 `json:"block_height"`
		} `json:"status"`
		Value uint64 `json:"value"`
	}

	er = json.Unmarshal(c, &result)
	if er != nil {
		return
	}

	for _, r := range result {
		if !r.Status.Confirmed {
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
		ur.MinedAt = r.Status.Height
		ur.BtcAddr = addr
		res = append(res, ur)
	}

	return
}

func GetUnspent(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	var er error

	res, er = GetUnspentFromBlockstream(addr, "https://blockstream.info/api/address/")
	if er == nil {
		if Verbose {
			println("GetUnspentFromBlockstream OK")
		}
		return
	}
	if Verbose {
		println("GetUnspentFromBlockstream:", er.Error())
	}

	res, er = GetUnspentFromBlockchair(addr, "bitcoin")
	if er == nil {
		if Verbose {
			println("GetUnspentFromBlockchair OK")
		}
		return
	}
	if Verbose {
		println("GetUnspentFromBlockchair:", er.Error())
	}

	return
}

func GetUnspentTestnet(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	var er error

	res, er = GetUnspentFromBlockstream(addr, "https://blockstream.info/testnet/api/address/")
	if er == nil {
		if Verbose {
			println("GetUnspentFromBlockstream OK")
		}
		return
	}
	if Verbose {
		println("GetUnspentFromBlockstream:", er.Error())
	}

	return
}
