package main

import (
	"fmt"
	"bytes"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

// hex dump with max 32 bytes per line
func hex_dump(d []byte) (s string) {
	for {
		le := 32
		if len(d) < le {
			le = len(d)
		}
		s += "       " + hex.EncodeToString(d[:le]) + "\n"
		d = d[le:]
		if len(d)==0 {
			return
		}
	}
}


func dump_sigscript(d []byte) {
	if len(d) < 10 + 34 { // at least 10 bytes for sig and 34 bytes key
		fmt.Println("       WARNING: Short sigScript")
		fmt.Print(hex_dump(d))
		return
	}
	rd := bytes.NewReader(d)

	// ECDSA Signature
	le, _ := rd.ReadByte()
	sd := make([]byte, le)
	_, er := rd.Read(sd)
	if er != nil {
		fmt.Println("       WARNING: Signature too short", er.Error())
		fmt.Print(hex_dump(d))
		return
	}
	sig, er := btc.NewSignature(sd)
	if er != nil {
		fmt.Println("       WARNING: Signature broken", er.Error())
		fmt.Print(hex_dump(d))
		return
	}
	fmt.Printf("       R = %64s\n", hex.EncodeToString(sig.R.Bytes()))
	fmt.Printf("       S = %64s\n", hex.EncodeToString(sig.S.Bytes()))
	fmt.Printf("       HashType = %02x\n", sig.HashType)

	// Key
	le, er = rd.ReadByte()
	if er != nil {
		fmt.Println("       WARNING: PublicKey not present")
		fmt.Print(hex_dump(d))
		return
	}
	sd = make([]byte, le)
	_, er = rd.Read(sd)
	if er != nil {
		fmt.Println("       WARNING: PublicKey too short", er.Error())
		fmt.Print(hex_dump(d))
		return
	}
	fmt.Printf("       PublicKeyType = %02x\n", sd[0])
	key, er := btc.NewPublicKey(sd)
	if er != nil {
		fmt.Println("       WARNING: PublicKey broken", er.Error())
		fmt.Print(hex_dump(d))
		return
	}
	fmt.Printf("       X = %64s\n", hex.EncodeToString(key.X.Bytes()))
	if le>=65 {
		fmt.Printf("       Y = %64s\n", hex.EncodeToString(key.Y.Bytes()))
	}

	if rd.Len() != 0 {
		fmt.Println("       WARNING: Extra bytes at the end of sigScript")
		fmt.Print(hex_dump(d[len(d)-rd.Len():]))
	}
}


// dump raw transaction
func dump_raw_tx() {
	tx := raw_tx_from_file(*dumptxfn)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	var unsigned int

	fmt.Println("Version:", tx.Version)
	fmt.Println("TX IN cnt:", len(tx.TxIn))
	for i := range tx.TxIn {
		fmt.Printf("%4d) %s sl=%d seq=%08x\n", i, tx.TxIn[i].Input.String(),
			len(tx.TxIn[i].ScriptSig), tx.TxIn[i].Sequence)
		if len(tx.TxIn[i].ScriptSig) > 0 {
			dump_sigscript(tx.TxIn[i].ScriptSig)
		} else {
			unsigned++
		}
	}
	fmt.Println("TX OUT cnt:", len(tx.TxOut))
	for i := range tx.TxOut {
		fmt.Printf("%4d) %20s BTC to ", i, btc.UintToBtc(tx.TxOut[i].Value))
		addr := btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, *testnet)
		if addr != nil {
			fmt.Println("address", addr.String())
		} else {
			fmt.Println("Pk_script", hex.EncodeToString(tx.TxOut[i].Pk_script))
		}
	}
	fmt.Println("Lock Time:", tx.Lock_time)
	if unsigned>0 {
		fmt.Println("Number of unsigned inputs:", unsigned)
	} else {
		fmt.Println("All the inputs seems to be signed")
	}
}
