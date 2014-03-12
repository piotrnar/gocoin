package main

import (
	"fmt"
	"io/ioutil"
	"math/big"
	"encoding/hex"
	"crypto/ecdsa"
	"github.com/piotrnar/gocoin/btc"
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
		println(er.Error())
		return
	}

	fmt.Println("The P2SH data points to address", ms.BtcAddr(*testnet).String())

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
		println(e.Error())
		return
	}

	var privkey *ecdsa.PrivateKey
	//var compr bool

	for i := range publ_addrs {
		if publ_addrs[i].Hash160==ad2s.Hash160 {
			privkey = new(ecdsa.PrivateKey)
			pub, e := btc.NewPublicKey(publ_addrs[i].Pubkey)
			if e != nil {
				println(e.Error())
				return
			}
			privkey.PublicKey = pub.PublicKey
			privkey.D = new(big.Int).SetBytes(priv_keys[i][:])
			//compr = compressed_key[i]
			break
		}
	}

	if privkey==nil {
		println("You do not know a key fro address", ad2s.String())
		return
	}

	for i := range tx.TxIn {
		ms, er := btc.NewMultiSigFromScript(tx.TxIn[i].ScriptSig)
		if er != nil {
			println(er.Error())
			return
		}
		hash := tx.SignatureHash(ms.P2SH(), i, btc.SIGHASH_ALL)
		fmt.Println("Input number", i, len(ms.Signatures), " - hash to sign:", hex.EncodeToString(hash))

		btcsig := &btc.Signature{HashType:0x01}
		btcsig.R, btcsig.S, e = btc.EcdsaSign(privkey, hash)
		if e != nil {
			println(e.Error())
			return
		}

		ms.Signatures = append(ms.Signatures, btcsig)
		tx.TxIn[i].ScriptSig = ms.Bytes()
	}
	write_tx_file(tx)
}
