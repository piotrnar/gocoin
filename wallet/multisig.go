package main

import (
	"fmt"
	"io/ioutil"
	"encoding/hex"
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
		fmt.Println("ERROR: Cannot decode the raw multisig transaction")
		return
	}
	//ioutil.WriteFile("1_"+MultiToSignOut, []byte(hex.EncodeToString(tx.Serialize())), 0666)
	for i := range tx.TxIn {
		dump_raw_sigscript(tx.TxIn[i].ScriptSig)
	}
}
