package main

import (
	"os"
	"fmt"
	"bytes"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


// apply the chnages to the balance folder
func apply_to_balance(tx *btc.Tx) {
	fmt.Println("Applying the transaction to the balance/ folder...")
	f, _ := os.Create("balance/unspent.txt")
	if f != nil {
		for j:=0; j<len(unspentOuts); j++ {
			if j>len(tx.TxIn) {
				fmt.Fprintln(f, unspentOuts[j], unspentOutsLabel[j])
			}
		}
		if *verbose {
			fmt.Println(len(tx.TxIn), "spent output(s) removed from 'balance/unspent.txt'")
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


// dump hashes to be signed
func dump_hashes_to_sign(tx *btc.Tx) {
	for in := range tx.TxIn {
		uo := UO(unspentOuts[in])
		if uo==nil {
			println("Unknown content of unspent input number", in)
			os.Exit(1)
		}
		pubad := btc.NewAddrFromPkScript(uo.Pk_script, *testnet)
		hash := tx.SignatureHash(uo.Pk_script, in, btc.SIGHASH_ALL)
		fmt.Printf("Input #%d:\n\tHash : %s\n\tAddr : %s\n", in, hex.EncodeToString(hash), pubad.String())
	}
}


// prepare a signed transaction
func sign_tx(tx *btc.Tx) {
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
			fmt.Println("WARNING: You do not have key for", hex.EncodeToString(uo.Pk_script))
		}
	}
}

func write_tx_file(tx *btc.Tx) {
	signedrawtx := tx.Serialize()
	tx.Hash = btc.NewSha2Hash(signedrawtx)

	hs := tx.Hash.String()
	fmt.Println(hs)

	f, _ := os.Create(hs[:8]+".txt")
	if f != nil {
		f.Write([]byte(hex.EncodeToString(signedrawtx)))
		f.Close()
		fmt.Println("Transaction data stored in", hs[:8]+".txt")
	}
}


// prepare a signed transaction
func make_signed_tx() {
	// Make an empty transaction
	tx := new(btc.Tx)
	tx.Version = 1
	tx.Lock_time = 0

	// Select as many inputs as we need to pay the full amount (with the fee)
	var btcsofar uint64
	for inpcnt:=0; inpcnt<len(unspentOuts); inpcnt++ {
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
		fmt.Printf("Spending %d out of %d outputs...\n", len(tx.TxIn), len(unspentOuts))
	}

	// Build transaction outputs:
	for o := range sendTo {
		if sendTo[o].addr.StealthAddr != nil {
			tx.TxOut = append(tx.TxOut, stealth_txout(sendTo[o].addr.StealthAddr, sendTo[o].amount)...)
		} else {
			tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: sendTo[o].amount,
				Pk_script: sendTo[o].addr.OutScript()})
		}
	}

	if changeBtc > 0 {
		// Add one more output (with the change)
		chad := get_change_addr()
		if *verbose {
			fmt.Println("Sending change", changeBtc, "to", chad.String())
		}
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: changeBtc, Pk_script: chad.OutScript()})
	}

	if *message!="" {
		// Add NULL output with an arbitrary message
		scr := new(bytes.Buffer)
		scr.WriteByte(0x6a) // OP_RETURN
		btc.WritePutLen(scr, uint32(len(*message)))
		scr.Write([]byte(*message))
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: 0, Pk_script: scr.Bytes()})
	}

	if *hashes {
		dump_hashes_to_sign(tx)
	} else {
		sign_tx(tx)
		write_tx_file(tx)

		if *apply2bal {
			apply_to_balance(tx)
		}
	}
}


// sign raw transaction with all the keys we have (or just dump hashes to be signed)
func process_raw_tx() {
	tx := raw_tx_from_file(*rawtx)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	if len(unspentOuts) < len(tx.TxIn) {
		println("Insuffcient number of provided unspent outputs", len(unspentOuts), len(tx.TxIn))
		return
	}

	if *hashes {
		dump_hashes_to_sign(tx)
	} else {
		sign_tx(tx)
		write_tx_file(tx)
	}
}
