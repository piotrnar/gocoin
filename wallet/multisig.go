package main

import (
	"fmt"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

const MultiToSignOut = "multi2sign.txt"


// add P2SH pre-signing data into a raw tx
func make_p2sh() {
	tx := raw_tx_from_file(*rawtx)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	d, er := hex.DecodeString(*p2sh)
	if er != nil {
		println("P2SH hex data:", er.Error())
		return
	}

	ms, er := btc.NewMultiSigFromP2SH(d)
	if er != nil {
		println("Decode P2SH:", er.Error())
		return
	}

	fmt.Println("The P2SH data points to address", ms.BtcAddr(testnet).String())

	sd := ms.Bytes()

	for i := range tx.TxIn {
		tx.TxIn[i].ScriptSig = sd
		fmt.Println("Input number", i, " - hash to sign:", hex.EncodeToString(tx.SignatureHash(d, i, btc.SIGHASH_ALL)))
	}
	ioutil.WriteFile(MultiToSignOut, []byte(hex.EncodeToString(tx.Serialize())), 0666)
	fmt.Println("Transaction with", len(tx.TxIn), "inputs ready for multi-signing, stored in", MultiToSignOut)
}


func multisig_sign() {
	tx := raw_tx_from_file(*rawtx)
	if tx == nil {
		println("ERROR: Cannot decode the raw multisig transaction")
		println("Always use -msign <addr> along with -raw multi2sign.txt")
		return
	}

	ad2s, e := btc.NewAddrFromString(*multisign)
	if e != nil {
		println("BTC addr:", e.Error())
		return
	}

	var privkey []byte

	for i := range keys {
		if keys[i].BtcAddr.Hash160==ad2s.Hash160 {
			privkey = keys[i].Key
			break
		}
	}

	if privkey==nil {
		println("You do not know a key for address", ad2s.String())
		return
	}

	for i := range tx.TxIn {
		ms, er := btc.NewMultiSigFromScript(tx.TxIn[i].ScriptSig)
		if er != nil {
			println("WARNING: Input", i, "- not multisig:", er.Error())
			continue
		}
		hash := tx.SignatureHash(ms.P2SH(), i, btc.SIGHASH_ALL)
		//fmt.Println("Input number", i, len(ms.Signatures), " - hash to sign:", hex.EncodeToString(hash))

		r, s, e := btc.EcdsaSign(privkey, hash)
		if e != nil {
			println(e.Error())
			return
		}
		btcsig := &btc.Signature{HashType:0x01}
		btcsig.R.Set(r)
		btcsig.S.Set(s)

		ms.Signatures = append(ms.Signatures, btcsig)
		tx.TxIn[i].ScriptSig = ms.Bytes()
	}

	// Now re-order the signatures as they shall be:
	for i := range tx.TxIn {
		ms, er := btc.NewMultiSigFromScript(tx.TxIn[i].ScriptSig)
		if er != nil {
			//println(er.Error())
			continue
		}
		hash := tx.SignatureHash(ms.P2SH(), i, btc.SIGHASH_ALL)
		//fmt.Println("Input number", i, " - hash to sign:", hex.EncodeToString(hash))
		//fmt.Println(" ... number of signatures:", len(ms.Signatures))

		var sigs []*btc.Signature
		for ki := range ms.PublicKeys {
			//pk := btc.NewPublicKey(ms.PublicKeys[ki])
			//fmt.Println(ki, hex.EncodeToString(ms.PublicKeys[ki]))
			var sig *btc.Signature
			for si := range ms.Signatures {
				if btc.EcdsaVerify(ms.PublicKeys[ki], ms.Signatures[si].Bytes(), hash) {
					//fmt.Println("Key number", ki, "has signature number", si)
					sig = ms.Signatures[si]
					break
				}
			}
			if sig != nil {
				sigs = append(sigs, sig)
			} else if *verbose {
				fmt.Println("WARNING: Key number", ki, "has no matching signature")
			}

			if !*allowextramsigns && uint(len(sigs))>=ms.SigsNeeded {
				break
			}

		}

		if len(ms.Signatures) > len(sigs) {
			fmt.Println("WARNING: Some signatures are obsolete and will be removed", len(ms.Signatures), "=>", len(sigs))
		} else if len(ms.Signatures) < len(sigs) {
			fmt.Println("It appears that same key is re-used.", len(sigs)-len(ms.Signatures), "more signatures were added")
		}
		ms.Signatures = sigs
		tx.TxIn[i].ScriptSig = ms.Bytes()
	}

	write_tx_file(tx)
}
