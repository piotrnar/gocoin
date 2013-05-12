package main

import (
	"os"
	"fmt"
	"bytes"
	"math/big"
	"crypto/rand"
	"crypto/ecdsa"
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
		if btcsofar >= spendBtc + feeBtc {
			break
		}
	}
	changeBtc = btcsofar - (spendBtc + feeBtc)
	fmt.Printf("Spending %d out of %d outputs...\n", inpcnt+1, len(unspentOuts))

	// Build transaction outputs:
	tx.TxOut = make([]*btc.TxOut, len(sendTo))
	for o := range sendTo {
		tx.TxOut[o] = &btc.TxOut{Value: sendTo[o].amount, Pk_script: sendTo[o].addr.OutScript()}
	}

	if changeBtc > 0 {
		// Add one more output (with the change)
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: changeBtc, Pk_script: get_change_addr().OutScript()})
	}

	//fmt.Println("Unsigned:", hex.EncodeToString(tx.Serialize()))

	for in := range tx.TxIn {
		uo := UO(unspentOuts[in])
		var found bool
		for j := range publ_addrs {
			if publ_addrs[j].Owns(uo.Pk_script) {
				pub_key, e := btc.NewPublicKey(publ_addrs[j].Pubkey)
				if e != nil {
					println("NewPublicKey:", e.Error(), "\007")
					os.Exit(1)
				}

				// Load the key (private and public)
				var key ecdsa.PrivateKey
				key.D = new(big.Int).SetBytes(priv_keys[j][:])
				key.PublicKey = pub_key.PublicKey

				//Calculate proper transaction hash
				h := tx.SignatureHash(uo.Pk_script, in, btc.SIGHASH_ALL)
				//fmt.Println("SignatureHash:", btc.NewUint256(h).String())

				// Sign
				r, s, err := ecdsa.Sign(rand.Reader, &key, h)
				if err != nil {
					println("Sign:", err.Error(), "\007")
					os.Exit(1)
				}
				rb := r.Bytes()
				sb := s.Bytes()

				if rb[0] >= 0x80 { // I thinnk this is needed, thought I am not quite sure... :P
					rb = append([]byte{0x00}, rb...)
				}

				if sb[0] >= 0x80 { // I thinnk this is needed, thought I am not quite sure... :P
					sb = append([]byte{0x00}, sb...)
				}

				// Output the signing result into a buffer, in format expected by bitcoin protocol
				busig := new(bytes.Buffer)
				busig.WriteByte(0x30)
				busig.WriteByte(byte(4+len(rb)+len(sb)))
				busig.WriteByte(0x02)
				busig.WriteByte(byte(len(rb)))
				busig.Write(rb)
				busig.WriteByte(0x02)
				busig.WriteByte(byte(len(sb)))
				busig.Write(sb)
				busig.WriteByte(0x01) // hash type

				// Output the signature and the public key into tx.ScriptSig
				buscr := new(bytes.Buffer)
				buscr.WriteByte(byte(busig.Len()))
				buscr.Write(busig.Bytes())

				buscr.WriteByte(byte(len(publ_addrs[j].Pubkey)))
				buscr.Write(publ_addrs[j].Pubkey)

				// assign:
				tx.TxIn[in].ScriptSig = buscr.Bytes()

				found = true
				break
			}
		}
		if !found {
			fmt.Println("You do not have private key for input number", hex.EncodeToString(uo.Pk_script), "\007")
			os.Exit(1)
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

	f, _ = os.Create("balance/unspent.txt")
	if f != nil {
		for j:=uint(0); j<uint(len(unspentOuts)); j++ {
			if j>inpcnt {
				fmt.Fprintln(f, unspentOuts[j], unspentOutsLabel[j])
			}
		}
		fmt.Println(inpcnt, "spent output(s) removed from 'balance/unspent.txt'")

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
			fmt.Println(addback, "new output(s) appended to 'balance/unspent.txt'")
		}
	}
}
