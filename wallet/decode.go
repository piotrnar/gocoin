package main

import (
	"fmt"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/rawtxlib"
)

// dump_raw_tx dumps a raw transaction.
func dump_raw_tx() {
	tx := raw_tx_from_file(*dumptxfn)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		cleanExit(1)
	}
	dump_tx(tx)
}

func getpo(prevout *btc.TxPrevOut) (po *btc.TxOut) {
	if tx := tx_from_balance(btc.NewUint256(prevout.Hash[:]), false); tx != nil {
		if int(prevout.Vout) >= len(tx.TxOut) {
			println("ERROR: Vout TOO BIG (%d/%d)!", int(prevout.Vout), len(tx.TxOut))
		} else {
			po = tx.TxOut[prevout.Vout]
		}

	}
	return
}

func dump_tx(tx *btc.Tx) {
	rawtxlib.Decode(os.Stdout, tx, getpo, testnet, litecoin)
}
