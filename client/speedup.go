package main

import (
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/newec"
)

func EC_Verify(k, s, h []byte) bool {
	res := newec.Verify(k, s, h)
	if !res {
		println("EC_Verify faield")
		println("pk", hex.EncodeToString(k))
		println("si", hex.EncodeToString(s))
		println("me", hex.EncodeToString(h))
	}
	return res
}

func init() {
	fmt.Println("Using NewEC wrapper for EC_Verify")
	btc.EC_Verify = EC_Verify
}
