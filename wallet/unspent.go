package main

import (
	"os"
	"fmt"
	"bufio"
	"strconv"
	"strings"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	// set in load_balance():
	unspentOuts []*btc.TxPrevOut
	unspentOutsLabel []string
	loadedTxs map[[32]byte] *btc.Tx = make(map[[32]byte] *btc.Tx)
	totBtc uint64
)


// load the content of the "balance/" folder
func load_balance(showbalance bool) {
	var unknownInputs, multisigInputs int
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
		if l[64]=='-' {
			txid := btc.NewUint256FromString(string(l[:64]))
			rst := strings.SplitN(string(l[65:]), " ", 2)
			vout, _ := strconv.ParseUint(rst[0], 10, 32)
			uns := new(btc.TxPrevOut)
			copy(uns.Hash[:], txid.Hash[:])
			uns.Vout = uint32(vout)
			lab := ""
			if len(rst)>1 {
				lab = rst[1]
			}

			str := string(l)
			if sti:=strings.Index(str, "_StealthC:"); sti!=-1 {
				c, e := hex.DecodeString(str[sti+10:sti+10+64])
				if e != nil {
					fmt.Println("ERROR at stealth", txid.String(), vout, e.Error())
				} else {
					// add a new key to the wallet
					sec := btc.DeriveNextPrivate(keys[first_determ_idx].Key, c)
					is_stealth[len(keys)] = true
					rec := btc.NewPrivateAddr(sec, ver_secret(), true) // stealth keys are always compressed
					rec.BtcAddr.Extra.Label = lab
					keys = append(keys, rec)
				}
			}

			if _, ok := loadedTxs[txid.Hash]; !ok {
				tx := tx_from_balance(txid, true)
				loadedTxs[txid.Hash] = tx
			}

			// Sum up all the balance and check if we have private key for this input
			uo := UO(uns)

			add_it := true

			if !btc.IsP2SH(uo.Pk_script) {
				fnd := false
				for j := range keys {
					if keys[j].BtcAddr.Owns(uo.Pk_script) {
						fnd = true
						break
					}
				}

				if !fnd {
					if *onlvalid {
						add_it = false
					}
					if showbalance {
						unknownInputs++
						if *verbose {
							ss := uns.String()
							ss = ss[:8]+"..."+ss[len(ss)-12:]
							fmt.Println(ss, "does not belong to your wallet (cannot sign it)")
						}
					}
				}
			} else {
				if *onlvalid {
					add_it = false
				}
				if *verbose {
					ss := uns.String()
					ss = ss[:8]+"..."+ss[len(ss)-12:]
					fmt.Println(ss, "belongs to a multisig address")
				}
				multisigInputs++
			}

			if add_it {
				unspentOuts = append(unspentOuts, uns)
				unspentOutsLabel = append(unspentOutsLabel, lab)
				totBtc += UO(uns).Value
			}
		}
	}
	f.Close()
	fmt.Printf("You have %.8f BTC in %d unspent outputs. %d inputs are multisig type\n",
		float64(totBtc)/1e8, len(unspentOuts), multisigInputs)
	if showbalance {
		if unknownInputs > 0 {
			fmt.Printf("WARNING: Some inputs (%d) cannot be spent with this password (-v to print them)\n", unknownInputs);
		}
	}
}

// Get TxOut record, by the given TxPrevOut
func UO(uns *btc.TxPrevOut) *btc.TxOut {
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
	fmt.Println("Applying the transaction to the balance/ folder...")
	f, _ := os.Create("balance/unspent.txt")
	if f != nil {
		for j:=len(tx.TxIn); j<len(unspentOuts); j++ {
			fmt.Fprintln(f, unspentOuts[j], unspentOutsLabel[j])
		}
		if *verbose {
			fmt.Println(len(tx.TxIn), "spent output(s) removed from 'balance/unspent.txt'")
		}

		var addback int
		for out := range tx.TxOut {
			for j := range keys {
				if keys[j].BtcAddr.Owns(tx.TxOut[out].Pk_script) {
					fmt.Fprintf(f, "%s-%03d # %.8f BTC @ %s\n", tx.Hash.String(), out,
						float64(tx.TxOut[out].Value)/1e8, keys[j].BtcAddr.String())
					addback++
				}
			}
		}
		f.Close()

		if addback > 0 {
			f, _ = os.Create("balance/"+tx.Hash.String()+".tx")
			if f != nil {
				f.Write(tx.Serialize())
				f.Close()
			}
			if *verbose {
				fmt.Println(addback, "new output(s) appended to 'balance/unspent.txt'")
			}
		}
	}
}
