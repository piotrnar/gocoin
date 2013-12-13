package main

import (
	"os"
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


// prepare a signed transaction
func make_signed_tx() {
	// Make an empty transaction
	tx := new(btc.Tx)
	tx.Version = 1
	tx.Lock_time = 0

	// Select as many inputs as we need to pay the full amount (with the fee)
	var btcsofar uint64
	var inpcnt uint
	for inpcnt=0; inpcnt<uint(len(unspentOuts)); inpcnt++ {
		uo := UO(unspentOuts[inpcnt])
		// add the input to our transaction:
		tin := new(btc.TxIn)
		tin.Input = *unspentOuts[inpcnt]
		tin.Sequence = 0xffffffff
		tx.TxIn = append(tx.TxIn, tin)

		btcsofar += uo.Value
		if !*useallinputs && ( btcsofar >= spendBtc + feeBtc ) {
			break
		}
	}
	changeBtc = btcsofar - (spendBtc + feeBtc)
	if *verbose {
		fmt.Printf("Spending %d out of %d outputs...\n", inpcnt+1, len(unspentOuts))
	}

	// Build transaction outputs:
	tx.TxOut = make([]*btc.TxOut, len(sendTo))
	for o := range sendTo {
		tx.TxOut[o] = &btc.TxOut{Value: sendTo[o].amount, Pk_script: sendTo[o].addr.OutScript()}
	}

	if changeBtc > 0 {
		// Add one more output (with the change)
		chad := get_change_addr()
		if *verbose {
			fmt.Println("Sending change", changeBtc, "to", chad.String())
		}
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: changeBtc, Pk_script: chad.OutScript()})
	}

	//fmt.Println("Unsigned:", hex.EncodeToString(tx.Serialize()))

	for in := range tx.TxIn {
		uo := UO(unspentOuts[in])
		var found bool
		for j := range publ_addrs {
			if publ_addrs[j].Owns(uo.Pk_script) {
				found = true
				er := tx.Sign(in, uo.Pk_script, btc.SIGHASH_ALL, publ_addrs[j].Pubkey, priv_keys[j][:])
				if er != nil {
					fmt.Println("Error signing input", in, "of", len(tx.TxIn))
					fmt.Println("...", er.Error())
				}
				break
			}
		}
		if !found {
			fmt.Println("You do not have private key for input number", hex.EncodeToString(uo.Pk_script))
		}
	}

	rawtx := tx.Serialize()
	tx.Hash = btc.NewSha2Hash(rawtx)

	hs := tx.Hash.String()
	fmt.Println(hs)

	f, _ := os.Create(hs[:8]+".txt")
	if f != nil {
		f.Write([]byte(hex.EncodeToString(rawtx)))
		f.Close()
		fmt.Println("Transaction data stored in", hs[:8]+".txt")
	}

	if *apply2bal {
		fmt.Println("Applying the transaction to the balance/ folder...")
		f, _ = os.Create("balance/unspent.txt")
		if f != nil {
			for j:=uint(0); j<uint(len(unspentOuts)); j++ {
				if j>inpcnt {
					fmt.Fprintln(f, unspentOuts[j], unspentOutsLabel[j])
				}
			}
			if *verbose {
				fmt.Println(inpcnt, "spent output(s) removed from 'balance/unspent.txt'")
			}

			var addback int
			for out := range tx.TxOut {
				for j := range publ_addrs {
					if publ_addrs[j].Owns(tx.TxOut[out].Pk_script) {
						fmt.Fprintf(f, "%s-%03d # %.8f / %s\n", tx.Hash.String(), out,
							float64(tx.TxOut[out].Value)/1e8, publ_addrs[j].String())
						addback++
					}
				}
			}
			f.Close()
			if addback > 0 {
				f, _ = os.Create("balance/"+hs+".tx")
				if f != nil {
					f.Write(rawtx)
					f.Close()
				}
				if *verbose {
					fmt.Println(addback, "new output(s) appended to 'balance/unspent.txt'")
				}
			}
		}
	}
}
