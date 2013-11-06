package utils

import (
	"encoding/json"
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"io/ioutil"
	"net/http"
)


type onetx struct {
	Hasg string
	Ver uint32
	Lock_time uint32
	Size uint
	In []struct {
		Prev_out struct {
			Hash string
			N uint32
		}
		ScriptSig string
		Coinbase string
	}
	Out []struct {
		Value string
		ScriptPubKey string
	}
}



func GetTxFromExplorer(txid *btc.Uint256) []byte {
	url := "http://blockexplorer.com/rawtx/" + txid.String()
	println(url)
	r, er := http.Get(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var txx onetx
		er = json.Unmarshal(c[:], &txx)
		if er == nil {
			tx := new(btc.Tx)
			tx.Version = txx.Ver
			tx.TxIn = make([]*btc.TxIn, len(txx.In))
			for i := range txx.In {
				tx.TxIn[i] = new(btc.TxIn)
				tx.TxIn[i].Input.Hash = btc.NewUint256FromString(txx.In[i].Prev_out.Hash).Hash
				tx.TxIn[i].Input.Vout = txx.In[i].Prev_out.N
				if txx.In[i].Prev_out.N==0xffffffff &&
					txx.In[i].Prev_out.Hash=="0000000000000000000000000000000000000000000000000000000000000000" {
					tx.TxIn[i].ScriptSig, _ = hex.DecodeString(txx.In[i].Coinbase)
				} else {
					tx.TxIn[i].ScriptSig, _ = btc.DecodeScript(txx.In[i].ScriptSig)
				}
				tx.TxIn[i].Sequence = 0xffffffff
			}
			tx.TxOut = make([]*btc.TxOut, len(txx.Out))
			for i := range txx.Out {
				tx.TxOut[i] = new(btc.TxOut)
				tx.TxOut[i].Value = btc.ParseValue(txx.Out[i].Value)
				tx.TxOut[i].Pk_script, _ = btc.DecodeScript(txx.Out[i].ScriptPubKey)
			}
			tx.Lock_time = txx.Lock_time
			rawtx := tx.Serialize()
			if txx.Size != uint(len(rawtx)) {
				fmt.Printf("Transaction size mismatch: %d expexted, %d decoded\n", txx.Size, len(rawtx))
				return nil
			}
			curid := btc.NewSha2Hash(rawtx)
			if !curid.Equal(txid) {
				fmt.Println("The downloaded transaction does not match its ID.", txid.String())
				return nil
			}
			return rawtx
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
	return nil
}
