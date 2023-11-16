package main

import (
	"encoding/hex"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	// modify transaciotn and save it back on disk (e.g. to change a fee)
	if len(os.Args) < 2 {
		println("Specify filename with the tranaction to modify")
		return
	}
	d, er := os.ReadFile(os.Args[1])
	if er != nil {
		println(er.Error())
		return
	}
	tx, _ := btc.NewTx(d) // assuming binary format
	if tx == nil {
		println("Not a valid tx file")
		return
	}
	println(len(tx.TxOut), tx.TxOut[1].Value)
	tx.TxOut[len(tx.TxOut)-1].Value -= 3000 // decrease value of the last output

	// store on tisk as hex-encoded
	os.WriteFile("newtx.txt", []byte(hex.EncodeToString(tx.Serialize())), 0700)
}
