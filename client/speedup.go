package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/newec"
)

func EC_Verify(k, s, h []byte) bool {
	res := newec.Verify(k, s, h)
	if !res {
		CountSafe("ECVerifyFail")
	}
	return res
}

func init() {
	fmt.Println("Using NewEC wrapper for EC_Verify")
	btc.EC_Verify = EC_Verify
}
