package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/newec"
	"github.com/piotrnar/gocoin/client/config"
)

func EC_Verify(k, s, h []byte) bool {
	res := newec.Verify(k, s, h)
	if !res {
		config.CountSafe("ECVerifyFail")
	}
	return res
}

func init() {
	fmt.Println("Using NewEC wrapper for EC_Verify")
	btc.EC_Verify = EC_Verify
}
