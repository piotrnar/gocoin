package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/btc"
)

func TxNotify (idx *btc.TxPrevOut, valpk *btc.TxOut) {
	if valpk!=nil {
		fmt.Println(" + unspent", idx.String(), valpk.String())
	} else {
		fmt.Println(" - unspent", idx.String())
	}
}
