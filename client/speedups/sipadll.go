package main

import (
	"fmt"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/cgo/sipadll"
)

func init() {
	fmt.Println("Using libsecp256k1-5.dll for EC_Verify, Schnorr_Verify & CheckPayToContract")
	btc.EC_Verify = sipadll.EC_Verify
	btc.Schnorr_Verify = sipadll.Schnorr_Verify
	btc.Check_PayToContract = sipadll.CheckPayToContract
}
