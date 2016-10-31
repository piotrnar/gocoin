package main

import (
	"os"
	"fmt"
	"bytes"
	"strconv"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)


func raw_tx_from_file(fn string) *btc.Tx {
	d, er := ioutil.ReadFile(fn)
	if er != nil {
		fmt.Println(er.Error())
		return nil
	}

	dat, er := hex.DecodeString(string(d))
	if er != nil {
		fmt.Println("hex.DecodeString failed - assume binary transaction file")
		dat = d
	}
	tx, txle := btc.NewTx(dat)

	if tx != nil && txle != len(dat) {
		fmt.Println("WARNING: Raw transaction length mismatch", txle, len(dat))
	}

	return tx
}


func write_tx_file(tx *btc.Tx) {
	signedrawtx := tx.Serialize()
	tx.SetHash(signedrawtx)

	hs := tx.Hash.String()
	fmt.Println(hs)

	f, _ := os.Create(hs[:8]+".txt")
	if f != nil {
		f.Write([]byte(hex.EncodeToString(signedrawtx)))
		f.Close()
		fmt.Println("Transaction data stored in", hs[:8]+".txt")
	}
}


func main() {
	if len(os.Args)!=5 {
		fmt.Println("This tool needs to be executed with 4 arguments:")
		fmt.Println(" 1) Name of the unsigned transaction file")
		fmt.Println(" 2) Input index to add the key & signature to")
		fmt.Println(" 3) Hex dump of the canonical signature")
		fmt.Println(" 4) Hex dump of the public key")
		return
	}
	tx := raw_tx_from_file(os.Args[1])
	if tx==nil {
		return
	}

	in, er := strconv.ParseUint(os.Args[2], 10, 32)
	if er != nil {
		println("Input index:", er.Error())
		return
	}

	if int(in) >= len(tx.TxIn) {
		println("Input index too big:", int(in), "/", len(tx.TxIn))
		return
	}

	sig, er := hex.DecodeString(os.Args[3])
	if er != nil {
		println("Signature:", er.Error())
		return
	}

	pk, er := hex.DecodeString(os.Args[4])
	if er != nil {
		println("Public key:", er.Error())
		return
	}

	buf := new(bytes.Buffer)
	btc.WriteVlen(buf, uint64(len(sig)))
	buf.Write(sig)
	btc.WriteVlen(buf, uint64(len(pk)))
	buf.Write(pk)

	tx.TxIn[in].ScriptSig = buf.Bytes()

	write_tx_file(tx)
}
