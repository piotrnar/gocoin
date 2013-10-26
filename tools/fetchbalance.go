package main

import (
	"encoding/json"
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"io/ioutil"
	"net/http"
	"os"
	"github.com/piotrnar/gocoin/client/wallet"
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



func GetTx(txid *btc.Uint256, vout int) bool {
	r, er := http.Get("http://blockexplorer.com/rawtx/" + txid.String())
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
			curid := btc.NewSha2Hash(rawtx)
			if !curid.Equal(txid) {
				fmt.Println("The downloaded transaction does not match its ID.")
				return false
			}
			ioutil.WriteFile("balance/"+curid.String()+".tx", rawtx, 0666)
			return true
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
	return false
}


func print_help() {
	fmt.Println("Specify at lest one parameter on the command line.")
	fmt.Println("  Name of one text file containing bitcoin addresses,")
	fmt.Println("... or space separteted bitcoin addresses themselves.")
}


func main() {
	fmt.Println("Gocoin FetchBalnace version", btc.SourcesTag)

	if len(os.Args) < 2 {
		print_help()
		return
	}

	var addrs[] *btc.BtcAddr

	if len(os.Args)==2 {
		fi, er := os.Stat(os.Args[1])
		if er==nil && fi.Size()>10 && !fi.IsDir() {
			wal := wallet.NewWallet(os.Args[1])
			if wal != nil {
				fmt.Println("Found", len(wal.Addrs), "address(es) in", wal.FileName)
				addrs = wal.Addrs
			}
		}
	}

	if len(addrs)==0 {
		for i := 1; i < len(os.Args); i++ {
			a, e := btc.NewAddrFromString(os.Args[i])
			if e != nil {
				println(os.Args[i], ": ", e.Error())
				return
			} else {
				addrs = append(addrs, a)
			}
		}
	}

	if len(addrs)==0 {
		print_help()
		return
	}

	url := "http://blockchain.info/unspent?active="
	for i := range addrs {
		if i > 0 {
			url += "|"
		}
		url += addrs[i].String()
	}

	var sum, outcnt uint64
	r, er := http.Get(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var r restype
		er = json.Unmarshal(c[:], &r)
		if er == nil {
			os.RemoveAll("balance/")
			os.Mkdir("balance/", 0700)
			unsp, _ := os.Create("balance/unspent.txt")
			for i := 0; i < len(r.Unspent_outputs); i++ {
				pkscr, _ := hex.DecodeString(r.Unspent_outputs[i].Script)
				b58adr := "???"
				if pkscr != nil {
					ba := btc.NewAddrFromPkScript(pkscr, btc.ADDRVER_BTC)
					if ba != nil {
						b58adr = ba.String()
					}
				}
				txidlsb, _ := hex.DecodeString(r.Unspent_outputs[i].Tx_hash)
				if txidlsb != nil {
					txid := btc.NewUint256(txidlsb)
					if GetTx(txid, int(r.Unspent_outputs[i].Tx_output_n)) {
						fmt.Fprintf(unsp, "%s-%03d # %.8f @ %s, %d confs\n",
							txid.String(), r.Unspent_outputs[i].Tx_output_n,
							float64(r.Unspent_outputs[i].Value) / 1e8,
							b58adr, r.Unspent_outputs[i].Confirmations)
						sum += r.Unspent_outputs[i].Value
						outcnt++
					} else {
						fmt.Printf(" - cannot fetch %s-%03d\n", txid.String(), r.Unspent_outputs[i].Tx_output_n)
					}
				}
			}
			unsp.Close()
			if outcnt > 0 {
				fmt.Printf("Total %.8f BTC in %d unspent outputs.\n", float64(sum)/1e8, outcnt)
				fmt.Println("The data has been stored in 'balance' folder.")
				fmt.Println("Use it with the wallet app to spend any of it.")
			} else {
				fmt.Println("The fateched balance is empty.")
			}
		} else {
			fmt.Println("Unspent json.Unmarshal", er.Error())
		}
	} else {
		if er != nil {
			fmt.Println("Unspent ", er.Error())
		} else {
			fmt.Println("Unspent HTTP StatusCode", r.StatusCode)
		}
	}
}
