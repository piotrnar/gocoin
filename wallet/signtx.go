package main

import (
	"os"
	"fmt"
	"bytes"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/ltc"
)


// dump hashes to be signed
func dump_hashes_to_sign(tx *btc.Tx) {
	for in := range tx.TxIn {
		uo := getUO(&tx.TxIn[in].Input)
		if uo==nil {
			println("Unknown content of unspent input number", in)
			os.Exit(1)
		}
		var pubad *btc.BtcAddr
		if litecoin {
			pubad = ltc.NewAddrFromPkScript(uo.Pk_script, testnet)
		} else {
			pubad = btc.NewAddrFromPkScript(uo.Pk_script, testnet)
		}
		if pubad!=nil {
			hash := tx.SignatureHash(uo.Pk_script, in, btc.SIGHASH_ALL)
			fmt.Printf("Input #%d:\n\tHash : %s\n\tAddr : %s\n", in, hex.EncodeToString(hash), pubad.String())
		} else {
			println("Cannot decode pkscript of unspent input number", in)
			os.Exit(1)
		}
	}
}


// prepare a signed transaction
func sign_tx(tx *btc.Tx) (all_signed bool) {
	var multisig_done bool
	all_signed = true

	// go througch each input
	for in := range tx.TxIn {
		if ms, _ := btc.NewMultiSigFromScript(tx.TxIn[in].ScriptSig); ms != nil {
			hash := tx.SignatureHash(ms.P2SH(), in, btc.SIGHASH_ALL)
			for ki := range ms.PublicKeys {
				k := public_to_key(ms.PublicKeys[ki])
				if k != nil {
					r, s, e := btc.EcdsaSign(k.Key, hash)
					if e != nil {
						println("ERROR in sign_tx:", e.Error())
						all_signed = false
					} else {
						btcsig := &btc.Signature{HashType:0x01}
						btcsig.R.Set(r)
						btcsig.S.Set(s)

						ms.Signatures = append(ms.Signatures, btcsig)
						tx.TxIn[in].ScriptSig = ms.Bytes()
						multisig_done = true
					}
				}
			}
		} else {
			uo := getUO(&tx.TxIn[in].Input)
			if uo==nil {
				println("ERROR: Unkown input. Missing balance folder?")
				all_signed = false
				continue
			}
			adr := btc.NewAddrFromPkScript(uo.Pk_script, testnet)
			if adr == nil {
				fmt.Println("WARNING: Don't know how to sign input number", in)
				fmt.Println(" Pk_script:", hex.EncodeToString(uo.Pk_script))
				all_signed = false
				continue
			}
			k := hash_to_key(adr.Hash160)
			if k == nil {
				fmt.Println("WARNING: You do not have key for", adr.String(), "at input", in)
				all_signed = false
				continue
			}
			er := tx.Sign(in, uo.Pk_script, btc.SIGHASH_ALL, k.BtcAddr.Pubkey, k.Key)
			if er != nil {
				fmt.Println("ERROR: Sign failed for input number", in, er.Error())
				all_signed = false
			}
		}
	}

	// reorder signatures if we signed any multisig inputs
	if multisig_done && !multisig_reorder(tx) {
		all_signed = false
	}

	if !all_signed {
		fmt.Println("WARNING: Not all the inputs have been signed")
	}

	return
}

func write_tx_file(tx *btc.Tx) {
	signedrawtx := tx.Serialize()
	tx.Hash = btc.NewSha2Hash(signedrawtx)

	hs := tx.Hash.String()
	fmt.Println("TxID", hs)

	f, _ := os.Create(hs[:8]+".txt")
	if f != nil {
		f.Write([]byte(hex.EncodeToString(signedrawtx)))
		f.Close()
		fmt.Println("Transaction data stored in", hs[:8]+".txt")
	}
}


// prepare a signed transaction
func make_signed_tx() {

	if len(unspentOuts)==0 {
		println("ERROR: balance/ folder does not contain any unspent outputs")
		os.Exit(1)
	}

	// Make an empty transaction
	tx := new(btc.Tx)
	tx.Version = 1
	tx.Lock_time = 0

	// Select as many inputs as we need to pay the full amount (with the fee)
	var btcsofar uint64
	for inpcnt:=0; inpcnt<len(unspentOuts); inpcnt++ {
		uo := getUO(&tx.TxIn[inpcnt].Input)
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
		outs, er := btc.NewSpendOutputs(sendTo[o].addr, sendTo[o].amount, testnet)
		if er != nil {
			fmt.Println("ERROR:", er.Error())
			os.Exit(1)
		}
		tx.TxOut = append(tx.TxOut, outs...)
	}

	if changeBtc > 0 {
		// Add one more output (with the change)
		chad := get_change_addr()
		if *verbose {
			fmt.Println("Sending change", changeBtc, "to", chad.String())
		}
		outs, er := btc.NewSpendOutputs(chad, changeBtc, testnet)
		if er != nil {
			fmt.Println("ERROR:", er.Error())
			os.Exit(1)
		}
		tx.TxOut = append(tx.TxOut, outs...)
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
		signed := sign_tx(tx)
		write_tx_file(tx)

		if apply2bal && signed {
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

	if *hashes {
		dump_hashes_to_sign(tx)
	} else {
		sign_tx(tx)
		write_tx_file(tx)
	}
}
