package main

import (
	"encoding/json"
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"io/ioutil"
	"net/http"
	"os"
)

type restype struct {
	Unspent_outputs []struct {
		Tx_hash       string
		Tx_index      uint64
		Tx_output_n   uint64
		Script        string
		Value         uint64
		Value_hex     string
		Confirmations uint64
	}
}

type onetx struct {
	Hasg string
	Ver uint32
	Lock_time uint32
	In []struct {
		Prev_out struct {
			Hash string
			N uint32
		}
		ScriptSig string
	}
	Out []struct {
		Value string
		ScriptPubKey string
	}
}



func GetTx(id string, vout int) {
	r, er := http.Get("http://blockexplorer.com/rawtx/" + id)
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
				tx.TxIn[i].ScriptSig, _ = btc.DecodeScript(txx.In[i].ScriptSig)
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
			ioutil.WriteFile("balance/"+btc.NewSha2Hash(rawtx).String()+".tx", rawtx, 0666)
		} else {
			println("UNM:", er.Error())
		}
	} else {
		if er != nil {
			println("Get:", er.Error())
		} else {
			println("Status Code", r.StatusCode)
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		println("Give me at least one address")
		return
	}
	url := "http://blockchain.info/unspent?active="
	for i := 1; i < len(os.Args); i++ {
		if i > 1 {
			url += "|"
		}
		url += os.Args[i]
	}

	var sum uint64
	r, er := http.Get(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var r restype
		er = json.Unmarshal(c[:], &r)
		if er == nil {
			os.RemoveAll("balance/")
			os.Mkdir("balance/", os.ModeDir)
			unsp, _ := os.Create("balance/unspent.txt")
			for i := 0; i < len(r.Unspent_outputs); i++ {
				sum += r.Unspent_outputs[i].Value
				pkscr, _ := hex.DecodeString(r.Unspent_outputs[i].Script)
				b58adr := "???"
				if pkscr != nil {
					ba := btc.NewAddrFromPkScript(pkscr, btc.ADDRVER_BTC)
					if ba != nil {
						b58adr = ba.String()
					}
				}
				txid, _ := hex.DecodeString(r.Unspent_outputs[i].Tx_hash)
				if txid != nil {
					txstr := btc.NewUint256(txid).String()
					fmt.Fprintf(unsp, "%s-%03d # %.8f @ %s, %d confs\n",
						txstr, r.Unspent_outputs[i].Tx_output_n,
						float64(r.Unspent_outputs[i].Value) / 1e8,
						b58adr, r.Unspent_outputs[i].Confirmations)

					GetTx(txstr, int(r.Unspent_outputs[i].Tx_output_n))
				}
			}
			unsp.Close()
		} else {
			println(er.Error())
		}
	}
	fmt.Printf("Total %.8f BTC\n", float64(sum)/1e8)
	//println(url)
}
