package main

/*
  This is a EC_Verify speedup that is advised for non Windows systems.

  1) Build and install sipa's secp256k1 lib for your system

  2) Copy this file one level up and remove "speedup.go" from there

  3) Rebuild clinet.exe and enjoy sipa's verify lib.
*/

import (
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/cgo/sipasec"
)

func EC_Verify(k, s, h []byte) bool {
	return sipasec.EC_Verify(k, s, h) == 1
}

func init() {
	common.Log.Println("Using libsecp256k1.a of Bitcoin Core for EC_Verify")
	btc.EC_Verify = EC_Verify
}
