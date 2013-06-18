package main

/*
  If you want EC verify operations to work about 150 times faster,
  you can try using a cgo wrapper for secp256k1 lib developer by sipa:
  https://github.com/sipa/secp256k1

  In order to do it, just copy this file one level up (to the
  "client" folder) and try "go build" there.

  If it complains about not being able to build
  "github.com/piotrnar/gocoin/sipasec", either try to fix it,
  or just remove the file you copied and continue in slow mode.
*/

import (
	"fmt"
	"github.com/piotrnar/gocoin/sipasec"
	"github.com/piotrnar/gocoin/btc"
)

func EC_Verify(k, s, h []byte) bool {
	return sipasec.EC_Verify(k, s, h)==1
}

func init() {
	fmt.Println("Using secp256k1 by sipa for EC_Verify")
	btc.EC_Verify = EC_Verify
}
