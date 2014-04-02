package utils

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"io/ioutil"
	"net/http"
)

type onetx struct {
	Hasg      string
	Ver       uint32
	Lock_time uint32
	Size      uint
	In        []struct {
		Prev_out struct {
			Hash string
			N    uint32
		}
		ScriptSig string
		Coinbase  string
		Sequence  uint32
	}
	Out []struct {
		Value        string
		ScriptPubKey string
	}
}

// Download (and re-assemble) raw transaction from blockexplorer.com
func GetTxFromExplorer(txid *btc.Uint256) ([]byte, []byte) {
	url := "http://blockexplorer.com/rawtx/" + txid.String()
	r, er := http.Get(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var txx onetx
		er = json.Unmarshal(c[:], &txx)
		if er == nil {
			// This part looks weird, but this is how I solved seq=FFFFFFFF, if the field not present:
			for i := range txx.In {
				txx.In[i].Sequence = 0xffffffff
			}
			json.Unmarshal(c[:], &txx)
			// ... end of the weird solution

			tx := new(btc.Tx)
			tx.Version = txx.Ver
			tx.TxIn = make([]*btc.TxIn, len(txx.In))
			for i := range txx.In {
				tx.TxIn[i] = new(btc.TxIn)
				tx.TxIn[i].Input.Hash = btc.NewUint256FromString(txx.In[i].Prev_out.Hash).Hash
				tx.TxIn[i].Input.Vout = txx.In[i].Prev_out.N
				if txx.In[i].Prev_out.N == 0xffffffff &&
					txx.In[i].Prev_out.Hash == "0000000000000000000000000000000000000000000000000000000000000000" {
					tx.TxIn[i].ScriptSig, _ = hex.DecodeString(txx.In[i].Coinbase)
				} else {
					tx.TxIn[i].ScriptSig, _ = btc.DecodeScript(txx.In[i].ScriptSig)
				}
				tx.TxIn[i].Sequence = txx.In[i].Sequence
			}
			tx.TxOut = make([]*btc.TxOut, len(txx.Out))
			for i := range txx.Out {
				am, er := btc.StringToSatoshis(txx.Out[i].Value)
				if er != nil {
					fmt.Println("Incorrect BTC amount", txx.Out[i].Value, er.Error())
					return nil, nil
				}
				tx.TxOut[i] = new(btc.TxOut)
				tx.TxOut[i].Value = am
				tx.TxOut[i].Pk_script, _ = btc.DecodeScript(txx.Out[i].ScriptPubKey)
			}
			tx.Lock_time = txx.Lock_time
			rawtx := tx.Serialize()
			if txx.Size != uint(len(rawtx)) {
				fmt.Printf("Transaction size mismatch: %d expexted, %d decoded\n", txx.Size, len(rawtx))
				return nil, rawtx
			}
			curid := btc.NewSha2Hash(rawtx)
			if !curid.Equal(txid) {
				fmt.Println("The downloaded transaction does not match its ID.", txid.String())
				return nil, rawtx
			}
			return rawtx, rawtx
		} else {
			fmt.Println("json.Unmarshal:", er.Error())
		}
	} else {
		if er != nil {
			fmt.Println("http.Get:", er.Error())
		} else {
			fmt.Println("StatusCode=", r.StatusCode)
		}
	}
	return nil, nil
}


// Download raw transaction from blockr.io
func GetTxFromBlockrIo(txid *btc.Uint256) (raw []byte) {
	url := "http://btc.blockr.io/api/v1/tx/raw/" + txid.String()
	r, er := http.Get(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var txx struct {
			Status string
			Data   struct {
				Tx struct {
					Hex string
				}
			}
		}
		er = json.Unmarshal(c[:], &txx)
		if er == nil {
			raw, _ = hex.DecodeString(txx.Data.Tx.Hex)
		} else {
			println("er", er.Error())
		}
	}
	return
}
