package main

import (
	"os"
	"fmt"
	"bufio"
	"strconv"
	"strings"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)


type unspRec struct {
	btc.TxPrevOut
	label string
	key *btc.PrivateAddr
	stealth bool
	spent bool
}

var (
	// set in load_balance():
	unspentOuts []*unspRec
	loadedTxs map[[32]byte] *btc.Tx = make(map[[32]byte] *btc.Tx)
	totBtc uint64
)

func (u *unspRec) String() string {
	return fmt.Sprint(u.TxPrevOut.String(), " ", u.label)
}

func NewUnspRec(l []byte) (uns *unspRec) {
	if l[64]!='-' {
		return nil
	}

	txid := btc.NewUint256FromString(string(l[:64]))
	if txid==nil {
		return nil
	}

	rst := strings.SplitN(string(l[65:]), " ", 2)
	vout, e := strconv.ParseUint(rst[0], 10, 32)
	if e != nil {
		return nil
	}

	uns = new(unspRec)
	uns.TxPrevOut.Hash = txid.Hash
	uns.TxPrevOut.Vout = uint32(vout)
	if len(rst)>1 {
		uns.label = rst[1]
	}

	if first_determ_idx < len(keys) {
		str := string(l)
		if sti:=strings.Index(str, "_StealthC:"); sti!=-1 {
			c, e := hex.DecodeString(str[sti+10:sti+10+64])
			if e != nil {
				fmt.Println("ERROR at stealth", txid.String(), vout, e.Error())
			} else {
				// add a new key to the wallet
				sec := btc.DeriveNextPrivate(keys[first_determ_idx].Key, c)
				rec := btc.NewPrivateAddr(sec, ver_secret(), true) // stealth keys are always compressed
				rec.BtcAddr.Extra.Label = uns.label
				keys = append(keys, rec)
				uns.stealth = true
				uns.key = rec
			}
		}
	}

	if _, ok := loadedTxs[txid.Hash]; !ok {
		loadedTxs[txid.Hash] = tx_from_balance(txid, true)
	}

	return
}


// load the content of the "balance/" folder
func load_balance(showbalance bool) {
	var unknownInputs int
	f, e := os.Open("balance/unspent.txt")
	if e != nil {
		println(e.Error())
		return
	}
	rd := bufio.NewReader(f)
	for {
		l, _, e := rd.ReadLine()
		if len(l)==0 && e!=nil {
			break
		}

		if uns:=NewUnspRec(l); uns!=nil {
			uo := UO(uns)
			if uns.key==nil {
				uns.key = pkscr_to_key(uo.Pk_script)
			}
			// Sum up all the balance and check if we have private key for this input
			if uns.key!=nil {
				totBtc += uo.Value
			} else {
				unknownInputs++
				if *verbose {
					fmt.Println("WARNING: Don't know how to sign", uns.TxPrevOut.String())
				}
			}
			unspentOuts = append(unspentOuts, uns)
		} else {
			println("ERROR in unspent.txt: ", string(l))
		}
	}
	f.Close()
	fmt.Printf("You have %.8f BTC in %d unspent outputs\n",
		float64(totBtc)/1e8, len(unspentOuts)-unknownInputs)
	if showbalance && unknownInputs > 0 {
		fmt.Println("WARNING:", unknownInputs, "unspendable input(s). Add -v switch to print them.")
	}
}

// Get TxOut record, by the given TxPrevOut
func UO(rec *unspRec) *btc.TxOut {
	uns := rec.TxPrevOut
	tx, _ := loadedTxs[uns.Hash]
	if tx==nil {
		println("Unknown content of input", uns.String())
		os.Exit(1)
	}
	return tx.TxOut[uns.Vout]
}

// Look for specific TxPrevOut in unspentOuts
func getUO(pto *btc.TxPrevOut) *btc.TxOut {
	for i := range unspentOuts {
		if unspentOuts[i].Hash==pto.Hash && unspentOuts[i].Vout==pto.Vout {
			return UO(unspentOuts[i])
		}
	}
	return nil
}


// apply the chnages to the balance folder
func apply_to_balance(tx *btc.Tx) {
	f, _ := os.Create("balance/unspent.txt")
	if f != nil {
		// append new outputs at the end of unspentOuts
		ioutil.WriteFile("balance/"+tx.Hash.String()+".tx", tx.Serialize(), 0600)

		fmt.Println("Adding", len(tx.TxOut), "new output(s) to the balance/ folder...")
		for out := range tx.TxOut {
			if k:=pkscr_to_key(tx.TxOut[out].Pk_script); k!=nil {
				uns := new(unspRec)
				uns.key = k
				uns.TxPrevOut.Hash = tx.Hash.Hash
				uns.TxPrevOut.Vout = uint32(out)
				uns.label = fmt.Sprint("# ", btc.UintToBtc(tx.TxOut[out].Value), " BTC @ ", k.BtcAddr.String())
				//stealth bool TODO: maybe we can fix it...
				unspentOuts = append(unspentOuts, uns)
			}
		}

		for j:= range unspentOuts {
			if !unspentOuts[j].spent {
				fmt.Fprintln(f, unspentOuts[j].String())
			}
		}
		f.Close()
	} else {
		println("ERROR: Cannot create balance/unspent.txt")
	}
}
